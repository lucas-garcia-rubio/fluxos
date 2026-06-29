package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
	"github.com/lucas-garcia-rubio/fluxos/internal/parse"
	"github.com/lucas-garcia-rubio/fluxos/internal/project"
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
		if err := runTrace(args); err != nil {
			fmt.Fprintf(os.Stderr, "fluxos trace: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "fluxos: unknown command %q\n", cmd)
		usage()
		os.Exit(2)
	}
}

func runTrace(args []string) error {
	fmt.Println("TODO: trace")
	return nil
}

func runIndex(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: fluxos index <path>")
	}
	root := args[0]
	files, err := project.Discover(root)
	if err != nil {
		return err
	}
	for _, f := range files {
		tree, source, err := parse.Parse(f)
		if err != nil {
			return fmt.Errorf("parse %s: %w", f, err)
		}
		types, err := java.Extract(source, tree)
		if err != nil {
			return fmt.Errorf("extract %s: %w", f, err)
		}
		for _, t := range types {
			fmt.Printf("%s: %+v\n", f, t)
		}
		tree.Close()
	}
	return nil
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
