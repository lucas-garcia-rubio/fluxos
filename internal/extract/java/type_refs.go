package java

import (
	"strings"
	"unicode"
)

var primitiveTypes = map[string]struct{}{
	"boolean": {},
	"byte":    {},
	"char":    {},
	"double":  {},
	"float":   {},
	"int":     {},
	"long":    {},
	"short":   {},
	"void":    {},
}

// NewTypeRef preserves source text while normalizing generics, arrays and varargs.
func NewTypeRef(raw string, variadic bool) TypeRef {
	ref := TypeRef{Raw: raw}
	typeName := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, eraseGenericArguments(strings.TrimSpace(raw)))

	if variadic && strings.HasSuffix(typeName, "...") {
		typeName = strings.TrimSuffix(typeName, "...")
	}
	for strings.HasSuffix(typeName, "[]") {
		ref.ArrayDepth++
		typeName = strings.TrimSuffix(typeName, "[]")
	}
	if variadic {
		ref.ArrayDepth++
	}
	_, ref.Primitive = primitiveTypes[typeName]
	ref.Unresolved = typeName != "" && !ref.Primitive
	return ref
}

// BaseName returns the erased type name without array suffixes.
func (r TypeRef) BaseName() string {
	typeName := strings.Map(func(ch rune) rune {
		if unicode.IsSpace(ch) {
			return -1
		}
		return ch
	}, eraseGenericArguments(strings.TrimSpace(r.Raw)))
	typeName = strings.TrimSuffix(typeName, "...")
	for strings.HasSuffix(typeName, "[]") {
		typeName = strings.TrimSuffix(typeName, "[]")
	}
	return typeName
}

// SignatureToken prefers the canonical FQCN and preserves array depth.
func (r TypeRef) SignatureToken() string {
	typeName := r.FQCN
	if typeName == "" {
		typeName = r.BaseName()
	}
	return typeName + strings.Repeat("[]", r.ArrayDepth)
}
