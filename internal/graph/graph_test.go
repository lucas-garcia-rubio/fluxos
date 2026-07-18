package graph

import (
	"testing"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
	"github.com/lucas-garcia-rubio/fluxos/internal/resolve"
)

func TestNewGraphEmpty(t *testing.T) {
	g := NewGraph()
	if len(g.Nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(g.Nodes))
	}
	if len(g.Edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(g.Edges))
	}
}

func TestGetOrCreate(t *testing.T) {
	g := NewGraph()
	h := resolve.MethodHandle{TypeFQCN: "com.foo.User", Method: "getName", Signature: "()"}

	n1 := g.GetOrCreate(h)
	if n1 == nil {
		t.Fatal("GetOrCreate returned nil")
	}
	if n1.State != StateWhite {
		t.Errorf("expected new node StateWhite (0), got %d", n1.State)
	}
	if len(g.Nodes) != 1 {
		t.Errorf("expected 1 node in graph, got %d", len(g.Nodes))
	}

	// Segunda chamada com mesmo handle devolve o MESMO ponteiro (não cria outro).
	n2 := g.GetOrCreate(h)
	if n1 != n2 {
		t.Error("GetOrCreate returned different pointer for same handle — should reuse")
	}
	if len(g.Nodes) != 1 {
		t.Errorf("after second GetOrCreate same handle, expected still 1 node, got %d", len(g.Nodes))
	}
}

func TestIsGrayBlackOnMissingHandle(t *testing.T) {
	g := NewGraph()
	h := resolve.MethodHandle{TypeFQCN: "com.foo.User", Method: "getName", Signature: "()"}

	if g.IsGray(h) {
		t.Error("IsGray should be false for non-existent handle")
	}
	if g.IsBlack(h) {
		t.Error("IsBlack should be false for non-existent handle")
	}
}

func TestMarkGrayAndIsGray(t *testing.T) {
	g := NewGraph()
	h := resolve.MethodHandle{TypeFQCN: "com.foo.User", Method: "getName", Signature: "()"}

	g.MarkGray(h)

	if !g.IsGray(h) {
		t.Error("after MarkGray, IsGray should be true")
	}
	if g.IsBlack(h) {
		t.Error("after MarkGray, IsBlack should be false")
	}
}

func TestMarkBlackAndIsBlack(t *testing.T) {
	g := NewGraph()
	h := resolve.MethodHandle{TypeFQCN: "com.foo.User", Method: "getName", Signature: "()"}

	g.MarkBlack(h)

	if g.IsGray(h) {
		t.Error("after MarkBlack, IsGray should be false")
	}
	if !g.IsBlack(h) {
		t.Error("after MarkBlack, IsBlack should be true")
	}
}

func TestMarkBlackOverridesGray(t *testing.T) {
	g := NewGraph()
	h := resolve.MethodHandle{TypeFQCN: "com.foo.User", Method: "getName", Signature: "()"}

	g.MarkGray(h)
	g.MarkBlack(h)

	if g.IsGray(h) {
		t.Error("after MarkGray then MarkBlack, IsGray should be false")
	}
	if !g.IsBlack(h) {
		t.Error("after MarkGray then MarkBlack, IsBlack should be true")
	}
}

func TestAddEdgeCreatesNodes(t *testing.T) {
	g := NewGraph()
	from := resolve.MethodHandle{TypeFQCN: "com.foo.User", Method: "getName", Signature: "()"}
	to := resolve.MethodHandle{TypeFQCN: "com.foo.Main", Method: "run", Signature: "()"}
	call := java.CallSite{MethodName: "run", Receiver: "user"}

	g.AddEdge(from, to, call, true)

	if len(g.Nodes) != 2 {
		t.Errorf("expected 2 nodes after AddEdge, got %d", len(g.Nodes))
	}
	if len(g.Edges) != 1 {
		t.Errorf("expected 1 edge, got %d", len(g.Edges))
	}

	// Nodes criados com StateWhite.
	if !g.IsGray(from) && !g.IsBlack(from) {
		// from exists and is white (not gray, not black)
		if n, ok := g.Nodes[from]; !ok || n.State != StateWhite {
			t.Error("from node not created with StateWhite")
		}
	}

	// Edge armazenada com From, To, Call corretos.
	e := g.Edges[0]
	if e.From != from {
		t.Errorf("edge From mismatch: got %+v, want %+v", e.From, from)
	}
	if e.To != to {
		t.Errorf("edge To mismatch: got %+v, want %+v", e.To, to)
	}
	if e.Call.MethodName != "run" {
		t.Errorf("edge Call.MethodName mismatch: got %q, want %q", e.Call.MethodName, "run")
	}
	if e.Call.Receiver != "user" {
		t.Errorf("edge Call.Receiver mismatch: got %q, want %q", e.Call.Receiver, "user")
	}
	if !e.Cycle {
		t.Error("edge Cycle should preserve the value passed to AddEdge")
	}
}

func TestAddEdgeMultigraph(t *testing.T) {
	// Graph é multigrafo: múltiplas arestas entre o mesmo par (A → B com 2
	// chamadas distintas vira 2 Edges).
	g := NewGraph()
	from := resolve.MethodHandle{TypeFQCN: "com.foo.User", Method: "run", Signature: "()"}
	to := resolve.MethodHandle{TypeFQCN: "com.foo.Main", Method: "execute", Signature: "()"}

	g.AddEdge(from, to, java.CallSite{MethodName: "execute", Line: 10}, false)
	g.AddEdge(from, to, java.CallSite{MethodName: "execute", Line: 20}, false)

	if len(g.Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(g.Nodes))
	}
	if len(g.Edges) != 2 {
		t.Errorf("expected 2 edges between same pair, got %d", len(g.Edges))
	}

	if g.Edges[0].Call.Line != 10 {
		t.Errorf("first edge Line: got %d, want 10", g.Edges[0].Call.Line)
	}
	if g.Edges[1].Call.Line != 20 {
		t.Errorf("second edge Line: got %d, want 20", g.Edges[1].Call.Line)
	}
}

func TestMarkTerminalSetsKindNoteAndCandidates(t *testing.T) {
	g := NewGraph()
	h := resolve.MethodHandle{TypeFQCN: "contract.Svc#noimpl#abc", Method: "run", Signature: "()"}
	g.MarkTerminal(h, NodeTerminalNoImplementation, "no impls", []string{"a.A", "b.B"})

	node, ok := g.Nodes[h]
	if !ok {
		t.Fatal("MarkTerminal did not create node")
	}
	if node.Kind != NodeTerminalNoImplementation {
		t.Errorf("kind = %v, want NodeTerminalNoImplementation", node.Kind)
	}
	if node.Note != "no impls" {
		t.Errorf("note = %q", node.Note)
	}
	if len(node.Candidates) != 2 || node.Candidates[0] != "a.A" || node.Candidates[1] != "b.B" {
		t.Errorf("candidates = %+v", node.Candidates)
	}
}

func TestMarkTerminalIsIdempotent(t *testing.T) {
	g := NewGraph()
	h := resolve.MethodHandle{TypeFQCN: "contract.Svc#noimpl#abc", Method: "run", Signature: "()"}
	g.MarkTerminal(h, NodeTerminalNoImplementation, "first", []string{"a.A"})
	g.MarkTerminal(h, NodeTerminalNoImplementation, "second", []string{"b.B"})

	node := g.Nodes[h]
	if node.Note != "second" {
		t.Errorf("note should be overwritten = %q", node.Note)
	}
	if len(node.Candidates) != 1 || node.Candidates[0] != "b.B" {
		t.Errorf("candidates should be overwritten = %+v", node.Candidates)
	}
	if len(g.Nodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(g.Nodes))
	}
}

func TestMarkTerminalCopiesCandidatesDefensively(t *testing.T) {
	g := NewGraph()
	h := resolve.MethodHandle{TypeFQCN: "contract.Svc#ambimpl#abc", Method: "run", Signature: "()"}
	original := []string{"a.A", "b.B"}
	g.MarkTerminal(h, NodeTerminalAmbiguousImplementation, "ambiguous", original)

	original[0] = "MUTATED"
	node := g.Nodes[h]
	if node.Candidates[0] == "MUTATED" {
		t.Fatal("MarkTerminal should copy candidates defensively")
	}
}

func TestMarkExternalDoesNotOverrideTerminal(t *testing.T) {
	g := NewGraph()
	h := resolve.MethodHandle{TypeFQCN: "ext.Lib", Method: "run", Signature: "()"}
	g.MarkTerminal(h, NodeTerminalUnresolved, "unresolved", nil)
	g.MarkExternal(h)
	if g.Nodes[h].Kind != NodeTerminalUnresolved {
		t.Fatal("MarkExternal should not override terminal kind")
	}
}

func TestMarkExternalMarksMethodNode(t *testing.T) {
	g := NewGraph()
	h := resolve.MethodHandle{TypeFQCN: "ext.Lib", Method: "run", Signature: "()"}
	g.MarkExternal(h)
	if g.Nodes[h].Kind != NodeExternal {
		t.Fatalf("kind = %v, want NodeExternal", g.Nodes[h].Kind)
	}
}
