package java

import (
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// extractCalls percorre o body de um método (ou constructor) achando todos os
// method_invocation recursivamente. Devolve slice de CallSite.
// filePath é o caminho do arquivo .java — populado em CallSite.File pra mensagens
// de erro/warning do resolver poderem referenciar arquivo:linha.
func extractCalls(source []byte, methodNode *sitter.Node, filePath string) []CallSite {
	var calls []CallSite
	body := methodNode.ChildByFieldName("body")
	if body == nil {
		return calls
	}
	walkAndCollectCalls(body, source, filePath, &calls)
	return calls
}

// walkAndCollectCalls faz DFS na AST. Se o nó atual é method_invocation, adiciona
// CallSite. Depois recursa em todos os named children (pega chamadas aninhadas
// como `foo(bar(baz()))` — vira 3 CallSites).
func walkAndCollectCalls(node *sitter.Node, source []byte, filePath string, calls *[]CallSite) {
	if isNestedExecutableBoundary(node) {
		return
	}
	if node.Kind() == "method_invocation" {
		*calls = append(*calls, buildCallSite(source, node, filePath))
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		walkAndCollectCalls(node.NamedChild(uint(i)), source, filePath, calls)
	}
}

func isNestedExecutableBoundary(node *sitter.Node) bool {
	switch node.Kind() {
	case "lambda_expression", "class_body", "interface_body", "enum_body", "annotation_type_body":
		return true
	default:
		return false
	}
}

// buildCallSite extrai MethodName (field "name"), Receiver (field "object" ou ""),
// Args (filhos nomeados de "arguments"), File e Line de um method_invocation.
//
// Receiver pode ser sourceText de:
//   - nil → "" (chamada não-qualificada: `foo()`)
//   - `this` keyword → "this"
//   - `super` keyword → "super"
//   - `identifier` → nome simples (ex.: "userService")
//   - `field_access` → encadeado (ex.: "System.out", "this.userService")
//   - `method_invocation` → chamada encadeada (ex.: "getFoo()")
//   - `object_creation_expression` → "new Foo()"
//
// O resolver (Passo 6+) decide o que fazer com cada caso.
func buildCallSite(source []byte, node *sitter.Node, filePath string) CallSite {
	cs := CallSite{
		Kind:      CallInvocation,
		File:      filePath,
		Line:      int(node.StartPosition().Row) + 1, // 0-indexed → 1-indexed
		StartByte: uint(node.StartByte()),
		EndByte:   uint(node.EndByte()),
		Args:      []string{},
	}
	if nameNode := node.ChildByFieldName("name"); nameNode != nil {
		cs.MethodName = sourceText(source, nameNode)
	}
	if recvNode := node.ChildByFieldName("object"); recvNode != nil {
		cs.Receiver = sourceText(source, recvNode)
	}
	if argsNode := node.ChildByFieldName("arguments"); argsNode != nil {
		for i := 0; i < int(argsNode.NamedChildCount()); i++ {
			argNode := argsNode.NamedChild(uint(i))
			cs.Args = append(cs.Args, sourceText(source, argNode))
		}
	}
	cs.ArgCount = len(cs.Args)
	return cs
}
