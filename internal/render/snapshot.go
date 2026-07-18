// Package render defines format-neutral views of a call graph.
package render

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"

	"github.com/lucas-garcia-rubio/fluxos/internal/graph"
	"github.com/lucas-garcia-rubio/fluxos/internal/resolve"
)

type MethodView struct {
	TypeFQCN  string
	Method    string
	Signature string
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
	Method     MethodView
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
	From  string
	To    string
	Call  CallView
	Cycle bool
}

type Snapshot struct {
	Target MethodView
	Nodes  []NodeView
	Edges  []EdgeView
}

func NewSnapshot(g *graph.Graph, target resolve.MethodHandle) Snapshot {
	snapshot := Snapshot{
		Target: methodView(target, false),
		Nodes:  make([]NodeView, 0),
		Edges:  make([]EdgeView, 0),
	}
	if g == nil {
		return snapshot
	}

	nodes := make([]*graph.Node, 0, len(g.Nodes))
	for _, node := range g.Nodes {
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(i, j int) bool {
		return compareHandles(nodes[i].Handle, nodes[j].Handle) < 0
	})
	for _, node := range nodes {
		kind := nodeKind(node.Kind)
		method := methodView(node.Handle, isTerminalKind(kind))
		candidates := append([]string{}, node.Candidates...)
		sort.Strings(candidates)
		snapshot.Nodes = append(snapshot.Nodes, NodeView{
			ID:         stableNodeID(node.Handle),
			Method:     method,
			Kind:       kind,
			Label:      nodeLabel(method, kind, len(candidates)),
			Note:       node.Note,
			Candidates: candidates,
		})
	}

	edges := append([]graph.Edge{}, g.Edges...)
	sort.SliceStable(edges, func(i, j int) bool {
		return compareEdges(edges[i], edges[j]) < 0
	})
	for _, edge := range edges {
		snapshot.Edges = append(snapshot.Edges, EdgeView{
			From: stableNodeID(edge.From),
			To:   stableNodeID(edge.To),
			Call: CallView{
				Kind:       edge.Call.Kind.String(),
				File:       edge.Call.File,
				Line:       edge.Call.Line,
				StartByte:  edge.Call.StartByte,
				Receiver:   edge.Call.Receiver,
				MethodName: edge.Call.MethodName,
			},
			Cycle: edge.Cycle,
		})
	}
	return snapshot
}

func stableNodeID(handle resolve.MethodHandle) string {
	sum := sha256.Sum256([]byte(handle.TypeFQCN + "\x00" + handle.Method + "\x00" + handle.Signature))
	return fmt.Sprintf("m_%x", sum[:6])
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

func nodeLabel(method MethodView, kind NodeKind, candidateCount int) string {
	typeFQCN := method.TypeFQCN
	if typeFQCN == "" {
		typeFQCN = "<unknown>"
	}
	signature := method.Signature
	if signature == "" {
		signature = "()"
	}
	base := typeFQCN + "." + method.Method + signature
	switch kind {
	case NodeUnresolved:
		return base + " [unresolved]"
	case NodeNoImplementation:
		return base + " [no implementation]"
	case NodeAmbiguousType:
		return base + " [ambiguous type]"
	case NodeAmbiguousOverload:
		return base + " [ambiguous overload]"
	case NodeAmbiguousImplementation:
		return fmt.Sprintf("%s [ambiguous: %d implementations]", base, candidateCount)
	default:
		return base
	}
}

func compareEdges(a, b graph.Edge) int {
	if cmp := compareHandles(a.From, b.From); cmp != 0 {
		return cmp
	}
	if cmp := compareHandles(a.To, b.To); cmp != 0 {
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
	if a.Cycle == b.Cycle {
		return 0
	}
	if !a.Cycle {
		return -1
	}
	return 1
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
