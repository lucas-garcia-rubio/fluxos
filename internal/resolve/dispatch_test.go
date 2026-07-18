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
	if !strings.Contains(res.Targets[0].Key.Method.TypeFQCN, "#noimpl#") {
		t.Fatalf("expected noimpl token in handle, got %q", res.Targets[0].Key.Method.TypeFQCN)
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
	if res.Targets[0].Key.RuntimeTypeFQCN != "contract.Defaulted" {
		t.Fatalf("default runtime = %q", res.Targets[0].Key.RuntimeTypeFQCN)
	}
}

func TestDispatchZeroImplementationsWithAmbiguousDefaultsPreservesOverloadTerminal(t *testing.T) {
	iface := &java.TypeDecl{
		Kind: java.TypeKindInterface, Name: "Defaulted", FQCN: "contract.Defaulted", File: "D.java",
		Methods: []java.MethodDecl{
			{Name: "run", Signature: "(java.lang.String)", Modifier: []string{"public", "default"}, Params: []java.Param{{Type: java.NewTypeRef("java.lang.String", false)}}},
			{Name: "run", Signature: "(java.lang.Integer)", Modifier: []string{"public", "default"}, Params: []java.Param{{Type: java.NewTypeRef("java.lang.Integer", false)}}},
		},
	}
	r := dispatchFixture(t, iface)
	ctx := MethodContext{LocalVars: []java.LocalVarDecl{localVar("svc", "contract.Defaulted")}}
	res := r.Resolve(java.CallSite{Receiver: "svc", MethodName: "run", Args: []string{"value()"}, ArgCount: 1, StartByte: 1}, ctx)
	assertTerminalKind(t, res, ResolutionAmbiguousOverload)
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
	if res.Targets[0].Key.Method.TypeFQCN != "contract.SingleImpl" {
		t.Fatalf("expected impl owner, got %q", res.Targets[0].Key.Method.TypeFQCN)
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

func TestDispatchSingleImplementationWithIncompatibleOverloadBecomesUnresolved(t *testing.T) {
	iface := &java.TypeDecl{Kind: java.TypeKindInterface, Name: "Single", FQCN: "contract.Single", File: "S.java"}
	impl := &java.TypeDecl{
		Kind: java.TypeKindClass, Name: "SingleImpl", FQCN: "contract.SingleImpl", File: "I.java",
		Interfaces: []java.TypeRef{java.NewTypeRef("contract.Single", false)},
		Methods: []java.MethodDecl{{
			Name: "run", Signature: "(java.lang.String)",
			Params: []java.Param{{Type: java.NewTypeRef("java.lang.String", false)}},
		}},
	}
	r := dispatchFixture(t, iface, impl)
	ctx := MethodContext{LocalVars: []java.LocalVarDecl{localVar("svc", "contract.Single")}}
	res := r.Resolve(java.CallSite{Receiver: "svc", MethodName: "run", Args: []string{"1"}, ArgCount: 1, StartByte: 1}, ctx)
	assertTerminalKind(t, res, ResolutionUnresolved)
}

func TestDispatchSingleImplementationPreservesLexicalOverloadAmbiguity(t *testing.T) {
	iface := &java.TypeDecl{
		Kind: java.TypeKindInterface, Name: "Single", FQCN: "contract.Single", File: "S.java",
		Methods: []java.MethodDecl{
			overloadMethod("run", false, "java.lang.String"),
			overloadMethod("run", false, "java.lang.Integer"),
		},
	}
	impl := mkType("contract.SingleImpl",
		overloadMethod("run", false, "java.lang.String"),
		overloadMethod("run", false, "java.lang.Integer"),
	)
	impl.Interfaces = []java.TypeRef{ref("contract.Single")}
	r := dispatchFixture(t, iface, impl)
	ctx := MethodContext{LocalVars: []java.LocalVarDecl{localVar("svc", "contract.Single")}}

	result := r.Resolve(java.CallSite{Receiver: "svc", MethodName: "run", Args: []string{"value()"}, ArgCount: 1, StartByte: 1}, ctx)
	assertTerminalKind(t, result, ResolutionAmbiguousOverload)
	if got := result.Targets[0].Key.Method.TypeFQCN; !strings.HasPrefix(got, iface.FQCN+"#ambover#") {
		t.Fatalf("ambiguous overload owner = %q, want lexical owner %q", got, iface.FQCN)
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
	if res.DispatchSite == nil || len(res.DispatchSite.Candidates) != 2 {
		t.Fatalf("dispatch site = %+v", res.DispatchSite)
	}
	for i, c := range wantCandidates {
		if res.DispatchSite.Candidates[i].ImplementationFQCN != c {
			t.Fatalf("candidates[%d] = %q, want %q (full: %+v)", i, res.DispatchSite.Candidates[i].ImplementationFQCN, c, res.DispatchSite.Candidates)
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
	if res.Targets[0].Key.Method.TypeFQCN != "svc.Concrete" {
		t.Fatalf("expected concrete owner, got %q", res.Targets[0].Key.Method.TypeFQCN)
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
	if res.Targets[0].Key.Method.TypeFQCN != "base.Concrete" {
		t.Fatalf("expected impl owner, got %q", res.Targets[0].Key.Method.TypeFQCN)
	}
}
