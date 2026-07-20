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
// ctx permite que a selecao da unica implementation use tipos obvios dos
// argumentos sem fazer narrowing pelo initializer do receiver (reservado a M4).
func (r *SyntacticResolver) dispatchPolymorphic(typ *java.TypeDecl, call java.CallSite, ctx MethodContext) Resolution {
	lexicalCandidates := r.methodCandidatesOnType(typ, call.MethodName)
	lexical := r.selectMethodCandidates(lexicalCandidates, call, typ, ctx)
	return r.dispatchPolymorphicSelection(typ, call, ctx, lexicalCandidates, lexical)
}

func (r *SyntacticResolver) dispatchPolymorphicSelection(
	typ *java.TypeDecl,
	call java.CallSite,
	ctx MethodContext,
	lexicalCandidates []index.MethodResolution,
	lexical candidateSelection,
) Resolution {
	impls := r.Index.ImplementationsOf(typ.FQCN)
	if lexical.Candidate != nil && !isVirtualMethod(lexical.Candidate.Method) {
		return r.bindSelection(lexical, typ.FQCN, call)
	}
	if len(lexicalCandidates) > 0 && !lexical.Found {
		return unresolvedSelection(typ.FQCN, call,
			fmt.Sprintf("no compatible overload %q on compile-time receiver %s", call.MethodName, typ.FQCN))
	}
	if lexical.Found && lexical.Candidate == nil {
		return lexical.Resolution
	}
	if len(impls) != 1 && lexical.Found && (len(lexical.Resolution.Targets) != 1 || lexical.Resolution.Targets[0].Kind != ResolutionConcrete) {
		return lexical.Resolution
	}

	switch len(impls) {
	case 0:
		if lexical.Candidate != nil && java.HasModifier(lexical.Candidate.Method.Modifier, "default") {
			return r.bindSelection(lexical, typ.FQCN, call)
		}
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
		selection := r.selectMethodCandidates(defaults, call, typ, ctx)
		if selection.Found && len(selection.Resolution.Targets) == 1 && selection.Resolution.Targets[0].Kind == ResolutionConcrete {
			return r.bindSelection(selection, typ.FQCN, call)
		}
		if selection.Found {
			return selection.Resolution
		}
		return Resolution{Targets: []ResolvedTarget{TerminalTarget(
			ResolutionNoImplementation, typ.FQCN, call.MethodName, "", call,
			fmt.Sprintf("no concrete implementations of %s in project", typ.FQCN), nil,
		)}}

	case 1:
		impl := impls[0]
		if lexical.Found && lexical.Candidate != nil {
			return r.bindSelection(lexical, impl.FQCN, call)
		}
		// Preserve M3 behavior only for incomplete models that omit the method
		// entirely. A declared but incompatible overload must not expose an
		// implementation-only overload.
		candidates := r.Index.EffectiveMethodCandidates(impl.FQCN, call.MethodName)
		selection := r.selectMethodCandidates(candidates, call, impl, ctx)
		if !selection.Found {
			if len(candidates) > 0 {
				return unresolvedSelection(impl.FQCN, call,
					fmt.Sprintf("no compatible overload %q on unique implementation %s of %s", call.MethodName, impl.FQCN, typ.FQCN))
			}
			return Resolution{Targets: []ResolvedTarget{TerminalTarget(
				ResolutionNoImplementation, typ.FQCN, call.MethodName, "", call,
				fmt.Sprintf("unique implementation %s of %s lacks method %q", impl.FQCN, typ.FQCN, call.MethodName), nil,
			)}}
		}
		if len(selection.Resolution.Targets) != 1 || selection.Resolution.Targets[0].Kind != ResolutionConcrete {
			// AmbiguousOverload ou outro terminal já producido por selectMethodCandidates.
			return selection.Resolution
		}
		return selection.withRuntime(impl.FQCN)

	default:
		candidates := make([]ImplementationCandidate, 0, len(impls))
		signature := ""
		if lexical.Candidate != nil && lexical.Candidate.Method != nil {
			signature = lexical.Candidate.Method.Signature
		}
		for _, impl := range impls {
			candidate := ImplementationCandidate{ImplementationFQCN: impl.FQCN, Kind: ResolutionNoImplementation}
			if lexical.Found {
				candidate = r.implementationCandidate(impl, lexical, call)
			} else {
				candidate.Kind = ResolutionUnresolved
				candidate.Note = fmt.Sprintf("compile-time receiver %s does not declare method %q", typ.FQCN, call.MethodName)
			}
			candidates = append(candidates, candidate)
		}
		caller := ctx.Execution
		if caller.RuntimeTypeFQCN == "" {
			caller.RuntimeTypeFQCN = contextRuntime(ctx)
		}
		policy := r.policy()
		decision := policy.Apply(caller, typ, call, candidates)
		resolution := Resolution{DispatchSite: NewDispatchSite(caller, typ.FQCN, call.MethodName, signature, call, candidates)}
		if len(decision.Targets) > 0 {
			resolution.Targets = append(resolution.Targets, decision.Targets...)
		}
		if decision.Terminal != nil {
			resolution.Targets = append(resolution.Targets, *decision.Terminal)
		}
		if len(resolution.Targets) == 0 {
			// Nenhuma policy产出: fallback para terminal ambiguous (preserva
			// comportamento M3 mesmo se uma policy custom não decidir).
			t := TerminalTarget(
				ResolutionAmbiguousImplementation, typ.FQCN, call.MethodName, "", call,
				fmt.Sprintf("multiple concrete implementations of %s", typ.FQCN), nil,
			)
			resolution.Targets = append(resolution.Targets, t)
		}
		if decision.Omitted > 0 {
			resolution.Truncations = append(resolution.Truncations, PolicyTruncation{
				Caller:  caller,
				Call:    call,
				Omitted: decision.Omitted,
				Note:    fmt.Sprintf("limited by --max-impls on %s", typ.FQCN),
			})
		}
		return resolution
	}
}

func (r *SyntacticResolver) implementationCandidate(impl *java.TypeDecl, selection candidateSelection, call java.CallSite) ImplementationCandidate {
	result := r.bindSelection(selection, impl.FQCN, call)
	candidate := ImplementationCandidate{ImplementationFQCN: impl.FQCN, Kind: ResolutionUnresolved}
	if len(result.Targets) != 1 {
		candidate.Note = result.Note
		return candidate
	}
	target := result.Targets[0]
	candidate.Kind = target.Kind
	candidate.Note = target.Note
	if target.Kind == ResolutionConcrete {
		candidate.Target = target.Key.Method
	}
	return candidate
}

func (r *SyntacticResolver) bindSelection(selection candidateSelection, runtime string, call java.CallSite) Resolution {
	if selection.Candidate == nil || selection.Candidate.Method == nil || selection.Candidate.DeclaringType == nil {
		return selection.Resolution
	}
	candidate := *selection.Candidate
	if runtime == "" {
		runtime = candidate.DeclaringType.FQCN
	}
	if !isVirtualMethod(candidate.Method) {
		if java.HasModifier(candidate.Method.Modifier, "static") {
			runtime = candidate.DeclaringType.FQCN
		}
		return selection.withRuntime(runtime)
	}
	if r.Index == nil {
		return selection.withRuntime(runtime)
	}
	if _, indexed := r.Index.TypeByFQCN(runtime); !indexed {
		return selection.withRuntime(runtime)
	}
	exact := r.Index.EffectiveMethod(runtime, candidate.Method.Key())
	if len(exact) != 1 || exact[0].DeclaringType == nil || exact[0].Method == nil {
		return unresolvedSelection(runtime, call,
			fmt.Sprintf("exact virtual method %s%s has %d effective targets on runtime type %s", candidate.Method.Name, candidate.Method.Signature, len(exact), runtime))
	}
	handle := MethodHandle{TypeFQCN: exact[0].DeclaringType.FQCN, Method: exact[0].Method.Name, Signature: exact[0].Method.Signature}
	return Resolution{Targets: []ResolvedTarget{ConcreteTarget(ExecutionKey{Method: handle, RuntimeTypeFQCN: runtime})}}
}

func isVirtualMethod(method *java.MethodDecl) bool {
	if method == nil || method.Kind == java.MethodConstructor || method.Kind == java.MethodCompactConstructor {
		return false
	}
	return !java.HasModifier(method.Modifier, "static") &&
		!java.HasModifier(method.Modifier, "private") &&
		!java.HasModifier(method.Modifier, "final")
}

// resolveOnPolymorphicOrType é o ponto único de entrada do dispatch polimórfico
// para receivers de tipo identificado. Se typ é interface/abstract e o call não
// é static-bound, despacha; caso contrário, segue o caminho existente de
// resolveOnType.
func (r *SyntacticResolver) resolveOnPolymorphicOrType(typ *java.TypeDecl, call java.CallSite, ctx MethodContext) Resolution {
	if isPolymorphicReceiver(typ) && !r.staticBoundCall(typ, call) {
		return r.dispatchPolymorphic(typ, call, ctx)
	}
	return r.resolveOnType(typ, call, ctx)
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
