package mermaid

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
	"github.com/lucas-garcia-rubio/fluxos/internal/graph"
	"github.com/lucas-garcia-rubio/fluxos/internal/resolve"
)

func handle(typeFQCN, method string, signature ...string) resolve.MethodHandle {
	sig := "()"
	if len(signature) > 0 {
		sig = signature[0]
	}
	return resolve.MethodHandle{TypeFQCN: typeFQCN, Method: method, Signature: sig}
}

func TestRenderEmptyGraph(t *testing.T) {
	if got, want := Render(graph.NewGraph()), "flowchart TD\n"; got != want {
		t.Fatalf("Render(empty):\n%q\nwant:\n%q", got, want)
	}
}

func TestRenderNilGraph(t *testing.T) {
	if got, want := Render(nil), "flowchart TD\n"; got != want {
		t.Fatalf("Render(nil):\n%q\nwant:\n%q", got, want)
	}
}

func TestRenderIsolatedNode(t *testing.T) {
	g := graph.NewGraph()
	h := handle("com.example.Worker", "run")
	g.GetOrCreate(h)

	want := "flowchart TD\n  " + nodeID(h) + "[\"com.example.Worker.run()\"]\n"
	if got := Render(g); got != want {
		t.Fatalf("Render(isolated):\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderIsDeterministic(t *testing.T) {
	a := handle("com.example.A", "start")
	b := handle("com.example.B", "work")

	first := graph.NewGraph()
	first.AddEdge(a, b, java.CallSite{File: "A.java", Line: 20}, false)
	first.AddEdge(a, b, java.CallSite{File: "A.java", Line: 10}, false)

	second := graph.NewGraph()
	second.AddEdge(a, b, java.CallSite{File: "A.java", Line: 10}, false)
	second.AddEdge(a, b, java.CallSite{File: "A.java", Line: 20}, false)

	if got, want := Render(first), Render(second); got != want {
		t.Fatalf("insertion order changed output:\nfirst:\n%s\nsecond:\n%s", got, want)
	}
}

func TestRenderCycleAndMultiedges(t *testing.T) {
	a := handle("A", "a")
	b := handle("B", "b")
	g := graph.NewGraph()
	g.AddEdge(a, b, java.CallSite{Line: 10}, false)
	g.AddEdge(a, b, java.CallSite{Line: 20}, false)
	g.AddEdge(b, a, java.CallSite{Line: 30}, true)

	got := Render(g)
	forward := "  " + nodeID(a) + " --> " + nodeID(b) + "\n"
	if count := strings.Count(got, forward); count != 2 {
		t.Fatalf("expected 2 parallel edges, got %d:\n%s", count, got)
	}
	cycle := "  %% cycle\n  " + nodeID(b) + " --> " + nodeID(a) + "\n"
	if !strings.Contains(got, cycle) {
		t.Fatalf("cycle marker is missing or misplaced:\n%s", got)
	}
}

func TestRenderSelfLoop(t *testing.T) {
	h := handle("Recursive", "call")
	g := graph.NewGraph()
	g.AddEdge(h, h, java.CallSite{}, true)

	want := "  %% cycle\n  " + nodeID(h) + " --> " + nodeID(h) + "\n"
	if got := Render(g); !strings.Contains(got, want) {
		t.Fatalf("self-loop is missing:\n%s", got)
	}
}

func TestNodeIDEscapingAndStability(t *testing.T) {
	h := handle(`com.example."Worker"`, "run")
	id := nodeID(h)
	if !regexp.MustCompile(`^m_[0-9a-f]{12}$`).MatchString(id) {
		t.Fatalf("invalid node ID %q", id)
	}
	if id != nodeID(h) {
		t.Fatal("node ID changed for the same handle")
	}
	if id == nodeID(handle(h.TypeFQCN, "other")) {
		t.Fatal("different handles produced the same node ID")
	}
	if got, want := nodeLabel(h), `com.example.#quot;Worker#quot;.run()`; got != want {
		t.Fatalf("escaped label: got %q, want %q", got, want)
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
	if got := Render(g); got != string(want) {
		t.Fatalf("golden mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestNodeIDAndLabelIncludeSignature(t *testing.T) {
	withoutArgs := handle("Service", "run", "()")
	withString := handle("Service", "run", "(String)")
	if nodeID(withoutArgs) == nodeID(withString) {
		t.Fatal("overloads produced the same node ID")
	}
	if got, want := nodeLabel(withString), "Service.run(String)"; got != want {
		t.Fatalf("node label = %q, want %q", got, want)
	}
}

func TestRenderSortsOverloads(t *testing.T) {
	g := graph.NewGraph()
	withString := handle("Service", "run", "(String)")
	withoutArgs := handle("Service", "run", "()")
	g.GetOrCreate(withString)
	g.GetOrCreate(withoutArgs)

	got := Render(g)
	if strings.Index(got, "Service.run()") > strings.Index(got, "Service.run(String)") {
		t.Fatalf("overloads are not sorted by signature:\n%s", got)
	}
}
