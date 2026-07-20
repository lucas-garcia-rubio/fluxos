package json

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/lucas-garcia-rubio/fluxos/internal/render"
)

var errJSONWrite = errors.New("write failed")

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errJSONWrite
}

func sampleSnapshot() render.Snapshot {
	return render.Snapshot{
		SchemaVersion: render.SnapshotSchemaVersion,
		Target: render.ExecutionView{
			Method:          render.MethodView{TypeFQCN: "app.Workflow", Method: "start", Signature: "()"},
			RuntimeTypeFQCN: "app.Workflow",
		},
		Nodes: []render.NodeView{
			{
				ID: "m_root", Execution: render.ExecutionView{
					Method:          render.MethodView{TypeFQCN: "app.Workflow", Method: "start", Signature: "()"},
					RuntimeTypeFQCN: "app.Workflow",
				},
				Kind:       render.NodeMethod,
				Label:      "app.Workflow.start()",
				Candidates: []string{},
			},
		},
		Edges: []render.EdgeView{
			{
				From: "m_root", To: "m_target",
				Call: render.CallView{Kind: "invocation", File: "App.java", Line: 10, StartByte: 100, MethodName: "run"},
			},
		},
		Truncations: []render.TruncationView{
			{
				ID: "t_omit", Kind: "maxNodes",
				Caller: render.ExecutionView{
					Method:          render.MethodView{TypeFQCN: "app.Workflow", Method: "start", Signature: "()"},
					RuntimeTypeFQCN: "app.Workflow",
				},
				Call:    render.CallView{Kind: "invocation", File: "App.java", StartByte: 100, MethodName: "run"},
				Omitted: 3,
			},
		},
	}
}

func renderString(t *testing.T, snapshot render.Snapshot) string {
	t.Helper()
	var out bytes.Buffer
	if err := Render(&out, snapshot); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return out.String()
}

func TestRenderProducesSchemaVersionOne(t *testing.T) {
	got := renderString(t, sampleSnapshot())
	if !strings.Contains(got, "\"schemaVersion\": 1") {
		t.Fatalf("schemaVersion missing:\n%s", got)
	}
}

func TestRenderEmitsNonNilArraysForEmptySnapshot(t *testing.T) {
	snapshot := render.Snapshot{
		SchemaVersion: render.SnapshotSchemaVersion,
		Target: render.ExecutionView{
			Method:          render.MethodView{TypeFQCN: "T", Method: "t", Signature: "()"},
			RuntimeTypeFQCN: "T",
		},
	}
	got := renderString(t, snapshot)
	if !strings.Contains(got, "\"nodes\": []") {
		t.Fatalf("nodes should be empty array: %s", got)
	}
	if !strings.Contains(got, "\"edges\": []") {
		t.Fatalf("edges should be empty array: %s", got)
	}
	if !strings.Contains(got, "\"truncations\": []") {
		t.Fatalf("truncations should be empty array: %s", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("output should end with newline: %q", got)
	}
}

func TestRenderRoundTripsStructurally(t *testing.T) {
	snapshot := sampleSnapshot()
	out := renderString(t, snapshot)

	var decoded snapshotJSON
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("Unmarshal: %v\noutput:\n%s", err, out)
	}
	if decoded.SchemaVersion != 1 {
		t.Fatalf("decoded schemaVersion = %d, want 1", decoded.SchemaVersion)
	}
	if len(decoded.Nodes) != 1 || decoded.Nodes[0].ID != "m_root" {
		t.Fatalf("decoded nodes = %+v", decoded.Nodes)
	}
	if len(decoded.Edges) != 1 || decoded.Edges[0].Call.MethodName != "run" {
		t.Fatalf("decoded edges = %+v", decoded.Edges)
	}
	if decoded.Edges[0].DispatchSite != nil {
		t.Fatalf("dispatchSite should be null when absent: %+v", decoded.Edges[0].DispatchSite)
	}
	if len(decoded.Truncations) != 1 || decoded.Truncations[0].Omitted != 3 {
		t.Fatalf("decoded truncations = %+v", decoded.Truncations)
	}
}

func TestRenderDispatchSiteObjectWhenPresent(t *testing.T) {
	caller := render.ExecutionView{
		Method:          render.MethodView{TypeFQCN: "Caller", Method: "run", Signature: "()"},
		RuntimeTypeFQCN: "Caller",
	}
	snapshot := render.Snapshot{
		SchemaVersion: render.SnapshotSchemaVersion,
		Target:        caller,
		Nodes:         []render.NodeView{},
		Edges: []render.EdgeView{{
			From: "m_caller", To: "m_target",
			Call:         render.CallView{Kind: "invocation", File: "Caller.java", StartByte: 50, MethodName: "work"},
			DispatchSite: &render.DispatchSiteView{
				ID: "ds_abc", Caller: caller, ReceiverFQCN: "Contract", Method: "work", Signature: "()",
				Call:       render.CallView{Kind: "invocation", File: "Caller.java", StartByte: 50, MethodName: "work"},
				Candidates: []render.ImplementationCandidateView{{ImplementationFQCN: "a.Impl", Kind: "concrete", Target: render.MethodView{TypeFQCN: "a.Impl", Method: "work", Signature: "()"}}},
			},
		}},
	}
	var decoded snapshotJSON
	if err := json.Unmarshal([]byte(renderString(t, snapshot)), &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	site := decoded.Edges[0].DispatchSite
	if site == nil {
		t.Fatal("dispatchSite should be an object when present")
	}
	if site.ID != "ds_abc" || site.ReceiverFQCN != "Contract" {
		t.Fatalf("dispatchSite decoded wrong: %+v", site)
	}
	if len(site.Candidates) != 1 || site.Candidates[0].ImplementationFQCN != "a.Impl" {
		t.Fatalf("candidates decoded wrong: %+v", site.Candidates)
	}
}

func TestRenderPreservesOrder(t *testing.T) {
	snapshot := render.Snapshot{
		SchemaVersion: render.SnapshotSchemaVersion,
		Target: render.ExecutionView{
			Method: render.MethodView{TypeFQCN: "T", Method: "t", Signature: "()"},
		},
		Nodes: []render.NodeView{
			{ID: "m_b", Execution: render.ExecutionView{Method: render.MethodView{TypeFQCN: "B"}}},
			{ID: "m_a", Execution: render.ExecutionView{Method: render.MethodView{TypeFQCN: "A"}}},
		},
	}
	got := renderString(t, snapshot)
	if strings.Index(got, "m_b") > strings.Index(got, "m_a") {
		t.Fatalf("order not preserved:\n%s", got)
	}
}

func TestRenderCandidatesAlwaysArray(t *testing.T) {
	snapshot := render.Snapshot{
		SchemaVersion: render.SnapshotSchemaVersion,
		Target: render.ExecutionView{
			Method: render.MethodView{TypeFQCN: "T", Method: "t", Signature: "()"},
		},
		Nodes: []render.NodeView{{
			ID:         "m_x",
			Execution:  render.ExecutionView{Method: render.MethodView{TypeFQCN: "X"}},
			Candidates: nil,
		}},
	}
	var decoded snapshotJSON
	if err := json.Unmarshal([]byte(renderString(t, snapshot)), &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.Nodes[0].Candidates == nil || !reflect.DeepEqual(decoded.Nodes[0].Candidates, []string{}) {
		t.Fatalf("nil candidates must serialize as empty array: %+v", decoded.Nodes[0].Candidates)
	}
}

func TestRenderPropagatesWriterErrors(t *testing.T) {
	if err := Render(failingWriter{}, render.Snapshot{}); !errors.Is(err, errJSONWrite) {
		t.Fatalf("Render writer error = %v", err)
	}
}

func TestRenderDoesNotEscapeHTMLEntities(t *testing.T) {
	snapshot := render.Snapshot{
		SchemaVersion: render.SnapshotSchemaVersion,
		Target: render.ExecutionView{
			Method: render.MethodView{TypeFQCN: "T", Method: "t", Signature: "()"},
		},
		Nodes: []render.NodeView{{
			ID: "m_x", Execution: render.ExecutionView{Method: render.MethodView{TypeFQCN: "X"}},
			Label: `<html> & "stuff"`,
		}},
	}
	got := renderString(t, snapshot)
	if !strings.Contains(got, `<html> & \"stuff\"`) {
		t.Fatalf("HTML entities should not be escaped beyond JSON quote escaping:\n%s", got)
	}
}
