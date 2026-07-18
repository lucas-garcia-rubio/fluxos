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
//     Para cada target em Resolution.Targets:
//     - adiciona aresta (handle → target.Handle);
//     - se target.Kind == Concrete, recursa em target se type+method existirem no índice;
//     - se target.Kind == External, marca Node como NodeExternal (não recursa);
//     - qualquer outro Kind é terminal: marca Node como NodeTerminal* com Note/Candidates.
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
			Params:        method.Params,
			LocalVars:     method.LocalVars,
			File:          enclosingType.File,
		}
		resolution := resolver.Resolve(call, ctx)
		for _, target := range resolution.Targets {
			cycle := g.IsGray(target.Handle)
			g.AddEdge(handle, target.Handle, call, cycle)
			switch target.Kind {
			case resolve.ResolutionConcrete:
				if cycle {
					continue
				}
				targetType, typeExists := table.TypeByFQCN(target.Handle.TypeFQCN)
				targetMethod, methodExists := table.Method(target.Handle.TypeFQCN, java.MethodKey{
					Name:      target.Handle.Method,
					Signature: target.Handle.Signature,
				})
				if typeExists && methodExists {
					Walk(g, targetType, *targetMethod, table, resolver)
				}
			case resolve.ResolutionExternal:
				g.MarkExternal(target.Handle)
			default:
				g.MarkTerminal(target.Handle, toNodeKind(target.Kind), target.Note, target.Candidates)
			}
		}
	}

	g.MarkBlack(handle)
}

// toNodeKind mapeia ResolutionKind para NodeKind. Concrete e External não são
// terminais (NodeMethod e NodeExternal respectivamente); os cinco kinds
// restantes são 1-para-1 com NodeTerminal*.
func toNodeKind(kind resolve.ResolutionKind) NodeKind {
	switch kind {
	case resolve.ResolutionUnresolved:
		return NodeTerminalUnresolved
	case resolve.ResolutionNoImplementation:
		return NodeTerminalNoImplementation
	case resolve.ResolutionAmbiguousType:
		return NodeTerminalAmbiguousType
	case resolve.ResolutionAmbiguousOverload:
		return NodeTerminalAmbiguousOverload
	case resolve.ResolutionAmbiguousImplementation:
		return NodeTerminalAmbiguousImplementation
	default:
		return NodeMethod
	}
}
