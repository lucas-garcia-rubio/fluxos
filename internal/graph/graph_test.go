package graph

import (
	"testing"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
	"github.com/lucas-garcia-rubio/fluxos/internal/resolve"
)

func graphTestKey(typeFQCN, method, signature string) resolve.ExecutionKey {
	return resolve.ExecutionKey{
		Method:          resolve.MethodHandle{TypeFQCN: typeFQCN, Method: method, Signature: signature},
		RuntimeTypeFQCN: typeFQCN,
	}
}

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
	h := graphTestKey("com.foo.User", "getName", "()")

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

func TestGetOrCreateDistinguishesRuntimeContexts(t *testing.T) {
	handle := resolve.MethodHandle{TypeFQCN: "base.Base", Method: "run", Signature: "()"}
	first := resolve.ExecutionKey{Method: handle, RuntimeTypeFQCN: "app.First"}
	second := resolve.ExecutionKey{Method: handle, RuntimeTypeFQCN: "app.Second"}
	g := NewGraph()

	if g.GetOrCreate(first) == g.GetOrCreate(second) || len(g.Nodes) != 2 {
		t.Fatalf("runtime contexts collapsed: %+v", g.Nodes)
	}
	g.MarkGray(first)
	if !g.IsGray(first) || g.IsGray(second) {
		t.Fatalf("DFS state was shared across runtime contexts: %+v", g.Nodes)
	}
}

func TestIsGrayBlackOnMissingHandle(t *testing.T) {
	g := NewGraph()
	h := graphTestKey("com.foo.User", "getName", "()")

	if g.IsGray(h) {
		t.Error("IsGray should be false for non-existent handle")
	}
	if g.IsBlack(h) {
		t.Error("IsBlack should be false for non-existent handle")
	}
}

func TestMarkGrayAndIsGray(t *testing.T) {
	g := NewGraph()
	h := graphTestKey("com.foo.User", "getName", "()")

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
	h := graphTestKey("com.foo.User", "getName", "()")

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
	h := graphTestKey("com.foo.User", "getName", "()")

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
	from := graphTestKey("com.foo.User", "getName", "()")
	to := graphTestKey("com.foo.Main", "run", "()")
	call := java.CallSite{MethodName: "run", Receiver: "user"}

	g.AddEdge(from, to, call, nil, true)

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
	from := graphTestKey("com.foo.User", "run", "()")
	to := graphTestKey("com.foo.Main", "execute", "()")

	g.AddEdge(from, to, java.CallSite{MethodName: "execute", Line: 10}, nil, false)
	g.AddEdge(from, to, java.CallSite{MethodName: "execute", Line: 20}, nil, false)

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

func TestAddEdgeCopiesCallAndDispatchSite(t *testing.T) {
	from := graphTestKey("Caller", "run", "()")
	to := graphTestKey("Target", "work", "()")
	targetType := java.NewTypeRef("Target", false)
	call := java.CallSite{Args: []string{"original"}, TargetType: &targetType}
	site := resolve.NewDispatchSite(from, "Contract", "work", "()", call, []resolve.ImplementationCandidate{{
		ImplementationFQCN: "Impl", Target: to.Method, Kind: resolve.ResolutionConcrete,
	}})
	g := NewGraph()
	g.AddEdge(from, to, call, site, false)

	call.Args[0] = "changed"
	call.TargetType.Raw = "Changed"
	site.Call.Args[0] = "site-changed"
	site.Candidates[0].ImplementationFQCN = "ChangedImpl"
	edge := g.Edges[0]
	if edge.Call.Args[0] != "original" || edge.Call.TargetType.Raw != "Target" {
		t.Fatalf("edge call aliases input: %+v", edge.Call)
	}
	if edge.DispatchSite == site || edge.DispatchSite.Call.Args[0] != "original" || edge.DispatchSite.Candidates[0].ImplementationFQCN != "Impl" {
		t.Fatalf("edge dispatch site aliases input: %+v", edge.DispatchSite)
	}
}

func TestMarkTerminalSetsKindNoteAndCandidates(t *testing.T) {
	g := NewGraph()
	h := graphTestKey("contract.Svc#noimpl#abc", "run", "()")
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
	h := graphTestKey("contract.Svc#noimpl#abc", "run", "()")
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
	h := graphTestKey("contract.Svc#ambimpl#abc", "run", "()")
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
	h := graphTestKey("ext.Lib", "run", "()")
	g.MarkTerminal(h, NodeTerminalUnresolved, "unresolved", nil)
	g.MarkExternal(h)
	if g.Nodes[h].Kind != NodeTerminalUnresolved {
		t.Fatal("MarkExternal should not override terminal kind")
	}
}

func TestMarkExternalMarksMethodNode(t *testing.T) {
	g := NewGraph()
	h := graphTestKey("ext.Lib", "run", "()")
	g.MarkExternal(h)
	if g.Nodes[h].Kind != NodeExternal {
		t.Fatalf("kind = %v, want NodeExternal", g.Nodes[h].Kind)
	}
}
