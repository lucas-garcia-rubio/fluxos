package resolve

import (
	"strings"
	"testing"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
)

func policyCandidates() []ImplementationCandidate {
	return []ImplementationCandidate{
		{ImplementationFQCN: "a.Impl", Target: MethodHandle{TypeFQCN: "Contract", Method: "run", Signature: "()"}, Kind: ResolutionConcrete},
		{ImplementationFQCN: "b.Impl", Target: MethodHandle{TypeFQCN: "Contract", Method: "run", Signature: "()"}, Kind: ResolutionConcrete},
		{ImplementationFQCN: "c.Impl", Target: MethodHandle{TypeFQCN: "Contract", Method: "run", Signature: "()"}, Kind: ResolutionConcrete},
	}
}

func policyCaller() ExecutionKey {
	return ExecutionKey{
		Method:          MethodHandle{TypeFQCN: "app.Workflow", Method: "start", Signature: "()"},
		RuntimeTypeFQCN: "app.Workflow",
	}
}

func policyCall() java.CallSite {
	return java.CallSite{MethodName: "run", File: "App.java", StartByte: 100}
}

func policySite(candidates []ImplementationCandidate) *DispatchSite {
	return NewDispatchSite(policyCaller(), "Contract", "run", "()", policyCall(), candidates)
}

func TestTerminalPolicyProducesAmbiguousTerminalWithNoFanOut(t *testing.T) {
	decision := (TerminalPolicy{}).Apply(policySite(policyCandidates()))
	if len(decision.Targets) != 1 {
		t.Fatalf("TerminalPolicy must produce one terminal: %+v", decision.Targets)
	}
	terminal := decision.Targets[0]
	if terminal.Kind != ResolutionAmbiguousImplementation {
		t.Fatalf("kind = %v, want AmbiguousImplementation", terminal.Kind)
	}
	if !strings.HasPrefix(terminal.Key.Method.TypeFQCN, "Contract#ambimpl#") {
		t.Fatalf("terminal receiver = %q, want Contract#ambimpl# prefix", terminal.Key.Method.TypeFQCN)
	}
	if decision.Omitted != 0 {
		t.Fatalf("TerminalPolicy never omits: got %d", decision.Omitted)
	}
}

func TestAllPolicyFansOutConcreteTargetsInCanonicalOrderWithRuntimeContext(t *testing.T) {
	candidates := []ImplementationCandidate{
		{ImplementationFQCN: "z.Impl", Target: MethodHandle{TypeFQCN: "Contract", Method: "run", Signature: "()"}, Kind: ResolutionConcrete},
		{ImplementationFQCN: "a.Impl", Target: MethodHandle{TypeFQCN: "Contract", Method: "run", Signature: "()"}, Kind: ResolutionConcrete},
	}
	decision := (AllPolicy{}).Apply(policySite(candidates))
	if len(decision.Targets) != 2 || decision.Omitted != 0 {
		t.Fatalf("decision = %+v", decision)
	}
	for i, want := range []string{"a.Impl", "z.Impl"} {
		target := decision.Targets[i]
		if target.Kind != ResolutionConcrete || target.Key.RuntimeTypeFQCN != want {
			t.Fatalf("target %d = %+v, want concrete runtime %q", i, target, want)
		}
	}
}

func TestAllPolicyMaxImplsCountsNonConcreteCandidatesAndEmitsEachAdmittedResult(t *testing.T) {
	candidates := []ImplementationCandidate{
		{ImplementationFQCN: "a.Broken", Kind: ResolutionNoImplementation, Note: "method missing"},
		{ImplementationFQCN: "b.Good", Target: MethodHandle{TypeFQCN: "Contract", Method: "run", Signature: "()"}, Kind: ResolutionConcrete},
		{ImplementationFQCN: "c.Broken", Kind: ResolutionUnresolved, Note: "not reachable"},
	}
	decision := (AllPolicy{MaxImpls: 2}).Apply(policySite(candidates))
	if len(decision.Targets) != 2 || decision.Omitted != 1 {
		t.Fatalf("decision = %+v, want two admitted and one omitted", decision)
	}
	if decision.Targets[0].Kind != ResolutionNoImplementation || decision.Targets[1].Kind != ResolutionConcrete {
		t.Fatalf("admitted results = %+v", decision.Targets)
	}
}

func TestAllPolicyMaxImplsZeroMeansUnlimited(t *testing.T) {
	decision := (AllPolicy{MaxImpls: 0}).Apply(policySite(policyCandidates()))
	if len(decision.Targets) != 3 || decision.Omitted != 0 {
		t.Fatalf("MaxImpls=0 decision = %+v", decision)
	}
}

func TestAllPolicyEmptyCandidatesReturnsEmpty(t *testing.T) {
	decision := (AllPolicy{}).Apply(policySite(nil))
	if len(decision.Targets) != 0 || decision.Omitted != 0 {
		t.Fatalf("decision = %+v, want empty", decision)
	}
}

func TestFixedPolicyNoneKeepsAmbiguousTerminal(t *testing.T) {
	policy := FixedPolicy{Choices: map[string]FixedChoice{"Contract": {Kind: FixedChoiceNone}}}
	decision := policy.Apply(policySite(policyCandidates()))
	if len(decision.Targets) != 1 || decision.Targets[0].Kind != ResolutionAmbiguousImplementation || decision.Omitted != 0 {
		t.Fatalf("None must preserve the ambiguous terminal: %+v", decision)
	}
}

func TestFixedPolicyAllAndExplicitUseOnlyTheirSelectedCandidates(t *testing.T) {
	all := (FixedPolicy{Choices: map[string]FixedChoice{"Contract": {Kind: FixedChoiceAll}}}).Apply(policySite(policyCandidates()))
	if len(all.Targets) != 3 {
		t.Fatalf("All should fan out to all candidates: %+v", all.Targets)
	}

	explicit := (FixedPolicy{Choices: map[string]FixedChoice{
		"Contract": {Kind: FixedChoiceExplicit, Impls: []string{"a.Impl", "c.Impl"}},
	}}).Apply(policySite(policyCandidates()))
	if len(explicit.Targets) != 2 || explicit.Targets[0].Key.RuntimeTypeFQCN != "a.Impl" || explicit.Targets[1].Key.RuntimeTypeFQCN != "c.Impl" {
		t.Fatalf("Explicit targets = %+v", explicit.Targets)
	}
}

func TestFixedPolicyExplicitBrokenSelectedCandidateDoesNotLeakUnselectedTerminal(t *testing.T) {
	candidates := []ImplementationCandidate{
		{ImplementationFQCN: "a.Good", Target: MethodHandle{TypeFQCN: "Contract", Method: "run", Signature: "()"}, Kind: ResolutionConcrete},
		{ImplementationFQCN: "b.Broken", Kind: ResolutionNoImplementation, Note: "unselected"},
		{ImplementationFQCN: "c.Broken", Kind: ResolutionUnresolved, Note: "selected"},
	}
	policy := FixedPolicy{Choices: map[string]FixedChoice{
		"Contract": {Kind: FixedChoiceExplicit, Impls: []string{"c.Broken"}},
	}}
	decision := policy.Apply(policySite(candidates))
	if len(decision.Targets) != 1 || decision.Targets[0].Kind != ResolutionUnresolved || decision.Targets[0].Key.RuntimeTypeFQCN != "c.Broken" {
		t.Fatalf("selected broken decision = %+v", decision)
	}
}

func TestFixedPolicyFallsBackForUnmappedReceiver(t *testing.T) {
	policy := FixedPolicy{
		Choices:  map[string]FixedChoice{"Other": {Kind: FixedChoiceAll}},
		Fallback: TerminalPolicy{},
	}
	decision := policy.Apply(policySite(policyCandidates()))
	if len(decision.Targets) != 1 || decision.Targets[0].Kind != ResolutionAmbiguousImplementation {
		t.Fatalf("unmapped receiver must fall back to TerminalPolicy: %+v", decision)
	}
}

func TestFixedPolicyDefaultsToTerminalPolicyWhenFallbackIsNil(t *testing.T) {
	decision := (FixedPolicy{Choices: map[string]FixedChoice{}}).Apply(policySite(policyCandidates()))
	if len(decision.Targets) != 1 || decision.Targets[0].Kind != ResolutionAmbiguousImplementation {
		t.Fatalf("nil Fallback must default to TerminalPolicy: %+v", decision)
	}
}

func TestSitePolicyUsesSiteIDPrecedenceAndFallback(t *testing.T) {
	site := policySite(policyCandidates())
	policy := SitePolicy{
		Choices: map[DispatchSiteID]DispatchChoice{
			site.ID: {Mode: ChoiceModeSelected, ImplementationFQCN: "b.Impl"},
		},
		Fallback: AllPolicy{},
	}
	decision := policy.Apply(site)
	if len(decision.Targets) != 1 || decision.Targets[0].Key.RuntimeTypeFQCN != "b.Impl" {
		t.Fatalf("site choice should take precedence: %+v", decision)
	}

	other := policySite(policyCandidates())
	other.ID = "other-site"
	decision = policy.Apply(other)
	if len(decision.Targets) != 3 {
		t.Fatalf("unmapped site should use fallback: %+v", decision)
	}
}

func TestSitePolicySelectedBrokenCandidateDoesNotLeakUnselectedTerminal(t *testing.T) {
	site := policySite([]ImplementationCandidate{
		{ImplementationFQCN: "a.Broken", Kind: ResolutionNoImplementation, Note: "unselected"},
		{ImplementationFQCN: "b.Broken", Kind: ResolutionUnresolved, Note: "selected"},
		{ImplementationFQCN: "c.Good", Target: MethodHandle{TypeFQCN: "Contract", Method: "run", Signature: "()"}, Kind: ResolutionConcrete},
	})
	policy := SitePolicy{
		Choices: map[DispatchSiteID]DispatchChoice{
			site.ID: {Mode: ChoiceModeSelected, ImplementationFQCN: "b.Broken"},
		},
	}
	decision := policy.Apply(site)
	if len(decision.Targets) != 1 || decision.Targets[0].Kind != ResolutionUnresolved || decision.Targets[0].Key.RuntimeTypeFQCN != "b.Broken" {
		t.Fatalf("selected site decision = %+v", decision)
	}
}
