package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/lucas-garcia-rubio/fluxos/internal/project"
)

func requireUsageError(t *testing.T, err error) {
	t.Helper()
	var usageErr *UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("error = %v, want UsageError", err)
	}
}

func TestParseTraceOptionsDefaults(t *testing.T) {
	got, err := parseTraceOptions([]string{"app.Workflow.start"})
	if err != nil {
		t.Fatalf("parseTraceOptions: %v", err)
	}
	want := TraceOptions{
		Target:            TargetSpec{TypeName: "app.Workflow", Method: "start"},
		ProjectRoot:       ".",
		Format:            FormatMermaid,
		Direction:         "TD",
		Scope:             project.ScopeModeMain,
		IncludeUnresolved: true,
		MaxImpls:          5,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("options = %+v, want %+v", got, want)
	}
}

func TestParseIndexOptionsDefaults(t *testing.T) {
	got, err := parseIndexOptions([]string{"./project"})
	if err != nil {
		t.Fatalf("parseIndexOptions: %v", err)
	}
	want := IndexOptions{ProjectRoot: "./project", Scope: project.ScopeModeMain}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("options = %+v, want %+v", got, want)
	}
}

func TestParseTraceOptionsAcceptsFlagsAroundPositionals(t *testing.T) {
	tests := [][]string{
		{"--format=mermaid", "app.Workflow.start", "./project"},
		{"app.Workflow.start", "--format", "mermaid", "./project"},
		{"app.Workflow.start", "./project", "--format=mermaid"},
	}
	for _, args := range tests {
		got, err := parseTraceOptions(args)
		if err != nil {
			t.Fatalf("parseTraceOptions(%v): %v", args, err)
		}
		if got.ProjectRoot != "./project" || got.Format != FormatMermaid {
			t.Fatalf("parseTraceOptions(%v) = %+v", args, got)
		}
	}
}

func TestParseTraceOptionsAcceptsEqualsAndSeparateValues(t *testing.T) {
	got, err := parseTraceOptions([]string{
		"--format=mermaid",
		"--direction", "TD",
		"--scope=main",
		"--include-unresolved=true",
		"--max-depth", "0",
		"--max-nodes=0",
		"--max-impls", "5",
		"app.Workflow.start",
	})
	if err != nil {
		t.Fatalf("parseTraceOptions: %v", err)
	}
	if !got.DirectionSet || got.Direction != "TD" || got.MaxImpls != 5 || !got.IncludeUnresolved {
		t.Fatalf("options = %+v", got)
	}
}

func TestParseTraceOptionsParsesBooleanForms(t *testing.T) {
	got, err := parseTraceOptions([]string{
		"--all-impls=false",
		"--no-prompt=false",
		"--include-unresolved",
		"app.Workflow.start",
	})
	if err != nil {
		t.Fatalf("parseTraceOptions: %v", err)
	}
	if got.AllImpls || got.NoPrompt || !got.IncludeUnresolved {
		t.Fatalf("options = %+v", got)
	}

	got, err = parseTraceOptions([]string{"--all-impls", "app.Workflow.start"})
	if err != nil {
		t.Fatalf("parse bare boolean: %v", err)
	}
	if !got.AllImpls {
		t.Fatal("bare --all-impls did not set true")
	}
}

func TestParseTraceOptionsDoubleDashEndsFlags(t *testing.T) {
	got, err := parseTraceOptions([]string{"app.Workflow.start", "--", "-project"})
	if err != nil {
		t.Fatalf("parseTraceOptions: %v", err)
	}
	if got.ProjectRoot != "-project" {
		t.Fatalf("project root = %q, want -project", got.ProjectRoot)
	}
}

func TestParseTraceOptionsRejectsInvalidFlagsAndValues(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "unknown", args: []string{"--unknown", "app.Workflow.start"}, want: "unknown flag"},
		{name: "short", args: []string{"-f", "app.Workflow.start"}, want: "unknown short flag"},
		{name: "duplicate", args: []string{"--format=mermaid", "--format", "mermaid", "app.Workflow.start"}, want: "duplicate flag"},
		{name: "missing value", args: []string{"--format", "--scope=main", "app.Workflow.start"}, want: "requires a value"},
		{name: "empty value", args: []string{"--format=", "app.Workflow.start"}, want: "non-empty value"},
		{name: "empty boolean", args: []string{"--all-impls=", "app.Workflow.start"}, want: "boolean value"},
		{name: "invalid format", args: []string{"--format=yaml", "app.Workflow.start"}, want: "invalid --format"},
		{name: "invalid direction", args: []string{"--direction=down", "app.Workflow.start"}, want: "invalid --direction"},
		{name: "invalid scope", args: []string{"--scope=tests", "app.Workflow.start"}, want: "invalid --scope"},
		{name: "invalid boolean", args: []string{"--no-prompt=yes", "app.Workflow.start"}, want: "invalid --no-prompt"},
		{name: "invalid integer", args: []string{"--max-depth=many", "app.Workflow.start"}, want: "non-negative integer"},
		{name: "negative integer", args: []string{"--max-depth", "-1", "app.Workflow.start"}, want: "must not be negative"},
		{name: "missing target", args: nil, want: "usage"},
		{name: "invalid target", args: []string{"Workflow"}, want: "expected TypeName.method"},
		{name: "extra positional", args: []string{"app.Workflow.start", "one", "two"}, want: "at most one project path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseTraceOptions(tt.args)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("parseTraceOptions(%v) error = %v, want containing %q", tt.args, err, tt.want)
			}
			requireUsageError(t, err)
		})
	}
}

func TestParseIndexOptionsRejectsInvalidArguments(t *testing.T) {
	tests := [][]string{
		nil,
		{"one", "two"},
		{"--format=json", "one"},
		{"--scope=invalid", "one"},
	}
	for _, args := range tests {
		if _, err := parseIndexOptions(args); err == nil {
			t.Fatalf("parseIndexOptions(%v) succeeded", args)
		} else {
			requireUsageError(t, err)
		}
	}
}

func TestParseTraceOptionsRejectsConflictsBeforeSupportGate(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{args: []string{"--all-impls", "--pick-impls=x.Y", "app.Workflow.start"}, want: "cannot be combined"},
		{args: []string{"--format=dot", "--direction=TD", "app.Workflow.start"}, want: "--direction cannot be combined"},
	}
	for _, tt := range tests {
		_, err := parseTraceOptions(tt.args)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Fatalf("parseTraceOptions(%v) error = %v, want containing %q", tt.args, err, tt.want)
		}
		requireUsageError(t, err)
	}
}

func TestValidateTraceSupportAcceptsCompatibilityValues(t *testing.T) {
	opts, err := parseTraceOptions([]string{
		"--format=mermaid", "--direction=TD", "--scope=main",
		"--all-impls=false", "--no-prompt=false", "--include-unresolved=true",
		"--max-depth=0", "--max-nodes=0", "--max-impls=5",
		"app.Workflow.start",
	})
	if err != nil {
		t.Fatalf("parseTraceOptions: %v", err)
	}
	if err := validateTraceSupport(opts); err != nil {
		t.Fatalf("validateTraceSupport: %v", err)
	}
}

func TestValidateTraceSupportRejectsReservedFeatures(t *testing.T) {
	tests := [][]string{
		{"--format=json", "app.Workflow.start"},
		{"--pick-impls=x.Y", "app.Workflow.start"},
		{"--all-impls", "app.Workflow.start"},
		{"--no-prompt", "app.Workflow.start"},
		{"--include-unresolved=false", "app.Workflow.start"},
		{"--max-depth=1", "app.Workflow.start"},
		{"--max-nodes=1", "app.Workflow.start"},
		{"--max-impls=4", "app.Workflow.start"},
	}
	for _, args := range tests {
		opts, err := parseTraceOptions(args)
		if err != nil {
			t.Fatalf("parseTraceOptions(%v): %v", args, err)
		}
		err = validateTraceSupport(opts)
		if err == nil || !strings.Contains(err.Error(), "not implemented yet") {
			t.Fatalf("validateTraceSupport(%v) error = %v", args, err)
		}
		requireUsageError(t, err)
	}
}

func TestValidateTraceSupportAcceptsScopeAll(t *testing.T) {
	opts, err := parseTraceOptions([]string{"--scope=all", "app.Workflow.start"})
	if err != nil {
		t.Fatalf("parseTraceOptions: %v", err)
	}
	if err := validateTraceSupport(opts); err != nil {
		t.Fatalf("validateTraceSupport: %v", err)
	}
}

func TestValidateTraceSupportAcceptsDOTAndDirections(t *testing.T) {
	tests := [][]string{
		{"--format=dot", "app.Workflow.start"},
		{"--direction=LR", "app.Workflow.start"},
		{"--direction=BT", "app.Workflow.start"},
		{"--direction=RL", "app.Workflow.start"},
	}
	for _, args := range tests {
		opts, err := parseTraceOptions(args)
		if err != nil {
			t.Fatalf("parseTraceOptions(%v): %v", args, err)
		}
		if err := validateTraceSupport(opts); err != nil {
			t.Fatalf("validateTraceSupport(%v): %v", args, err)
		}
	}
}

func TestValidateIndexSupportAcceptsScopeAll(t *testing.T) {
	opts, err := parseIndexOptions([]string{"--scope=all", "./project"})
	if err != nil {
		t.Fatalf("parseIndexOptions: %v", err)
	}
	if err := validateIndexSupport(opts); err != nil {
		t.Fatalf("validateIndexSupport: %v", err)
	}
}

func TestValidateIndexSupportAcceptsScopeMain(t *testing.T) {
	opts, err := parseIndexOptions([]string{"--scope=main", "./project"})
	if err != nil {
		t.Fatalf("parseIndexOptions: %v", err)
	}
	if err := validateIndexSupport(opts); err != nil {
		t.Fatalf("validateIndexSupport: %v", err)
	}
}
