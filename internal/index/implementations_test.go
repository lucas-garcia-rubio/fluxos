package index

import (
	"reflect"
	"testing"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
)

func implType(kind java.TypeKind, fqcn string, modifiers ...string) *java.TypeDecl {
	return &java.TypeDecl{
		Kind:     kind,
		Name:     simpleName(fqcn),
		FQCN:     fqcn,
		Modifier: modifiers,
		File:     fqcn + ".java",
	}
}

func implClass(fqcn string, modifiers ...string) *java.TypeDecl {
	return implType(java.TypeKindClass, fqcn, modifiers...)
}
func implInterface(fqcn string) *java.TypeDecl {
	return implType(java.TypeKindInterface, fqcn)
}
func implRecord(fqcn string) *java.TypeDecl {
	return implType(java.TypeKindRecord, fqcn)
}
func implEnum(fqcn string) *java.TypeDecl {
	return implType(java.TypeKindEnum, fqcn)
}

func implNames(types []*java.TypeDecl) []string {
	return typeNames(types)
}

// assertImplementations fails the test if ImplementationsOf(fqcn) does not match
// the wantFQCNs slice exactly, in order. When wantFQCNs is empty it only checks
// that the returned slice is empty, avoiding the nil-vs-empty ambiguity of
// reflect.DeepEqual on []string.
func assertImplementations(t *testing.T, table *Table, fqcn string, wantFQCNs ...string) {
	t.Helper()
	got := implNames(table.ImplementationsOf(fqcn))
	if len(wantFQCNs) == 0 {
		if len(got) != 0 {
			t.Fatalf("ImplementationsOf(%q) = %v, want empty", fqcn, got)
		}
		return
	}
	want := append([]string(nil), wantFQCNs...)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ImplementationsOf(%q) = %v, want %v", fqcn, got, want)
	}
}

func TestImplementationTableDirectInterface(t *testing.T) {
	iface := implInterface("contract.Service")
	impl := implClass("impl.ServiceImpl")
	impl.Interfaces = []java.TypeRef{hierarchyRef(iface.FQCN)}
	table := hierarchyTable(t, iface, impl)

	assertImplementations(t, table, iface.FQCN, impl.FQCN)
}

func TestImplementationTableIncludesConcreteSubclass(t *testing.T) {
	iface := implInterface("contract.Service")
	parent := implClass("impl.ParentService")
	parent.Interfaces = []java.TypeRef{hierarchyRef(iface.FQCN)}
	child := implClass("impl.ChildService")
	child.SuperClass = hierarchyRef(parent.FQCN)
	table := hierarchyTable(t, iface, parent, child)

	assertImplementations(t, table, iface.FQCN, child.FQCN, parent.FQCN)
}

func TestImplementationTablePropagatesSubinterface(t *testing.T) {
	root := implInterface("contract.Root")
	child := implInterface("contract.Child")
	child.Interfaces = []java.TypeRef{hierarchyRef(root.FQCN)}
	impl := implClass("impl.Concrete")
	impl.Interfaces = []java.TypeRef{hierarchyRef(child.FQCN)}
	table := hierarchyTable(t, root, child, impl)

	assertImplementations(t, table, child.FQCN, impl.FQCN)
	assertImplementations(t, table, root.FQCN, impl.FQCN)
}

func TestImplementationTableIncludesAbstractAncestors(t *testing.T) {
	base := implClass("base.AbstractBase", "abstract")
	middle := implClass("base.AbstractMiddle", "abstract")
	middle.SuperClass = hierarchyRef(base.FQCN)
	concrete := implClass("base.ConcreteBase")
	concrete.SuperClass = hierarchyRef(middle.FQCN)
	table := hierarchyTable(t, base, middle, concrete)

	assertImplementations(t, table, base.FQCN, concrete.FQCN)
	assertImplementations(t, table, middle.FQCN, concrete.FQCN)
}

func TestImplementationTableIncludesInterfaceThroughAbstractSuperclass(t *testing.T) {
	iface := implInterface("contract.Service")
	abstractBase := implClass("base.AbstractBase", "abstract")
	abstractBase.Interfaces = []java.TypeRef{hierarchyRef(iface.FQCN)}
	concrete := implClass("base.Concrete")
	concrete.SuperClass = hierarchyRef(abstractBase.FQCN)
	table := hierarchyTable(t, iface, abstractBase, concrete)

	assertImplementations(t, table, iface.FQCN, concrete.FQCN)
	assertImplementations(t, table, abstractBase.FQCN, concrete.FQCN)
}

func TestImplementationTableIncludesConcreteAncestorAndDescendant(t *testing.T) {
	iface := implInterface("contract.Service")
	ancestor := implClass("impl.ConcreteAncestor")
	ancestor.Interfaces = []java.TypeRef{hierarchyRef(iface.FQCN)}
	descendant := implClass("impl.ConcreteDescendant")
	descendant.SuperClass = hierarchyRef(ancestor.FQCN)
	table := hierarchyTable(t, iface, ancestor, descendant)

	assertImplementations(t, table, iface.FQCN, ancestor.FQCN, descendant.FQCN)
}

func TestImplementationTableAcceptsClassRecordAndEnum(t *testing.T) {
	iface := implInterface("kinds.Contract")
	classImpl := implClass("kinds.ClassImpl")
	classImpl.Interfaces = []java.TypeRef{hierarchyRef(iface.FQCN)}
	recordImpl := implRecord("kinds.DataRecord")
	recordImpl.Interfaces = []java.TypeRef{hierarchyRef(iface.FQCN)}
	enumImpl := implEnum("kinds.ModeEnum")
	enumImpl.Interfaces = []java.TypeRef{hierarchyRef(iface.FQCN)}
	table := hierarchyTable(t, iface, classImpl, recordImpl, enumImpl)

	// Canonical FQCN order: ClassImpl < DataRecord < ModeEnum.
	assertImplementations(t, table, iface.FQCN, classImpl.FQCN, recordImpl.FQCN, enumImpl.FQCN)
}

func TestImplementationTableRejectsInterfaceAndAbstractCandidates(t *testing.T) {
	root := implInterface("contract.Root")
	middle := implInterface("contract.Middle")
	middle.Interfaces = []java.TypeRef{hierarchyRef(root.FQCN)}
	abstractImpl := implClass("impl.AbstractService", "abstract")
	abstractImpl.Interfaces = []java.TypeRef{hierarchyRef(middle.FQCN)}
	table := hierarchyTable(t, root, middle, abstractImpl)

	// Abstract classes never appear as candidates, but they remain polymorphic
	// keys with an explicit zero-implementation entry.
	assertImplementations(t, table, root.FQCN)
	assertImplementations(t, table, middle.FQCN)
	assertImplementations(t, table, abstractImpl.FQCN)
}

func TestImplementationTableDeduplicatesDiamond(t *testing.T) {
	root := implInterface("contract.Root")
	left := implInterface("contract.Left")
	right := implInterface("contract.Right")
	left.Interfaces = []java.TypeRef{hierarchyRef(root.FQCN)}
	right.Interfaces = []java.TypeRef{hierarchyRef(root.FQCN)}
	impl := implClass("impl.Service")
	impl.Interfaces = []java.TypeRef{hierarchyRef(left.FQCN), hierarchyRef(right.FQCN)}
	table := hierarchyTable(t, root, left, right, impl)

	assertImplementations(t, table, root.FQCN, impl.FQCN)
	assertImplementations(t, table, left.FQCN, impl.FQCN)
	assertImplementations(t, table, right.FQCN, impl.FQCN)
}

func TestImplementationTableDeduplicatesRepeatedInterfaceRefs(t *testing.T) {
	iface := implInterface("contract.Service")
	impl := implClass("impl.ServiceImpl")
	impl.Interfaces = []java.TypeRef{hierarchyRef(iface.FQCN), hierarchyRef(iface.FQCN), hierarchyRef(iface.FQCN)}
	table := hierarchyTable(t, iface, impl)

	assertImplementations(t, table, iface.FQCN, impl.FQCN)
}

func TestImplementationTableStopsAtInterfaceCycle(t *testing.T) {
	left := implInterface("contract.Left")
	right := implInterface("contract.Right")
	left.Interfaces = []java.TypeRef{hierarchyRef(right.FQCN)}
	right.Interfaces = []java.TypeRef{hierarchyRef(left.FQCN)}
	impl := implClass("impl.Service")
	impl.Interfaces = []java.TypeRef{hierarchyRef(left.FQCN)}
	table := hierarchyTable(t, left, right, impl)

	assertImplementations(t, table, left.FQCN, impl.FQCN)
	assertImplementations(t, table, right.FQCN, impl.FQCN)
}

func TestImplementationTableStopsAtSuperclassCycle(t *testing.T) {
	abstractA := implClass("base.AbstractA", "abstract")
	abstractB := implClass("base.AbstractB", "abstract")
	abstractA.SuperClass = hierarchyRef(abstractB.FQCN)
	abstractB.SuperClass = hierarchyRef(abstractA.FQCN)
	concrete := implClass("base.Concrete")
	concrete.SuperClass = hierarchyRef(abstractA.FQCN)
	table := hierarchyTable(t, abstractA, abstractB, concrete)

	assertImplementations(t, table, abstractA.FQCN, concrete.FQCN)
	assertImplementations(t, table, abstractB.FQCN, concrete.FQCN)
}

func TestImplementationTableIgnoresUnresolvedAndExternalRefs(t *testing.T) {
	iface := implInterface("contract.Service")
	impl := implClass("impl.ServiceImpl")
	impl.Interfaces = []java.TypeRef{
		hierarchyRef(iface.FQCN),
		{Raw: "Missing", Unresolved: true},
		{Raw: "outside.External"},
	}
	table := hierarchyTable(t, iface, impl)

	// Unresolved refs and external (non-registered) refs do not create keys or
	// candidates; only contract.Service receives the implementation.
	if got := len(table.implementations); got != 1 {
		t.Fatalf("implementation table size = %d, want 1 (got %v)", got, table.implementations)
	}
	assertImplementations(t, table, iface.FQCN, impl.FQCN)
	assertImplementations(t, table, "Missing")
	assertImplementations(t, table, "outside.External")
}

func TestImplementationTableBuildsAfterCanonicalization(t *testing.T) {
	// ServiceImpl references the interface by simple name + import; only after
	// canonicalization does Interfaces[0].FQCN become "contract.Service", which
	// is what the implementation table needs as a key.
	serviceIface := &java.TypeDecl{
		Kind: java.TypeKindInterface,
		Name: "Service", FQCN: "contract.Service",
		File: "contract/Service.java",
		Modifier:     []string{},
		Interfaces:  []java.TypeRef{},
		Fields:      []java.FieldDecl{},
		Methods:     []java.MethodDecl{},
	}
	serviceImpl := &java.TypeDecl{
		Kind:         java.TypeKindClass,
		Name:         "ServiceImpl", FQCN: "impl.ServiceImpl",
		File:         "impl/ServiceImpl.java",
		Modifier:     []string{},
		Interfaces:   []java.TypeRef{{Raw: "Service"}},
		Fields:       []java.FieldDecl{},
		Methods:      []java.MethodDecl{},
	}
	units := []*java.CompilationUnit{
		{File: "contract/Service.java", Package: "contract", Types: []*java.TypeDecl{serviceIface}},
		{File: "impl/ServiceImpl.java", Package: "impl", Imports: []java.ImportDecl{{Target: "contract.Service"}}, Types: []*java.TypeDecl{serviceImpl}},
	}
	table, err := Build(units)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	assertImplementations(t, table, "contract.Service", "impl.ServiceImpl")
}

func TestImplementationTableSupportsNestedTypes(t *testing.T) {
	outer := implClass("nested.Outer")
	contract := implType(java.TypeKindInterface, "nested.Outer.Contract")
	contract.EnclosingFQCN = outer.FQCN
	base := implClass("nested.Outer.Base", "abstract", "static")
	base.EnclosingFQCN = outer.FQCN
	base.Interfaces = []java.TypeRef{hierarchyRef(contract.FQCN)}
	impl := implClass("nested.Outer.Impl", "static")
	impl.EnclosingFQCN = outer.FQCN
	impl.SuperClass = hierarchyRef(base.FQCN)
	innerImpl := implClass("nested.Outer.InnerImpl")
	innerImpl.EnclosingFQCN = outer.FQCN
	innerImpl.Interfaces = []java.TypeRef{hierarchyRef(contract.FQCN)}

	table := hierarchyTable(t, outer, contract, base, impl, innerImpl)

	assertImplementations(t, table, contract.FQCN, impl.FQCN, innerImpl.FQCN)
	assertImplementations(t, table, base.FQCN, impl.FQCN)
}

func TestImplementationTableIncludesNonStaticInnerCandidate(t *testing.T) {
	outer := implClass("nested.Outer")
	iface := implInterface("nested.Outer.Contract")
	iface.EnclosingFQCN = outer.FQCN
	innerImpl := implClass("nested.Outer.InnerImpl")
	innerImpl.EnclosingFQCN = outer.FQCN
	innerImpl.Interfaces = []java.TypeRef{hierarchyRef(iface.FQCN)}

	table := hierarchyTable(t, outer, iface, innerImpl)

	// A non-static inner class is a valid runtime type even though it requires
	// an enclosing instance to construct. Membership is unaffected by
	// accessibility or constructibility.
	assertImplementations(t, table, iface.FQCN, innerImpl.FQCN)
}

func TestImplementationTableInitializesZeroImplementationEntries(t *testing.T) {
	emptyIface := implInterface("contract.Empty")
	emptyAbstract := implClass("base.EmptyAbstract", "abstract")
	unrelated := implClass("impl.Unrelated")
	table := hierarchyTable(t, emptyIface, emptyAbstract, unrelated)

	if _, ok := table.implementations[emptyIface.FQCN]; !ok {
		t.Errorf("missing entry for interface %q", emptyIface.FQCN)
	} else if got := table.ImplementationsOf(emptyIface.FQCN); len(got) != 0 {
		t.Errorf("ImplementationsOf(%q) = %v, want empty", emptyIface.FQCN, got)
	}
	if _, ok := table.implementations[emptyAbstract.FQCN]; !ok {
		t.Errorf("missing entry for abstract class %q", emptyAbstract.FQCN)
	} else if got := table.ImplementationsOf(emptyAbstract.FQCN); len(got) != 0 {
		t.Errorf("ImplementationsOf(%q) = %v, want empty", emptyAbstract.FQCN, got)
	}
	if _, ok := table.implementations[unrelated.FQCN]; ok {
		t.Errorf("concrete class %q should not be a key", unrelated.FQCN)
	}
}

func TestImplementationTableIsDeterministicAcrossInputOrder(t *testing.T) {
	iface := implInterface("contract.Service")
	alpha := implClass("impl.AlphaService")
	alpha.Interfaces = []java.TypeRef{hierarchyRef(iface.FQCN)}
	beta := implClass("impl.BetaService")
	beta.Interfaces = []java.TypeRef{hierarchyRef(iface.FQCN)}
	gamma := implClass("impl.GammaService")
	gamma.Interfaces = []java.TypeRef{hierarchyRef(iface.FQCN)}

	build := func(order ...*java.TypeDecl) []string {
		return implNames(hierarchyTable(t, order...).ImplementationsOf(iface.FQCN))
	}
	want := []string{alpha.FQCN, beta.FQCN, gamma.FQCN}
	if got := build(iface, gamma, alpha, beta); !reflect.DeepEqual(got, want) {
		t.Fatalf("reverse-ish order: got %v want %v", got, want)
	}
	if got := build(iface, beta, gamma, alpha); !reflect.DeepEqual(got, want) {
		t.Fatalf("shuffled order: got %v want %v", got, want)
	}
}

func TestBuildInitializesEmptyImplementationTable(t *testing.T) {
	table, err := Build(nil)
	if err != nil {
		t.Fatalf("Build(nil): %v", err)
	}
	if table.implementations == nil {
		t.Fatal("implementations field is nil after Build(nil)")
	}
	if got := table.ImplementationsOf("anything"); len(got) != 0 {
		t.Fatalf("ImplementationsOf on empty table = %v, want empty", got)
	}
}

func TestImplementationsOfReturnsDefensiveCopy(t *testing.T) {
	iface := implInterface("contract.Service")
	impl := implClass("impl.ServiceImpl")
	impl.Interfaces = []java.TypeRef{hierarchyRef(iface.FQCN)}
	table := hierarchyTable(t, iface, impl)

	first := table.ImplementationsOf(iface.FQCN)
	original := append([]string(nil), implNames(first)...)
	first[0] = nil // mutate caller copy

	got := implNames(table.ImplementationsOf(iface.FQCN))
	if !reflect.DeepEqual(got, original) {
		t.Fatalf("ImplementationsOf mutated by caller: got %v want %v", got, original)
	}
}

func TestImplementationsOfPreservesCanonicalPointers(t *testing.T) {
	iface := implInterface("contract.Service")
	impl := implClass("impl.ServiceImpl")
	impl.Interfaces = []java.TypeRef{hierarchyRef(iface.FQCN)}
	table := hierarchyTable(t, iface, impl)

	got := table.ImplementationsOf(iface.FQCN)
	if len(got) != 1 || got[0] != impl {
		t.Fatalf("ImplementationsOf canonical pointer mismatch: got %+v, want %v", got, impl)
	}
	if canonical := table.TypesByFQCN[impl.FQCN]; canonical != impl {
		t.Fatalf("TypesByFQCN[%q] = %p, want %p (canonical pointer identity)", impl.FQCN, canonical, impl)
	}
}

func TestImplementationsOfNilUnknownAndConcreteAreNonNilEmpty(t *testing.T) {
	var nilTable *Table
	if got := nilTable.ImplementationsOf("contract.Service"); got == nil || len(got) != 0 {
		t.Fatalf("nilTable.ImplementationsOf = %v, want non-nil empty", got)
	}

	iface := implInterface("contract.Service")
	impl := implClass("impl.ServiceImpl")
	impl.Interfaces = []java.TypeRef{hierarchyRef(iface.FQCN)}
	table := hierarchyTable(t, iface, impl)

	if got := table.ImplementationsOf("contract.Unknown"); got == nil || len(got) != 0 {
		t.Fatalf("ImplementationsOf(unknown) = %v, want non-nil empty", got)
	}
	if got := table.ImplementationsOf(impl.FQCN); got == nil || len(got) != 0 {
		t.Fatalf("ImplementationsOf(concrete) = %v, want non-nil empty", got)
	}
}
