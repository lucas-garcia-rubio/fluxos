package java

import (
	"reflect"
	"testing"
)

func TestExtractClassInterfaces(t *testing.T) {
	types := extractJavaSource(t, `
package com.example;
class ServiceImpl implements Service, Auditable {}
`)

	typ := findTypeBySimpleName(t, types, "ServiceImpl")
	if typ.FQCN != "com.example.ServiceImpl" {
		t.Fatalf("FQCN = %q, want %q", typ.FQCN, "com.example.ServiceImpl")
	}
	if want := []TypeRef{NewTypeRef("Service", false), NewTypeRef("Auditable", false)}; !reflect.DeepEqual(typ.Interfaces, want) {
		t.Fatalf("interfaces = %v, want %v", typ.Interfaces, want)
	}
}

func TestExtractExtendedInterfaces(t *testing.T) {
	types := extractJavaSource(t, `
interface Child extends Parent {}
interface Bottom extends Left, Right {}
`)

	tests := []struct {
		name string
		want []TypeRef
	}{
		{name: "Child", want: []TypeRef{NewTypeRef("Parent", false)}},
		{name: "Bottom", want: []TypeRef{NewTypeRef("Left", false), NewTypeRef("Right", false)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			typ := findTypeBySimpleName(t, types, tt.name)
			if !reflect.DeepEqual(typ.Interfaces, tt.want) {
				t.Fatalf("interfaces = %v, want %v", typ.Interfaces, tt.want)
			}
		})
	}
}

func TestExtractSuperclassRawForms(t *testing.T) {
	types := extractJavaSource(t, `
class Child extends Parent {}
class Qualified extends com.example.base.Parent {}
class Generic extends Parent<String> {}
`)

	tests := map[string]string{
		"Child":     "Parent",
		"Qualified": "com.example.base.Parent",
		"Generic":   "Parent<String>",
	}
	for name, want := range tests {
		t.Run(name, func(t *testing.T) {
			if got := findTypeBySimpleName(t, types, name).SuperClass.Raw; got != want {
				t.Fatalf("superclass = %q, want %q", got, want)
			}
		})
	}
}

func TestExtractTypeModifiers(t *testing.T) {
	types := extractJavaSource(t, `
public abstract class Base {}
final class Value {}
interface Plain {}
`)

	tests := []struct {
		name string
		want []string
	}{
		{name: "Base", want: []string{"public", "abstract"}},
		{name: "Value", want: []string{"final"}},
		{name: "Plain", want: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findTypeBySimpleName(t, types, tt.name).Modifier
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("modifiers = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractMethodModifiers(t *testing.T) {
	types := extractJavaSource(t, `
class Util {
    public static void run() {}
    private static void hidden() {}
    void instance() {}
}
interface Contract {
    default void work() {}
    static void create() {}
}
`)

	tests := []struct {
		typeName   string
		methodName string
		want       []string
	}{
		{typeName: "Util", methodName: "run", want: []string{"public", "static"}},
		{typeName: "Util", methodName: "hidden", want: []string{"private", "static"}},
		{typeName: "Util", methodName: "instance", want: []string{}},
		{typeName: "Contract", methodName: "work", want: []string{"default"}},
		{typeName: "Contract", methodName: "create", want: []string{"static"}},
	}
	for _, tt := range tests {
		t.Run(tt.typeName+"/"+tt.methodName, func(t *testing.T) {
			typ := findTypeBySimpleName(t, types, tt.typeName)
			var got []string
			for _, method := range typ.Methods {
				if method.Name == tt.methodName {
					got = method.Modifier
					break
				}
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("modifiers = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasModifierUsesExactMembership(t *testing.T) {
	if !HasModifier([]string{"public", "static"}, "static") {
		t.Fatal("HasModifier did not find exact modifier")
	}
	for _, modifiers := range [][]string{{"@Static"}, {"nonstatic"}, {"Static"}, nil} {
		if HasModifier(modifiers, "static") {
			t.Fatalf("HasModifier matched %v as static", modifiers)
		}
	}
}

func TestExtractMultipleFieldDeclarators(t *testing.T) {
	types := extractJavaSource(t, `
class Example {
    private int first = 1, second, third = 3;
    String single;
    java.util.List<String>[] genericArray;
}
`)

	fields := findTypeBySimpleName(t, types, "Example").Fields
	if len(fields) != 5 {
		t.Fatalf("field count = %d, want 5: %+v", len(fields), fields)
	}
	want := []FieldDecl{
		{Name: "first", Modifier: []string{"private"}, Type: NewTypeRef("int", false), Initializer: "1"},
		{Name: "second", Modifier: []string{"private"}, Type: NewTypeRef("int", false)},
		{Name: "third", Modifier: []string{"private"}, Type: NewTypeRef("int", false), Initializer: "3"},
		{Name: "single", Modifier: []string{}, Type: NewTypeRef("String", false)},
		{Name: "genericArray", Modifier: []string{}, Type: NewTypeRef("java.util.List<String>[]", false)},
	}
	if !reflect.DeepEqual(fields, want) {
		t.Fatalf("fields = %+v, want %+v", fields, want)
	}
}

func TestExtractMethodKindsAndSignatures(t *testing.T) {
	types := extractJavaSource(t, `
class Example {
    Example(String name) {}
    void run() {}
    void run(int count, java.util.List<String> values) {}
    void log(String... values) {}
}
`)

	methods := findTypeBySimpleName(t, types, "Example").Methods
	if len(methods) != 4 {
		t.Fatalf("method count = %d, want 4", len(methods))
	}
	tests := []struct {
		index     int
		kind      MethodKind
		name      string
		signature string
	}{
		{index: 0, kind: MethodConstructor, name: "<init>", signature: "(String)"},
		{index: 1, kind: MethodOrdinary, name: "run", signature: "()"},
		{index: 2, kind: MethodOrdinary, name: "run", signature: "(int,java.util.List)"},
		{index: 3, kind: MethodOrdinary, name: "log", signature: "(String[])"},
	}
	for _, tt := range tests {
		method := methods[tt.index]
		if method.Kind != tt.kind || method.Name != tt.name || method.Signature != tt.signature {
			t.Errorf("method %d = {kind:%s name:%q signature:%q}, want {kind:%s name:%q signature:%q}", tt.index, method.Kind, method.Name, method.Signature, tt.kind, tt.name, tt.signature)
		}
	}
	if params := methods[3].Params; len(params) != 1 || !params[0].Variadic {
		t.Fatalf("variadic params = %+v, want one variadic param", params)
	}
}
