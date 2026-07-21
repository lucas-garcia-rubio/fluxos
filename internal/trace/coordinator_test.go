package trace

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
	"github.com/lucas-garcia-rubio/fluxos/internal/graph"
	"github.com/lucas-garcia-rubio/fluxos/internal/index"
	"github.com/lucas-garcia-rubio/fluxos/internal/parse"
	"github.com/lucas-garcia-rubio/fluxos/internal/project"
	"github.com/lucas-garcia-rubio/fluxos/internal/resolve"
)

type recordingSelector struct {
	selections func([]resolve.DispatchSite) []Selection
	calls      [][]resolve.DispatchSite
	err        error
	errors     []error
}

func (s *recordingSelector) Select(sites []resolve.DispatchSite) ([]Selection, error) {
	s.calls = append(s.calls, sites)
	if call := len(s.calls) - 1; call < len(s.errors) && s.errors[call] != nil {
		return nil, s.errors[call]
	}
	if s.err != nil {
		return nil, s.err
	}
	return s.selections(sites), nil
}

type resultFactory struct {
	results []graph.BuildResult
	calls   []resolve.DispatchPolicy
}

func (f *resultFactory) Build(_ resolve.ExecutionKey, _ *index.Table, policy resolve.DispatchPolicy, _ graph.BuildOptions) graph.BuildResult {
	f.calls = append(f.calls, policy)
	result := f.results[0]
	f.results = f.results[1:]
	return result
}

func coordinatorKey(name string) resolve.ExecutionKey {
	return resolve.ExecutionKey{
		Method:          resolve.MethodHandle{TypeFQCN: name, Method: "run", Signature: "()"},
		RuntimeTypeFQCN: name,
	}
}

func coordinatorSite(id, caller, receiver, file string, byteOffset uint, candidates ...string) *resolve.DispatchSite {
	items := make([]resolve.ImplementationCandidate, 0, len(candidates))
	for _, fqcn := range candidates {
		items = append(items, resolve.ImplementationCandidate{
			ImplementationFQCN: fqcn,
			Kind:               resolve.ResolutionConcrete,
			Target:             coordinatorKey(fqcn).Method,
		})
	}
	site := resolve.NewDispatchSite(
		coordinatorKey(caller), receiver, "call", "()",
		java.CallSite{Kind: java.CallInvocation, MethodName: "call", File: file, StartByte: byteOffset},
		items,
	)
	site.ID = resolve.DispatchSiteID(id)
	return site
}

func coordinatorResult(sites ...*resolve.DispatchSite) graph.BuildResult {
	g := graph.NewGraph()
	from := coordinatorKey("Root")
	to := coordinatorKey("Target")
	for _, site := range sites {
		g.AddEdge(from, to, site.Call, site, false)
	}
	return graph.BuildResult{Graph: g}
}

func emptyCoordinatorResult() graph.BuildResult {
	return graph.BuildResult{Graph: graph.NewGraph()}
}

func cycleCoordinatorResult(site *resolve.DispatchSite) graph.BuildResult {
	g := graph.NewGraph()
	key := coordinatorKey("Root")
	g.AddEdge(key, key, site.Call, site, true)
	return graph.BuildResult{Graph: g}
}

func selectAll(sites []resolve.DispatchSite) []Selection {
	result := make([]Selection, 0, len(sites))
	for _, site := range sites {
		result = append(result, Selection{SiteID: site.ID, Choice: resolve.DispatchChoice{Mode: resolve.ChoiceAll}})
	}
	return result
}

func selectFirstCandidates(sites []resolve.DispatchSite) []Selection {
	result := make([]Selection, 0, len(sites))
	for _, site := range sites {
		result = append(result, Selection{
			SiteID: site.ID,
			Choice: resolve.DispatchChoice{Mode: resolve.ChoiceSelected, ImplementationFQCN: site.Candidates[0].ImplementationFQCN},
		})
	}
	return result
}

func interactiveTable(t *testing.T) *index.Table {
	t.Helper()
	root := filepath.Join("..", "..", "testdata", "m4", "interactive")
	discovered, err := project.DiscoverWithOptions(root, project.DiscoverOptions{Scope: project.ScopeModeMain})
	if err != nil {
		t.Fatalf("discover interactive fixture: %v", err)
	}
	units := make([]*java.CompilationUnit, 0, len(discovered.Files))
	for _, file := range discovered.Files {
		tree, source, err := parse.Parse(file.Path)
		if err != nil {
			t.Fatalf("parse %s: %v", file.Path, err)
		}
		logicalFile, err := filepath.Rel(discovered.Root, file.Path)
		if err != nil {
			logicalFile = file.Path
		}
		unit, err := java.ExtractUnit(filepath.ToSlash(logicalFile), source, tree)
		tree.Close()
		if err != nil {
			t.Fatalf("extract %s: %v", file.Path, err)
		}
		unit.SourceRoot = file.SourceRoot
		units = append(units, unit)
	}
	table, err := index.Build(units)
	if err != nil {
		t.Fatalf("index interactive fixture: %v", err)
	}
	return table
}

func interactiveRoot() resolve.ExecutionKey {
	return resolve.ExecutionKey{
		Method:          resolve.MethodHandle{TypeFQCN: "app.Workflow", Method: "start", Signature: "()"},
		RuntimeTypeFQCN: "app.Workflow",
	}
}

type interactiveSelector struct {
	chooseNone bool
	calls      [][]resolve.DispatchSite
}

func (s *interactiveSelector) Select(sites []resolve.DispatchSite) ([]Selection, error) {
	s.calls = append(s.calls, append([]resolve.DispatchSite(nil), sites...))
	if len(s.calls) == 1 {
		if len(sites) != 1 || sites[0].ReceiverFQCN != "contracts.A" {
			return nil, fmt.Errorf("first frontier = %#v, want contracts.A", sites)
		}
		if s.chooseNone {
			return []Selection{{SiteID: sites[0].ID, Choice: resolve.DispatchChoice{Mode: resolve.ChoiceNone}}}, nil
		}
		return []Selection{{
			SiteID: sites[0].ID,
			Choice: resolve.DispatchChoice{Mode: resolve.ChoiceSelected, ImplementationFQCN: "app.AlphaA"},
		}}, nil
	}
	if len(s.calls) == 2 {
		if len(sites) != 1 || sites[0].ReceiverFQCN != "contracts.B" {
			return nil, fmt.Errorf("second frontier = %#v, want contracts.B", sites)
		}
		return []Selection{{
			SiteID: sites[0].ID,
			Choice: resolve.DispatchChoice{Mode: resolve.ChoiceSelected, ImplementationFQCN: "app.GammaB"},
		}}, nil
	}
	return nil, fmt.Errorf("unexpected selector call %d", len(s.calls))
}

func fixtureMethodKey(typeFQCN, method string) resolve.ExecutionKey {
	return resolve.ExecutionKey{
		Method:          resolve.MethodHandle{TypeFQCN: typeFQCN, Method: method, Signature: "()"},
		RuntimeTypeFQCN: typeFQCN,
	}
}

func graphHasNode(result graph.BuildResult, key resolve.ExecutionKey) bool {
	if result.Graph == nil {
		return false
	}
	_, ok := result.Graph.Nodes[key]
	return ok
}

func TestCoordinatorNilSelectorBuildsExactlyOnce(t *testing.T) {
	factory := &resultFactory{results: []graph.BuildResult{coordinatorResult()}}
	base := resolve.AllPolicy{MaxImpls: 9}
	got, err := (Coordinator{}).Build(Request{
		Root:         coordinatorKey("Root"),
		Table:        nil,
		BuildOptions: graph.BuildOptions{MaxDepth: 3, MaxNodes: 4},
		MaxImpls:     2,
		BasePolicy:   base,
		BuildFactory: factory.Build,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(factory.calls) != 1 || got.Graph == nil {
		t.Fatalf("build count/result = %d/%+v, want one result", len(factory.calls), got)
	}
	if !reflect.DeepEqual(factory.calls[0], base) {
		t.Fatalf("fast path policy = %#v, want %#v", factory.calls[0], base)
	}
}

func TestCoordinatorSelectorUsesTerminalFallbackRegardlessOfBasePolicy(t *testing.T) {
	site := coordinatorSite("site", "Caller", "Contract", "A.java", 1, "impl.A", "impl.B")
	factory := &resultFactory{results: []graph.BuildResult{
		coordinatorResult(site),
		coordinatorResult(site),
		coordinatorResult(),
	}}
	selector := &recordingSelector{selections: selectAll}
	if _, err := (Coordinator{}).Build(Request{
		BasePolicy:   resolve.AllPolicy{MaxImpls: 1},
		Selector:     selector,
		BuildFactory: factory.Build,
	}); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(factory.calls) != 3 {
		t.Fatalf("build count = %d, want discovery, discovery, final", len(factory.calls))
	}
	for i, policy := range factory.calls {
		sitePolicy, ok := policy.(resolve.SitePolicy)
		if !ok {
			t.Fatalf("build %d policy = %#v, want SitePolicy", i, policy)
		}
		if _, ok := sitePolicy.Fallback.(resolve.TerminalPolicy); !ok {
			t.Fatalf("build %d fallback = %#v, want TerminalPolicy", i, sitePolicy.Fallback)
		}
		if i < 2 && len(sitePolicy.Choices) != i {
			t.Fatalf("build %d choices = %#v, want %d selected entries", i, sitePolicy.Choices, i)
		}
	}
	discovery := factory.calls[0].(resolve.SitePolicy)
	if decision := discovery.Apply(site); len(decision.Targets) != 1 || decision.Targets[0].Kind != resolve.ResolutionAmbiguousImplementation {
		t.Fatalf("unknown site discovery decision = %+v, want terminal fallback", decision)
	}
	final := factory.calls[2].(resolve.SitePolicy)
	if final.Choices[site.ID].Mode != resolve.ChoiceAll {
		t.Fatalf("chosen site policy = %#v, want all choice", final.Choices)
	}
	if decision := final.Apply(site); len(decision.Targets) != 2 {
		t.Fatalf("chosen site final decision = %+v, want both candidates", decision)
	}
}

func TestCoordinatorProductionFixtureSelectsSuccessiveRuntimePaths(t *testing.T) {
	table := interactiveTable(t)
	selector := &interactiveSelector{}
	result, err := (Coordinator{}).Build(Request{
		Root:       interactiveRoot(),
		Table:      table,
		BasePolicy: resolve.TerminalPolicy{},
		MaxImpls:   2,
		Selector:   selector,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(selector.calls) != 2 || selector.calls[0][0].ReceiverFQCN != "contracts.A" || selector.calls[1][0].ReceiverFQCN != "contracts.B" {
		t.Fatalf("successive frontiers = %#v, want A then B", selector.calls)
	}
	for _, key := range []resolve.ExecutionKey{
		interactiveRoot(),
		fixtureMethodKey("app.AlphaA", "run"),
		fixtureMethodKey("app.GammaB", "work"),
	} {
		if !graphHasNode(result, key) {
			t.Fatalf("final graph is missing selected runtime path node %+v", key)
		}
	}
	for _, key := range []resolve.ExecutionKey{
		fixtureMethodKey("app.BetaA", "run"),
		fixtureMethodKey("app.DeltaB", "work"),
	} {
		if graphHasNode(result, key) {
			t.Fatalf("final graph contains unselected runtime path node %+v", key)
		}
	}
	if result.Graph == nil || len(result.Graph.Edges) != 2 {
		t.Fatalf("final graph edges = %d, want exactly selected A and B paths", len(result.Graph.Edges))
	}
}

func TestCoordinatorProductionFixtureNoneDoesNotOpenNestedPath(t *testing.T) {
	table := interactiveTable(t)
	selector := &interactiveSelector{chooseNone: true}
	result, err := (Coordinator{}).Build(Request{
		Root:       interactiveRoot(),
		Table:      table,
		BasePolicy: resolve.TerminalPolicy{},
		MaxImpls:   2,
		Selector:   selector,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(selector.calls) != 1 || selector.calls[0][0].ReceiverFQCN != "contracts.A" {
		t.Fatalf("selector calls = %#v, want only contracts.A", selector.calls)
	}
	if result.Graph == nil {
		t.Fatal("final graph is nil")
	}
	for _, edge := range result.Graph.Edges {
		if edge.DispatchSite != nil && edge.DispatchSite.ReceiverFQCN == "contracts.B" {
			t.Fatal("none choice opened nested contracts.B path")
		}
	}
	for _, key := range []resolve.ExecutionKey{
		fixtureMethodKey("app.AlphaA", "run"),
		fixtureMethodKey("app.BetaA", "run"),
		fixtureMethodKey("app.GammaB", "work"),
		fixtureMethodKey("app.DeltaB", "work"),
	} {
		if graphHasNode(result, key) {
			t.Fatalf("none choice reached nested implementation node %+v", key)
		}
	}
}

func TestCoordinatorProductionFixtureLimitsPreventLaterFrontier(t *testing.T) {
	table := interactiveTable(t)
	selector := &interactiveSelector{}
	result, err := (Coordinator{}).Build(Request{
		Root:         interactiveRoot(),
		Table:        table,
		BuildOptions: graph.BuildOptions{MaxDepth: 3, MaxNodes: 2},
		MaxImpls:     1,
		BasePolicy:   resolve.TerminalPolicy{},
		Selector:     selector,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(selector.calls) != 1 || selector.calls[0][0].ReceiverFQCN != "contracts.A" {
		t.Fatalf("bounded selector calls = %#v, want only first frontier", selector.calls)
	}
	if !graphHasNode(result, fixtureMethodKey("app.AlphaA", "run")) {
		t.Fatal("bounded final graph lost the selected first implementation")
	}
	for _, edge := range result.Graph.Edges {
		if edge.DispatchSite != nil && edge.DispatchSite.ReceiverFQCN == "contracts.B" {
			t.Fatal("MaxNodes bound allowed contracts.B into a later frontier")
		}
	}
}

func TestCoordinatorBuildsFreshRoundsAndFinalOnly(t *testing.T) {
	a := coordinatorSite("a", "CallerA", "Contract", "/project/src/main/java/a/A.java", 20, "impl.A")
	b := coordinatorSite("b", "CallerB", "Contract", "/project/src/main/java/b/B.java", 10, "impl.B")
	final := coordinatorResult()
	final.Truncations = []graph.Truncation{{Kind: graph.TruncationMaxNodes, Omitted: 7}}
	factory := &resultFactory{results: []graph.BuildResult{
		coordinatorResult(a),
		coordinatorResult(a, b),
		emptyCoordinatorResult(),
		final,
	}}
	selector := &recordingSelector{selections: selectFirstCandidates}
	got, err := (Coordinator{}).Build(Request{
		Root:         coordinatorKey("Root"),
		Selector:     selector,
		MaxImpls:     5,
		BuildOptions: graph.BuildOptions{MaxDepth: 2, MaxNodes: 8},
		BuildFactory: factory.Build,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(factory.calls) != 4 || len(selector.calls) != 2 {
		t.Fatalf("build/selector calls = %d/%d, want 4/2", len(factory.calls), len(selector.calls))
	}
	if len(selector.calls[0]) != 1 || selector.calls[0][0].ID != "a" || len(selector.calls[1]) != 1 || selector.calls[1][0].ID != "b" {
		t.Fatalf("successive frontiers = %#v, %#v", selector.calls[0], selector.calls[1])
	}
	last, ok := factory.calls[3].(resolve.SitePolicy)
	if !ok || len(last.Choices) != 2 || last.MaxImpls != 5 {
		t.Fatalf("final policy = %#v, want two choices and MaxImpls=5", factory.calls[3])
	}
	if !reflect.DeepEqual(got.Truncations, final.Truncations) || got.Graph != final.Graph {
		t.Fatalf("returned discovery data: got graph=%p truncations=%+v, want graph=%p truncations=%+v", got.Graph, got.Truncations, final.Graph, final.Truncations)
	}
}

func TestCoordinatorCanonicalFrontierDeduplicatesAndDetaches(t *testing.T) {
	a := coordinatorSite("a", "Caller", "Contract", "/project/src/main/java/z/Z.java", 30, "impl.Z", "impl.A")
	// Reversing the candidate slice must not turn an identical site into a
	// conflicting duplicate.
	other := resolve.CloneDispatchSite(a)
	other.Candidates[0], other.Candidates[1] = other.Candidates[1], other.Candidates[0]
	b := coordinatorSite("b", "Caller", "Contract", "/project/src/main/java/a/A.java", 10, "impl.B")
	first := coordinatorResult(a, other, b)
	factory := &resultFactory{results: []graph.BuildResult{
		first,
		emptyCoordinatorResult(),
		coordinatorResult(),
	}}
	selector := &recordingSelector{selections: func(sites []resolve.DispatchSite) []Selection {
		if len(sites) != 2 || sites[0].ID != "b" || sites[1].ID != "a" {
			t.Fatalf("frontier order = %v", []resolve.DispatchSiteID{sites[0].ID, sites[1].ID})
		}
		sites[0].Call.Args = []string{"mutated"}
		sites[0].Candidates[0].ImplementationFQCN = "mutated"
		return selectAll(sites)
	}}
	if _, err := (Coordinator{}).Build(Request{Selector: selector, BuildFactory: factory.Build}); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(selector.calls) != 1 || len(selector.calls[0]) != 2 {
		t.Fatalf("selector calls = %#v", selector.calls)
	}
	for _, edge := range first.Graph.Edges {
		if edge.DispatchSite.Candidates[0].ImplementationFQCN == "mutated" || len(edge.DispatchSite.Call.Args) != 0 {
			t.Fatal("selector mutation aliased the emitted graph")
		}
	}
}

func TestCompareSitesUsesEveryCanonicalTieBreaker(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(left, right *resolve.DispatchSite)
	}{
		{"caller type", func(left, right *resolve.DispatchSite) {
			left.Caller.Method.TypeFQCN, right.Caller.Method.TypeFQCN = "a.Caller", "b.Caller"
		}},
		{"caller method", func(left, right *resolve.DispatchSite) {
			left.Caller.Method.Method, right.Caller.Method.Method = "a", "b"
		}},
		{"caller signature", func(left, right *resolve.DispatchSite) {
			left.Caller.Method.Signature, right.Caller.Method.Signature = "(a)", "(b)"
		}},
		{"caller runtime", func(left, right *resolve.DispatchSite) {
			left.Caller.RuntimeTypeFQCN, right.Caller.RuntimeTypeFQCN = "a.Runtime", "b.Runtime"
		}},
		{"logical file", func(left, right *resolve.DispatchSite) {
			left.Call.File, right.Call.File = "src/main/java/a/Caller.java", "src/main/java/b/Caller.java"
		}},
		{"start byte", func(left, right *resolve.DispatchSite) {
			left.Call.StartByte, right.Call.StartByte = 2, 10
		}},
		{"call kind", func(left, right *resolve.DispatchSite) {
			left.Call.Kind, right.Call.Kind = java.CallInvocation, java.CallObjectCreation
		}},
		{"receiver", func(left, right *resolve.DispatchSite) {
			left.ReceiverFQCN, right.ReceiverFQCN = "a.Contract", "b.Contract"
		}},
		{"method", func(left, right *resolve.DispatchSite) {
			left.Method, right.Method = "a", "b"
		}},
		{"signature", func(left, right *resolve.DispatchSite) {
			left.Signature, right.Signature = "()", "(z)"
		}},
		{"id", func(left, right *resolve.DispatchSite) {
			left.ID, right.ID = "a", "b"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			left := coordinatorSite("same", "Caller", "Contract", "file.java", 1, "impl.A")
			right := resolve.CloneDispatchSite(left)
			test.mutate(left, right)
			if got := compareSites(left, right); got >= 0 {
				t.Fatalf("compareSites(left,right) = %d, want negative", got)
			}
			if got := compareSites(right, left); got <= 0 {
				t.Fatalf("compareSites(right,left) = %d, want positive", got)
			}
		})
	}
}

func TestCoordinatorValidationIsAtomic(t *testing.T) {
	a := coordinatorSite("a", "CallerA", "Contract", "A.java", 1, "impl.A")
	b := coordinatorSite("b", "CallerB", "Contract", "B.java", 2, "impl.B")
	factory := &resultFactory{results: []graph.BuildResult{coordinatorResult(a, b)}}
	selector := &recordingSelector{selections: func(sites []resolve.DispatchSite) []Selection {
		return []Selection{
			{SiteID: sites[0].ID, Choice: resolve.DispatchChoice{Mode: resolve.ChoiceAll}},
			{SiteID: sites[1].ID, Choice: resolve.DispatchChoice{Mode: resolve.ChoiceSelected, ImplementationFQCN: "missing"}},
		}
	}}
	got, err := (Coordinator{}).Build(Request{Selector: selector, BuildFactory: factory.Build})
	if err == nil || !reflect.DeepEqual(got, graph.BuildResult{}) {
		t.Fatalf("Build result/error = %+v/%v, want zero result and error", got, err)
	}
	policy := factory.calls[0].(resolve.SitePolicy)
	if len(policy.Choices) != 0 {
		t.Fatalf("invalid batch partially merged: %#v", policy.Choices)
	}
}

func TestCoordinatorRejectsConflictingMetadataForSameID(t *testing.T) {
	first := coordinatorSite("same", "Caller", "Contract", "A.java", 1, "impl.A")
	second := resolve.CloneDispatchSite(first)
	second.ReceiverFQCN = "OtherContract"
	factory := &resultFactory{results: []graph.BuildResult{coordinatorResult(first, second)}}
	selector := &recordingSelector{selections: selectAll}
	got, err := (Coordinator{}).Build(Request{Selector: selector, BuildFactory: factory.Build})
	if err == nil || !reflect.DeepEqual(got, graph.BuildResult{}) {
		t.Fatalf("conflicting metadata result/error = %+v/%v", got, err)
	}
	if len(selector.calls) != 0 {
		t.Fatal("selector called despite conflicting site metadata")
	}
}

func TestCoordinatorRejectsMalformedBatches(t *testing.T) {
	site := coordinatorSite("site", "Caller", "Contract", "A.java", 1, "impl.A")
	tests := []struct {
		name  string
		batch func(resolve.DispatchSite) []Selection
	}{
		{"missing", func(resolve.DispatchSite) []Selection { return nil }},
		{"duplicate", func(site resolve.DispatchSite) []Selection {
			return []Selection{{SiteID: site.ID, Choice: resolve.DispatchChoice{Mode: resolve.ChoiceAll}}, {SiteID: site.ID, Choice: resolve.DispatchChoice{Mode: resolve.ChoiceAll}}}
		}},
		{"unknown", func(resolve.DispatchSite) []Selection {
			return []Selection{{SiteID: "unknown", Choice: resolve.DispatchChoice{Mode: resolve.ChoiceAll}}}
		}},
		{"unknown mode", func(site resolve.DispatchSite) []Selection {
			return []Selection{{SiteID: site.ID, Choice: resolve.DispatchChoice{Mode: "invalid"}}}
		}},
		{"none fqcn", func(site resolve.DispatchSite) []Selection {
			return []Selection{{SiteID: site.ID, Choice: resolve.DispatchChoice{Mode: resolve.ChoiceNone, ImplementationFQCN: "impl.A"}}}
		}},
		{"selected missing fqcn", func(site resolve.DispatchSite) []Selection {
			return []Selection{{SiteID: site.ID, Choice: resolve.DispatchChoice{Mode: resolve.ChoiceSelected}}}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			factory := &resultFactory{results: []graph.BuildResult{coordinatorResult(site)}}
			selector := &recordingSelector{selections: func(sites []resolve.DispatchSite) []Selection { return test.batch(sites[0]) }}
			got, err := (Coordinator{}).Build(Request{Selector: selector, BuildFactory: factory.Build})
			if err == nil || !reflect.DeepEqual(got, graph.BuildResult{}) {
				t.Fatalf("Build result/error = %+v/%v, want zero result and error", got, err)
			}
			if len(factory.calls) != 1 || len(factory.calls[0].(resolve.SitePolicy).Choices) != 0 {
				t.Fatalf("malformed batch changed choices: %#v", factory.calls)
			}
		})
	}
}

func TestCoordinatorCancellationPreservesSentinel(t *testing.T) {
	factory := &resultFactory{results: []graph.BuildResult{coordinatorResult(coordinatorSite("a", "Caller", "Contract", "A.java", 1, "impl.A"))}}
	selector := &recordingSelector{err: errors.Join(errors.New("user stopped"), ErrSelectionCanceled)}
	got, err := (Coordinator{}).Build(Request{Selector: selector, BuildFactory: factory.Build})
	if !errors.Is(err, ErrSelectionCanceled) || !reflect.DeepEqual(got, graph.BuildResult{}) {
		t.Fatalf("cancellation result/error = %+v/%v", got, err)
	}
}

func TestCoordinatorCancellationAfterAccumulatedRoundSkipsFinalBuild(t *testing.T) {
	a := coordinatorSite("a", "CallerA", "Contract", "A.java", 1, "impl.A")
	b := coordinatorSite("b", "CallerB", "Contract", "B.java", 2, "impl.B")
	factory := &resultFactory{results: []graph.BuildResult{
		coordinatorResult(a),
		coordinatorResult(b),
	}}
	selector := &recordingSelector{
		selections: selectFirstCandidates,
		errors: []error{
			nil,
			fmt.Errorf("selector stopped: %w", ErrSelectionCanceled),
		},
	}
	got, err := (Coordinator{}).Build(Request{Selector: selector, BuildFactory: factory.Build})
	if !errors.Is(err, ErrSelectionCanceled) || !reflect.DeepEqual(got, graph.BuildResult{}) {
		t.Fatalf("cancellation result/error = %+v/%v", got, err)
	}
	if len(selector.calls) != 2 || len(factory.calls) != 2 {
		t.Fatalf("selector/build calls = %d/%d, want two discovery calls and no final build", len(selector.calls), len(factory.calls))
	}
}

func TestCoordinatorUsesSiteIDForDifferentChoicesAndOnlyOnce(t *testing.T) {
	a := coordinatorSite("a", "CallerA", "Contract", "A.java", 1, "impl.A", "impl.B")
	b := coordinatorSite("b", "CallerB", "Contract", "B.java", 1, "impl.A", "impl.B")
	factory := &resultFactory{results: []graph.BuildResult{
		coordinatorResult(a, b),
		coordinatorResult(a, b),
		coordinatorResult(),
	}}
	selector := &recordingSelector{selections: func(sites []resolve.DispatchSite) []Selection {
		return []Selection{
			{SiteID: "a", Choice: resolve.DispatchChoice{Mode: resolve.ChoiceSelected, ImplementationFQCN: "impl.A"}},
			{SiteID: "b", Choice: resolve.DispatchChoice{Mode: resolve.ChoiceSelected, ImplementationFQCN: "impl.B"}},
		}
	}}
	if _, err := (Coordinator{}).Build(Request{Selector: selector, BuildFactory: factory.Build}); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(selector.calls) != 1 || len(factory.calls) != 3 {
		t.Fatalf("selector/build calls = %d/%d, want 1/3", len(selector.calls), len(factory.calls))
	}
	final := factory.calls[2].(resolve.SitePolicy)
	if final.Choices["a"].ImplementationFQCN != "impl.A" || final.Choices["b"].ImplementationFQCN != "impl.B" {
		t.Fatalf("site choices collapsed by receiver: %#v", final.Choices)
	}
}

func TestCoordinatorCycleCompletesByChoiceProgress(t *testing.T) {
	site := coordinatorSite("cycle", "Root", "Contract", "Cycle.java", 1, "impl.Cycle")
	factory := &resultFactory{results: []graph.BuildResult{
		cycleCoordinatorResult(site),
		cycleCoordinatorResult(site),
		coordinatorResult(),
	}}
	selector := &recordingSelector{selections: selectFirstCandidates}
	if _, err := (Coordinator{}).Build(Request{Selector: selector, BuildFactory: factory.Build}); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(selector.calls) != 1 || len(factory.calls) != 3 {
		t.Fatalf("cycle selector/build calls = %d/%d, want 1/3", len(selector.calls), len(factory.calls))
	}
}
