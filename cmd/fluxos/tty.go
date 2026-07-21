package main

import (
	"io"

	"golang.org/x/term"
)

// TTYDetector is the capability boundary used by the CLI before enabling an
// interactive selector. Input and output are deliberately separate because a
// redirected output stream must not be treated as an interactive terminal.
type TTYDetector interface {
	IsInputTTY(io.Reader) bool
	IsOutputTTY(io.Writer) bool
}

// fdTTYDetector is the production detector. It only accepts streams exposing
// the standard Fd method; arbitrary readers and writers are not guessed to be
// terminals.
type fdTTYDetector struct{}

func (fdTTYDetector) IsInputTTY(stream io.Reader) bool {
	return isTerminalStream(stream)
}

func (fdTTYDetector) IsOutputTTY(stream io.Writer) bool {
	return isTerminalStream(stream)
}

func isTerminalStream(stream any) bool {
	fdStream, ok := stream.(interface{ Fd() uintptr })
	if !ok || fdStream == nil {
		return false
	}

	fd := fdStream.Fd()
	if fd == ^uintptr(0) {
		return false
	}
	return term.IsTerminal(int(fd))
}

// fullTTY reports whether all streams needed for an interactive prompt are
// terminals. Keeping this check at the command boundary prevents prompt
// policy from leaking into the selector package.
func fullTTY(detector TTYDetector, streams IO) bool {
	if detector == nil || streams.In == nil || streams.Out == nil || streams.ErrOut == nil {
		return false
	}
	return detector.IsInputTTY(streams.In) &&
		detector.IsOutputTTY(streams.Out) &&
		detector.IsOutputTTY(streams.ErrOut)
}
