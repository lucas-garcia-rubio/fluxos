package main

import (
	"strconv"
	"strings"

	"github.com/lucas-garcia-rubio/fluxos/internal/project"
	"github.com/lucas-garcia-rubio/fluxos/internal/resolve"
)

type OutputFormat string

const (
	FormatMermaid OutputFormat = "mermaid"
	FormatDOT     OutputFormat = "dot"
	FormatJSON    OutputFormat = "json"
)

type TraceOptions struct {
	Target            TargetSpec
	ProjectRoot       string
	Format            OutputFormat
	Direction         string
	DirectionSet      bool
	Scope             project.ScopeMode
	PickImpls         map[string]resolve.FixedChoice
	AllImpls          bool
	NoPrompt          bool
	IncludeUnresolved bool
	MaxDepth          int
	MaxNodes          int
	MaxImpls          int
}

type IndexOptions struct {
	ProjectRoot string
	Scope       project.ScopeMode
}

type optionKind int

const (
	optionValue optionKind = iota
	optionBool
)

var traceOptionKinds = map[string]optionKind{
	"format":             optionValue,
	"direction":          optionValue,
	"scope":              optionValue,
	"pick-impls":         optionValue,
	"all-impls":          optionBool,
	"no-prompt":          optionBool,
	"include-unresolved": optionBool,
	"max-depth":          optionValue,
	"max-nodes":          optionValue,
	"max-impls":          optionValue,
}

var indexOptionKinds = map[string]optionKind{
	"scope": optionValue,
}

func defaultTraceOptions() TraceOptions {
	return TraceOptions{
		ProjectRoot:       ".",
		Format:            FormatMermaid,
		Direction:         "TD",
		Scope:             project.ScopeModeMain,
		IncludeUnresolved: true,
		MaxNodes:          1000,
		MaxImpls:          5,
	}
}

func parseTraceOptions(args []string) (TraceOptions, error) {
	opts := defaultTraceOptions()
	flags, positionals, err := scanOptions(args, traceOptionKinds)
	if err != nil {
		return TraceOptions{}, err
	}

	if value, ok := flags["format"]; ok {
		opts.Format, err = parseOutputFormat(value)
		if err != nil {
			return TraceOptions{}, err
		}
	}
	if value, ok := flags["direction"]; ok {
		opts.Direction, err = parseDirection(value)
		opts.DirectionSet = true
		if err != nil {
			return TraceOptions{}, err
		}
	}
	if value, ok := flags["scope"]; ok {
		opts.Scope, err = parseScopeMode(value)
		if err != nil {
			return TraceOptions{}, err
		}
	}
	if value, ok := flags["pick-impls"]; ok {
		opts.PickImpls, err = parsePickImpls(value)
		if err != nil {
			return TraceOptions{}, err
		}
	}
	if value, ok := flags["all-impls"]; ok {
		opts.AllImpls, err = parseBoolOption("all-impls", value)
		if err != nil {
			return TraceOptions{}, err
		}
	}
	if value, ok := flags["no-prompt"]; ok {
		opts.NoPrompt, err = parseBoolOption("no-prompt", value)
		if err != nil {
			return TraceOptions{}, err
		}
	}
	if value, ok := flags["include-unresolved"]; ok {
		opts.IncludeUnresolved, err = parseBoolOption("include-unresolved", value)
		if err != nil {
			return TraceOptions{}, err
		}
	}
	if value, ok := flags["max-depth"]; ok {
		opts.MaxDepth, err = parseLimit("max-depth", value)
		if err != nil {
			return TraceOptions{}, err
		}
	}
	if value, ok := flags["max-nodes"]; ok {
		opts.MaxNodes, err = parseLimit("max-nodes", value)
		if err != nil {
			return TraceOptions{}, err
		}
	}
	if value, ok := flags["max-impls"]; ok {
		opts.MaxImpls, err = parseLimit("max-impls", value)
		if err != nil {
			return TraceOptions{}, err
		}
	}

	if len(positionals) == 0 {
		return TraceOptions{}, usageErrorf("usage: fluxos trace [options] <[FQCN.]TypeName.method[(signature)]> [project-path]")
	}
	if len(positionals) > 2 {
		return TraceOptions{}, usageErrorf("trace accepts one target and at most one project path; got %d positional arguments", len(positionals))
	}
	opts.Target, err = ParseTargetSpec(positionals[0])
	if err != nil {
		return TraceOptions{}, &UsageError{Err: err}
	}
	if len(positionals) == 2 {
		opts.ProjectRoot = positionals[1]
	}

	if opts.AllImpls && len(opts.PickImpls) > 0 {
		return TraceOptions{}, usageErrorf("--all-impls=true cannot be combined with --pick-impls")
	}
	if opts.DirectionSet && opts.Format != FormatMermaid {
		return TraceOptions{}, usageErrorf("--direction cannot be combined with --format=%s", opts.Format)
	}
	return opts, nil
}

func parseIndexOptions(args []string) (IndexOptions, error) {
	flags, positionals, err := scanOptions(args, indexOptionKinds)
	if err != nil {
		return IndexOptions{}, err
	}
	if len(positionals) != 1 {
		return IndexOptions{}, usageErrorf("usage: fluxos index [options] <project-path>")
	}
	opts := IndexOptions{ProjectRoot: positionals[0], Scope: project.ScopeModeMain}
	if value, ok := flags["scope"]; ok {
		opts.Scope, err = parseScopeMode(value)
		if err != nil {
			return IndexOptions{}, err
		}
	}
	return opts, nil
}

func scanOptions(args []string, kinds map[string]optionKind) (map[string]string, []string, error) {
	flags := make(map[string]string)
	positionals := make([]string, 0, len(args))
	optionsEnded := false
	for i := 0; i < len(args); i++ {
		token := args[i]
		if optionsEnded {
			positionals = append(positionals, token)
			continue
		}
		if token == "--" {
			optionsEnded = true
			continue
		}
		if !strings.HasPrefix(token, "-") {
			positionals = append(positionals, token)
			continue
		}
		if !strings.HasPrefix(token, "--") {
			return nil, nil, usageErrorf("unknown short flag %q; only long flags are supported", token)
		}

		nameValue := strings.TrimPrefix(token, "--")
		name, value, hasValue := strings.Cut(nameValue, "=")
		kind, known := kinds[name]
		if !known {
			return nil, nil, usageErrorf("unknown flag --%s", name)
		}
		if _, duplicate := flags[name]; duplicate {
			return nil, nil, usageErrorf("duplicate flag --%s", name)
		}

		switch kind {
		case optionBool:
			if !hasValue {
				value = "true"
			} else if value == "" {
				return nil, nil, usageErrorf("flag --%s requires a boolean value", name)
			}
		default:
			if !hasValue {
				if i+1 >= len(args) || args[i+1] == "--" || strings.HasPrefix(args[i+1], "--") {
					return nil, nil, usageErrorf("flag --%s requires a value", name)
				}
				i++
				value = args[i]
			}
			if value == "" {
				return nil, nil, usageErrorf("flag --%s requires a non-empty value", name)
			}
		}
		flags[name] = value
	}
	return flags, positionals, nil
}

func parseOutputFormat(value string) (OutputFormat, error) {
	switch OutputFormat(value) {
	case FormatMermaid, FormatDOT, FormatJSON:
		return OutputFormat(value), nil
	default:
		return "", usageErrorf("invalid --format value %q; want mermaid, dot, or json", value)
	}
}

func parseDirection(value string) (string, error) {
	switch value {
	case "TD", "LR", "BT", "RL":
		return value, nil
	default:
		return "", usageErrorf("invalid --direction value %q; want TD, LR, BT, or RL", value)
	}
}

func parseScopeMode(value string) (project.ScopeMode, error) {
	switch project.ScopeMode(value) {
	case project.ScopeModeMain, project.ScopeModeAll:
		return project.ScopeMode(value), nil
	default:
		return "", usageErrorf("invalid --scope value %q; want main or all", value)
	}
}

func parseBoolOption(name, value string) (bool, error) {
	switch value {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, usageErrorf("invalid --%s value %q; want true or false", name, value)
	}
}

func parseLimit(name, value string) (int, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, usageErrorf("invalid --%s value %q; want a non-negative integer", name, value)
	}
	if parsed < 0 {
		return 0, usageErrorf("invalid --%s value %q; must not be negative", name, value)
	}
	return parsed, nil
}

func validateIndexSupport(opts IndexOptions) error {
	return nil
}
