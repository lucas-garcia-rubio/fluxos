package index

import (
	"reflect"
	"testing"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
)

func staticMethod(name string, parameterTypes ...string) java.MethodDecl {
	params := make([]java.Param, len(parameterTypes))
	for i, parameterType := range parameterTypes {
		params[i].Type = java.NewTypeRef(parameterType, false)
	}
	return java.MethodDecl{Kind: java.MethodOrdinary, Name: name, Modifier: []string{"public", "static"}, Params: params}
}

func buildStaticMethodTable(t *testing.T, types ...*java.TypeDecl) *Table {
	t.Helper()
	units := make([]*java.CompilationUnit, 0, len(types))
	for _, typ := range types {
		units = append(units, &java.CompilationUnit{File: typ.File, Package: staticTestPackageName(typ.FQCN), Types: []*java.TypeDecl{typ}})
	}
	table, err := Build(units)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return table
}

func staticTestPackageName(fqcn string) string {
	for i := len(fqcn) - 1; i >= 0; i-- {
		if fqcn[i] == '.' {
			return fqcn[:i]
		}
	}
	return ""
}

func TestStaticMethodCandidatesDirectAndFilters(t *testing.T) {
	typ := fixtureType("Util", "pkg.Util", "Util.java",
		staticMethod("run", "String"),
		staticMethod("run"),
		java.MethodDecl{Kind: java.MethodOrdinary, Name: "run", Modifier: []string{"public"}, Params: []java.Param{{Type: java.NewTypeRef("int", false)}}},
		java.MethodDecl{Kind: java.MethodConstructor, Name: "<init>", Signature: "()", Modifier: []string{"public", "static"}},
		java.MethodDecl{Kind: java.MethodCompactConstructor, Name: "run", Modifier: []string{"static"}, Params: []java.Param{{Type: java.NewTypeRef("long", false)}}},
	)
	table := buildStaticMethodTable(t, typ)

	got := table.StaticMethodCandidates("pkg.Util", "run")
	if len(got) != 2 {
		t.Fatalf("StaticMethodCandidates = %+v, want two direct static methods", got)
	}
	if got[0].DeclaringType != typ || got[0].Method.Signature != "()" || got[1].Method.Signature != "(java.lang.String)" {
		t.Fatalf("StaticMethodCandidates = %+v, want sorted direct declarations", got)
	}
	if constructors := table.StaticMethodCandidates("pkg.Util", "<init>"); len(constructors) != 0 {
		t.Fatalf("constructor candidates = %+v, want none", constructors)
	}
}

func TestStaticMethodCandidatesUsesDeclaringOwnerForInheritedClassMethod(t *testing.T) {
	parent := fixtureType("Parent", "pkg.Parent", "Parent.java", staticMethod("run"))
	child := fixtureType("Child", "pkg.Child", "Child.java")
	child.SuperClass = java.NewTypeRef("Parent", false)
	table := buildStaticMethodTable(t, child, parent)

	got := table.StaticMethodCandidates("pkg.Child", "run")
	if len(got) != 1 || got[0].DeclaringType != parent || got[0].Method != &parent.Methods[0] {
		t.Fatalf("StaticMethodCandidates = %+v, want declaration on pkg.Parent", got)
	}
}

func TestStaticMethodCandidatesInterfaceStaticsAreDirectOnly(t *testing.T) {
	parent := fixtureType("Parent", "pkg.Parent", "Parent.java", staticMethod("create"))
	parent.Kind = java.TypeKindInterface
	child := fixtureType("Child", "pkg.Child", "Child.java", staticMethod("create", "String"))
	child.Kind = java.TypeKindInterface
	child.Interfaces = []java.TypeRef{java.NewTypeRef("Parent", false)}
	table := buildStaticMethodTable(t, child, parent)

	parentCandidates := table.StaticMethodCandidates("pkg.Parent", "create")
	if len(parentCandidates) != 1 || parentCandidates[0].DeclaringType != parent {
		t.Fatalf("parent StaticMethodCandidates = %+v, want direct interface static", parentCandidates)
	}
	childCandidates := table.StaticMethodCandidates("pkg.Child", "create")
	if len(childCandidates) != 1 || childCandidates[0].DeclaringType != child || childCandidates[0].Method.Signature != "(java.lang.String)" {
		t.Fatalf("child StaticMethodCandidates = %+v, want only direct interface static", childCandidates)
	}
}

func TestStaticMethodCandidatesAreSortedAndDefensive(t *testing.T) {
	base := fixtureType("Base", "z.Base", "Base.java", staticMethod("run", "long"))
	middle := fixtureType("Middle", "m.Middle", "Middle.java", staticMethod("run", "String"))
	middle.SuperClass = java.NewTypeRef("z.Base", false)
	child := fixtureType("Child", "a.Child", "Child.java", staticMethod("run"))
	child.SuperClass = java.NewTypeRef("m.Middle", false)
	table := buildStaticMethodTable(t, base, child, middle)

	got := table.StaticMethodCandidates("a.Child", "run")
	wantOwners := []string{"a.Child", "m.Middle", "z.Base"}
	owners := make([]string, len(got))
	for i, candidate := range got {
		owners[i] = candidate.DeclaringType.FQCN
	}
	if !reflect.DeepEqual(owners, wantOwners) {
		t.Fatalf("declaring owners = %v, want %v", owners, wantOwners)
	}
	got[0] = MethodResolution{}
	if fresh := table.StaticMethodCandidates("a.Child", "run"); len(fresh) != 3 || fresh[0].DeclaringType != child {
		t.Fatalf("StaticMethodCandidates exposed candidate slice: %+v", fresh)
	}
}

func TestStaticMethodCandidatesRequiresExactFQCNAndReturnsNonNil(t *testing.T) {
	table := buildStaticMethodTable(t, fixtureType("Util", "pkg.Util", "Util.java", staticMethod("run")))
	for _, owner := range []string{"Util", "missing.Util"} {
		if got := table.StaticMethodCandidates(owner, "run"); got == nil || len(got) != 0 {
			t.Fatalf("StaticMethodCandidates(%q) = %+v, want non-nil empty slice", owner, got)
		}
	}
	var nilTable *Table
	if got := nilTable.StaticMethodCandidates("pkg.Util", "run"); got == nil || len(got) != 0 {
		t.Fatalf("nil StaticMethodCandidates = %+v, want non-nil empty slice", got)
	}
}
