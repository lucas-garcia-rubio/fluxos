package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var errWrite = errors.New("write failed")

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errWrite
}

func traceFixtureRoot() string {
	return filepath.Join("..", "..", "testdata", "trace")
}

func m3FixtureRoot(name string) string {
	return filepath.Join("..", "..", "testdata", "m3", name)
}

func TestRunTraceMermaidGolden(t *testing.T) {
	root := traceFixtureRoot()
	var out bytes.Buffer

	if err := runTrace([]string{"Workflow.start", root}, &out); err != nil {
		t.Fatalf("runTrace: %v", err)
	}

	want, err := os.ReadFile(filepath.Join(root, "expected.mmd"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if out.String() != string(want) {
		t.Fatalf("trace output mismatch:\ngot:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestRunTraceAcceptsFQCNTarget(t *testing.T) {
	root := traceFixtureRoot()
	var out bytes.Buffer

	if err := runTrace([]string{"com.foo.Workflow.start", root}, &out); err != nil {
		t.Fatalf("runTrace FQCN: %v", err)
	}
	want, err := os.ReadFile(filepath.Join(root, "expected.mmd"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if out.String() != string(want) {
		t.Fatalf("FQCN trace output mismatch:\ngot:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestRunTraceAcceptsSignatureTarget(t *testing.T) {
	root := traceFixtureRoot()
	var out bytes.Buffer

	if err := runTrace([]string{"com.foo.Workflow.start()", root}, &out); err != nil {
		t.Fatalf("runTrace signature: %v", err)
	}
	want, err := os.ReadFile(filepath.Join(root, "expected.mmd"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if out.String() != string(want) {
		t.Fatalf("signature trace output mismatch:\ngot:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestRunTraceInheritanceFixture(t *testing.T) {
	var out bytes.Buffer
	if err := runTrace([]string{"app.Workflow.start", m3FixtureRoot("inheritance")}, &out); err != nil {
		t.Fatalf("runTrace inheritance: %v", err)
	}
	for _, label := range []string{
		"base.BaseService.inheritedMethod()",
		"base.GrandparentService.grandparentMethod()",
		"contract.ChildContract.childDefault()",
		"contract.RootContract.rootDefault()",
	} {
		if !strings.Contains(out.String(), label) {
			t.Errorf("inheritance trace missing %q:\n%s", label, out.String())
		}
	}
}

func TestRunTraceImportsFixture(t *testing.T) {
	var out bytes.Buffer
	if err := runTrace([]string{"app.Workflow.start", m3FixtureRoot("imports")}, &out); err != nil {
		t.Fatalf("runTrace imports: %v", err)
	}
	for _, label := range []string{
		"explicit.ExplicitTasks.explicitRun()",
		"explicit.ExplicitTasks.explicitRun(types.Helper)",
		"wildcard.WildcardTasks.wildcardRun()",
		"inherited.BaseTasks.inheritedRun()",
		"contract.ContractTasks.interfaceRun()",
		"app.Workflow.currentRun()",
		"types.Helper.work()",
	} {
		if !strings.Contains(out.String(), label) {
			t.Errorf("imports trace missing %q:\n%s", label, out.String())
		}
	}
}

func TestRunTraceConstructorsFixture(t *testing.T) {
	var out bytes.Buffer
	if err := runTrace([]string{"app.Workflow.run", m3FixtureRoot("constructors-methodrefs")}, &out); err != nil {
		t.Fatalf("runTrace constructors: %v", err)
	}
	for _, label := range []string{
		"model.DefaultValue.<init>()",
		"model.OverloadedValue.<init>(java.lang.String)",
		"model.OverloadedValue.<init>(java.lang.String,int)",
		"model.DelegatingValue.<init>(java.lang.String)",
		"model.DelegatingValue.<init>(java.lang.String,boolean)",
		"model.ChildValue.<init>(java.lang.String)",
		"model.BaseValue.<init>(java.lang.String)",
		"model.Point.<init>(int,int)",
	} {
		if !strings.Contains(out.String(), label) {
			t.Errorf("constructors trace missing %q:\n%s", label, out.String())
		}
	}
}

func TestRunTraceArgumentErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing spec", want: "usage"},
		{name: "invalid spec", args: []string{"Workflow"}, want: "expected TypeName.method or FQCN.method"},
		{name: "missing class", args: []string{"Missing.start", traceFixtureRoot()}, want: `class "Missing" not found`},
		{name: "unknown FQCN", args: []string{"com.foo.Missing.start", traceFixtureRoot()}, want: `"com.foo.Missing" not found`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runTrace(tt.args, io.Discard)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("runTrace(%v) error = %v, want containing %q", tt.args, err, tt.want)
			}
		})
	}
}

func TestRunTraceWriterError(t *testing.T) {
	err := runTrace([]string{"Workflow.start", traceFixtureRoot()}, failingWriter{})
	if !errors.Is(err, errWrite) {
		t.Fatalf("runTrace writer error = %v, want wrapped %v", err, errWrite)
	}
	if !strings.Contains(err.Error(), "write trace") {
		t.Fatalf("runTrace writer error = %q, want context", err)
	}
}

func TestBuildUnitsPreservesMetadataAndFlattenCompatibility(t *testing.T) {
	units, err := buildUnits(traceFixtureRoot())
	if err != nil {
		t.Fatalf("buildUnits: %v", err)
	}
	if len(units) != 1 {
		t.Fatalf("unit count = %d, want 1", len(units))
	}
	unit := units[0]
	wantSourceRoot := filepath.Join(traceFixtureRoot(), "src", "main", "java")
	if unit.SourceRoot != wantSourceRoot || unit.Package != "com.foo" || unit.Imports == nil || len(unit.Imports) != 0 {
		t.Fatalf("unit metadata = %+v", unit)
	}

	indexedUnits, table, err := buildIndex(traceFixtureRoot())
	if err != nil {
		t.Fatalf("buildIndex: %v", err)
	}
	if len(indexedUnits) != len(units) || indexedUnits[0].File != units[0].File || len(indexedUnits[0].Types) != len(units[0].Types) {
		t.Fatalf("buildIndex did not preserve unit structure: got %+v, want %+v", indexedUnits, units)
	}
	for _, typ := range flattenTypes(indexedUnits) {
		if got, ok := table.TypeByFQCN(typ.FQCN); !ok || !reflect.DeepEqual(got, typ) {
			t.Fatalf("indexed type %q = %+v, %v", typ.FQCN, got, ok)
		}
	}
}
