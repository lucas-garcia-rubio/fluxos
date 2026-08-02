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
	if got, want := DiagramNodeLabel(node, false, false), "Workflow.start() [ambiguous overload] [runtime: First]"; got != want {
		t.Fatalf("short diagram label = %q, want %q", got, want)
	}
	if got, want := DiagramNodeLabel(node, true, true), node.Label; got != want {
		t.Fatalf("FQCN diagram label = %q, want %q", got, want)
	}
}

func TestDiagramNodeLabelFallsBackToCanonicalLabel(t *testing.T) {
	node := NodeView{Label: "com.example.Workflow.start()"}
	if got, want := DiagramNodeLabel(node, false, false), "Workflow.start()"; got != want {
		t.Fatalf("fallback diagram label = %q, want %q", got, want)
	}
}

func TestDiagramExecutionLabelHandlesUnknownAndSyntheticTypes(t *testing.T) {
	unknown := ExecutionView{Method: MethodView{Method: "run"}}
	if got, want := DiagramExecutionLabel(unknown, false, false), "<unknown>.run()"; got != want {
		t.Fatalf("unknown execution label = %q, want %q", got, want)
	}
	synthetic := ExecutionView{Method: MethodView{TypeFQCN: "Contract#ambimpl#abc123", Method: "run"}}
	if got, want := DiagramExecutionLabel(synthetic, false, false), "Contract.run()"; got != want {
		t.Fatalf("synthetic execution label = %q, want %q", got, want)
	}
}

func TestDiagramLabelsFormatOwnerRuntimeAndParametersIndependently(t *testing.T) {
	method := MethodView{
		TypeFQCN:  "app.Workflow",
		Method:    "start",
		Signature: "(app.First,java.util.Map,int,int[],Thing,refs.Outer.Inner)",
	}
	node := NodeView{
		Label: "app.Workflow.start(app.First,java.util.Map,int,int[],Thing,refs.Outer.Inner) [runtime: app.Runtime]",
		Execution: ExecutionView{
			Method:          method,
			RuntimeTypeFQCN: "app.Runtime",
		},
	}

	tests := []struct {
		name           string
		showFQCN       bool
		showFQCNParams bool
		want           string
	}{
		{name: "default", want: "Workflow.start(First,Map,int,int[],Thing,Outer.Inner) [runtime: Runtime]"},
		{name: "owner only", showFQCN: true, want: "app.Workflow.start(First,Map,int,int[],Thing,Outer.Inner) [runtime: app.Runtime]"},
		{name: "parameters only", showFQCNParams: true, want: "Workflow.start(app.First,java.util.Map,int,int[],Thing,refs.Outer.Inner) [runtime: Runtime]"},
		{name: "both", showFQCN: true, showFQCNParams: true, want: node.Label},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DiagramNodeLabel(node, tt.showFQCN, tt.showFQCNParams); got != tt.want {
				t.Fatalf("DiagramNodeLabel = %q, want %q", got, tt.want)
			}
		})
	}

	execution := ExecutionView{Method: method}
	if got, want := DiagramExecutionLabel(execution, false, false), "Workflow.start(First,Map,int,int[],Thing,Outer.Inner)"; got != want {
		t.Fatalf("compact truncation execution label = %q, want %q", got, want)
	}
	if got, want := DiagramExecutionLabel(execution, false, true), "Workflow.start(app.First,java.util.Map,int,int[],Thing,refs.Outer.Inner)"; got != want {
		t.Fatalf("FQCN truncation execution label = %q, want %q", got, want)
	}
}
