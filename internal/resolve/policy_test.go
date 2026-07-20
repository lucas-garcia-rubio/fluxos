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
