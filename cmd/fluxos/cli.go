package main

import (
	"errors"
	"fmt"
	"io"
)

type IO struct {
	In     io.Reader
	Out    io.Writer
	ErrOut io.Writer
}

type UsageError struct {
	Err error
}

func (e *UsageError) Error() string {
	return e.Err.Error()
}

func (e *UsageError) Unwrap() error {
	return e.Err
}

func usageErrorf(format string, args ...any) error {
	return &UsageError{Err: fmt.Errorf(format, args...)}
}

func runCLI(args []string, streams IO) int {
	streams = normalizeIO(streams)
	if len(args) == 0 {
		writeUsage(streams.ErrOut)
		return 2
	}

	command := args[0]
	var err error
	switch command {
	case "trace":
		err = runTraceCommand(args[1:], streams)
	case "index":
		err = runIndexCommand(args[1:], streams)
	default:
		fmt.Fprintf(streams.ErrOut, "fluxos: unknown command %q\n", command)
		writeUsage(streams.ErrOut)
		return 2
	}
	if err == nil {
		return 0
	}

	fmt.Fprintf(streams.ErrOut, "fluxos %s: %v\n", command, err)
	var usageErr *UsageError
	if errors.As(err, &usageErr) {
		return 2
	}
	return 1
}

func normalizeIO(streams IO) IO {
	if streams.Out == nil {
		streams.Out = io.Discard
	}
	if streams.ErrOut == nil {
		streams.ErrOut = io.Discard
	}
	return streams
}

func writeUsage(out io.Writer) {
	fmt.Fprintln(out, "usage: fluxos <command>")
	fmt.Fprintln(out, "commands: index, trace")
}

func runTraceCommand(args []string, streams IO) error {
	opts, err := parseTraceOptions(args)
	if err != nil {
		return err
	}
	if err := validateTraceSupport(opts); err != nil {
		return err
	}
	return executeTrace(opts, streams)
}

func runIndexCommand(args []string, streams IO) error {
	opts, err := parseIndexOptions(args)
	if err != nil {
		return err
	}
	if err := validateIndexSupport(opts); err != nil {
		return err
	}
	return executeIndex(opts, streams)
}
