package resolve

import (
	"fmt"
	"sort"
	"strings"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
	"github.com/lucas-garcia-rubio/fluxos/internal/index"
)

// SyntacticResolver é a implementação concreta do Resolver baseada em
// tree-sitter + heurísticas sobre a AST. Não usa type info beyond what's
// diretamente acessível no MethodContext.
//
// M3 Passo 5 canonicaliza receivers por package e imports, sem ainda percorrer
// heranca cross-file.
type SyntacticResolver struct {
	Index *index.Table
}

// NewSyntacticResolver constrói um resolver pronto pra usar.
func NewSyntacticResolver(table *index.Table) *SyntacticResolver {
	return &SyntacticResolver{Index: table}
}

// Resolve decide qual método call aponta, baseado em call.Receiver e no
// MethodContext. Ver Passo 8 em PLANO_M2.md pra algoritmo completo.
func (r *SyntacticResolver) Resolve(call java.CallSite, ctx MethodContext) Resolution {
	switch call.Receiver {
	case "", "this":
		return r.resolveOnType(ctx.EnclosingType, call)
	case "super":
		return r.resolveSuper(call, ctx)
	default:
		return r.resolveIdentifier(call.Receiver, call, ctx)
	}
}

func (r *SyntacticResolver) resolveSuper(call java.CallSite, ctx MethodContext) Resolution {
	if ctx.EnclosingType == nil {
		return Resolution{Note: "no enclosing type"}
	}
	if ctx.EnclosingType.SuperClass.Raw == "" {
		return Resolution{Note: fmt.Sprintf("type %s has no superclass", ctx.EnclosingType.FQCN)}
	}

	if r.Index == nil {
		return Resolution{Note: "project index is unavailable"}
	}
	superType, ok := r.Index.DirectSuperclass(ctx.EnclosingType.FQCN)
	if !ok {
		return Resolution{Note: fmt.Sprintf("superclass %q not found in project", ctx.EnclosingType.SuperClass.SignatureToken())}
	}
	return r.resolveOnType(superType, call)
}

func (r *SyntacticResolver) resolveIdentifier(receiver string, call java.CallSite, ctx MethodContext) Resolution {
	if local, ok := findLocalVarAt(ctx.LocalVars, receiver, call.StartByte); ok {
		t, note := r.resolveType(local.Type, ctx)
		if t == nil {
			return Resolution{Note: fmt.Sprintf("local var type %q unresolved: %s", local.Type.Raw, note)}
		}
		return r.resolveOnType(t, call)
	}

	if param := findParam(ctx.Params, receiver); param != nil {
		t, note := r.resolveType(param.Type, ctx)
		if t == nil {
			return Resolution{Note: fmt.Sprintf("param type %q unresolved: %s", param.Type.Raw, note)}
		}
		return r.resolveOnType(t, call)
	}

	if ctx.EnclosingType != nil {
		if field, ok := r.effectiveField(ctx.EnclosingType, receiver); ok {
			fieldCtx := ctx
			fieldCtx.EnclosingType = field.DeclaringType
			fieldCtx.File = field.DeclaringType.File
			t, note := r.resolveType(field.Field.Type, fieldCtx)
			if t == nil {
				return Resolution{Note: fmt.Sprintf("field type %q unresolved: %s", field.Field.Type.Raw, note)}
			}
			return r.resolveOnType(t, call)
		}
	}

	t, note := r.resolveType(java.NewTypeRef(receiver, false), ctx)
	if t == nil {
		return Resolution{Note: fmt.Sprintf("receiver %q is not a local var, field, or resolvable type: %s", receiver, note)}
	}
	return r.resolveOnType(t, call)
}

// findLocalVarAt devolve a local var visível no ponto da chamada (byte offset).
// Filtro: nome bate, call dentro de [ScopeStart, ScopeEnd), DeclStart <= callPos.
// Desempate: bloco mais interno (maior ScopeStart vence — shadowing).
// Retorna (LocalVarDecl, true) ou (zero, false).
func findLocalVarAt(vars []java.LocalVarDecl, name string, callPos uint) (java.LocalVarDecl, bool) {
	var winner java.LocalVarDecl
	found := false
	for _, v := range vars {
		if v.Name != name {
			continue
		}
		if callPos < v.ScopeStart || callPos >= v.ScopeEnd {
			continue
		}
		if v.DeclStart > callPos {
			continue
		}
		if !found || v.ScopeStart > winner.ScopeStart {
			winner = v
			found = true
		}
	}
	return winner, found
}

func findParam(params []java.Param, name string) *java.Param {
	for i := range params {
		if params[i].Name == name {
			return &params[i]
		}
	}
	return nil
}

func (r *SyntacticResolver) resolveType(ref java.TypeRef, ctx MethodContext) (*java.TypeDecl, string) {
	if r.Index == nil {
		return nil, "project index is unavailable"
	}
	unit := r.Index.UnitsByFile[ctx.File]
	if unit == nil && ctx.EnclosingType != nil {
		unit = r.Index.UnitForType(ctx.EnclosingType.FQCN)
	}
	resolution := r.Index.ResolveTypeRef(ref, unit)
	if len(resolution.Candidates) > 1 {
		return nil, fmt.Sprintf("ambiguous type; candidates: %s", strings.Join(resolution.Candidates, ", "))
	}
	if resolution.Ref.FQCN == "" {
		return nil, "no candidate"
	}
	typ, ok := r.Index.TypeByFQCN(resolution.Ref.FQCN)
	if !ok {
		return nil, fmt.Sprintf("external type %s", resolution.Ref.FQCN)
	}
	return typ, ""
}

// resolveOnType seleciona métodos por nome e aridade. Tipos de argumentos serão
// usados em uma etapa posterior, quando type refs estiverem canonicalizados.
func (r *SyntacticResolver) resolveOnType(t *java.TypeDecl, call java.CallSite) Resolution {
	if t == nil {
		return Resolution{Note: "no enclosing type"}
	}
	candidates := r.methodCandidatesOnType(t, call.MethodName)
	if selection := selectMethodCandidates(candidates, call, t); selection.Found {
		return selection.Resolution
	}
	return Resolution{
		Note: fmt.Sprintf("method %q with arity %d not found on %s", call.MethodName, call.ArgCount, t.FQCN),
	}
}

type candidateSelection struct {
	Resolution Resolution
	Found      bool
}

func selectMethodCandidates(candidates []index.MethodResolution, call java.CallSite, receiver *java.TypeDecl) candidateSelection {
	applicable := make([]index.MethodResolution, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.DeclaringType != nil && candidate.Method != nil && arityCompatible(*candidate.Method, call.ArgCount) {
			applicable = append(applicable, candidate)
		}
	}
	if len(applicable) == 0 {
		return candidateSelection{}
	}
	if len(applicable) == 1 {
		candidate := applicable[0]
		return candidateSelection{Found: true, Resolution: Resolution{Targets: []MethodHandle{{
			TypeFQCN:  candidate.DeclaringType.FQCN,
			Method:    candidate.Method.Name,
			Signature: candidate.Method.Signature,
		}}}}
	}

	descriptions := make([]string, len(applicable))
	sameOwner := true
	owner := applicable[0].DeclaringType.FQCN
	for i, candidate := range applicable {
		if candidate.DeclaringType.FQCN != owner {
			sameOwner = false
		}
		descriptions[i] = candidate.DeclaringType.FQCN + "." + candidate.Method.Name + candidate.Method.Signature
	}
	if sameOwner {
		for i, candidate := range applicable {
			descriptions[i] = candidate.Method.Signature
		}
	}
	sort.Strings(descriptions)
	subject := owner
	if receiver != nil {
		subject = receiver.FQCN
	}
	return candidateSelection{
		Found:      true,
		Resolution: Resolution{Note: fmt.Sprintf("ambiguous overload %q on %s: %s", call.MethodName, subject, strings.Join(descriptions, ", "))},
	}
}

func (r *SyntacticResolver) methodCandidatesOnType(t *java.TypeDecl, name string) []index.MethodResolution {
	result := make([]index.MethodResolution, 0)
	if t == nil {
		return result
	}
	if r.Index != nil {
		if _, ok := r.Index.TypeByFQCN(t.FQCN); ok {
			return r.Index.EffectiveMethodCandidates(t.FQCN, name)
		}
	}
	for i := range t.Methods {
		method := &t.Methods[i]
		if method.Name == name {
			result = append(result, index.MethodResolution{DeclaringType: t, Method: method})
		}
	}
	return result
}

func (r *SyntacticResolver) effectiveField(t *java.TypeDecl, name string) (index.FieldResolution, bool) {
	if r.Index != nil {
		if _, ok := r.Index.TypeByFQCN(t.FQCN); ok {
			return r.Index.EffectiveField(t.FQCN, name)
		}
	}
	for i := range t.Fields {
		if t.Fields[i].Name == name {
			return index.FieldResolution{DeclaringType: t, Field: &t.Fields[i]}, true
		}
	}
	return index.FieldResolution{}, false
}

func arityCompatible(method java.MethodDecl, argCount int) bool {
	paramCount := len(method.Params)
	if paramCount == 0 || !method.Params[paramCount-1].Variadic {
		return argCount == paramCount
	}
	return argCount >= paramCount-1
}
