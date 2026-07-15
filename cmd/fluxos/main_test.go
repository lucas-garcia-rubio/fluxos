package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var errWrite = errors.New("write failed")

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errWrite
}

func traceFixtureRoot() string {
	return filepath.Join("..", "..", "testdata", "trace")
}

func TestRunTraceMermaidGolden(t *testing.T) {
	root := traceFixtureRoot()
	var out bytes.Buffer

	if err := runTrace([]string{"Workflow.start", root}, &out); err != nil {
		t.Fatalf("runTrace: %v", err)
	}

	want, err := os.ReadFile(filepath.Join(root, "expected.mmd"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if out.String() != string(want) {
		t.Fatalf("trace output mismatch:\ngot:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestRunTraceArgumentErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing spec", want: "usage"},
		{name: "invalid spec", args: []string{"Workflow"}, want: "expected ClassName.methodName"},
		{name: "missing class", args: []string{"Missing.start", traceFixtureRoot()}, want: "class \"Missing\" not found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runTrace(tt.args, io.Discard)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("runTrace(%v) error = %v, want containing %q", tt.args, err, tt.want)
			}
		})
	}
}

func TestRunTraceWriterError(t *testing.T) {
	err := runTrace([]string{"Workflow.start", traceFixtureRoot()}, failingWriter{})
	if !errors.Is(err, errWrite) {
		t.Fatalf("runTrace writer error = %v, want wrapped %v", err, errWrite)
	}
	if !strings.Contains(err.Error(), "write trace") {
		t.Fatalf("runTrace writer error = %q, want context", err)
	}
}
