package mermaid

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
	"github.com/lucas-garcia-rubio/fluxos/internal/graph"
	"github.com/lucas-garcia-rubio/fluxos/internal/render"
	"github.com/lucas-garcia-rubio/fluxos/internal/resolve"
)

var errMermaidWrite = errors.New("write failed")

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errMermaidWrite
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) {
	return len(p) - 1, nil
}

func handle(typeFQCN, method string) resolve.ExecutionKey {
	return resolve.ExecutionKey{
		Method:          resolve.MethodHandle{TypeFQCN: typeFQCN, Method: method, Signature: "()"},
		RuntimeTypeFQCN: typeFQCN,
	}
}

func renderString(t *testing.T, snapshot render.Snapshot, direction Direction, showFQCN ...bool) string {
	t.Helper()
	var out bytes.Buffer
	show := len(showFQCN) > 0 && showFQCN[0]
	if err := Render(&out, snapshot, direction, show); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return out.String()
}

func TestRenderSnapshotEmpty(t *testing.T) {
	snapshot := render.NewSnapshot(nil, handle("Target", "run"))
	if got, want := renderString(t, snapshot, DirectionTD), "flowchart TD\n"; got != want {
		t.Fatalf("Render(empty) = %q, want %q", got, want)
	}
}

func TestRenderSnapshotIsolatedNode(t *testing.T) {
	snapshot := render.Snapshot{
		Nodes: []render.NodeView{{ID: "m_worker", Label: "com.example.Worker.run()", Execution: render.ExecutionView{Method: render.MethodView{TypeFQCN: "com.example.Worker"}}}},
		Edges: []render.EdgeView{},
	}
	want := "flowchart TD\n  m_worker[\"Worker.run()\"]\n"
	if got := renderString(t, snapshot, DirectionTD); got != want {
		t.Fatalf("Render(isolated):\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderDirectionsChangeOnlyHeader(t *testing.T) {
	snapshot := render.Snapshot{
		Nodes: []render.NodeView{{ID: "m_worker", Label: "Worker.run()"}},
		Edges: []render.EdgeView{},
	}
	td := renderString(t, snapshot, DirectionTD)
	for _, direction := range []Direction{DirectionLR, DirectionBT, DirectionRL} {
		got := renderString(t, snapshot, direction)
		want := strings.Replace(td, "flowchart TD\n", "flowchart "+string(direction)+"\n", 1)
		if got != want {
			t.Fatalf("direction %s changed body:\ngot:\n%s\nwant:\n%s", direction, got, want)
		}
	}
}

func TestRenderRejectsInvalidDirectionBeforeWrite(t *testing.T) {
	var out bytes.Buffer
	err := Render(&out, render.Snapshot{}, Direction("DOWN"), false)
	if err == nil || !strings.Contains(err.Error(), "invalid Mermaid direction") {
		t.Fatalf("Render invalid direction error = %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("invalid direction wrote %q", out.String())
	}
}

func TestRenderSnapshotEscapesLabels(t *testing.T) {
	snapshot := render.Snapshot{
		Nodes: []render.NodeView{{ID: "m_worker", Label: `com.example."Worker".run()`}},
		Edges: []render.EdgeView{},
	}
	want := `m_worker["com.example.#quot;Worker#quot;.run()"]`
	if got := renderString(t, snapshot, DirectionTD); !strings.Contains(got, want) {
		t.Fatalf("escaped node missing from:\n%s", got)
	}
}

func TestRenderSnapshotPreservesCyclesAndMultiedges(t *testing.T) {
	snapshot := render.Snapshot{
		Nodes: []render.NodeView{
			{ID: "m_a", Label: "A.a()"},
			{ID: "m_b", Label: "B.b()"},
		},
		Edges: []render.EdgeView{
			{From: "m_a", To: "m_b"},
			{From: "m_a", To: "m_b"},
			{From: "m_b", To: "m_a", Cycle: true},
		},
	}
	got := renderString(t, snapshot, DirectionTD)
	if count := strings.Count(got, "  m_a --> m_b\n"); count != 2 {
		t.Fatalf("parallel edge count = %d, want 2:\n%s", count, got)
	}
	if !strings.Contains(got, "  %% cycle\n  m_b --> m_a\n") {
		t.Fatalf("cycle marker missing or misplaced:\n%s", got)
	}
}

func TestRenderPropagatesWriterErrors(t *testing.T) {
	if err := Render(failingWriter{}, render.Snapshot{}, DirectionTD, false); !errors.Is(err, errMermaidWrite) {
		t.Fatalf("Render writer error = %v", err)
	}
	if err := Render(shortWriter{}, render.Snapshot{}, DirectionTD, false); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("Render short write error = %v, want io.ErrShortWrite", err)
	}
}

func TestRenderGolden(t *testing.T) {
	a := handle("com.example.A", "start")
	b := handle("com.example.B", "work")
	c := handle("com.example.C", "finish")
	g := graph.NewGraph()
	g.GetOrCreate(c)
	g.GetOrCreate(a)
	g.GetOrCreate(b)
	g.AddEdge(a, b, java.CallSite{File: "A.java", Line: 20}, nil, false)
	g.AddEdge(c, a, java.CallSite{File: "C.java", Line: 40}, nil, true)
	g.AddEdge(b, c, java.CallSite{File: "B.java", Line: 30}, nil, false)
	g.AddEdge(a, b, java.CallSite{File: "A.java", Line: 10}, nil, false)

	snapshot := render.NewSnapshot(g, a)
	for _, mode := range []struct {
		name     string
		showFQCN bool
		golden   string
	}{
		{name: "default compact", golden: "testdata/callgraph.short.golden"},
		{name: "show-fqcn", showFQCN: true, golden: "testdata/callgraph.golden"},
	} {
		t.Run(mode.name, func(t *testing.T) {
			want, err := os.ReadFile(mode.golden)
			if err != nil {
				t.Fatalf("read golden: %v", err)
			}
			if got := renderString(t, snapshot, DirectionTD, mode.showFQCN); got != string(want) {
				t.Fatalf("golden mismatch:\ngot:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestRenderAppendsTruncationMarkersAfterAnalysisNodes(t *testing.T) {
	caller := render.ExecutionView{
		Method:          render.MethodView{TypeFQCN: "app.Workflow", Method: "start", Signature: "()"},
		RuntimeTypeFQCN: "app.Workflow",
	}
	snapshot := render.Snapshot{
		SchemaVersion: render.SnapshotSchemaVersion,
		Nodes:         []render.NodeView{{ID: "m_root", Label: "app.Workflow.start()"}},
		Edges:         []render.EdgeView{},
		Truncations: []render.TruncationView{{
			ID: "t_abc123", Kind: "maxNodes", Caller: caller,
			Call:    render.CallView{File: "App.java", StartByte: 100, MethodName: "run"},
			Omitted: 3, Note: "",
		}},
	}
	got := renderString(t, snapshot, DirectionTD)
	if !strings.Contains(got, "  t_abc123[\"% truncation: node limit; omitted 3 while tracing Workflow.start()\"]\n") {
		t.Fatalf("truncation marker missing or malformed:\n%s", got)
	}
	// Marker aparece após o analysis node.
	rootIdx := strings.Index(got, "m_root")
	truncIdx := strings.Index(got, "t_abc123")
	if rootIdx == -1 || truncIdx == -1 || rootIdx > truncIdx {
		t.Fatalf("truncation marker should follow analysis nodes:\n%s", got)
	}
	// Não participa de edges.
	if strings.Contains(got, "t_abc123 -->") || strings.Contains(got, "--> t_abc123") {
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
	got := renderString(t, snapshot, DirectionTD, true)
	if !strings.Contains(got, `m_nested["refs.References.Nested.run() [runtime: app.First]"]`) {
		t.Fatalf("full node label missing:\n%s", got)
	}
	if !strings.Contains(got, `t_nested["% truncation: node limit; omitted 1 while tracing refs.References.Nested.run()"]`) {
		t.Fatalf("full truncation label missing:\n%s", got)
	}
}

func TestTruncationLabelHidesSyntheticTerminalHandle(t *testing.T) {
	snapshot := render.Snapshot{Truncations: []render.TruncationView{{
		ID: "t_internal", Kind: "maxNodes", Omitted: 1,
		Caller: render.ExecutionView{Method: render.MethodView{TypeFQCN: "Contract#ambimpl#abc", Method: "run"}},
	}}}
	got := renderString(t, snapshot, DirectionTD)
	if strings.Contains(got, "#ambimpl#") || !strings.Contains(got, "Contract.run()") {
		t.Fatalf("truncation label exposed synthetic handle: %s", got)
	}
}
