package java

import sitter "github.com/tree-sitter/go-tree-sitter"

func buildMethodReferenceCall(source []byte, node *sitter.Node, filePath string) CallSite {
	call := newCallSite(source, node, filePath, CallMethodReference)
	separator := -1
	for i := 0; i < int(node.ChildCount()); i++ {
		if child := node.Child(uint(i)); child != nil && child.Kind() == "::" {
			separator = i
			break
		}
	}
	if separator <= 0 {
		return call
	}

	qualifier := node.Child(uint(separator - 1))
	if qualifier == nil {
		return call
	}
	qualifierText := sourceText(source, qualifier)
	call.ReferenceQualifier = classifyReferenceQualifier(qualifier)

	var terminal *sitter.Node
	for i := separator + 1; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child != nil && child.Kind() != "type_arguments" {
			terminal = child
		}
	}
	if terminal == nil {
		return call
	}
	if terminal.Kind() == "new" {
		call.Kind = CallConstructorReference
		call.MethodName = "<init>"
		target := NewTypeRef(qualifierText, false)
		call.TargetType = &target
		return call
	}
	call.Receiver = qualifierText
	call.MethodName = sourceText(source, terminal)
	return call
}

func classifyReferenceQualifier(node *sitter.Node) ReferenceQualifierKind {
	if node == nil {
		return ReferenceQualifierUnknown
	}
	switch node.Kind() {
	case "super":
		return ReferenceQualifierSuper
	case "identifier", "scoped_identifier", "field_access":
		return ReferenceQualifierName
	case "type_identifier", "scoped_type_identifier", "generic_type", "array_type", "integral_type", "floating_point_type", "boolean_type", "void_type":
		return ReferenceQualifierType
	case "this", "method_invocation", "object_creation_expression", "parenthesized_expression":
		return ReferenceQualifierExpression
	default:
		return ReferenceQualifierUnknown
	}
}
