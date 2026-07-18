package resolve

import (
	"fmt"
	"strings"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
	"github.com/lucas-garcia-rubio/fluxos/internal/index"
)

// isPolymorphicReceiver reports whether typ é elegível para dispatch
// polimórfico M3: uma interface ou uma abstract class do projeto.
func isPolymorphicReceiver(typ *java.TypeDecl) bool {
	if typ == nil {
		return false
	}
	if typ.Kind == java.TypeKindInterface {
		return true
	}
	return typ.Kind == java.TypeKindClass && java.HasModifier(typ.Modifier, "abstract")
}

// staticBoundCall reports whether every method named call.MethodName reachable
// from typ is static. When true, the call is static-bound (não despacha
// polimorficamente) mesmo se typ for interface ou abstract.
func (r *SyntacticResolver) staticBoundCall(typ *java.TypeDecl, call java.CallSite) bool {
	if r.Index == nil || typ == nil {
		return false
	}
	candidates := r.methodCandidatesOnType(typ, call.MethodName)
	if len(candidates) == 0 {
		return false
	}
	for _, candidate := range candidates {
		if candidate.Method == nil || !java.HasModifier(candidate.Method.Modifier, "static") {
			return false
		}
	}
	return true
}

// dispatchPolymorphic aplica a política M3 de dispatch sobre a
// ImplementationTable. Receivers interface/abstract com 0 impls tentam default
// method na própria interface; receivers com 1 impl descendem para o effective
// method da impl; receivers com várias impls viram terminal AmbiguousImplementation.
//
// ctx não é usado hoje mas faz parte da assinatura para futuras extensões
// (ex.: narrowing por initializer em M4).
func (r *SyntacticResolver) dispatchPolymorphic(typ *java.TypeDecl, call java.CallSite, _ MethodContext) Resolution {
	impls := r.Index.ImplementationsOf(typ.FQCN)

	switch len(impls) {
	case 0:
		// Sem impls: tenta default method direto na interface. Declarações
		// abstract em interfaces (sem modifier "default") não são callable sem
		// impl, então filtramos para só default methods.
		all := r.Index.EffectiveMethodCandidates(typ.FQCN, call.MethodName)
		defaults := make([]index.MethodResolution, 0, len(all))
		for _, c := range all {
			if c.Method != nil && java.HasModifier(c.Method.Modifier, "default") {
				defaults = append(defaults, c)
			}
		}
		selection := selectMethodCandidates(defaults, call, typ)
		if selection.Found && len(selection.Resolution.Targets) == 1 && selection.Resolution.Targets[0].Kind == ResolutionConcrete {
			return selection.Resolution
		}
		return Resolution{Targets: []ResolvedTarget{TerminalTarget(
			ResolutionNoImplementation, typ.FQCN, call.MethodName, "", call,
			fmt.Sprintf("no concrete implementations of %s in project", typ.FQCN), nil,
		)}}

	case 1:
		impl := impls[0]
		candidates := r.Index.EffectiveMethodCandidates(impl.FQCN, call.MethodName)
		selection := selectMethodCandidates(candidates, call, impl)
		if !selection.Found {
			return Resolution{Targets: []ResolvedTarget{TerminalTarget(
				ResolutionNoImplementation, typ.FQCN, call.MethodName, "", call,
				fmt.Sprintf("unique implementation %s of %s lacks method %q", impl.FQCN, typ.FQCN, call.MethodName), nil,
			)}}
		}
		target := selection.Resolution.Targets[0]
		if target.Kind != ResolutionConcrete || len(selection.Resolution.Targets) != 1 {
			// AmbiguousOverload ou outro terminal já producido por selectMethodCandidates.
			return selection.Resolution
		}
		return selection.Resolution

	default:
		candidates := make([]string, len(impls))
		for i, impl := range impls {
			candidates[i] = impl.FQCN
		}
		return Resolution{Targets: []ResolvedTarget{TerminalTarget(
			ResolutionAmbiguousImplementation, typ.FQCN, call.MethodName, "", call,
			fmt.Sprintf("multiple concrete implementations of %s", typ.FQCN), candidates,
		)}}
	}
}

// resolveOnPolymorphicOrType é o ponto único de entrada do dispatch polimórfico
// para receivers de tipo identificado. Se typ é interface/abstract e o call não
// é static-bound, despacha; caso contrário, segue o caminho existente de
// resolveOnType.
func (r *SyntacticResolver) resolveOnPolymorphicOrType(typ *java.TypeDecl, call java.CallSite, ctx MethodContext) Resolution {
	if isPolymorphicReceiver(typ) && !r.staticBoundCall(typ, call) {
		return r.dispatchPolymorphic(typ, call, ctx)
	}
	return r.resolveOnType(typ, call)
}

// unresolvedKindFromNote classifica a saída de resolveType. Hoje resolveType
// retorna ("", "ambiguous type; candidates: X, Y") para ambiguous; todos os
// outros failures viram ResolutionUnresolved. Interno ao resolver — não é
// parseado pelo renderer.
func unresolvedKindFromNote(note string) ResolutionKind {
	if strings.Contains(note, "ambiguous type") {
		return ResolutionAmbiguousType
	}
	return ResolutionUnresolved
}
