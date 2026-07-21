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
	assertTraceGoldenAtRoot(t, root, target, golden)
}

func assertTraceGoldenAtRoot(t *testing.T, root, target, golden string) {
	t.Helper()
	var out bytes.Buffer
	if err := runTrace([]string{target, root}, &out); err != nil {
		t.Fatalf("runTrace(%q, %q): %v", target, root, err)
	}
	want, err := os.ReadFile(filepath.Join(root, golden))
	if err != nil {
		t.Fatalf("read golden %s/%s: %v", root, golden, err)
	}
	if !bytes.Equal(out.Bytes(), want) {
		t.Fatalf("trace output mismatch for %s target %s (%s):\n%s", root, target, golden, firstDiffContext(string(want), out.String()))
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

func TestRuntimeContextGoldens(t *testing.T) {
	root := m4FixtureRoot("runtime-context")
	tests := []struct {
		name   string
		target string
		golden string
		format string
	}{
		{name: "two inherited runtime contexts", target: "app.Workflow.start", golden: "expected-start.mmd"},
		{name: "inherited root", target: "app.First.run", golden: "expected-first.mmd"},
		{name: "structured ambiguous dispatch", target: "app.Workflow.ambiguous", golden: "expected-ambiguous.mmd"},
		{name: "DOT runtime contexts", target: "app.Workflow.start", golden: "expected-start.dot", format: "dot"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.format == "" {
				assertTraceGoldenAtRoot(t, root, tt.target, tt.golden)
				return
			}
			var out bytes.Buffer
			if err := runTrace([]string{"--format=" + tt.format, tt.target, root}, &out); err != nil {
				t.Fatalf("runTrace(%q, %q): %v", tt.target, root, err)
			}
			want, err := os.ReadFile(filepath.Join(root, tt.golden))
			if err != nil {
				t.Fatalf("read golden: %v", err)
			}
			if !bytes.Equal(out.Bytes(), want) {
				t.Fatalf("trace output mismatch:\n%s", firstDiffContext(string(want), out.String()))
			}
		})
	}
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

func TestScopeMainTraceGolden(t *testing.T) {
	root := m4FixtureRoot("scope")
	tests := []struct {
		name   string
		target string
		golden string
		format string
	}{
		{name: "mermaid single implementation", target: "app.Workflow.start", golden: "expected-start.mmd"},
		{name: "dot single implementation", target: "app.Workflow.start", golden: "expected-start.dot", format: "dot"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{tt.target, root}
			if tt.format != "" {
				args = []string{"--format=" + tt.format, tt.target, root}
			}
			var out bytes.Buffer
			if err := runTrace(args, &out); err != nil {
				t.Fatalf("runTrace(%v): %v", args, err)
			}
			want, err := os.ReadFile(filepath.Join(root, tt.golden))
			if err != nil {
				t.Fatalf("read golden: %v", err)
			}
			if !bytes.Equal(out.Bytes(), want) {
				t.Fatalf("scope main trace mismatch (%s):\n%s", tt.golden, firstDiffContext(string(want), out.String()))
			}
		})
	}
}

func TestScopeAllTraceGolden(t *testing.T) {
	root := m4FixtureRoot("scope")
	var out bytes.Buffer
	if err := runTrace([]string{"--scope=all", "app.Workflow.start", root}, &out); err != nil {
		t.Fatalf("runTrace scope=all: %v", err)
	}
	want, err := os.ReadFile(filepath.Join(root, "expected-start-all.mmd"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if !bytes.Equal(out.Bytes(), want) {
		t.Fatalf("scope all trace mismatch:\n%s", firstDiffContext(string(want), out.String()))
	}
}

func TestScopeMainRejectsTestOnlyTarget(t *testing.T) {
	root := m4FixtureRoot("scope")
	err := runTrace([]string{"app.WorkflowTest.run", root}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("test-only target under scope=main succeeded")
	}
	if !strings.Contains(err.Error(), `type "app.WorkflowTest" not found`) {
		t.Fatalf("error = %v, want not-found message", err)
	}
}

func TestScopeDuplicateFQCNFailsUnderAll(t *testing.T) {
	root := m4FixtureRoot("scope-duplicate")
	var errOut bytes.Buffer
	code := runCLI([]string{"index", "--scope=all", root}, IO{Out: &bytes.Buffer{}, ErrOut: &errOut})
	if code == 0 {
		t.Fatalf("runCLI index scope=all duplicate FQCN: expected nonzero exit, got 0 (stderr=%q)", errOut.String())
	}
	if !strings.Contains(errOut.String(), `duplicate FQCN "foo.Greeter"`) {
		t.Fatalf("stderr = %q, want duplicate FQCN message", errOut.String())
	}
}

func TestScopeDuplicateFQCNAcceptedUnderMain(t *testing.T) {
	root := m4FixtureRoot("scope-duplicate")
	var out bytes.Buffer
	code := runCLI([]string{"index", "--scope=main", root}, IO{Out: &out, ErrOut: &bytes.Buffer{}})
	if code != 0 {
		t.Fatalf("runCLI index scope=main duplicate FQCN: exit=%d", code)
	}
	if !strings.Contains(out.String(), "Greeter") {
		t.Fatalf("index scope=main output missing Greeter: %s", out.String())
	}
}

func TestM4LimitsGoldens(t *testing.T) {
	root := m4FixtureRoot("limits")
	tests := []struct {
		name   string
		args   []string
		golden string
	}{
		{name: "deep max-depth=2 mermaid", args: []string{"--max-depth=2", "app.Deep.a", root}, golden: "expected-deep-max-depth-2.mmd"},
		{name: "deep max-depth=2 dot", args: []string{"--format=dot", "--max-depth=2", "app.Deep.a", root}, golden: "expected-deep-max-depth-2.dot"},
		{name: "deep max-depth=2 json", args: []string{"--format=json", "--max-depth=2", "app.Deep.a", root}, golden: "expected-deep-max-depth-2.json"},
		{name: "diamond max-nodes=3 mermaid", args: []string{"--max-nodes=3", "app.Diamond.top", root}, golden: "expected-diamond-max-nodes-3.mmd"},
		{name: "diamond max-nodes=3 dot", args: []string{"--format=dot", "--max-nodes=3", "app.Diamond.top", root}, golden: "expected-diamond-max-nodes-3.dot"},
		{name: "diamond max-nodes=3 json", args: []string{"--format=json", "--max-nodes=3", "app.Diamond.top", root}, golden: "expected-diamond-max-nodes-3.json"},
		{name: "cycle mermaid", args: []string{"app.Looper.loop", root}, golden: "expected-cycle.mmd"},
		{name: "cycle dot", args: []string{"--format=dot", "app.Looper.loop", root}, golden: "expected-cycle.dot"},
		{name: "cycle json", args: []string{"--format=json", "app.Looper.loop", root}, golden: "expected-cycle.json"},
		{name: "fan max-nodes=5 mermaid", args: []string{"--max-nodes=5", "app.Fan.callAll", root}, golden: "expected-fan-max-nodes-5.mmd"},
		{name: "fan max-nodes=5 dot", args: []string{"--format=dot", "--max-nodes=5", "app.Fan.callAll", root}, golden: "expected-fan-max-nodes-5.dot"},
		{name: "fan max-nodes=5 json", args: []string{"--format=json", "--max-nodes=5", "app.Fan.callAll", root}, golden: "expected-fan-max-nodes-5.json"},
		{name: "mixed max-depth=3 mermaid", args: []string{"--max-depth=3", "app.Mixed.go", root}, golden: "expected-mixed-max-depth-3.mmd"},
		{name: "mixed max-depth=3 dot", args: []string{"--format=dot", "--max-depth=3", "app.Mixed.go", root}, golden: "expected-mixed-max-depth-3.dot"},
		{name: "mixed max-depth=3 json", args: []string{"--format=json", "--max-depth=3", "app.Mixed.go", root}, golden: "expected-mixed-max-depth-3.json"},
		{name: "deep max-nodes=1 mermaid", args: []string{"--max-nodes=1", "app.Deep.a", root}, golden: "expected-deep-max-nodes-1.mmd"},
		{name: "deep max-nodes=1 dot", args: []string{"--format=dot", "--max-nodes=1", "app.Deep.a", root}, golden: "expected-deep-max-nodes-1.dot"},
		{name: "deep max-nodes=1 json", args: []string{"--format=json", "--max-nodes=1", "app.Deep.a", root}, golden: "expected-deep-max-nodes-1.json"},
		{name: "deep default mermaid", args: []string{"app.Deep.a", root}, golden: "expected-deep-default.mmd"},
		{name: "deep default dot", args: []string{"--format=dot", "app.Deep.a", root}, golden: "expected-deep-default.dot"},
		{name: "deep default json", args: []string{"--format=json", "app.Deep.a", root}, golden: "expected-deep-default.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := runTrace(tt.args, &out); err != nil {
				t.Fatalf("runTrace(%v): %v", tt.args, err)
			}
			want, err := os.ReadFile(filepath.Join(root, tt.golden))
			if err != nil {
				t.Fatalf("read golden: %v", err)
			}
			if !bytes.Equal(out.Bytes(), want) {
				t.Fatalf("trace output mismatch (%s):\n%s", tt.golden, firstDiffContext(string(want), out.String()))
			}
		})
	}
}

func TestM4LimitsStabilityUnderRepeatedRuns(t *testing.T) {
	root := m4FixtureRoot("limits")
	args := []string{"--max-nodes=5", "app.Fan.callAll", root}
	var first bytes.Buffer
	if err := runTrace(args, &first); err != nil {
		t.Fatalf("first runTrace: %v", err)
	}
	for i := 0; i < 20; i++ {
		var out bytes.Buffer
		if err := runTrace(args, &out); err != nil {
			t.Fatalf("iteration %d runTrace: %v", i, err)
		}
		if !bytes.Equal(first.Bytes(), out.Bytes()) {
			t.Fatalf("iteration %d produced different output:\nfirst:\n%s\ncurrent:\n%s", i, first.String(), out.String())
		}
	}
}

func TestRunTraceM2M3RuntimeContextJSONGoldens(t *testing.T) {
	tests := []struct {
		name   string
		root   string
		target string
		golden string
	}{
		{name: "M2 trace", root: traceFixtureRoot(), target: "Workflow.start", golden: "expected.json"},
		{name: "M3 cross-file basic", root: m3FixtureRoot("cross-file-basic"), target: "app.Workflow.start", golden: "expected.json"},
		{name: "M3 imports", root: m3FixtureRoot("imports"), target: "app.Workflow.start", golden: "expected.json"},
		{name: "M3 inheritance", root: m3FixtureRoot("inheritance"), target: "app.Workflow.start", golden: "expected.json"},
		{name: "M3 constructors", root: m3FixtureRoot("constructors-methodrefs"), target: "app.Workflow.run", golden: "expected-run.json"},
		{name: "M3 method references", root: m3FixtureRoot("constructors-methodrefs"), target: "app.Workflow.references", golden: "expected-references.json"},
		{name: "M3 nested", root: m3FixtureRoot("constructors-methodrefs"), target: "refs.References.Nested.run", golden: "expected-nested.json"},
		{name: "M3 dispatch", root: m3FixtureRoot("dispatch"), target: "app.Workflow.start", golden: "expected.json"},
		{name: "M3 polymorphism", root: m3FixtureRoot("polymorphism"), target: "app.Workflow.start", golden: "expected.json"},
		{name: "M3 overloads calls", root: m3FixtureRoot("overloads"), target: "app.Workflow.calls", golden: "expected-calls.json"},
		{name: "M3 overloads string", root: m3FixtureRoot("overloads"), target: "overloads.Workflow.run(java.lang.String)", golden: "expected-string.json"},
		{name: "M3 overloads int", root: m3FixtureRoot("overloads"), target: "overloads.Workflow.run(int)", golden: "expected-int.json"},
		{name: "M3 source roots", root: m3FixtureRoot("source-roots"), target: "app.Workflow.start", golden: "expected.json"},
		{name: "runtime-context start", root: m4FixtureRoot("runtime-context"), target: "app.Workflow.start", golden: "expected-start.json"},
		{name: "runtime-context first", root: m4FixtureRoot("runtime-context"), target: "app.First.run", golden: "expected-first.json"},
		{name: "runtime-context ambiguous", root: m4FixtureRoot("runtime-context"), target: "app.Workflow.ambiguous", golden: "expected-ambiguous.json"},
		{name: "scope main", root: m4FixtureRoot("scope"), target: "app.Workflow.start", golden: "expected-start.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := runTrace([]string{"--format=json", tt.target, tt.root}, &out); err != nil {
				t.Fatalf("runTrace: %v", err)
			}
			want, err := os.ReadFile(filepath.Join(tt.root, tt.golden))
			if err != nil {
				t.Fatalf("read golden: %v", err)
			}
			if !bytes.Equal(out.Bytes(), want) {
				t.Fatalf("JSON golden mismatch (%s):\n%s", tt.golden, firstDiffContext(string(want), out.String()))
			}
		})
	}
}

func TestM4InteractiveGoldens(t *testing.T) {
	root := m4FixtureRoot("interactive")
	tests := []struct {
		name   string
		args   []string
		golden string
	}{
		{name: "terminal default mermaid", args: []string{"app.Workflow.start", root}, golden: "expected-terminal.mmd"},
		{name: "terminal default dot", args: []string{"--format=dot", "app.Workflow.start", root}, golden: "expected-terminal.dot"},
		{name: "terminal default json", args: []string{"--format=json", "app.Workflow.start", root}, golden: "expected-terminal.json"},
		{name: "all impls mermaid", args: []string{"--all-impls=true", "app.Workflow.start", root}, golden: "expected-all-impls.mmd"},
		{name: "all impls dot", args: []string{"--format=dot", "--all-impls=true", "app.Workflow.start", root}, golden: "expected-all-impls.dot"},
		{name: "all impls json", args: []string{"--format=json", "--all-impls=true", "app.Workflow.start", root}, golden: "expected-all-impls.json"},
		{name: "all impls max-impls=1 mermaid", args: []string{"--all-impls=true", "--max-impls=1", "app.Workflow.start", root}, golden: "expected-all-impls-max-impls-1.mmd"},
		{name: "all impls max-impls=1 dot", args: []string{"--format=dot", "--all-impls=true", "--max-impls=1", "app.Workflow.start", root}, golden: "expected-all-impls-max-impls-1.dot"},
		{name: "all impls max-impls=1 json", args: []string{"--format=json", "--all-impls=true", "--max-impls=1", "app.Workflow.start", root}, golden: "expected-all-impls-max-impls-1.json"},
		{name: "pick none keeps terminal", args: []string{"--pick-impls=contracts.A=none", "app.Workflow.start", root}, golden: "expected-terminal.mmd"},
		{name: "pick explicit leaves nested ambiguity", args: []string{"--pick-impls=contracts.A=app.AlphaA", "app.Workflow.start", root}, golden: "expected-pick-alpha.mmd"},
		{name: "pick multiple explicit mappings", args: []string{"--pick-impls=contracts.A=app.AlphaA,contracts.B=app.GammaB", "app.Workflow.start", root}, golden: "expected-pick-alpha-gamma.mmd"},
		{name: "pick all honors max impls", args: []string{"--pick-impls=contracts.A=all", "--max-impls=1", "app.Workflow.start", root}, golden: "expected-pick-all-max-impls-1.mmd"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := runTrace(tt.args, &out); err != nil {
				t.Fatalf("runTrace(%v): %v", tt.args, err)
			}
			want, err := os.ReadFile(filepath.Join(root, tt.golden))
			if err != nil {
				t.Fatalf("read golden: %v", err)
			}
			if !bytes.Equal(out.Bytes(), want) {
				t.Fatalf("trace output mismatch (%s):\n%s", tt.golden, firstDiffContext(string(want), out.String()))
			}
		})
	}
}

func TestM4PickImplsWorksAcrossFormats(t *testing.T) {
	root := m4FixtureRoot("interactive")
	for _, format := range []string{"mermaid", "dot", "json"} {
		t.Run(format, func(t *testing.T) {
			var out bytes.Buffer
			args := []string{"--format=" + format, "--pick-impls=contracts.A=app.AlphaA,contracts.B=app.GammaB", "app.Workflow.start", root}
			if err := runTrace(args, &out); err != nil {
				t.Fatalf("runTrace(%v): %v", args, err)
			}
			for _, want := range []string{"app.Workflow.start()", "app.AlphaA.run()", "app.GammaB.work()"} {
				if !strings.Contains(out.String(), want) {
					t.Fatalf("%s output missing %q:\n%s", format, want, out.String())
				}
			}
			unwanted := []string{"app.BetaA", "app.DeltaB", "[ambiguous:"}
			if format == "json" {
				// JSON preserves every dispatch candidate as metadata; only execution
				// nodes and ambiguous terminals must be absent from the selected graph.
				unwanted = []string{`"runtimeType": "app.BetaA"`, `"runtimeType": "app.DeltaB"`, `"kind": "ambiguousImplementation"`}
			}
			for _, unwanted := range unwanted {
				if strings.Contains(out.String(), unwanted) {
					t.Fatalf("%s output contains unselected %q:\n%s", format, unwanted, out.String())
				}
			}
		})
	}
}
