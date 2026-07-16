package java

import "testing"

func TestNewTypeRefPreservesRawAndNormalizesShape(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		variadic   bool
		base       string
		arrays     int
		primitive  bool
		signature  string
		unresolved bool
	}{
		{name: "generic array", raw: "Map<String, List<User>>[][]", base: "Map", arrays: 2, signature: "Map[][]", unresolved: true},
		{name: "qualified", raw: "com.foo.Helper", base: "com.foo.Helper", signature: "com.foo.Helper", unresolved: true},
		{name: "primitive", raw: "int[]", base: "int", arrays: 1, primitive: true, signature: "int[]"},
		{name: "variadic", raw: "String", variadic: true, base: "String", arrays: 1, signature: "String[]", unresolved: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref := NewTypeRef(tt.raw, tt.variadic)
			if ref.Raw != tt.raw || ref.BaseName() != tt.base || ref.ArrayDepth != tt.arrays || ref.Primitive != tt.primitive || ref.Unresolved != tt.unresolved || ref.SignatureToken() != tt.signature {
				t.Fatalf("NewTypeRef(%q) = %+v (base %q, signature %q)", tt.raw, ref, ref.BaseName(), ref.SignatureToken())
			}
		})
	}
}
