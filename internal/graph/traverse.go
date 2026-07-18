package graph

import (
	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
	"github.com/lucas-garcia-rubio/fluxos/internal/index"
	"github.com/lucas-garcia-rubio/fluxos/internal/resolve"
)

// Walk faz DFS recursivo a partir de uma execution key, construindo o
// grafo transitivo de chamadas em g. Usa gray/black coloring pra detectar
// ciclos e evitar reprocessamento.
//
// Algoritmo:
//  1. Se a execution atual é gray → ciclo; retorna (aresta já foi adicionada pelo caller).
//  2. Se a execution atual é black → já processada; retorna.
//  3. Marca gray.
//  4. Para cada CallSite em method.Calls, pergunta Resolver → Resolution.
//     Para cada target em Resolution.Targets:
//     - adiciona aresta (root → target.Key) e copia eventual DispatchSite;
//     - se target.Kind == Concrete, recursa em target se type+method existirem no índice;
//     - se target.Kind == External, marca Node como NodeExternal (não recursa);
//     - qualquer outro Kind é terminal: marca Node como NodeTerminal* com Note/Candidates.
//  5. Marca black.
//
// method.Calls já vem populado do Passo 2 (extractCalls). Walk não re-extrai.
func Walk(
	g *Graph,
	root resolve.ExecutionKey,
	table *index.Table,
	resolver resolve.Resolver,
) {
	// Cycle detection: gray = visitando agora na pilha atual.
	if g.IsGray(root) {
		return
	}
	// Já processado em visita anterior.
	if g.IsBlack(root) {
		return
	}

	enclosingType, typeExists := table.TypeByFQCN(root.Method.TypeFQCN)
	method, methodExists := table.Method(root.Method.TypeFQCN, java.MethodKey{
		Name: root.Method.Method, Signature: root.Method.Signature,
	})
	if !typeExists || !methodExists {
		return
	}

	g.MarkGray(root)

	for _, call := range method.Calls {
		ctx := resolve.MethodContext{
			EnclosingType: enclosingType,
			Execution:     root,
			Params:        method.Params,
			LocalVars:     method.LocalVars,
			File:          enclosingType.File,
		}
		resolution := resolver.Resolve(call, ctx)
		for _, target := range resolution.Targets {
			cycle := g.IsGray(target.Key)
			g.AddEdge(root, target.Key, call, resolution.DispatchSite, cycle)
			switch target.Kind {
			case resolve.ResolutionConcrete:
				if cycle {
					continue
				}
				Walk(g, target.Key, table, resolver)
			case resolve.ResolutionExternal:
				g.MarkExternal(target.Key)
			default:
				g.MarkTerminal(target.Key, toNodeKind(target.Kind), target.Note, dispatchCandidates(resolution.DispatchSite))
			}
		}
	}

	g.MarkBlack(root)
}

func dispatchCandidates(site *resolve.DispatchSite) []string {
	if site == nil || len(site.Candidates) == 0 {
		return nil
	}
	candidates := make([]string, len(site.Candidates))
	for i, candidate := range site.Candidates {
		candidates[i] = candidate.ImplementationFQCN
	}
	return candidates
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
