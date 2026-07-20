package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
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

func executeTrace(opts TraceOptions, streams IO) error {
	snapshot, err := buildTraceSnapshot(opts)
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
	_, table, err := buildIndex(opts.ProjectRoot, opts.Scope)
	if err != nil {
		return render.Snapshot{}, err
	}

	target, err := ResolveTarget(table, opts.Target)
	if err != nil {
		return render.Snapshot{}, err
	}

	resolver := resolve.NewSyntacticResolverWithPolicy(table, dispatchPolicyFor(opts))
	result := graph.Build(target.Execution, table, resolver, graph.BuildOptions{
		MaxDepth: opts.MaxDepth,
		MaxNodes: opts.MaxNodes,
	})
	return render.NewResultSnapshot(result, target.Execution), nil
}

// dispatchPolicyFor traduz TraceOptions em uma DispatchPolicy. Default mantém
// a TerminalPolicy M3 (sem fan-out). --all-impls=true ativa AllPolicy com o
// MaxImpls configurado.
func dispatchPolicyFor(opts TraceOptions) resolve.DispatchPolicy {
	if !opts.AllImpls {
		return resolve.TerminalPolicy{}
	}
	return resolve.AllPolicy{MaxImpls: opts.MaxImpls}
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

func walk(node *tree_sitter.Node, depth int, currentFieldName string) {
	indent := strings.Repeat("   ", depth)
	if currentFieldName != "" {
		fmt.Printf("%s%s: %s\n", indent, currentFieldName, node.Kind())
	} else {
		fmt.Printf("%s%s\n", indent, node.Kind())
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child.IsNamed() {
			childFieldName := node.FieldNameForChild(uint32(i))
			walk(child, depth+1, childFieldName)
		}
	}
}
