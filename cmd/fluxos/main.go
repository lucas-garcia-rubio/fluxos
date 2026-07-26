package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
	"github.com/lucas-garcia-rubio/fluxos/internal/graph"
	"github.com/lucas-garcia-rubio/fluxos/internal/index"
	"github.com/lucas-garcia-rubio/fluxos/internal/parse"
	"github.com/lucas-garcia-rubio/fluxos/internal/project"
	"github.com/lucas-garcia-rubio/fluxos/internal/render"
	"github.com/lucas-garcia-rubio/fluxos/internal/render/dot"
	renderjson "github.com/lucas-garcia-rubio/fluxos/internal/render/json"
	"github.com/lucas-garcia-rubio/fluxos/internal/render/mermaid"
	"github.com/lucas-garcia-rubio/fluxos/internal/resolve"
	"github.com/lucas-garcia-rubio/fluxos/internal/trace"
)

func main() {
	os.Exit(runCLI(os.Args[1:], IO{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}))
}

func executeIndex(opts IndexOptions, streams IO) error {
	units, _, err := buildIndex(opts.ProjectRoot, opts.Scope)
	if err != nil {
		return err
	}
	allTypes := flattenTypes(units)
	out, err := json.MarshalIndent(allTypes, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if _, err := fmt.Fprintln(streams.Out, string(out)); err != nil {
		return fmt.Errorf("write index: %w", err)
	}
	return nil
}

func runTrace(args []string, out io.Writer) error {
	return runTraceCommand(args, IO{Out: out})
}

func executeTraceWithService(opts TraceOptions, streams IO, service traceService) error {
	selector := service.selectorFor(opts, streams)
	snapshot, err := buildTraceSnapshotWithSelector(opts, selector)
	if err != nil {
		return err
	}
	if err := renderTrace(streams.Out, snapshot, opts); err != nil {
		return fmt.Errorf("write trace: %w", err)
	}
	return nil
}

func renderTrace(out io.Writer, snapshot render.Snapshot, opts TraceOptions) error {
	switch opts.Format {
	case FormatMermaid:
		return mermaid.Render(out, snapshot, mermaid.Direction(opts.Direction))
	case FormatDOT:
		return dot.Render(out, snapshot)
	case FormatJSON:
		return renderjson.Render(out, snapshot)
	default:
		return fmt.Errorf("unsupported trace format %q", opts.Format)
	}
}

func buildTraceSnapshot(opts TraceOptions) (render.Snapshot, error) {
	return buildTraceSnapshotWithSelector(opts, nil)
}

func buildTraceSnapshotWithSelector(opts TraceOptions, selector trace.ImplementationSelector) (render.Snapshot, error) {
	_, table, err := buildIndex(opts.ProjectRoot, opts.Scope)
	if err != nil {
		return render.Snapshot{}, err
	}

	target, err := ResolveTarget(table, opts.Target)
	if err != nil {
		return render.Snapshot{}, err
	}

	policy, err := dispatchPolicyFor(opts, table)
	if err != nil {
		return render.Snapshot{}, err
	}

	result, err := (trace.Coordinator{}).Build(trace.Request{
		Root:  target.Execution,
		Table: table,
		BuildOptions: graph.BuildOptions{
			MaxDepth: opts.MaxDepth,
			MaxNodes: opts.MaxNodes,
		},
		MaxImpls:   opts.MaxImpls,
		BasePolicy: policy,
		Selector:   selector,
	})
	if err != nil {
		return render.Snapshot{}, err
	}
	return render.NewResultSnapshotWithIncludeUnresolved(result, target.Execution, opts.IncludeUnresolved), nil
}

// dispatchPolicyFor traduz TraceOptions em uma DispatchPolicy. Default mantém
// a TerminalPolicy M3 (sem fan-out). --all-impls=true ativa AllPolicy com o
// MaxImpls configurado. --pick-impls ativa FixedPolicy com fallback
// TerminalPolicy; o mapa de escolhas é pré-validado contra o index antes do
// Build começar, para falhar cedo em scripts declarativos.
func dispatchPolicyFor(opts TraceOptions, table *index.Table) (resolve.DispatchPolicy, error) {
	if len(opts.PickImpls) > 0 {
		if err := validatePickImpls(opts.PickImpls, table); err != nil {
			return nil, err
		}
		return resolve.FixedPolicy{
			Choices:  opts.PickImpls,
			Fallback: resolve.TerminalPolicy{},
			MaxImpls: opts.MaxImpls,
		}, nil
	}
	if opts.AllImpls {
		return resolve.AllPolicy{MaxImpls: opts.MaxImpls}, nil
	}
	return resolve.TerminalPolicy{}, nil
}

// validatePickImpls verifica que cada receiver mapeado é de fato ambíguo
// (interface/abstract com >=2 impls) e que cada FQCN em Explicit é uma
// implementation válida do receiver. Erros são deterministicamente ordenados
// para que scripts falhem cedo com mensagens reproduzíveis.
func validatePickImpls(choices map[string]resolve.FixedChoice, table *index.Table) error {
	receivers := make([]string, 0, len(choices))
	for receiver := range choices {
		receivers = append(receivers, receiver)
	}
	slices.Sort(receivers)

	for _, receiver := range receivers {
		_, ok := table.TypeByFQCN(receiver)
		if !ok {
			return fmt.Errorf("pick-impls: receiver %q not found in project", receiver)
		}
		impls := table.ImplementationsOf(receiver)
		if len(impls) < 2 {
			return fmt.Errorf("pick-impls: receiver %q has %d implementation(s); pick-impls only applies to ambiguous dispatchers (>=2 impls)", receiver, len(impls))
		}

		choice := choices[receiver]
		if choice.Kind != resolve.FixedChoiceExplicit {
			continue
		}
		valid := make(map[string]bool, len(impls))
		for _, impl := range impls {
			valid[impl.FQCN] = true
		}
		for _, fqcn := range choice.Impls {
			if !valid[fqcn] {
				sorted := make([]string, 0, len(impls))
				for _, impl := range impls {
					sorted = append(sorted, impl.FQCN)
				}
				slices.Sort(sorted)
				return fmt.Errorf("pick-impls: candidate %q not an implementation of %q; valid: %v", fqcn, receiver, sorted)
			}
		}
	}
	return nil
}

func buildIndex(root string, scope project.ScopeMode) ([]*java.CompilationUnit, *index.Table, error) {
	units, err := buildUnits(root, scope)
	if err != nil {
		return nil, nil, err
	}
	table, err := index.Build(units)
	if err != nil {
		return nil, nil, err
	}
	return units, table, nil
}

func buildUnits(root string, scope project.ScopeMode) ([]*java.CompilationUnit, error) {
	discoveredProject, err := project.DiscoverWithOptions(root, project.DiscoverOptions{Scope: scope})
	if err != nil {
		return nil, err
	}
	units := make([]*java.CompilationUnit, 0, len(discoveredProject.Files))
	for _, file := range discoveredProject.Files {
		tree, source, err := parse.Parse(file.Path)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", file.Path, err)
		}
		logicalFile, relErr := filepath.Rel(discoveredProject.Root, file.Path)
		if relErr != nil {
			logicalFile = file.Path
		}
		unit, extractErr := java.ExtractUnit(filepath.ToSlash(logicalFile), source, tree)
		tree.Close()
		if extractErr != nil {
			return nil, fmt.Errorf("extract %s: %w", file.Path, extractErr)
		}
		unit.SourceRoot = file.SourceRoot
		units = append(units, unit)
	}
	return units, nil
}

func flattenTypes(units []*java.CompilationUnit) []*java.TypeDecl {
	var allTypes []*java.TypeDecl
	for _, unit := range units {
		allTypes = append(allTypes, unit.Types...)
	}
	return allTypes
}
