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

	superType := r.findTypeInFile(ctx.EnclosingType.SuperClass.FQCN, ctx.File)
	if superType == nil {
		return Resolution{Note: fmt.Sprintf("superclass %q not found in same file", ctx.EnclosingType.SuperClass.SignatureToken())}
	}
	return r.resolveOnType(superType, call)
}

func (r *SyntacticResolver) resolveIdentifier(receiver string, call java.CallSite, ctx MethodContext) Resolution {
	if typeName, ok := ctx.LocalVars[receiver]; ok {
		t, note := r.resolveType(typeName, ctx)
		if t == nil {
			return Resolution{Note: fmt.Sprintf("local var type %q unresolved: %s", typeName.Raw, note)}
		}
		return r.resolveOnType(t, call)
	}

	if ctx.EnclosingType != nil {
		for _, field := range ctx.EnclosingType.Fields {
			if field.Name != receiver {
				continue
			}
			t, note := r.resolveType(field.Type, ctx)
			if t == nil {
				return Resolution{Note: fmt.Sprintf("field type %q unresolved: %s", field.Type.Raw, note)}
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

func (r *SyntacticResolver) findTypeInFile(name, file string) *java.TypeDecl {
	if r.Index == nil {
		return nil
	}
	unit := r.Index.UnitsByFile[file]
	if unit == nil {
		return nil
	}
	for _, t := range unit.Types {
		if t.FQCN == name || t.Name == name {
			return t
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
	var candidates []java.MethodDecl
	for _, m := range t.Methods {
		if m.Name == call.MethodName && arityCompatible(m, call.ArgCount) {
			candidates = append(candidates, m)
		}
	}
	if len(candidates) == 1 {
		method := candidates[0]
		return Resolution{Targets: []MethodHandle{{
			TypeFQCN:  t.FQCN,
			Method:    method.Name,
			Signature: method.Signature,
		}}}
	}
	if len(candidates) > 1 {
		signatures := make([]string, len(candidates))
		for i, method := range candidates {
			signatures[i] = method.Signature
		}
		sort.Strings(signatures)
		return Resolution{Note: fmt.Sprintf("ambiguous overload %q on %s: %s", call.MethodName, t.FQCN, strings.Join(signatures, ", "))}
	}
	return Resolution{
		Note: fmt.Sprintf("method %q with arity %d not found on %s", call.MethodName, call.ArgCount, t.FQCN),
	}
}

func arityCompatible(method java.MethodDecl, argCount int) bool {
	paramCount := len(method.Params)
	if paramCount == 0 || !method.Params[paramCount-1].Variadic {
		return argCount == paramCount
	}
	return argCount >= paramCount-1
}
