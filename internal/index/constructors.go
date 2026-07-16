package index

import (
	"sort"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
)

// ConstructorCandidates returns constructors declared directly by typeFQCN.
func (t *Table) ConstructorCandidates(typeFQCN string) []MethodResolution {
	candidates := make([]MethodResolution, 0)
	if t == nil {
		return candidates
	}

	typ, ok := t.TypesByFQCN[typeFQCN]
	if !ok {
		return candidates
	}
	for _, method := range t.MethodsByType[typeFQCN] {
		if !isConstructor(method) {
			continue
		}
		candidates = append(candidates, MethodResolution{DeclaringType: typ, Method: method})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Method.Signature < candidates[j].Method.Signature
	})
	return candidates
}

// Constructor looks up a constructor declared directly by typeFQCN.
func (t *Table) Constructor(typeFQCN, signature string) (MethodResolution, bool) {
	if t == nil {
		return MethodResolution{}, false
	}

	typ, ok := t.TypesByFQCN[typeFQCN]
	if !ok {
		return MethodResolution{}, false
	}
	method, ok := t.MethodsByType[typeFQCN][java.MethodKey{Name: "<init>", Signature: signature}]
	if !ok || !isConstructor(method) {
		return MethodResolution{}, false
	}
	return MethodResolution{DeclaringType: typ, Method: method}, true
}

func synthesizeImplicitConstructor(typ *java.TypeDecl) {
	switch typ.Kind {
	case java.TypeKindClass:
		for i := range typ.Methods {
			if isConstructor(&typ.Methods[i]) {
				return
			}
		}
		typ.Methods = append(typ.Methods, syntheticConstructor(typ, []java.Param{}))
	case java.TypeKindRecord:
		canonical := java.MethodDecl{Params: append([]java.Param{}, typ.RecordComponents...)}
		java.RebuildSignature(&canonical)
		for i := range typ.Methods {
			method := &typ.Methods[i]
			if !isConstructor(method) {
				continue
			}
			if method.Kind == java.MethodCompactConstructor || method.Signature == canonical.Signature {
				return
			}
		}
		typ.Methods = append(typ.Methods, syntheticConstructor(typ, canonical.Params))
	}
}

func syntheticConstructor(typ *java.TypeDecl, params []java.Param) java.MethodDecl {
	method := java.MethodDecl{
		Kind:      java.MethodConstructor,
		Name:      "<init>",
		Modifier:  constructorAccessibility(typ.Modifier),
		Params:    append([]java.Param{}, params...),
		Calls:     make([]java.CallSite, 0),
		LocalVars: []java.LocalVarDecl{},
		Synthetic: true,
	}
	java.RebuildSignature(&method)
	return method
}

func constructorAccessibility(modifiers []string) []string {
	for _, accessibility := range []string{"public", "protected", "private"} {
		if java.HasModifier(modifiers, accessibility) {
			return []string{accessibility}
		}
	}
	return []string{}
}

func isConstructor(method *java.MethodDecl) bool {
	return method != nil && method.Name == "<init>" &&
		(method.Kind == java.MethodConstructor || method.Kind == java.MethodCompactConstructor)
}
