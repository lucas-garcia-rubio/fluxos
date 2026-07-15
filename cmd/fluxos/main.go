package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
	"github.com/lucas-garcia-rubio/fluxos/internal/graph"
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
	allTypes, err := buildIndex(args[0])
	if err != nil {
		return err
	}
	out, err := json.MarshalIndent(allTypes, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	fmt.Println(string(out))
	return nil
}

func runTrace(args []string, out io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: fluxos trace <ClassName.methodName> [project-path]")
	}
	spec := args[0]
	projectRoot := "."
	if len(args) >= 2 {
		projectRoot = args[1]
	}

	// M2 só suporta ClassName.methodName (2 partes). FQCN é M3.
	parts := strings.Split(spec, ".")
	if len(parts) != 2 {
		return fmt.Errorf("invalid spec %q, expected ClassName.methodName", spec)
	}
	className, methodName := parts[0], parts[1]

	allTypes, err := buildIndex(projectRoot)
	if err != nil {
		return err
	}

	targetClass, err := findClassByName(allTypes, className)
	if err != nil {
		return err
	}

	targetMethod, err := findMethodByName(targetClass, methodName)
	if err != nil {
		return err
	}

	resolver := resolve.NewSyntacticResolver(allTypes)
	g := graph.NewGraph()
	graph.Walk(g, targetClass, targetMethod, allTypes, resolver)

	if _, err := fmt.Fprint(out, mermaid.Render(g)); err != nil {
		return fmt.Errorf("write trace: %w", err)
	}
	return nil
}

// buildIndex percorre root, parseia cada .java, extrai TypeDecls. Compartilhado
// entre runIndex (output JSON) e runTrace (lookup antes do trace).
func buildIndex(root string) ([]*java.TypeDecl, error) {
	files, err := project.Discover(root)
	if err != nil {
		return nil, err
	}
	var allTypes []*java.TypeDecl
	for _, f := range files {
		tree, source, err := parse.Parse(f)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", f, err)
		}
		types, err := java.Extract(f, source, tree)
		if err != nil {
			return nil, fmt.Errorf("extract %s: %w", f, err)
		}
		allTypes = append(allTypes, types...)
		tree.Close()
	}
	return allTypes, nil
}

// findClassByName busca linear por tipo com Name == name. Erro se 0 ou >1 matches
// (>1 = nome ambíguo; M3 vai permitir FQCN pra desambiguar).
func findClassByName(types []*java.TypeDecl, name string) (*java.TypeDecl, error) {
	var matches []*java.TypeDecl
	for _, t := range types {
		if t.Name == name {
			matches = append(matches, t)
		}
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("class %q not found", name)
	case 1:
		return matches[0], nil
	default:
		return nil, fmt.Errorf("ambiguous class name %q (%d matches; M3 vai suportar FQCN)", name, len(matches))
	}
}

// findMethodByName busca linear em class.Methods. Mesma lógica do findClassByName
// (overloads = M3).
func findMethodByName(class *java.TypeDecl, name string) (java.MethodDecl, error) {
	var matches []java.MethodDecl
	for _, m := range class.Methods {
		if m.Name == name {
			matches = append(matches, m)
		}
	}
	switch len(matches) {
	case 0:
		return java.MethodDecl{}, fmt.Errorf("method %q not found in %s", name, class.Name)
	case 1:
		return matches[0], nil
	default:
		signatures := make([]string, len(matches))
		for i, method := range matches {
			signatures[i] = method.Signature
		}
		sort.Strings(signatures)
		return java.MethodDecl{}, fmt.Errorf("ambiguous method %q in %s; available signatures: %s", name, class.Name, strings.Join(signatures, ", "))
	}
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
