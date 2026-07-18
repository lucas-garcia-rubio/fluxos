// Package mermaid renders call graphs as deterministic Mermaid flowcharts.
package mermaid

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"

	"github.com/lucas-garcia-rubio/fluxos/internal/graph"
	"github.com/lucas-garcia-rubio/fluxos/internal/resolve"
)

// Render returns a complete Mermaid flowchart for g.
func Render(g *graph.Graph) string {
	var out strings.Builder
	out.WriteString("flowchart TD\n")
	if g == nil {
		return out.String()
	}

	for _, node := range sortedNodes(g) {
		fmt.Fprintf(&out, "  %s[\"%s\"]\n", nodeID(node.Handle), nodeLabel(node))
	}
	for _, edge := range sortedEdges(g) {
		if edge.Cycle {
			out.WriteString("  %% cycle\n")
		}
		fmt.Fprintf(&out, "  %s --> %s\n", nodeID(edge.From), nodeID(edge.To))
	}
	return out.String()
}

func nodeID(handle resolve.MethodHandle) string {
	sum := sha256.Sum256([]byte(handle.TypeFQCN + "\x00" + handle.Method + "\x00" + handle.Signature))
	return fmt.Sprintf("m_%x", sum[:6])
}

// nodeLabel derives the label from the node kind. Concrete and external nodes
// share the legacy "FQCN.method(signature)" format; terminal nodes append a
// deterministic suffix. Candidates on AmbiguousImplementation nodes surface as
// the count so two terminals of the same name with different fan-out stay
// visually distinct.
func nodeLabel(node *graph.Node) string {
	base := labelBase(node.Handle)
	switch node.Kind {
	case graph.NodeTerminalUnresolved:
		return base + " [unresolved]"
	case graph.NodeTerminalNoImplementation:
		return base + " [no implementation]"
	case graph.NodeTerminalAmbiguousType:
		return base + " [ambiguous type]"
	case graph.NodeTerminalAmbiguousOverload:
		return base + " [ambiguous overload]"
	case graph.NodeTerminalAmbiguousImplementation:
		return fmt.Sprintf("%s [ambiguous: %d implementations]", base, len(node.Candidates))
	default:
		return base
	}
}

// labelBase trims the "#<kind>#<hash>" suffix that TerminalHandle adds to
// TypeFQCN so the rendered label shows the original receiver. IDs still hash
// the full TypeFQCN, so two terminals with the same receiver/method stay
// distinct in the diagram even though their label prefixes match. Terminal
// handles carry an empty signature (the overload was not resolved); we render
// "()" so the label reads as a call site.
func labelBase(handle resolve.MethodHandle) string {
	fqcn := handle.TypeFQCN
	if i := strings.IndexByte(fqcn, '#'); i >= 0 {
		fqcn = fqcn[:i]
	}
	if fqcn == "" {
		fqcn = "<unknown>"
	}
	signature := handle.Signature
	if signature == "" {
		signature = "()"
	}
	return escapeLabel(fqcn + "." + handle.Method + signature)
}

func escapeLabel(label string) string {
	return strings.ReplaceAll(label, `"`, "#quot;")
}

func sortedNodes(g *graph.Graph) []*graph.Node {
	nodes := make([]*graph.Node, 0, len(g.Nodes))
	for _, node := range g.Nodes {
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(i, j int) bool {
		return compareHandles(nodes[i].Handle, nodes[j].Handle) < 0
	})
	return nodes
}

func sortedEdges(g *graph.Graph) []graph.Edge {
	edges := append([]graph.Edge(nil), g.Edges...)
	sort.SliceStable(edges, func(i, j int) bool {
		a, b := edges[i], edges[j]
		if cmp := compareHandles(a.From, b.From); cmp != 0 {
			return cmp < 0
		}
		if cmp := compareHandles(a.To, b.To); cmp != 0 {
			return cmp < 0
		}
		if a.Call.File != b.Call.File {
			return a.Call.File < b.Call.File
		}
		if a.Call.Line != b.Call.Line {
			return a.Call.Line < b.Call.Line
		}
		if a.Call.StartByte != b.Call.StartByte {
			return a.Call.StartByte < b.Call.StartByte
		}
		if a.Call.Kind != b.Call.Kind {
			return a.Call.Kind < b.Call.Kind
		}
		if a.Call.Receiver != b.Call.Receiver {
			return a.Call.Receiver < b.Call.Receiver
		}
		if a.Call.MethodName != b.Call.MethodName {
			return a.Call.MethodName < b.Call.MethodName
		}
		return !a.Cycle && b.Cycle
	})
	return edges
}

func compareHandles(a, b resolve.MethodHandle) int {
	if a.TypeFQCN < b.TypeFQCN {
		return -1
	}
	if a.TypeFQCN > b.TypeFQCN {
		return 1
	}
	if a.Method < b.Method {
		return -1
	}
	if a.Method > b.Method {
		return 1
	}
	if a.Signature < b.Signature {
		return -1
	}
	if a.Signature > b.Signature {
		return 1
	}
	return 0
}
