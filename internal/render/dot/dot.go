// Package dot renders Graphviz DOT from render snapshots.
package dot

import (
	"fmt"
	"io"
	"strings"

	"github.com/lucas-garcia-rubio/fluxos/internal/render"
)

func Render(out io.Writer, snapshot render.Snapshot) error {
	var payload strings.Builder
	payload.WriteString("digraph fluxos {\n")
	for _, node := range snapshot.Nodes {
		fmt.Fprintf(&payload, "  %s [label=%s, kind=%s];\n", quote(node.ID), quote(node.Label), quote(string(node.Kind)))
	}
	for _, edge := range snapshot.Edges {
		fmt.Fprintf(&payload, "  %s -> %s", quote(edge.From), quote(edge.To))
		if edge.Cycle {
			payload.WriteString(" [color=\"red\", style=\"dashed\"]")
		}
		payload.WriteString(";\n")
	}
	payload.WriteString("}\n")
	return writeAll(out, payload.String())
}

func quote(value string) string {
	escaped := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"\n", `\n`,
		"\r", `\r`,
	).Replace(value)
	return `"` + escaped + `"`
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
