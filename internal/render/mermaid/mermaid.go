// Package mermaid renders deterministic Mermaid flowcharts from render snapshots.
package mermaid

import (
	"fmt"
	"strings"

	"github.com/lucas-garcia-rubio/fluxos/internal/render"
)

func RenderSnapshot(snapshot render.Snapshot) string {
	var out strings.Builder
	out.WriteString("flowchart TD\n")
	for _, node := range snapshot.Nodes {
		fmt.Fprintf(&out, "  %s[\"%s\"]\n", node.ID, escapeLabel(node.Label))
	}
	for _, edge := range snapshot.Edges {
		if edge.Cycle {
			out.WriteString("  %% cycle\n")
		}
		fmt.Fprintf(&out, "  %s --> %s\n", edge.From, edge.To)
	}
	return out.String()
}

func escapeLabel(label string) string {
	return strings.ReplaceAll(label, `"`, "#quot;")
}
