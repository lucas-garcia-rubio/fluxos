package render

import (
	"reflect"
	"testing"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
	"github.com/lucas-garcia-rubio/fluxos/internal/graph"
	"github.com/lucas-garcia-rubio/fluxos/internal/resolve"
)

func testHandle(typeFQCN, method, signature string) resolve.ExecutionKey {
	return resolve.ExecutionKey{
		Method:          resolve.MethodHandle{TypeFQCN: typeFQCN, Method: method, Signature: signature},
		RuntimeTypeFQCN: typeFQCN,
	}
}

func findNode(t *testing.T, snapshot Snapshot, method MethodView) NodeView {
	t.Helper()
	for _, node := range snapshot.Nodes {
		if node.Execution.Method == method {
			return node
		}
	}
	t.Fatalf("node %+v not found in %+v", method, snapshot.Nodes)
	return NodeView{}
}

func TestNewSnapshotPreservesLegacyMethodIDs(t *testing.T) {
	g := graph.NewGraph()
	worker := testHandle("com.example.Worker", "run", "()")
	overload := testHandle("Service", "run", "(String)")
	g.GetOrCreate(worker)
	g.GetOrCreate(overload)

	snapshot := NewSnapshot(g, worker)
	if got := findNode(t, snapshot, methodView(worker.Method, false)).ID; got != "m_7c57aeb2fd25" {
		t.Fatalf("worker ID = %q, want m_7c57aeb2fd25", got)
	}
	if got := findNode(t, snapshot, methodView(overload.Method, false)).ID; got != "m_54c4cb581b54" {
		t.Fatalf("overload ID = %q, want m_54c4cb581b54", got)
	}
}

func TestNewSnapshotQualifiesCoexistingRuntimeContexts(t *testing.T) {
	handle := resolve.MethodHandle{TypeFQCN: "base.Base", Method: "run", Signature: "()"}
	first := resolve.ExecutionKey{Method: handle, RuntimeTypeFQCN: "app.First"}
	second := resolve.ExecutionKey{Method: handle, RuntimeTypeFQCN: "app.Second"}
	g := graph.NewGraph()
	g.GetOrCreate(second)
	g.GetOrCreate(first)

	snapshot := NewSnapshot(g, first)
	if len(snapshot.Nodes) != 2 {
		t.Fatalf("nodes = %+v", snapshot.Nodes)
	}
	if snapshot.Nodes[0].Execution != executionView(first, false) || snapshot.Nodes[1].Execution != executionView(second, false) {
		t.Fatalf("runtime contexts not sorted deterministically: %+v", snapshot.Nodes)
	}
	if snapshot.Nodes[0].ID != stableExecutionID(first, true) || snapshot.Nodes[1].ID != stableExecutionID(second, true) || snapshot.Nodes[0].ID == snapshot.Nodes[1].ID {
		t.Fatalf("runtime-aware IDs = %+v", snapshot.Nodes)
	}
	if snapshot.Nodes[0].Label != "base.Base.run() [runtime: app.First]" || snapshot.Nodes[1].Label != "base.Base.run() [runtime: app.Second]" {
		t.Fatalf("runtime-aware labels = %+v", snapshot.Nodes)
	}
}

func TestNewSnapshotSingleInheritedContextKeepsLegacyPresentation(t *testing.T) {
	handle := resolve.MethodHandle{TypeFQCN: "base.Base", Method: "run", Signature: "()"}
	key := resolve.ExecutionKey{Method: handle, RuntimeTypeFQCN: "app.First"}
	g := graph.NewGraph()
	g.GetOrCreate(key)

	node := NewSnapshot(g, key).Nodes[0]
	if node.ID != stableNodeID(handle) || node.Label != "base.Base.run()" {
		t.Fatalf("single context changed legacy presentation: %+v", node)
	}
	if node.Execution.RuntimeTypeFQCN != "app.First" {
		t.Fatalf("runtime metadata lost: %+v", node.Execution)
	}
}

func TestNewSnapshotSanitizesTerminalMethodsAfterHashing(t *testing.T) {
	handle := testHandle("contract.Svc#noimpl#abc123", "run", "")
	g := graph.NewGraph()
	g.MarkTerminal(handle, graph.NodeTerminalNoImplementation, "none", nil)

	node := NewSnapshot(g, handle).Nodes[0]
	if node.ID != "m_6fd272c9775e" {
		t.Fatalf("terminal ID = %q, want legacy raw-handle ID", node.ID)
	}
	wantMethod := MethodView{TypeFQCN: "contract.Svc", Method: "run"}
	if node.Execution.Method != wantMethod {
		t.Fatalf("terminal method = %+v, want %+v", node.Execution.Method, wantMethod)
	}
	if node.Label != "contract.Svc.run() [no implementation]" {
		t.Fatalf("terminal label = %q", node.Label)
	}
}

func TestNewSnapshotKeepsTerminalKindsAndCallSitesDistinct(t *testing.T) {
	call1 := java.CallSite{File: "App.java", Line: 10, StartByte: 100}
	call2 := java.CallSite{File: "App.java", Line: 20, StartByte: 200}
	handles := []resolve.ExecutionKey{
		{Method: resolve.TerminalHandle("contract.Svc", "run", "", resolve.ResolutionNoImplementation, call1), RuntimeTypeFQCN: "Caller"},
		{Method: resolve.TerminalHandle("contract.Svc", "run", "", resolve.ResolutionUnresolved, call1), RuntimeTypeFQCN: "Caller"},
		{Method: resolve.TerminalHandle("contract.Svc", "run", "", resolve.ResolutionNoImplementation, call2), RuntimeTypeFQCN: "Caller"},
	}
	g := graph.NewGraph()
	g.MarkTerminal(handles[0], graph.NodeTerminalNoImplementation, "none", nil)
	g.MarkTerminal(handles[1], graph.NodeTerminalUnresolved, "unresolved", nil)
	g.MarkTerminal(handles[2], graph.NodeTerminalNoImplementation, "none", nil)

	snapshot := NewSnapshot(g, handles[0])
	ids := make(map[string]struct{}, len(snapshot.Nodes))
	for _, node := range snapshot.Nodes {
		ids[node.ID] = struct{}{}
		if node.Execution.Method.TypeFQCN != "contract.Svc" {
			t.Fatalf("terminal method leaked synthetic suffix: %+v", node.Execution.Method)
		}
	}
	if len(ids) != 3 {
		t.Fatalf("terminal IDs are not distinct: %+v", snapshot.Nodes)
	}
}

func TestNewSnapshotMapsNodeKindsAndLabels(t *testing.T) {
	tests := []struct {
		name       string
		graphKind  graph.NodeKind
		wantKind   NodeKind
		candidates []string
		wantLabel  string
	}{
		{name: "method", graphKind: graph.NodeMethod, wantKind: NodeMethod, wantLabel: "Type.run()"},
		{name: "external", graphKind: graph.NodeExternal, wantKind: NodeExternal, wantLabel: "Type.run()"},
		{name: "unresolved", graphKind: graph.NodeTerminalUnresolved, wantKind: NodeUnresolved, wantLabel: "Type.run() [unresolved]"},
		{name: "no implementation", graphKind: graph.NodeTerminalNoImplementation, wantKind: NodeNoImplementation, wantLabel: "Type.run() [no implementation]"},
		{name: "ambiguous type", graphKind: graph.NodeTerminalAmbiguousType, wantKind: NodeAmbiguousType, wantLabel: "Type.run() [ambiguous type]"},
		{name: "ambiguous overload", graphKind: graph.NodeTerminalAmbiguousOverload, wantKind: NodeAmbiguousOverload, wantLabel: "Type.run() [ambiguous overload]"},
		{name: "ambiguous implementation", graphKind: graph.NodeTerminalAmbiguousImplementation, wantKind: NodeAmbiguousImplementation, candidates: []string{"b.B", "a.A"}, wantLabel: "Type.run() [ambiguous: 2 implementations]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handle := testHandle("Type", "run", "()")
			g := graph.NewGraph()
			switch tt.graphKind {
			case graph.NodeMethod:
				g.GetOrCreate(handle)
			case graph.NodeExternal:
				g.MarkExternal(handle)
			default:
				g.MarkTerminal(handle, tt.graphKind, "note", tt.candidates)
			}
			node := NewSnapshot(g, handle).Nodes[0]
			if node.Kind != tt.wantKind || node.Label != tt.wantLabel {
				t.Fatalf("node = %+v, want kind=%q label=%q", node, tt.wantKind, tt.wantLabel)
			}
			if node.Candidates == nil {
				t.Fatal("candidates must be non-nil")
			}
		})
	}
}

func TestNewSnapshotSortsAndCopiesCandidates(t *testing.T) {
	handle := testHandle("Type#ambimpl#abc123", "run", "")
	candidates := []string{"z.Z", "a.A", "m.M"}
	g := graph.NewGraph()
	g.MarkTerminal(handle, graph.NodeTerminalAmbiguousImplementation, "note", candidates)

	node := NewSnapshot(g, handle).Nodes[0]
	want := []string{"a.A", "m.M", "z.Z"}
	if !reflect.DeepEqual(node.Candidates, want) {
		t.Fatalf("candidates = %v, want %v", node.Candidates, want)
	}
	candidates[0] = "changed"
	g.Nodes[handle].Candidates[1] = "changed-again"
	if !reflect.DeepEqual(node.Candidates, want) {
		t.Fatalf("snapshot candidates changed through graph alias: %v", node.Candidates)
	}
}

func TestNewSnapshotIsIndependentOfNodeInsertionOrder(t *testing.T) {
	a := testHandle("A", "run", "()")
	b := testHandle("B", "run", "()")
	first := graph.NewGraph()
	first.GetOrCreate(b)
	first.GetOrCreate(a)
	second := graph.NewGraph()
	second.GetOrCreate(a)
	second.GetOrCreate(b)

	firstSnapshot := NewSnapshot(first, a)
	secondSnapshot := NewSnapshot(second, a)
	if !reflect.DeepEqual(firstSnapshot, secondSnapshot) {
		t.Fatalf("insertion order changed snapshot:\nfirst=%+v\nsecond=%+v", firstSnapshot, secondSnapshot)
	}
}

func TestNewSnapshotOrdersEdgeEndpoints(t *testing.T) {
	a := testHandle("A", "run", "()")
	b := testHandle("B", "run", "()")
	c := testHandle("C", "run", "()")
	z := testHandle("Z", "run", "()")
	g := graph.NewGraph()
	g.AddEdge(a, c, java.CallSite{}, nil, false)
	g.AddEdge(z, a, java.CallSite{}, nil, false)
	g.AddEdge(a, b, java.CallSite{}, nil, false)
	g.AddEdge(a, a, java.CallSite{}, nil, false)

	edges := NewSnapshot(g, a).Edges
	want := [][2]string{
		{stableNodeID(a.Method), stableNodeID(a.Method)},
		{stableNodeID(a.Method), stableNodeID(b.Method)},
		{stableNodeID(a.Method), stableNodeID(c.Method)},
		{stableNodeID(z.Method), stableNodeID(a.Method)},
	}
	for i, edge := range edges {
		if edge.From != want[i][0] || edge.To != want[i][1] {
			t.Fatalf("edge %d endpoints = %s -> %s, want %s -> %s", i, edge.From, edge.To, want[i][0], want[i][1])
		}
	}
}

func TestNewSnapshotOrdersEveryCallSiteTieBreaker(t *testing.T) {
	from := testHandle("A", "run", "()")
	to := testHandle("B", "run", "()")
	g := graph.NewGraph()
	edges := []graph.Edge{
		{From: from, To: to, Call: java.CallSite{File: "z", Line: 1}},
		{From: from, To: to, Call: java.CallSite{File: "a", Line: 2}},
		{From: from, To: to, Call: java.CallSite{File: "a", Line: 1, StartByte: 2}},
		{From: from, To: to, Call: java.CallSite{File: "a", Line: 1, StartByte: 1, Kind: java.CallMethodReference}},
		{From: from, To: to, Call: java.CallSite{File: "a", Line: 1, StartByte: 1, Receiver: "z"}},
		{From: from, To: to, Call: java.CallSite{File: "a", Line: 1, StartByte: 1, Receiver: "a", MethodName: "z"}},
		{From: from, To: to, Call: java.CallSite{File: "a", Line: 1, StartByte: 1, Receiver: "a", MethodName: "a"}, Cycle: true},
		{From: from, To: to, Call: java.CallSite{File: "a", Line: 1, StartByte: 1, Receiver: "a", MethodName: "a"}},
	}
	for _, edge := range edges {
		g.AddEdge(edge.From, edge.To, edge.Call, edge.DispatchSite, edge.Cycle)
	}

	got := NewSnapshot(g, from).Edges
	if len(got) != len(edges) {
		t.Fatalf("edge count = %d, want %d", len(got), len(edges))
	}
	want := []struct {
		file     string
		line     int
		start    uint
		kind     string
		receiver string
		method   string
		cycle    bool
	}{
		{"a", 1, 1, "invocation", "a", "a", false},
		{"a", 1, 1, "invocation", "a", "a", true},
		{"a", 1, 1, "invocation", "a", "z", false},
		{"a", 1, 1, "invocation", "z", "", false},
		{"a", 1, 1, "methodReference", "", "", false},
		{"a", 1, 2, "invocation", "", "", false},
		{"a", 2, 0, "invocation", "", "", false},
		{"z", 1, 0, "invocation", "", "", false},
	}
	for i, edge := range got {
		entry := want[i]
		if edge.Call.File != entry.file || edge.Call.Line != entry.line || edge.Call.StartByte != entry.start || edge.Call.Kind != entry.kind || edge.Call.Receiver != entry.receiver || edge.Call.MethodName != entry.method || edge.Cycle != entry.cycle {
			t.Fatalf("edge %d = %+v, want %+v", i, edge, entry)
		}
	}
}

func TestNewSnapshotPreservesMultiedgesAndCycles(t *testing.T) {
	a := testHandle("A", "run", "()")
	b := testHandle("B", "run", "()")
	g := graph.NewGraph()
	g.AddEdge(a, b, java.CallSite{Line: 10}, nil, false)
	g.AddEdge(a, b, java.CallSite{Line: 20}, nil, false)
	g.AddEdge(b, a, java.CallSite{Line: 30}, nil, true)

	edges := NewSnapshot(g, a).Edges
	if len(edges) != 3 {
		t.Fatalf("edge count = %d, want 3", len(edges))
	}
	cycles := 0
	for _, edge := range edges {
		if edge.Cycle {
			cycles++
			if edge.From != stableNodeID(b.Method) || edge.To != stableNodeID(a.Method) {
				t.Fatalf("wrong cycle edge: %+v", edge)
			}
		}
	}
	if cycles != 1 {
		t.Fatalf("cycle count = %d, want 1", cycles)
	}
}

func TestNewSnapshotDoesNotAliasGraph(t *testing.T) {
	a := testHandle("A", "run", "()")
	b := testHandle("B", "run", "()")
	g := graph.NewGraph()
	g.AddEdge(a, b, java.CallSite{File: "A.java", Line: 10}, nil, false)
	g.Nodes[a].Note = "original"
	g.Nodes[a].Candidates = []string{"one"}
	snapshot := NewSnapshot(g, a)

	g.Nodes[a].State = graph.StateGray
	g.Nodes[a].Note = "changed"
	g.Nodes[a].Candidates[0] = "changed"
	g.Edges[0].Call.Line = 99
	delete(g.Nodes, b)
	g.GetOrCreate(testHandle("C", "run", "()"))
	if len(snapshot.Nodes) != 2 || len(snapshot.Edges) != 1 || snapshot.Edges[0].Call.Line != 10 {
		t.Fatalf("snapshot structure changed after graph mutation: %+v", snapshot)
	}
	node := findNode(t, snapshot, methodView(a.Method, false))
	if node.Note != "original" || !reflect.DeepEqual(node.Candidates, []string{"one"}) {
		t.Fatalf("snapshot node changed after graph mutation: %+v", node)
	}
}

func TestNewSnapshotCopiesAndSortsDispatchSite(t *testing.T) {
	from := testHandle("Caller", "run", "()")
	to := testHandle("Contract#ambimpl#site", "work", "()")
	call := java.CallSite{File: "Caller.java", Line: 10, Args: []string{"arg"}}
	site := resolve.NewDispatchSite(from, "Contract", "work", "()", call, []resolve.ImplementationCandidate{
		{ImplementationFQCN: "z.Impl", Target: resolve.MethodHandle{TypeFQCN: "Base", Method: "work", Signature: "()"}, Kind: resolve.ResolutionConcrete},
		{ImplementationFQCN: "a.Impl", Kind: resolve.ResolutionNoImplementation, Note: "missing"},
	})
	g := graph.NewGraph()
	g.AddEdge(from, to, call, site, false)

	snapshot := NewSnapshot(g, from)
	view := snapshot.Edges[0].DispatchSite
	if view == nil || view.ID != string(site.ID) || len(view.Candidates) != 2 || view.Candidates[0].ImplementationFQCN != "a.Impl" || view.Candidates[1].ImplementationFQCN != "z.Impl" {
		t.Fatalf("dispatch view = %+v", view)
	}
	if view.Candidates[0].Kind != "noImplementation" || view.Candidates[1].Kind != "concrete" {
		t.Fatalf("candidate kinds = %+v", view.Candidates)
	}
	g.Edges[0].DispatchSite.Call.Args[0] = "changed"
	g.Edges[0].DispatchSite.Candidates[0].ImplementationFQCN = "changed"
	g.Edges[0].DispatchSite = nil
	if view.Call.Kind != call.Kind.String() || view.Candidates[0].ImplementationFQCN != "a.Impl" {
		t.Fatalf("dispatch view aliases graph: %+v", view)
	}
}

func TestNewSnapshotNilAndEmptyUseNonNilSlices(t *testing.T) {
	target := testHandle("Target", "run", "()")
	for _, snapshot := range []Snapshot{NewSnapshot(nil, target), NewSnapshot(graph.NewGraph(), target)} {
		if snapshot.Target != executionView(target, false) || snapshot.Nodes == nil || snapshot.Edges == nil {
			t.Fatalf("snapshot = %+v", snapshot)
		}
	}
}

func TestNewSnapshotKeepsRawLabelsFormatNeutral(t *testing.T) {
	handle := testHandle(`com.example."Worker"`, "run", "()")
	g := graph.NewGraph()
	g.GetOrCreate(handle)
	if got := NewSnapshot(g, handle).Nodes[0].Label; got != `com.example."Worker".run()` {
		t.Fatalf("raw label = %q", got)
	}
}
