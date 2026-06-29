// Package parse isola o uso do tree-sitter (incluindo CGO) do resto do projeto.
// Tudo que envolve resources gerenciados por C fica confinado neste pacote.
package parse

import (
	"fmt"
	"os"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_java "github.com/tree-sitter/tree-sitter-java/bindings/go"
)

func Parse(path string) (*sitter.Tree, []byte, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}

	parser := sitter.NewParser()
	defer parser.Close()

	lang := sitter.NewLanguage(tree_sitter_java.Language())
	err = parser.SetLanguage(lang)
	if err != nil {
		return nil, nil, fmt.Errorf("set language: %w", err)
	}

	tree := parser.Parse(source, nil)
	return tree, source, nil
}
