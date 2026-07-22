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
	for _, truncation := range snapshot.Truncations {
		fmt.Fprintf(&payload, "  %s [shape=note, label=%s];\n",
			quote(truncation.ID), quote(truncationLabel(truncation)))
	}
	payload.WriteString("}\n")
	return writeAll(out, payload.String())
}

func truncationLabel(truncation render.TruncationView) string {
	return fmt.Sprintf("truncation: %s; omitted %d while tracing %s",
		truncationKindLabel(truncation.Kind), truncation.Omitted, executionLabel(truncation.Caller))
}

func truncationKindLabel(kind string) string {
	switch kind {
	case "maxDepth":
		return "depth limit"
	case "maxNodes":
		return "node limit"
	case "maxImpls":
		return "implementation limit"
	case "discoveryLimit":
		return "discovery limit"
	default:
		return kind + " limit"
	}
}

func executionLabel(execution render.ExecutionView) string {
	method := execution.Method
	typeFQCN := method.TypeFQCN
	if separator := strings.IndexByte(typeFQCN, '#'); separator >= 0 {
		typeFQCN = typeFQCN[:separator]
	}
	if typeFQCN == "" {
		typeFQCN = "<unknown>"
	}
	signature := method.Signature
	if signature == "" {
		signature = "()"
	}
	return typeFQCN + "." + method.Method + signature
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
