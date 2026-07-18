package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func assertTraceGolden(t *testing.T, fixture, target, golden string) {
	t.Helper()
	root := m3FixtureRoot(fixture)
	var out bytes.Buffer
	if err := runTrace([]string{target, root}, &out); err != nil {
		t.Fatalf("runTrace(%q, %q): %v", target, fixture, err)
	}
	want, err := os.ReadFile(filepath.Join(root, golden))
	if err != nil {
		t.Fatalf("read golden %s/%s: %v", fixture, golden, err)
	}
	if !bytes.Equal(out.Bytes(), want) {
		t.Fatalf("trace output mismatch for %s target %s (%s):\n%s", fixture, target, golden, firstDiffContext(string(want), out.String()))
	}
}

func firstDiffContext(want, got string) string {
	wantLines := strings.Split(want, "\n")
	gotLines := strings.Split(got, "\n")
	limit := len(wantLines)
	if len(gotLines) < limit {
		limit = len(gotLines)
	}
	for i := 0; i < limit; i++ {
		if wantLines[i] != gotLines[i] {
			return fmt.Sprintf("line %d\nwant: %s\n got: %s", i+1, wantLines[i], gotLines[i])
		}
	}
	return fmt.Sprintf("line count differs: want %d, got %d", len(wantLines), len(gotLines))
}

func TestRunTraceM3Goldens(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		target  string
		golden  string
	}{
		{name: "cross-file basic", fixture: "cross-file-basic", target: "app.Workflow.start", golden: "expected.mmd"},
		{name: "imports", fixture: "imports", target: "app.Workflow.start", golden: "expected.mmd"},
		{name: "inheritance", fixture: "inheritance", target: "app.Workflow.start", golden: "expected.mmd"},
		{name: "constructors", fixture: "constructors-methodrefs", target: "app.Workflow.run", golden: "expected-run.mmd"},
		{name: "method references include unresolved by default", fixture: "constructors-methodrefs", target: "app.Workflow.references", golden: "expected-references.mmd"},
		{name: "nested target", fixture: "constructors-methodrefs", target: "refs.References.Nested.run", golden: "expected-nested.mmd"},
		{name: "dispatch keeps ambiguity terminal without prompt", fixture: "dispatch", target: "app.Workflow.start", golden: "expected.mmd"},
		{name: "polymorphism", fixture: "polymorphism", target: "app.Workflow.start", golden: "expected.mmd"},
		{name: "overload calls", fixture: "overloads", target: "app.Workflow.calls", golden: "expected-calls.mmd"},
		{name: "overloaded string root", fixture: "overloads", target: "overloads.Workflow.run(java.lang.String)", golden: "expected-string.mmd"},
		{name: "overloaded int root", fixture: "overloads", target: "overloads.Workflow.run(int)", golden: "expected-int.mmd"},
		{name: "source roots", fixture: "source-roots", target: "app.Workflow.start", golden: "expected.mmd"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertTraceGolden(t, tt.fixture, tt.target, tt.golden)
		})
	}
}

func TestRunTraceTerminalIDsDoNotDependOnRootSpelling(t *testing.T) {
	relative := m3FixtureRoot("dispatch")
	absolute, err := filepath.Abs(relative)
	if err != nil {
		t.Fatalf("absolute fixture path: %v", err)
	}
	var relativeOut, absoluteOut bytes.Buffer
	if err := runTrace([]string{"app.Workflow.start", relative}, &relativeOut); err != nil {
		t.Fatalf("relative trace: %v", err)
	}
	if err := runTrace([]string{"app.Workflow.start", absolute}, &absoluteOut); err != nil {
		t.Fatalf("absolute trace: %v", err)
	}
	if relativeOut.String() != absoluteOut.String() {
		t.Fatalf("root spelling changed terminal IDs:\n%s", firstDiffContext(relativeOut.String(), absoluteOut.String()))
	}
}

func TestRunTraceOverloadedRootWithoutSignatureListsCandidates(t *testing.T) {
	err := runTrace([]string{"overloads.Workflow.run", m3FixtureRoot("overloads")}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("overloaded root without signature succeeded")
	}
	want := "(int), (java.lang.String)"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("overload error = %q, want signatures %q", err, want)
	}
}
