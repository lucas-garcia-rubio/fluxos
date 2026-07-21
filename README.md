# fluxos

`fluxos` builds deterministic Mermaid, DOT, or JSON call graphs from Java source. It discovers Java
compilation units, indexes project types and methods, and traces calls across files,
packages, inheritance, constructors, method references, and polymorphic receivers.

## Build

The project requires the Go version declared in `go.mod`, CGO, and a C toolchain because
the tree-sitter runtime is native code.

```bash
go build ./cmd/fluxos
```

The resulting executable is `./fluxos` when built from the repository root.

## Index a project

```bash
./fluxos index ./path/to/java-project
```

The command prints the extracted Java types as JSON. If the supplied project contains
one or more Maven/Gradle-style `src/main/java` directories, all such main source roots
are indexed in deterministic path order. Build outputs and generated directories are
excluded. Test sources are excluded by default; use `--scope=all` to include
`src/test/java`. If no conventional main source root exists, the supplied path is treated
as an explicit source root.

## Trace a call graph

```bash
./fluxos trace com.example.Workflow.start ./path/to/java-project
```

Targets may use a unique simple type name, a fully qualified class name, or an exact
method signature:

```text
Workflow.start
com.example.Workflow.start
com.example.Workflow.start()
com.example.Workflow.run(java.lang.String,int)
com.example.Outer.Inner.run
```

An overloaded root method requires an exact signature. Trace output is Mermaid by
default and is always written to standard output. For the same source tree, target, and
options, node IDs, labels, nodes, edges, and truncations are deterministic.

### Output and limits

```bash
./fluxos trace --direction=LR com.example.Workflow.start ./project
./fluxos trace --format=dot com.example.Workflow.start ./project
./fluxos trace --format=json --max-depth=12 --max-nodes=1000 com.example.Workflow.start ./project
```

Supported formats are `mermaid`, `dot`, and `json`. `--direction=TD|LR|BT|RL` applies
only to Mermaid. `--max-depth=0` means unlimited, `--max-nodes` defaults to 1000, and an
explicit `--max-nodes=0` disables that limit. Limits produce visible truncation metadata
instead of silently dropping analysis results.

### Polymorphic implementations

The default remains conservative: a receiver with multiple concrete implementations
produces an ambiguous terminal. Scripts can opt into all implementations:

```bash
./fluxos trace --all-impls --max-impls=5 com.example.Workflow.start ./project
```

Or select implementations declaratively by receiver FQCN:

```bash
./fluxos trace \
  --pick-impls='com.example.UserService=com.example.JdbcUserService,com.example.MailService=none' \
  com.example.Workflow.start ./project
```

Each mapping is `<receiver-fqcn>=<implementation-fqcn|all|none>`, separated by commas.
`none` keeps the ambiguous terminal, `all` fans out up to `--max-impls`, and an explicit
implementation follows only that runtime type. Unmapped receivers retain the default
ambiguous terminal. Unknown receivers and candidates fail before graph output is written.
Declarative picks never prompt; interactive selection is not implemented yet.

## Terminal nodes

Some calls cannot safely resolve to one project method. The graph keeps these outcomes
visible instead of guessing:

- `[unresolved]`: the receiver, type, method, or constructor cannot be resolved from
  indexed source.
- `[no implementation]`: an interface or abstract receiver has no concrete indexed
  implementation.
- `[ambiguous overload]`: conservative overload selection leaves multiple candidates.
- `[ambiguous: N implementations]`: a polymorphic receiver has multiple concrete
  implementations and no explicit `--all-impls` or `--pick-impls` selection applies.

See [LIMITATIONS.md](LIMITATIONS.md) for the supported analysis boundary.
