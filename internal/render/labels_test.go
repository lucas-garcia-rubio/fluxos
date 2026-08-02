package render

import "testing"

func TestDisplayTypeName(t *testing.T) {
	tests := map[string]string{
		"":                        "",
		"Workflow":                "Workflow",
		"com.example.Workflow":    "Workflow",
		"refs.References.Nested":  "References.Nested",
		"Contract#ambimpl#abc123": "Contract",
		"unknown.value":           "unknown.value",
	}
	for input, want := range tests {
		if got := DisplayTypeName(input); got != want {
			t.Errorf("DisplayTypeName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestDiagramNodeLabelShortensRuntimeAndRetainsTerminalSuffix(t *testing.T) {
	node := NodeView{
		Label: "com.example.Workflow.start() [ambiguous overload] [runtime: app.First]",
		Execution: ExecutionView{
			Method:          MethodView{TypeFQCN: "com.example.Workflow"},
			RuntimeTypeFQCN: "app.First",
		},
	}
	if got, want := DiagramNodeLabel(node, false), "Workflow.start() [ambiguous overload] [runtime: First]"; got != want {
		t.Fatalf("short diagram label = %q, want %q", got, want)
	}
	if got, want := DiagramNodeLabel(node, true), node.Label; got != want {
		t.Fatalf("FQCN diagram label = %q, want %q", got, want)
	}
}

func TestDiagramNodeLabelFallsBackToCanonicalLabel(t *testing.T) {
	node := NodeView{Label: "com.example.Workflow.start()"}
	if got, want := DiagramNodeLabel(node, false), "Workflow.start()"; got != want {
		t.Fatalf("fallback diagram label = %q, want %q", got, want)
	}
}

func TestDiagramExecutionLabelHandlesUnknownAndSyntheticTypes(t *testing.T) {
	unknown := ExecutionView{Method: MethodView{Method: "run"}}
	if got, want := DiagramExecutionLabel(unknown, false), "<unknown>.run()"; got != want {
		t.Fatalf("unknown execution label = %q, want %q", got, want)
	}
	synthetic := ExecutionView{Method: MethodView{TypeFQCN: "Contract#ambimpl#abc123", Method: "run"}}
	if got, want := DiagramExecutionLabel(synthetic, false), "Contract.run()"; got != want {
		t.Fatalf("synthetic execution label = %q, want %q", got, want)
	}
}
