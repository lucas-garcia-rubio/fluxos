package graph

import (
	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
	"github.com/lucas-garcia-rubio/fluxos/internal/index"
	"github.com/lucas-garcia-rubio/fluxos/internal/resolve"
)

// Walk faz DFS recursivo a partir de (enclosingType, method), construindo o
// grafo transitivo de chamadas em g. Usa gray/black coloring pra detectar
// ciclos e evitar reprocessamento.
//
// Algoritmo:
//  1. Se handle atual é gray → ciclo; retorna (aresta já foi adicionada pelo caller).
//  2. Se handle atual é black → já processado; retorna.
//  3. Marca gray.
//  4. Para cada CallSite em method.Calls, pergunta Resolver → Resolution.
//     Para cada target em Resolution.Targets, adiciona aresta e recursa
//     (só se o target existe no projeto; external targets ficam como aresta sem recursão).
//  5. Marca black.
//
// method.Calls já vem populado do Passo 2 (extractCalls). Walk não re-extrai.
func Walk(
	g *Graph,
	enclosingType *java.TypeDecl,
	method java.MethodDecl,
	table *index.Table,
	resolver resolve.Resolver,
) {
	handle := resolve.MethodHandle{
		TypeFQCN:  enclosingType.FQCN,
		Method:    method.Name,
		Signature: method.Signature,
	}

	// Cycle detection: gray = visitando agora na pilha atual.
	if g.IsGray(handle) {
		return
	}
	// Já processado em visita anterior.
	if g.IsBlack(handle) {
		return
	}

	g.MarkGray(handle)

	for _, call := range method.Calls {
		ctx := resolve.MethodContext{
			EnclosingType: enclosingType,
			LocalVars:     method.LocalVars,
			File:          enclosingType.File,
		}
		resolution := resolver.Resolve(call, ctx)
		for _, target := range resolution.Targets {
			cycle := g.IsGray(target)
			g.AddEdge(handle, target, call, cycle)
			// Recursão só se target existe no projeto. External target
			// (biblioteca, reflexão) fica como aresta terminal.
			targetType, typeExists := table.TypeByFQCN(target.TypeFQCN)
			targetMethod, methodExists := table.Method(target.TypeFQCN, java.MethodKey{
				Name:      target.Method,
				Signature: target.Signature,
			})
			if typeExists && methodExists {
				Walk(g, targetType, *targetMethod, table, resolver)
			}
		}
	}

	g.MarkBlack(handle)
}
