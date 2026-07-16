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

	for _, handle := range sortedHandles(g) {
		fmt.Fprintf(&out, "  %s[\"%s\"]\n", nodeID(handle), nodeLabel(handle))
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

func nodeLabel(handle resolve.MethodHandle) string {
	return escapeLabel(handle.TypeFQCN + "." + handle.Method + handle.Signature)
}

func escapeLabel(label string) string {
	return strings.ReplaceAll(label, `"`, "#quot;")
}

func sortedHandles(g *graph.Graph) []resolve.MethodHandle {
	handles := make([]resolve.MethodHandle, 0, len(g.Nodes))
	for handle := range g.Nodes {
		handles = append(handles, handle)
	}
	sort.Slice(handles, func(i, j int) bool {
		return compareHandles(handles[i], handles[j]) < 0
	})
	return handles
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
