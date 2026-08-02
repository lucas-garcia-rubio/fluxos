// Package dot renders Graphviz DOT from render snapshots.
package dot

import (
	"fmt"
	"io"
	"strings"

	"github.com/lucas-garcia-rubio/fluxos/internal/render"
)

func Render(out io.Writer, snapshot render.Snapshot, showFQCN, showFQCNParams bool) error {
	var payload strings.Builder
	payload.WriteString("digraph fluxos {\n")
	for _, node := range snapshot.Nodes {
		fmt.Fprintf(&payload, "  %s [label=%s, kind=%s];\n", quote(node.ID), quote(render.DiagramNodeLabel(node, showFQCN, showFQCNParams)), quote(string(node.Kind)))
	}
	for _, edge := range snapshot.Edges {
		fmt.Fprintf(&payload, "  %s -> %s", quote(edge.From), quote(edge.To))
		if edge.Cycle {
			payload.WriteString(" [color=\"red\", style=\"dashed\"]")
		}
		payload.WriteString(";\n")
	}
	for _, truncation := range snapshot.Truncations {
		fmt.Fprintf(&payload, "  %s [shape=note, label=%s];\n",
			quote(truncation.ID), quote(render.DiagramTruncationLabel(truncation, showFQCN, showFQCNParams)))
	}
	payload.WriteString("}\n")
	return writeAll(out, payload.String())
}

func escapeValue(value string) string {
	return strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"\n", `\n`,
		"\r", `\r`,
	).Replace(value)
}

func quote(value string) string {
	return `"` + escapeValue(value) + `"`
}

func writeAll(out io.Writer, payload string) error {
	n, err := io.WriteString(out, payload)
	if err != nil {
		return err
	}
	if n != len(payload) {
		return io.ErrShortWrite
	}
	return nil
}
