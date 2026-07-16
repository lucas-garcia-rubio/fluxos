package java

import (
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// extractCalls percorre o body de um método (ou constructor) achando calls
// recursivamente. Devolve slice de CallSite.
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

// walkAndCollectCalls faz DFS pre-order na AST. Emite o CallSite atual e depois
// recursa nos named children, preservando chamadas aninhadas e seus argumentos.
func walkAndCollectCalls(node *sitter.Node, source []byte, filePath string, calls *[]CallSite) {
	if isNestedExecutableBoundary(node) {
		return
	}
	switch node.Kind() {
	case "method_invocation":
		*calls = append(*calls, buildCallSite(source, node, filePath))
	case "object_creation_expression":
		*calls = append(*calls, buildObjectCreationCall(source, node, filePath))
	case "explicit_constructor_invocation":
		*calls = append(*calls, buildExplicitConstructorCall(source, node, filePath))
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
	cs := newCallSite(source, node, filePath, CallInvocation)
	if nameNode := node.ChildByFieldName("name"); nameNode != nil {
		cs.MethodName = sourceText(source, nameNode)
	}
	if recvNode := node.ChildByFieldName("object"); recvNode != nil {
		cs.Receiver = sourceText(source, recvNode)
	}
	return cs
}

func buildObjectCreationCall(source []byte, node *sitter.Node, filePath string) CallSite {
	cs := newCallSite(source, node, filePath, CallObjectCreation)
	cs.MethodName = "<init>"
	if typeNode := node.ChildByFieldName("type"); typeNode != nil {
		targetType := NewTypeRef(sourceText(source, typeNode), false)
		cs.TargetType = &targetType
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(uint(i))
			if child.StartByte() >= typeNode.StartByte() {
				break
			}
			if child.Kind() != "type_arguments" {
				cs.Receiver = sourceText(source, child)
				break
			}
		}
	}
	cs.Anonymous = findFirstByKind(node, "class_body") != nil
	return cs
}

func buildExplicitConstructorCall(source []byte, node *sitter.Node, filePath string) CallSite {
	cs := newCallSite(source, node, filePath, CallSuperConstructor)
	cs.MethodName = "<init>"
	if constructor := node.ChildByFieldName("constructor"); constructor != nil && constructor.Kind() == "this" {
		cs.Kind = CallThisConstructor
	}
	if qualifier := node.ChildByFieldName("object"); qualifier != nil {
		cs.Receiver = sourceText(source, qualifier)
	}
	return cs
}

func newCallSite(source []byte, node *sitter.Node, filePath string, kind CallKind) CallSite {
	cs := CallSite{
		Kind:      kind,
		File:      filePath,
		Line:      int(node.StartPosition().Row) + 1, // 0-indexed → 1-indexed
		StartByte: uint(node.StartByte()),
		EndByte:   uint(node.EndByte()),
		Args:      []string{},
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
