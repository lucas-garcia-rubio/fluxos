package java

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lucas-garcia-rubio/fluxos/internal/parse"
)

func TestExtractLocalVars(t *testing.T) {
	source := []byte(`
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
	path := filepath.Join(t.TempDir(), "Example.java")
	if err := os.WriteFile(path, source, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	tree, parsedSource, err := parse.Parse(path)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	defer tree.Close()

	types, err := Extract(path, parsedSource, tree)
	if err != nil {
		t.Fatalf("extract fixture: %v", err)
	}
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
	source := []byte(`interface Example { void run(); }`)
	path := filepath.Join(t.TempDir(), "Example.java")
	if err := os.WriteFile(path, source, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	tree, parsedSource, err := parse.Parse(path)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	defer tree.Close()

	types, err := Extract(path, parsedSource, tree)
	if err != nil {
		t.Fatalf("extract fixture: %v", err)
	}
	if got := types[0].Methods[0].LocalVars; got != nil {
		t.Fatalf("expected nil local vars, got %v", got)
	}
}
