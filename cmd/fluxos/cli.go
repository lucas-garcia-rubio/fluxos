package main

import (
	"errors"
	"fmt"
	"io"
	"strings"

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
	if args[0] == "version" || args[0] == "--version" {
		if len(args) != 1 {
			fmt.Fprintf(streams.ErrOut, "fluxos: %s does not accept arguments\n", args[0])
			return 2
		}
		fmt.Fprintf(streams.Out, "fluxos %s\n", version)
		return 0
	}
	if args[0] == "--help" || args[0] == "-h" {
		if len(args) != 1 {
			fmt.Fprintln(streams.ErrOut, "fluxos: --help does not accept arguments")
			return 2
		}
		writeGlobalHelp(streams.Out)
		return 0
	}
	if args[0] == "help" {
		if len(args) == 1 {
			writeGlobalHelp(streams.Out)
			return 0
		}
		if len(args) == 2 {
			switch args[1] {
			case "trace":
				writeTraceHelp(streams.Out)
				return 0
			case "index":
				writeIndexHelp(streams.Out)
				return 0
			}
		}
		fmt.Fprintf(streams.ErrOut, "fluxos: unknown help topic %q\n", strings.Join(args[1:], " "))
		return 2
	}

	command := args[0]
	if len(args) == 2 && (args[1] == "--help" || args[1] == "-h") {
		switch command {
		case "trace":
			writeTraceHelp(streams.Out)
			return 0
		case "index":
			writeIndexHelp(streams.Out)
			return 0
		}
	}
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
	fmt.Fprintln(out, "commands: index, trace, version")
}

func writeGlobalHelp(out io.Writer) {
	fmt.Fprintln(out, "fluxos — deterministic Java call graphs")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Usage:")
	fmt.Fprintln(out, "  fluxos <command> [options]")
	fmt.Fprintln(out, "  fluxos help [trace|index]")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Commands:")
	fmt.Fprintln(out, "  trace  trace a method and render Mermaid, DOT, or JSON")
	fmt.Fprintln(out, "  index  print the indexed Java types as JSON")
	fmt.Fprintln(out, "  version  print the CLI version")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Examples:")
	fmt.Fprintln(out, "  fluxos trace com.example.Workflow.start ./project")
	fmt.Fprintln(out, "  fluxos trace --format=dot com.example.Workflow.start ./project")
	fmt.Fprintln(out, "  fluxos trace --format=json com.example.Workflow.start ./project")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Use fluxos trace --help or fluxos index --help for options.")
}

func writeTraceHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: fluxos trace [options] <target> [project-path]")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Target: [FQCN.]TypeName.method[(signature)] (for example, Workflow.start or com.example.Workflow.start(java.lang.String,int)).")
	fmt.Fprintln(out, "The project path defaults to .; Mermaid is written to stdout by default.")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  --format=mermaid|dot|json       output format (default: mermaid)")
	fmt.Fprintln(out, "  --direction=TD|LR|BT|RL         Mermaid direction (default: TD)")
	fmt.Fprintln(out, "  --scope=main|all                source scope (default: main)")
	fmt.Fprintln(out, "  --include-unresolved[=true|false] include unresolved terminals (default: true)")
	fmt.Fprintln(out, "  --max-depth=N                   depth limit; 0 means unlimited (default: 0)")
	fmt.Fprintln(out, "  --max-nodes=N                   node limit; 0 means unlimited (default: 1000)")
	fmt.Fprintln(out, "  --max-impls=N                   implementations per dispatch; 0 is unlimited (default: 5)")
	fmt.Fprintln(out, "  --all-impls[=true|false]        fan out to all implementations, up to --max-impls")
	fmt.Fprintln(out, "  --pick-impls=<receiver-fqcn>=<implementation-fqcn|all|none>[,...]")
	fmt.Fprintln(out, "  --no-prompt[=true|false]        never open the TTY picker")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Examples:")
	fmt.Fprintln(out, "  fluxos trace --direction=LR Workflow.start ./project")
	fmt.Fprintln(out, "  fluxos trace --format=dot Workflow.start ./project")
	fmt.Fprintln(out, "  fluxos trace --format=json --include-unresolved=false Workflow.start ./project")
}

func writeIndexHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: fluxos index [options] <project-path>")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Print indexed Java types as JSON to stdout.")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  --scope=main|all  source scope (default: main)")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Example:")
	fmt.Fprintln(out, "  fluxos index --scope=all ./project")
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
