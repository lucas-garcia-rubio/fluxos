package java

import (
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// extractLocalVars percorre o body de um método e devolve as variáveis locais
// declaradas explicitamente, com byte range do bloco onde foram declaradas.
// Resolver usa esses ranges para fazer scoped lookup com shadowing e
// decl-before-use.
func extractLocalVars(source []byte, methodNode *sitter.Node) []LocalVarDecl {
	body := methodNode.ChildByFieldName("body")
	if body == nil {
		return nil
	}
	var out []LocalVarDecl
	walkAndCollectLocalVars(body, source, uint(body.StartByte()), uint(body.EndByte()), &out)
	return out
}

func walkAndCollectLocalVars(node *sitter.Node, source []byte, scopeStart, scopeEnd uint, out *[]LocalVarDecl) {
	if isNestedExecutableBoundary(node) {
		return
	}
	if node.Kind() == "block" {
		scopeStart = uint(node.StartByte())
		scopeEnd = uint(node.EndByte())
	}
	if node.Kind() == "local_variable_declaration" {
		typeNode := node.ChildByFieldName("type")
		if typeNode != nil && sourceText(source, typeNode) != "var" {
			typeRef := NewTypeRef(sourceText(source, typeNode), false)
			for i := 0; i < int(node.NamedChildCount()); i++ {
				child := node.NamedChild(uint(i))
				if child.Kind() != "variable_declarator" {
					continue
				}
				nameNode := child.ChildByFieldName("name")
				if nameNode == nil {
					continue
				}
				*out = append(*out, LocalVarDecl{
					Name:       sourceText(source, nameNode),
					Type:       typeRef,
					ScopeStart: scopeStart,
					ScopeEnd:   scopeEnd,
					DeclStart:  uint(child.StartByte()),
				})
			}
		}
	}

	for i := 0; i < int(node.NamedChildCount()); i++ {
		walkAndCollectLocalVars(node.NamedChild(uint(i)), source, scopeStart, scopeEnd, out)
	}
}
