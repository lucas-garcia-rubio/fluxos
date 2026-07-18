package index

import (
	"sort"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
)

// ImplementationTable maps the canonical FQCN of every polymorphic type known
// to the project (every interface and every abstract class) to the concrete
// types that may satisfy it at runtime, sorted by candidate FQCN. Entries with
// zero implementations are kept so callers can distinguish "unknown type" from
// "known polymorphic type without implementations in the project".
type ImplementationTable map[string][]*java.TypeDecl

// ImplementationsOf returns the concrete types registered as implementations
// of the given polymorphic type. The returned slice is always non-nil: it is
// empty for nil tables, unknown types, concrete types, and known polymorphic
// types without any implementation. The slice is a defensive copy; mutating it
// does not affect the table's internal state.
func (t *Table) ImplementationsOf(typeFQCN string) []*java.TypeDecl {
	if t == nil {
		return []*java.TypeDecl{}
	}
	return cloneTypes(t.implementations[typeFQCN])
}

// isPolymorphicKey reports whether typ is an eligible key for the
// implementation table: an interface or an abstract class.
func isPolymorphicKey(typ *java.TypeDecl) bool {
	if typ == nil {
		return false
	}
	if typ.Kind == java.TypeKindInterface {
		return true
	}
	return typ.Kind == java.TypeKindClass && java.HasModifier(typ.Modifier, "abstract")
}

// isConcreteImplementation reports whether typ can appear as a runtime
// implementation: a class without the abstract modifier, a record, or an enum.
// Interfaces, abstract classes, and unknown kinds never qualify.
func isConcreteImplementation(typ *java.TypeDecl) bool {
	if typ == nil {
		return false
	}
	if java.HasModifier(typ.Modifier, "abstract") {
		return false
	}
	switch typ.Kind {
	case java.TypeKindClass, java.TypeKindRecord, java.TypeKindEnum:
		return true
	default:
		return false
	}
}

// buildImplementationTable populates the implementation table by classifying
// every indexed type as either a polymorphic key or a concrete candidate, then
// propagating each concrete candidate to every polymorphic ancestor reachable
// through the existing cycle-safe interface and superclass closures.
func (t *Table) buildImplementationTable() ImplementationTable {
	impls := make(ImplementationTable)
	if t == nil {
		return impls
	}

	orderedTypes := make([]*java.TypeDecl, 0, len(t.TypesByFQCN))
	for _, typ := range t.TypesByFQCN {
		orderedTypes = append(orderedTypes, typ)
	}
	sort.Slice(orderedTypes, func(i, j int) bool {
		return orderedTypes[i].FQCN < orderedTypes[j].FQCN
	})

	for _, typ := range orderedTypes {
		if isPolymorphicKey(typ) {
			impls[typ.FQCN] = []*java.TypeDecl{}
		}
	}

	for _, candidate := range orderedTypes {
		if !isConcreteImplementation(candidate) {
			continue
		}

		targets := make(map[string]struct{})
		for _, iface := range t.InterfaceClosure(candidate.FQCN) {
			if iface.Kind == java.TypeKindInterface {
				targets[iface.FQCN] = struct{}{}
			}
		}
		for _, ancestor := range t.SuperclassChain(candidate.FQCN) {
			if ancestor.Kind == java.TypeKindClass && java.HasModifier(ancestor.Modifier, "abstract") {
				targets[ancestor.FQCN] = struct{}{}
			}
		}

		if len(targets) == 0 {
			continue
		}
		orderedTargets := make([]string, 0, len(targets))
		for target := range targets {
			orderedTargets = append(orderedTargets, target)
		}
		sort.Strings(orderedTargets)

		for _, target := range orderedTargets {
			impls[target] = append(impls[target], candidate)
		}
	}

	return impls
}
