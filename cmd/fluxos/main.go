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
	"github.com/lucas-garcia-rubio/fluxos/internal/render/mermaid"
	"github.com/lucas-garcia-rubio/fluxos/internal/resolve"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

func main() {
	os.Exit(runCLI(os.Args[1:], IO{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}))
}

func executeIndex(opts IndexOptions, streams IO) error {
	units, _, err := buildIndex(opts.ProjectRoot)
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
	if _, err := fmt.Fprint(streams.Out, mermaid.RenderSnapshot(snapshot)); err != nil {
		return fmt.Errorf("write trace: %w", err)
	}
	return nil
}

func buildTraceSnapshot(opts TraceOptions) (render.Snapshot, error) {
	_, table, err := buildIndex(opts.ProjectRoot)
	if err != nil {
		return render.Snapshot{}, err
	}

	targetClass, targetMethod, err := ResolveTarget(table, opts.Target)
	if err != nil {
		return render.Snapshot{}, err
	}

	resolver := resolve.NewSyntacticResolver(table)
	g := graph.NewGraph()
	graph.Walk(g, targetClass, *targetMethod, table, resolver)

	targetHandle := resolve.MethodHandle{
		TypeFQCN:  targetClass.FQCN,
		Method:    targetMethod.Name,
		Signature: targetMethod.Signature,
	}
	return render.NewSnapshot(g, targetHandle), nil
}

func buildIndex(root string) ([]*java.CompilationUnit, *index.Table, error) {
	units, err := buildUnits(root)
	if err != nil {
		return nil, nil, err
	}
	table, err := index.Build(units)
	if err != nil {
		return nil, nil, err
	}
	return units, table, nil
}

func buildUnits(root string) ([]*java.CompilationUnit, error) {
	discoveredProject, err := project.Discover(root)
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
