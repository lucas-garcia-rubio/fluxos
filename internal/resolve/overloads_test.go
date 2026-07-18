package resolve

import (
	"strings"
	"testing"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
	"github.com/lucas-garcia-rubio/fluxos/internal/index"
)

func overloadCandidate(owner *java.TypeDecl, method *java.MethodDecl) index.MethodResolution {
	return index.MethodResolution{DeclaringType: owner, Method: method}
}

func overloadMethod(name string, variadic bool, parameterTypes ...string) java.MethodDecl {
	params := make([]java.Param, len(parameterTypes))
	for i, typeName := range parameterTypes {
		isVariadic := variadic && i == len(parameterTypes)-1
		params[i] = java.Param{Type: java.NewTypeRef(typeName, isVariadic), Variadic: isVariadic}
	}
	method := java.MethodDecl{Name: name, Params: params}
	java.RebuildSignature(&method)
	return method
}

func TestInferArgumentTypeLiterals(t *testing.T) {
	r := newTestResolver([]*java.TypeDecl{mkType("Caller")})
	ctx := MethodContext{EnclosingType: r.Index.TypesByFQCN["Caller"]}
	cases := []struct {
		source string
		want   string
	}{
		{source: `"text"`, want: "java.lang.String"},
		{source: `'x'`, want: "char"},
		{source: "true", want: "boolean"},
		{source: "1", want: "int"},
		{source: "1L", want: "long"},
		{source: "1.0", want: "double"},
		{source: "1.0f", want: "float"},
	}
	for _, tt := range cases {
		t.Run(tt.source, func(t *testing.T) {
			got := r.inferArgumentType(tt.source, java.CallSite{}, ctx)
			if !got.known || got.ref.SignatureToken() != tt.want {
				t.Fatalf("inferArgumentType(%q) = %+v, want %q", tt.source, got, tt.want)
			}
		})
	}
}

func TestInferArgumentTypeObjectCreationThisAndCast(t *testing.T) {
	value := mkType("model.Value")
	caller := mkType("app.Caller")
	caller.File = "Caller.java"
	r := newTestResolver([]*java.TypeDecl{value, caller})
	ctx := MethodContext{EnclosingType: caller, File: caller.File}
	for source, want := range map[string]string{
		"new model.Value()": "model.Value",
		"this":              "app.Caller",
		"(model.Value) x":   "model.Value",
	} {
		got := r.inferArgumentType(source, java.CallSite{}, ctx)
		if !got.known || got.ref.SignatureToken() != want {
			t.Errorf("inferArgumentType(%q) = %+v, want %q", source, got, want)
		}
	}
}

func TestInferArgumentTypeLexicalIdentifiers(t *testing.T) {
	stringRef := ref("java.lang.String")
	caller := mkType("Caller")
	caller.Fields = []java.FieldDecl{{Name: "field", Type: stringRef}}
	r := newTestResolver([]*java.TypeDecl{caller})
	ctx := MethodContext{
		EnclosingType: caller,
		Params:        []java.Param{{Name: "parameter", Type: stringRef}},
		LocalVars:     []java.LocalVarDecl{localVar("local", "java.lang.String")},
	}
	for _, source := range []string{"field", "parameter", "local"} {
		got := r.inferArgumentType(source, java.CallSite{StartByte: 1}, ctx)
		if !got.known || got.ref.SignatureToken() != "java.lang.String" {
			t.Errorf("inferArgumentType(%q) = %+v", source, got)
		}
	}
}

func TestSelectOverloadByLiteralAndObjectType(t *testing.T) {
	request := mkType("model.Request")
	other := mkType("model.Other")
	owner := mkType("Service",
		overloadMethod("run", false, "java.lang.String"),
		overloadMethod("run", false, "int"),
		overloadMethod("accept", false, "model.Request"),
		overloadMethod("accept", false, "model.Other"),
	)
	r := newTestResolver([]*java.TypeDecl{request, other, owner})
	owner = r.Index.TypesByFQCN["Service"]
	cases := []struct {
		call java.CallSite
		want string
	}{
		{call: java.CallSite{MethodName: "run", Args: []string{`"x"`}, ArgCount: 1}, want: "(java.lang.String)"},
		{call: java.CallSite{MethodName: "run", Args: []string{"1"}, ArgCount: 1}, want: "(int)"},
		{call: java.CallSite{MethodName: "accept", Args: []string{"new model.Request()"}, ArgCount: 1}, want: "(model.Request)"},
	}
	for _, tt := range cases {
		selection := r.selectMethodCandidates(r.methodCandidatesOnType(owner, tt.call.MethodName), tt.call, owner, MethodContext{EnclosingType: owner})
		if !selection.Found || len(selection.Resolution.Targets) != 1 || selection.Resolution.Targets[0].Handle.Signature != tt.want {
			t.Fatalf("select %s(%v) = %+v, want %s", tt.call.MethodName, tt.call.Args, selection, tt.want)
		}
	}
}

func TestSelectOverloadByProjectSubtype(t *testing.T) {
	contract := &java.TypeDecl{Kind: java.TypeKindInterface, Name: "Contract", FQCN: "Contract"}
	impl := mkType("Impl")
	impl.Interfaces = []java.TypeRef{ref("Contract")}
	other := mkType("Other")
	owner := mkType("Service", overloadMethod("run", false, "Contract"), overloadMethod("run", false, "Other"))
	r := newTestResolver([]*java.TypeDecl{contract, impl, other, owner})
	owner = r.Index.TypesByFQCN["Service"]
	call := java.CallSite{MethodName: "run", Args: []string{"new Impl()"}, ArgCount: 1}
	selection := r.selectMethodCandidates(r.methodCandidatesOnType(owner, "run"), call, owner, MethodContext{EnclosingType: owner})
	if len(selection.Resolution.Targets) != 1 || selection.Resolution.Targets[0].Handle.Signature != "(Contract)" {
		t.Fatalf("subtype selection = %+v", selection)
	}
}

func TestSelectOverloadNullAndUnknownPreserveAmbiguity(t *testing.T) {
	request := mkType("Request")
	other := mkType("Other")
	owner := mkType("Service", overloadMethod("run", false, "Request"), overloadMethod("run", false, "Other"))
	r := newTestResolver([]*java.TypeDecl{request, other, owner})
	owner = r.Index.TypesByFQCN["Service"]
	for _, source := range []string{"null", "factory()"} {
		call := java.CallSite{MethodName: "run", Args: []string{source}, ArgCount: 1}
		selection := r.selectMethodCandidates(r.methodCandidatesOnType(owner, "run"), call, owner, MethodContext{EnclosingType: owner})
		assertTerminalKind(t, selection.Resolution, ResolutionAmbiguousOverload)
	}
}

func TestSelectOverloadNullRejectsPrimitive(t *testing.T) {
	request := mkType("Request")
	owner := mkType("Service", overloadMethod("run", false, "Request"), overloadMethod("run", false, "int"))
	r := newTestResolver([]*java.TypeDecl{request, owner})
	owner = r.Index.TypesByFQCN["Service"]
	call := java.CallSite{MethodName: "run", Args: []string{"null"}, ArgCount: 1}
	selection := r.selectMethodCandidates(r.methodCandidatesOnType(owner, "run"), call, owner, MethodContext{EnclosingType: owner})
	if len(selection.Resolution.Targets) != 1 || selection.Resolution.Targets[0].Handle.Signature != "(Request)" {
		t.Fatalf("null selection = %+v", selection)
	}
}

func TestSelectOverloadDoesNotApplyBoxingOrWidening(t *testing.T) {
	owner := mkType("Service", overloadMethod("run", false, "long"), overloadMethod("run", false, "java.lang.Integer"))
	r := newTestResolver([]*java.TypeDecl{owner})
	owner = r.Index.TypesByFQCN["Service"]
	call := java.CallSite{MethodName: "run", Args: []string{"1"}, ArgCount: 1}
	result := r.resolveOnType(owner, call, MethodContext{EnclosingType: owner})
	assertTerminalKind(t, result, ResolutionUnresolved)
}

func TestSingleCandidateStillChecksArgumentCompatibility(t *testing.T) {
	owner := mkType("Service", overloadMethod("run", false, "java.lang.String"))
	r := newTestResolver([]*java.TypeDecl{owner})
	owner = r.Index.TypesByFQCN["Service"]
	call := java.CallSite{MethodName: "run", Args: []string{"1"}, ArgCount: 1}
	result := r.resolveOnType(owner, call, MethodContext{EnclosingType: owner})
	assertTerminalKind(t, result, ResolutionUnresolved)
}

func TestInferArgumentTypeRequiresWholeExpression(t *testing.T) {
	request := mkType("Request")
	caller := mkType("Caller")
	r := newTestResolver([]*java.TypeDecl{request, caller})
	ctx := MethodContext{EnclosingType: caller}
	for _, source := range []string{`"a" == "b"`, "new Request() == other", "(Request)", "(int) 1 == 2", "(char) 'a' + 1"} {
		if got := r.inferArgumentType(source, java.CallSite{}, ctx); got.known {
			t.Errorf("inferArgumentType(%q) = %+v, want unknown", source, got)
		}
	}
}

func TestSelectOverloadVarargsUsesElementType(t *testing.T) {
	owner := mkType("Service",
		overloadMethod("run", true, "java.lang.String"),
		overloadMethod("run", true, "int"),
	)
	r := newTestResolver([]*java.TypeDecl{owner})
	owner = r.Index.TypesByFQCN["Service"]
	call := java.CallSite{MethodName: "run", Args: []string{`"a"`, `"b"`}, ArgCount: 2}
	selection := r.selectMethodCandidates(r.methodCandidatesOnType(owner, "run"), call, owner, MethodContext{EnclosingType: owner})
	if len(selection.Resolution.Targets) != 1 || selection.Resolution.Targets[0].Handle.Signature != "(java.lang.String[])" {
		t.Fatalf("varargs selection = %+v", selection)
	}
}

func TestSelectConstructorOverloadByArgumentType(t *testing.T) {
	caller := mkType("app.Caller")
	caller.Name, caller.File = "Caller", "Caller.java"
	value := mkType("app.Value",
		mkConstructor([]string{"public"}, "java.lang.String"),
		mkConstructor([]string{"public"}, "int"),
	)
	value.Name, value.File = "Value", "Value.java"
	r := newResolverFromUnits(t, &java.CompilationUnit{File: caller.File, Package: "app", Types: []*java.TypeDecl{caller, value}})
	target := java.NewTypeRef("Value", false)
	call := java.CallSite{Kind: java.CallObjectCreation, MethodName: "<init>", TargetType: &target, Args: []string{`"value"`}, ArgCount: 1}
	result := r.Resolve(call, MethodContext{EnclosingType: caller, File: caller.File})
	if len(result.Targets) != 1 || result.Targets[0].Kind != ResolutionConcrete || result.Targets[0].Handle.Signature != "(java.lang.String)" {
		t.Fatalf("constructor overload = %+v", result)
	}
}

func TestExternalTypeWithoutClasspathBecomesUnresolvedTerminal(t *testing.T) {
	caller := mkType("app.Caller")
	caller.Name, caller.File = "Caller", "Caller.java"
	r := newResolverFromUnits(t, &java.CompilationUnit{File: caller.File, Package: "app", Types: []*java.TypeDecl{caller}})
	ctx := MethodContext{EnclosingType: caller, File: caller.File, Params: []java.Param{{Name: "value", Type: ref("String")}}}
	result := r.Resolve(java.CallSite{Receiver: "value", MethodName: "isEmpty"}, ctx)
	assertTerminalKind(t, result, ResolutionUnresolved)
	if result.Targets[0].Note != `param type "String" unresolved: external type java.lang.String` {
		t.Fatalf("external note = %q", result.Targets[0].Note)
	}
}

func TestExternalSuperclassCallsBecomeUnresolvedTerminals(t *testing.T) {
	caller := mkType("app.Caller")
	caller.Name, caller.File = "Caller", "Caller.java"
	caller.SuperClass = ref("java.util.AbstractList")
	r := newResolverFromUnits(t, &java.CompilationUnit{File: caller.File, Package: "app", Types: []*java.TypeDecl{caller}})
	ctx := MethodContext{EnclosingType: caller, File: caller.File}
	calls := []java.CallSite{
		{Receiver: "super", MethodName: "size"},
		{Kind: java.CallSuperConstructor, MethodName: "<init>"},
		{Kind: java.CallMethodReference, Receiver: "super", MethodName: "size", ReferenceQualifier: java.ReferenceQualifierSuper},
	}
	for _, call := range calls {
		result := r.Resolve(call, ctx)
		assertTerminalKind(t, result, ResolutionUnresolved)
		if !strings.HasPrefix(result.Targets[0].Handle.TypeFQCN, "java.util.AbstractList#unresolved#") {
			t.Fatalf("external super owner = %q", result.Targets[0].Handle.TypeFQCN)
		}
	}
}

func TestAmbiguousSuperclassCallsBecomeAmbiguousTypeTerminals(t *testing.T) {
	left := mkType("left.Parent")
	left.Name, left.File = "Parent", "Left.java"
	right := mkType("right.Parent")
	right.Name, right.File = "Parent", "Right.java"
	caller := mkType("app.Caller")
	caller.Name, caller.File = "Caller", "Caller.java"
	caller.SuperClass = ref("Parent")
	r := newResolverFromUnits(t,
		&java.CompilationUnit{File: left.File, Package: "left", Types: []*java.TypeDecl{left}},
		&java.CompilationUnit{File: right.File, Package: "right", Types: []*java.TypeDecl{right}},
		&java.CompilationUnit{File: caller.File, Package: "app", Imports: []java.ImportDecl{{Target: "left", Wildcard: true}, {Target: "right", Wildcard: true}}, Types: []*java.TypeDecl{caller}},
	)
	ctx := MethodContext{EnclosingType: caller, File: caller.File}
	for _, call := range []java.CallSite{
		{Receiver: "super", MethodName: "run"},
		{Kind: java.CallSuperConstructor, MethodName: "<init>"},
		{Kind: java.CallMethodReference, Receiver: "super", MethodName: "run", ReferenceQualifier: java.ReferenceQualifierSuper},
	} {
		assertTerminalKind(t, r.Resolve(call, ctx), ResolutionAmbiguousType)
	}
}
