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
		t.Fatalf("local vars count: got %d (%v), want %d", len(got), got, len(want))
	}
	for name, typeName := range want {
		if got[name] != typeName {
			t.Errorf("local var %q: got %q, want %q", name, got[name], typeName)
		}
	}
	if _, ok := got["inferred"]; ok {
		t.Error("var declaration should be ignored until type inference is supported")
	}
}

func TestExtractLocalVarsWithoutBody(t *testing.T) {
	types := extractJavaSource(t, `interface Example { void run(); }`)
	if got := types[0].Methods[0].LocalVars; got != nil {
		t.Fatalf("expected nil local vars, got %v", got)
	}
}
