package resolve

import (
	"reflect"
	"testing"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
)

func TestDispatchSiteIDIsStableAndUsesCallerRuntime(t *testing.T) {
	caller := ExecutionKey{Method: MethodHandle{TypeFQCN: "app.Workflow", Method: "run", Signature: "()"}, RuntimeTypeFQCN: "app.First"}
	call := java.CallSite{Kind: java.CallInvocation, File: "src/main/java/app/Workflow.java", StartByte: 42, Args: []string{"x"}}
	candidates := []ImplementationCandidate{{ImplementationFQCN: "app.Second"}, {ImplementationFQCN: "app.First"}}
	first := NewDispatchSite(caller, "contract.Service", "execute", "()", call, candidates)
	absolute := call
	absolute.File = "/tmp/project/src/main/java/app/Workflow.java"
	second := NewDispatchSite(caller, "contract.Service", "execute", "()", absolute, []ImplementationCandidate{candidates[1], candidates[0]})
	if first.ID != second.ID {
		t.Fatalf("stable IDs differ: %q vs %q", first.ID, second.ID)
	}
	otherCaller := caller
	otherCaller.RuntimeTypeFQCN = "app.Second"
	if first.ID == NewDispatchSite(otherCaller, "contract.Service", "execute", "()", call, candidates).ID {
		t.Fatal("caller runtime must discriminate dispatch sites")
	}
	otherCall := call
	otherCall.StartByte++
	if first.ID == NewDispatchSite(caller, "contract.Service", "execute", "()", otherCall, candidates).ID {
		t.Fatal("call position must discriminate dispatch sites")
	}
	otherCall = call
	otherCall.Kind = java.CallMethodReference
	if first.ID == NewDispatchSite(caller, "contract.Service", "execute", "()", otherCall, candidates).ID {
		t.Fatal("call kind must discriminate dispatch sites")
	}
	if first.Candidates[0].ImplementationFQCN != "app.First" {
		t.Fatalf("candidates not sorted: %+v", first.Candidates)
	}
}

func TestCloneDispatchSiteDeepCopiesCallMetadata(t *testing.T) {
	target := java.NewTypeRef("app.Value", false)
	original := NewDispatchSite(ExecutionKey{RuntimeTypeFQCN: "app.Caller"}, "contract.Service", "run", "()",
		java.CallSite{Args: []string{"value"}, TargetType: &target},
		[]ImplementationCandidate{{ImplementationFQCN: "app.Impl"}})
	clone := CloneDispatchSite(original)
	clone.Call.Args[0] = "changed"
	clone.Call.TargetType.Raw = "changed.Type"
	clone.Candidates[0].ImplementationFQCN = "changed.Impl"
	if original.Call.Args[0] != "value" || original.Call.TargetType.Raw != "app.Value" || original.Candidates[0].ImplementationFQCN != "app.Impl" {
		t.Fatalf("clone mutated original: %+v", original)
	}
}

func TestMultipleImplementationsUseOneLexicalSignatureAndStructuredCandidates(t *testing.T) {
	contract := &java.TypeDecl{Kind: java.TypeKindInterface, Name: "Service", FQCN: "contract.Service", Methods: []java.MethodDecl{overloadMethod("run", false, "java.lang.String")}}
	base := mkType("base.Base", overloadMethod("run", false, "java.lang.String"))
	first := mkType("app.First")
	first.SuperClass = ref("base.Base")
	first.Interfaces = []java.TypeRef{ref("contract.Service")}
	second := mkType("app.Second")
	second.SuperClass = ref("base.Base")
	second.Interfaces = []java.TypeRef{ref("contract.Service")}
	r := newTestResolver([]*java.TypeDecl{second, contract, first, base})
	ctx := MethodContext{
		EnclosingType: r.Index.TypesByFQCN["base.Base"],
		Execution:     ExecutionKey{Method: MethodHandle{TypeFQCN: "base.Base", Method: "caller", Signature: "()"}, RuntimeTypeFQCN: "app.First"},
		LocalVars:     []java.LocalVarDecl{localVar("service", "contract.Service")},
	}
	result := r.Resolve(java.CallSite{Receiver: "service", MethodName: "run", Args: []string{`"x"`}, ArgCount: 1, File: "App.java", StartByte: 7}, ctx)
	assertTerminalKind(t, result, ResolutionAmbiguousImplementation)
	if result.DispatchSite == nil || result.DispatchSite.Signature != "(java.lang.String)" || len(result.DispatchSite.Candidates) != 2 {
		t.Fatalf("dispatch site = %+v", result.DispatchSite)
	}
	want := []ImplementationCandidate{
		{ImplementationFQCN: "app.First", Target: MethodHandle{TypeFQCN: "base.Base", Method: "run", Signature: "(java.lang.String)"}, Kind: ResolutionConcrete},
		{ImplementationFQCN: "app.Second", Target: MethodHandle{TypeFQCN: "base.Base", Method: "run", Signature: "(java.lang.String)"}, Kind: ResolutionConcrete},
	}
	if !reflect.DeepEqual(result.DispatchSite.Candidates, want) {
		t.Fatalf("candidates = %+v, want %+v", result.DispatchSite.Candidates, want)
	}
	if got := result.Targets[0].Key.RuntimeTypeFQCN; got != "app.First" {
		t.Fatalf("terminal runtime discriminator = %q", got)
	}
}

func TestNonVirtualMethodOnPolymorphicReceiverDoesNotCreateDispatchSite(t *testing.T) {
	base := mkType("base.Base", java.MethodDecl{Name: "run", Signature: "()", Modifier: []string{"final"}})
	base.Modifier = []string{"abstract"}
	first := mkType("app.First")
	first.SuperClass = ref("base.Base")
	second := mkType("app.Second")
	second.SuperClass = ref("base.Base")
	r := newTestResolver([]*java.TypeDecl{base, first, second})
	ctx := MethodContext{LocalVars: []java.LocalVarDecl{localVar("value", "base.Base")}}
	result := r.Resolve(java.CallSite{Receiver: "value", MethodName: "run", StartByte: 1}, ctx)
	if result.DispatchSite != nil || len(result.Targets) != 1 || result.Targets[0].Kind != ResolutionConcrete {
		t.Fatalf("non-virtual resolution = %+v", result)
	}
	if got := result.Targets[0].Key; got.Method.TypeFQCN != "base.Base" || got.RuntimeTypeFQCN != "base.Base" {
		t.Fatalf("non-virtual target = %+v", got)
	}
}
