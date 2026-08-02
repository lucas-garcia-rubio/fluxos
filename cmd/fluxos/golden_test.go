package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func assertTraceGolden(t *testing.T, fixture, target, golden string) {
	t.Helper()
	root := m3FixtureRoot(fixture)
	assertTraceGoldenPair(t, root, target, golden, "")
}

func shortGoldenPath(golden string) string {
	extension := filepath.Ext(golden)
	return strings.TrimSuffix(golden, extension) + ".short" + extension
}

func assertTraceGoldenPair(t *testing.T, root, target, golden, format string) {
	t.Helper()
	baseArgs := []string{target, root}
	if format != "" {
		baseArgs = []string{"--format=" + format, target, root}
	}
	assertTraceArgsGoldenPair(t, root, baseArgs, golden)
}

func assertTraceArgsGoldenPair(t *testing.T, root string, baseArgs []string, golden string) {
	t.Helper()
	for _, mode := range []struct {
		name           string
		showFQCN       bool
		showFQCNParams bool
		golden         string
	}{
		{name: "default compact", golden: shortGoldenPath(golden)},
		{name: "show-fqcn full", showFQCN: true, showFQCNParams: true, golden: golden},
	} {
		t.Run(mode.name, func(t *testing.T) {
			args := append([]string{}, baseArgs...)
			if mode.showFQCN {
				args = append([]string{"--show-fqcn=true"}, args...)
			}
			if mode.showFQCNParams {
				args = append([]string{"--show-fqcn-params=true"}, args...)
			}
			var out bytes.Buffer
			if err := runTrace(args, &out); err != nil {
				t.Fatalf("runTrace(%v): %v", args, err)
			}
			want, err := os.ReadFile(filepath.Join(root, mode.golden))
			if err != nil {
				t.Fatalf("read golden %s/%s: %v", root, mode.golden, err)
			}
			if !bytes.Equal(out.Bytes(), want) {
				t.Fatalf("trace output mismatch for %s (%v, %s):\n%s", root, args, mode.golden, firstDiffContext(string(want), out.String()))
			}
		})
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
				assertTraceGoldenPair(t, root, tt.target, tt.golden, "")
				return
			}
			assertTraceGoldenPair(t, root, tt.target, tt.golden, tt.format)
		})
	}
}

func TestRuntimeContextShowFQCNParamsGoldens(t *testing.T) {
	root := m4FixtureRoot("runtime-context")
	for _, tt := range []struct {
		name   string
		format string
		golden string
	}{
		{name: "Mermaid", format: "mermaid", golden: "expected-start.params.mmd"},
		{name: "DOT", format: "dot", golden: "expected-start.params.dot"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{"--show-fqcn-params=true", "app.Workflow.start", root}
			if tt.format == "dot" {
				args = append([]string{"--format=dot"}, args...)
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
				t.Fatalf("trace output mismatch (%s):\n%s", tt.golden, firstDiffContext(string(want), out.String()))
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

func TestRunTraceExcludesUnresolvedAcrossFormats(t *testing.T) {
	root := m3FixtureRoot("constructors-methodrefs")
	wantNodes := map[string]string{
		"m_e8df649834e2": "app.Workflow.references()",
		"m_f9c86dc9202f": "model.BaseValue.<init>(java.lang.String)",
		"m_84ed2044c2ed": "model.BaseValue.value() [runtime: model.BaseValue]",
		"m_f97d577def56": "model.BaseValue.value() [runtime: refs.References]",
		"m_77b0ebe86b3f": "model.DefaultValue.<init>()",
		"m_bd677b62ab48": "refs.References.<init>()",
		"m_2292119d3129": "refs.References.own()",
		"m_6cdb9c52f7f1": "refs.References.references()",
		"m_bc924d7eafb8": "refs.References.Nested.<init>()",
		"m_10373ee5a41d": "refs.References.Nested.run()",
		"m_5c65cea9be33": "refs.References.Service.run()",
		"m_e3566a83e1a6": "support.Validator.normalize(java.lang.String)",
		"m_f121243d1d9b": "support.Validator.require(java.lang.String)",
	}
	wantDiagramNodes := map[string]string{
		"m_e8df649834e2": "Workflow.references()",
		"m_f9c86dc9202f": "BaseValue.<init>(String)",
		"m_84ed2044c2ed": "BaseValue.value() [runtime: BaseValue]",
		"m_f97d577def56": "BaseValue.value() [runtime: References]",
		"m_77b0ebe86b3f": "DefaultValue.<init>()",
		"m_bd677b62ab48": "References.<init>()",
		"m_2292119d3129": "References.own()",
		"m_6cdb9c52f7f1": "References.references()",
		"m_bc924d7eafb8": "References.Nested.<init>()",
		"m_10373ee5a41d": "References.Nested.run()",
		"m_5c65cea9be33": "References.Service.run()",
		"m_e3566a83e1a6": "Validator.normalize(String)",
		"m_f121243d1d9b": "Validator.require(String)",
	}
	wantEdges := [][2]string{
		{"m_e8df649834e2", "m_bd677b62ab48"},
		{"m_e8df649834e2", "m_6cdb9c52f7f1"},
		{"m_bd677b62ab48", "m_f9c86dc9202f"},
		{"m_2292119d3129", "m_f121243d1d9b"},
		{"m_6cdb9c52f7f1", "m_84ed2044c2ed"},
		{"m_6cdb9c52f7f1", "m_f97d577def56"},
		{"m_6cdb9c52f7f1", "m_77b0ebe86b3f"},
		{"m_6cdb9c52f7f1", "m_2292119d3129"},
		{"m_6cdb9c52f7f1", "m_bc924d7eafb8"},
		{"m_6cdb9c52f7f1", "m_10373ee5a41d"},
		{"m_6cdb9c52f7f1", "m_5c65cea9be33"},
		{"m_6cdb9c52f7f1", "m_e3566a83e1a6"},
		{"m_10373ee5a41d", "m_f121243d1d9b"},
		{"m_5c65cea9be33", "m_f121243d1d9b"},
		{"m_e3566a83e1a6", "m_f121243d1d9b"},
	}
	for _, format := range []string{"mermaid", "dot", "json"} {
		t.Run(format, func(t *testing.T) {
			args := []string{"--include-unresolved=false"}
			if format != "mermaid" {
				args = append(args, "--format="+format)
			}
			args = append(args, "app.Workflow.references", root)

			var out bytes.Buffer
			if err := runTrace(args, &out); err != nil {
				t.Fatalf("runTrace(%v): %v", args, err)
			}
			got := parseFilteredCLIOutput(t, format, out.String())
			wantLabels := wantDiagramNodes
			if format == "json" {
				wantLabels = wantNodes
			}
			if !reflect.DeepEqual(got.labels, wantLabels) {
				t.Fatalf("%s node labels differ:\ngot=%v\nwant=%v", format, got.labels, wantLabels)
			}
			if !reflect.DeepEqual(got.edges, wantEdges) {
				t.Fatalf("%s edges differ:\ngot=%v\nwant=%v", format, got.edges, wantEdges)
			}
			if format != "mermaid" {
				for id, kind := range got.kinds {
					if kind != "method" {
						t.Fatalf("%s node %s kind = %q, want method", format, id, kind)
					}
				}
			}
		})
	}
}

func TestRunTraceIncludeUnresolvedFalsePreservesDispatchTerminalsAcrossFormats(t *testing.T) {
	root := m3FixtureRoot("dispatch")
	for _, tt := range []struct {
		format string
		golden string
	}{
		{format: "mermaid", golden: "expected.short.mmd"},
		{format: "dot"},
		{format: "json", golden: "expected.json"},
	} {
		t.Run(tt.format, func(t *testing.T) {
			args := []string{"--include-unresolved=false"}
			if tt.format != "mermaid" {
				args = append(args, "--format="+tt.format)
			}
			args = append(args, "app.Workflow.start", root)
			var out bytes.Buffer
			if err := runTrace(args, &out); err != nil {
				t.Fatalf("runTrace(%v): %v", args, err)
			}
			var want []byte
			if tt.golden != "" {
				var err error
				want, err = os.ReadFile(filepath.Join(root, tt.golden))
				if err != nil {
					t.Fatalf("read golden: %v", err)
				}
			} else {
				defaultArgs := []string{"--format=" + tt.format, "app.Workflow.start", root}
				var defaultOut bytes.Buffer
				if err := runTrace(defaultArgs, &defaultOut); err != nil {
					t.Fatalf("default runTrace(%v): %v", defaultArgs, err)
				}
				want = defaultOut.Bytes()
			}
			if !bytes.Equal(out.Bytes(), want) {
				t.Fatalf("--include-unresolved=false changed non-unresolved %s output:\n%s", tt.format, firstDiffContext(string(want), out.String()))
			}
		})
	}
}

func TestRunTraceShowFQCNDoesNotChangeJSON(t *testing.T) {
	root := m4FixtureRoot("runtime-context")
	outputs := make([]string, 4)
	for i, mode := range []struct {
		showFQCN       bool
		showFQCNParams bool
	}{
		{},
		{showFQCN: true},
		{showFQCNParams: true},
		{showFQCN: true, showFQCNParams: true},
	} {
		args := []string{"--format=json"}
		if mode.showFQCN {
			args = append(args, "--show-fqcn=true")
		}
		if mode.showFQCNParams {
			args = append(args, "--show-fqcn-params=true")
		}
		args = append(args, "app.Workflow.start", root)
		var out bytes.Buffer
		if err := runTrace(args, &out); err != nil {
			t.Fatalf("runTrace(%v): %v", args, err)
		}
		outputs[i] = out.String()
	}
	for i := 1; i < len(outputs); i++ {
		if outputs[0] != outputs[i] {
			t.Fatalf("diagram label flags changed JSON output:\ndefault=%s\nmode %d=%s", outputs[0], i, outputs[i])
		}
	}
}

type filteredCLIOutput struct {
	labels map[string]string
	kinds  map[string]string
	edges  [][2]string
}

func parseFilteredCLIOutput(t *testing.T, format, output string) filteredCLIOutput {
	t.Helper()
	switch format {
	case "mermaid":
		return parseFilteredMermaid(t, output)
	case "dot":
		return parseFilteredDOT(t, output)
	case "json":
		return parseFilteredJSON(t, output)
	default:
		t.Fatalf("unknown output format %q", format)
		return filteredCLIOutput{}
	}
}

func parseFilteredMermaid(t *testing.T, output string) filteredCLIOutput {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
	if len(lines) == 0 || lines[0] != "flowchart TD" {
		t.Fatalf("invalid Mermaid header: %q", output)
	}
	got := filteredCLIOutput{labels: map[string]string{}, kinds: map[string]string{}}
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if from, to, ok := strings.Cut(line, " --> "); ok {
			got.edges = append(got.edges, [2]string{from, to})
			continue
		}
		id, label, ok := strings.Cut(line, `["`)
		if !ok || !strings.HasSuffix(label, `"]`) || id == "" {
			t.Fatalf("invalid Mermaid statement: %q", line)
		}
		if _, duplicate := got.labels[id]; duplicate {
			t.Fatalf("duplicate Mermaid node %q", id)
		}
		got.labels[id] = strings.TrimSuffix(label, `"]`)
	}
	return got
}

func parseFilteredDOT(t *testing.T, output string) filteredCLIOutput {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
	if len(lines) < 2 || lines[0] != "digraph fluxos {" || lines[len(lines)-1] != "}" {
		t.Fatalf("invalid DOT envelope: %q", output)
	}
	got := filteredCLIOutput{labels: map[string]string{}, kinds: map[string]string{}}
	for _, line := range lines[1 : len(lines)-1] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Contains(line, " -> ") {
			edgeLine := strings.TrimSuffix(line, ";")
			from, to, ok := strings.Cut(edgeLine, " -> ")
			if !ok {
				t.Fatalf("invalid DOT edge: %q", line)
			}
			from = strings.Trim(from, `"`)
			to = strings.Trim(to, `"`)
			got.edges = append(got.edges, [2]string{from, to})
			continue
		}
		const nodeMarker = `" [label="`
		if !strings.HasSuffix(line, `];`) {
			t.Fatalf("invalid DOT statement: %q", line)
		}
		id, rest, ok := strings.Cut(strings.TrimSuffix(line, `];`), nodeMarker)
		if !ok || !strings.HasPrefix(id, `"`) || !strings.HasSuffix(rest, `"`) {
			t.Fatalf("invalid DOT node: %q", line)
		}
		id = strings.TrimPrefix(id, `"`)
		label, kind, ok := strings.Cut(strings.TrimSuffix(rest, `"`), `", kind="`)
		if !ok || kind == "" {
			t.Fatalf("invalid DOT node: %q", line)
		}
		if _, duplicate := got.labels[id]; duplicate {
			t.Fatalf("duplicate DOT node %q", id)
		}
		got.labels[id] = label
		got.kinds[id] = kind
	}
	return got
}

func parseFilteredJSON(t *testing.T, output string) filteredCLIOutput {
	t.Helper()
	var payload struct {
		SchemaVersion int `json:"schemaVersion"`
		Nodes         []struct {
			ID    string `json:"id"`
			Kind  string `json:"kind"`
			Label string `json:"label"`
		} `json:"nodes"`
		Edges []struct {
			From string `json:"from"`
			To   string `json:"to"`
		} `json:"edges"`
	}
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if payload.SchemaVersion != 1 {
		t.Fatalf("schemaVersion = %d, want 1", payload.SchemaVersion)
	}
	got := filteredCLIOutput{labels: map[string]string{}, kinds: map[string]string{}}
	for _, node := range payload.Nodes {
		if _, duplicate := got.labels[node.ID]; duplicate {
			t.Fatalf("duplicate JSON node %q", node.ID)
		}
		got.labels[node.ID] = node.Label
		got.kinds[node.ID] = node.Kind
	}
	for _, edge := range payload.Edges {
		got.edges = append(got.edges, [2]string{edge.From, edge.To})
	}
	return got
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
			assertTraceGoldenPair(t, root, tt.target, tt.golden, tt.format)
		})
	}
}

func TestScopeAllTraceGolden(t *testing.T) {
	root := m4FixtureRoot("scope")
	for _, mode := range []struct {
		name           string
		showFQCN       bool
		showFQCNParams bool
		golden         string
	}{
		{name: "default compact", golden: "expected-start-all.short.mmd"},
		{name: "show-fqcn full", showFQCN: true, showFQCNParams: true, golden: "expected-start-all.mmd"},
	} {
		t.Run(mode.name, func(t *testing.T) {
			args := []string{"--scope=all", "app.Workflow.start", root}
			if mode.showFQCN {
				args = append([]string{"--show-fqcn=true"}, args...)
			}
			if mode.showFQCNParams {
				args = append([]string{"--show-fqcn-params=true"}, args...)
			}
			var out bytes.Buffer
			if err := runTrace(args, &out); err != nil {
				t.Fatalf("runTrace(%v): %v", args, err)
			}
			want, err := os.ReadFile(filepath.Join(root, mode.golden))
			if err != nil {
				t.Fatalf("read golden: %v", err)
			}
			if !bytes.Equal(out.Bytes(), want) {
				t.Fatalf("scope all trace mismatch (%s):\n%s", mode.golden, firstDiffContext(string(want), out.String()))
			}
		})
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
			if filepath.Ext(tt.golden) != ".json" {
				assertTraceArgsGoldenPair(t, root, tt.args, tt.golden)
				return
			}
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
			if filepath.Ext(tt.golden) != ".json" {
				assertTraceArgsGoldenPair(t, root, tt.args, tt.golden)
				return
			}
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
			wants := []string{"Workflow.start()", "AlphaA.run()", "GammaB.work()"}
			if format == "json" {
				wants = []string{"app.Workflow.start()", "app.AlphaA.run()", "app.GammaB.work()"}
			}
			for _, want := range wants {
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
