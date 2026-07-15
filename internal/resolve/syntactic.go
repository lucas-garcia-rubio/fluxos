package resolve

import (
	"fmt"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
)

// SyntacticResolver é a implementação concreta do Resolver baseada em
// tree-sitter + heurísticas sobre a AST. Não usa type info beyond what's
// diretamente acessível no MethodContext.
//
// Passo 6 cobre só `this.method()` e `method()` unqualified. Passo 7 adiciona
// field/localvar; Passo 8 adiciona super/static. Outros casos viram unresolved
// com note explicando.
type SyntacticResolver struct {
	// Passo 6: sem estado. Passo 7+ vai usar pra lookups (field types, etc.).
}

// NewSyntacticResolver constrói um resolver pronto pra usar.
func NewSyntacticResolver() *SyntacticResolver {
	return &SyntacticResolver{}
}

// Resolve decide qual método call aponta, baseado em call.Receiver e no
// MethodContext. Ver Passo 6 em PLANO_M2.md pra algoritmo completo.
func (r *SyntacticResolver) Resolve(call java.CallSite, ctx MethodContext) Resolution {
	switch call.Receiver {
	case "", "this":
		return r.resolveOnType(ctx.EnclosingType, call.MethodName)
	default:
		return Resolution{
			Note: fmt.Sprintf("receiver not handled in Passo 6: %q", call.Receiver),
		}
	}
}

// resolveOnType procura methodName nos Methods de t. Devolve Resolution com
// 1 target se achar, ou Resolution com Note explicando se não.
func (r *SyntacticResolver) resolveOnType(t *java.TypeDecl, methodName string) Resolution {
	if t == nil {
		return Resolution{Note: "no enclosing type"}
	}
	for _, m := range t.Methods {
		if m.Name == methodName {
			return Resolution{
				Targets: []MethodHandle{{TypeFQCN: t.FQCN, Method: methodName}},
			}
		}
	}
	return Resolution{
		Note: fmt.Sprintf("method %q not found on %s", methodName, t.FQCN),
	}
}
