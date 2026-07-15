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

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
	"github.com/lucas-garcia-rubio/fluxos/internal/index"
)

var errWrite = errors.New("write failed")

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errWrite
}

func traceFixtureRoot() string {
	return filepath.Join("..", "..", "testdata", "trace")
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

func TestRunTraceArgumentErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing spec", want: "usage"},
		{name: "invalid spec", args: []string{"Workflow"}, want: "expected ClassName.methodName"},
		{name: "missing class", args: []string{"Missing.start", traceFixtureRoot()}, want: "class \"Missing\" not found"},
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

func TestFindMethodByNameListsOverloadSignatures(t *testing.T) {
	typ := &java.TypeDecl{Name: "Service", Methods: []java.MethodDecl{
		{Name: "run", Signature: "(String)"},
		{Name: "run", Signature: "()"},
	}}
	typ.FQCN = typ.Name
	table, err := index.Build([]*java.CompilationUnit{{File: "Service.java", Types: []*java.TypeDecl{typ}}})
	if err != nil {
		t.Fatalf("build index: %v", err)
	}

	_, err = findMethodByName(table, typ, "run")
	if err == nil || !strings.Contains(err.Error(), "available signatures: (), (String)") {
		t.Fatalf("findMethodByName error = %v", err)
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
	if !reflect.DeepEqual(indexedUnits, units) {
		t.Fatalf("buildIndex units differ:\ngot:  %+v\nwant: %+v", indexedUnits, units)
	}
	for _, typ := range flattenTypes(units) {
		if got, ok := table.TypeByFQCN(typ.FQCN); !ok || !reflect.DeepEqual(got, typ) {
			t.Fatalf("indexed type %q = %+v, %v", typ.FQCN, got, ok)
		}
	}
}

func TestFindClassByNameListsSortedCandidates(t *testing.T) {
	first := &java.TypeDecl{Name: "Service", FQCN: "z.Service"}
	second := &java.TypeDecl{Name: "Service", FQCN: "a.Service"}
	table, err := index.Build([]*java.CompilationUnit{{File: "Services.java", Types: []*java.TypeDecl{first, second}}})
	if err != nil {
		t.Fatalf("build index: %v", err)
	}

	_, err = findClassByName(table, "Service")
	if err == nil || !strings.Contains(err.Error(), "candidates: a.Service, z.Service") {
		t.Fatalf("findClassByName error = %v", err)
	}
}
