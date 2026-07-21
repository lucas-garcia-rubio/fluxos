package graph

import (
	"container/heap"
	"sort"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
	"github.com/lucas-garcia-rubio/fluxos/internal/index"
	"github.com/lucas-garcia-rubio/fluxos/internal/resolve"
)

// BuildOptions controla os limites aplicados durante o planning do Build.
// MaxDepth e MaxNodes zero significam unlimited. Valores negativos nunca
// chegam aqui: o parser de CLI rejeita antes.
type BuildOptions struct {
	MaxDepth int
	MaxNodes int
}

// BuildResult é a saída do Build. Graph carrega apenas os nodes que foram
// efetivamente admitidos (Concrete) ou marcados como externais/terminais.
// Truncations é metadata out-of-band sobre o que foi omitido por limites.
type BuildResult struct {
	Graph       *Graph
	Truncations []Truncation
}

// Build substitui o DFS direto (Walk) por uma engine de duas fases: planning
// por menor depth e emission DFS sobre as keys admitidas. Walk permanece
// disponível como wrapper unlimited, preservando callers M3.
func Build(root resolve.ExecutionKey, table *index.Table, resolver resolve.Resolver, opts BuildOptions) BuildResult {
	g := NewGraph()
	result := BuildResult{Graph: g, Truncations: []Truncation{}}

	// Phase 1 — planning por menor depth. planned contains every analysis node
	// that may be emitted, including terminal and external targets. concreteDepth
	// is kept separately because only concrete targets enter the frontier.
	planned := map[resolve.ExecutionKey]struct{}{root: {}}
	concreteDepth := map[resolve.ExecutionKey]int{root: 0}
	plans := map[resolve.ExecutionKey]*resolvedPlan{}
	terminals := map[resolve.ExecutionKey]terminalMarker{}
	truncationSet := map[string]Truncation{}

	frontier := &frontierHeap{}
	heap.Init(frontier)
	heap.Push(frontier, &frontierItem{key: root, depth: 0})

	for frontier.Len() > 0 {
		item := heap.Pop(frontier).(*frontierItem)
		key, depth := item.key, item.depth
		if existing, ok := concreteDepth[key]; ok && existing < depth {
			continue
		}

		if plans[key] == nil {
			plans[key] = buildPlan(key, table, resolver)
		}
		plan := plans[key]
		if plan == nil {
			continue
		}

		if opts.MaxDepth > 0 && depth >= opts.MaxDepth {
			recordMaxDepthTruncations(truncationSet, key, plan)
			continue
		}

		for i := range plan.calls {
			pc := plan.calls[i]
			for _, pt := range pc.policyTruncations {
				recordTruncation(truncationSet, Truncation{
					Kind: TruncationMaxImpls, Caller: pt.Caller, Call: pt.Call, Omitted: pt.Omitted, Note: pt.Note,
				})
			}
			omittedHere := 0
			for _, target := range pc.targets {
				if _, seen := planned[target.Key]; seen {
					if target.Kind == resolve.ResolutionConcrete {
						candidateDepth := depth + 1
						if existing, ok := concreteDepth[target.Key]; ok && candidateDepth < existing {
							concreteDepth[target.Key] = candidateDepth
							heap.Push(frontier, &frontierItem{key: target.Key, depth: candidateDepth})
						}
					} else {
						registerTerminal(terminals, target, pc.site)
					}
					continue
				}
				if opts.MaxNodes > 0 && len(planned)+1 > opts.MaxNodes {
					omittedHere++
					continue
				}

				planned[target.Key] = struct{}{}
				if target.Kind == resolve.ResolutionConcrete {
					concreteDepth[target.Key] = depth + 1
					heap.Push(frontier, &frontierItem{key: target.Key, depth: depth + 1})
				} else {
					registerTerminal(terminals, target, pc.site)
				}
			}
			if omittedHere > 0 {
				recordTruncation(truncationSet, Truncation{
					Kind: TruncationMaxNodes, Caller: key, Call: pc.call, Omitted: omittedHere,
				})
			}
		}
	}

	// Phase 2 — emission DFS sobre keys admitidas, em ordem source.
	emit(g, root, plans, planned, concreteDepth, terminals, opts.MaxDepth)

	result.Truncations = make([]Truncation, 0, len(truncationSet))
	for _, t := range truncationSet {
		result.Truncations = append(result.Truncations, t)
	}
	sort.Slice(result.Truncations, func(i, j int) bool {
		return compareTruncations(result.Truncations[i], result.Truncations[j]) < 0
	})
	return result
}

type resolvedPlan struct {
	enclosingType *java.TypeDecl
	method        *java.MethodDecl
	calls         []planCall
}

type planCall struct {
	call              java.CallSite
	site              *resolve.DispatchSite
	targets           []resolve.ResolvedTarget
	policyTruncations []resolve.PolicyTruncation
}

type terminalMarker struct {
	kind       NodeKind
	note       string
	candidates []string
}

func buildPlan(key resolve.ExecutionKey, table *index.Table, resolver resolve.Resolver) *resolvedPlan {
	enclosingType, typeExists := table.TypeByFQCN(key.Method.TypeFQCN)
	method, methodExists := table.Method(key.Method.TypeFQCN, java.MethodKey{
		Name: key.Method.Method, Signature: key.Method.Signature,
	})
	if !typeExists || !methodExists {
		return nil
	}
	plan := &resolvedPlan{enclosingType: enclosingType, method: method}
	if len(method.Calls) == 0 {
		return plan
	}
	plan.calls = make([]planCall, 0, len(method.Calls))
	ctx := resolve.MethodContext{
		EnclosingType: enclosingType,
		Execution:     key,
		Params:        method.Params,
		LocalVars:     method.LocalVars,
		File:          enclosingType.File,
	}
	for _, call := range method.Calls {
		resolution := resolver.Resolve(call, ctx)
		plan.calls = append(plan.calls, planCall{
			call:              call,
			site:              resolution.DispatchSite,
			targets:           resolution.Targets,
			policyTruncations: resolution.Truncations,
		})
	}
	return plan
}

func registerTerminal(terminals map[resolve.ExecutionKey]terminalMarker, target resolve.ResolvedTarget, site *resolve.DispatchSite) {
	if target.Kind == resolve.ResolutionExternal {
		if _, ok := terminals[target.Key]; !ok {
			terminals[target.Key] = terminalMarker{kind: NodeExternal}
		}
		return
	}
	if _, ok := terminals[target.Key]; ok {
		return
	}
	terminals[target.Key] = terminalMarker{
		kind:       toNodeKind(target.Kind),
		note:       target.Note,
		candidates: dispatchCandidates(site),
	}
}

func recordMaxDepthTruncations(set map[string]Truncation, caller resolve.ExecutionKey, plan *resolvedPlan) {
	for i := range plan.calls {
		pc := plan.calls[i]
		if len(pc.targets) > 0 {
			recordTruncation(set, Truncation{
				Kind: TruncationMaxDepth, Caller: caller, Call: pc.call, Omitted: len(pc.targets),
			})
		}
	}
}

func recordTruncation(set map[string]Truncation, t Truncation) {
	id := t.ID()
	if existing, ok := set[id]; ok {
		if existing.Omitted < t.Omitted {
			existing.Omitted = t.Omitted
			set[id] = existing
		}
		return
	}
	set[id] = t
}

func emit(g *Graph, key resolve.ExecutionKey, plans map[resolve.ExecutionKey]*resolvedPlan, planned map[resolve.ExecutionKey]struct{}, concreteDepth map[resolve.ExecutionKey]int, terminals map[resolve.ExecutionKey]terminalMarker, maxDepth int) {
	if _, ok := planned[key]; !ok {
		return
	}
	if g.IsGray(key) || g.IsBlack(key) {
		return
	}
	plan := plans[key]
	if plan == nil {
		// Concrete admitido cujo tipo/método não existe no index: o node é
		// criado pelo AddEdge do caller, mas permanece StateWhite para reter
		// compatibilidade com Walk M3 (não expande, não é gray/black).
		return
	}
	g.MarkGray(key)
	if maxDepth > 0 && concreteDepth[key] >= maxDepth {
		g.MarkBlack(key)
		return
	}
	for i := range plan.calls {
		pc := plan.calls[i]
		for _, target := range pc.targets {
			switch target.Kind {
			case resolve.ResolutionConcrete:
				if _, ok := planned[target.Key]; !ok {
					continue
				}
				cycle := g.IsGray(target.Key)
				g.AddEdge(key, target.Key, pc.call, pc.site, cycle)
				if !cycle {
					emit(g, target.Key, plans, planned, concreteDepth, terminals, maxDepth)
				}
			case resolve.ResolutionExternal:
				if _, ok := planned[target.Key]; !ok {
					continue
				}
				g.AddEdge(key, target.Key, pc.call, pc.site, false)
				g.MarkExternal(target.Key)
			default:
				if _, ok := planned[target.Key]; !ok {
					continue
				}
				g.AddEdge(key, target.Key, pc.call, pc.site, false)
				info := terminals[target.Key]
				g.MarkTerminal(target.Key, info.kind, info.note, info.candidates)
			}
		}
	}
	g.MarkBlack(key)
}

type frontierItem struct {
	key   resolve.ExecutionKey
	depth int
	index int
}

type frontierHeap []*frontierItem

func (h frontierHeap) Len() int { return len(h) }

func (h frontierHeap) Less(i, j int) bool {
	if h[i].depth != h[j].depth {
		return h[i].depth < h[j].depth
	}
	return compareExecutionKeysLocal(h[i].key, h[j].key) < 0
}

func (h frontierHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *frontierHeap) Push(x any) {
	item := x.(*frontierItem)
	item.index = len(*h)
	*h = append(*h, item)
}

func (h *frontierHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	*h = old[:n-1]
	return item
}
