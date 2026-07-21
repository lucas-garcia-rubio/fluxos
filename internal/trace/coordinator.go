// Package trace coordinates policy discovery and the authoritative graph
// build. It deliberately has no knowledge of prompts, terminals, or
// renderers.
package trace

import (
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/lucas-garcia-rubio/fluxos/internal/graph"
	"github.com/lucas-garcia-rubio/fluxos/internal/index"
	"github.com/lucas-garcia-rubio/fluxos/internal/resolve"
)

// BuildFactory constructs a fresh resolver and graph result for one
// coordinator build. A factory is called once for the nil-selector fast path,
// once for every discovery round, and once for the final build.
//
// The production factory below uses the policy supplied to it to construct a
// fresh syntactic resolver before calling graph.Build. Tests can replace it to
// provide deterministic graphs while still observing every policy and build.
type BuildFactory func(
	root resolve.ExecutionKey,
	table *index.Table,
	policy resolve.DispatchPolicy,
	opts graph.BuildOptions,
) graph.BuildResult

// Request contains the immutable inputs for one coordinated trace build.
type Request struct {
	Root         resolve.ExecutionKey
	Table        *index.Table
	BuildOptions graph.BuildOptions
	MaxImpls     int
	BasePolicy   resolve.DispatchPolicy
	Selector     ImplementationSelector

	// BuildFactory is optional. When nil, the coordinator uses the production
	// factory.
	BuildFactory BuildFactory
}

// Coordinator runs discovery rounds and then performs one fresh final build.
type Coordinator struct{}

// Build runs the nil-selector fast path or the site-choice discovery loop.
// Discovery results, including their truncations, are never returned.
func (c Coordinator) Build(req Request) (graph.BuildResult, error) {
	if req.Selector == nil {
		basePolicy := req.BasePolicy
		if basePolicy == nil {
			basePolicy = resolve.TerminalPolicy{}
		}
		result := c.buildFresh(req, basePolicy)
		return result, nil
	}

	// Discovery must be conservative for every site that has not been chosen
	// yet. BasePolicy belongs only to the non-interactive fast path: allowing an
	// AllPolicy or partial FixedPolicy fallback here would fan out speculative
	// implementations before the selector has chosen the parent site.
	discoveryFallback := resolve.TerminalPolicy{}
	choices := make(map[resolve.DispatchSiteID]resolve.DispatchChoice)
	for {
		discovery := c.buildFresh(req, resolve.SitePolicy{
			Choices:  cloneChoices(choices),
			Fallback: discoveryFallback,
			MaxImpls: req.MaxImpls,
		})

		frontier, err := frontierFromResult(discovery)
		if err != nil {
			return graph.BuildResult{}, err
		}
		frontier = excludeChosen(frontier, choices)
		if len(frontier) == 0 {
			break
		}

		batch, err := req.Selector.Select(detachedFrontier(frontier))
		if err != nil {
			return graph.BuildResult{}, err
		}
		validated, err := validateSelectionBatch(frontier, batch)
		if err != nil {
			return graph.BuildResult{}, err
		}
		if err := mergeWithProgress(choices, validated); err != nil {
			return graph.BuildResult{}, err
		}
	}

	// The last discovery result is intentionally discarded. The final build
	// must be a fresh resolver/build with the complete choice map.
	return c.buildFresh(req, resolve.SitePolicy{
		Choices:  cloneChoices(choices),
		Fallback: discoveryFallback,
		MaxImpls: req.MaxImpls,
	}), nil
}

func (c Coordinator) buildFresh(req Request, policy resolve.DispatchPolicy) graph.BuildResult {
	factory := req.BuildFactory
	if factory == nil {
		factory = productionBuildFactory
	}
	return factory(req.Root, req.Table, policy, req.BuildOptions)
}

func productionBuildFactory(root resolve.ExecutionKey, table *index.Table, policy resolve.DispatchPolicy, opts graph.BuildOptions) graph.BuildResult {
	resolver := resolve.NewSyntacticResolverWithPolicy(table, policy)
	return graph.Build(root, table, resolver, opts)
}

func cloneChoices(choices map[resolve.DispatchSiteID]resolve.DispatchChoice) map[resolve.DispatchSiteID]resolve.DispatchChoice {
	clone := make(map[resolve.DispatchSiteID]resolve.DispatchChoice, len(choices))
	for id, choice := range choices {
		clone[id] = choice
	}
	return clone
}

// frontierFromResult considers only dispatch sites attached to emitted edges.
// It also checks all sites, including already chosen ones, for conflicting
// metadata before the caller removes chosen IDs.
func frontierFromResult(result graph.BuildResult) ([]*resolve.DispatchSite, error) {
	if result.Graph == nil {
		return nil, nil
	}

	byID := make(map[resolve.DispatchSiteID]*resolve.DispatchSite)
	for _, edge := range result.Graph.Edges {
		if edge.DispatchSite == nil {
			continue
		}
		if edge.DispatchSite.ID == "" {
			return nil, fmt.Errorf("dispatch site has empty ID")
		}
		candidate := canonicalSite(edge.DispatchSite)
		if previous, ok := byID[candidate.ID]; ok {
			if !semanticSiteEqual(previous, candidate) {
				return nil, fmt.Errorf("dispatch site %q has conflicting metadata", candidate.ID)
			}
			continue
		}
		byID[candidate.ID] = candidate
	}

	frontier := make([]*resolve.DispatchSite, 0, len(byID))
	for _, site := range byID {
		frontier = append(frontier, site)
	}
	sort.Slice(frontier, func(i, j int) bool {
		return compareSites(frontier[i], frontier[j]) < 0
	})
	return frontier, nil
}

func excludeChosen(frontier []*resolve.DispatchSite, choices map[resolve.DispatchSiteID]resolve.DispatchChoice) []*resolve.DispatchSite {
	filtered := make([]*resolve.DispatchSite, 0, len(frontier))
	for _, site := range frontier {
		if _, chosen := choices[site.ID]; !chosen {
			filtered = append(filtered, site)
		}
	}
	return filtered
}

func detachedFrontier(frontier []*resolve.DispatchSite) []resolve.DispatchSite {
	detached := make([]resolve.DispatchSite, 0, len(frontier))
	for _, site := range frontier {
		clone := resolve.CloneDispatchSite(site)
		if clone == nil {
			continue
		}
		detached = append(detached, *clone)
	}
	return detached
}

func validateSelectionBatch(frontier []*resolve.DispatchSite, batch []Selection) (map[resolve.DispatchSiteID]resolve.DispatchChoice, error) {
	known := make(map[resolve.DispatchSiteID]*resolve.DispatchSite, len(frontier))
	for _, site := range frontier {
		known[site.ID] = site
	}
	if len(batch) != len(frontier) {
		return nil, fmt.Errorf("selector returned %d selections for %d dispatch sites", len(batch), len(frontier))
	}

	validated := make(map[resolve.DispatchSiteID]resolve.DispatchChoice, len(batch))
	for _, selection := range batch {
		if selection.SiteID == "" {
			return nil, fmt.Errorf("selector returned selection with empty SiteID")
		}
		if _, duplicate := validated[selection.SiteID]; duplicate {
			return nil, fmt.Errorf("selector returned duplicate selection for site %q", selection.SiteID)
		}
		site, ok := known[selection.SiteID]
		if !ok {
			return nil, fmt.Errorf("selector returned unknown dispatch site %q", selection.SiteID)
		}

		choice := selection.Choice
		switch choice.Mode {
		case resolve.ChoiceNone, resolve.ChoiceAll:
			if choice.ImplementationFQCN != "" {
				return nil, fmt.Errorf("choice for site %q has implementation FQCN with mode %q", selection.SiteID, choice.Mode)
			}
		case resolve.ChoiceSelected:
			if choice.ImplementationFQCN == "" {
				return nil, fmt.Errorf("selected choice for site %q has empty implementation FQCN", selection.SiteID)
			}
			if !hasCandidate(site, choice.ImplementationFQCN) {
				return nil, fmt.Errorf("implementation %q is not a candidate for dispatch site %q", choice.ImplementationFQCN, selection.SiteID)
			}
		default:
			return nil, fmt.Errorf("choice for site %q has unknown mode %q", selection.SiteID, choice.Mode)
		}
		validated[selection.SiteID] = choice
	}

	for _, site := range frontier {
		if _, ok := validated[site.ID]; !ok {
			return nil, fmt.Errorf("selector omitted dispatch site %q", site.ID)
		}
	}
	return validated, nil
}

func hasCandidate(site *resolve.DispatchSite, fqcn string) bool {
	for _, candidate := range site.Candidates {
		if candidate.ImplementationFQCN == fqcn {
			return true
		}
	}
	return false
}

func mergeWithProgress(choices map[resolve.DispatchSiteID]resolve.DispatchChoice, validated map[resolve.DispatchSiteID]resolve.DispatchChoice) error {
	before := len(choices)
	for id := range validated {
		if _, exists := choices[id]; exists {
			return fmt.Errorf("selection for dispatch site %q made no progress", id)
		}
	}
	for id, choice := range validated {
		choices[id] = choice
	}
	if len(choices) != before+len(validated) {
		return fmt.Errorf("selection batch made no progress")
	}
	return nil
}

func canonicalSite(site *resolve.DispatchSite) *resolve.DispatchSite {
	clone := resolve.CloneDispatchSite(site)
	if clone == nil {
		return nil
	}
	if len(clone.Call.Args) == 0 {
		clone.Call.Args = nil
	}
	if len(clone.Candidates) == 0 {
		clone.Candidates = nil
	} else {
		sort.SliceStable(clone.Candidates, func(i, j int) bool {
			return compareCandidates(clone.Candidates[i], clone.Candidates[j]) < 0
		})
	}
	return clone
}

func compareCandidates(left, right resolve.ImplementationCandidate) int {
	if result := strings.Compare(left.ImplementationFQCN, right.ImplementationFQCN); result != 0 {
		return result
	}
	if result := strings.Compare(left.Target.TypeFQCN, right.Target.TypeFQCN); result != 0 {
		return result
	}
	if result := strings.Compare(left.Target.Method, right.Target.Method); result != 0 {
		return result
	}
	if result := strings.Compare(left.Target.Signature, right.Target.Signature); result != 0 {
		return result
	}
	if left.Kind != right.Kind {
		if left.Kind < right.Kind {
			return -1
		}
		return 1
	}
	return strings.Compare(left.Note, right.Note)
}

func semanticSiteEqual(left, right *resolve.DispatchSite) bool {
	return reflect.DeepEqual(left, right)
}

func compareSites(left, right *resolve.DispatchSite) int {
	if left == nil || right == nil {
		if left == right {
			return 0
		}
		if left == nil {
			return -1
		}
		return 1
	}
	leftStrings := []string{
		left.Caller.Method.TypeFQCN,
		left.Caller.Method.Method,
		left.Caller.Method.Signature,
		left.Caller.RuntimeTypeFQCN,
		logicalSourcePath(left.Call.File),
		left.Call.Kind.String(),
		left.ReceiverFQCN,
		left.Method,
		left.Signature,
		string(left.ID),
	}
	rightStrings := []string{
		right.Caller.Method.TypeFQCN,
		right.Caller.Method.Method,
		right.Caller.Method.Signature,
		right.Caller.RuntimeTypeFQCN,
		logicalSourcePath(right.Call.File),
		right.Call.Kind.String(),
		right.ReceiverFQCN,
		right.Method,
		right.Signature,
		string(right.ID),
	}
	for i := 0; i < 5; i++ {
		if result := strings.Compare(leftStrings[i], rightStrings[i]); result != 0 {
			return result
		}
	}
	if left.Call.StartByte != right.Call.StartByte {
		if left.Call.StartByte < right.Call.StartByte {
			return -1
		}
		return 1
	}
	for i := 5; i < len(leftStrings); i++ {
		if result := strings.Compare(leftStrings[i], rightStrings[i]); result != 0 {
			return result
		}
	}
	return 0
}

func logicalSourcePath(path string) string {
	path = filepath.ToSlash(filepath.Clean(path))
	for _, marker := range []string{"/src/main/java/", "/src/test/java/"} {
		if index := strings.LastIndex(path, marker); index >= 0 {
			return strings.TrimPrefix(path[index+1:], "/")
		}
	}
	return strings.TrimPrefix(path, "./")
}
