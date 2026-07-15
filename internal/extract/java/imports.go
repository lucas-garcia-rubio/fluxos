package java

import sitter "github.com/tree-sitter/go-tree-sitter"

func extractImports(source []byte, root *sitter.Node) []ImportDecl {
	imports := make([]ImportDecl, 0)
	for i := 0; i < int(root.NamedChildCount()); i++ {
		node := root.NamedChild(uint(i))
		if node.Kind() != "import_declaration" {
			continue
		}

		decl := ImportDecl{}
		for j := 0; j < int(node.ChildCount()); j++ {
			child := node.Child(uint(j))
			switch child.Kind() {
			case "static":
				decl.Static = true
			case "asterisk":
				decl.Wildcard = true
			case "identifier", "scoped_identifier":
				if decl.Target == "" {
					decl.Target = sourceText(source, child)
				}
			}
		}
		imports = append(imports, decl)
	}
	return imports
}
