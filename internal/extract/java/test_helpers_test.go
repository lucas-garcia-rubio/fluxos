package java

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lucas-garcia-rubio/fluxos/internal/parse"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func parseJavaSource(t *testing.T, source string) (*sitter.Tree, []byte) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "Fixture.java")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("write Java fixture: %v", err)
	}

	tree, parsedSource, err := parse.Parse(path)
	if err != nil {
		t.Fatalf("parse Java fixture: %v", err)
	}
	t.Cleanup(tree.Close)
	if tree.RootNode().HasError() {
		t.Fatalf("Java fixture contains parse errors:\n%s", source)
	}
	return tree, parsedSource
}

func extractJavaSource(t *testing.T, source string) []*TypeDecl {
	t.Helper()
	tree, parsedSource := parseJavaSource(t, source)
	types, err := Extract("Fixture.java", parsedSource, tree)
	if err != nil {
		t.Fatalf("extract Java fixture: %v", err)
	}
	return types
}

func findAllNodesByKind(node *sitter.Node, kind string) []*sitter.Node {
	var result []*sitter.Node
	if node.Kind() == kind {
		result = append(result, node)
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		result = append(result, findAllNodesByKind(node.NamedChild(uint(i)), kind)...)
	}
	return result
}

func findTypeBySimpleName(t *testing.T, types []*TypeDecl, name string) *TypeDecl {
	t.Helper()
	for _, typ := range types {
		if typ.Name == name {
			return typ
		}
	}
	t.Fatalf("type %q not found in %+v", name, types)
	return nil
}

func namedChildKinds(node *sitter.Node) []string {
	result := make([]string, 0, node.NamedChildCount())
	for i := 0; i < int(node.NamedChildCount()); i++ {
		result = append(result, node.NamedChild(uint(i)).Kind())
	}
	return result
}
