package resolve

import (
	"fmt"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
)

// SyntacticResolver é a implementação concreta do Resolver baseada em
// tree-sitter + heurísticas sobre a AST. Não usa type info beyond what's
// diretamente acessível no MethodContext.
//
// Passo 7 cobre `this.method()`, chamadas unqualified e receptores que sejam
// fields ou variáveis locais. Passo 8 adiciona super/static.
type SyntacticResolver struct {
	Types []*java.TypeDecl
}

// NewSyntacticResolver constrói um resolver pronto pra usar.
func NewSyntacticResolver(types []*java.TypeDecl) *SyntacticResolver {
	return &SyntacticResolver{Types: types}
}

// Resolve decide qual método call aponta, baseado em call.Receiver e no
// MethodContext. Ver Passo 7 em PLANO_M2.md pra algoritmo completo.
func (r *SyntacticResolver) Resolve(call java.CallSite, ctx MethodContext) Resolution {
	switch call.Receiver {
	case "", "this":
		return r.resolveOnType(ctx.EnclosingType, call.MethodName)
	case "super":
		return Resolution{Note: "super receiver not handled yet"}
	default:
		return r.resolveIdentifier(call.Receiver, call.MethodName, ctx)
	}
}

func (r *SyntacticResolver) resolveIdentifier(receiver, methodName string, ctx MethodContext) Resolution {
	if typeName, ok := ctx.LocalVars[receiver]; ok {
		t := r.findTypeByName(typeName)
		if t == nil {
			return Resolution{Note: fmt.Sprintf("local var type %q not found in project", typeName)}
		}
		return r.resolveOnType(t, methodName)
	}

	if ctx.EnclosingType != nil {
		for _, field := range ctx.EnclosingType.Fields {
			if field.Name != receiver {
				continue
			}
			t := r.findTypeByName(field.Type)
			if t == nil {
				return Resolution{Note: fmt.Sprintf("field type %q not found in project", field.Type)}
			}
			return r.resolveOnType(t, methodName)
		}
	}

	return Resolution{Note: fmt.Sprintf("receiver %q is not a local var or field", receiver)}
}

func (r *SyntacticResolver) findTypeByName(name string) *java.TypeDecl {
	for _, t := range r.Types {
		if t.Name == name || t.FQCN == name {
			return t
		}
	}
	return nil
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
