// Package json serializes render.Snapshot into the schema v1 JSON output.
// Arrays are always non-nil; optionals that semantically absent turn into
// empty string, zero, empty array, or null (dispatchSite only).
package json

import (
	"encoding/json"
	"io"

	"github.com/lucas-garcia-rubio/fluxos/internal/render"
)

// Render escreve o snapshot como JSON schema v1 com indentação de dois
// espaços e newline final.
func Render(out io.Writer, snapshot render.Snapshot) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(toJSON(snapshot)); err != nil {
		return err
	}
	return nil
}

type snapshotJSON struct {
	SchemaVersion int             `json:"schemaVersion"`
	Target        executionJSON   `json:"target"`
	Nodes         []nodeJSON      `json:"nodes"`
	Edges         []edgeJSON      `json:"edges"`
	Truncations   []truncationJSON `json:"truncations"`
}

type methodJSON struct {
	Type      string `json:"type"`
	Method    string `json:"method"`
	Signature string `json:"signature"`
}

type executionJSON struct {
	Method      methodJSON `json:"method"`
	RuntimeType string     `json:"runtimeType"`
}

type callJSON struct {
	Kind       string `json:"kind"`
	File       string `json:"file"`
	Line       int    `json:"line"`
	StartByte  uint   `json:"startByte"`
	Receiver   string `json:"receiver"`
	MethodName string `json:"methodName"`
}

type candidateJSON struct {
	ImplementationFQCN string     `json:"implementationFQCN"`
	Target             methodJSON `json:"target"`
	Kind               string     `json:"kind"`
	Note               string     `json:"note"`
}

type dispatchSiteJSON struct {
	ID           string         `json:"id"`
	Caller       executionJSON  `json:"caller"`
	ReceiverFQCN string         `json:"receiverFQCN"`
	Method       string         `json:"method"`
	Signature    string         `json:"signature"`
	Call         callJSON       `json:"call"`
	Candidates   []candidateJSON `json:"candidates"`
}

type nodeJSON struct {
	ID         string        `json:"id"`
	Execution  executionJSON `json:"execution"`
	Kind       string        `json:"kind"`
	Label      string        `json:"label"`
	Note       string        `json:"note"`
	Candidates []string      `json:"candidates"`
}

type edgeJSON struct {
	From         string            `json:"from"`
	To           string            `json:"to"`
	Call         callJSON          `json:"call"`
	DispatchSite *dispatchSiteJSON `json:"dispatchSite"`
	Cycle        bool              `json:"cycle"`
}

type truncationJSON struct {
	ID      string        `json:"id"`
	Kind    string        `json:"kind"`
	Caller  executionJSON `json:"caller"`
	Call    callJSON      `json:"call"`
	Omitted int           `json:"omitted"`
	Note    string        `json:"note"`
}

func toJSON(snapshot render.Snapshot) snapshotJSON {
	return snapshotJSON{
		SchemaVersion: snapshot.SchemaVersion,
		Target:        executionToJSON(snapshot.Target),
		Nodes:         nodesToJSON(snapshot.Nodes),
		Edges:         edgesToJSON(snapshot.Edges),
		Truncations:   truncationsToJSON(snapshot.Truncations),
	}
}

func executionToJSON(execution render.ExecutionView) executionJSON {
	return executionJSON{
		Method:      methodToJSON(execution.Method),
		RuntimeType: execution.RuntimeTypeFQCN,
	}
}

func methodToJSON(method render.MethodView) methodJSON {
	return methodJSON{
		Type:      method.TypeFQCN,
		Method:    method.Method,
		Signature: method.Signature,
	}
}

func callToJSON(call render.CallView) callJSON {
	return callJSON{
		Kind:       call.Kind,
		File:       call.File,
		Line:       call.Line,
		StartByte:  call.StartByte,
		Receiver:   call.Receiver,
		MethodName: call.MethodName,
	}
}

func nodesToJSON(nodes []render.NodeView) []nodeJSON {
	out := make([]nodeJSON, 0, len(nodes))
	for _, node := range nodes {
		candidates := node.Candidates
		if candidates == nil {
			candidates = []string{}
		}
		out = append(out, nodeJSON{
			ID:         node.ID,
			Execution:  executionToJSON(node.Execution),
			Kind:       string(node.Kind),
			Label:      node.Label,
			Note:       node.Note,
			Candidates: candidates,
		})
	}
	return out
}

func edgesToJSON(edges []render.EdgeView) []edgeJSON {
	out := make([]edgeJSON, 0, len(edges))
	for _, edge := range edges {
		out = append(out, edgeJSON{
			From:         edge.From,
			To:           edge.To,
			Call:         callToJSON(edge.Call),
			DispatchSite: dispatchSiteToJSON(edge.DispatchSite),
			Cycle:        edge.Cycle,
		})
	}
	return out
}

func dispatchSiteToJSON(site *render.DispatchSiteView) *dispatchSiteJSON {
	if site == nil {
		return nil
	}
	candidates := make([]candidateJSON, 0, len(site.Candidates))
	for _, candidate := range site.Candidates {
		candidates = append(candidates, candidateJSON{
			ImplementationFQCN: candidate.ImplementationFQCN,
			Target:             methodToJSON(candidate.Target),
			Kind:               candidate.Kind,
			Note:               candidate.Note,
		})
	}
	return &dispatchSiteJSON{
		ID:           site.ID,
		Caller:       executionToJSON(site.Caller),
		ReceiverFQCN: site.ReceiverFQCN,
		Method:       site.Method,
		Signature:    site.Signature,
		Call:         callToJSON(site.Call),
		Candidates:   candidates,
	}
}

func truncationsToJSON(truncations []render.TruncationView) []truncationJSON {
	out := make([]truncationJSON, 0, len(truncations))
	for _, truncation := range truncations {
		out = append(out, truncationJSON{
			ID:      truncation.ID,
			Kind:    truncation.Kind,
			Caller:  executionToJSON(truncation.Caller),
			Call:    callToJSON(truncation.Call),
			Omitted: truncation.Omitted,
			Note:    truncation.Note,
		})
	}
	return out
}
