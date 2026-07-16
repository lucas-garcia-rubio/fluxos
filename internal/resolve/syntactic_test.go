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

func mkStaticMethod(name string) java.MethodDecl {
	return java.MethodDecl{Name: name, Signature: "()", Modifier: []string{"public", "static"}}
}

func mkCall(receiver, methodName string) java.CallSite {
	return java.CallSite{Receiver: receiver, MethodName: methodName}
}

func ref(raw string) java.TypeRef {
	return java.NewTypeRef(raw, false)
}

// localVar cria uma LocalVarDecl visível em qualquer posição (escopo cobre todo
// o método e declaração precede qualquer call). Útil em testes onde não vale a
// pena construir byte ranges reais.
func localVar(name, typeName string) java.LocalVarDecl {
	return java.LocalVarDecl{
		Name:       name,
		Type:       ref(typeName),
		ScopeStart: 0,
		ScopeEnd:   ^uint(0),
		DeclStart:  0,
	}
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

func newResolverFromUnits(t *testing.T, units ...*java.CompilationUnit) *SyntacticResolver {
	t.Helper()
	table, err := index.Build(units)
	if err != nil {
		t.Fatalf("index.Build: %v", err)
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
	ctx := MethodContext{LocalVars: []java.LocalVarDecl{localVar("helper", "Helper")}}

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
		LocalVars:     []java.LocalVarDecl{localVar("helper", "LocalHelper")},
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
	ctx := MethodContext{LocalVars: []java.LocalVarDecl{localVar("helper", "Helper")}}

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

	want := MethodHandle{TypeFQCN: "Base", Method: "touch", Signature: "()"}
	if len(res.Targets) != 1 || res.Targets[0] != want {
		t.Fatalf("cross-file superclass target = %+v (note: %q), want %+v", res.Targets, res.Note, want)
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
	utils := mkType("Utils", mkStaticMethod("log"))
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
	utils := mkType("Utils", mkStaticMethod("log"))
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
		LocalVars: []java.LocalVarDecl{localVar("helper", "LocalHelper")},
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
	utils := mkType("com.example.Utils", mkStaticMethod("log"))
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

	res := r.Resolve(mkCall("helper", "run"), MethodContext{LocalVars: []java.LocalVarDecl{localVar("helper", "Helper")}})

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

func TestResolveInheritedMethodAndFieldCrossFile(t *testing.T) {
	helper := mkType("support.Helper", mkMethod("work"))
	helper.Name = "Helper"
	helper.File = "Helper.java"
	parent := mkType("base.Parent", java.MethodDecl{Name: "inherited", Signature: "()", Modifier: []string{"public"}})
	parent.Name = "Parent"
	parent.File = "Parent.java"
	parent.Fields = []java.FieldDecl{{Name: "helper", Modifier: []string{"protected"}, Type: ref("support.Helper")}}
	child := mkType("app.Child")
	child.Name = "Child"
	child.File = "Child.java"
	child.SuperClass = ref("base.Parent")
	r := newResolverFromUnits(t,
		&java.CompilationUnit{File: child.File, Package: "app", Types: []*java.TypeDecl{child}},
		&java.CompilationUnit{File: parent.File, Package: "base", Types: []*java.TypeDecl{parent}},
		&java.CompilationUnit{File: helper.File, Package: "support", Types: []*java.TypeDecl{helper}},
	)
	ctx := MethodContext{EnclosingType: child, File: child.File}

	method := r.Resolve(mkCall("", "inherited"), ctx)
	wantMethod := MethodHandle{TypeFQCN: "base.Parent", Method: "inherited", Signature: "()"}
	if len(method.Targets) != 1 || method.Targets[0] != wantMethod {
		t.Fatalf("inherited method = %+v (note: %q), want %+v", method.Targets, method.Note, wantMethod)
	}
	field := r.Resolve(mkCall("helper", "work"), ctx)
	wantField := MethodHandle{TypeFQCN: "support.Helper", Method: "work", Signature: "()"}
	if len(field.Targets) != 1 || field.Targets[0] != wantField {
		t.Fatalf("inherited field = %+v (note: %q), want %+v", field.Targets, field.Note, wantField)
	}
}

func TestResolveExplicitAndWildcardStaticImports(t *testing.T) {
	caller := mkType("app.Caller")
	caller.Name = "Caller"
	caller.File = "Caller.java"
	explicit := mkType("tasks.Explicit", mkStaticMethod("explicitRun"))
	explicit.Name = "Explicit"
	explicit.File = "Explicit.java"
	explicit.Modifier = []string{"public"}
	wildcard := mkType("tasks.Wildcard", mkStaticMethod("wildcardRun"))
	wildcard.Name = "Wildcard"
	wildcard.File = "Wildcard.java"
	wildcard.Modifier = []string{"public"}
	r := newResolverFromUnits(t,
		&java.CompilationUnit{File: caller.File, Package: "app", Imports: []java.ImportDecl{
			{Target: "tasks.Explicit.explicitRun", Static: true},
			{Target: "tasks.Wildcard", Static: true, Wildcard: true},
		}, Types: []*java.TypeDecl{caller}},
		&java.CompilationUnit{File: explicit.File, Package: "tasks", Types: []*java.TypeDecl{explicit}},
		&java.CompilationUnit{File: wildcard.File, Package: "tasks", Types: []*java.TypeDecl{wildcard}},
	)
	ctx := MethodContext{EnclosingType: caller, File: caller.File}

	for _, test := range []struct {
		name string
		want MethodHandle
	}{
		{name: "explicitRun", want: MethodHandle{TypeFQCN: "tasks.Explicit", Method: "explicitRun", Signature: "()"}},
		{name: "wildcardRun", want: MethodHandle{TypeFQCN: "tasks.Wildcard", Method: "wildcardRun", Signature: "()"}},
	} {
		res := r.Resolve(mkCall("", test.name), ctx)
		if len(res.Targets) != 1 || res.Targets[0] != test.want {
			t.Fatalf("%s targets = %+v (note: %q), want %+v", test.name, res.Targets, res.Note, test.want)
		}
	}
}

func TestResolveStaticImportsDeduplicateAndRejectPrivate(t *testing.T) {
	caller := mkType("app.Caller")
	caller.Name, caller.File = "Caller", "Caller.java"
	publicRun := mkStaticMethod("run")
	privateRun := mkStaticMethod("hidden")
	privateRun.Modifier = []string{"private", "static"}
	utils := mkType("tasks.Utils", publicRun, privateRun)
	utils.Name, utils.File = "Utils", "Utils.java"
	utils.Modifier = []string{"public"}
	r := newResolverFromUnits(t,
		&java.CompilationUnit{File: caller.File, Package: "app", Imports: []java.ImportDecl{
			{Target: "tasks.Utils.run", Static: true},
			{Target: "tasks.Utils.run", Static: true},
			{Target: "tasks.Utils.hidden", Static: true},
		}, Types: []*java.TypeDecl{caller}},
		&java.CompilationUnit{File: utils.File, Package: "tasks", Types: []*java.TypeDecl{utils}},
	)
	ctx := MethodContext{EnclosingType: caller, File: caller.File}

	if res := r.Resolve(mkCall("", "run"), ctx); len(res.Targets) != 1 || res.Targets[0].TypeFQCN != "tasks.Utils" {
		t.Fatalf("duplicate static import = %+v (note: %q)", res.Targets, res.Note)
	}
	if res := r.Resolve(mkCall("", "hidden"), ctx); len(res.Targets) != 0 {
		t.Fatalf("private static import resolved: %+v", res.Targets)
	}
}

func TestResolveStaticImportRejectsInaccessibleOwnerType(t *testing.T) {
	caller := mkType("app.Caller")
	caller.Name, caller.File = "Caller", "Caller.java"
	hidden := mkType("tasks.Hidden", mkStaticMethod("run"))
	hidden.Name, hidden.File = "Hidden", "Hidden.java"
	r := newResolverFromUnits(t,
		&java.CompilationUnit{File: caller.File, Package: "app", Imports: []java.ImportDecl{{Target: "tasks.Hidden.run", Static: true}}, Types: []*java.TypeDecl{caller}},
		&java.CompilationUnit{File: hidden.File, Package: "tasks", Types: []*java.TypeDecl{hidden}},
	)

	res := r.Resolve(mkCall("", "run"), MethodContext{EnclosingType: caller, File: caller.File})
	if len(res.Targets) != 0 {
		t.Fatalf("static import through inaccessible owner resolved: %+v", res.Targets)
	}
}

func TestResolveStaticImportPrecedenceUsesApplicableArity(t *testing.T) {
	caller := mkType("app.Caller", java.MethodDecl{
		Name: "run", Signature: "(int)", Params: []java.Param{{Type: ref("int")}},
	})
	caller.Name = "Caller"
	caller.File = "Caller.java"
	explicit := mkType("tasks.Explicit", mkStaticMethod("run"))
	explicit.Name = "Explicit"
	explicit.File = "Explicit.java"
	explicit.Modifier = []string{"public"}
	wildcard := mkType("tasks.Wildcard", mkStaticMethod("run"))
	wildcard.Name = "Wildcard"
	wildcard.File = "Wildcard.java"
	wildcard.Modifier = []string{"public"}
	r := newResolverFromUnits(t,
		&java.CompilationUnit{File: caller.File, Package: "app", Imports: []java.ImportDecl{
			{Target: "tasks.Explicit.run", Static: true},
			{Target: "tasks.Wildcard", Static: true, Wildcard: true},
		}, Types: []*java.TypeDecl{caller}},
		&java.CompilationUnit{File: explicit.File, Package: "tasks", Types: []*java.TypeDecl{explicit}},
		&java.CompilationUnit{File: wildcard.File, Package: "tasks", Types: []*java.TypeDecl{wildcard}},
	)
	ctx := MethodContext{EnclosingType: caller, File: caller.File}

	zero := r.Resolve(java.CallSite{MethodName: "run", ArgCount: 0}, ctx)
	wantImported := MethodHandle{TypeFQCN: "tasks.Explicit", Method: "run", Signature: "()"}
	if len(zero.Targets) != 1 || zero.Targets[0] != wantImported {
		t.Fatalf("zero-arity precedence = %+v (note: %q), want %+v", zero.Targets, zero.Note, wantImported)
	}
	one := r.Resolve(java.CallSite{MethodName: "run", ArgCount: 1}, ctx)
	wantCurrent := MethodHandle{TypeFQCN: "app.Caller", Method: "run", Signature: "(int)"}
	if len(one.Targets) != 1 || one.Targets[0] != wantCurrent {
		t.Fatalf("current precedence = %+v (note: %q), want %+v", one.Targets, one.Note, wantCurrent)
	}
}

func TestResolveStaticImportCollisionDoesNotFanOut(t *testing.T) {
	caller := mkType("app.Caller")
	caller.Name = "Caller"
	caller.File = "Caller.java"
	left := mkType("left.Tasks", mkStaticMethod("run"))
	left.Name, left.File = "Tasks", "Left.java"
	left.Modifier = []string{"public"}
	right := mkType("right.Tasks", mkStaticMethod("run"))
	right.Name, right.File = "Tasks", "Right.java"
	right.Modifier = []string{"public"}
	r := newResolverFromUnits(t,
		&java.CompilationUnit{File: caller.File, Package: "app", Imports: []java.ImportDecl{
			{Target: "right.Tasks.run", Static: true},
			{Target: "left.Tasks.run", Static: true},
		}, Types: []*java.TypeDecl{caller}},
		&java.CompilationUnit{File: left.File, Package: "left", Types: []*java.TypeDecl{left}},
		&java.CompilationUnit{File: right.File, Package: "right", Types: []*java.TypeDecl{right}},
	)

	res := r.Resolve(mkCall("", "run"), MethodContext{EnclosingType: caller, File: caller.File})
	if len(res.Targets) != 0 {
		t.Fatalf("static collision fanned out: %+v", res.Targets)
	}
	if !strings.Contains(res.Note, "ambiguous explicit static import") || !strings.Contains(res.Note, "left.Tasks.run()") || !strings.Contains(res.Note, "right.Tasks.run()") {
		t.Fatalf("static collision note = %q", res.Note)
	}
}

func TestResolveThisDoesNotUseStaticImportAndTypeReceiverRequiresStatic(t *testing.T) {
	caller := mkType("app.Caller")
	caller.Name, caller.File = "Caller", "Caller.java"
	utils := mkType("tasks.Utils", mkMethod("instance"), mkStaticMethod("staticRun"))
	utils.Name, utils.File = "Utils", "Utils.java"
	r := newResolverFromUnits(t,
		&java.CompilationUnit{File: caller.File, Package: "app", Imports: []java.ImportDecl{
			{Target: "tasks.Utils.staticRun", Static: true},
			{Target: "tasks.Utils"},
		}, Types: []*java.TypeDecl{caller}},
		&java.CompilationUnit{File: utils.File, Package: "tasks", Types: []*java.TypeDecl{utils}},
	)
	ctx := MethodContext{EnclosingType: caller, File: caller.File}

	if res := r.Resolve(mkCall("this", "staticRun"), ctx); len(res.Targets) != 0 {
		t.Fatalf("this used static import: %+v", res.Targets)
	}
	if res := r.Resolve(mkCall("Utils", "instance"), ctx); len(res.Targets) != 0 {
		t.Fatalf("type receiver selected instance method: %+v", res.Targets)
	}
	if res := r.Resolve(mkCall("Utils", "staticRun"), ctx); len(res.Targets) != 1 || res.Targets[0].TypeFQCN != "tasks.Utils" {
		t.Fatalf("type receiver static method = %+v (note: %q)", res.Targets, res.Note)
	}
}

func TestResolveInterfaceStaticMethodRequiresTypeReceiver(t *testing.T) {
	contract := mkType("contract.Tasks", java.MethodDecl{Name: "create", Signature: "()", Modifier: []string{"static"}})
	contract.Kind = java.TypeKindInterface
	contract.Modifier = []string{"public"}
	contract.Name, contract.File = "Tasks", "Tasks.java"
	caller := mkType("app.Caller")
	caller.Name, caller.File = "Caller", "Caller.java"
	caller.Fields = []java.FieldDecl{{Name: "tasks", Type: ref("contract.Tasks")}}
	r := newResolverFromUnits(t,
		&java.CompilationUnit{File: caller.File, Package: "app", Imports: []java.ImportDecl{{Target: "contract.Tasks"}}, Types: []*java.TypeDecl{caller}},
		&java.CompilationUnit{File: contract.File, Package: "contract", Types: []*java.TypeDecl{contract}},
	)
	ctx := MethodContext{EnclosingType: caller, File: caller.File}

	if res := r.Resolve(mkCall("tasks", "create"), ctx); len(res.Targets) != 0 {
		t.Fatalf("interface static method resolved through instance: %+v", res.Targets)
	}
	if res := r.Resolve(mkCall("Tasks", "create"), ctx); len(res.Targets) != 1 || res.Targets[0].TypeFQCN != "contract.Tasks" {
		t.Fatalf("interface static type receiver = %+v (note: %q)", res.Targets, res.Note)
	}
}

func TestResolveImportedInheritedStaticMethodUsesDeclaringOwner(t *testing.T) {
	caller := mkType("app.Caller")
	caller.Name, caller.File = "Caller", "Caller.java"
	parent := mkType("tasks.Parent", mkStaticMethod("run"))
	parent.Name, parent.File = "Parent", "Parent.java"
	child := mkType("tasks.Child")
	child.Name, child.File = "Child", "Child.java"
	child.Modifier = []string{"public"}
	child.SuperClass = ref("tasks.Parent")
	r := newResolverFromUnits(t,
		&java.CompilationUnit{File: caller.File, Package: "app", Imports: []java.ImportDecl{{Target: "tasks.Child.run", Static: true}}, Types: []*java.TypeDecl{caller}},
		&java.CompilationUnit{File: parent.File, Package: "tasks", Types: []*java.TypeDecl{parent}},
		&java.CompilationUnit{File: child.File, Package: "tasks", Types: []*java.TypeDecl{child}},
	)

	res := r.Resolve(mkCall("", "run"), MethodContext{EnclosingType: caller, File: caller.File})
	want := MethodHandle{TypeFQCN: "tasks.Parent", Method: "run", Signature: "()"}
	if len(res.Targets) != 1 || res.Targets[0] != want {
		t.Fatalf("inherited static import = %+v (note: %q), want %+v", res.Targets, res.Note, want)
	}
}

// Passo 7: scoped lookup com byte ranges

func TestResolveLocalVarInScope(t *testing.T) {
	helper := mkType("Helper", mkMethod("run"))
	r := newTestResolver([]*java.TypeDecl{helper})
	// call no byte 50; local visível em [0, 100), declarada no byte 10.
	ctx := MethodContext{LocalVars: []java.LocalVarDecl{{
		Name: "helper", Type: ref("Helper"),
		ScopeStart: 0, ScopeEnd: 100, DeclStart: 10,
	}}}

	res := r.Resolve(java.CallSite{Receiver: "helper", MethodName: "run", StartByte: 50}, ctx)

	want := MethodHandle{TypeFQCN: "Helper", Method: "run", Signature: "()"}
	if len(res.Targets) != 1 || res.Targets[0] != want {
		t.Fatalf("got %+v (note: %q), want %+v", res.Targets, res.Note, want)
	}
}

func TestResolveLocalVarDeclaredAfterCallRejected(t *testing.T) {
	helper := mkType("Helper", mkMethod("run"))
	r := newTestResolver([]*java.TypeDecl{helper})
	// call no byte 5; local declarada no byte 10 — não deveria resolver.
	ctx := MethodContext{LocalVars: []java.LocalVarDecl{{
		Name: "helper", Type: ref("Helper"),
		ScopeStart: 0, ScopeEnd: 100, DeclStart: 10,
	}}}

	res := r.Resolve(java.CallSite{Receiver: "helper", MethodName: "run", StartByte: 5}, ctx)

	if len(res.Targets) != 0 {
		t.Fatalf("local var declarada após a call não deveria resolver: %+v", res)
	}
}

func TestResolveLocalVarOutOfScopeRejected(t *testing.T) {
	helper := mkType("Helper", mkMethod("run"))
	r := newTestResolver([]*java.TypeDecl{helper})
	// call no byte 200 (fora do bloco onde a local vive).
	ctx := MethodContext{LocalVars: []java.LocalVarDecl{{
		Name: "helper", Type: ref("Helper"),
		ScopeStart: 50, ScopeEnd: 100, DeclStart: 60,
	}}}

	res := r.Resolve(java.CallSite{Receiver: "helper", MethodName: "run", StartByte: 200}, ctx)

	if len(res.Targets) != 0 {
		t.Fatalf("local var fora do escopo não deveria resolver: %+v", res)
	}
}

func TestResolveLocalVarShadowedByInnerBlock(t *testing.T) {
	outer := mkType("OuterHelper", mkMethod("run"))
	inner := mkType("InnerHelper", mkMethod("run"))
	r := newTestResolver([]*java.TypeDecl{outer, inner})
	// dois blocos sobrepostos: outer [0, 200) e inner [50, 100).
	// call no byte 60 — inner deve vencer por shadowing.
	ctx := MethodContext{LocalVars: []java.LocalVarDecl{
		{Name: "helper", Type: ref("OuterHelper"), ScopeStart: 0, ScopeEnd: 200, DeclStart: 10},
		{Name: "helper", Type: ref("InnerHelper"), ScopeStart: 50, ScopeEnd: 100, DeclStart: 55},
	}}

	res := r.Resolve(java.CallSite{Receiver: "helper", MethodName: "run", StartByte: 60}, ctx)

	want := MethodHandle{TypeFQCN: "InnerHelper", Method: "run", Signature: "()"}
	if len(res.Targets) != 1 || res.Targets[0] != want {
		t.Fatalf("got %+v (note: %q), want inner shadow %+v", res.Targets, res.Note, want)
	}
}

func TestResolveParamShadowsField(t *testing.T) {
	paramType := mkType("ParamType", mkMethod("run"))
	fieldType := mkType("FieldType", mkMethod("run"))
	enclosing := mkType("Owner")
	enclosing.Fields = []java.FieldDecl{{Name: "helper", Type: ref("FieldType")}}
	r := newTestResolver([]*java.TypeDecl{enclosing, paramType, fieldType})
	ctx := MethodContext{
		EnclosingType: enclosing,
		Params:        []java.Param{{Name: "helper", Type: ref("ParamType")}},
	}

	res := r.Resolve(mkCall("helper", "run"), ctx)

	want := MethodHandle{TypeFQCN: "ParamType", Method: "run", Signature: "()"}
	if len(res.Targets) != 1 || res.Targets[0] != want {
		t.Fatalf("got %+v (note: %q), want param target %+v", res.Targets, res.Note, want)
	}
}

func TestResolveLocalVarShadowsParam(t *testing.T) {
	localType := mkType("LocalType", mkMethod("run"))
	paramType := mkType("ParamType", mkMethod("run"))
	r := newTestResolver([]*java.TypeDecl{localType, paramType})
	ctx := MethodContext{
		Params:    []java.Param{{Name: "helper", Type: ref("ParamType")}},
		LocalVars: []java.LocalVarDecl{localVar("helper", "LocalType")},
	}

	res := r.Resolve(mkCall("helper", "run"), ctx)

	want := MethodHandle{TypeFQCN: "LocalType", Method: "run", Signature: "()"}
	if len(res.Targets) != 1 || res.Targets[0] != want {
		t.Fatalf("got %+v (note: %q), want local target %+v", res.Targets, res.Note, want)
	}
}

func TestResolveFieldWhenNoLocalNoParam(t *testing.T) {
	fieldType := mkType("FieldType", mkMethod("run"))
	enclosing := mkType("Owner")
	enclosing.Fields = []java.FieldDecl{{Name: "helper", Type: ref("FieldType")}}
	r := newTestResolver([]*java.TypeDecl{enclosing, fieldType})
	ctx := MethodContext{EnclosingType: enclosing}

	res := r.Resolve(mkCall("helper", "run"), ctx)

	want := MethodHandle{TypeFQCN: "FieldType", Method: "run", Signature: "()"}
	if len(res.Targets) != 1 || res.Targets[0] != want {
		t.Fatalf("got %+v (note: %q), want field target %+v", res.Targets, res.Note, want)
	}
}
