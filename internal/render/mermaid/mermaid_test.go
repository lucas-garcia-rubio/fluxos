package mermaid

import (
	"os"
	"strings"
	"testing"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
	"github.com/lucas-garcia-rubio/fluxos/internal/graph"
	"github.com/lucas-garcia-rubio/fluxos/internal/render"
	"github.com/lucas-garcia-rubio/fluxos/internal/resolve"
)

func handle(typeFQCN, method string) resolve.MethodHandle {
	return resolve.MethodHandle{TypeFQCN: typeFQCN, Method: method, Signature: "()"}
}

func TestRenderSnapshotEmpty(t *testing.T) {
	snapshot := render.NewSnapshot(nil, handle("Target", "run"))
	if got, want := RenderSnapshot(snapshot), "flowchart TD\n"; got != want {
		t.Fatalf("RenderSnapshot(empty) = %q, want %q", got, want)
	}
}

func TestRenderSnapshotIsolatedNode(t *testing.T) {
	snapshot := render.Snapshot{
		Nodes: []render.NodeView{{ID: "m_worker", Label: "com.example.Worker.run()"}},
		Edges: []render.EdgeView{},
	}
	want := "flowchart TD\n  m_worker[\"com.example.Worker.run()\"]\n"
	if got := RenderSnapshot(snapshot); got != want {
		t.Fatalf("RenderSnapshot(isolated):\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderSnapshotEscapesLabels(t *testing.T) {
	snapshot := render.Snapshot{
		Nodes: []render.NodeView{{ID: "m_worker", Label: `com.example."Worker".run()`}},
		Edges: []render.EdgeView{},
	}
	want := `m_worker["com.example.#quot;Worker#quot;.run()"]`
	if got := RenderSnapshot(snapshot); !strings.Contains(got, want) {
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
	got := RenderSnapshot(snapshot)
	if count := strings.Count(got, "  m_a --> m_b\n"); count != 2 {
		t.Fatalf("parallel edge count = %d, want 2:\n%s", count, got)
	}
	if !strings.Contains(got, "  %% cycle\n  m_b --> m_a\n") {
		t.Fatalf("cycle marker missing or misplaced:\n%s", got)
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
	if got := RenderSnapshot(render.NewSnapshot(g, a)); got != string(want) {
		t.Fatalf("golden mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}
