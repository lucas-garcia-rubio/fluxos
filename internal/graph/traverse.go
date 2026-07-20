package graph

import (
	"github.com/lucas-garcia-rubio/fluxos/internal/index"
	"github.com/lucas-garcia-rubio/fluxos/internal/resolve"
)

// Walk mantém a assinatura M3 para callers internos: aplica o Build ilimitado
// e copia o resultado no *Graph recebido. A lógica real de planejamento e
// emissão vive em build.go desde o Passo 6.
func Walk(
	g *Graph,
	root resolve.ExecutionKey,
	table *index.Table,
	resolver resolve.Resolver,
) {
	result := Build(root, table, resolver, BuildOptions{})
	for key, node := range result.Graph.Nodes {
		g.Nodes[key] = node
	}
	g.Edges = append(g.Edges, result.Graph.Edges...)
}

// dispatchCandidates espelha ImplementationCandidate.ImplementationFQCN quando
// o DispatchSite carrega múltiplas impls ambíguas. Renderer usa isso para
// popular Node.Candidates em terminais ambiguousImplementation.
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
