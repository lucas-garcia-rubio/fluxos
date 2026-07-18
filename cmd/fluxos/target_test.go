package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
	"github.com/lucas-garcia-rubio/fluxos/internal/index"
)

func TestParseTargetSpecSimpleName(t *testing.T) {
	got, err := ParseTargetSpec("Workflow.start")
	if err != nil {
		t.Fatalf("ParseTargetSpec: %v", err)
	}
	want := TargetSpec{TypeName: "Workflow", Method: "start"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestParseTargetSpecFQCN(t *testing.T) {
	got, err := ParseTargetSpec("com.foo.Workflow.start")
	if err != nil {
		t.Fatalf("ParseTargetSpec: %v", err)
	}
	want := TargetSpec{TypeName: "com.foo.Workflow", Method: "start"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestParseTargetSpecNestedFQCN(t *testing.T) {
	got, err := ParseTargetSpec("com.foo.Outer.Inner.run")
	if err != nil {
		t.Fatalf("ParseTargetSpec: %v", err)
	}
	want := TargetSpec{TypeName: "com.foo.Outer.Inner", Method: "run"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestParseTargetSpecEmptySignature(t *testing.T) {
	got, err := ParseTargetSpec("com.foo.Workflow.start()")
	if err != nil {
		t.Fatalf("ParseTargetSpec: %v", err)
	}
	want := TargetSpec{TypeName: "com.foo.Workflow", Method: "start", HasSignature: true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestParseTargetSpecFullSignature(t *testing.T) {
	got, err := ParseTargetSpec("com.foo.Workflow.start(java.lang.String,int)")
	if err != nil {
		t.Fatalf("ParseTargetSpec: %v", err)
	}
	want := TargetSpec{
		TypeName:     "com.foo.Workflow",
		Method:       "start",
		Signature:    "java.lang.String,int",
		HasSignature: true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestParseTargetSpecRejectsEmptySpec(t *testing.T) {
	if _, err := ParseTargetSpec(""); err == nil {
		t.Fatalf("expected error for empty spec")
	}
}

func TestParseTargetSpecRejectsMissingDot(t *testing.T) {
	if _, err := ParseTargetSpec("Workf"); err == nil {
		t.Fatalf("expected error for spec without '.'")
	}
}

func TestParseTargetSpecRejectsEmptyMethod(t *testing.T) {
	_, err := ParseTargetSpec("Workflow.")
	if err == nil || !strings.Contains(err.Error(), "empty method name") {
		t.Fatalf("error = %v, want empty method name", err)
	}
}

func TestParseTargetSpecRejectsEmptyType(t *testing.T) {
	_, err := ParseTargetSpec(".start")
	if err == nil || !strings.Contains(err.Error(), "empty type name") {
		t.Fatalf("error = %v, want empty type name", err)
	}
}

func TestParseTargetSpecRejectsUnclosedParen(t *testing.T) {
	_, err := ParseTargetSpec("Workflow.start(")
	if err == nil || !strings.Contains(err.Error(), "'('") {
		t.Fatalf("error = %v, want complaint about '('", err)
	}
}

func TestParseTargetSpecRejectsCloseParenWithoutOpen(t *testing.T) {
	_, err := ParseTargetSpec("Workflow.start)")
	if err == nil || !strings.Contains(err.Error(), "matching '('") {
		t.Fatalf("error = %v, want missing matching '('", err)
	}
}

func TestParseTargetSpecRejectsSpacesInSignature(t *testing.T) {
	_, err := ParseTargetSpec("Workflow.start(String, int)")
	if err == nil || !strings.Contains(err.Error(), "spaces are not allowed") {
		t.Fatalf("error = %v, want spaces are not allowed", err)
	}
}

func TestParseTargetSpecRejectsEmptyParameterType(t *testing.T) {
	cases := []string{
		"Workflow.start(,String)",
		"Workflow.start(String,)",
		"Workflow.start(String,,int)",
	}
	for _, spec := range cases {
		if _, err := ParseTargetSpec(spec); err == nil || !strings.Contains(err.Error(), "empty parameter type") {
			t.Fatalf("ParseTargetSpec(%q) err = %v, want empty parameter type", spec, err)
		}
	}
}

func TestResolveTargetByFQCN(t *testing.T) {
	table := buildTargetTestTable(t)
	target, err := ResolveTarget(table, TargetSpec{TypeName: "app.Workflow", Method: "start"})
	if err != nil {
		t.Fatalf("ResolveTarget: %v", err)
	}
	if target.DeclaringType.FQCN != "app.Workflow" || target.Method.Name != "start" {
		t.Fatalf("got type=%s method=%s", target.DeclaringType.FQCN, target.Method.Name)
	}
	assertRootExecution(t, target, "app.Workflow", "app.Workflow", "start", "()", "app.Workflow")
}

func TestResolveTargetBySimpleNameUnique(t *testing.T) {
	table := buildTargetTestTable(t)
	target, err := ResolveTarget(table, TargetSpec{TypeName: "Workflow", Method: "start"})
	if err != nil {
		t.Fatalf("ResolveTarget: %v", err)
	}
	if target.RequestedType.FQCN != "app.Workflow" {
		t.Fatalf("got type=%s, want app.Workflow", target.RequestedType.FQCN)
	}
}

func TestResolveTargetAmbiguousSimpleNameListsFQCNs(t *testing.T) {
	table := buildTargetTestTableWithHomonym(t)
	_, err := ResolveTarget(table, TargetSpec{TypeName: "Service", Method: "run"})
	if err == nil || !strings.Contains(err.Error(), "candidates: left.Service, right.Service") {
		t.Fatalf("err = %v, want candidates list", err)
	}
}

func TestResolveTargetUnknownFQCN(t *testing.T) {
	table := buildTargetTestTable(t)
	_, err := ResolveTarget(table, TargetSpec{TypeName: "app.Missing", Method: "start"})
	if err == nil || !strings.Contains(err.Error(), `"app.Missing" not found`) {
		t.Fatalf("err = %v, want unknown type", err)
	}
}

func TestResolveTargetWithSignatureExact(t *testing.T) {
	table := buildTargetTestTableWithOverload(t)
	target, err := ResolveTarget(table, TargetSpec{
		TypeName:     "app.Workflow",
		Method:       "start",
		Signature:    "java.lang.String",
		HasSignature: true,
	})
	if err != nil {
		t.Fatalf("ResolveTarget: %v", err)
	}
	if target.Method.Name != "start" || target.Method.Signature != "(java.lang.String)" {
		t.Fatalf("got method=%s%s", target.Method.Name, target.Method.Signature)
	}
}

func TestResolveTargetWithoutSignatureUniqueOverload(t *testing.T) {
	table := buildTargetTestTable(t)
	target, err := ResolveTarget(table, TargetSpec{TypeName: "app.Workflow", Method: "start"})
	if err != nil {
		t.Fatalf("ResolveTarget: %v", err)
	}
	if target.Method.Name != "start" {
		t.Fatalf("got method=%s", target.Method.Name)
	}
}

func TestResolveTargetWithoutSignatureAmbiguousListsSignatures(t *testing.T) {
	table := buildTargetTestTableWithOverload(t)
	_, err := ResolveTarget(table, TargetSpec{TypeName: "app.Workflow", Method: "start"})
	if err == nil || !strings.Contains(err.Error(), "available signatures: (), (java.lang.String)") {
		t.Fatalf("err = %v, want signatures list", err)
	}
}

func TestResolveTargetUnknownMethod(t *testing.T) {
	table := buildTargetTestTable(t)
	_, err := ResolveTarget(table, TargetSpec{TypeName: "app.Workflow", Method: "missing"})
	if err == nil || !strings.Contains(err.Error(), `method "missing" not found`) {
		t.Fatalf("err = %v, want method not found", err)
	}
}

func TestResolveTargetWithUnknownSignatureListsAvailable(t *testing.T) {
	table := buildTargetTestTableWithOverload(t)
	_, err := ResolveTarget(table, TargetSpec{
		TypeName:     "app.Workflow",
		Method:       "start",
		Signature:    "int",
		HasSignature: true,
	})
	if err == nil || !strings.Contains(err.Error(), "available signatures: (), (java.lang.String)") {
		t.Fatalf("err = %v, want signatures list", err)
	}
}

func TestResolveTargetPreservesRequestedRuntimeForInheritedMethod(t *testing.T) {
	parent := &java.TypeDecl{
		Name: "Parent", FQCN: "app.Parent",
		Methods: []java.MethodDecl{{Name: "run", Signature: "()", Modifier: []string{"public"}}},
	}
	child := &java.TypeDecl{
		Name: "Child", FQCN: "app.Child",
		SuperClass: java.NewTypeRef("Parent", false),
	}
	table := buildTableFromTypes(t, "app", parent, child)

	target, err := ResolveTarget(table, TargetSpec{TypeName: "app.Child", Method: "run"})
	if err != nil {
		t.Fatalf("ResolveTarget: %v", err)
	}
	if target.RequestedType != child || target.DeclaringType != parent || target.Method != &parent.Methods[0] {
		t.Fatalf("inherited target = %+v, want requested app.Child and declaring app.Parent.run()", target)
	}
	assertRootExecution(t, target, "app.Child", "app.Parent", "run", "()", "app.Child")
}

func TestResolveTargetAmbiguousDefaultsListsDeclaringOwners(t *testing.T) {
	left := &java.TypeDecl{
		Kind: java.TypeKindInterface, Name: "Left", FQCN: "app.Left",
		Methods: []java.MethodDecl{{Name: "run", Signature: "()", Modifier: []string{"default"}}},
	}
	right := &java.TypeDecl{
		Kind: java.TypeKindInterface, Name: "Right", FQCN: "app.Right",
		Methods: []java.MethodDecl{{Name: "run", Signature: "()", Modifier: []string{"default"}}},
	}
	child := &java.TypeDecl{
		Kind: java.TypeKindClass, Name: "Child", FQCN: "app.Child",
		Interfaces: []java.TypeRef{java.NewTypeRef("Left", false), java.NewTypeRef("Right", false)},
	}
	table := buildTableFromTypes(t, "app", left, right, child)

	_, err := ResolveTarget(table, TargetSpec{TypeName: "app.Child", Method: "run"})
	if err == nil || !strings.Contains(err.Error(), "app.Left.run()") || !strings.Contains(err.Error(), "app.Right.run()") {
		t.Fatalf("ambiguous default error = %v", err)
	}
}

func TestResolveTargetConstructorBySignature(t *testing.T) {
	value := &java.TypeDecl{
		Kind: java.TypeKindClass, Name: "Value", FQCN: "app.Value",
		Methods: []java.MethodDecl{
			{Kind: java.MethodConstructor, Name: "<init>", Params: []java.Param{}},
			{Kind: java.MethodConstructor, Name: "<init>", Params: []java.Param{{Type: java.NewTypeRef("String", false)}}},
		},
	}
	table := buildTableFromTypes(t, "app", value)

	target, err := ResolveTarget(table, TargetSpec{TypeName: "app.Value", Method: "<init>", Signature: "java.lang.String", HasSignature: true})
	if err != nil {
		t.Fatalf("ResolveTarget constructor: %v", err)
	}
	if target.DeclaringType != value || target.Method.Kind != java.MethodConstructor || target.Method.Signature != "(java.lang.String)" {
		t.Fatalf("constructor target = %+v", target)
	}
	assertRootExecution(t, target, "app.Value", "app.Value", "<init>", "(java.lang.String)", "app.Value")
}

func TestResolveTargetConstructorWithoutSignatureAmbiguous(t *testing.T) {
	value := &java.TypeDecl{
		Kind: java.TypeKindClass, Name: "Value", FQCN: "app.Value",
		Methods: []java.MethodDecl{
			{Kind: java.MethodConstructor, Name: "<init>", Params: []java.Param{}},
			{Kind: java.MethodConstructor, Name: "<init>", Params: []java.Param{{Type: java.NewTypeRef("int", false)}}},
		},
	}
	table := buildTableFromTypes(t, "app", value)

	_, err := ResolveTarget(table, TargetSpec{TypeName: "app.Value", Method: "<init>"})
	if err == nil || !strings.Contains(err.Error(), "available signatures: (), (int)") {
		t.Fatalf("ambiguous constructor error = %v", err)
	}
}

func TestResolveTargetSyntheticDefaultConstructor(t *testing.T) {
	value := &java.TypeDecl{Kind: java.TypeKindClass, Name: "Value", FQCN: "app.Value"}
	table := buildTableFromTypes(t, "app", value)

	target, err := ResolveTarget(table, TargetSpec{TypeName: "app.Value", Method: "<init>", HasSignature: true})
	if err != nil {
		t.Fatalf("ResolveTarget synthetic constructor: %v", err)
	}
	if target.DeclaringType != value || !target.Method.Synthetic || target.Method.Signature != "()" {
		t.Fatalf("synthetic constructor target = %+v", target)
	}
}

func TestResolveTargetStaticUsesDeclaringRuntime(t *testing.T) {
	parent := &java.TypeDecl{
		Name: "Parent", FQCN: "app.Parent",
		Methods: []java.MethodDecl{{Name: "run", Signature: "()", Modifier: []string{"public", "static"}}},
	}
	child := &java.TypeDecl{Name: "Child", FQCN: "app.Child", SuperClass: java.NewTypeRef("Parent", false)}
	table := buildTableFromTypes(t, "app", parent, child)

	target, err := ResolveTarget(table, TargetSpec{TypeName: "app.Child", Method: "run"})
	if err != nil {
		t.Fatalf("ResolveTarget: %v", err)
	}
	assertRootExecution(t, target, "app.Child", "app.Parent", "run", "()", "app.Parent")
}

func TestResolveTargetInterfaceDefaultUsesRequestedAssumedRuntime(t *testing.T) {
	contract := &java.TypeDecl{
		Kind: java.TypeKindInterface, Name: "Contract", FQCN: "app.Contract",
		Methods: []java.MethodDecl{{Name: "run", Signature: "()", Modifier: []string{"public", "default"}}},
	}
	table := buildTableFromTypes(t, "app", contract)

	target, err := ResolveTarget(table, TargetSpec{TypeName: "app.Contract", Method: "run"})
	if err != nil {
		t.Fatalf("ResolveTarget: %v", err)
	}
	assertRootExecution(t, target, "app.Contract", "app.Contract", "run", "()", "app.Contract")
}

func assertRootExecution(t *testing.T, target RootTarget, requested, declaring, method, signature, runtime string) {
	t.Helper()
	if target.RequestedType.FQCN != requested || target.DeclaringType.FQCN != declaring ||
		target.Execution.Method.TypeFQCN != declaring || target.Execution.Method.Method != method ||
		target.Execution.Method.Signature != signature || target.Execution.RuntimeTypeFQCN != runtime {
		t.Fatalf("root target = %+v, want requested=%s declaring=%s method=%s%s runtime=%s",
			target, requested, declaring, method, signature, runtime)
	}
}

func buildTargetTestTable(t *testing.T) *index.Table {
	t.Helper()
	workflow := &java.TypeDecl{
		Name:    "Workflow",
		FQCN:    "app.Workflow",
		Methods: []java.MethodDecl{{Name: "start", Signature: "()"}},
	}
	return buildTableFromTypes(t, "app", workflow)
}

func buildTargetTestTableWithHomonym(t *testing.T) *index.Table {
	t.Helper()
	left := &java.TypeDecl{Name: "Service", FQCN: "left.Service"}
	right := &java.TypeDecl{Name: "Service", FQCN: "right.Service"}
	return buildTableFromTypesInPackages(t,
		[]*java.TypeDecl{left}, "left",
		[]*java.TypeDecl{right}, "right",
	)
}

func buildTargetTestTableWithOverload(t *testing.T) *index.Table {
	t.Helper()
	workflow := &java.TypeDecl{
		Name: "Workflow",
		FQCN: "app.Workflow",
		Methods: []java.MethodDecl{
			{Name: "start", Signature: "()"},
			{Name: "start", Signature: "(java.lang.String)", Params: []java.Param{{Type: java.NewTypeRef("java.lang.String", false)}}},
		},
	}
	return buildTableFromTypes(t, "app", workflow)
}

func buildTableFromTypes(t *testing.T, pkg string, types ...*java.TypeDecl) *index.Table {
	t.Helper()
	file := pkg + ".java"
	unit := &java.CompilationUnit{File: file, Package: pkg, Types: types}
	table, err := index.Build([]*java.CompilationUnit{unit})
	if err != nil {
		t.Fatalf("index.Build: %v", err)
	}
	return table
}

func buildTableFromTypesInPackages(t *testing.T, t1 []*java.TypeDecl, p1 string, t2 []*java.TypeDecl, p2 string) *index.Table {
	t.Helper()
	u1 := &java.CompilationUnit{File: p1 + ".java", Package: p1, Types: t1}
	u2 := &java.CompilationUnit{File: p2 + ".java", Package: p2, Types: t2}
	table, err := index.Build([]*java.CompilationUnit{u1, u2})
	if err != nil {
		t.Fatalf("index.Build: %v", err)
	}
	return table
}
