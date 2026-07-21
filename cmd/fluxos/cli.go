package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/lucas-garcia-rubio/fluxos/internal/prompt"
	"github.com/lucas-garcia-rubio/fluxos/internal/trace"
)

type IO struct {
	In     io.Reader
	Out    io.Writer
	ErrOut io.Writer
}

type selectorFactory func(streams IO, maxImpls int) trace.ImplementationSelector

// traceService contains the command-boundary dependencies needed to decide
// whether a trace may prompt. Keeping these dependencies on the service makes
// the routing testable without package globals or terminal access.
type traceService struct {
	ttyDetector     TTYDetector
	selectorFactory selectorFactory
}

func productionTraceService() traceService {
	return traceService{
		ttyDetector: fdTTYDetector{},
		selectorFactory: func(streams IO, maxImpls int) trace.ImplementationSelector {
			return prompt.NewPicker(streams.In, streams.ErrOut, maxImpls)
		},
	}
}

func (service traceService) selectorFor(opts TraceOptions, streams IO) trace.ImplementationSelector {
	if !opts.NoPrompt && !opts.AllImpls && len(opts.PickImpls) == 0 && fullTTY(service.ttyDetector, streams) {
		if service.selectorFactory == nil {
			return nil
		}
		return service.selectorFactory(streams, opts.MaxImpls)
	}
	return nil
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
	return runCLIWithService(args, streams, productionTraceService())
}

func runCLIWithService(args []string, streams IO, service traceService) int {
	streams = normalizeIO(streams)
	if len(args) == 0 {
		writeUsage(streams.ErrOut)
		return 2
	}

	command := args[0]
	var err error
	switch command {
	case "trace":
		err = runTraceCommandWithService(args[1:], streams, service)
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
	return runTraceCommandWithService(args, streams, productionTraceService())
}

func runTraceCommandWithService(args []string, streams IO, service traceService) error {
	opts, err := parseTraceOptions(args)
	if err != nil {
		return err
	}
	if err := validateTraceSupport(opts); err != nil {
		return err
	}
	return executeTraceWithService(opts, streams, service)
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
