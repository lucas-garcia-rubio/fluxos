package graph

import (
	"testing"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
	"github.com/lucas-garcia-rubio/fluxos/internal/index"
	"github.com/lucas-garcia-rubio/fluxos/internal/resolve"
)

// stubResolver maps call.MethodName → Resolution. Útil pra testes porque
// deixa explícito qual call produz qual target.
type stubResolver struct {
	rules map[string]resolve.Resolution
}

type contextResolver struct {
	localVars []java.LocalVarDecl
	params    []java.Param
}

func (r *contextResolver) Resolve(_ java.CallSite, ctx resolve.MethodContext) resolve.Resolution {
	r.localVars = ctx.LocalVars
	r.params = ctx.Params
	return resolve.Resolution{Note: "captured context"}
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
	return java.MethodDecl{Name: name, Signature: "()", Calls: calls}
}

func mkCall(methodName string) java.CallSite {
	return java.CallSite{MethodName: methodName}
}

func mkHandle(fqcn, method string) resolve.MethodHandle {
	return resolve.MethodHandle{TypeFQCN: fqcn, Method: method, Signature: "()"}
}

func resolution(targets ...resolve.MethodHandle) resolve.Resolution {
	resolved := make([]resolve.ResolvedTarget, len(targets))
	for i, h := range targets {
		resolved[i] = resolve.ConcreteTarget(h)
	}
	return resolve.Resolution{Targets: resolved}
}

func tableForTypes(types []*java.TypeDecl) *index.Table {
	unitsByFile := make(map[string]*java.CompilationUnit)
	for _, typ := range types {
		unit := unitsByFile[typ.File]
		if unit == nil {
			unit = &java.CompilationUnit{File: typ.File, Types: make([]*java.TypeDecl, 0)}
			unitsByFile[typ.File] = unit
		}
		unit.Types = append(unit.Types, typ)
	}
	units := make([]*java.CompilationUnit, 0, len(unitsByFile))
	for _, unit := range unitsByFile {
		units = append(units, unit)
	}
	table, err := index.Build(units)
	if err != nil {
		panic(err)
	}
	return table
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
	Walk(g, types[0], types[0].Methods[0], tableForTypes(types), resolver)

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
	for _, edge := range g.Edges {
		if edge.Cycle {
			t.Errorf("linear edge %+v should not be marked as cycle", edge)
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
	Walk(g, types[0], types[0].Methods[0], tableForTypes(types), resolver)

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
	cycleEdges := 0
	for _, edge := range g.Edges {
		if edge.Cycle {
			cycleEdges++
			if edge.From != mkHandle("B", "b") || edge.To != mkHandle("A", "a") {
				t.Errorf("wrong edge marked as cycle: %+v", edge)
			}
		}
	}
	if cycleEdges != 1 {
		t.Errorf("expected exactly 1 cycle edge, got %d", cycleEdges)
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
	Walk(g, types[0], types[0].Methods[0], tableForTypes(types), resolver)

	if len(g.Nodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(g.Nodes))
	}
	if len(g.Edges) != 1 {
		t.Errorf("expected 1 edge (A→A), got %d", len(g.Edges))
	}
	if !g.IsBlack(mkHandle("A", "a")) {
		t.Error("A should be black after self-loop walk")
	}
	if !g.Edges[0].Cycle {
		t.Error("self-loop edge should be marked as cycle")
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
	Walk(g, types[0], types[0].Methods[0], tableForTypes(types), resolver)

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
	Walk(g, types[0], types[0].Methods[0], tableForTypes(types), resolver)

	if len(g.Nodes) != 3 {
		t.Errorf("expected 3 nodes (A, X, Y), got %d", len(g.Nodes))
	}
	if len(g.Edges) != 2 {
		t.Errorf("expected 2 edges (A→X, A→Y), got %d", len(g.Edges))
	}
	// Verifica arestas específicas.
	edgeToX, edgeToY := false, false
	for _, e := range g.Edges {
		if e.Cycle {
			t.Errorf("fan-out edge %+v should not be marked as cycle", e)
		}
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
	Walk(g, types[0], types[0].Methods[0], tableForTypes(types), resolver)

	// 2 nodes: A (do projeto) e External (criado por AddEdge mas não recursado).
	if len(g.Nodes) != 2 {
		t.Errorf("expected 2 nodes (A e External), got %d", len(g.Nodes))
	}
	if len(g.Edges) != 1 {
		t.Errorf("expected 1 edge, got %d", len(g.Edges))
	}
	if g.Edges[0].Cycle {
		t.Error("external target edge should not be marked as cycle")
	}
	// A black; External white (nunca Walk-ado).
	if !g.IsBlack(mkHandle("A", "a")) {
		t.Error("A should be black")
	}
	if g.IsBlack(mkHandle("External", "doStuff")) || g.IsGray(mkHandle("External", "doStuff")) {
		t.Error("External target should remain white (never walked)")
	}
}

func TestWalkPassesLocalVarsToResolver(t *testing.T) {
	method := mkMethod("run", mkCall("helper"))
	method.LocalVars = []java.LocalVarDecl{{Name: "helper", Type: java.NewTypeRef("Helper", false)}}
	typ := mkType("Example", method)
	resolver := &contextResolver{}

	Walk(NewGraph(), typ, method, tableForTypes([]*java.TypeDecl{typ}), resolver)

	if len(resolver.localVars) != 1 || resolver.localVars[0].Name != "helper" || resolver.localVars[0].Type.Raw != "Helper" {
		t.Fatalf("resolver received local vars = %+v, want [{Name: helper, Type.Raw: Helper}]", resolver.localVars)
	}
}

func TestWalkPassesParamsToResolver(t *testing.T) {
	method := mkMethod("run", mkCall("helper"))
	method.Params = []java.Param{{Name: "helper", Type: java.NewTypeRef("Helper", false)}}
	typ := mkType("Example", method)
	resolver := &contextResolver{}

	Walk(NewGraph(), typ, method, tableForTypes([]*java.TypeDecl{typ}), resolver)

	if len(resolver.params) != 1 || resolver.params[0].Name != "helper" || resolver.params[0].Type.Raw != "Helper" {
		t.Fatalf("resolver received params = %+v, want [{Name: helper, Type.Raw: Helper}]", resolver.params)
	}
}

func TestWalkKeepsOverloadsDistinctAndDetectsCycle(t *testing.T) {
	zero := java.MethodDecl{
		Name:      "run",
		Signature: "()",
		Calls:     []java.CallSite{{MethodName: "toInt"}},
	}
	withInt := java.MethodDecl{
		Name:      "run",
		Signature: "(int)",
		Params:    []java.Param{{Type: java.NewTypeRef("int", false)}},
		Calls:     []java.CallSite{{MethodName: "toZero"}},
	}
	typ := mkType("Service", zero, withInt)
	resolver := stubResolver{rules: map[string]resolve.Resolution{
		"toInt":  resolution(resolve.MethodHandle{TypeFQCN: "Service", Method: "run", Signature: "(int)"}),
		"toZero": resolution(resolve.MethodHandle{TypeFQCN: "Service", Method: "run", Signature: "()"}),
	}}

	g := NewGraph()
	Walk(g, typ, zero, tableForTypes([]*java.TypeDecl{typ}), resolver)

	if len(g.Nodes) != 2 {
		t.Fatalf("node count = %d, want 2 overload nodes", len(g.Nodes))
	}
	if len(g.Edges) != 2 {
		t.Fatalf("edge count = %d, want 2", len(g.Edges))
	}
	if !g.Edges[1].Cycle {
		t.Fatalf("back edge between overloads was not marked: %+v", g.Edges)
	}
}

func TestWalkInheritedMethodUsesDeclaringTypeAndTraversesBody(t *testing.T) {
	parentOnly := mkMethod("parentOnly")
	inherited := mkMethod("inherited", mkCall("parentOnly"))
	inherited.Modifier = []string{"public"}
	parent := mkType("Parent", inherited, parentOnly)
	parent.File = "Parent.java"
	entry := mkMethod("entry", mkCall("inherited"))
	child := mkType("Child", entry)
	child.File = "Child.java"
	child.SuperClass = java.NewTypeRef("Parent", false)
	table := tableForTypes([]*java.TypeDecl{child, parent})

	g := NewGraph()
	Walk(g, child, entry, table, resolve.NewSyntacticResolver(table))

	for _, handle := range []resolve.MethodHandle{
		{TypeFQCN: "Child", Method: "entry", Signature: "()"},
		{TypeFQCN: "Parent", Method: "inherited", Signature: "()"},
		{TypeFQCN: "Parent", Method: "parentOnly", Signature: "()"},
	} {
		if !g.IsBlack(handle) {
			t.Fatalf("expected inherited traversal node %+v to be black; nodes=%+v", handle, g.Nodes)
		}
	}
	if len(g.Edges) != 2 {
		t.Fatalf("edge count = %d, want 2; edges=%+v", len(g.Edges), g.Edges)
	}
}

func TestWalkObjectCreationTraversesConstructorBody(t *testing.T) {
	targetType := java.NewTypeRef("Value", false)
	run := mkMethod("run", java.CallSite{
		Kind: java.CallObjectCreation, MethodName: "<init>", TargetType: &targetType,
	})
	caller := mkType("Caller", run)
	caller.File = "Caller.java"
	validate := mkMethod("validate")
	constructor := java.MethodDecl{
		Kind: java.MethodConstructor, Name: "<init>", Signature: "()", Modifier: []string{"public"},
		Calls: []java.CallSite{{MethodName: "validate"}},
	}
	value := mkType("Value", constructor, validate)
	value.File = "Value.java"
	table := tableForTypes([]*java.TypeDecl{caller, value})

	g := NewGraph()
	Walk(g, caller, run, table, resolve.NewSyntacticResolver(table))

	constructorHandle := resolve.MethodHandle{TypeFQCN: "Value", Method: "<init>", Signature: "()"}
	validateHandle := resolve.MethodHandle{TypeFQCN: "Value", Method: "validate", Signature: "()"}
	if !g.IsBlack(constructorHandle) || !g.IsBlack(validateHandle) {
		t.Fatalf("constructor body was not traversed; nodes=%+v", g.Nodes)
	}
	if len(g.Edges) != 2 || g.Edges[0].Call.Kind != java.CallObjectCreation {
		t.Fatalf("constructor edges = %+v", g.Edges)
	}
}

func TestWalkMethodReferencePreservesKindAndTraversesBody(t *testing.T) {
	entry := mkMethod("entry", java.CallSite{
		Kind: java.CallMethodReference, Receiver: "Target", MethodName: "referenced",
		ReferenceQualifier: java.ReferenceQualifierName,
	})
	caller := mkType("Caller", entry)
	caller.File = "Caller.java"
	leaf := mkMethod("leaf")
	referenced := mkMethod("referenced", mkCall("leaf"))
	target := mkType("Target", referenced, leaf)
	target.File = "Target.java"
	table := tableForTypes([]*java.TypeDecl{caller, target})

	g := NewGraph()
	Walk(g, caller, entry, table, resolve.NewSyntacticResolver(table))

	referencedHandle := resolve.MethodHandle{TypeFQCN: "Target", Method: "referenced", Signature: "()"}
	leafHandle := resolve.MethodHandle{TypeFQCN: "Target", Method: "leaf", Signature: "()"}
	if !g.IsBlack(referencedHandle) || !g.IsBlack(leafHandle) {
		t.Fatalf("referenced body was not traversed; nodes=%+v", g.Nodes)
	}
	if len(g.Edges) != 2 || g.Edges[0].Call.Kind != java.CallMethodReference {
		t.Fatalf("method reference edges = %+v", g.Edges)
	}
}

func TestWalkConstructorReferencePreservesKind(t *testing.T) {
	targetType := java.NewTypeRef("Value", false)
	entry := mkMethod("entry", java.CallSite{
		Kind: java.CallConstructorReference, MethodName: "<init>", TargetType: &targetType,
	})
	caller := mkType("Caller", entry)
	constructor := java.MethodDecl{Kind: java.MethodConstructor, Name: "<init>", Signature: "()", Modifier: []string{"public"}}
	value := mkType("Value", constructor)
	table := tableForTypes([]*java.TypeDecl{caller, value})

	g := NewGraph()
	Walk(g, caller, entry, table, resolve.NewSyntacticResolver(table))
	if len(g.Edges) != 1 || g.Edges[0].Call.Kind != java.CallConstructorReference {
		t.Fatalf("constructor reference edges = %+v", g.Edges)
	}
}
