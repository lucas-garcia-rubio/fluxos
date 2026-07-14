package graph

import (
	"testing"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
	"github.com/lucas-garcia-rubio/fluxos/internal/resolve"
)

// stubResolver maps call.MethodName → Resolution. Útil pra testes porque
// deixa explícito qual call produz qual target.
type stubResolver struct {
	rules map[string]resolve.Resolution
}

func (s stubResolver) Resolve(call java.CallSite, ctx resolve.MethodContext) resolve.Resolution {
	if r, ok := s.rules[call.MethodName]; ok {
		return r
	}
	return resolve.Resolution{Note: "no rule for " + call.MethodName}
}

// Helpers pra construir fixtures in-memory (sem precisar de .java real).
func mkType(fqcn string, methods ...java.MethodDecl) *java.TypeDecl {
	return &java.TypeDecl{
		Kind:    java.TypeKindClass,
		Name:    fqcn,
		FQCN:    fqcn,
		Methods: methods,
	}
}

func mkMethod(name string, calls ...java.CallSite) java.MethodDecl {
	return java.MethodDecl{Name: name, Calls: calls}
}

func mkCall(methodName string) java.CallSite {
	return java.CallSite{MethodName: methodName}
}

func mkHandle(fqcn, method string) resolve.MethodHandle {
	return resolve.MethodHandle{TypeFQCN: fqcn, Method: method}
}

func resolution(targets ...resolve.MethodHandle) resolve.Resolution {
	return resolve.Resolution{Targets: targets}
}

// Cenário 1: caminho linear A.a → B.b → C.c.
// Espera: 3 nodes (todos black), 2 edges (A→B, B→C).
func TestWalkLinear(t *testing.T) {
	types := []*java.TypeDecl{
		mkType("A", mkMethod("a", mkCall("toB"))),
		mkType("B", mkMethod("b", mkCall("toC"))),
		mkType("C", mkMethod("c")),
	}
	resolver := stubResolver{
		rules: map[string]resolve.Resolution{
			"toB": resolution(mkHandle("B", "b")),
			"toC": resolution(mkHandle("C", "c")),
		},
	}

	g := NewGraph()
	Walk(g, types[0], types[0].Methods[0], types, resolver)

	if len(g.Nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(g.Nodes))
	}
	if len(g.Edges) != 2 {
		t.Errorf("expected 2 edges, got %d", len(g.Edges))
	}
	for h, n := range g.Nodes {
		if n.State != StateBlack {
			t.Errorf("node %v should be black, got state %d", h, n.State)
		}
	}
}

// Cenário 2: ciclo A.a → B.b → A.a (back-edge).
// Espera: 2 nodes (todos black), 2 edges (A→B, B→A).
// Walk não entra em recursão infinita — quando visita A de novo, A está gray.
func TestWalkCycle(t *testing.T) {
	types := []*java.TypeDecl{
		mkType("A", mkMethod("a", mkCall("toB"))),
		mkType("B", mkMethod("b", mkCall("toA"))),
	}
	resolver := stubResolver{
		rules: map[string]resolve.Resolution{
			"toB": resolution(mkHandle("B", "b")),
			"toA": resolution(mkHandle("A", "a")),
		},
	}

	g := NewGraph()
	Walk(g, types[0], types[0].Methods[0], types, resolver)

	if len(g.Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(g.Nodes))
	}
	if len(g.Edges) != 2 {
		t.Errorf("expected 2 edges (A→B and B→A), got %d", len(g.Edges))
	}
	for h, n := range g.Nodes {
		if n.State != StateBlack {
			t.Errorf("node %v should be black after cycle, got state %d", h, n.State)
		}
	}
}

// Cenário 3: self-loop A.a → A.a.
// Espera: 1 node, 1 edge (A→A).
func TestWalkSelfLoop(t *testing.T) {
	types := []*java.TypeDecl{
		mkType("A", mkMethod("a", mkCall("toSelf"))),
	}
	resolver := stubResolver{
		rules: map[string]resolve.Resolution{
			"toSelf": resolution(mkHandle("A", "a")),
		},
	}

	g := NewGraph()
	Walk(g, types[0], types[0].Methods[0], types, resolver)

	if len(g.Nodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(g.Nodes))
	}
	if len(g.Edges) != 1 {
		t.Errorf("expected 1 edge (A→A), got %d", len(g.Edges))
	}
	if !g.IsBlack(mkHandle("A", "a")) {
		t.Error("A should be black after self-loop walk")
	}
}

// Cenário 4: call unresolved (resolver devolve 0 targets).
// Espera: 1 node (A black), 0 edges.
func TestWalkUnresolved(t *testing.T) {
	types := []*java.TypeDecl{
		mkType("A", mkMethod("a", mkCall("external"))),
	}
	resolver := stubResolver{
		rules: map[string]resolve.Resolution{
			"external": {Note: "external lib"}, // Targets vazio
		},
	}

	g := NewGraph()
	Walk(g, types[0], types[0].Methods[0], types, resolver)

	if len(g.Nodes) != 1 {
		t.Errorf("expected 1 node (just A), got %d", len(g.Nodes))
	}
	if len(g.Edges) != 0 {
		t.Errorf("expected 0 edges (no targets resolved), got %d", len(g.Edges))
	}
	if !g.IsBlack(mkHandle("A", "a")) {
		t.Error("A should be black")
	}
}

// Cenário 5: fan-out (uma call resolve pra 2 targets — polimorfismo).
// Espera: 3 nodes (A, X, Y), 2 edges (A→X, A→Y), todos black.
func TestWalkFanOut(t *testing.T) {
	types := []*java.TypeDecl{
		mkType("A", mkMethod("a", mkCall("polymorphic"))),
		mkType("X", mkMethod("x")),
		mkType("Y", mkMethod("y")),
	}
	resolver := stubResolver{
		rules: map[string]resolve.Resolution{
			"polymorphic": resolution(mkHandle("X", "x"), mkHandle("Y", "y")),
		},
	}

	g := NewGraph()
	Walk(g, types[0], types[0].Methods[0], types, resolver)

	if len(g.Nodes) != 3 {
		t.Errorf("expected 3 nodes (A, X, Y), got %d", len(g.Nodes))
	}
	if len(g.Edges) != 2 {
		t.Errorf("expected 2 edges (A→X, A→Y), got %d", len(g.Edges))
	}
	// Verifica arestas específicas.
	edgeToX, edgeToY := false, false
	for _, e := range g.Edges {
		if e.To == mkHandle("X", "x") {
			edgeToX = true
		}
		if e.To == mkHandle("Y", "y") {
			edgeToY = true
		}
	}
	if !edgeToX {
		t.Error("missing edge A→X")
	}
	if !edgeToY {
		t.Error("missing edge A→Y")
	}
}

// Cenário extra: external target (target existe na Resolution mas não no index).
// Aresta é adicionada mas Walk não recursa — folha externa.
func TestWalkExternalTarget(t *testing.T) {
	types := []*java.TypeDecl{
		mkType("A", mkMethod("a", mkCall("external"))),
		// sem tipo "External" no index
	}
	resolver := stubResolver{
		rules: map[string]resolve.Resolution{
			"external": resolution(mkHandle("External", "doStuff")),
		},
	}

	g := NewGraph()
	Walk(g, types[0], types[0].Methods[0], types, resolver)

	// 2 nodes: A (do projeto) e External (criado por AddEdge mas não recursado).
	if len(g.Nodes) != 2 {
		t.Errorf("expected 2 nodes (A e External), got %d", len(g.Nodes))
	}
	if len(g.Edges) != 1 {
		t.Errorf("expected 1 edge, got %d", len(g.Edges))
	}
	// A black; External white (nunca Walk-ado).
	if !g.IsBlack(mkHandle("A", "a")) {
		t.Error("A should be black")
	}
	if g.IsBlack(mkHandle("External", "doStuff")) || g.IsGray(mkHandle("External", "doStuff")) {
		t.Error("External target should remain white (never walked)")
	}
}
