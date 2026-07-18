# fluxos

`fluxos` builds deterministic Mermaid call graphs from Java source. It discovers Java
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
are indexed in deterministic path order. `src/test/java`, build outputs, and generated
directories are excluded. If no conventional main source root exists, the supplied path
is treated as an explicit source root.

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

An overloaded root method requires an exact signature. Trace output is Mermaid
flowchart text written to standard output. For the same source tree and target, node
IDs, labels, nodes, and edges are deterministic.

## Terminal nodes

Some calls cannot safely resolve to one project method. The graph keeps these outcomes
visible instead of guessing:

- `[unresolved]`: the receiver, type, method, or constructor cannot be resolved from
  indexed source.
- `[no implementation]`: an interface or abstract receiver has no concrete indexed
  implementation.
- `[ambiguous overload]`: conservative overload selection leaves multiple candidates.
- `[ambiguous: N implementations]`: a polymorphic receiver has multiple concrete
  implementations; M3 does not fan out automatically.

See [LIMITATIONS.md](LIMITATIONS.md) for the supported analysis boundary.
