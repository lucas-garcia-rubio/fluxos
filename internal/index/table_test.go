package index

import (
	"reflect"
	"strings"
	"testing"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
)

func fixtureType(name, fqcn, file string, methods ...java.MethodDecl) *java.TypeDecl {
	return &java.TypeDecl{Name: name, FQCN: fqcn, File: file, Methods: methods}
}

func TestBuildAndLookup(t *testing.T) {
	serviceFile := "src/com/foo/Service.java"
	otherFile := "src/com/bar/Service.java"
	service := fixtureType("Service", "com.foo.Service", serviceFile,
		java.MethodDecl{Name: "run", Signature: "(String)", Params: []java.Param{{Type: java.NewTypeRef("String", false)}}},
		java.MethodDecl{Name: "run", Signature: "()"},
	)
	other := fixtureType("Service", "com.bar.Service", otherFile)
	units := []*java.CompilationUnit{
		{File: serviceFile, Package: "com.foo", Types: []*java.TypeDecl{service}},
		{File: otherFile, Package: "com.bar", Types: []*java.TypeDecl{other}},
	}

	table, err := Build(units)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got, ok := table.TypeByFQCN("com.foo.Service"); !ok || got != service {
		t.Fatalf("TypeByFQCN = %+v, %v", got, ok)
	}
	if got := table.TypesBySimple("Service"); !reflect.DeepEqual(got, []*java.TypeDecl{other, service}) {
		t.Fatalf("TypesBySimple = %+v", got)
	}
	if got := table.TypesInPackage("com.foo"); !reflect.DeepEqual(got, []*java.TypeDecl{service}) {
		t.Fatalf("TypesInPackage = %+v", got)
	}
	if got := table.UnitForType(service.FQCN); got != units[0] {
		t.Fatalf("UnitForType = %+v", got)
	}
	key := java.MethodKey{Name: "run", Signature: "()"}
	if got, ok := table.Method(service.FQCN, key); !ok || got.Signature != "()" {
		t.Fatalf("Method = %+v, %v", got, ok)
	}
	candidates := table.MethodCandidates(service.FQCN, "run")
	if len(candidates) != 2 || candidates[0].Signature != "()" || candidates[1].Signature != "(java.lang.String)" {
		t.Fatalf("MethodCandidates = %+v", candidates)
	}
}

func TestBuildRejectsDuplicateFQCNDeterministically(t *testing.T) {
	first := &java.CompilationUnit{File: "z.java", Types: []*java.TypeDecl{fixtureType("Same", "pkg.Same", "z.java")}}
	second := &java.CompilationUnit{File: "a.java", Types: []*java.TypeDecl{fixtureType("Same", "pkg.Same", "a.java")}}

	for _, units := range [][]*java.CompilationUnit{{first, second}, {second, first}} {
		_, err := Build(units)
		if err == nil || err.Error() != `index: duplicate FQCN "pkg.Same" in "a.java" and "z.java"` {
			t.Fatalf("Build duplicate error = %v", err)
		}
	}
}

func TestBuildRejectsDuplicateFileAndMethod(t *testing.T) {
	first := &java.CompilationUnit{File: "Same.java", SourceRoot: "z"}
	second := &java.CompilationUnit{File: "Same.java", SourceRoot: "a"}
	third := &java.CompilationUnit{File: "Same.java", SourceRoot: "m"}
	for _, units := range [][]*java.CompilationUnit{{first, second, third}, {third, first, second}, {second, third, first}} {
		_, err := Build(units)
		if err == nil || err.Error() != `index: duplicate compilation unit file "Same.java" for source roots "a" and "m"` {
			t.Fatalf("duplicate unit error = %v", err)
		}
	}

	typ := fixtureType("Service", "Service", "Service.java",
		java.MethodDecl{Name: "run", Signature: "()"},
		java.MethodDecl{Name: "run", Signature: "()"},
	)
	if _, err := Build([]*java.CompilationUnit{{File: typ.File, Types: []*java.TypeDecl{typ}}}); err == nil || !strings.Contains(err.Error(), "duplicate method") {
		t.Fatalf("duplicate method error = %v", err)
	}
}

func TestCandidateSlicesAreDefensiveCopies(t *testing.T) {
	first := fixtureType("Service", "a.Service", "a.java", java.MethodDecl{Name: "run", Signature: "()"})
	second := fixtureType("Service", "b.Service", "b.java")
	table, err := Build([]*java.CompilationUnit{
		{File: first.File, Package: "a", Types: []*java.TypeDecl{first}},
		{File: second.File, Package: "b", Types: []*java.TypeDecl{second}},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	types := table.TypesBySimple("Service")
	types[0] = second
	if got := table.TypesBySimple("Service"); got[0] != first {
		t.Fatalf("TypesBySimple exposed internal slice: %+v", got)
	}
	methods := table.MethodCandidates(first.FQCN, "run")
	methods[0] = nil
	key := java.MethodKey{Name: "run", Signature: "()"}
	if got, ok := table.Method(first.FQCN, key); !ok || got != &first.Methods[0] {
		t.Fatalf("Method = %+v, %v", got, ok)
	}
}

func TestNilTableLookups(t *testing.T) {
	var table *Table
	if table.UnitForType("Missing") != nil {
		t.Fatal("UnitForType on nil table returned a unit")
	}
	if typ, ok := table.TypeByFQCN("Missing"); typ != nil || ok {
		t.Fatalf("TypeByFQCN on nil table = %+v, %v", typ, ok)
	}
	if method, ok := table.Method("Missing", java.MethodKey{}); method != nil || ok {
		t.Fatalf("Method on nil table = %+v, %v", method, ok)
	}
	if table.TypesBySimple("Missing") == nil || table.TypesInPackage("missing") == nil || table.MethodCandidates("Missing", "run") == nil {
		t.Fatal("nil table candidate slices must be non-nil")
	}
}

func TestEmptyTableReturnsNonNilCandidates(t *testing.T) {
	table, err := Build(nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if table.TypesBySimple("Missing") == nil || table.TypesInPackage("missing") == nil || table.MethodCandidates("Missing", "run") == nil {
		t.Fatal("empty candidate slices must be non-nil")
	}
}
