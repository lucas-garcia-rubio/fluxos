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

func handle(typeFQCN, method string) resolve.MethodHandle {
	return resolve.MethodHandle{TypeFQCN: typeFQCN, Method: method, Signature: "()"}
}

func renderString(t *testing.T, snapshot render.Snapshot, direction Direction) string {
	t.Helper()
	var out bytes.Buffer
	if err := Render(&out, snapshot, direction); err != nil {
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
		Nodes: []render.NodeView{{ID: "m_worker", Label: "com.example.Worker.run()"}},
		Edges: []render.EdgeView{},
	}
	want := "flowchart TD\n  m_worker[\"com.example.Worker.run()\"]\n"
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
	err := Render(&out, render.Snapshot{}, Direction("DOWN"))
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
	if err := Render(failingWriter{}, render.Snapshot{}, DirectionTD); !errors.Is(err, errMermaidWrite) {
		t.Fatalf("Render writer error = %v", err)
	}
	if err := Render(shortWriter{}, render.Snapshot{}, DirectionTD); !errors.Is(err, io.ErrShortWrite) {
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
	g.AddEdge(a, b, java.CallSite{File: "A.java", Line: 20}, false)
	g.AddEdge(c, a, java.CallSite{File: "C.java", Line: 40}, true)
	g.AddEdge(b, c, java.CallSite{File: "B.java", Line: 30}, false)
	g.AddEdge(a, b, java.CallSite{File: "A.java", Line: 10}, false)

	want, err := os.ReadFile("testdata/callgraph.golden")
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if got := renderString(t, render.NewSnapshot(g, a), DirectionTD); got != string(want) {
		t.Fatalf("golden mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}
