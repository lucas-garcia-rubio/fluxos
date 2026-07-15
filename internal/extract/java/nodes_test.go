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
	if want := []string{"Service", "Auditable"}; !reflect.DeepEqual(typ.Interfaces, want) {
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
		want []string
	}{
		{name: "Child", want: []string{"Parent"}},
		{name: "Bottom", want: []string{"Left", "Right"}},
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
			if got := findTypeBySimpleName(t, types, name).SuperClass; got != want {
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
		{Name: "first", Modifier: []string{"private"}, Type: "int", Initializer: "1"},
		{Name: "second", Modifier: []string{"private"}, Type: "int"},
		{Name: "third", Modifier: []string{"private"}, Type: "int", Initializer: "3"},
		{Name: "single", Modifier: []string{}, Type: "String"},
		{Name: "genericArray", Modifier: []string{}, Type: "java.util.List<String>[]"},
	}
	if !reflect.DeepEqual(fields, want) {
		t.Fatalf("fields = %+v, want %+v", fields, want)
	}
}
