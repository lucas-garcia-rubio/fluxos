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

func receiverType(fqcn string) *java.TypeDecl {
	return &java.TypeDecl{Kind: java.TypeKindInterface, Name: fqcn, FQCN: fqcn}
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

func TestTerminalPolicyProducesAmbiguousTerminalWithNoFanOut(t *testing.T) {
	policy := TerminalPolicy{}
	decision := policy.Apply(policyCaller(), receiverType("Contract"), policyCall(), policyCandidates())
	if len(decision.Targets) != 0 {
		t.Fatalf("TerminalPolicy must not fan out: %+v", decision.Targets)
	}
	if decision.Terminal == nil {
		t.Fatal("TerminalPolicy must produce a terminal")
	}
	if decision.Terminal.Kind != ResolutionAmbiguousImplementation {
		t.Fatalf("kind = %v, want AmbiguousImplementation", decision.Terminal.Kind)
	}
	if !strings.HasPrefix(decision.Terminal.Key.Method.TypeFQCN, "Contract#ambimpl#") {
		t.Fatalf("terminal receiver = %q, want Contract#ambimpl# prefix", decision.Terminal.Key.Method.TypeFQCN)
	}
	if decision.Omitted != 0 {
		t.Fatalf("TerminalPolicy never omits: got %d", decision.Omitted)
	}
}

func TestAllPolicyFansOutToConcreteTargets(t *testing.T) {
	policy := AllPolicy{}
	decision := policy.Apply(policyCaller(), receiverType("Contract"), policyCall(), policyCandidates())
	if len(decision.Targets) != 3 {
		t.Fatalf("targets = %+v, want 3", decision.Targets)
	}
	if decision.Terminal != nil {
		t.Fatalf("no terminal expected when all candidates are concrete: %+v", decision.Terminal)
	}
	for i, target := range decision.Targets {
		if target.Kind != ResolutionConcrete {
			t.Fatalf("target %d kind = %v, want Concrete", i, target.Kind)
		}
		want := ExecutionKey{
			Method:          MethodHandle{TypeFQCN: "Contract", Method: "run", Signature: "()"},
			RuntimeTypeFQCN: target.Key.RuntimeTypeFQCN,
		}
		if target.Key.Method != want.Method {
			t.Fatalf("target %d method = %+v, want %+v", i, target.Key.Method, want.Method)
		}
	}
}

func TestAllPolicyPreservesRuntimeContextPerImpl(t *testing.T) {
	policy := AllPolicy{}
	decision := policy.Apply(policyCaller(), receiverType("Contract"), policyCall(), policyCandidates())
	if len(decision.Targets) != 3 {
		t.Fatalf("targets = %+v", decision.Targets)
	}
	runtimes := map[string]bool{}
	for _, target := range decision.Targets {
		runtimes[target.Key.RuntimeTypeFQCN] = true
	}
	for _, want := range []string{"a.Impl", "b.Impl", "c.Impl"} {
		if !runtimes[want] {
			t.Fatalf("missing runtime context %q in %+v", want, runtimes)
		}
	}
}

func TestAllPolicyAppliesMaxImplsAndReportsOmitted(t *testing.T) {
	policy := AllPolicy{MaxImpls: 2}
	decision := policy.Apply(policyCaller(), receiverType("Contract"), policyCall(), policyCandidates())
	if len(decision.Targets) != 2 {
		t.Fatalf("targets = %+v, want 2 (admitted by MaxImpls)", decision.Targets)
	}
	if decision.Omitted != 1 {
		t.Fatalf("omitted = %d, want 1", decision.Omitted)
	}
}

func TestAllPolicyMaxImplsZeroMeansUnlimited(t *testing.T) {
	policy := AllPolicy{MaxImpls: 0}
	decision := policy.Apply(policyCaller(), receiverType("Contract"), policyCall(), policyCandidates())
	if len(decision.Targets) != 3 {
		t.Fatalf("MaxImpls=0 should admit all: %+v", decision.Targets)
	}
	if decision.Omitted != 0 {
		t.Fatalf("omitted = %d, want 0", decision.Omitted)
	}
}

func TestAllPolicyAdmitsInCanonicalOrder(t *testing.T) {
	// candidates fora de ordem canonical; AllPolicy não reordena, mas deve
	// preservar a ordem recebida. Ordenação final é responsabilidade do
	// Build/snapshot. Verificamos aqui a estabilidade da policy.
	candidates := []ImplementationCandidate{
		{ImplementationFQCN: "z.Impl", Target: MethodHandle{TypeFQCN: "Contract", Method: "run", Signature: "()"}, Kind: ResolutionConcrete},
		{ImplementationFQCN: "a.Impl", Target: MethodHandle{TypeFQCN: "Contract", Method: "run", Signature: "()"}, Kind: ResolutionConcrete},
	}
	policy := AllPolicy{}
	decision := policy.Apply(policyCaller(), receiverType("Contract"), policyCall(), candidates)
	if len(decision.Targets) != 2 {
		t.Fatalf("targets = %+v", decision.Targets)
	}
	if decision.Targets[0].Key.RuntimeTypeFQCN != "z.Impl" {
		t.Fatalf("order not preserved: %+v", decision.Targets)
	}
}

func TestAllPolicyTurnsNonConcreteCandidateIntoIsolatedTerminal(t *testing.T) {
	candidates := []ImplementationCandidate{
		{ImplementationFQCN: "good.Impl", Target: MethodHandle{TypeFQCN: "Contract", Method: "run", Signature: "()"}, Kind: ResolutionConcrete},
		{ImplementationFQCN: "broken.Impl", Kind: ResolutionNoImplementation, Note: "method missing"},
	}
	policy := AllPolicy{}
	decision := policy.Apply(policyCaller(), receiverType("Contract"), policyCall(), candidates)
	if len(decision.Targets) != 1 {
		t.Fatalf("targets = %+v, want 1 concrete", decision.Targets)
	}
	if decision.Terminal == nil || decision.Terminal.Kind != ResolutionNoImplementation {
		t.Fatalf("terminal = %+v, want NoImplementation", decision.Terminal)
	}
}

func TestAllPolicyEmptyCandidatesReturnsEmpty(t *testing.T) {
	policy := AllPolicy{}
	decision := policy.Apply(policyCaller(), receiverType("Contract"), policyCall(), nil)
	if len(decision.Targets) != 0 || decision.Terminal != nil || decision.Omitted != 0 {
		t.Fatalf("decision = %+v, want empty", decision)
	}
}

func TestFixedPolicyNoneKeepsAmbiguousTerminal(t *testing.T) {
	policy := FixedPolicy{Choices: map[string]FixedChoice{"Contract": {Kind: FixedChoiceNone}}}
	decision := policy.Apply(policyCaller(), receiverType("Contract"), policyCall(), policyCandidates())
	if len(decision.Targets) != 0 || decision.Terminal == nil || decision.Terminal.Kind != ResolutionAmbiguousImplementation || decision.Omitted != 0 {
		t.Fatalf("None must preserve the ambiguous terminal: %+v", decision)
	}
}

func TestFixedPolicyAllAdmitsEveryConcreteCandidate(t *testing.T) {
	policy := FixedPolicy{Choices: map[string]FixedChoice{"Contract": {Kind: FixedChoiceAll}}}
	decision := policy.Apply(policyCaller(), receiverType("Contract"), policyCall(), policyCandidates())
	if len(decision.Targets) != 3 {
		t.Fatalf("All should fan out to all 3 concrete candidates: %+v", decision.Targets)
	}
	if decision.Terminal != nil {
		t.Fatalf("All should not produce terminal when all candidates are concrete: %+v", decision.Terminal)
	}
}

func TestFixedPolicyExplicitAdmitsOnlyListedImpls(t *testing.T) {
	policy := FixedPolicy{Choices: map[string]FixedChoice{
		"Contract": {Kind: FixedChoiceExplicit, Impls: []string{"a.Impl", "c.Impl"}},
	}}
	decision := policy.Apply(policyCaller(), receiverType("Contract"), policyCall(), policyCandidates())
	if len(decision.Targets) != 2 {
		t.Fatalf("Explicit should admit only listed impls: %+v", decision.Targets)
	}
	for _, target := range decision.Targets {
		if target.Key.RuntimeTypeFQCN == "b.Impl" {
			t.Fatalf("b.Impl should not be admitted: %+v", decision.Targets)
		}
	}
}

func TestFixedPolicyPreservesRuntimeContextPerImpl(t *testing.T) {
	policy := FixedPolicy{Choices: map[string]FixedChoice{"Contract": {Kind: FixedChoiceAll}}}
	decision := policy.Apply(policyCaller(), receiverType("Contract"), policyCall(), policyCandidates())
	runtimes := map[string]bool{}
	for _, target := range decision.Targets {
		runtimes[target.Key.RuntimeTypeFQCN] = true
	}
	for _, want := range []string{"a.Impl", "b.Impl", "c.Impl"} {
		if !runtimes[want] {
			t.Fatalf("missing runtime %q in %+v", want, runtimes)
		}
	}
}

func TestFixedPolicyFallsBackForUnmappedReceiver(t *testing.T) {
	fallback := TerminalPolicy{}
	policy := FixedPolicy{
		Choices:  map[string]FixedChoice{"Other": {Kind: FixedChoiceAll}},
		Fallback: fallback,
	}
	decision := policy.Apply(policyCaller(), receiverType("Contract"), policyCall(), policyCandidates())
	if decision.Terminal == nil || decision.Terminal.Kind != ResolutionAmbiguousImplementation {
		t.Fatalf("unmapped receiver must fall back to TerminalPolicy: %+v", decision)
	}
}

func TestFixedPolicyDefaultsToTerminalPolicyWhenFallbackIsNil(t *testing.T) {
	policy := FixedPolicy{Choices: map[string]FixedChoice{}}
	decision := policy.Apply(policyCaller(), receiverType("Contract"), policyCall(), policyCandidates())
	if decision.Terminal == nil {
		t.Fatalf("nil Fallback must default to TerminalPolicy: %+v", decision)
	}
}

func TestFixedPolicyExplicitAdmitsPreservingOrderFromCandidates(t *testing.T) {
	candidates := []ImplementationCandidate{
		{ImplementationFQCN: "z.Impl", Target: MethodHandle{TypeFQCN: "Contract", Method: "run", Signature: "()"}, Kind: ResolutionConcrete},
		{ImplementationFQCN: "a.Impl", Target: MethodHandle{TypeFQCN: "Contract", Method: "run", Signature: "()"}, Kind: ResolutionConcrete},
		{ImplementationFQCN: "m.Impl", Target: MethodHandle{TypeFQCN: "Contract", Method: "run", Signature: "()"}, Kind: ResolutionConcrete},
	}
	policy := FixedPolicy{Choices: map[string]FixedChoice{
		"Contract": {Kind: FixedChoiceExplicit, Impls: []string{"m.Impl", "z.Impl"}},
	}}
	decision := policy.Apply(policyCaller(), receiverType("Contract"), policyCall(), candidates)
	if len(decision.Targets) != 2 {
		t.Fatalf("targets = %+v", decision.Targets)
	}
	// ordem segue a do slice de candidates recebido (z, m)
	if decision.Targets[0].Key.RuntimeTypeFQCN != "z.Impl" || decision.Targets[1].Key.RuntimeTypeFQCN != "m.Impl" {
		t.Fatalf("order not preserved: %+v", decision.Targets)
	}
}

func TestFixedPolicyExplicitTurnsNonConcreteIntoTerminal(t *testing.T) {
	candidates := []ImplementationCandidate{
		{ImplementationFQCN: "good.Impl", Target: MethodHandle{TypeFQCN: "Contract", Method: "run", Signature: "()"}, Kind: ResolutionConcrete},
		{ImplementationFQCN: "broken.Impl", Kind: ResolutionNoImplementation, Note: "missing"},
	}
	policy := FixedPolicy{Choices: map[string]FixedChoice{
		"Contract": {Kind: FixedChoiceExplicit, Impls: []string{"good.Impl"}},
	}}
	decision := policy.Apply(policyCaller(), receiverType("Contract"), policyCall(), candidates)
	if len(decision.Targets) != 1 {
		t.Fatalf("targets = %+v", decision.Targets)
	}
	if decision.Terminal == nil || decision.Terminal.Kind != ResolutionNoImplementation {
		t.Fatalf("terminal = %+v, want NoImplementation", decision.Terminal)
	}
}

func TestFixedPolicyAllAppliesMaxImpls(t *testing.T) {
	policy := FixedPolicy{
		Choices:  map[string]FixedChoice{"Contract": {Kind: FixedChoiceAll}},
		Fallback: TerminalPolicy{},
		MaxImpls: 1,
	}
	decision := policy.Apply(policyCaller(), receiverType("Contract"), policyCall(), policyCandidates())
	if len(decision.Targets) != 1 || decision.Omitted != 2 {
		t.Fatalf("mapped all must honor MaxImpls: %+v", decision)
	}
}
