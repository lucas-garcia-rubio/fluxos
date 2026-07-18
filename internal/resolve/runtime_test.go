package resolve

import (
	"testing"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
)

func TestThisAndUnqualifiedCallsRebindExactMethodOnCurrentRuntime(t *testing.T) {
	base := mkType("base.Base", overloadMethod("run", false, "java.lang.String"))
	child := mkType("app.Child", overloadMethod("run", false, "java.lang.String"), overloadMethod("run", false, "int"))
	child.SuperClass = ref("base.Base")
	r := newTestResolver([]*java.TypeDecl{base, child})
	base = r.Index.TypesByFQCN["base.Base"]
	ctx := MethodContext{
		EnclosingType: base,
		Execution: ExecutionKey{
			Method:          MethodHandle{TypeFQCN: "base.Base", Method: "caller", Signature: "()"},
			RuntimeTypeFQCN: "app.Child",
		},
	}
	for _, receiver := range []string{"", "this"} {
		result := r.Resolve(java.CallSite{Receiver: receiver, MethodName: "run", Args: []string{`"x"`}, ArgCount: 1}, ctx)
		want := ExecutionKey{Method: MethodHandle{TypeFQCN: "app.Child", Method: "run", Signature: "(java.lang.String)"}, RuntimeTypeFQCN: "app.Child"}
		if len(result.Targets) != 1 || result.Targets[0].Key != want {
			t.Fatalf("receiver %q target = %+v, want %+v", receiver, result.Targets, want)
		}
	}
}

func TestRuntimeRebindingDoesNotExposeRuntimeOnlyOverload(t *testing.T) {
	base := mkType("base.Base", overloadMethod("run", false, "java.lang.String"))
	child := mkType("app.Child", overloadMethod("run", false, "int"))
	child.SuperClass = ref("base.Base")
	r := newTestResolver([]*java.TypeDecl{base, child})
	ctx := MethodContext{
		EnclosingType: r.Index.TypesByFQCN["base.Base"],
		Execution:     ExecutionKey{RuntimeTypeFQCN: "app.Child"},
	}
	result := r.Resolve(java.CallSite{Receiver: "this", MethodName: "run", Args: []string{"1"}, ArgCount: 1}, ctx)
	assertTerminalKind(t, result, ResolutionUnresolved)
}

func TestPolymorphicDispatchDoesNotExposeImplementationOnlyOverload(t *testing.T) {
	contract := &java.TypeDecl{
		Kind: java.TypeKindInterface, Name: "Service", FQCN: "contract.Service",
		Methods: []java.MethodDecl{overloadMethod("run", false, "java.lang.String")},
	}
	impl := mkType("app.ServiceImpl",
		overloadMethod("run", false, "java.lang.String"),
		overloadMethod("run", false, "int"),
	)
	impl.Interfaces = []java.TypeRef{ref("contract.Service")}
	r := newTestResolver([]*java.TypeDecl{contract, impl})
	ctx := MethodContext{LocalVars: []java.LocalVarDecl{localVar("service", "contract.Service")}}

	result := r.Resolve(java.CallSite{Receiver: "service", MethodName: "run", Args: []string{"1"}, ArgCount: 1}, ctx)
	assertTerminalKind(t, result, ResolutionUnresolved)
}

func TestCurrentObjectCallDispatchesWhenRuntimeIsAbstract(t *testing.T) {
	hook := mkMethod("hook")
	hook.Modifier = []string{"protected", "abstract"}
	base := mkType("base.Base", hook)
	base.Modifier = []string{"abstract"}
	first := mkType("app.First", mkMethod("hook"))
	first.SuperClass = ref("base.Base")
	second := mkType("app.Second", mkMethod("hook"))
	second.SuperClass = ref("base.Base")
	r := newTestResolver([]*java.TypeDecl{base, first, second})
	base = r.Index.TypesByFQCN[base.FQCN]
	ctx := MethodContext{
		EnclosingType: base,
		Execution: ExecutionKey{
			Method:          MethodHandle{TypeFQCN: base.FQCN, Method: "run", Signature: "()"},
			RuntimeTypeFQCN: base.FQCN,
		},
	}

	for _, receiver := range []string{"", "this"} {
		result := r.Resolve(java.CallSite{Receiver: receiver, MethodName: "hook"}, ctx)
		assertTerminalKind(t, result, ResolutionAmbiguousImplementation)
		if result.DispatchSite == nil || len(result.DispatchSite.Candidates) != 2 {
			t.Fatalf("receiver %q dispatch site = %+v", receiver, result.DispatchSite)
		}
	}
}

func TestBoundMethodReferenceUsesPolymorphicDispatch(t *testing.T) {
	contractMethod := overloadMethod("run", false, "java.lang.String")
	contract := &java.TypeDecl{Kind: java.TypeKindInterface, Name: "Service", FQCN: "contract.Service", Methods: []java.MethodDecl{contractMethod}}
	first := mkType("app.First", overloadMethod("run", false, "java.lang.String"))
	first.Interfaces = []java.TypeRef{ref("contract.Service")}
	second := mkType("app.Second", overloadMethod("run", false, "java.lang.String"))
	second.Interfaces = []java.TypeRef{ref("contract.Service")}
	r := newTestResolver([]*java.TypeDecl{contract, first, second})
	ctx := MethodContext{LocalVars: []java.LocalVarDecl{localVar("service", "contract.Service")}}

	result := r.Resolve(java.CallSite{Kind: java.CallMethodReference, Receiver: "service", MethodName: "run"}, ctx)
	assertTerminalKind(t, result, ResolutionAmbiguousImplementation)
	if result.DispatchSite == nil || len(result.DispatchSite.Candidates) != 2 {
		t.Fatalf("dispatch site = %+v", result.DispatchSite)
	}
}

func TestThisMethodReferenceUsesKnownConcreteRuntime(t *testing.T) {
	hook := mkMethod("hook")
	hook.Modifier = []string{"protected", "abstract"}
	base := mkType("base.Base", hook)
	base.Modifier = []string{"abstract"}
	first := mkType("app.First", mkMethod("hook"))
	first.SuperClass = ref("base.Base")
	second := mkType("app.Second", mkMethod("hook"))
	second.SuperClass = ref("base.Base")
	r := newTestResolver([]*java.TypeDecl{base, first, second})
	ctx := MethodContext{
		EnclosingType: r.Index.TypesByFQCN[base.FQCN],
		Execution:     ExecutionKey{RuntimeTypeFQCN: first.FQCN},
	}

	result := r.Resolve(java.CallSite{Kind: java.CallMethodReference, Receiver: "this", MethodName: "hook"}, ctx)
	want := ExecutionKey{Method: MethodHandle{TypeFQCN: first.FQCN, Method: "hook", Signature: "()"}, RuntimeTypeFQCN: first.FQCN}
	if result.DispatchSite != nil || len(result.Targets) != 1 || result.Targets[0].Key != want {
		t.Fatalf("this method reference = %+v, want %+v", result, want)
	}
}

func TestParameterizedDefaultMethodReferenceUsesLexicalSelection(t *testing.T) {
	method := overloadMethod("run", false, "java.lang.String")
	method.Modifier = append(method.Modifier, "default")
	contract := &java.TypeDecl{
		Kind: java.TypeKindInterface, Name: "Service", FQCN: "contract.Service",
		Methods: []java.MethodDecl{method},
	}
	r := newTestResolver([]*java.TypeDecl{contract})
	ctx := MethodContext{LocalVars: []java.LocalVarDecl{localVar("service", contract.FQCN)}}

	result := r.Resolve(java.CallSite{Kind: java.CallMethodReference, Receiver: "service", MethodName: "run"}, ctx)
	want := ExecutionKey{Method: MethodHandle{TypeFQCN: contract.FQCN, Method: "run", Signature: "(java.lang.String)"}, RuntimeTypeFQCN: contract.FQCN}
	if len(result.Targets) != 1 || result.Targets[0].Kind != ResolutionConcrete || result.Targets[0].Key != want {
		t.Fatalf("default method reference = %+v, want %+v", result, want)
	}
}

func TestRuntimePropagationForSuperStaticAndConstructors(t *testing.T) {
	parent := mkType("base.Parent", mkMethod("run"), java.MethodDecl{Name: "util", Signature: "()", Modifier: []string{"static"}}, mkConstructor([]string{"public"}))
	child := mkType("app.Child", mkConstructor([]string{"public"}))
	child.SuperClass = ref("base.Parent")
	r := newTestResolver([]*java.TypeDecl{parent, child})
	child = r.Index.TypesByFQCN["app.Child"]
	ctx := MethodContext{EnclosingType: child, Execution: ExecutionKey{RuntimeTypeFQCN: child.FQCN}}

	superResult := r.Resolve(java.CallSite{Receiver: "super", MethodName: "run"}, ctx)
	if got := superResult.Targets[0].Key; got.Method.TypeFQCN != "base.Parent" || got.RuntimeTypeFQCN != "app.Child" {
		t.Fatalf("super target = %+v", got)
	}
	staticResult := r.Resolve(java.CallSite{Receiver: "base.Parent", MethodName: "util"}, ctx)
	if got := staticResult.Targets[0].Key; got.RuntimeTypeFQCN != "base.Parent" {
		t.Fatalf("static runtime = %+v", got)
	}

	target := java.NewTypeRef("app.Child", false)
	created := r.Resolve(java.CallSite{Kind: java.CallObjectCreation, MethodName: "<init>", TargetType: &target}, ctx)
	if got := created.Targets[0].Key.RuntimeTypeFQCN; got != "app.Child" {
		t.Fatalf("constructed runtime = %q", got)
	}
	for _, kind := range []java.CallKind{java.CallThisConstructor, java.CallSuperConstructor} {
		result := r.Resolve(java.CallSite{Kind: kind, MethodName: "<init>"}, ctx)
		if got := result.Targets[0].Key.RuntimeTypeFQCN; got != "app.Child" {
			t.Fatalf("%s runtime = %q", kind, got)
		}
	}
}

func TestMethodReferencesPropagateRuntimeByQualifier(t *testing.T) {
	parent := mkType("base.Parent", mkMethod("run"), java.MethodDecl{Name: "util", Signature: "()", Modifier: []string{"public", "static"}}, mkConstructor([]string{"public"}))
	child := mkType("app.Child", mkMethod("run"))
	child.SuperClass = ref("base.Parent")
	r := newTestResolver([]*java.TypeDecl{parent, child})
	child = r.Index.TypesByFQCN["app.Child"]
	ctx := MethodContext{EnclosingType: child, Execution: ExecutionKey{RuntimeTypeFQCN: "app.Child"}}

	thisRef := r.Resolve(java.CallSite{Kind: java.CallMethodReference, Receiver: "this", MethodName: "run"}, ctx)
	if got := thisRef.Targets[0].Key; got.Method.TypeFQCN != "app.Child" || got.RuntimeTypeFQCN != "app.Child" {
		t.Fatalf("this method reference = %+v", got)
	}
	superRef := r.Resolve(java.CallSite{Kind: java.CallMethodReference, Receiver: "super", MethodName: "run", ReferenceQualifier: java.ReferenceQualifierSuper}, ctx)
	if got := superRef.Targets[0].Key; got.Method.TypeFQCN != "base.Parent" || got.RuntimeTypeFQCN != "app.Child" {
		t.Fatalf("super method reference = %+v", got)
	}
	staticRef := r.Resolve(java.CallSite{Kind: java.CallMethodReference, Receiver: "base.Parent", MethodName: "util", ReferenceQualifier: java.ReferenceQualifierType}, ctx)
	if got := staticRef.Targets[0].Key.RuntimeTypeFQCN; got != "base.Parent" {
		t.Fatalf("static method reference runtime = %q", got)
	}
	target := java.NewTypeRef("app.Child", false)
	constructorRef := r.Resolve(java.CallSite{Kind: java.CallConstructorReference, MethodName: "<init>", TargetType: &target}, ctx)
	if got := constructorRef.Targets[0].Key.RuntimeTypeFQCN; got != "app.Child" {
		t.Fatalf("constructor reference runtime = %q", got)
	}
}

func TestMethodContextExecutionFallsBackToLexicalType(t *testing.T) {
	typ := mkType("app.Service", mkMethod("run"))
	r := newTestResolver([]*java.TypeDecl{typ})
	typ = r.Index.TypesByFQCN[typ.FQCN]
	result := r.Resolve(java.CallSite{Receiver: "this", MethodName: "run"}, MethodContext{EnclosingType: typ})
	if got := result.Targets[0].Key.RuntimeTypeFQCN; got != typ.FQCN {
		t.Fatalf("runtime fallback = %q, want %q", got, typ.FQCN)
	}
}
