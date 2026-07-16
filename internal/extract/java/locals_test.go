package java

import (
	"testing"
)

func TestExtractLocalVars(t *testing.T) {
	types := extractJavaSource(t, `
package com.example;

class Example {
    void run() {
        Helper first = new Helper(), second;
        String label = "test";
        var inferred = new Helper();
        if (true) {
            Logger logger = new Logger();
        }
    }
}
`)
	if len(types) != 1 || len(types[0].Methods) != 1 {
		t.Fatalf("expected one type with one method, got %+v", types)
	}

	got := types[0].Methods[0].LocalVars
	want := map[string]string{
		"first":  "Helper",
		"second": "Helper",
		"label":  "String",
		"logger": "Logger",
	}
	if len(got) != len(want) {
		t.Fatalf("local vars count: got %d (%+v), want %d", len(got), got, len(want))
	}
	for _, decl := range got {
		if typeName, ok := want[decl.Name]; !ok {
			t.Errorf("unexpected local var %q", decl.Name)
		} else if decl.Type.Raw != typeName {
			t.Errorf("local var %q: got %q, want %q", decl.Name, decl.Type.Raw, typeName)
		}
		if decl.DeclStart == 0 {
			t.Errorf("local var %q: DeclStart not populated", decl.Name)
		}
	}
}

func TestExtractLocalVarsWithoutBody(t *testing.T) {
	types := extractJavaSource(t, `interface Example { void run(); }`)
	if got := types[0].Methods[0].LocalVars; got != nil {
		t.Fatalf("expected nil local vars, got %v", got)
	}
}

func TestExtractLocalVarsCarriesScopeRanges(t *testing.T) {
	types := extractJavaSource(t, `
package com.example;

class Example {
    void run() {
        Helper outer = new Helper();
        if (true) {
            Helper inner = new Helper();
        }
    }
}
`)
	got := types[0].Methods[0].LocalVars
	var outer, inner *LocalVarDecl
	for i := range got {
		switch got[i].Name {
		case "outer":
			outer = &got[i]
		case "inner":
			inner = &got[i]
		}
	}
	if outer == nil || inner == nil {
		t.Fatalf("missing outer/inner: %+v", got)
	}
	if outer.ScopeStart >= outer.ScopeEnd {
		t.Errorf("outer scope not a valid range: [%d, %d)", outer.ScopeStart, outer.ScopeEnd)
	}
	if inner.ScopeStart >= inner.ScopeEnd {
		t.Errorf("inner scope not a valid range: [%d, %d)", inner.ScopeStart, inner.ScopeEnd)
	}
	// inner block deve estar contido dentro do escopo do outer (body)
	if inner.ScopeStart < outer.ScopeStart || inner.ScopeEnd > outer.ScopeEnd {
		t.Errorf("inner scope [%d, %d) deve estar dentro de outer [%d, %d)",
			inner.ScopeStart, inner.ScopeEnd, outer.ScopeStart, outer.ScopeEnd)
	}
	// inner scope deve ser estritamente menor (mais interno) que outer
	if inner.ScopeStart <= outer.ScopeStart {
		t.Errorf("inner scope start %d deveria ser > outer scope start %d", inner.ScopeStart, outer.ScopeStart)
	}
}

func TestExtractLocalVarsDeclarationOrder(t *testing.T) {
	types := extractJavaSource(t, `
package com.example;

class Example {
    void run() {
        Helper first = new Helper();
        call();
        Helper second = new Helper();
    }
    void call() {}
}
`)
	method := types[0].Methods[0]
	if len(method.LocalVars) != 2 {
		t.Fatalf("want 2 local vars, got %d", len(method.LocalVars))
	}
	first := method.LocalVars[0]
	second := method.LocalVars[1]
	if first.Name != "first" || second.Name != "second" {
		t.Fatalf("order wrong: %+v", method.LocalVars)
	}
	if first.DeclStart >= second.DeclStart {
		t.Errorf("expected first.DeclStart < second.DeclStart; got %d >= %d", first.DeclStart, second.DeclStart)
	}
}
