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

func TestRunTraceDefaultsProjectRootToWorkingDirectory(t *testing.T) {
	root := traceFixtureRoot()
	want, err := os.ReadFile(filepath.Join(root, "expected.mmd"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	t.Chdir(root)

	var out bytes.Buffer
	if err := runTrace([]string{"Workflow.start"}, &out); err != nil {
		t.Fatalf("runTrace without project root: %v", err)
	}
	if !bytes.Equal(out.Bytes(), want) {
		t.Fatalf("default project root output mismatch:\ngot:\n%s\nwant:\n%s", out.Bytes(), want)
	}
}

func TestRunTraceRejectsProjectPathBeforeTarget(t *testing.T) {
	var out bytes.Buffer
	err := runTrace([]string{traceFixtureRoot(), "Workflow.start"}, &out)
	if err == nil {
		t.Fatal("runTrace accepted project path before target")
	}
	if out.Len() != 0 {
		t.Fatalf("runTrace wrote payload before returning error: %q", out.String())
	}
}

// Extra positionals are ignored by the M3 CLI. Passo 1 intentionally changes
// this characterization to require a usage error before project discovery.
func TestRunTraceCurrentlyIgnoresExtraPositionals(t *testing.T) {
	root := traceFixtureRoot()
	var baseline, withExtras bytes.Buffer
	if err := runTrace([]string{"Workflow.start", root}, &baseline); err != nil {
		t.Fatalf("baseline runTrace: %v", err)
	}
	if err := runTrace([]string{"Workflow.start", root, "ignored", "--also-ignored"}, &withExtras); err != nil {
		t.Fatalf("runTrace with extra positionals: %v", err)
	}
	if !bytes.Equal(withExtras.Bytes(), baseline.Bytes()) {
		t.Fatalf("extra positionals changed output:\ngot:\n%s\nwant:\n%s", withExtras.Bytes(), baseline.Bytes())
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

func TestRunTraceErrorsDoNotWritePayload(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing target", want: "usage"},
		{name: "invalid target", args: []string{"Workflow"}, want: "expected TypeName.method or FQCN.method"},
		{name: "missing target class", args: []string{"Missing.start", traceFixtureRoot()}, want: `class "Missing" not found`},
		{name: "invalid project root", args: []string{"Workflow.start", filepath.Join(t.TempDir(), "missing")}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			err := runTrace(tt.args, &out)
			if err == nil {
				t.Fatalf("runTrace(%v) succeeded", tt.args)
			}
			if tt.want != "" && !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("runTrace(%v) error = %q, want containing %q", tt.args, err, tt.want)
			}
			if out.Len() != 0 {
				t.Fatalf("runTrace(%v) wrote payload before returning error: %q", tt.args, out.String())
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

func TestBuildIndexPolymorphismFixture(t *testing.T) {
	_, table, err := buildIndex(m3FixtureRoot("polymorphism"))
	if err != nil {
		t.Fatalf("buildIndex: %v", err)
	}

	cases := []struct {
		fqcn string
		want []string
	}{
		{"noimpl.NoService", nil},
		{"single.SingleService", []string{"single.SingleServiceImpl"}},
		{"multi.MultiService", []string{"multi.FirstService", "multi.SecondService"}},
		{"transitive.ChildContract", []string{"transitive.ViaChild"}},
		{"transitive.RootContract", []string{"transitive.ViaChild"}},
		{"base.AbstractBase", []string{"base.ConcreteBase", "base.ConcreteChild"}},
		{"base.AbstractMiddle", []string{"base.ConcreteBase", "base.ConcreteChild"}},
		{"kinds.KindContract", []string{"kinds.DataRecord", "kinds.ModeEnum"}},
		{"nested.NestedTypes.Contract", []string{"nested.NestedTypes.Impl", "nested.NestedTypes.InnerImpl"}},
		{"nested.NestedTypes.Base", []string{"nested.NestedTypes.Impl"}},
	}
	for _, tt := range cases {
		t.Run(tt.fqcn, func(t *testing.T) {
			got := fqcnList(table.ImplementationsOf(tt.fqcn))
			if len(tt.want) == 0 {
				if len(got) != 0 {
					t.Fatalf("ImplementationsOf(%q) = %v, want empty", tt.fqcn, got)
				}
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ImplementationsOf(%q) = %v, want %v", tt.fqcn, got, tt.want)
			}
		})
	}
}

func fqcnList(types []*java.TypeDecl) []string {
	out := make([]string, len(types))
	for i, typ := range types {
		out[i] = typ.FQCN
	}
	return out
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
