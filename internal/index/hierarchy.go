package index

import (
	"sort"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
)

type MethodResolution struct {
	DeclaringType *java.TypeDecl
	Method        *java.MethodDecl
}

type FieldResolution struct {
	DeclaringType *java.TypeDecl
	Field         *java.FieldDecl
}

func (t *Table) DirectSuperclass(typeFQCN string) (*java.TypeDecl, bool) {
	if t == nil {
		return nil, false
	}
	typ, ok := t.TypesByFQCN[typeFQCN]
	if !ok || typ.SuperClass.FQCN == "" {
		return nil, false
	}
	superclass, ok := t.TypesByFQCN[typ.SuperClass.FQCN]
	return superclass, ok
}

func (t *Table) SuperclassChain(typeFQCN string) []*java.TypeDecl {
	result := make([]*java.TypeDecl, 0)
	visited := map[string]struct{}{typeFQCN: {}}
	current := typeFQCN
	for {
		superclass, ok := t.DirectSuperclass(current)
		if !ok {
			return result
		}
		if _, seen := visited[superclass.FQCN]; seen {
			return result
		}
		visited[superclass.FQCN] = struct{}{}
		result = append(result, superclass)
		current = superclass.FQCN
	}
}

func (t *Table) DirectInterfaces(typeFQCN string) []*java.TypeDecl {
	result := make([]*java.TypeDecl, 0)
	if t == nil {
		return result
	}
	typ, ok := t.TypesByFQCN[typeFQCN]
	if !ok {
		return result
	}
	seen := make(map[string]struct{}, len(typ.Interfaces))
	for _, ref := range typ.Interfaces {
		if ref.FQCN == "" {
			continue
		}
		iface, exists := t.TypesByFQCN[ref.FQCN]
		if !exists {
			continue
		}
		if _, duplicate := seen[iface.FQCN]; duplicate {
			continue
		}
		seen[iface.FQCN] = struct{}{}
		result = append(result, iface)
	}
	sortTypes(result)
	return result
}

func (t *Table) InterfaceClosure(typeFQCN string) []*java.TypeDecl {
	interfaces, _ := t.interfaceClosure(typeFQCN)
	return interfaces
}

func (t *Table) interfaceClosure(typeFQCN string) ([]*java.TypeDecl, map[string]int) {
	type hierarchyNode struct {
		typ      *java.TypeDecl
		distance int
	}

	result := make([]*java.TypeDecl, 0)
	distances := make(map[string]int)
	if t == nil {
		return result, distances
	}
	root, ok := t.TypesByFQCN[typeFQCN]
	if !ok {
		return result, distances
	}

	visited := map[string]struct{}{typeFQCN: {}}
	queue := []hierarchyNode{{typ: root, distance: 0}}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, iface := range t.DirectInterfaces(current.typ.FQCN) {
			if _, seen := visited[iface.FQCN]; seen {
				continue
			}
			visited[iface.FQCN] = struct{}{}
			distances[iface.FQCN] = current.distance + 1
			result = append(result, iface)
			queue = append(queue, hierarchyNode{typ: iface, distance: current.distance + 1})
		}
		if superclass, exists := t.DirectSuperclass(current.typ.FQCN); exists {
			if _, seen := visited[superclass.FQCN]; !seen {
				visited[superclass.FQCN] = struct{}{}
				queue = append(queue, hierarchyNode{typ: superclass, distance: current.distance + 1})
			}
		}
	}

	sort.Slice(result, func(i, j int) bool {
		left, right := result[i], result[j]
		if distances[left.FQCN] != distances[right.FQCN] {
			return distances[left.FQCN] < distances[right.FQCN]
		}
		return left.FQCN < right.FQCN
	})
	return result, distances
}

func (t *Table) EffectiveMethod(typeFQCN string, key java.MethodKey) []MethodResolution {
	result := make([]MethodResolution, 0)
	for _, candidate := range t.EffectiveMethodCandidates(typeFQCN, key.Name) {
		if candidate.Method.Key() == key {
			result = append(result, candidate)
		}
	}
	return result
}

func (t *Table) EffectiveMethodCandidates(typeFQCN, name string) []MethodResolution {
	result := make([]MethodResolution, 0)
	if t == nil {
		return result
	}
	initial, ok := t.TypesByFQCN[typeFQCN]
	if !ok {
		return result
	}

	classDistances := make(map[string]int)
	selectedByClass := make(map[java.MethodKey]struct{})
	classTypes := append([]*java.TypeDecl{initial}, t.SuperclassChain(typeFQCN)...)
	for distance, owner := range classTypes {
		classDistances[owner.FQCN] = distance
		for i := range owner.Methods {
			method := &owner.Methods[i]
			if method.Name != name || method.Kind == java.MethodConstructor || method.Kind == java.MethodCompactConstructor {
				continue
			}
			if distance > 0 && !t.inheritableAlong(classTypes, distance, method.Modifier) {
				continue
			}
			key := method.Key()
			if _, selected := selectedByClass[key]; selected {
				continue
			}
			selectedByClass[key] = struct{}{}
			result = append(result, MethodResolution{DeclaringType: owner, Method: method})
		}
	}

	interfaces, interfaceDistances := t.interfaceClosure(typeFQCN)
	defaultsByKey := make(map[java.MethodKey][]MethodResolution)
	declarationsByKey := make(map[java.MethodKey][]*java.TypeDecl)
	for _, owner := range interfaces {
		for i := range owner.Methods {
			method := &owner.Methods[i]
			key := method.Key()
			if method.Name != name || method.Kind == java.MethodConstructor || method.Kind == java.MethodCompactConstructor || java.HasModifier(method.Modifier, "static") || java.HasModifier(method.Modifier, "private") {
				continue
			}
			if _, covered := selectedByClass[key]; covered {
				continue
			}
			declarationsByKey[key] = append(declarationsByKey[key], owner)
			if !java.HasModifier(method.Modifier, "default") {
				continue
			}
			defaultsByKey[key] = append(defaultsByKey[key], MethodResolution{DeclaringType: owner, Method: method})
		}
	}
	for key, candidates := range defaultsByKey {
		for _, candidate := range candidates {
			lessSpecific := false
			for _, other := range declarationsByKey[key] {
				if other != candidate.DeclaringType && t.interfaceExtends(other.FQCN, candidate.DeclaringType.FQCN) {
					lessSpecific = true
					break
				}
			}
			if !lessSpecific {
				result = append(result, candidate)
			}
		}
	}

	sort.Slice(result, func(i, j int) bool {
		left, right := result[i], result[j]
		if left.Method.Name != right.Method.Name {
			return left.Method.Name < right.Method.Name
		}
		if left.Method.Signature != right.Method.Signature {
			return left.Method.Signature < right.Method.Signature
		}
		leftDistance, leftClass := classDistances[left.DeclaringType.FQCN]
		rightDistance, rightClass := classDistances[right.DeclaringType.FQCN]
		if !leftClass {
			leftDistance = interfaceDistances[left.DeclaringType.FQCN]
		}
		if !rightClass {
			rightDistance = interfaceDistances[right.DeclaringType.FQCN]
		}
		if leftDistance != rightDistance {
			return leftDistance < rightDistance
		}
		return left.DeclaringType.FQCN < right.DeclaringType.FQCN
	})
	return result
}

func (t *Table) EffectiveField(typeFQCN, name string) (FieldResolution, bool) {
	if t == nil {
		return FieldResolution{}, false
	}
	initial, ok := t.TypesByFQCN[typeFQCN]
	if !ok {
		return FieldResolution{}, false
	}
	owners := append([]*java.TypeDecl{initial}, t.SuperclassChain(typeFQCN)...)
	for distance, owner := range owners {
		for i := range owner.Fields {
			field := &owner.Fields[i]
			if field.Name != name || distance > 0 && !t.inheritableAlong(owners, distance, field.Modifier) {
				continue
			}
			return FieldResolution{DeclaringType: owner, Field: field}, true
		}
	}
	return FieldResolution{}, false
}

func (t *Table) inheritableAlong(chain []*java.TypeDecl, ownerIndex int, modifiers []string) bool {
	if java.HasModifier(modifiers, "private") {
		return false
	}
	if java.HasModifier(modifiers, "public") || java.HasModifier(modifiers, "protected") {
		return true
	}
	if ownerIndex < 1 || ownerIndex >= len(chain) {
		return false
	}
	ownerUnit := t.UnitForType(chain[ownerIndex].FQCN)
	if ownerUnit == nil {
		return false
	}
	for _, descendant := range chain[:ownerIndex] {
		descendantUnit := t.UnitForType(descendant.FQCN)
		if descendantUnit == nil || descendantUnit.Package != ownerUnit.Package {
			return false
		}
	}
	return true
}

func (t *Table) interfaceExtends(descendantFQCN, ancestorFQCN string) bool {
	visited := map[string]struct{}{descendantFQCN: {}}
	queue := []string{descendantFQCN}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, parent := range t.DirectInterfaces(current) {
			if parent.FQCN == ancestorFQCN {
				return true
			}
			if _, seen := visited[parent.FQCN]; seen {
				continue
			}
			visited[parent.FQCN] = struct{}{}
			queue = append(queue, parent.FQCN)
		}
	}
	return false
}
