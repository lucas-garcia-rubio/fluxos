package java

import sitter "github.com/tree-sitter/go-tree-sitter"

// ExtractUnit preserva os metadados que pertencem ao arquivo e extrai seus
// tipos top-level. SourceRoot será preenchido quando o modelo de projeto for
// introduzido no Passo 3.
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
		switch child.Kind() {
		case "class_declaration":
			unit.Types = append(unit.Types, extractTypeDecl(filePath, source, child, pkg, TypeKindClass))
		case "interface_declaration":
			unit.Types = append(unit.Types, extractTypeDecl(filePath, source, child, pkg, TypeKindInterface))
		case "enum_declaration":
			unit.Types = append(unit.Types, extractTypeDecl(filePath, source, child, pkg, TypeKindEnum))
		case "record_declaration":
			unit.Types = append(unit.Types, extractTypeDecl(filePath, source, child, pkg, TypeKindRecord))
		}
	}

	return unit, nil
}
