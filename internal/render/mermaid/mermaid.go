// Package mermaid renders deterministic Mermaid flowcharts from render snapshots.
package mermaid

import (
	"fmt"
	"io"
	"strings"

	"github.com/lucas-garcia-rubio/fluxos/internal/render"
)

type Direction string

const (
	DirectionTD Direction = "TD"
	DirectionLR Direction = "LR"
	DirectionBT Direction = "BT"
	DirectionRL Direction = "RL"
)

func Render(out io.Writer, snapshot render.Snapshot, direction Direction) error {
	if !direction.valid() {
		return fmt.Errorf("invalid Mermaid direction %q", direction)
	}

	var payload strings.Builder
	fmt.Fprintf(&payload, "flowchart %s\n", direction)
	for _, node := range snapshot.Nodes {
		fmt.Fprintf(&payload, "  %s[\"%s\"]\n", node.ID, escapeLabel(node.Label))
	}
	for _, edge := range snapshot.Edges {
		if edge.Cycle {
			payload.WriteString("  %% cycle\n")
		}
		fmt.Fprintf(&payload, "  %s --> %s\n", edge.From, edge.To)
	}
	return writeAll(out, payload.String())
}

func (d Direction) valid() bool {
	switch d {
	case DirectionTD, DirectionLR, DirectionBT, DirectionRL:
		return true
	default:
		return false
	}
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

func escapeLabel(label string) string {
	return strings.ReplaceAll(label, `"`, "#quot;")
}
