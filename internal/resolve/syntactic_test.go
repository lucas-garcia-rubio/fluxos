package resolve

import (
	"strings"
	"testing"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
	"github.com/lucas-garcia-rubio/fluxos/internal/index"
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
	return java.MethodDecl{Name: name, Signature: "()"}
}

func mkCall(receiver, methodName string) java.CallSite {
	return java.CallSite{Receiver: receiver, MethodName: methodName}
}

func ref(raw string) java.TypeRef {
	return java.NewTypeRef(raw, false)
}

func newTestResolver(types []*java.TypeDecl) *SyntacticResolver {
	unitsByFile := make(map[string]*java.CompilationUnit)
	for _, typ := range types {
		unit := unitsByFile[typ.File]
		if unit == nil {
			unit = &java.CompilationUnit{File: typ.File, Types: make([]*java.TypeDecl, 0)}
			unitsByFile[typ.File] = unit
		}
		unit.Types = append(unit.Types, typ)
	}
	units := make([]*java.CompilationUnit, 0, len(unitsByFile))
	for _, unit := range unitsByFile {
		units = append(units, unit)
	}
	table, err := index.Build(units)
	if err != nil {
		panic(err)
	}
	return NewSyntacticResolver(table)
}

// Teste 1: this.foo() com foo existente no enclosing type.
func TestResolveThisMethodExists(t *testing.T) {
	r := newTestResolver(nil)
	enclosing := mkType("User", mkMethod("foo"))
	ctx := MethodContext{EnclosingType: enclosing}

	res := r.Resolve(mkCall("this", "foo"), ctx)

	if len(res.Targets) != 1 {
		t.Fatalf("expected 1 target, got %d (note: %q)", len(res.Targets), res.Note)
	}
	want := MethodHandle{TypeFQCN: "User", Method: "foo", Signature: "()"}
	if res.Targets[0] != want {
		t.Errorf("target mismatch: got %+v, want %+v", res.Targets[0], want)
	}
}

// Teste 2: this.foo() sem foo no enclosing type.
func TestResolveThisMethodMissing(t *testing.T) {
	r := newTestResolver(nil)
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
	r := newTestResolver(nil)
	enclosing := mkType("User", mkMethod("foo"))
	ctx := MethodContext{EnclosingType: enclosing}

	res := r.Resolve(mkCall("", "foo"), ctx)

	if len(res.Targets) != 1 {
		t.Fatalf("expected 1 target, got %d (note: %q)", len(res.Targets), res.Note)
	}
	want := MethodHandle{TypeFQCN: "User", Method: "foo", Signature: "()"}
	if res.Targets[0] != want {
		t.Errorf("target mismatch: got %+v, want %+v", res.Targets[0], want)
	}
}

// Teste 4: identifier desconhecido continua unresolved.
func TestResolveUnknownIdentifier(t *testing.T) {
	r := newTestResolver(nil)
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

// Teste 5: super.foo() sem superclass continua unresolved.
func TestResolveSuperWithoutSuperclass(t *testing.T) {
	r := newTestResolver(nil)
	enclosing := mkType("User", mkMethod("foo"))
	ctx := MethodContext{EnclosingType: enclosing}

	res := r.Resolve(mkCall("super", "foo"), ctx)

	if len(res.Targets) != 0 {
		t.Errorf("expected 0 targets for type without superclass, got %d", len(res.Targets))
	}
	if res.Note == "" {
		t.Error("expected Note explaining missing superclass")
	}
}

// Teste 6: complex receiver (System.out) continua unresolved.
func TestResolveComplexReceiverNotHandled(t *testing.T) {
	r := newTestResolver(nil)
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
	r := newTestResolver(nil)
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
	enclosing.Fields = []java.FieldDecl{{Name: "helper", Type: ref("Helper")}}
	r := newTestResolver([]*java.TypeDecl{enclosing, helper})

	res := r.Resolve(mkCall("helper", "log"), MethodContext{EnclosingType: enclosing})

	want := MethodHandle{TypeFQCN: "Helper", Method: "log", Signature: "()"}
	if len(res.Targets) != 1 || res.Targets[0] != want {
		t.Fatalf("got targets %+v (note: %q), want [%+v]", res.Targets, res.Note, want)
	}
}

func TestResolveFieldWithExternalType(t *testing.T) {
	enclosing := mkType("User")
	enclosing.Fields = []java.FieldDecl{{Name: "client", Type: ref("ExternalClient")}}
	r := newTestResolver([]*java.TypeDecl{enclosing})

	res := r.Resolve(mkCall("client", "send"), MethodContext{EnclosingType: enclosing})

	if len(res.Targets) != 0 || res.Note == "" {
		t.Fatalf("expected unresolved field type, got %+v", res)
	}
}

func TestResolveLocalVarMethod(t *testing.T) {
	helper := mkType("Helper", mkMethod("log"))
	r := newTestResolver([]*java.TypeDecl{helper})
	ctx := MethodContext{LocalVars: map[string]java.TypeRef{"helper": ref("Helper")}}

	res := r.Resolve(mkCall("helper", "log"), ctx)

	want := MethodHandle{TypeFQCN: "Helper", Method: "log", Signature: "()"}
	if len(res.Targets) != 1 || res.Targets[0] != want {
		t.Fatalf("got targets %+v (note: %q), want [%+v]", res.Targets, res.Note, want)
	}
}

func TestResolveLocalVarTakesPrecedenceOverField(t *testing.T) {
	localType := mkType("LocalHelper", mkMethod("run"))
	fieldType := mkType("FieldHelper", mkMethod("run"))
	enclosing := mkType("User")
	enclosing.Fields = []java.FieldDecl{{Name: "helper", Type: ref("FieldHelper")}}
	r := newTestResolver([]*java.TypeDecl{enclosing, localType, fieldType})
	ctx := MethodContext{
		EnclosingType: enclosing,
		LocalVars:     map[string]java.TypeRef{"helper": ref("LocalHelper")},
	}

	res := r.Resolve(mkCall("helper", "run"), ctx)

	want := MethodHandle{TypeFQCN: "LocalHelper", Method: "run", Signature: "()"}
	if len(res.Targets) != 1 || res.Targets[0] != want {
		t.Fatalf("got targets %+v (note: %q), want local target %+v", res.Targets, res.Note, want)
	}
}

func TestResolveLocalVarMethodMissing(t *testing.T) {
	helper := mkType("Helper", mkMethod("other"))
	r := newTestResolver([]*java.TypeDecl{helper})
	ctx := MethodContext{LocalVars: map[string]java.TypeRef{"helper": ref("Helper")}}

	res := r.Resolve(mkCall("helper", "log"), ctx)

	if len(res.Targets) != 0 || res.Note == "" {
		t.Fatalf("expected unresolved missing method, got %+v", res)
	}
}

func TestResolveSuperMethod(t *testing.T) {
	const file = "Example.java"
	base := mkType("Base", mkMethod("touch"))
	base.File = file
	child := mkType("Child")
	child.File = file
	child.SuperClass = ref("Base")
	r := newTestResolver([]*java.TypeDecl{child, base})
	ctx := MethodContext{EnclosingType: child, File: file}

	res := r.Resolve(mkCall("super", "touch"), ctx)

	want := MethodHandle{TypeFQCN: "Base", Method: "touch", Signature: "()"}
	if len(res.Targets) != 1 || res.Targets[0] != want {
		t.Fatalf("got targets %+v (note: %q), want [%+v]", res.Targets, res.Note, want)
	}
}

func TestResolveSuperWithoutEnclosingType(t *testing.T) {
	r := newTestResolver(nil)

	res := r.Resolve(mkCall("super", "touch"), MethodContext{})

	if len(res.Targets) != 0 || res.Note == "" {
		t.Fatalf("expected unresolved super without enclosing type, got %+v", res)
	}
}

func TestResolveSuperclassInOtherFile(t *testing.T) {
	base := mkType("Base", mkMethod("touch"))
	base.File = "Base.java"
	child := mkType("Child")
	child.File = "Child.java"
	child.SuperClass = ref("Base")
	r := newTestResolver([]*java.TypeDecl{child, base})
	ctx := MethodContext{EnclosingType: child, File: child.File}

	res := r.Resolve(mkCall("super", "touch"), ctx)

	if len(res.Targets) != 0 || res.Note == "" {
		t.Fatalf("expected cross-file superclass to remain unresolved, got %+v", res)
	}
}

func TestResolveSuperMethodMissing(t *testing.T) {
	const file = "Example.java"
	base := mkType("Base", mkMethod("other"))
	base.File = file
	child := mkType("Child")
	child.File = file
	child.SuperClass = ref("Base")
	r := newTestResolver([]*java.TypeDecl{child, base})
	ctx := MethodContext{EnclosingType: child, File: file}

	res := r.Resolve(mkCall("super", "touch"), ctx)

	if len(res.Targets) != 0 || res.Note == "" {
		t.Fatalf("expected missing superclass method to remain unresolved, got %+v", res)
	}
}

func TestResolveStaticMethodInSameFile(t *testing.T) {
	const file = "Example.java"
	caller := mkType("Caller")
	caller.File = file
	utils := mkType("Utils", mkMethod("log"))
	utils.File = file
	r := newTestResolver([]*java.TypeDecl{caller, utils})
	ctx := MethodContext{EnclosingType: caller, File: file}

	res := r.Resolve(mkCall("Utils", "log"), ctx)

	want := MethodHandle{TypeFQCN: "Utils", Method: "log", Signature: "()"}
	if len(res.Targets) != 1 || res.Targets[0] != want {
		t.Fatalf("got targets %+v (note: %q), want [%+v]", res.Targets, res.Note, want)
	}
}

func TestResolveStaticTypeInSamePackageOtherFile(t *testing.T) {
	caller := mkType("Caller")
	caller.File = "Caller.java"
	utils := mkType("Utils", mkMethod("log"))
	utils.File = "Utils.java"
	r := newTestResolver([]*java.TypeDecl{caller, utils})
	ctx := MethodContext{EnclosingType: caller, File: caller.File}

	res := r.Resolve(mkCall("Utils", "log"), ctx)

	want := MethodHandle{TypeFQCN: "Utils", Method: "log", Signature: "()"}
	if len(res.Targets) != 1 || res.Targets[0] != want {
		t.Fatalf("got targets %+v (note: %q), want [%+v]", res.Targets, res.Note, want)
	}
}

func TestResolveLocalVarTakesPrecedenceOverType(t *testing.T) {
	const file = "Example.java"
	localType := mkType("LocalHelper", mkMethod("run"))
	localType.File = file
	classType := mkType("helper", mkMethod("run"))
	classType.File = file
	r := newTestResolver([]*java.TypeDecl{localType, classType})
	ctx := MethodContext{
		File:      file,
		LocalVars: map[string]java.TypeRef{"helper": ref("LocalHelper")},
	}

	res := r.Resolve(mkCall("helper", "run"), ctx)

	want := MethodHandle{TypeFQCN: "LocalHelper", Method: "run", Signature: "()"}
	if len(res.Targets) != 1 || res.Targets[0] != want {
		t.Fatalf("got targets %+v (note: %q), want local target %+v", res.Targets, res.Note, want)
	}
}

func TestResolveFieldTakesPrecedenceOverType(t *testing.T) {
	const file = "Example.java"
	fieldType := mkType("FieldHelper", mkMethod("run"))
	fieldType.File = file
	classType := mkType("helper", mkMethod("run"))
	classType.File = file
	enclosing := mkType("Caller")
	enclosing.File = file
	enclosing.Fields = []java.FieldDecl{{Name: "helper", Type: ref("FieldHelper")}}
	r := newTestResolver([]*java.TypeDecl{enclosing, fieldType, classType})
	ctx := MethodContext{EnclosingType: enclosing, File: file}

	res := r.Resolve(mkCall("helper", "run"), ctx)

	want := MethodHandle{TypeFQCN: "FieldHelper", Method: "run", Signature: "()"}
	if len(res.Targets) != 1 || res.Targets[0] != want {
		t.Fatalf("got targets %+v (note: %q), want field target %+v", res.Targets, res.Note, want)
	}
}

func TestResolveQualifiedStaticReceiver(t *testing.T) {
	const file = "Example.java"
	utils := mkType("com.example.Utils", mkMethod("log"))
	utils.Name = "Utils"
	utils.File = file
	r := newTestResolver([]*java.TypeDecl{utils})

	res := r.Resolve(mkCall("com.example.Utils", "log"), MethodContext{File: file})

	want := MethodHandle{TypeFQCN: "com.example.Utils", Method: "log", Signature: "()"}
	if len(res.Targets) != 1 || res.Targets[0] != want {
		t.Fatalf("got targets %+v (note: %q), want [%+v]", res.Targets, res.Note, want)
	}
}

func TestResolveOverloadByArity(t *testing.T) {
	typ := mkType("Service",
		java.MethodDecl{Name: "run", Signature: "()"},
		java.MethodDecl{Name: "run", Signature: "(String)", Params: []java.Param{{Type: ref("String")}}},
	)
	r := newTestResolver([]*java.TypeDecl{typ})

	res := r.Resolve(java.CallSite{MethodName: "run", ArgCount: 1}, MethodContext{EnclosingType: typ})

	want := MethodHandle{TypeFQCN: "Service", Method: "run", Signature: "(java.lang.String)"}
	if len(res.Targets) != 1 || res.Targets[0] != want {
		t.Fatalf("targets = %+v (note: %q), want [%+v]", res.Targets, res.Note, want)
	}
}

func TestResolveAmbiguousOverload(t *testing.T) {
	typ := mkType("Service",
		java.MethodDecl{Name: "run", Signature: "(String)", Params: []java.Param{{Type: ref("String")}}},
		java.MethodDecl{Name: "run", Signature: "(int)", Params: []java.Param{{Type: ref("int")}}},
	)
	r := newTestResolver([]*java.TypeDecl{typ})

	res := r.Resolve(java.CallSite{MethodName: "run", ArgCount: 1}, MethodContext{EnclosingType: typ})

	if len(res.Targets) != 0 || !strings.Contains(res.Note, "ambiguous overload") {
		t.Fatalf("expected overload ambiguity, got %+v", res)
	}
	if !strings.Contains(res.Note, "(int), (java.lang.String)") {
		t.Fatalf("ambiguity note is not deterministically sorted: %q", res.Note)
	}
}

func TestResolveVariadicArity(t *testing.T) {
	typ := mkType("Logger", java.MethodDecl{
		Name:      "log",
		Signature: "(String[])",
		Params:    []java.Param{{Type: java.NewTypeRef("String", true), Variadic: true}},
	})
	r := newTestResolver([]*java.TypeDecl{typ})

	for _, argCount := range []int{0, 1, 3} {
		res := r.Resolve(java.CallSite{MethodName: "log", ArgCount: argCount}, MethodContext{EnclosingType: typ})
		if len(res.Targets) != 1 {
			t.Errorf("argCount %d: targets = %+v, note = %q", argCount, res.Targets, res.Note)
		}
	}
}

func TestResolveDoesNotChooseFirstDuplicateSimpleName(t *testing.T) {
	first := mkType("a.Helper", mkMethod("run"))
	first.Name = "Helper"
	second := mkType("b.Helper", mkMethod("run"))
	second.Name = "Helper"
	r := newTestResolver([]*java.TypeDecl{first, second})

	res := r.Resolve(mkCall("helper", "run"), MethodContext{LocalVars: map[string]java.TypeRef{"helper": ref("Helper")}})

	if len(res.Targets) != 0 || res.Note == "" {
		t.Fatalf("duplicate simple name must remain unresolved: %+v", res)
	}
}

func TestResolveReportsWildcardTypeAmbiguityDeterministically(t *testing.T) {
	caller := mkType("app.Caller")
	caller.Name = "Caller"
	caller.File = "Caller.java"
	caller.Fields = []java.FieldDecl{{Name: "helper", Type: ref("Helper")}}
	left := mkType("left.Helper", mkMethod("run"))
	left.Name = "Helper"
	left.File = "Left.java"
	right := mkType("right.Helper", mkMethod("run"))
	right.Name = "Helper"
	right.File = "Right.java"
	table, err := index.Build([]*java.CompilationUnit{
		{
			File:    caller.File,
			Package: "app",
			Imports: []java.ImportDecl{{Target: "right", Wildcard: true}, {Target: "left", Wildcard: true}},
			Types:   []*java.TypeDecl{caller},
		},
		{File: left.File, Package: "left", Types: []*java.TypeDecl{left}},
		{File: right.File, Package: "right", Types: []*java.TypeDecl{right}},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	res := NewSyntacticResolver(table).Resolve(mkCall("helper", "run"), MethodContext{EnclosingType: caller, File: caller.File})
	if len(res.Targets) != 0 || !strings.Contains(res.Note, "candidates: left.Helper, right.Helper") {
		t.Fatalf("expected deterministic type ambiguity, got %+v", res)
	}
}
