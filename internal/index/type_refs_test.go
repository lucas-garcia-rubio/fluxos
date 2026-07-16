package index

import (
	"reflect"
	"testing"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
)

func refType(name, fqcn, file string) *java.TypeDecl {
	return &java.TypeDecl{Name: name, FQCN: fqcn, File: file}
}

func TestResolveTypeRefPrecedence(t *testing.T) {
	caller := refType("Caller", "app.Caller", "Caller.java")
	local := refType("Helper", "app.Helper", "LocalHelper.java")
	explicit := refType("Helper", "explicit.Helper", "ExplicitHelper.java")
	wildcard := refType("Helper", "wild.Helper", "WildcardHelper.java")
	unit := &java.CompilationUnit{
		File:    caller.File,
		Package: "app",
		Imports: []java.ImportDecl{
			{Target: "wild", Wildcard: true},
			{Target: "explicit.Helper"},
		},
		Types: []*java.TypeDecl{caller},
	}
	table, err := Build([]*java.CompilationUnit{
		unit,
		{File: local.File, Package: "app", Types: []*java.TypeDecl{local}},
		{File: explicit.File, Package: "explicit", Types: []*java.TypeDecl{explicit}},
		{File: wildcard.File, Package: "wild", Types: []*java.TypeDecl{wildcard}},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	got := table.ResolveTypeRef(java.NewTypeRef("Helper", false), unit)
	if got.Ref.FQCN != "explicit.Helper" || got.Ref.Unresolved {
		t.Fatalf("explicit import did not win: %+v", got)
	}
}

func TestResolveTypeRefSamePackageAndWildcardAmbiguity(t *testing.T) {
	caller := refType("Caller", "app.Caller", "Caller.java")
	neighbor := refType("Neighbor", "app.Neighbor", "Neighbor.java")
	left := refType("Helper", "left.Helper", "Left.java")
	right := refType("Helper", "right.Helper", "Right.java")
	unit := &java.CompilationUnit{
		File:    caller.File,
		Package: "app",
		Imports: []java.ImportDecl{{Target: "right", Wildcard: true}, {Target: "left", Wildcard: true}},
		Types:   []*java.TypeDecl{caller},
	}
	table, err := Build([]*java.CompilationUnit{
		unit,
		{File: neighbor.File, Package: "app", Types: []*java.TypeDecl{neighbor}},
		{File: left.File, Package: "left", Types: []*java.TypeDecl{left}},
		{File: right.File, Package: "right", Types: []*java.TypeDecl{right}},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	samePackage := table.ResolveTypeRef(java.NewTypeRef("Neighbor", false), unit)
	if samePackage.Ref.FQCN != "app.Neighbor" {
		t.Fatalf("same-package resolution = %+v", samePackage)
	}
	ambiguous := table.ResolveTypeRef(java.NewTypeRef("Helper", false), unit)
	if !ambiguous.Ref.Unresolved || ambiguous.Ref.FQCN != "" || !reflect.DeepEqual(ambiguous.Candidates, []string{"left.Helper", "right.Helper"}) {
		t.Fatalf("wildcard ambiguity = %+v", ambiguous)
	}
}

func TestResolveTypeRefKnownAndUnknownTypes(t *testing.T) {
	table, err := Build(nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	primitive := table.ResolveTypeRef(java.NewTypeRef("int[]", false), nil)
	if !primitive.Ref.Primitive || primitive.Ref.Unresolved || primitive.Ref.SignatureToken() != "int[]" {
		t.Fatalf("primitive resolution = %+v", primitive)
	}
	lang := table.ResolveTypeRef(java.NewTypeRef("String", false), nil)
	if lang.Ref.FQCN != "java.lang.String" || lang.Ref.Unresolved {
		t.Fatalf("java.lang resolution = %+v", lang)
	}
	unknown := table.ResolveTypeRef(java.NewTypeRef("Missing", false), nil)
	if !unknown.Ref.Unresolved || unknown.Ref.FQCN != "" || len(unknown.Candidates) != 0 {
		t.Fatalf("unknown resolution = %+v", unknown)
	}
}

func TestBuildCanonicalizesTypeReferencesAndSignatures(t *testing.T) {
	caller := refType("Caller", "app.Caller", "Caller.java")
	caller.SuperClass = java.NewTypeRef("Parent", false)
	caller.Interfaces = []java.TypeRef{java.NewTypeRef("Contract", false)}
	caller.Fields = []java.FieldDecl{{Name: "service", Type: java.NewTypeRef("Service", false)}}
	caller.Methods = []java.MethodDecl{{
		Name:       "run",
		Signature:  "(List[])",
		ReturnType: java.NewTypeRef("Service", false),
		Params:     []java.Param{{Name: "values", Type: java.NewTypeRef("List<Service>[]", false)}},
		LocalVars:  []java.LocalVarDecl{{Name: "local", Type: java.NewTypeRef("Service", false)}},
	}}
	unit := &java.CompilationUnit{
		File:    caller.File,
		Package: "app",
		Imports: []java.ImportDecl{
			{Target: "dep.Service"},
			{Target: "dep.Parent"},
			{Target: "dep.Contract"},
			{Target: "java.util.List"},
		},
		Types: []*java.TypeDecl{caller},
	}
	dependencies := []*java.TypeDecl{
		refType("Service", "dep.Service", "Service.java"),
		refType("Parent", "dep.Parent", "Parent.java"),
		refType("Contract", "dep.Contract", "Contract.java"),
	}
	units := []*java.CompilationUnit{unit}
	for _, typ := range dependencies {
		units = append(units, &java.CompilationUnit{File: typ.File, Package: "dep", Types: []*java.TypeDecl{typ}})
	}

	table, err := Build(units)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	method := &caller.Methods[0]
	if caller.SuperClass.FQCN != "dep.Parent" || caller.Interfaces[0].FQCN != "dep.Contract" || caller.Fields[0].Type.FQCN != "dep.Service" {
		t.Fatalf("type relationships were not canonicalized: %+v", caller)
	}
	if method.ReturnType.FQCN != "dep.Service" || method.LocalVars[0].Type.FQCN != "dep.Service" || method.Params[0].Type.FQCN != "java.util.List" {
		t.Fatalf("method type refs were not canonicalized: %+v", method)
	}
	if method.Signature != "(java.util.List[])" {
		t.Fatalf("signature = %q, want %q", method.Signature, "(java.util.List[])")
	}
	if got, ok := table.Method(caller.FQCN, method.Key()); !ok || got != method {
		t.Fatalf("canonical method key lookup = %+v, %v", got, ok)
	}
}
