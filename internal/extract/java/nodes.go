package java

import (
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Extract percorre a AST a partir de tree.RootNode() e devolve todas as
// declarações de tipo encontradas (classes, interfaces, enums, records).
// source são os bytes do arquivo — necessário pra extrair texto dos nós.
// Fields e Methods dentro de cada TypeDecl não são preenchidos aqui
// (isso fica pra Passo 7).
func Extract(source []byte, tree *sitter.Tree) ([]*TypeDecl, error) {
	root := tree.RootNode()
	pkg := extractPackage(source, root)

	var types []*TypeDecl
	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(uint(i))
		switch child.Kind() {
		case "class_declaration":
			types = append(types, extractTypeDecl(source, child, pkg, TypeKindClass))
		case "interface_declaration":
			types = append(types, extractTypeDecl(source, child, pkg, TypeKindInterface))
		case "enum_declaration":
			types = append(types, extractTypeDecl(source, child, pkg, TypeKindEnum))
		case "record_declaration":
			types = append(types, extractTypeDecl(source, child, pkg, TypeKindRecord))
		}
	}
	return types, nil
}

// extractPackage acha a declaração de package no nível superior e devolve o FQCN.
// Devolve "" se o arquivo está no package default.
func extractPackage(source []byte, root *sitter.Node) string {
	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(uint(i))
		if child.Kind() != "package_declaration" {
			continue
		}
		// scoped_identifier é filho nomeado posicional (sem field name) — achar por Kind.
		for j := 0; j < int(child.NamedChildCount()); j++ {
			inner := child.NamedChild(uint(j))
			if inner.Kind() == "scoped_identifier" || inner.Kind() == "identifier" {
				return sourceText(source, inner)
			}
		}
	}
	return ""
}

func extractTypeDecl(source []byte, node *sitter.Node, pkg string, kind TypeKind) *TypeDecl {
	decl := &TypeDecl{Kind: kind}

	if nameNode := node.ChildByFieldName("name"); nameNode != nil {
		decl.Name = sourceText(source, nameNode)
	}
	if pkg != "" && decl.Name != "" {
		decl.FQCN = pkg + "." + decl.Name
	} else {
		decl.FQCN = decl.Name
	}

	// Superclass só faz sentido em classe. O nó `superclass` é um wrapper em
	// volta do `type_identifier` (range inclui a keyword "extends") — drilla.
	if kind == TypeKindClass {
		if super := node.ChildByFieldName("superclass"); super != nil {
			if inner := findFirstByKind(super, "type_identifier"); inner != nil {
				decl.SuperClass = sourceText(source, inner)
			}
		}
	}

	// Interfaces: field name varia por Kind (ver TREESITTER.md, "assimetria pegadinha").
	// O wrapper (super_interfaces/extends_interfaces) contém um `type_list`,
	// que por sua vez contém os `type_identifier`s. Drilla dois níveis.
	var ifNode *sitter.Node
	for _, field := range []string{"super_interfaces", "extends_interfaces", "interfaces"} {
		if n := node.ChildByFieldName(field); n != nil {
			ifNode = n
			break
		}
	}
	if ifNode != nil {
		list := ifNode
		if ifNode.Kind() != "type_list" {
			list = findFirstByKind(ifNode, "type_list")
		}
		if list != nil {
			decl.Interfaces = extractTypeList(source, list)
		}
	}

	return decl
}

// findFirstByKind devolve o primeiro filho nomeado de node com o Kind dado,
// ou nil se não encontrar.
func findFirstByKind(node *sitter.Node, kind string) *sitter.Node {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		c := node.NamedChild(uint(i))
		if c.Kind() == kind {
			return c
		}
	}
	return nil
}

func extractModifiers(source []byte, node *sitter.Node) []string {
	modsNode := node.ChildByFieldName("modifiers")
	if modsNode == nil {
		return nil
	}
	var mods []string
	for i := 0; i < int(modsNode.NamedChildCount()); i++ {
		child := modsNode.NamedChild(uint(i))
		mods = append(mods, sourceText(source, child))
	}
	return mods
}

func extractTypeList(source []byte, node *sitter.Node) []string {
	var result []string
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(uint(i))
		result = append(result, sourceText(source, child))
	}
	return result
}

func sourceText(source []byte, node *sitter.Node) string {
	return string(source[node.StartByte():node.EndByte()])
}
