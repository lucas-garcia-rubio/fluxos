package resolve

import (
	"fmt"
	"sort"
	"strings"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
	"github.com/lucas-garcia-rubio/fluxos/internal/index"
)

func (r *SyntacticResolver) resolveMethodReference(call java.CallSite, ctx MethodContext) Resolution {
	if r.Index == nil {
		return Resolution{Note: "project index is unavailable"}
	}
	if call.ReferenceQualifier == java.ReferenceQualifierSuper || call.Receiver == "super" {
		return r.resolveSuperMethodReference(call, ctx)
	}
	if call.Receiver == "this" {
		return r.resolveBoundMethodReference(ctx.EnclosingType, call, ctx)
	}
	if call.ReferenceQualifier == java.ReferenceQualifierExpression {
		return Resolution{Note: fmt.Sprintf("complex method reference receiver %q is unsupported", call.Receiver)}
	}

	if typ, found, note := r.lexicalReferenceReceiver(call.Receiver, call, ctx); found {
		if typ == nil {
			return Resolution{Note: note}
		}
		return r.resolveBoundMethodReference(typ, call, ctx)
	}
	typ, note := r.resolveType(java.NewTypeRef(call.Receiver, false), ctx)
	if typ == nil {
		return Resolution{Note: fmt.Sprintf("method reference receiver %q unresolved: %s", call.Receiver, note)}
	}
	return r.resolveTypeMethodReference(typ, call, ctx)
}

func (r *SyntacticResolver) lexicalReferenceReceiver(receiver string, call java.CallSite, ctx MethodContext) (*java.TypeDecl, bool, string) {
	if local, ok := findLocalVarAt(ctx.LocalVars, receiver, call.StartByte); ok {
		typ, note := r.resolveType(local.Type, ctx)
		return typ, true, fmt.Sprintf("local var type %q unresolved: %s", local.Type.Raw, note)
	}
	if param := findParam(ctx.Params, receiver); param != nil {
		typ, note := r.resolveType(param.Type, ctx)
		return typ, true, fmt.Sprintf("param type %q unresolved: %s", param.Type.Raw, note)
	}
	if ctx.EnclosingType != nil {
		if field, ok := r.effectiveField(ctx.EnclosingType, receiver); ok {
			fieldCtx := ctx
			fieldCtx.EnclosingType = field.DeclaringType
			fieldCtx.File = field.DeclaringType.File
			typ, note := r.resolveType(field.Field.Type, fieldCtx)
			return typ, true, fmt.Sprintf("field type %q unresolved: %s", field.Field.Type.Raw, note)
		}
	}
	return nil, false, ""
}

func (r *SyntacticResolver) resolveBoundMethodReference(typ *java.TypeDecl, call java.CallSite, ctx MethodContext) Resolution {
	if typ == nil {
		return Resolution{Note: "method reference has no receiver type"}
	}
	candidates := make([]index.MethodResolution, 0)
	for _, candidate := range r.Index.EffectiveMethodCandidates(typ.FQCN, call.MethodName) {
		if java.HasModifier(candidate.Method.Modifier, "static") || !r.methodReferenceAccessible(candidate, typ, ctx) {
			continue
		}
		candidates = append(candidates, candidate)
	}
	return selectReferenceCandidates(candidates, call, typ.FQCN, "method reference")
}

func (r *SyntacticResolver) resolveTypeMethodReference(typ *java.TypeDecl, call java.CallSite, ctx MethodContext) Resolution {
	if !r.typeAccessible(typ, ctx) {
		return Resolution{Note: fmt.Sprintf("method reference type %s is not accessible", typ.FQCN)}
	}
	candidates := make([]index.MethodResolution, 0)
	for _, candidate := range r.Index.StaticMethodCandidates(typ.FQCN, call.MethodName) {
		if r.methodReferenceAccessible(candidate, typ, ctx) {
			candidates = append(candidates, candidate)
		}
	}
	for _, candidate := range r.Index.EffectiveMethodCandidates(typ.FQCN, call.MethodName) {
		if !java.HasModifier(candidate.Method.Modifier, "static") && r.methodReferenceAccessible(candidate, typ, ctx) {
			candidates = append(candidates, candidate)
		}
	}
	return selectReferenceCandidates(candidates, call, typ.FQCN, "method reference")
}

func (r *SyntacticResolver) resolveSuperMethodReference(call java.CallSite, ctx MethodContext) Resolution {
	if ctx.EnclosingType == nil {
		return Resolution{Note: "super method reference has no enclosing type"}
	}
	superType, ok := r.Index.DirectSuperclass(ctx.EnclosingType.FQCN)
	if !ok {
		return Resolution{Note: fmt.Sprintf("type %s has no project superclass", ctx.EnclosingType.FQCN)}
	}
	candidates := make([]index.MethodResolution, 0)
	for _, candidate := range r.Index.EffectiveMethodCandidates(superType.FQCN, call.MethodName) {
		if java.HasModifier(candidate.Method.Modifier, "static") || !r.methodAccessible(candidate, ctx) {
			continue
		}
		candidates = append(candidates, candidate)
	}
	return selectReferenceCandidates(candidates, call, superType.FQCN, "super method reference")
}

func (r *SyntacticResolver) resolveConstructorReference(call java.CallSite, ctx MethodContext) Resolution {
	if r.Index == nil {
		return Resolution{Note: "project index is unavailable"}
	}
	if call.TargetType == nil {
		return Resolution{Note: "constructor reference has no target type"}
	}
	if call.TargetType.ArrayDepth > 0 {
		return Resolution{Note: "array constructor references are unsupported"}
	}
	typ, note := r.resolveType(*call.TargetType, ctx)
	if typ == nil {
		return Resolution{Note: fmt.Sprintf("constructor reference type %q unresolved: %s", call.TargetType.Raw, note)}
	}
	if !r.typeAccessible(typ, ctx) {
		return Resolution{Note: fmt.Sprintf("constructor reference type %s is not accessible", typ.FQCN)}
	}
	switch typ.Kind {
	case java.TypeKindClass:
		if java.HasModifier(typ.Modifier, "abstract") {
			return Resolution{Note: fmt.Sprintf("cannot reference constructor of abstract type %s", typ.FQCN)}
		}
	case java.TypeKindRecord:
	default:
		return Resolution{Note: fmt.Sprintf("cannot reference constructor of %s %s", typ.Kind, typ.FQCN)}
	}
	if !r.isStaticMemberType(typ) {
		return Resolution{Note: fmt.Sprintf("non-static inner constructor reference %s is unsupported", typ.FQCN)}
	}
	candidates := r.accessibleConstructors(typ, ctx, false)
	return selectReferenceCandidates(candidates, call, typ.FQCN, "constructor reference")
}

func (r *SyntacticResolver) isStaticMemberType(typ *java.TypeDecl) bool {
	if typ == nil || typ.EnclosingFQCN == "" {
		return true
	}
	if typ.Kind == java.TypeKindInterface || typ.Kind == java.TypeKindEnum || typ.Kind == java.TypeKindRecord || java.HasModifier(typ.Modifier, "static") {
		return true
	}
	enclosing, ok := r.Index.TypeByFQCN(typ.EnclosingFQCN)
	return ok && enclosing.Kind == java.TypeKindInterface
}

func (r *SyntacticResolver) methodAccessible(candidate index.MethodResolution, ctx MethodContext) bool {
	if candidate.DeclaringType == nil || candidate.Method == nil {
		return false
	}
	modifiers := candidate.Method.Modifier
	if java.HasModifier(modifiers, "private") {
		return r.sameNest(candidate.DeclaringType.FQCN, ctx)
	}
	if java.HasModifier(modifiers, "public") || candidate.DeclaringType.Kind == java.TypeKindInterface {
		return true
	}
	if r.samePackage(candidate.DeclaringType.FQCN, ctx) {
		return true
	}
	return java.HasModifier(modifiers, "protected") && r.callerIsSubclassOf(candidate.DeclaringType.FQCN, ctx)
}

func (r *SyntacticResolver) methodReferenceAccessible(candidate index.MethodResolution, qualifierType *java.TypeDecl, ctx MethodContext) bool {
	if !r.methodAccessible(candidate, ctx) {
		return false
	}
	if candidate.Method == nil || !java.HasModifier(candidate.Method.Modifier, "protected") || r.samePackage(candidate.DeclaringType.FQCN, ctx) {
		return true
	}
	if qualifierType == nil || ctx.EnclosingType == nil {
		return false
	}
	if qualifierType.FQCN == ctx.EnclosingType.FQCN {
		return true
	}
	for _, superclass := range r.Index.SuperclassChain(qualifierType.FQCN) {
		if superclass.FQCN == ctx.EnclosingType.FQCN {
			return true
		}
	}
	return false
}

func selectReferenceCandidates(candidates []index.MethodResolution, call java.CallSite, owner, kind string) Resolution {
	type candidateKey struct {
		owner string
		key   java.MethodKey
	}
	seen := make(map[candidateKey]struct{})
	applicable := make([]index.MethodResolution, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.DeclaringType == nil || candidate.Method == nil {
			continue
		}
		key := candidateKey{owner: candidate.DeclaringType.FQCN, key: candidate.Method.Key()}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		applicable = append(applicable, candidate)
	}
	sort.Slice(applicable, func(i, j int) bool {
		left, right := applicable[i], applicable[j]
		if left.DeclaringType.FQCN != right.DeclaringType.FQCN {
			return left.DeclaringType.FQCN < right.DeclaringType.FQCN
		}
		if left.Method.Name != right.Method.Name {
			return left.Method.Name < right.Method.Name
		}
		return left.Method.Signature < right.Method.Signature
	})
	if len(applicable) == 0 {
		return Resolution{Targets: []ResolvedTarget{TerminalTarget(
			ResolutionUnresolved, owner, call.MethodName, "", call,
			fmt.Sprintf("%s %q not found on %s", kind, call.MethodName, owner), nil,
		)}}
	}
	if len(applicable) == 1 {
		candidate := applicable[0]
		handle := MethodHandle{
			TypeFQCN:  candidate.DeclaringType.FQCN,
			Method:    candidate.Method.Name,
			Signature: candidate.Method.Signature,
		}
		return Resolution{Targets: []ResolvedTarget{ConcreteTarget(handle)}}
	}
	descriptions := make([]string, len(applicable))
	for i, candidate := range applicable {
		descriptions[i] = candidate.DeclaringType.FQCN + "." + candidate.Method.Name + candidate.Method.Signature
	}
	return Resolution{Targets: []ResolvedTarget{TerminalTarget(
		ResolutionAmbiguousOverload, owner, call.MethodName, "", call,
		fmt.Sprintf("ambiguous %s %q on %s: %s", kind, call.MethodName, owner, strings.Join(descriptions, ", ")), nil,
	)}}
}
