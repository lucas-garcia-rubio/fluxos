package index

import (
	"reflect"
	"testing"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
)

func hierarchyRef(fqcn string) java.TypeRef {
	return java.TypeRef{Raw: fqcn, FQCN: fqcn}
}

func hierarchyType(kind java.TypeKind, fqcn string) *java.TypeDecl {
	return &java.TypeDecl{Kind: kind, Name: simpleName(fqcn), FQCN: fqcn, File: fqcn + ".java"}
}

func hierarchyMethod(name string, modifiers []string, parameterTypes ...string) java.MethodDecl {
	method := java.MethodDecl{Name: name, Modifier: modifiers}
	for _, parameterType := range parameterTypes {
		ref := java.NewTypeRef(parameterType, false)
		if parameterType == "java.lang.String" {
			ref.FQCN = parameterType
			ref.Unresolved = false
		}
		method.Params = append(method.Params, java.Param{Type: ref})
	}
	return method
}

func hierarchyTable(t *testing.T, types ...*java.TypeDecl) *Table {
	t.Helper()
	units := make([]*java.CompilationUnit, 0, len(types))
	for _, typ := range types {
		pkg := ""
		for i := len(typ.FQCN) - 1; i >= 0; i-- {
			if typ.FQCN[i] == '.' {
				pkg = typ.FQCN[:i]
				break
			}
		}
		units = append(units, &java.CompilationUnit{File: typ.File, Package: pkg, Types: []*java.TypeDecl{typ}})
	}
	table, err := Build(units)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return table
}

func typeNames(types []*java.TypeDecl) []string {
	names := make([]string, len(types))
	for i, typ := range types {
		names[i] = typ.FQCN
	}
	return names
}

func resolutionOwners(resolutions []MethodResolution) []string {
	owners := make([]string, len(resolutions))
	for i, resolution := range resolutions {
		owners[i] = resolution.DeclaringType.FQCN + resolution.Method.Signature
	}
	return owners
}

func TestSuperclassTraversalIsCanonicalNearestFirstAndCycleSafe(t *testing.T) {
	child := hierarchyType(java.TypeKindClass, "app.Child")
	parent := hierarchyType(java.TypeKindClass, "base.Parent")
	grandparent := hierarchyType(java.TypeKindClass, "base.Grandparent")
	child.SuperClass = hierarchyRef(parent.FQCN)
	parent.SuperClass = hierarchyRef(grandparent.FQCN)
	grandparent.SuperClass = hierarchyRef(child.FQCN)
	table := hierarchyTable(t, grandparent, child, parent)

	if got, ok := table.DirectSuperclass(child.FQCN); !ok || got != parent {
		t.Fatalf("DirectSuperclass = %+v, %v", got, ok)
	}
	if got := typeNames(table.SuperclassChain(child.FQCN)); !reflect.DeepEqual(got, []string{parent.FQCN, grandparent.FQCN}) {
		t.Fatalf("SuperclassChain = %v", got)
	}

	unresolved := hierarchyType(java.TypeKindClass, "app.Unresolved")
	unresolved.SuperClass = java.TypeRef{Raw: "Parent", Unresolved: true}
	external := hierarchyType(java.TypeKindClass, "app.External")
	external.SuperClass = hierarchyRef("outside.Parent")
	table = hierarchyTable(t, unresolved, external, parent)
	if _, ok := table.DirectSuperclass(unresolved.FQCN); ok {
		t.Fatal("DirectSuperclass used a simple-name fallback")
	}
	if got := table.SuperclassChain(external.FQCN); len(got) != 0 {
		t.Fatalf("external SuperclassChain = %v", typeNames(got))
	}
}

func TestInterfaceClosureIsDeterministicAndCycleSafe(t *testing.T) {
	child := hierarchyType(java.TypeKindClass, "app.Child")
	parent := hierarchyType(java.TypeKindClass, "base.Parent")
	left := hierarchyType(java.TypeKindInterface, "contract.Left")
	right := hierarchyType(java.TypeKindInterface, "contract.Right")
	root := hierarchyType(java.TypeKindInterface, "contract.Root")
	fromParent := hierarchyType(java.TypeKindInterface, "contract.ParentContract")
	child.SuperClass = hierarchyRef(parent.FQCN)
	child.Interfaces = []java.TypeRef{hierarchyRef(right.FQCN), hierarchyRef(left.FQCN), hierarchyRef(left.FQCN), {Raw: "Missing", Unresolved: true}}
	left.Interfaces = []java.TypeRef{hierarchyRef(root.FQCN)}
	right.Interfaces = []java.TypeRef{hierarchyRef(root.FQCN)}
	root.Interfaces = []java.TypeRef{hierarchyRef(left.FQCN)}
	parent.Interfaces = []java.TypeRef{hierarchyRef(fromParent.FQCN)}
	table := hierarchyTable(t, root, right, parent, child, fromParent, left)

	if got := typeNames(table.DirectInterfaces(child.FQCN)); !reflect.DeepEqual(got, []string{left.FQCN, right.FQCN}) {
		t.Fatalf("DirectInterfaces = %v", got)
	}
	want := []string{left.FQCN, right.FQCN, fromParent.FQCN, root.FQCN}
	if got := typeNames(table.InterfaceClosure(child.FQCN)); !reflect.DeepEqual(got, want) {
		t.Fatalf("InterfaceClosure = %v, want %v", got, want)
	}
}

func TestEffectiveMethodsOverrideByFullKeyAndPreserveOwner(t *testing.T) {
	child := hierarchyType(java.TypeKindClass, "app.Child")
	parent := hierarchyType(java.TypeKindClass, "app.Parent")
	grandparent := hierarchyType(java.TypeKindClass, "app.Grandparent")
	child.SuperClass = hierarchyRef(parent.FQCN)
	parent.SuperClass = hierarchyRef(grandparent.FQCN)
	child.Methods = []java.MethodDecl{hierarchyMethod("run", []string{"public"}, "int")}
	parent.Methods = []java.MethodDecl{hierarchyMethod("run", []string{"public"})}
	grandparent.Methods = []java.MethodDecl{
		hierarchyMethod("run", []string{"public"}),
		hierarchyMethod("run", []string{"public"}, "java.lang.String"),
		hierarchyMethod("run", []string{"public"}, "long"),
	}
	grandparent.Methods[2].Kind = java.MethodConstructor
	table := hierarchyTable(t, grandparent, child, parent)

	want := []string{"app.Parent()", "app.Child(int)", "app.Grandparent(java.lang.String)"}
	if got := resolutionOwners(table.EffectiveMethodCandidates(child.FQCN, "run")); !reflect.DeepEqual(got, want) {
		t.Fatalf("EffectiveMethodCandidates = %v, want %v", got, want)
	}
	key := java.MethodKey{Name: "run", Signature: "()"}
	got := table.EffectiveMethod(child.FQCN, key)
	if len(got) != 1 || got[0].DeclaringType != parent || got[0].Method != &parent.Methods[0] {
		t.Fatalf("EffectiveMethod = %+v", got)
	}
}

func TestEffectiveMethodsApplyInheritedVisibility(t *testing.T) {
	child := hierarchyType(java.TypeKindClass, "child.Child")
	parent := hierarchyType(java.TypeKindClass, "base.Parent")
	grandparent := hierarchyType(java.TypeKindClass, "base.Grandparent")
	child.SuperClass = hierarchyRef(parent.FQCN)
	parent.SuperClass = hierarchyRef(grandparent.FQCN)
	parent.Methods = []java.MethodDecl{
		hierarchyMethod("visible", []string{"public"}),
		hierarchyMethod("visible", []string{"protected"}, "int"),
		hierarchyMethod("visible", nil, "long"),
		hierarchyMethod("visible", []string{"private"}, "byte"),
	}
	grandparent.Methods = []java.MethodDecl{hierarchyMethod("visible", []string{"protected"}, "java.lang.String")}
	table := hierarchyTable(t, child, parent, grandparent)

	want := []string{"base.Parent()", "base.Parent(int)", "base.Grandparent(java.lang.String)"}
	if got := resolutionOwners(table.EffectiveMethodCandidates(child.FQCN, "visible")); !reflect.DeepEqual(got, want) {
		t.Fatalf("cross-package methods = %v, want %v", got, want)
	}

	samePackage := hierarchyType(java.TypeKindClass, "base.Child")
	samePackage.SuperClass = hierarchyRef(parent.FQCN)
	table = hierarchyTable(t, samePackage, parent, grandparent)
	if got := table.EffectiveMethod(samePackage.FQCN, java.MethodKey{Name: "visible", Signature: "(long)"}); len(got) != 1 || got[0].DeclaringType != parent {
		t.Fatalf("same-package method = %+v", got)
	}
}

func TestPackagePrivateMembersDoNotReappearAcrossPackageBoundary(t *testing.T) {
	ancestor := hierarchyType(java.TypeKindClass, "p.Ancestor")
	middle := hierarchyType(java.TypeKindClass, "q.Middle")
	child := hierarchyType(java.TypeKindClass, "p.Child")
	middle.SuperClass = hierarchyRef(ancestor.FQCN)
	child.SuperClass = hierarchyRef(middle.FQCN)
	ancestor.Methods = []java.MethodDecl{hierarchyMethod("packageMethod", nil)}
	ancestor.Fields = []java.FieldDecl{{Name: "packageField"}}
	table := hierarchyTable(t, ancestor, middle, child)

	if got := table.EffectiveMethodCandidates(child.FQCN, "packageMethod"); len(got) != 0 {
		t.Fatalf("package-private method reappeared after crossing package: %+v", got)
	}
	if got, ok := table.EffectiveField(child.FQCN, "packageField"); ok {
		t.Fatalf("package-private field reappeared after crossing package: %+v", got)
	}
}

func TestEffectiveMethodDefaultsSpecificityAndAmbiguity(t *testing.T) {
	child := hierarchyType(java.TypeKindClass, "app.Child")
	left := hierarchyType(java.TypeKindInterface, "contract.Left")
	right := hierarchyType(java.TypeKindInterface, "contract.Right")
	root := hierarchyType(java.TypeKindInterface, "contract.Root")
	child.Interfaces = []java.TypeRef{hierarchyRef(right.FQCN), hierarchyRef(left.FQCN)}
	left.Interfaces = []java.TypeRef{hierarchyRef(root.FQCN)}
	defaultMethod := func(parameterType string, modifiers ...string) java.MethodDecl {
		if parameterType == "" {
			return hierarchyMethod("run", modifiers)
		}
		return hierarchyMethod("run", modifiers, parameterType)
	}
	root.Methods = []java.MethodDecl{defaultMethod("", "default")}
	left.Methods = []java.MethodDecl{defaultMethod("", "default"), defaultMethod("int", "default")}
	right.Methods = []java.MethodDecl{
		defaultMethod("int", "default"),
		defaultMethod("long", "default", "static"),
		defaultMethod("byte", "default", "private"),
	}
	table := hierarchyTable(t, root, right, child, left)

	if got := resolutionOwners(table.EffectiveMethod(child.FQCN, java.MethodKey{Name: "run", Signature: "()"})); !reflect.DeepEqual(got, []string{"contract.Left()"}) {
		t.Fatalf("specific default = %v", got)
	}
	want := []string{"contract.Left(int)", "contract.Right(int)"}
	if got := resolutionOwners(table.EffectiveMethod(child.FQCN, java.MethodKey{Name: "run", Signature: "(int)"})); !reflect.DeepEqual(got, want) {
		t.Fatalf("unrelated defaults = %v, want %v", got, want)
	}
	if got := table.EffectiveMethod(child.FQCN, java.MethodKey{Name: "run", Signature: "(long)"}); len(got) != 0 {
		t.Fatalf("static default candidates = %+v", got)
	}

	child.Methods = []java.MethodDecl{defaultMethod("int", "public")}
	table = hierarchyTable(t, root, right, child, left)
	if got := table.EffectiveMethod(child.FQCN, java.MethodKey{Name: "run", Signature: "(int)"}); len(got) != 1 || got[0].DeclaringType != child {
		t.Fatalf("class method did not block defaults: %+v", got)
	}
}

func TestEffectiveMethodAbstractSubinterfaceSuppressesParentDefault(t *testing.T) {
	child := hierarchyType(java.TypeKindClass, "app.Child")
	sub := hierarchyType(java.TypeKindInterface, "contract.Sub")
	root := hierarchyType(java.TypeKindInterface, "contract.Root")
	child.Interfaces = []java.TypeRef{hierarchyRef(sub.FQCN)}
	sub.Interfaces = []java.TypeRef{hierarchyRef(root.FQCN)}
	sub.Methods = []java.MethodDecl{hierarchyMethod("run", []string{"public"})}
	root.Methods = []java.MethodDecl{hierarchyMethod("run", []string{"default"})}
	table := hierarchyTable(t, root, child, sub)

	if got := table.EffectiveMethod(child.FQCN, java.MethodKey{Name: "run", Signature: "()"}); len(got) != 0 {
		t.Fatalf("abstract redeclaration left parent default effective: %+v", got)
	}
}

func TestEffectiveFieldHidingVisibilityAndOwner(t *testing.T) {
	child := hierarchyType(java.TypeKindClass, "child.Child")
	parent := hierarchyType(java.TypeKindClass, "base.Parent")
	grandparent := hierarchyType(java.TypeKindClass, "base.Grandparent")
	child.SuperClass = hierarchyRef(parent.FQCN)
	parent.SuperClass = hierarchyRef(grandparent.FQCN)
	child.Fields = []java.FieldDecl{{Name: "hidden", Modifier: []string{"private"}}}
	parent.Fields = []java.FieldDecl{
		{Name: "hidden", Modifier: []string{"public"}},
		{Name: "inherited", Modifier: []string{"protected"}},
		{Name: "packageOnly"},
		{Name: "privateOnly", Modifier: []string{"private"}},
	}
	grandparent.Fields = []java.FieldDecl{{Name: "deep", Modifier: []string{"public"}}}
	table := hierarchyTable(t, grandparent, child, parent)

	for _, test := range []struct {
		name  string
		owner *java.TypeDecl
	}{
		{name: "hidden", owner: child},
		{name: "inherited", owner: parent},
		{name: "deep", owner: grandparent},
	} {
		got, ok := table.EffectiveField(child.FQCN, test.name)
		if !ok || got.DeclaringType != test.owner {
			t.Fatalf("EffectiveField(%q) = %+v, %v", test.name, got, ok)
		}
	}
	for _, name := range []string{"packageOnly", "privateOnly", "missing"} {
		if got, ok := table.EffectiveField(child.FQCN, name); ok {
			t.Fatalf("EffectiveField(%q) = %+v", name, got)
		}
	}

	samePackage := hierarchyType(java.TypeKindClass, "base.Child")
	samePackage.SuperClass = hierarchyRef(parent.FQCN)
	table = hierarchyTable(t, samePackage, parent, grandparent)
	if got, ok := table.EffectiveField(samePackage.FQCN, "packageOnly"); !ok || got.DeclaringType != parent || got.Field != &parent.Fields[2] {
		t.Fatalf("same-package field = %+v, %v", got, ok)
	}
}

func TestNilHierarchyLookupsReturnEmptyResults(t *testing.T) {
	var table *Table
	if _, ok := table.DirectSuperclass("missing.Type"); ok {
		t.Fatal("nil DirectSuperclass succeeded")
	}
	if table.SuperclassChain("missing.Type") == nil || table.DirectInterfaces("missing.Type") == nil || table.InterfaceClosure("missing.Type") == nil {
		t.Fatal("nil hierarchy slices must be non-nil")
	}
	if table.EffectiveMethod(java.MethodKey{}.Name, java.MethodKey{}) == nil || table.EffectiveMethodCandidates("missing.Type", "run") == nil {
		t.Fatal("nil method slices must be non-nil")
	}
	if _, ok := table.EffectiveField("missing.Type", "field"); ok {
		t.Fatal("nil EffectiveField succeeded")
	}
}
