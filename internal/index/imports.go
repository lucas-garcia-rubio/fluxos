package index

import (
	"sort"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
)

// StaticMethodCandidates returns static methods that are members of the exact owner.
func (t *Table) StaticMethodCandidates(typeFQCN, name string) []MethodResolution {
	if t == nil {
		return []MethodResolution{}
	}
	owner, ok := t.TypeByFQCN(typeFQCN)
	if !ok {
		return []MethodResolution{}
	}

	type candidateKey struct {
		declaringType string
		method        java.MethodKey
	}
	seen := make(map[candidateKey]struct{})
	candidates := make([]MethodResolution, 0)
	for _, candidate := range t.EffectiveMethodCandidates(typeFQCN, name) {
		if candidate.DeclaringType == nil || candidate.Method == nil {
			continue
		}
		method := candidate.Method
		if !java.HasModifier(method.Modifier, "static") || method.Kind == java.MethodConstructor || method.Kind == java.MethodCompactConstructor || method.Name == "<init>" {
			continue
		}
		if owner.Kind == java.TypeKindInterface && candidate.DeclaringType.FQCN != owner.FQCN {
			continue
		}

		key := candidateKey{declaringType: candidate.DeclaringType.FQCN, method: method.Key()}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		candidates = append(candidates, candidate)
	}

	sort.Slice(candidates, func(i, j int) bool {
		left, right := candidates[i], candidates[j]
		if left.DeclaringType.FQCN != right.DeclaringType.FQCN {
			return left.DeclaringType.FQCN < right.DeclaringType.FQCN
		}
		if left.Method.Name != right.Method.Name {
			return left.Method.Name < right.Method.Name
		}
		return left.Method.Signature < right.Method.Signature
	})
	return candidates
}
