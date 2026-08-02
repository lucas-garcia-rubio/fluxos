package dot

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/lucas-garcia-rubio/fluxos/internal/render"
)

var errDOTWrite = errors.New("write failed")

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errDOTWrite
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) {
	return len(p) - 1, nil
}

func renderString(t *testing.T, snapshot render.Snapshot, showFQCN ...bool) string {
	t.Helper()
	var out bytes.Buffer
	show := len(showFQCN) > 0 && showFQCN[0]
	if err := Render(&out, snapshot, show); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return out.String()
}

func TestRenderEmptyGraph(t *testing.T) {
	if got, want := renderString(t, render.Snapshot{}), "digraph fluxos {\n}\n"; got != want {
		t.Fatalf("Render(empty) = %q, want %q", got, want)
	}
}

func TestRenderNodesKindsEdgesAndCycles(t *testing.T) {
	snapshot := render.Snapshot{
		Nodes: []render.NodeView{
			{ID: "m_a", Label: "A.start()", Kind: render.NodeMethod},
			{ID: "m_b", Label: "B.run() [unresolved]", Kind: render.NodeUnresolved},
		},
		Edges: []render.EdgeView{
			{From: "m_a", To: "m_b"},
			{From: "m_a", To: "m_b"},
			{From: "m_b", To: "m_a", Cycle: true},
		},
	}
	got := renderString(t, snapshot)
	if !strings.Contains(got, `"m_a" [label="A.start()", kind="method"];`) {
		t.Fatalf("method node missing:\n%s", got)
	}
	if !strings.Contains(got, `"m_b" [label="B.run() [unresolved]", kind="unresolved"];`) {
		t.Fatalf("terminal node missing:\n%s", got)
	}
	if count := strings.Count(got, `"m_a" -> "m_b";`); count != 2 {
		t.Fatalf("parallel edge count = %d, want 2:\n%s", count, got)
	}
	if !strings.Contains(got, `"m_b" -> "m_a" [color="red", style="dashed"];`) {
		t.Fatalf("cycle edge missing:\n%s", got)
	}
}

func TestRenderPreservesSnapshotOrder(t *testing.T) {
	snapshot := render.Snapshot{
		Nodes: []render.NodeView{
			{ID: "m_b", Label: "B", Kind: render.NodeMethod},
			{ID: "m_a", Label: "A", Kind: render.NodeMethod},
		},
		Edges: []render.EdgeView{
			{From: "m_b", To: "m_a"},
			{From: "m_a", To: "m_b"},
		},
	}
	got := renderString(t, snapshot)
	if strings.Index(got, `"m_b" [`) > strings.Index(got, `"m_a" [`) {
		t.Fatalf("renderer reordered nodes:\n%s", got)
	}
	if strings.Index(got, `"m_b" -> "m_a"`) > strings.Index(got, `"m_a" -> "m_b"`) {
		t.Fatalf("renderer reordered edges:\n%s", got)
	}
}

func TestQuoteEscapesDOTStrings(t *testing.T) {
	input := "quote\" slash\\ line\ncarriage\rtab\t\\N"
	want := "\"quote\\\" slash\\\\ line\\ncarriage\\rtab\t\\\\N\""
	if got := quote(input); got != want {
		t.Fatalf("quote = %q, want %q", got, want)
	}
}

func TestRenderPropagatesWriterErrors(t *testing.T) {
	if err := Render(failingWriter{}, render.Snapshot{}, false); !errors.Is(err, errDOTWrite) {
		t.Fatalf("Render writer error = %v", err)
	}
	if err := Render(shortWriter{}, render.Snapshot{}, false); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("Render short write error = %v, want io.ErrShortWrite", err)
	}
}

func TestRenderGolden(t *testing.T) {
	snapshot := render.Snapshot{
		Nodes: []render.NodeView{
			{ID: "m_a", Label: "com.example.A.start()", Kind: render.NodeMethod},
			{ID: "m_b", Label: "com.example.B.run() [ambiguous: 2 implementations]", Kind: render.NodeAmbiguousImplementation},
			{ID: "m_escape", Label: "Escaped \"quote\" \\path\nnext\rreturn\t\\N", Kind: render.NodeExternal},
		},
		Edges: []render.EdgeView{
			{From: "m_a", To: "m_b"},
			{From: "m_a", To: "m_b"},
			{From: "m_b", To: "m_a", Cycle: true},
		},
	}
	for _, mode := range []struct {
		name     string
		showFQCN bool
		golden   string
	}{
		{name: "default compact", golden: "testdata/callgraph.short.golden"},
		{name: "show-fqcn", showFQCN: true, golden: "testdata/callgraph.fqcn.golden"},
	} {
		t.Run(mode.name, func(t *testing.T) {
			want, err := os.ReadFile(mode.golden)
			if err != nil {
				t.Fatalf("read golden: %v", err)
			}
			if got := renderString(t, snapshot, mode.showFQCN); got != string(want) {
				t.Fatalf("golden mismatch:\ngot:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestRenderOutputIsAcceptedByGraphviz(t *testing.T) {
	dotPath, err := exec.LookPath("dot")
	if err != nil {
		t.Skip("Graphviz dot is not installed")
	}
	source, err := os.ReadFile("testdata/callgraph.golden")
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	cmd := exec.Command(dotPath, "-Tsvg")
	cmd.Stdin = bytes.NewReader(source)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("dot -Tsvg: %v", err)
	}
	if len(output) == 0 {
		t.Fatal("dot -Tsvg produced empty output")
	}
}

func TestRenderAppendsTruncationMarkersAfterAnalysisNodes(t *testing.T) {
	caller := render.ExecutionView{
		Method:          render.MethodView{TypeFQCN: "app.Workflow", Method: "start", Signature: "()"},
		RuntimeTypeFQCN: "app.Workflow",
	}
	snapshot := render.Snapshot{
		SchemaVersion: render.SnapshotSchemaVersion,
		Nodes:         []render.NodeView{{ID: "m_root", Label: "app.Workflow.start()", Kind: render.NodeMethod}},
		Edges:         []render.EdgeView{},
		Truncations: []render.TruncationView{{
			ID: "t_abc123", Kind: "maxNodes", Caller: caller,
			Call:    render.CallView{File: "App.java", StartByte: 100, MethodName: "run"},
			Omitted: 3, Note: "",
		}},
	}
	got := renderString(t, snapshot)
	if !strings.Contains(got, `"t_abc123" [shape=note, label="truncation: node limit; omitted 3 while tracing Workflow.start()"];`) {
		t.Fatalf("truncation marker missing or malformed:\n%s", got)
	}
	rootIdx := strings.Index(got, `"m_root"`)
	truncIdx := strings.Index(got, `"t_abc123"`)
	if rootIdx == -1 || truncIdx == -1 || rootIdx > truncIdx {
		t.Fatalf("truncation marker should follow analysis nodes:\n%s", got)
	}
	if strings.Contains(got, `"t_abc123" ->`) || strings.Contains(got, `-> "t_abc123"`) {
		t.Fatalf("truncation marker must not participate in edges:\n%s", got)
	}
}

func TestRenderShowFQCNRestoresFullNodeAndTruncationLabels(t *testing.T) {
	caller := render.ExecutionView{Method: render.MethodView{TypeFQCN: "refs.References.Nested", Method: "run", Signature: "()"}, RuntimeTypeFQCN: "app.First"}
	snapshot := render.Snapshot{
		Nodes: []render.NodeView{{
			ID: "m_nested", Label: "refs.References.Nested.run() [runtime: app.First]",
			Execution: caller,
		}},
		Truncations: []render.TruncationView{{ID: "t_nested", Kind: "maxNodes", Caller: caller, Omitted: 1}},
	}
	got := renderString(t, snapshot, true)
	if !strings.Contains(got, `"m_nested" [label="refs.References.Nested.run() [runtime: app.First]"`) {
		t.Fatalf("full node label missing:\n%s", got)
	}
	if !strings.Contains(got, `"t_nested" [shape=note, label="truncation: node limit; omitted 1 while tracing refs.References.Nested.run()"]`) {
		t.Fatalf("full truncation label missing:\n%s", got)
	}
}

func TestTruncationLabelHidesSyntheticTerminalHandle(t *testing.T) {
	snapshot := render.Snapshot{Truncations: []render.TruncationView{{
		ID: "t_internal", Kind: "maxNodes", Omitted: 1,
		Caller: render.ExecutionView{Method: render.MethodView{TypeFQCN: "Contract#ambimpl#abc", Method: "run"}},
	}}}
	got := renderString(t, snapshot)
	if strings.Contains(got, "#ambimpl#") || !strings.Contains(got, "Contract.run()") {
		t.Fatalf("truncation label exposed synthetic handle: %s", got)
	}
}
