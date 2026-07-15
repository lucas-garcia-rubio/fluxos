package resolve

import (
	"testing"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
)

// Helpers pra construir fixtures in-memory.
func mkType(fqcn string, methods ...java.MethodDecl) *java.TypeDecl {
	return &java.TypeDecl{
		Kind:    java.TypeKindClass,
		Name:    fqcn,
		FQCN:    fqcn,
		Methods: methods,
	}
}

func mkMethod(name string) java.MethodDecl {
	return java.MethodDecl{Name: name}
}

func mkCall(receiver, methodName string) java.CallSite {
	return java.CallSite{Receiver: receiver, MethodName: methodName}
}

// Teste 1: this.foo() com foo existente no enclosing type.
func TestResolveThisMethodExists(t *testing.T) {
	r := NewSyntacticResolver(nil)
	enclosing := mkType("User", mkMethod("foo"))
	ctx := MethodContext{EnclosingType: enclosing}

	res := r.Resolve(mkCall("this", "foo"), ctx)

	if len(res.Targets) != 1 {
		t.Fatalf("expected 1 target, got %d (note: %q)", len(res.Targets), res.Note)
	}
	want := MethodHandle{TypeFQCN: "User", Method: "foo"}
	if res.Targets[0] != want {
		t.Errorf("target mismatch: got %+v, want %+v", res.Targets[0], want)
	}
}

// Teste 2: this.foo() sem foo no enclosing type.
func TestResolveThisMethodMissing(t *testing.T) {
	r := NewSyntacticResolver(nil)
	enclosing := mkType("User", mkMethod("bar")) // sem foo
	ctx := MethodContext{EnclosingType: enclosing}

	res := r.Resolve(mkCall("this", "foo"), ctx)

	if len(res.Targets) != 0 {
		t.Errorf("expected 0 targets, got %d", len(res.Targets))
	}
	if res.Note == "" {
		t.Error("expected non-empty Note explaining missing method")
	}
}

// Teste 3: foo() (unqualified) — mesmo caminho que this.foo().
func TestResolveUnqualifiedMethodExists(t *testing.T) {
	r := NewSyntacticResolver(nil)
	enclosing := mkType("User", mkMethod("foo"))
	ctx := MethodContext{EnclosingType: enclosing}

	res := r.Resolve(mkCall("", "foo"), ctx)

	if len(res.Targets) != 1 {
		t.Fatalf("expected 1 target, got %d (note: %q)", len(res.Targets), res.Note)
	}
	want := MethodHandle{TypeFQCN: "User", Method: "foo"}
	if res.Targets[0] != want {
		t.Errorf("target mismatch: got %+v, want %+v", res.Targets[0], want)
	}
}

// Teste 4: identifier desconhecido continua unresolved.
func TestResolveUnknownIdentifier(t *testing.T) {
	r := NewSyntacticResolver(nil)
	enclosing := mkType("User", mkMethod("foo"))
	ctx := MethodContext{EnclosingType: enclosing}

	res := r.Resolve(mkCall("userService", "create"), ctx)

	if len(res.Targets) != 0 {
		t.Errorf("expected 0 targets for identifier receiver, got %d", len(res.Targets))
	}
	if res.Note == "" {
		t.Error("expected Note explaining receiver not handled")
	}
}

// Teste 5: super.foo() — deferido para o Passo 8.
func TestResolveSuperNotHandled(t *testing.T) {
	r := NewSyntacticResolver(nil)
	enclosing := mkType("User", mkMethod("foo"))
	ctx := MethodContext{EnclosingType: enclosing}

	res := r.Resolve(mkCall("super", "foo"), ctx)

	if len(res.Targets) != 0 {
		t.Errorf("expected 0 targets for super in Passo 6, got %d", len(res.Targets))
	}
	if res.Note == "" {
		t.Error("expected Note explaining super not handled yet")
	}
}

// Teste 6: complex receiver (System.out) continua unresolved.
func TestResolveComplexReceiverNotHandled(t *testing.T) {
	r := NewSyntacticResolver(nil)
	enclosing := mkType("User", mkMethod("foo"))
	ctx := MethodContext{EnclosingType: enclosing}

	res := r.Resolve(mkCall("System.out", "println"), ctx)

	if len(res.Targets) != 0 {
		t.Errorf("expected 0 targets for complex receiver, got %d", len(res.Targets))
	}
	if res.Note == "" {
		t.Error("expected Note explaining complex receiver not handled")
	}
}

// Teste 7: enclosing type nil — proteção contra contexto malformado.
func TestResolveNilEnclosingType(t *testing.T) {
	r := NewSyntacticResolver(nil)
	ctx := MethodContext{EnclosingType: nil}

	res := r.Resolve(mkCall("this", "foo"), ctx)

	if len(res.Targets) != 0 {
		t.Errorf("expected 0 targets when enclosing type is nil, got %d", len(res.Targets))
	}
	if res.Note == "" {
		t.Error("expected Note explaining nil enclosing type")
	}
}

func TestResolveFieldMethod(t *testing.T) {
	helper := mkType("Helper", mkMethod("log"))
	enclosing := mkType("User")
	enclosing.Fields = []java.FieldDecl{{Name: "helper", Type: "Helper"}}
	r := NewSyntacticResolver([]*java.TypeDecl{enclosing, helper})

	res := r.Resolve(mkCall("helper", "log"), MethodContext{EnclosingType: enclosing})

	want := MethodHandle{TypeFQCN: "Helper", Method: "log"}
	if len(res.Targets) != 1 || res.Targets[0] != want {
		t.Fatalf("got targets %+v (note: %q), want [%+v]", res.Targets, res.Note, want)
	}
}

func TestResolveFieldWithExternalType(t *testing.T) {
	enclosing := mkType("User")
	enclosing.Fields = []java.FieldDecl{{Name: "client", Type: "ExternalClient"}}
	r := NewSyntacticResolver([]*java.TypeDecl{enclosing})

	res := r.Resolve(mkCall("client", "send"), MethodContext{EnclosingType: enclosing})

	if len(res.Targets) != 0 || res.Note == "" {
		t.Fatalf("expected unresolved field type, got %+v", res)
	}
}

func TestResolveLocalVarMethod(t *testing.T) {
	helper := mkType("Helper", mkMethod("log"))
	r := NewSyntacticResolver([]*java.TypeDecl{helper})
	ctx := MethodContext{LocalVars: map[string]string{"helper": "Helper"}}

	res := r.Resolve(mkCall("helper", "log"), ctx)

	want := MethodHandle{TypeFQCN: "Helper", Method: "log"}
	if len(res.Targets) != 1 || res.Targets[0] != want {
		t.Fatalf("got targets %+v (note: %q), want [%+v]", res.Targets, res.Note, want)
	}
}

func TestResolveLocalVarTakesPrecedenceOverField(t *testing.T) {
	localType := mkType("LocalHelper", mkMethod("run"))
	fieldType := mkType("FieldHelper", mkMethod("run"))
	enclosing := mkType("User")
	enclosing.Fields = []java.FieldDecl{{Name: "helper", Type: "FieldHelper"}}
	r := NewSyntacticResolver([]*java.TypeDecl{enclosing, localType, fieldType})
	ctx := MethodContext{
		EnclosingType: enclosing,
		LocalVars:     map[string]string{"helper": "LocalHelper"},
	}

	res := r.Resolve(mkCall("helper", "run"), ctx)

	want := MethodHandle{TypeFQCN: "LocalHelper", Method: "run"}
	if len(res.Targets) != 1 || res.Targets[0] != want {
		t.Fatalf("got targets %+v (note: %q), want local target %+v", res.Targets, res.Note, want)
	}
}

func TestResolveLocalVarMethodMissing(t *testing.T) {
	helper := mkType("Helper", mkMethod("other"))
	r := NewSyntacticResolver([]*java.TypeDecl{helper})
	ctx := MethodContext{LocalVars: map[string]string{"helper": "Helper"}}

	res := r.Resolve(mkCall("helper", "log"), ctx)

	if len(res.Targets) != 0 || res.Note == "" {
		t.Fatalf("expected unresolved missing method, got %+v", res)
	}
}
