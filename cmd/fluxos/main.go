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
	"github.com/lucas-garcia-rubio/fluxos/internal/render/mermaid"
	"github.com/lucas-garcia-rubio/fluxos/internal/resolve"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "index":
		if err := runIndex(args); err != nil {
			fmt.Fprintf(os.Stderr, "fluxos index: %v\n", err)
			os.Exit(1)
		}
	case "trace":
		if err := runTrace(args, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "fluxos trace: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "fluxos: unknown command %q\n", cmd)
		usage()
		os.Exit(2)
	}
}

func runIndex(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: fluxos index <path>")
	}
	units, _, err := buildIndex(args[0])
	if err != nil {
		return err
	}
	allTypes := flattenTypes(units)
	out, err := json.MarshalIndent(allTypes, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	fmt.Println(string(out))
	return nil
}

func runTrace(args []string, out io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: fluxos trace <[FQCN.]TypeName.method[(signature)]> [project-path]")
	}
	spec, err := ParseTargetSpec(args[0])
	if err != nil {
		return err
	}
	projectRoot := "."
	if len(args) >= 2 {
		projectRoot = args[1]
	}

	_, table, err := buildIndex(projectRoot)
	if err != nil {
		return err
	}

	targetClass, targetMethod, err := ResolveTarget(table, spec)
	if err != nil {
		return err
	}

	resolver := resolve.NewSyntacticResolver(table)
	g := graph.NewGraph()
	graph.Walk(g, targetClass, *targetMethod, table, resolver)

	if _, err := fmt.Fprint(out, mermaid.Render(g)); err != nil {
		return fmt.Errorf("write trace: %w", err)
	}
	return nil
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

func usage() {
	fmt.Fprintln(os.Stderr, "usage: fluxos <command>")
	fmt.Fprintln(os.Stderr, "commands: index, trace")
}
