package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucas-garcia-rubio/fluxos/internal/project"
	"github.com/lucas-garcia-rubio/fluxos/internal/resolve"
	"github.com/lucas-garcia-rubio/fluxos/internal/trace"
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

func TestRunCLIPickImplsIsNonInteractiveAndSeparatesStreams(t *testing.T) {
	var out, errOut bytes.Buffer
	code := runCLI([]string{
		"trace",
		"--pick-impls=contracts.A=app.AlphaA,contracts.B=app.GammaB",
		"app.Workflow.start",
		m4FixtureRoot("interactive"),
	}, IO{Out: &out, ErrOut: &errOut})
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, errOut.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("non-interactive pick wrote stderr: %q", errOut.String())
	}
	if !strings.Contains(out.String(), "app.GammaB.work()") {
		t.Fatalf("stdout does not contain selected graph:\n%s", out.String())
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
	units, _, err := buildIndex(root, project.ScopeModeMain)
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
	for _, args := range [][]string{
		{"trace", "Workflow.start", traceFixtureRoot()},
		{"trace", "--format=dot", "Workflow.start", traceFixtureRoot()},
	} {
		var errOut bytes.Buffer
		code := runCLI(args, IO{Out: failingWriter{}, ErrOut: &errOut})
		if code != 1 {
			t.Fatalf("runCLI(%v) exit = %d, want 1", args, code)
		}
		if !strings.Contains(errOut.String(), "write trace: write failed") {
			t.Fatalf("runCLI(%v) stderr = %q, want writer error", args, errOut.String())
		}
	}
}

func TestTraceServiceConstructsSelectorOnlyForFullTTY(t *testing.T) {
	in := &testStream{name: "in"}
	out := &testStream{name: "out"}
	errOut := &testStream{name: "err"}
	var constructed int
	service := traceService{
		ttyDetector: nil,
		selectorFactory: func(IO, int) trace.ImplementationSelector {
			constructed++
			return cmdTestSelector{}
		},
	}
	opts := defaultTraceOptions()

	for _, input := range []bool{false, true} {
		for _, stdout := range []bool{false, true} {
			for _, stderr := range []bool{false, true} {
				service.ttyDetector = fakeTTYDetector{
					input:  input,
					output: map[io.Writer]bool{out: stdout, errOut: stderr},
				}
				selector := service.selectorFor(opts, IO{In: in, Out: out, ErrOut: errOut})
				wantSelector := input && stdout && stderr
				if (selector != nil) != wantSelector {
					t.Fatalf("TTY combination input=%v stdout=%v stderr=%v selector=%v, want %v", input, stdout, stderr, selector != nil, wantSelector)
				}
			}
		}
	}
	if constructed != 1 {
		t.Fatalf("selector constructions = %d, want 1", constructed)
	}
}

func TestRunCLIInteractivePickerUsesStderrAndRendersOnce(t *testing.T) {
	root := m4FixtureRoot("interactive")
	in := strings.NewReader("1=1\n1=2\n")
	var out, errOut bytes.Buffer
	service := productionTraceService()
	service.ttyDetector = fakeTTYDetector{
		input:  true,
		output: map[io.Writer]bool{&out: true, &errOut: true},
	}

	code := runCLIWithService([]string{"trace", "app.Workflow.start", root}, IO{
		In: in, Out: &out, ErrOut: &errOut,
	}, service)
	if code != 0 {
		t.Fatalf("interactive exit = %d, stderr = %q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "app.AlphaA.run()") || !strings.Contains(out.String(), "app.GammaB.work()") {
		t.Fatalf("selected graph missing implementations:\n%s", out.String())
	}
	if strings.Contains(out.String(), "fluxos:") || !strings.Contains(errOut.String(), "fluxos:") {
		t.Fatalf("prompt stream separation failed: stdout=%q stderr=%q", out.String(), errOut.String())
	}
	if strings.Count(out.String(), "flowchart TD") != 1 {
		t.Fatalf("render count = %d, want one", strings.Count(out.String(), "flowchart TD"))
	}
}

func TestRunCLINonInteractivePoliciesNeverConstructOrReadSelector(t *testing.T) {
	root := m4FixtureRoot("interactive")
	tests := []struct {
		name string
		args []string
	}{
		{name: "no prompt", args: []string{"--no-prompt", "app.Workflow.start", root}},
		{name: "all", args: []string{"--all-impls", "app.Workflow.start", root}},
		{name: "pick partial", args: []string{"--pick-impls=contracts.A=app.AlphaA", "app.Workflow.start", root}},
		{name: "pick complete", args: []string{"--pick-impls=contracts.A=app.AlphaA,contracts.B=app.GammaB", "app.Workflow.start", root}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			service := productionTraceService()
			service.ttyDetector = fakeTTYDetector{
				input:  true,
				output: map[io.Writer]bool{&out: true, &errOut: true},
			}
			service.selectorFactory = func(IO, int) trace.ImplementationSelector {
				t.Fatal("selector constructed for an ineligible policy")
				return nil
			}

			code := runCLIWithService(append([]string{"trace"}, tt.args...), IO{
				In: cmdPanicReader{}, Out: &out, ErrOut: &errOut,
			}, service)
			if code != 0 {
				t.Fatalf("exit = %d, stderr = %q", code, errOut.String())
			}
		})
	}
}

func TestRunCLIExplicitFalseBooleansStillPromptOnFullTTY(t *testing.T) {
	root := m4FixtureRoot("interactive")
	for _, flag := range []string{"--no-prompt=false", "--all-impls=false"} {
		t.Run(flag, func(t *testing.T) {
			var out, errOut bytes.Buffer
			service := productionTraceService()
			service.ttyDetector = fakeTTYDetector{
				input:  true,
				output: map[io.Writer]bool{&out: true, &errOut: true},
			}
			code := runCLIWithService([]string{"trace", flag, "app.Workflow.start", root}, IO{
				In: strings.NewReader("1=1\n1=2\n"), Out: &out, ErrOut: &errOut,
			}, service)
			if code != 0 || !strings.Contains(out.String(), "app.GammaB.work()") {
				t.Fatalf("exit=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
			}
			if errOut.Len() == 0 {
				t.Fatal("full TTY prompt was not written")
			}
		})
	}
}

func TestRunCLIInteractiveAllHonorsMaxImplsAcrossRounds(t *testing.T) {
	root := m4FixtureRoot("interactive")
	var out, errOut bytes.Buffer
	service := productionTraceService()
	service.ttyDetector = fakeTTYDetector{
		input:  true,
		output: map[io.Writer]bool{&out: true, &errOut: true},
	}

	code := runCLIWithService([]string{"trace", "--max-impls=1", "app.Workflow.start", root}, IO{
		In: strings.NewReader("1=all\n1=all\n"), Out: &out, ErrOut: &errOut,
	}, service)
	if code != 0 {
		t.Fatalf("interactive all exit = %d, stderr = %q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "app.DeltaB.work()") || strings.Contains(out.String(), "app.GammaB.work()") {
		t.Fatalf("max-impls was not applied across rounds:\n%s", out.String())
	}
}

func TestRunCLIInteractiveCancellationReturnsOneWithoutPayload(t *testing.T) {
	root := m4FixtureRoot("interactive")
	var out, errOut bytes.Buffer
	service := productionTraceService()
	service.ttyDetector = fakeTTYDetector{
		input:  true,
		output: map[io.Writer]bool{&out: true, &errOut: true},
	}

	code := runCLIWithService([]string{"trace", "app.Workflow.start", root}, IO{
		In: strings.NewReader("cancel\n"), Out: &out, ErrOut: &errOut,
	}, service)
	if code != 1 {
		t.Fatalf("cancel exit = %d, want 1; stderr=%q", code, errOut.String())
	}
	if out.Len() != 0 {
		t.Fatalf("cancellation wrote payload: %q", out.String())
	}
	if !strings.Contains(errOut.String(), "implementation selection canceled") {
		t.Fatalf("cancellation diagnostic = %q", errOut.String())
	}
}

type cmdTestSelector struct{}

func (cmdTestSelector) Select([]resolve.DispatchSite) ([]trace.Selection, error) {
	return nil, nil
}

type cmdPanicReader struct{}

func (cmdPanicReader) Read([]byte) (int, error) {
	panic("ineligible path read stdin")
}
