package graph

import (
	"reflect"
	"sort"
	"testing"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
	"github.com/lucas-garcia-rubio/fluxos/internal/resolve"
)

func buildWith(types []*java.TypeDecl, root resolve.ExecutionKey, resolver resolve.Resolver, opts BuildOptions) BuildResult {
	return Build(root, tableForTypes(types), resolver, opts)
}

func keysOf(g *Graph) []resolve.ExecutionKey {
	out := make([]resolve.ExecutionKey, 0, len(g.Nodes))
	for k := range g.Nodes {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		return compareExecutionKeysLocal(out[i], out[j]) < 0
	})
	return out
}

func TestBuildUnlimitedPreservesWalkOutput(t *testing.T) {
	types := []*java.TypeDecl{
		mkType("A", mkMethod("a", mkCall("toB"))),
		mkType("B", mkMethod("b", mkCall("toC"))),
		mkType("C", mkMethod("c")),
	}
	resolver := stubResolver{rules: map[string]resolve.Resolution{
		"toB": resolution(mkHandle("B", "b")),
		"toC": resolution(mkHandle("C", "c")),
	}}

	walkGraph := NewGraph()
	Walk(walkGraph, mkHandle("A", "a"), tableForTypes(types), resolver)

	result := buildWith(types, mkHandle("A", "a"), resolver, BuildOptions{})

	if len(result.Graph.Nodes) != len(walkGraph.Nodes) {
		t.Fatalf("node count: build=%d walk=%d", len(result.Graph.Nodes), len(walkGraph.Nodes))
	}
	if len(result.Graph.Edges) != len(walkGraph.Edges) {
		t.Fatalf("edge count: build=%d walk=%d", len(result.Graph.Edges), len(walkGraph.Edges))
	}
	if len(result.Truncations) != 0 {
		t.Fatalf("unlimited build should not produce truncations: %+v", result.Truncations)
	}
	if !reflect.DeepEqual(keysOf(result.Graph), keysOf(walkGraph)) {
		t.Fatalf("node sets differ:\nbuild=%+v\nwalk=%+v", keysOf(result.Graph), keysOf(walkGraph))
	}
	for _, key := range keysOf(result.Graph) {
		if result.Graph.Nodes[key].State != walkGraph.Nodes[key].State {
			t.Fatalf("state mismatch for %+v: build=%d walk=%d", key, result.Graph.Nodes[key].State, walkGraph.Nodes[key].State)
		}
	}
}

func TestBuildMaxDepthLimitsExpansionAtBoundary(t *testing.T) {
	types := []*java.TypeDecl{
		mkType("A", mkMethod("a", mkCall("toB"))),
		mkType("B", mkMethod("b", mkCall("toC"))),
		mkType("C", mkMethod("c", mkCall("toD"))),
		mkType("D", mkMethod("d")),
	}
	resolver := stubResolver{rules: map[string]resolve.Resolution{
		"toB": resolution(mkHandle("B", "b")),
		"toC": resolution(mkHandle("C", "c")),
		"toD": resolution(mkHandle("D", "d")),
	}}

	result := buildWith(types, mkHandle("A", "a"), resolver, BuildOptions{MaxDepth: 1})

	want := []resolve.ExecutionKey{mkHandle("A", "a"), mkHandle("B", "b")}
	if got := keysOf(result.Graph); !reflect.DeepEqual(got, want) {
		t.Fatalf("nodes = %+v, want %+v", got, want)
	}
	if len(result.Truncations) != 1 {
		t.Fatalf("truncations = %+v, want 1", result.Truncations)
	}
	if result.Truncations[0].Kind != TruncationMaxDepth {
		t.Fatalf("kind = %q, want maxDepth", result.Truncations[0].Kind)
	}
	if result.Truncations[0].Caller != mkHandle("B", "b") {
		t.Fatalf("caller = %+v, want B.b", result.Truncations[0].Caller)
	}
	if result.Truncations[0].Omitted != 1 {
		t.Fatalf("omitted = %d, want 1", result.Truncations[0].Omitted)
	}
}

func TestBuildMaxDepthSuppressesTerminalAndExternalEdgesAtBoundary(t *testing.T) {
	terminal := resolve.TerminalTarget(
		resolve.ResolutionUnresolved,
		"missing.Service",
		"missing",
		"()",
		mkCall("toTerminal"),
		"unresolved",
		nil,
	)
	external := resolve.ResolvedTarget{Key: mkHandle("library.Service", "run"), Kind: resolve.ResolutionExternal}
	types := []*java.TypeDecl{
		mkType("A", mkMethod("a", mkCall("toB"))),
		mkType("B", mkMethod("b", mkCall("toBoundary"))),
	}
	resolver := stubResolver{rules: map[string]resolve.Resolution{
		"toB":        resolution(mkHandle("B", "b")),
		"toBoundary": {Targets: []resolve.ResolvedTarget{terminal, external}},
	}}

	result := buildWith(types, mkHandle("A", "a"), resolver, BuildOptions{MaxDepth: 1})
	if got := keysOf(result.Graph); !reflect.DeepEqual(got, []resolve.ExecutionKey{mkHandle("A", "a"), mkHandle("B", "b")}) {
		t.Fatalf("nodes = %+v, want only A.a and boundary B.b", got)
	}
	if len(result.Graph.Edges) != 1 || result.Graph.Edges[0].From != mkHandle("A", "a") || result.Graph.Edges[0].To != mkHandle("B", "b") {
		t.Fatalf("edges = %+v, want only A.a -> B.b", result.Graph.Edges)
	}
	if len(result.Truncations) != 1 {
		t.Fatalf("truncations = %+v, want one maxDepth entry", result.Truncations)
	}
	truncation := result.Truncations[0]
	if truncation.Kind != TruncationMaxDepth || truncation.Caller != mkHandle("B", "b") || truncation.Omitted != 2 {
		t.Fatalf("truncation = %+v, want B.b with two suppressed targets", truncation)
	}
}

func TestBuildMaxNodesCountsRoot(t *testing.T) {
	types := []*java.TypeDecl{
		mkType("A", mkMethod("a", mkCall("toB"), mkCall("toC"))),
		mkType("B", mkMethod("b")),
		mkType("C", mkMethod("c")),
	}
	resolver := stubResolver{rules: map[string]resolve.Resolution{
		"toB": resolution(mkHandle("B", "b")),
		"toC": resolution(mkHandle("C", "c")),
	}}

	result := buildWith(types, mkHandle("A", "a"), resolver, BuildOptions{MaxNodes: 2})

	if got := keysOf(result.Graph); len(got) != 2 {
		t.Fatalf("nodes = %+v, want 2", got)
	}
	if _, ok := result.Graph.Nodes[mkHandle("A", "a")]; !ok {
		t.Fatalf("root must always be admitted even with MaxNodes=2: %+v", result.Graph.Nodes)
	}
	if len(result.Truncations) != 1 {
		t.Fatalf("truncations = %+v, want 1", result.Truncations)
	}
	if result.Truncations[0].Kind != TruncationMaxNodes {
		t.Fatalf("kind = %q, want maxNodes", result.Truncations[0].Kind)
	}
}

func TestBuildMaxNodesOneAdmitsOnlyRoot(t *testing.T) {
	types := []*java.TypeDecl{
		mkType("A", mkMethod("a",
			java.CallSite{MethodName: "toB", StartByte: 10},
			java.CallSite{MethodName: "toC", StartByte: 20},
		)),
		mkType("B", mkMethod("b")),
		mkType("C", mkMethod("c")),
	}
	resolver := stubResolver{rules: map[string]resolve.Resolution{
		"toB": resolution(mkHandle("B", "b")),
		"toC": resolution(mkHandle("C", "c")),
	}}

	result := buildWith(types, mkHandle("A", "a"), resolver, BuildOptions{MaxNodes: 1})

	if got := keysOf(result.Graph); len(got) != 1 {
		t.Fatalf("nodes = %+v, want only root", got)
	}
	if _, ok := result.Graph.Nodes[mkHandle("A", "a")]; !ok {
		t.Fatalf("root must be present: %+v", result.Graph.Nodes)
	}
	// Truncations não entram em Graph.Nodes.
	if len(result.Truncations) != 2 {
		t.Fatalf("truncations = %+v, want 2 (one per omitted call)", result.Truncations)
	}
	for _, tr := range result.Truncations {
		if tr.Kind != TruncationMaxNodes || tr.Omitted != 1 {
			t.Fatalf("truncation = %+v, want maxNodes/omitted=1", tr)
		}
	}
}

func TestBuildMaxNodesCountsTerminalAndExternalTargets(t *testing.T) {
	terminal := resolve.TerminalTarget(
		resolve.ResolutionUnresolved,
		"missing.Service",
		"missing",
		"()",
		mkCall("toTerminal"),
		"unresolved",
		nil,
	)
	external := resolve.ResolvedTarget{Key: mkHandle("library.Service", "run"), Kind: resolve.ResolutionExternal}
	types := []*java.TypeDecl{
		mkType("A", mkMethod("a", mkCall("toTerminal"), mkCall("toExternal"))),
	}
	resolver := stubResolver{rules: map[string]resolve.Resolution{
		"toTerminal": {Targets: []resolve.ResolvedTarget{terminal}},
		"toExternal": {Targets: []resolve.ResolvedTarget{external}},
	}}

	result := buildWith(types, mkHandle("A", "a"), resolver, BuildOptions{MaxNodes: 2})
	if len(result.Graph.Nodes) != 2 {
		t.Fatalf("nodes = %+v, want root plus one analysis target", result.Graph.Nodes)
	}
	if _, ok := result.Graph.Nodes[terminal.Key]; !ok {
		t.Fatalf("terminal target should consume a MaxNodes slot: %+v", result.Graph.Nodes)
	}
	if _, ok := result.Graph.Nodes[external.Key]; ok {
		t.Fatalf("external target exceeded MaxNodes but was emitted: %+v", result.Graph.Nodes)
	}
	if len(result.Graph.Edges) != 1 || result.Graph.Edges[0].To != terminal.Key {
		t.Fatalf("edges = %+v, want only the admitted terminal target", result.Graph.Edges)
	}
	if len(result.Truncations) != 1 || result.Truncations[0].Kind != TruncationMaxNodes || result.Truncations[0].Omitted != 1 {
		t.Fatalf("truncations = %+v, want one omitted external target", result.Truncations)
	}
}

func TestBuildRevisitsKeyWithShorterDepth(t *testing.T) {
	types := []*java.TypeDecl{
		mkType("Top", mkMethod("top", mkCall("toLeft"), mkCall("toRight"))),
		mkType("Left", mkMethod("left", mkCall("toBottom"))),
		mkType("Right", mkMethod("right", mkCall("toBottom"))),
		mkType("Bottom", mkMethod("bottom")),
	}
	resolver := stubResolver{rules: map[string]resolve.Resolution{
		"toLeft":   resolution(mkHandle("Left", "left")),
		"toRight":  resolution(mkHandle("Right", "right")),
		"toBottom": resolution(mkHandle("Bottom", "bottom")),
	}}

	result := buildWith(types, mkHandle("Top", "top"), resolver, BuildOptions{MaxDepth: 2})

	want := []resolve.ExecutionKey{
		mkHandle("Bottom", "bottom"),
		mkHandle("Left", "left"),
		mkHandle("Right", "right"),
		mkHandle("Top", "top"),
	}
	if got := keysOf(result.Graph); !reflect.DeepEqual(got, want) {
		t.Fatalf("nodes = %+v, want %+v", got, want)
	}
	if len(result.Truncations) != 0 {
		t.Fatalf("truncations = %+v, want none (both paths fit within depth=2)", result.Truncations)
	}
}

func TestBuildDoesNotDuplicateMultiEdges(t *testing.T) {
	types := []*java.TypeDecl{
		mkType("Top", mkMethod("top", mkCall("toLeft"), mkCall("toRight"))),
		mkType("Left", mkMethod("left", mkCall("toBottom"))),
		mkType("Right", mkMethod("right", mkCall("toBottom"))),
		mkType("Bottom", mkMethod("bottom")),
	}
	resolver := stubResolver{rules: map[string]resolve.Resolution{
		"toLeft":   resolution(mkHandle("Left", "left")),
		"toRight":  resolution(mkHandle("Right", "right")),
		"toBottom": resolution(mkHandle("Bottom", "bottom")),
	}}

	result := buildWith(types, mkHandle("Top", "top"), resolver, BuildOptions{})

	wantEdges := 4 // Top->Left, Top->Right, Left->Bottom, Right->Bottom
	if len(result.Graph.Edges) != wantEdges {
		t.Fatalf("edge count = %d, want %d: %+v", len(result.Graph.Edges), wantEdges, result.Graph.Edges)
	}
}

func TestBuildCyclesAreBackEdgesOnEmission(t *testing.T) {
	types := []*java.TypeDecl{
		mkType("A", mkMethod("a", mkCall("toB"))),
		mkType("B", mkMethod("b", mkCall("toA"))),
	}
	resolver := stubResolver{rules: map[string]resolve.Resolution{
		"toB": resolution(mkHandle("B", "b")),
		"toA": resolution(mkHandle("A", "a")),
	}}

	result := buildWith(types, mkHandle("A", "a"), resolver, BuildOptions{})

	if len(result.Graph.Edges) != 2 {
		t.Fatalf("edges = %+v, want 2 (A->B and B->A back-edge)", result.Graph.Edges)
	}
	cycleEdges := 0
	for _, e := range result.Graph.Edges {
		if e.Cycle {
			cycleEdges++
			if e.From != mkHandle("B", "b") || e.To != mkHandle("A", "a") {
				t.Fatalf("wrong back-edge: %+v", e)
			}
		}
	}
	if cycleEdges != 1 {
		t.Fatalf("cycle edges = %d, want 1", cycleEdges)
	}
	if len(result.Truncations) != 0 {
		t.Fatalf("cycles should not produce truncations: %+v", result.Truncations)
	}
}

func TestBuildDiamondDeterministicOrdering(t *testing.T) {
	types := []*java.TypeDecl{
		mkType("Top", mkMethod("top", mkCall("toLeft"), mkCall("toRight"))),
		mkType("Left", mkMethod("left", mkCall("toBottom"))),
		mkType("Right", mkMethod("right", mkCall("toBottom"))),
		mkType("Bottom", mkMethod("bottom")),
	}
	resolver := stubResolver{rules: map[string]resolve.Resolution{
		"toLeft":   resolution(mkHandle("Left", "left")),
		"toRight":  resolution(mkHandle("Right", "right")),
		"toBottom": resolution(mkHandle("Bottom", "bottom")),
	}}

	first := buildWith(types, mkHandle("Top", "top"), resolver, BuildOptions{})
	for i := 0; i < 20; i++ {
		other := buildWith(types, mkHandle("Top", "top"), resolver, BuildOptions{})
		if !reflect.DeepEqual(keysOf(first.Graph), keysOf(other.Graph)) {
			t.Fatalf("iteration %d: nodes differ", i)
		}
		if len(other.Graph.Edges) != len(first.Graph.Edges) {
			t.Fatalf("iteration %d: edge count differ", i)
		}
	}
}

func TestBuildLimitedOutputAndTruncationsAreDeterministic(t *testing.T) {
	terminal := resolve.TerminalTarget(
		resolve.ResolutionUnresolved,
		"missing.Service",
		"missing",
		"()",
		mkCall("toTerminal"),
		"unresolved",
		nil,
	)
	external := resolve.ResolvedTarget{Key: mkHandle("library.Service", "run"), Kind: resolve.ResolutionExternal}
	types := []*java.TypeDecl{
		mkType("A", mkMethod("a", java.CallSite{MethodName: "toB", StartByte: 10}, java.CallSite{MethodName: "toExternal", StartByte: 20})),
		mkType("B", mkMethod("b", java.CallSite{MethodName: "toTerminal", StartByte: 30}, java.CallSite{MethodName: "toExternal", StartByte: 40})),
	}
	resolver := stubResolver{rules: map[string]resolve.Resolution{
		"toB":        resolution(mkHandle("B", "b")),
		"toTerminal": {Targets: []resolve.ResolvedTarget{terminal}},
		"toExternal": {Targets: []resolve.ResolvedTarget{external}},
	}}
	opts := BuildOptions{MaxDepth: 1, MaxNodes: 2}
	first := buildWith(types, mkHandle("A", "a"), resolver, opts)
	for i := 0; i < 20; i++ {
		other := buildWith(types, mkHandle("A", "a"), resolver, opts)
		if !reflect.DeepEqual(first.Graph.Edges, other.Graph.Edges) {
			t.Fatalf("iteration %d: edges differ: first=%+v other=%+v", i, first.Graph.Edges, other.Graph.Edges)
		}
		if !reflect.DeepEqual(first.Truncations, other.Truncations) {
			t.Fatalf("iteration %d: truncations differ: first=%+v other=%+v", i, first.Truncations, other.Truncations)
		}
	}
	if len(first.Truncations) != 3 || first.Truncations[0].Kind != TruncationMaxDepth || first.Truncations[1].Kind != TruncationMaxDepth || first.Truncations[2].Kind != TruncationMaxNodes {
		t.Fatalf("truncations = %+v, want deterministic maxDepth then maxNodes order", first.Truncations)
	}
}

func TestBuildLongChainShortcutAdmittedAtLowerDepth(t *testing.T) {
	// Mixed: Top.go -> Long.step1 -> Long.step2 -> Long.step3 -> Long.target
	//        Top.go -> Shortcut.jump -> Long.target (shorter path, depth 2)
	types := []*java.TypeDecl{
		mkType("Top", mkMethod("go", mkCall("step1"), mkCall("jump"))),
		mkType("Long",
			mkMethod("step1", mkCall("step2")),
			mkMethod("step2", mkCall("step3")),
			mkMethod("step3", mkCall("targetHit")),
			mkMethod("targetHit"),
		),
		mkType("Shortcut", mkMethod("jump", mkCall("targetHit"))),
	}
	resolver := stubResolver{rules: map[string]resolve.Resolution{
		"step1":     resolution(mkHandle("Long", "step1")),
		"step2":     resolution(mkHandle("Long", "step2")),
		"step3":     resolution(mkHandle("Long", "step3")),
		"targetHit": resolution(mkHandle("Long", "targetHit")),
		"jump":      resolution(mkHandle("Shortcut", "jump")),
	}}

	result := buildWith(types, mkHandle("Top", "go"), resolver, BuildOptions{MaxDepth: 3})

	// targetHit deve ser admitido (depth 2 via Shortcut), não truncado.
	if _, ok := result.Graph.Nodes[mkHandle("Long", "targetHit")]; !ok {
		t.Fatalf("target must be admitted via shortcut: %+v", keysOf(result.Graph))
	}
	for _, tr := range result.Truncations {
		if tr.Caller == mkHandle("Long", "targetHit") {
			t.Fatalf("target should not be a truncation caller: %+v", tr)
		}
	}
}

func TestBuildShorterReplanControlsMaxDepthAndOutput(t *testing.T) {
	types := []*java.TypeDecl{
		// The deep route is listed first in source order. The shorter route must
		// still determine the target's effective depth before its stale item is
		// processed.
		mkType("Top", mkMethod("go", mkCall("deepEntry"), mkCall("shortEntry"))),
		mkType("Deep",
			mkMethod("entry", mkCall("deepStep")),
			mkMethod("step", mkCall("hit")),
		),
		mkType("Short", mkMethod("entry", mkCall("hit"))),
		mkType("Target", mkMethod("hit", mkCall("leaf")), mkMethod("leaf", mkCall("beyond"))),
		mkType("Beyond", mkMethod("end")),
	}
	resolver := stubResolver{rules: map[string]resolve.Resolution{
		"deepEntry":  resolution(mkHandle("Deep", "entry")),
		"shortEntry": resolution(mkHandle("Short", "entry")),
		"deepStep":   resolution(mkHandle("Deep", "step")),
		"hit":        resolution(mkHandle("Target", "hit")),
		"leaf":       resolution(mkHandle("Target", "leaf")),
		"beyond":     resolution(mkHandle("Beyond", "end")),
	}}
	opts := BuildOptions{MaxDepth: 3}
	first := buildWith(types, mkHandle("Top", "go"), resolver, opts)

	target := mkHandle("Target", "hit")
	leaf := mkHandle("Target", "leaf")
	if _, ok := first.Graph.Nodes[target]; !ok {
		t.Fatalf("target must be admitted: %+v", keysOf(first.Graph))
	}
	if _, ok := first.Graph.Nodes[leaf]; !ok {
		t.Fatalf("target must expand through the shorter route: %+v", keysOf(first.Graph))
	}
	if len(first.Graph.Nodes) != 6 {
		t.Fatalf("nodes = %+v, want six unique admitted nodes", keysOf(first.Graph))
	}
	if len(first.Graph.Edges) != 6 {
		t.Fatalf("edges = %+v, want six unique topology edges", first.Graph.Edges)
	}
	targetLeafEdges := 0
	for _, edge := range first.Graph.Edges {
		if edge.From == target && edge.To == leaf {
			targetLeafEdges++
		}
	}
	if targetLeafEdges != 1 {
		t.Fatalf("target -> leaf edges = %d, want 1: %+v", targetLeafEdges, first.Graph.Edges)
	}
	if len(first.Truncations) != 1 || first.Truncations[0].Kind != TruncationMaxDepth || first.Truncations[0].Caller != leaf {
		t.Fatalf("truncations = %+v, want one maxDepth entry for Target.leaf", first.Truncations)
	}

	for i := 0; i < 20; i++ {
		other := buildWith(types, mkHandle("Top", "go"), resolver, opts)
		if !reflect.DeepEqual(first.Graph.Edges, other.Graph.Edges) {
			t.Fatalf("iteration %d: edges differ: first=%+v other=%+v", i, first.Graph.Edges, other.Graph.Edges)
		}
		if !reflect.DeepEqual(keysOf(first.Graph), keysOf(other.Graph)) {
			t.Fatalf("iteration %d: nodes differ", i)
		}
		if !reflect.DeepEqual(first.Truncations, other.Truncations) {
			t.Fatalf("iteration %d: truncations differ: first=%+v other=%+v", i, first.Truncations, other.Truncations)
		}
	}
}

func TestBuildTruncationsDeduplicatedPerCallSite(t *testing.T) {
	types := []*java.TypeDecl{
		mkType("Caller", mkMethod("run", java.CallSite{MethodName: "polymorphic", StartByte: 100})),
		mkType("A", mkMethod("a")),
		mkType("B", mkMethod("b")),
		mkType("C", mkMethod("c")),
	}
	resolver := stubResolver{rules: map[string]resolve.Resolution{
		"polymorphic": resolution(mkHandle("A", "a"), mkHandle("B", "b"), mkHandle("C", "c")),
	}}

	result := buildWith(types, mkHandle("Caller", "run"), resolver, BuildOptions{MaxNodes: 2})

	// 1 Truncation maxNodes para a call "polymorphic" do Caller.run.
	if len(result.Truncations) != 1 {
		t.Fatalf("truncations = %+v, want 1 (deduped per call site)", result.Truncations)
	}
	if result.Truncations[0].Omitted != 2 {
		t.Fatalf("omitted = %d, want 2 (3 concrete targets - 1 admitted slot)", result.Truncations[0].Omitted)
	}
}

func TestBuildTruncationsDoNotInflateMaxNodes(t *testing.T) {
	types := []*java.TypeDecl{
		mkType("Caller", mkMethod("run", java.CallSite{MethodName: "a", StartByte: 100}, java.CallSite{MethodName: "b", StartByte: 200})),
		mkType("A", mkMethod("a")),
		mkType("B", mkMethod("b")),
	}
	resolver := stubResolver{rules: map[string]resolve.Resolution{
		"a": resolution(mkHandle("A", "a")),
		"b": resolution(mkHandle("B", "b")),
	}}

	result := buildWith(types, mkHandle("Caller", "run"), resolver, BuildOptions{MaxNodes: 1})

	// Apenas root admitido; 2 truncations (uma por call site).
	if len(result.Graph.Nodes) != 1 {
		t.Fatalf("nodes = %+v, want only root (truncations do not inflate MaxNodes)", result.Graph.Nodes)
	}
	if len(result.Truncations) != 2 {
		t.Fatalf("truncations = %+v, want 2 (one per call site)", result.Truncations)
	}
}

func TestWalkRemainsUnlimitedForInternalCallers(t *testing.T) {
	types := []*java.TypeDecl{
		mkType("A", mkMethod("a", mkCall("toB"))),
		mkType("B", mkMethod("b", mkCall("toC"))),
		mkType("C", mkMethod("c")),
	}
	resolver := stubResolver{rules: map[string]resolve.Resolution{
		"toB": resolution(mkHandle("B", "b")),
		"toC": resolution(mkHandle("C", "c")),
	}}
	g := NewGraph()
	Walk(g, mkHandle("A", "a"), tableForTypes(types), resolver)
	if len(g.Nodes) != 3 {
		t.Fatalf("Walk should be unlimited: got %d nodes, want 3", len(g.Nodes))
	}
}

func TestTruncationIDIsStableAndRootIndependent(t *testing.T) {
	t1 := Truncation{
		Kind: TruncationMaxNodes,
		Caller: resolve.ExecutionKey{
			Method:          resolve.MethodHandle{TypeFQCN: "app.Workflow", Method: "start", Signature: "()"},
			RuntimeTypeFQCN: "app.Workflow",
		},
		Call:    java.CallSite{MethodName: "run", File: "App.java", StartByte: 100},
		Omitted: 1,
	}
	t2 := t1
	t2.Omitted = 99
	if t1.ID() != t2.ID() {
		t.Fatalf("ID must ignore Omitted: %q vs %q", t1.ID(), t2.ID())
	}
	// Independent of project root spelling: ID depende apenas de Caller/Call.
	if t1.ID() != "t_"+t1.ID()[2:] {
		// sanity: prefix t_
		if t1.ID()[:2] != "t_" {
			t.Fatalf("ID must start with t_: %q", t1.ID())
		}
	}
}

func TestTruncationIDIncludesKindCallerAndStartByte(t *testing.T) {
	base := Truncation{
		Kind: TruncationMaxNodes,
		Caller: resolve.ExecutionKey{
			Method:          resolve.MethodHandle{TypeFQCN: "app.Workflow", Method: "start", Signature: "()"},
			RuntimeTypeFQCN: "app.Workflow",
		},
		Call: java.CallSite{MethodName: "run", File: "App.java", StartByte: 100},
	}
	variants := []Truncation{
		{Kind: TruncationMaxDepth, Caller: base.Caller, Call: base.Call},
		{Kind: base.Kind, Caller: resolve.ExecutionKey{
			Method:          resolve.MethodHandle{TypeFQCN: "other.Workflow", Method: "start", Signature: "()"},
			RuntimeTypeFQCN: "app.Workflow",
		}, Call: base.Call},
		{Kind: base.Kind, Caller: base.Caller, Call: java.CallSite{MethodName: "run", File: "App.java", StartByte: 200}},
		{Kind: base.Kind, Caller: base.Caller, Call: java.CallSite{MethodName: "run", File: "Other.java", StartByte: 100}},
	}
	for i, v := range variants {
		if v.ID() == base.ID() {
			t.Fatalf("variant %d produced same ID as base: %q", i, base.ID())
		}
	}
}

func TestTruncationIDIgnoresOmittedAndNote(t *testing.T) {
	base := Truncation{
		Kind: TruncationMaxNodes,
		Caller: resolve.ExecutionKey{
			Method:          resolve.MethodHandle{TypeFQCN: "app.Workflow", Method: "start", Signature: "()"},
			RuntimeTypeFQCN: "app.Workflow",
		},
		Call: java.CallSite{MethodName: "run", File: "App.java", StartByte: 100},
	}
	derivatives := []Truncation{
		{Kind: base.Kind, Caller: base.Caller, Call: base.Call, Omitted: 100},
		{Kind: base.Kind, Caller: base.Caller, Call: base.Call, Note: "different note"},
		{Kind: base.Kind, Caller: base.Caller, Call: base.Call, Omitted: 50, Note: "another"},
	}
	for i, d := range derivatives {
		if d.ID() != base.ID() {
			t.Fatalf("derivative %d should have same ID as base: base=%q deriv=%q", i, base.ID(), d.ID())
		}
	}
}

func TestTruncationsSortedDeterministically(t *testing.T) {
	truncations := []Truncation{
		{Kind: TruncationMaxNodes, Caller: mkHandle("Z", "z"), Call: java.CallSite{StartByte: 50}},
		{Kind: TruncationMaxDepth, Caller: mkHandle("A", "a"), Call: java.CallSite{StartByte: 10}},
		{Kind: TruncationMaxNodes, Caller: mkHandle("A", "a"), Call: java.CallSite{StartByte: 10}},
		{Kind: TruncationMaxDepth, Caller: mkHandle("A", "a"), Call: java.CallSite{StartByte: 5}},
	}
	sort.Slice(truncations, func(i, j int) bool {
		return compareTruncations(truncations[i], truncations[j]) < 0
	})
	expectedOrder := []string{
		string(TruncationMaxDepth), string(TruncationMaxDepth),
		string(TruncationMaxNodes), string(TruncationMaxNodes),
	}
	got := make([]string, len(truncations))
	for i, tr := range truncations {
		got[i] = string(tr.Kind)
	}
	if !reflect.DeepEqual(got, expectedOrder) {
		t.Fatalf("order = %+v, want %+v", got, expectedOrder)
	}
	// maxDepth com StartByte=5 antes de StartByte=10
	if truncations[0].Call.StartByte != 5 || truncations[1].Call.StartByte != 10 {
		t.Fatalf("maxDepth tie-break by StartByte wrong: %+v", truncations)
	}
}
