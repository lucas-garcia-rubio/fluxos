package java

import (
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// extractLocalVars percorre o body de um método e devolve as variáveis locais
// declaradas explicitamente, indexadas por nome. M2 trata o método inteiro como
// um único escopo e não infere o tipo de declarações com var.
func extractLocalVars(source []byte, methodNode *sitter.Node) map[string]string {
	body := methodNode.ChildByFieldName("body")
	if body == nil {
		return nil
	}

	localVars := make(map[string]string)
	walkAndCollectLocalVars(body, source, localVars)
	if len(localVars) == 0 {
		return nil
	}
	return localVars
}

func walkAndCollectLocalVars(node *sitter.Node, source []byte, localVars map[string]string) {
	if node.Kind() == "local_variable_declaration" {
		typeNode := node.ChildByFieldName("type")
		if typeNode != nil && sourceText(source, typeNode) != "var" {
			typeName := sourceText(source, typeNode)
			for i := 0; i < int(node.NamedChildCount()); i++ {
				child := node.NamedChild(uint(i))
				if child.Kind() != "variable_declarator" {
					continue
				}
				if nameNode := child.ChildByFieldName("name"); nameNode != nil {
					localVars[sourceText(source, nameNode)] = typeName
				}
			}
		}
	}

	for i := 0; i < int(node.NamedChildCount()); i++ {
		walkAndCollectLocalVars(node.NamedChild(uint(i)), source, localVars)
	}
}
