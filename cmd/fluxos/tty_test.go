package main

import (
	"bytes"
	"io"
	"testing"
)

func TestFDTTYDetectorRejectsNonFDStreams(t *testing.T) {
	detector := fdTTYDetector{}

	if detector.IsInputTTY(bytes.NewBufferString("input")) {
		t.Fatal("bytes.Buffer input must not be treated as a TTY")
	}
	if detector.IsOutputTTY(&bytes.Buffer{}) {
		t.Fatal("bytes.Buffer output must not be treated as a TTY")
	}
}

func TestFullTTYRequiresInputOutputAndErrorOutput(t *testing.T) {
	in := &testStream{name: "in"}
	out := &testStream{name: "out"}
	errOut := &testStream{name: "err"}

	tests := []struct {
		name   string
		input  bool
		output map[io.Writer]bool
		want   bool
	}{
		{
			name:   "all streams",
			input:  true,
			output: map[io.Writer]bool{out: true, errOut: true},
			want:   true,
		},
		{
			name:   "input is not tty",
			input:  false,
			output: map[io.Writer]bool{out: true, errOut: true},
		},
		{
			name:   "stdout is not tty",
			input:  true,
			output: map[io.Writer]bool{out: false, errOut: true},
		},
		{
			name:   "stderr is not tty",
			input:  true,
			output: map[io.Writer]bool{out: true, errOut: false},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			detector := fakeTTYDetector{input: test.input, output: test.output}
			if got := fullTTY(detector, IO{In: in, Out: out, ErrOut: errOut}); got != test.want {
				t.Fatalf("fullTTY() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestFullTTYRejectsNilStreams(t *testing.T) {
	in := &testStream{name: "in"}
	out := &testStream{name: "out"}
	errOut := &testStream{name: "err"}
	detector := fakeTTYDetector{
		input:  true,
		output: map[io.Writer]bool{out: true, errOut: true},
	}

	for _, streams := range []IO{
		{In: nil, Out: out, ErrOut: errOut},
		{In: in, Out: nil, ErrOut: errOut},
		{In: in, Out: out, ErrOut: nil},
	} {
		if fullTTY(detector, streams) {
			t.Fatalf("fullTTY(%+v) = true, want false", streams)
		}
	}
}

type fakeTTYDetector struct {
	input  bool
	output map[io.Writer]bool
}

func (detector fakeTTYDetector) IsInputTTY(io.Reader) bool {
	return detector.input
}

func (detector fakeTTYDetector) IsOutputTTY(stream io.Writer) bool {
	return detector.output[stream]
}

type testStream struct {
	name string
}

func (*testStream) Read([]byte) (int, error) {
	return 0, io.EOF
}

func (*testStream) Write(p []byte) (int, error) {
	return len(p), nil
}
