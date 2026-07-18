package resolve

import (
	"strings"
	"testing"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
)

// dispatchFixture builds a resolver from the given types and exposes helpers
// to dispatch a call against a receiver declared as a local var.
func dispatchFixture(t *testing.T, types ...*java.TypeDecl) *SyntacticResolver {
	t.Helper()
	return newTestResolver(types)
}

func dispatchCallOnLocal(t *testing.T, r *SyntacticResolver, localType, receiverName, methodName string) Resolution {
	t.Helper()
	ctx := MethodContext{LocalVars: []java.LocalVarDecl{localVar(receiverName, localType)}}
	return r.Resolve(java.CallSite{Receiver: receiverName, MethodName: methodName, StartByte: 1}, ctx)
}

func TestDispatchZeroImplementationsBecomesNoImplementationTerminal(t *testing.T) {
	iface := &java.TypeDecl{Kind: java.TypeKindInterface, Name: "Empty", FQCN: "contract.Empty", File: "E.java"}
	r := dispatchFixture(t, iface)
	res := dispatchCallOnLocal(t, r, "contract.Empty", "svc", "run")
	if len(res.Targets) != 1 || res.Targets[0].Kind != ResolutionNoImplementation {
		t.Fatalf("expected NoImplementation terminal, got %+v", res.Targets)
	}
	if !strings.Contains(res.Targets[0].Handle.TypeFQCN, "#noimpl#") {
		t.Fatalf("expected noimpl token in handle, got %q", res.Targets[0].Handle.TypeFQCN)
	}
}

func TestDispatchZeroImplementationsWithDefaultDescendsToDefault(t *testing.T) {
	iface := &java.TypeDecl{
		Kind: java.TypeKindInterface, Name: "Defaulted", FQCN: "contract.Defaulted", File: "D.java",
		Methods: []java.MethodDecl{{
			Name: "run", Signature: "()", Modifier: []string{"public", "default"},
		}},
	}
	r := dispatchFixture(t, iface)
	res := dispatchCallOnLocal(t, r, "contract.Defaulted", "svc", "run")
	if len(res.Targets) != 1 || res.Targets[0].Kind != ResolutionConcrete {
		t.Fatalf("expected Concrete target on default method, got %+v", res.Targets)
	}
	if !res.Targets[0].Descend {
		t.Fatal("default method target should descend")
	}
}

func TestDispatchSingleImplementationDescendsToImpl(t *testing.T) {
	iface := &java.TypeDecl{Kind: java.TypeKindInterface, Name: "Single", FQCN: "contract.Single", File: "S.java"}
	impl := &java.TypeDecl{
		Kind: java.TypeKindClass, Name: "SingleImpl", FQCN: "contract.SingleImpl", File: "I.java",
		Interfaces: []java.TypeRef{java.NewTypeRef("contract.Single", false)},
		Methods:    []java.MethodDecl{{Name: "run", Signature: "()"}},
	}
	r := dispatchFixture(t, iface, impl)
	res := dispatchCallOnLocal(t, r, "contract.Single", "svc", "run")
	if len(res.Targets) != 1 || res.Targets[0].Kind != ResolutionConcrete {
		t.Fatalf("expected Concrete target on unique impl, got %+v", res.Targets)
	}
	if res.Targets[0].Handle.TypeFQCN != "contract.SingleImpl" {
		t.Fatalf("expected impl owner, got %q", res.Targets[0].Handle.TypeFQCN)
	}
}

func TestDispatchSingleImplementationWithoutMethodBecomesNoImplementation(t *testing.T) {
	iface := &java.TypeDecl{Kind: java.TypeKindInterface, Name: "Single", FQCN: "contract.Single", File: "S.java"}
	impl := &java.TypeDecl{
		Kind: java.TypeKindClass, Name: "SingleImpl", FQCN: "contract.SingleImpl", File: "I.java",
		Interfaces: []java.TypeRef{java.NewTypeRef("contract.Single", false)},
	}
	r := dispatchFixture(t, iface, impl)
	res := dispatchCallOnLocal(t, r, "contract.Single", "svc", "run")
	if len(res.Targets) != 1 || res.Targets[0].Kind != ResolutionNoImplementation {
		t.Fatalf("expected NoImplementation when unique impl lacks method, got %+v", res.Targets)
	}
}

func TestDispatchMultipleImplementationsTerminalWithSortedCandidates(t *testing.T) {
	iface := &java.TypeDecl{Kind: java.TypeKindInterface, Name: "Multi", FQCN: "contract.Multi", File: "M.java"}
	second := &java.TypeDecl{
		Kind: java.TypeKindClass, Name: "SecondImpl", FQCN: "contract.SecondImpl", File: "Second.java",
		Interfaces: []java.TypeRef{java.NewTypeRef("contract.Multi", false)},
	}
	first := &java.TypeDecl{
		Kind: java.TypeKindClass, Name: "FirstImpl", FQCN: "contract.FirstImpl", File: "First.java",
		Interfaces: []java.TypeRef{java.NewTypeRef("contract.Multi", false)},
	}
	r := dispatchFixture(t, iface, second, first) // shuffled input order
	res := dispatchCallOnLocal(t, r, "contract.Multi", "svc", "run")
	if len(res.Targets) != 1 || res.Targets[0].Kind != ResolutionAmbiguousImplementation {
		t.Fatalf("expected AmbiguousImplementation, got %+v", res.Targets)
	}
	wantCandidates := []string{"contract.FirstImpl", "contract.SecondImpl"}
	if len(res.Targets[0].Candidates) != 2 {
		t.Fatalf("candidates = %+v", res.Targets[0].Candidates)
	}
	for i, c := range wantCandidates {
		if res.Targets[0].Candidates[i] != c {
			t.Fatalf("candidates[%d] = %q, want %q (full: %+v)", i, res.Targets[0].Candidates[i], c, res.Targets[0].Candidates)
		}
	}
}

func TestDispatchSkipsConcreteReceiver(t *testing.T) {
	concrete := &java.TypeDecl{
		Kind: java.TypeKindClass, Name: "Concrete", FQCN: "svc.Concrete", File: "C.java",
		Methods: []java.MethodDecl{{Name: "run", Signature: "()"}},
	}
	r := dispatchFixture(t, concrete)
	res := dispatchCallOnLocal(t, r, "svc.Concrete", "c", "run")
	if len(res.Targets) != 1 || res.Targets[0].Kind != ResolutionConcrete {
		t.Fatalf("concrete receiver should not enter dispatch, got %+v", res.Targets)
	}
	if res.Targets[0].Handle.TypeFQCN != "svc.Concrete" {
		t.Fatalf("expected concrete owner, got %q", res.Targets[0].Handle.TypeFQCN)
	}
}

func TestDispatchSkipsStaticMethodOnInterface(t *testing.T) {
	iface := &java.TypeDecl{
		Kind: java.TypeKindInterface, Name: "StaticHost", FQCN: "contract.StaticHost", File: "SH.java",
		Methods: []java.MethodDecl{{Name: "helper", Signature: "()", Modifier: []string{"public", "static"}}},
	}
	r := dispatchFixture(t, iface)
	res := dispatchCallOnLocal(t, r, "contract.StaticHost", "h", "helper")
	if len(res.Targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(res.Targets))
	}
	// Static methods on interface do not enter polymorphic dispatch. They
	// resolve directly on the interface via resolveOnType, but resolveOnType
	// filters out static methods in interfaces — so we expect Unresolved here.
	if res.Targets[0].Kind != ResolutionUnresolved {
		t.Fatalf("static interface method through instance should be unresolved terminal, got %+v", res.Targets[0])
	}
}

func TestDispatchAbstractClassWithSingleImplDescends(t *testing.T) {
	abstractBase := &java.TypeDecl{
		Kind: java.TypeKindClass, Name: "Base", FQCN: "base.Base", File: "B.java",
		Modifier: []string{"abstract"},
	}
	impl := &java.TypeDecl{
		Kind: java.TypeKindClass, Name: "Concrete", FQCN: "base.Concrete", File: "C.java",
		SuperClass: java.NewTypeRef("base.Base", false),
		Methods:    []java.MethodDecl{{Name: "run", Signature: "()"}},
	}
	r := dispatchFixture(t, abstractBase, impl)
	res := dispatchCallOnLocal(t, r, "base.Base", "b", "run")
	if len(res.Targets) != 1 || res.Targets[0].Kind != ResolutionConcrete {
		t.Fatalf("expected Concrete target on abstract with single impl, got %+v", res.Targets)
	}
	if res.Targets[0].Handle.TypeFQCN != "base.Concrete" {
		t.Fatalf("expected impl owner, got %q", res.Targets[0].Handle.TypeFQCN)
	}
}
