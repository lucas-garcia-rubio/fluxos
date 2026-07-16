package java

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestExtractUnitPreservesPackageImportsAndTypes(t *testing.T) {
	tree, source := parseJavaSource(t, `
package com.example;
import com.foo.Service;
import com.foo.*;
import static com.foo.Util.run;
import static com.foo.Util.*;
class First {}
interface Second {}
`)

	unit, err := ExtractUnit("src/Example.java", source, tree)
	if err != nil {
		t.Fatalf("ExtractUnit: %v", err)
	}
	if unit.File != "src/Example.java" || unit.SourceRoot != "" || unit.Package != "com.example" {
		t.Fatalf("unit metadata = {file:%q sourceRoot:%q package:%q}", unit.File, unit.SourceRoot, unit.Package)
	}
	wantImports := []ImportDecl{
		{Target: "com.foo.Service"},
		{Target: "com.foo", Wildcard: true},
		{Target: "com.foo.Util.run", Static: true},
		{Target: "com.foo.Util", Static: true, Wildcard: true},
	}
	if !reflect.DeepEqual(unit.Imports, wantImports) {
		t.Fatalf("imports = %+v, want %+v", unit.Imports, wantImports)
	}
	if len(unit.Types) != 2 || unit.Types[0].FQCN != "com.example.First" || unit.Types[1].FQCN != "com.example.Second" {
		t.Fatalf("types = %+v", unit.Types)
	}
}

func TestExtractUnitUsesNonNilEmptySlices(t *testing.T) {
	tree, source := parseJavaSource(t, ``)
	unit, err := ExtractUnit("Empty.java", source, tree)
	if err != nil {
		t.Fatalf("ExtractUnit: %v", err)
	}
	if unit.Imports == nil || unit.Types == nil {
		t.Fatalf("empty unit slices must be non-nil: %+v", unit)
	}

	out, err := json.Marshal(unit)
	if err != nil {
		t.Fatalf("marshal unit: %v", err)
	}
	if got := string(out); !strings.Contains(got, `"imports":[]`) || !strings.Contains(got, `"types":[]`) {
		t.Fatalf("empty unit JSON = %s", got)
	}
}

func TestExtractRemainsCompatibleWithExtractUnitTypes(t *testing.T) {
	tree, source := parseJavaSource(t, `package com.example; class Example {}`)
	unit, err := ExtractUnit("Example.java", source, tree)
	if err != nil {
		t.Fatalf("ExtractUnit: %v", err)
	}
	types, err := Extract("Example.java", source, tree)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if !reflect.DeepEqual(types, unit.Types) {
		t.Fatalf("Extract types = %+v, ExtractUnit types = %+v", types, unit.Types)
	}
}

func TestExtractNamedNestedTypesInSourcePreorder(t *testing.T) {
	tree, source := parseJavaSource(t, `
package app;
class Outer {
    interface Contract { class Nested {} }
    enum Choice { ONE; record Data(int value) {} }
    void method() { class Local {} }
    Object anonymous = new Object() { class Hidden {} };
}
record Second() {}
`)
	unit, err := ExtractUnit("Types.java", source, tree)
	if err != nil {
		t.Fatalf("ExtractUnit: %v", err)
	}
	want := []struct{ fqcn, enclosing string }{
		{"app.Outer", ""},
		{"app.Outer.Contract", "app.Outer"},
		{"app.Outer.Contract.Nested", "app.Outer.Contract"},
		{"app.Outer.Choice", "app.Outer"},
		{"app.Outer.Choice.Data", "app.Outer.Choice"},
		{"app.Second", ""},
	}
	if len(unit.Types) != len(want) {
		t.Fatalf("types = %+v, want %d declarations", unit.Types, len(want))
	}
	for i, expected := range want {
		if unit.Types[i].FQCN != expected.fqcn || unit.Types[i].EnclosingFQCN != expected.enclosing {
			t.Errorf("type %d = %s enclosing %s, want %s enclosing %s", i, unit.Types[i].FQCN, unit.Types[i].EnclosingFQCN, expected.fqcn, expected.enclosing)
		}
	}
}
