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

// methodNode wraps a handle as a Node with Kind=NodeMethod for legacy label tests.
func methodNode(h resolve.MethodHandle) *graph.Node {
	return &graph.Node{Handle: h}
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

func TestSortedEdgesUsesReferenceTieBreakers(t *testing.T) {
	a := handle("A", "start")
	b := handle("B", "run")
	g := graph.NewGraph()
	g.AddEdge(a, b, java.CallSite{File: "A.java", Line: 1, StartByte: 20, Kind: java.CallMethodReference}, false)
	g.AddEdge(a, b, java.CallSite{File: "A.java", Line: 1, StartByte: 10, Kind: java.CallMethodReference}, false)
	g.AddEdge(a, b, java.CallSite{File: "A.java", Line: 1, StartByte: 10, Kind: java.CallInvocation}, false)

	edges := sortedEdges(g)
	if edges[0].Call.Kind != java.CallInvocation || edges[1].Call.StartByte != 10 || edges[1].Call.Kind != java.CallMethodReference || edges[2].Call.StartByte != 20 {
		t.Fatalf("edge order = %+v", edges)
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
	if got, want := nodeLabel(methodNode(h)), `com.example.#quot;Worker#quot;.run()`; got != want {
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
	if got, want := nodeLabel(methodNode(withString)), "Service.run(String)"; got != want {
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

func terminalNode(receiver, method string, kind graph.NodeKind, candidates []string, call java.CallSite) *graph.Node {
	handle := resolve.TerminalHandle(receiver, method, "", terminalEquivalentKind(kind), call)
	return &graph.Node{Handle: handle, Kind: kind, Candidates: candidates}
}

func terminalEquivalentKind(kind graph.NodeKind) resolve.ResolutionKind {
	switch kind {
	case graph.NodeTerminalUnresolved:
		return resolve.ResolutionUnresolved
	case graph.NodeTerminalNoImplementation:
		return resolve.ResolutionNoImplementation
	case graph.NodeTerminalAmbiguousType:
		return resolve.ResolutionAmbiguousType
	case graph.NodeTerminalAmbiguousOverload:
		return resolve.ResolutionAmbiguousOverload
	case graph.NodeTerminalAmbiguousImplementation:
		return resolve.ResolutionAmbiguousImplementation
	default:
		return resolve.ResolutionConcrete
	}
}

func TestRenderTerminalLabelsByKind(t *testing.T) {
	cases := []struct {
		name       string
		kind       graph.NodeKind
		candidates []string
		want       string
	}{
		{name: "Unresolved", kind: graph.NodeTerminalUnresolved, want: "contract.Empty.run() [unresolved]"},
		{name: "NoImplementation", kind: graph.NodeTerminalNoImplementation, want: "contract.Empty.run() [no implementation]"},
		{name: "AmbiguousType", kind: graph.NodeTerminalAmbiguousType, want: "contract.Empty.run() [ambiguous type]"},
		{name: "AmbiguousOverload", kind: graph.NodeTerminalAmbiguousOverload, want: "contract.Empty.run() [ambiguous overload]"},
		{name: "AmbiguousImplementation", kind: graph.NodeTerminalAmbiguousImplementation, candidates: []string{"a.A", "b.B"}, want: "contract.Empty.run() [ambiguous: 2 implementations]"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			node := terminalNode("contract.Empty", "run", tt.kind, tt.candidates, java.CallSite{File: "X.java", Line: 10, StartByte: 100})
			if got := nodeLabel(node); got != tt.want {
				t.Fatalf("nodeLabel = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderTerminalLabelTruncatesHashSuffix(t *testing.T) {
	// Two call sites producing the same receiver/method/kind must render the
	// same label prefix even though their node IDs differ.
	call1 := java.CallSite{File: "X.java", Line: 10, StartByte: 100}
	call2 := java.CallSite{File: "X.java", Line: 20, StartByte: 200}
	n1 := terminalNode("contract.Svc", "run", graph.NodeTerminalNoImplementation, nil, call1)
	n2 := terminalNode("contract.Svc", "run", graph.NodeTerminalNoImplementation, nil, call2)
	if nodeID(n1.Handle) == nodeID(n2.Handle) {
		t.Fatal("call sites should produce distinct node IDs")
	}
	if nodeLabel(n1) != nodeLabel(n2) {
		t.Fatalf("labels should match after hash truncation: %q vs %q", nodeLabel(n1), nodeLabel(n2))
	}
}

func TestRenderTerminalIDsAreStable(t *testing.T) {
	call := java.CallSite{File: "X.java", Line: 10, StartByte: 100}
	n := terminalNode("contract.Svc", "run", graph.NodeTerminalAmbiguousImplementation, []string{"a.A", "b.B"}, call)
	id := nodeID(n.Handle)
	if !regexp.MustCompile(`^m_[0-9a-f]{12}$`).MatchString(id) {
		t.Fatalf("terminal node ID format invalid: %q", id)
	}
	// Same call site + same receiver/method/kind must be stable.
	if id != nodeID(terminalNode("contract.Svc", "run", graph.NodeTerminalAmbiguousImplementation, []string{"a.A", "b.B"}, call).Handle) {
		t.Fatal("terminal node ID changed for identical inputs")
	}
}

func TestRenderTerminalAndConcreteNodesCoexist(t *testing.T) {
	g := graph.NewGraph()
	concrete := handle("contract.Impl", "run")
	terminal := terminalNode("contract.Svc", "run", graph.NodeTerminalNoImplementation, nil, java.CallSite{File: "X.java", Line: 10})
	g.GetOrCreate(concrete)
	g.MarkTerminal(terminal.Handle, graph.NodeTerminalNoImplementation, "no concrete implementations", nil)

	got := Render(g)
	if !strings.Contains(got, "contract.Impl.run()") {
		t.Fatalf("concrete node missing: %s", got)
	}
	if !strings.Contains(got, "contract.Svc.run() [no implementation]") {
		t.Fatalf("terminal label missing suffix: %s", got)
	}
}
