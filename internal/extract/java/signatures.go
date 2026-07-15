package java

import (
	"strings"
	"unicode"
)

func buildSignature(params []Param) string {
	types := make([]string, len(params))
	for i, param := range params {
		types[i] = normalizeSignatureType(param.Type, param.Variadic)
	}
	return "(" + strings.Join(types, ",") + ")"
}

func normalizeSignatureType(raw string, variadic bool) string {
	typeName := eraseGenericArguments(raw)
	typeName = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, typeName)
	if variadic {
		typeName += "[]"
	}
	return typeName
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
