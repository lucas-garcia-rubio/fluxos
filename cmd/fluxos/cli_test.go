package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCLIReturnsZeroAndSeparatesStreams(t *testing.T) {
	root := traceFixtureRoot()
	want, err := os.ReadFile(filepath.Join(root, "expected.mmd"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	var out, errOut bytes.Buffer
	code := runCLI([]string{"trace", "Workflow.start", root}, IO{Out: &out, ErrOut: &errOut})
	if code != 0 {
		t.Fatalf("runCLI exit = %d, stderr = %q", code, errOut.String())
	}
	if !bytes.Equal(out.Bytes(), want) {
		t.Fatalf("stdout mismatch:\ngot:\n%s\nwant:\n%s", out.Bytes(), want)
	}
	if errOut.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", errOut.String())
	}
}

func TestRunCLIClassifiesUsageErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing trace target", args: []string{"trace"}, want: "fluxos trace: usage:"},
		{name: "invalid target", args: []string{"trace", "Workflow"}, want: "fluxos trace: invalid target"},
		{name: "extra positional", args: []string{"trace", "Workflow.start", traceFixtureRoot(), "extra"}, want: "at most one project path"},
		{name: "reserved feature", args: []string{"trace", "--format=dot", "Workflow.start", "/missing"}, want: "not implemented yet"},
		{name: "missing index root", args: []string{"index"}, want: "fluxos index: usage:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := runCLI(tt.args, IO{Out: &out, ErrOut: &errOut})
			if code != 2 {
				t.Fatalf("runCLI(%v) exit = %d, want 2; stderr=%q", tt.args, code, errOut.String())
			}
			if out.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", out.String())
			}
			if !strings.Contains(errOut.String(), tt.want) || strings.Count(errOut.String(), "\n") != 1 {
				t.Fatalf("stderr = %q, want one line containing %q", errOut.String(), tt.want)
			}
		})
	}
}

func TestRunCLIClassifiesRuntimeErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "missing project", args: []string{"trace", "Workflow.start", filepath.Join(t.TempDir(), "missing")}},
		{name: "missing class", args: []string{"trace", "Missing.start", traceFixtureRoot()}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := runCLI(tt.args, IO{Out: &out, ErrOut: &errOut})
			if code != 1 {
				t.Fatalf("runCLI(%v) exit = %d, want 1; stderr=%q", tt.args, code, errOut.String())
			}
			if out.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", out.String())
			}
			if !strings.HasPrefix(errOut.String(), "fluxos trace: ") || strings.Count(errOut.String(), "\n") != 1 {
				t.Fatalf("stderr = %q, want one trace diagnostic", errOut.String())
			}
		})
	}
}

func TestRunCLIReportsGlobalUsageBlocks(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing command", want: "usage: fluxos <command>\ncommands: index, trace\n"},
		{name: "unknown command", args: []string{"unknown"}, want: "fluxos: unknown command \"unknown\"\nusage: fluxos <command>\ncommands: index, trace\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if code := runCLI(tt.args, IO{Out: &out, ErrOut: &errOut}); code != 2 {
				t.Fatalf("exit = %d, want 2", code)
			}
			if out.Len() != 0 || errOut.String() != tt.want {
				t.Fatalf("stdout=%q stderr=%q, want empty stdout and stderr %q", out.String(), errOut.String(), tt.want)
			}
		})
	}
}

func TestRunCLIIndexUsesInjectedOutput(t *testing.T) {
	root := traceFixtureRoot()
	units, _, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex: %v", err)
	}
	payload, err := json.MarshalIndent(flattenTypes(units), "", "  ")
	if err != nil {
		t.Fatalf("marshal expected index: %v", err)
	}
	want := append(payload, '\n')

	var out, errOut bytes.Buffer
	code := runCLI([]string{"index", root}, IO{Out: &out, ErrOut: &errOut})
	if code != 0 {
		t.Fatalf("runCLI index exit = %d, stderr = %q", code, errOut.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", errOut.String())
	}
	if !bytes.Equal(out.Bytes(), want) {
		t.Fatalf("index stdout mismatch:\ngot:\n%s\nwant:\n%s", out.Bytes(), want)
	}
}

func TestRunCLIWriterErrorIsRuntimeFailure(t *testing.T) {
	var errOut bytes.Buffer
	code := runCLI([]string{"trace", "Workflow.start", traceFixtureRoot()}, IO{Out: failingWriter{}, ErrOut: &errOut})
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "write trace: write failed") {
		t.Fatalf("stderr = %q, want writer error", errOut.String())
	}
}
