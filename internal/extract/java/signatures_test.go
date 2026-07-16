package java

import "testing"

func TestBuildSignature(t *testing.T) {
	tests := []struct {
		name   string
		params []Param
		want   string
	}{
		{name: "empty", want: "()"},
		{name: "simple", params: []Param{{Type: NewTypeRef("String", false)}, {Type: NewTypeRef("int", false)}}, want: "(String,int)"},
		{name: "array", params: []Param{{Type: NewTypeRef("String[]", false)}}, want: "(String[])"},
		{name: "qualified", params: []Param{{Type: NewTypeRef("java.time.Instant", false)}}, want: "(java.time.Instant)"},
		{name: "variadic", params: []Param{{Type: NewTypeRef("String", true), Variadic: true}}, want: "(String[])"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildSignature(tt.params); got != tt.want {
				t.Fatalf("buildSignature() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildSignatureErasesNestedGenerics(t *testing.T) {
	params := []Param{
		{Type: NewTypeRef("Map<String, List<User>>", false)},
		{Type: NewTypeRef(" java.util.List < User > [] ", false)},
	}
	if got, want := buildSignature(params), "(Map,java.util.List[])"; got != want {
		t.Fatalf("buildSignature() = %q, want %q", got, want)
	}
}

func TestMethodAndCallKindStrings(t *testing.T) {
	if got, want := MethodConstructor.String(), "constructor"; got != want {
		t.Fatalf("MethodConstructor.String() = %q, want %q", got, want)
	}
	if got, want := CallMethodReference.String(), "methodReference"; got != want {
		t.Fatalf("CallMethodReference.String() = %q, want %q", got, want)
	}
}
