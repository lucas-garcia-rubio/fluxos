package java

import (
	"strings"
)

func buildSignature(params []Param) string {
	types := make([]string, len(params))
	for i, param := range params {
		types[i] = param.Type.SignatureToken()
	}
	return "(" + strings.Join(types, ",") + ")"
}

// RebuildSignature refreshes a method key after its parameter refs are canonicalized.
func RebuildSignature(method *MethodDecl) {
	method.Signature = buildSignature(method.Params)
}

func eraseGenericArguments(raw string) string {
	var out strings.Builder
	depth := 0
	for _, r := range raw {
		switch r {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				out.WriteRune(r)
			}
		}
	}
	return out.String()
}
