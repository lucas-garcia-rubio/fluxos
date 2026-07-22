// Package render defines format-neutral views of a call graph.
package render

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
	"github.com/lucas-garcia-rubio/fluxos/internal/graph"
	"github.com/lucas-garcia-rubio/fluxos/internal/resolve"
)

type MethodView struct {
	TypeFQCN  string
	Method    string
	Signature string
}

type ExecutionView struct {
	Method          MethodView
	RuntimeTypeFQCN string
}

type ImplementationCandidateView struct {
	ImplementationFQCN string
	Target             MethodView
	Kind               string
	Note               string
}

type DispatchSiteView struct {
	ID           string
	Caller       ExecutionView
	ReceiverFQCN string
	Method       string
	Signature    string
	Call         CallView
	Candidates   []ImplementationCandidateView
}

type NodeKind string

const (
	NodeMethod                  NodeKind = "method"
	NodeExternal                NodeKind = "external"
	NodeUnresolved              NodeKind = "unresolved"
	NodeNoImplementation        NodeKind = "noImplementation"
	NodeAmbiguousType           NodeKind = "ambiguousType"
	NodeAmbiguousOverload       NodeKind = "ambiguousOverload"
	NodeAmbiguousImplementation NodeKind = "ambiguousImplementation"
)

type NodeView struct {
	ID         string
	Execution  ExecutionView
	Kind       NodeKind
	Label      string
	Note       string
	Candidates []string
}

type CallView struct {
	Kind       string
	File       string
	Line       int
	StartByte  uint
	Receiver   string
	MethodName string
}

type EdgeView struct {
	From         string
	To           string
	Call         CallView
	DispatchSite *DispatchSiteView
	Cycle        bool
}

type Snapshot struct {
	SchemaVersion int
	Target        ExecutionView
	Nodes         []NodeView
	Edges         []EdgeView
	Truncations   []TruncationView
}

// TruncationView é a representação detached de uma Truncation do graph package.
// Como Call carrega apenas tipos-valor e slices que a CallView já copia, a view
// fica independente do graph original.
type TruncationView struct {
	ID      string
	Kind    string
	Caller  ExecutionView
	Call    CallView
	Omitted int
	Note    string
}

const SnapshotSchemaVersion = 1

func NewSnapshot(g *graph.Graph, target resolve.ExecutionKey) Snapshot {
	return NewResultSnapshot(graph.BuildResult{Graph: g}, target)
}

func NewResultSnapshot(result graph.BuildResult, target resolve.ExecutionKey) Snapshot {
	return NewResultSnapshotWithIncludeUnresolved(result, target, true)
}

// NewResultSnapshotWithIncludeUnresolved projects a build result into the
// renderer-neutral view. Existing callers use NewResultSnapshot and retain
// unresolved terminals; false removes those nodes and every incident edge
// before context-sensitive IDs and ordering are computed.
func NewResultSnapshotWithIncludeUnresolved(result graph.BuildResult, target resolve.ExecutionKey, includeUnresolved bool) Snapshot {
	snapshot := Snapshot{
		SchemaVersion: SnapshotSchemaVersion,
		Target:        executionView(target, false),
		Nodes:         make([]NodeView, 0),
		Edges:         make([]EdgeView, 0),
		Truncations:   make([]TruncationView, 0),
	}
	g := result.Graph
	if g == nil {
		return snapshot
	}

	excluded := make(map[resolve.ExecutionKey]struct{})
	nodes := make([]*graph.Node, 0, len(g.Nodes))
	for _, node := range g.Nodes {
		if !includeUnresolved && node.Kind == graph.NodeTerminalUnresolved {
			excluded[node.Key] = struct{}{}
			continue
		}
		nodes = append(nodes, node)
	}
	contextCounts := make(map[resolve.MethodHandle]int, len(nodes))
	for _, node := range nodes {
		contextCounts[node.Key.Method]++
	}
	sort.Slice(nodes, func(i, j int) bool {
		return compareExecutionKeys(nodes[i].Key, nodes[j].Key) < 0
	})
	for _, node := range nodes {
		kind := nodeKind(node.Kind)
		execution := executionView(node.Key, isTerminalKind(kind))
		candidates := append([]string{}, node.Candidates...)
		sort.Strings(candidates)
		runtimeAware := contextCounts[node.Key.Method] > 1
		snapshot.Nodes = append(snapshot.Nodes, NodeView{
			ID:         stableExecutionID(node.Key, runtimeAware),
			Execution:  execution,
			Kind:       kind,
			Label:      nodeLabel(execution.Method, kind, len(candidates), runtimeLabel(node.Key, runtimeAware)),
			Note:       node.Note,
			Candidates: candidates,
		})
	}

	edges := append([]graph.Edge{}, g.Edges...)
	if !includeUnresolved {
		filtered := edges[:0]
		for _, edge := range edges {
			if _, excludedFrom := excluded[edge.From]; excludedFrom {
				continue
			}
			if _, excludedTo := excluded[edge.To]; excludedTo {
				continue
			}
			filtered = append(filtered, edge)
		}
		edges = filtered
	}
	sort.SliceStable(edges, func(i, j int) bool {
		return compareEdges(edges[i], edges[j]) < 0
	})
	for _, edge := range edges {
		snapshot.Edges = append(snapshot.Edges, EdgeView{
			From:         stableExecutionID(edge.From, contextCounts[edge.From.Method] > 1),
			To:           stableExecutionID(edge.To, contextCounts[edge.To.Method] > 1),
			Call:         callView(edge.Call),
			DispatchSite: dispatchSiteView(edge.DispatchSite),
			Cycle:        edge.Cycle,
		})
	}

	truncations := append([]graph.Truncation{}, result.Truncations...)
	for _, tr := range truncations {
		snapshot.Truncations = append(snapshot.Truncations, TruncationView{
			ID:      tr.ID(),
			Kind:    string(tr.Kind),
			Caller:  executionView(tr.Caller, false),
			Call:    callView(tr.Call),
			Omitted: tr.Omitted,
			Note:    tr.Note,
		})
	}
	return snapshot
}

func stableNodeID(handle resolve.MethodHandle) string {
	sum := sha256.Sum256([]byte(handle.TypeFQCN + "\x00" + handle.Method + "\x00" + handle.Signature))
	return fmt.Sprintf("m_%x", sum[:6])
}

func stableExecutionID(key resolve.ExecutionKey, runtimeAware bool) string {
	if !runtimeAware {
		return stableNodeID(key.Method)
	}
	sum := sha256.Sum256([]byte(key.Method.TypeFQCN + "\x00" + key.Method.Method + "\x00" + key.Method.Signature + "\x00" + key.RuntimeTypeFQCN))
	return fmt.Sprintf("m_%x", sum[:6])
}

func executionView(key resolve.ExecutionKey, terminal bool) ExecutionView {
	return ExecutionView{Method: methodView(key.Method, terminal), RuntimeTypeFQCN: key.RuntimeTypeFQCN}
}

func methodView(handle resolve.MethodHandle, terminal bool) MethodView {
	typeFQCN := handle.TypeFQCN
	if terminal {
		if separator := strings.IndexByte(typeFQCN, '#'); separator >= 0 {
			typeFQCN = typeFQCN[:separator]
		}
	}
	return MethodView{TypeFQCN: typeFQCN, Method: handle.Method, Signature: handle.Signature}
}

func nodeKind(kind graph.NodeKind) NodeKind {
	switch kind {
	case graph.NodeExternal:
		return NodeExternal
	case graph.NodeTerminalUnresolved:
		return NodeUnresolved
	case graph.NodeTerminalNoImplementation:
		return NodeNoImplementation
	case graph.NodeTerminalAmbiguousType:
		return NodeAmbiguousType
	case graph.NodeTerminalAmbiguousOverload:
		return NodeAmbiguousOverload
	case graph.NodeTerminalAmbiguousImplementation:
		return NodeAmbiguousImplementation
	default:
		return NodeMethod
	}
}

func isTerminalKind(kind NodeKind) bool {
	switch kind {
	case NodeUnresolved, NodeNoImplementation, NodeAmbiguousType, NodeAmbiguousOverload, NodeAmbiguousImplementation:
		return true
	default:
		return false
	}
}

func nodeLabel(method MethodView, kind NodeKind, candidateCount int, runtime string) string {
	typeFQCN := method.TypeFQCN
	if typeFQCN == "" {
		typeFQCN = "<unknown>"
	}
	signature := method.Signature
	if signature == "" {
		signature = "()"
	}
	base := typeFQCN + "." + method.Method + signature
	var label string
	switch kind {
	case NodeUnresolved:
		label = base + " [unresolved]"
	case NodeNoImplementation:
		label = base + " [no implementation]"
	case NodeAmbiguousType:
		label = base + " [ambiguous type]"
	case NodeAmbiguousOverload:
		label = base + " [ambiguous overload]"
	case NodeAmbiguousImplementation:
		label = fmt.Sprintf("%s [ambiguous: %d implementations]", base, candidateCount)
	default:
		label = base
	}
	if runtime != "" {
		label += " [runtime: " + runtime + "]"
	}
	return label
}

func runtimeLabel(key resolve.ExecutionKey, runtimeAware bool) string {
	if runtimeAware {
		return key.RuntimeTypeFQCN
	}
	return ""
}

func callView(call java.CallSite) CallView {
	return CallView{
		Kind: call.Kind.String(), File: call.File, Line: call.Line, StartByte: call.StartByte,
		Receiver: call.Receiver, MethodName: call.MethodName,
	}
}

func dispatchSiteView(site *resolve.DispatchSite) *DispatchSiteView {
	if site == nil {
		return nil
	}
	view := &DispatchSiteView{
		ID: string(site.ID), Caller: executionView(site.Caller, false), ReceiverFQCN: site.ReceiverFQCN,
		Method: site.Method, Signature: site.Signature, Call: callView(site.Call),
		Candidates: make([]ImplementationCandidateView, len(site.Candidates)),
	}
	for i, candidate := range site.Candidates {
		view.Candidates[i] = ImplementationCandidateView{
			ImplementationFQCN: candidate.ImplementationFQCN,
			Target:             methodView(candidate.Target, false),
			Kind:               resolutionKind(candidate.Kind),
			Note:               candidate.Note,
		}
	}
	sort.Slice(view.Candidates, func(i, j int) bool { return compareCandidateViews(view.Candidates[i], view.Candidates[j]) < 0 })
	return view
}

func resolutionKind(kind resolve.ResolutionKind) string {
	switch kind {
	case resolve.ResolutionConcrete:
		return "concrete"
	case resolve.ResolutionExternal:
		return "external"
	case resolve.ResolutionUnresolved:
		return "unresolved"
	case resolve.ResolutionNoImplementation:
		return "noImplementation"
	case resolve.ResolutionAmbiguousType:
		return "ambiguousType"
	case resolve.ResolutionAmbiguousOverload:
		return "ambiguousOverload"
	case resolve.ResolutionAmbiguousImplementation:
		return "ambiguousImplementation"
	default:
		return "unknown"
	}
}

func compareEdges(a, b graph.Edge) int {
	if cmp := compareExecutionKeys(a.From, b.From); cmp != 0 {
		return cmp
	}
	if cmp := compareExecutionKeys(a.To, b.To); cmp != 0 {
		return cmp
	}
	if a.Call.File != b.Call.File {
		return compareStrings(a.Call.File, b.Call.File)
	}
	if a.Call.Line != b.Call.Line {
		return compareInts(a.Call.Line, b.Call.Line)
	}
	if a.Call.StartByte != b.Call.StartByte {
		return compareUints(a.Call.StartByte, b.Call.StartByte)
	}
	if a.Call.Kind != b.Call.Kind {
		return compareInts(int(a.Call.Kind), int(b.Call.Kind))
	}
	if a.Call.Receiver != b.Call.Receiver {
		return compareStrings(a.Call.Receiver, b.Call.Receiver)
	}
	if a.Call.MethodName != b.Call.MethodName {
		return compareStrings(a.Call.MethodName, b.Call.MethodName)
	}
	if dispatchSiteID(a.DispatchSite) != dispatchSiteID(b.DispatchSite) {
		return compareStrings(dispatchSiteID(a.DispatchSite), dispatchSiteID(b.DispatchSite))
	}
	if a.Cycle == b.Cycle {
		return 0
	}
	if !a.Cycle {
		return -1
	}
	return 1
}

func dispatchSiteID(site *resolve.DispatchSite) string {
	if site == nil {
		return ""
	}
	return string(site.ID)
}

func compareExecutionKeys(a, b resolve.ExecutionKey) int {
	if cmp := compareHandles(a.Method, b.Method); cmp != 0 {
		return cmp
	}
	return compareStrings(a.RuntimeTypeFQCN, b.RuntimeTypeFQCN)
}

func compareCandidateViews(a, b ImplementationCandidateView) int {
	if a.ImplementationFQCN != b.ImplementationFQCN {
		return compareStrings(a.ImplementationFQCN, b.ImplementationFQCN)
	}
	if cmp := compareMethodViews(a.Target, b.Target); cmp != 0 {
		return cmp
	}
	if a.Kind != b.Kind {
		return compareStrings(a.Kind, b.Kind)
	}
	return compareStrings(a.Note, b.Note)
}

func compareMethodViews(a, b MethodView) int {
	return compareHandles(
		resolve.MethodHandle{TypeFQCN: a.TypeFQCN, Method: a.Method, Signature: a.Signature},
		resolve.MethodHandle{TypeFQCN: b.TypeFQCN, Method: b.Method, Signature: b.Signature},
	)
}

func compareHandles(a, b resolve.MethodHandle) int {
	if a.TypeFQCN != b.TypeFQCN {
		return compareStrings(a.TypeFQCN, b.TypeFQCN)
	}
	if a.Method != b.Method {
		return compareStrings(a.Method, b.Method)
	}
	return compareStrings(a.Signature, b.Signature)
}

func compareStrings(a, b string) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func compareInts(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func compareUints(a, b uint) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
