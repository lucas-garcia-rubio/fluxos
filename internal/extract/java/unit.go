package java

import sitter "github.com/tree-sitter/go-tree-sitter"

// ExtractUnit preserva os metadados que pertencem ao arquivo e extrai seus
// tipos top-level e named member types em source preorder.
func ExtractUnit(filePath string, source []byte, tree *sitter.Tree) (*CompilationUnit, error) {
	root := tree.RootNode()
	pkg := extractPackage(source, root)
	unit := &CompilationUnit{
		File:    filePath,
		Package: pkg,
		Imports: extractImports(source, root),
		Types:   make([]*TypeDecl, 0),
	}

	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(uint(i))
		if _, ok := declarationTypeKind(child.Kind()); ok {
			collectTypeDecls(unit, filePath, source, child, pkg, "")
		}
	}

	return unit, nil
}

func collectTypeDecls(unit *CompilationUnit, filePath string, source []byte, node *sitter.Node, pkg, enclosingFQCN string) {
	kind, ok := declarationTypeKind(node.Kind())
	if !ok {
		return
	}
	decl := extractTypeDecl(filePath, source, node, pkg, enclosingFQCN, kind)
	unit.Types = append(unit.Types, decl)
	if body := node.ChildByFieldName("body"); body != nil {
		for _, member := range typeBodyMembers(body) {
			if _, nested := declarationTypeKind(member.Kind()); nested {
				collectTypeDecls(unit, filePath, source, member, pkg, decl.FQCN)
			}
		}
	}
}

func declarationTypeKind(kind string) (TypeKind, bool) {
	switch kind {
	case "class_declaration":
		return TypeKindClass, true
	case "interface_declaration":
		return TypeKindInterface, true
	case "enum_declaration":
		return TypeKindEnum, true
	case "record_declaration":
		return TypeKindRecord, true
	default:
		return 0, false
	}
}
