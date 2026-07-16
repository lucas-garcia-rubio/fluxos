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
	switch call.Kind {
	case java.CallObjectCreation:
		return r.resolveObjectCreation(call, ctx)
	case java.CallThisConstructor:
		return r.resolveThisConstructor(call, ctx)
	case java.CallSuperConstructor:
		return r.resolveSuperConstructor(call, ctx)
	case java.CallMethodReference:
		return r.resolveMethodReference(call, ctx)
	case java.CallConstructorReference:
		return r.resolveConstructorReference(call, ctx)
	}
	return r.resolveInvocation(call, ctx)
}

func (r *SyntacticResolver) resolveInvocation(call java.CallSite, ctx MethodContext) Resolution {
	switch call.Receiver {
	case "":
		return r.resolveUnqualified(call, ctx)
	case "this":
		return r.resolveOnType(ctx.EnclosingType, call)
	case "super":
		return r.resolveSuper(call, ctx)
	default:
		return r.resolveIdentifier(call.Receiver, call, ctx)
	}
}

func (r *SyntacticResolver) resolveObjectCreation(call java.CallSite, ctx MethodContext) Resolution {
	if call.Anonymous {
		return Resolution{Note: "anonymous class construction is unsupported"}
	}
	if call.Receiver != "" {
		return Resolution{Note: fmt.Sprintf("qualified object creation receiver %q is unsupported", call.Receiver)}
	}
	if call.TargetType == nil {
		return Resolution{Note: "object creation has no target type"}
	}
	typ, note := r.resolveType(*call.TargetType, ctx)
	if typ == nil {
		return Resolution{Note: fmt.Sprintf("constructor type %q unresolved: %s", call.TargetType.Raw, note)}
	}
	if !r.typeAccessible(typ, ctx) {
		return Resolution{Note: fmt.Sprintf("constructor type %s is not accessible", typ.FQCN)}
	}
	switch typ.Kind {
	case java.TypeKindClass:
		if java.HasModifier(typ.Modifier, "abstract") {
			return Resolution{Note: fmt.Sprintf("cannot instantiate abstract type %s", typ.FQCN)}
		}
	case java.TypeKindRecord:
		// Records are directly instantiable.
	default:
		return Resolution{Note: fmt.Sprintf("cannot instantiate %s %s", typ.Kind, typ.FQCN)}
	}

	candidates := r.accessibleConstructors(typ, ctx, false)
	if selection := selectConstructorCandidates(candidates, call, typ); selection.Found {
		return selection.Resolution
	}
	return Resolution{Note: fmt.Sprintf("constructor with arity %d not found on %s", call.ArgCount, typ.FQCN)}
}

func (r *SyntacticResolver) resolveThisConstructor(call java.CallSite, ctx MethodContext) Resolution {
	if ctx.EnclosingType == nil {
		return Resolution{Note: "this constructor call has no enclosing type"}
	}
	if r.Index == nil {
		return Resolution{Note: "project index is unavailable"}
	}
	candidates := r.Index.ConstructorCandidates(ctx.EnclosingType.FQCN)
	if selection := selectConstructorCandidates(candidates, call, ctx.EnclosingType); selection.Found {
		return selection.Resolution
	}
	return Resolution{Note: fmt.Sprintf("constructor with arity %d not found on %s", call.ArgCount, ctx.EnclosingType.FQCN)}
}

func (r *SyntacticResolver) resolveSuperConstructor(call java.CallSite, ctx MethodContext) Resolution {
	if call.Receiver != "" {
		return Resolution{Note: fmt.Sprintf("qualified super constructor receiver %q is unsupported", call.Receiver)}
	}
	if ctx.EnclosingType == nil {
		return Resolution{Note: "super constructor call has no enclosing type"}
	}
	if r.Index == nil {
		return Resolution{Note: "project index is unavailable"}
	}
	superType, ok := r.Index.DirectSuperclass(ctx.EnclosingType.FQCN)
	if !ok {
		return Resolution{Note: fmt.Sprintf("type %s has no project superclass", ctx.EnclosingType.FQCN)}
	}
	candidates := r.accessibleConstructors(superType, ctx, true)
	if selection := selectConstructorCandidates(candidates, call, superType); selection.Found {
		return selection.Resolution
	}
	return Resolution{Note: fmt.Sprintf("constructor with arity %d not found on direct superclass %s", call.ArgCount, superType.FQCN)}
}

func (r *SyntacticResolver) resolveUnqualified(call java.CallSite, ctx MethodContext) Resolution {
	if selection := selectMethodCandidates(r.methodCandidatesOnType(ctx.EnclosingType, call.MethodName), call, ctx.EnclosingType); selection.Found {
		return selection.Resolution
	}

	unit := r.unitForContext(ctx)
	if unit != nil {
		if selection := selectStaticImportCandidates(r.staticImportCandidates(unit, call.MethodName, false, ctx), call, "explicit"); selection.Found {
			return selection.Resolution
		}
		if selection := selectStaticImportCandidates(r.staticImportCandidates(unit, call.MethodName, true, ctx), call, "wildcard"); selection.Found {
			return selection.Resolution
		}
	}

	owner := "<unknown>"
	if ctx.EnclosingType != nil {
		owner = ctx.EnclosingType.FQCN
	}
	return Resolution{Note: fmt.Sprintf("method %q with arity %d not found on %s or static imports", call.MethodName, call.ArgCount, owner)}
}

func selectStaticImportCandidates(candidates []index.MethodResolution, call java.CallSite, kind string) candidateSelection {
	selection := selectMethodCandidates(candidates, call, nil)
	if selection.Found && len(selection.Resolution.Targets) == 0 {
		selection.Resolution.Note = strings.Replace(selection.Resolution.Note, "ambiguous overload", "ambiguous "+kind+" static import", 1)
	}
	return selection
}

func selectConstructorCandidates(candidates []index.MethodResolution, call java.CallSite, owner *java.TypeDecl) candidateSelection {
	selection := selectMethodCandidates(candidates, call, owner)
	if selection.Found && len(selection.Resolution.Targets) == 0 {
		selection.Resolution.Note = strings.Replace(selection.Resolution.Note, "ambiguous overload", "ambiguous constructor", 1)
	}
	return selection
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
	return r.resolveStaticOnType(t, call, ctx)
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
	enclosingFQCN := ""
	if ctx.EnclosingType != nil {
		enclosingFQCN = ctx.EnclosingType.FQCN
	}
	resolution := r.Index.ResolveTypeRefInType(ref, unit, enclosingFQCN)
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
	if t.Kind == java.TypeKindInterface {
		instanceCandidates := candidates[:0]
		for _, candidate := range candidates {
			if !java.HasModifier(candidate.Method.Modifier, "static") {
				instanceCandidates = append(instanceCandidates, candidate)
			}
		}
		candidates = instanceCandidates
	}
	if selection := selectMethodCandidates(candidates, call, t); selection.Found {
		return selection.Resolution
	}
	return Resolution{
		Note: fmt.Sprintf("method %q with arity %d not found on %s", call.MethodName, call.ArgCount, t.FQCN),
	}
}

func (r *SyntacticResolver) resolveStaticOnType(t *java.TypeDecl, call java.CallSite, ctx MethodContext) Resolution {
	if t == nil {
		return Resolution{Note: "no receiver type"}
	}
	candidates := make([]index.MethodResolution, 0)
	if r.Index != nil {
		for _, candidate := range r.Index.StaticMethodCandidates(t.FQCN, call.MethodName) {
			if r.staticAccessible(candidate, ctx, false) {
				candidates = append(candidates, candidate)
			}
		}
	}
	if selection := selectMethodCandidates(candidates, call, t); selection.Found {
		return selection.Resolution
	}
	return Resolution{Note: fmt.Sprintf("static method %q with arity %d not found on %s", call.MethodName, call.ArgCount, t.FQCN)}
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

func (r *SyntacticResolver) staticImportCandidates(unit *java.CompilationUnit, name string, wildcard bool, ctx MethodContext) []index.MethodResolution {
	result := make([]index.MethodResolution, 0)
	if r.Index == nil || unit == nil {
		return result
	}
	type candidateKey struct {
		owner string
		key   java.MethodKey
	}
	seen := make(map[candidateKey]struct{})
	for _, importDecl := range unit.Imports {
		if !importDecl.Static || importDecl.Wildcard != wildcard {
			continue
		}
		owner, member := importDecl.Target, name
		if !wildcard {
			dot := strings.LastIndexByte(importDecl.Target, '.')
			if dot < 1 || dot == len(importDecl.Target)-1 {
				continue
			}
			owner, member = importDecl.Target[:dot], importDecl.Target[dot+1:]
			if member != name {
				continue
			}
		}
		importedOwner, ok := r.Index.TypeByFQCN(owner)
		if !ok || !r.typeAccessible(importedOwner, ctx) {
			continue
		}
		for _, candidate := range r.Index.StaticMethodCandidates(owner, member) {
			if !r.staticAccessible(candidate, ctx, true) {
				continue
			}
			key := candidateKey{owner: candidate.DeclaringType.FQCN, key: candidate.Method.Key()}
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			result = append(result, candidate)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i], result[j]
		if left.DeclaringType.FQCN != right.DeclaringType.FQCN {
			return left.DeclaringType.FQCN < right.DeclaringType.FQCN
		}
		if left.Method.Name != right.Method.Name {
			return left.Method.Name < right.Method.Name
		}
		return left.Method.Signature < right.Method.Signature
	})
	return result
}

func (r *SyntacticResolver) typeAccessible(typ *java.TypeDecl, ctx MethodContext) bool {
	if typ == nil {
		return false
	}
	if typ.EnclosingFQCN == "" {
		return java.HasModifier(typ.Modifier, "public") || r.samePackage(typ.FQCN, ctx)
	}
	enclosing, ok := r.Index.TypeByFQCN(typ.EnclosingFQCN)
	if !ok || !r.typeAccessible(enclosing, ctx) {
		return false
	}
	if enclosing.Kind == java.TypeKindInterface || java.HasModifier(typ.Modifier, "public") {
		return true
	}
	if java.HasModifier(typ.Modifier, "private") {
		return r.sameNest(typ.FQCN, ctx)
	}
	if r.samePackage(typ.FQCN, ctx) {
		return true
	}
	if java.HasModifier(typ.Modifier, "protected") {
		return r.callerIsSubclassOf(typ.EnclosingFQCN, ctx)
	}
	return false
}

func (r *SyntacticResolver) staticAccessible(candidate index.MethodResolution, ctx MethodContext, imported bool) bool {
	if candidate.DeclaringType == nil || candidate.Method == nil {
		return false
	}
	modifiers := candidate.Method.Modifier
	if java.HasModifier(modifiers, "private") {
		return !imported && r.sameNest(candidate.DeclaringType.FQCN, ctx)
	}
	if java.HasModifier(modifiers, "public") || candidate.DeclaringType.Kind == java.TypeKindInterface {
		return true
	}
	callerUnit := r.unitForContext(ctx)
	ownerUnit := r.Index.UnitForType(candidate.DeclaringType.FQCN)
	if callerUnit != nil && ownerUnit != nil && callerUnit.Package == ownerUnit.Package {
		return true
	}
	if java.HasModifier(modifiers, "protected") && ctx.EnclosingType != nil {
		for _, superclass := range r.Index.SuperclassChain(ctx.EnclosingType.FQCN) {
			if superclass.FQCN == candidate.DeclaringType.FQCN {
				return true
			}
		}
	}
	return false
}

func (r *SyntacticResolver) accessibleConstructors(owner *java.TypeDecl, ctx MethodContext, allowProtectedSubclass bool) []index.MethodResolution {
	result := make([]index.MethodResolution, 0)
	if r.Index == nil || owner == nil {
		return result
	}
	for _, candidate := range r.Index.ConstructorCandidates(owner.FQCN) {
		if r.constructorAccessible(candidate, ctx, allowProtectedSubclass) {
			result = append(result, candidate)
		}
	}
	return result
}

func (r *SyntacticResolver) constructorAccessible(candidate index.MethodResolution, ctx MethodContext, allowProtectedSubclass bool) bool {
	if candidate.DeclaringType == nil || candidate.Method == nil {
		return false
	}
	if r.sameNest(candidate.DeclaringType.FQCN, ctx) {
		return true
	}
	modifiers := candidate.Method.Modifier
	if java.HasModifier(modifiers, "private") {
		return false
	}
	if java.HasModifier(modifiers, "public") {
		return true
	}
	callerUnit := r.unitForContext(ctx)
	ownerUnit := r.Index.UnitForType(candidate.DeclaringType.FQCN)
	if callerUnit != nil && ownerUnit != nil && callerUnit.Package == ownerUnit.Package {
		return true
	}
	if allowProtectedSubclass && java.HasModifier(modifiers, "protected") && ctx.EnclosingType != nil {
		for _, superclass := range r.Index.SuperclassChain(ctx.EnclosingType.FQCN) {
			if superclass.FQCN == candidate.DeclaringType.FQCN {
				return true
			}
		}
	}
	return false
}

func (r *SyntacticResolver) samePackage(ownerFQCN string, ctx MethodContext) bool {
	if r.Index == nil {
		return false
	}
	callerUnit := r.unitForContext(ctx)
	ownerUnit := r.Index.UnitForType(ownerFQCN)
	return callerUnit != nil && ownerUnit != nil && callerUnit.Package == ownerUnit.Package
}

func (r *SyntacticResolver) sameNest(ownerFQCN string, ctx MethodContext) bool {
	if r.Index == nil || ctx.EnclosingType == nil {
		return false
	}
	return r.nestRoot(ownerFQCN) == r.nestRoot(ctx.EnclosingType.FQCN)
}

func (r *SyntacticResolver) nestRoot(fqcn string) string {
	for {
		typ, ok := r.Index.TypeByFQCN(fqcn)
		if !ok || typ.EnclosingFQCN == "" {
			return fqcn
		}
		fqcn = typ.EnclosingFQCN
	}
}

func (r *SyntacticResolver) callerIsSubclassOf(ownerFQCN string, ctx MethodContext) bool {
	if r.Index == nil || ctx.EnclosingType == nil {
		return false
	}
	for _, superclass := range r.Index.SuperclassChain(ctx.EnclosingType.FQCN) {
		if superclass.FQCN == ownerFQCN {
			return true
		}
	}
	return false
}

func (r *SyntacticResolver) unitForContext(ctx MethodContext) *java.CompilationUnit {
	if r.Index == nil {
		return nil
	}
	if unit := r.Index.UnitsByFile[ctx.File]; unit != nil {
		return unit
	}
	if ctx.EnclosingType != nil {
		return r.Index.UnitForType(ctx.EnclosingType.FQCN)
	}
	return nil
}

func arityCompatible(method java.MethodDecl, argCount int) bool {
	paramCount := len(method.Params)
	if paramCount == 0 || !method.Params[paramCount-1].Variadic {
		return argCount == paramCount
	}
	return argCount >= paramCount-1
}
