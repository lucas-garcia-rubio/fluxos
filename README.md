# fluxos

`fluxos` builds deterministic Mermaid, DOT, or JSON call graphs from Java source. It discovers Java
compilation units, indexes project types and methods, and traces calls across files,
packages, inheritance, constructors, method references, and polymorphic receivers.

## Build

The project requires the Go version declared in `go.mod`, CGO, and a C toolchain because
tree-sitter uses native code. `CGO_ENABLED=0` is not supported; there is no fallback.

```bash
go build ./cmd/fluxos
```

The resulting executable is `./fluxos` when built from the repository root.

A local build uses the default version:

```bash
go build -o fluxos ./cmd/fluxos
./fluxos --version  # fluxos dev
```

A release-like build can inject a version with `-ldflags`:

```bash
CGO_ENABLED=1 go build -ldflags="-X main.version=v1.0.0-rc1" -o fluxos ./cmd/fluxos
./fluxos --version  # fluxos v1.0.0-rc1
```

To verify a release-like build reproducibly, start from a clean Git checkout and run:

```bash
./scripts/verify-reproducible-build.sh v1.0.0
```

The verification requires a native Ubuntu 24.04 `linux/amd64` host by default. A host such as
Arch Linux can run a diagnostic-only proof with `--diagnostic`:

```bash
./scripts/verify-reproducible-build.sh --diagnostic v1.0.0
```

The script uses `GOENV=off`, `CGO_ENABLED=1`, `CC=gcc`, `GOOS=linux`, `GOARCH=amd64`,
`GOAMD64=v1`, clean `GOFLAGS`/`GOEXPERIMENT`/`SOURCE_DATE_EPOCH`/`CGO_*FLAGS`, and two
temporary `GOCACHE` directories. Each build uses `-a -trimpath -buildvcs=false -mod=readonly`
and `-ldflags "-s -w -X main.version=<version>"`. `-buildvcs=false` means the commit is
recorded in the verification log, not embedded in the binary.

Matching binaries prove repeatability on the same clean host and toolchain; they do not prove
reproducibility between different installations or different toolchains. A diagnostic proof is
never an official release qualification.

Release-candidate history is recorded in the [changelog](CHANGELOG.md), with the
[v1.0.0-rc1 release notes](docs/release-notes/v1.0.0-rc1.md) covering the artifact evidence and
remaining qualification limits. To promote one verified Linux/amd64 artifact and its metadata
outside the checkout, use a new or empty directory outside the Git worktree:

```bash
./scripts/verify-reproducible-build.sh \
  --output-dir /tmp/fluxos-v1.0.0-rc1 \
  v1.0.0-rc1
```

This produces `fluxos-v1.0.0-rc1-linux-amd64`, `SHA256SUMS`, and `BUILD_INFO.txt` only after
the two builds match and both version checks pass. Add `--diagnostic` on a non-baseline host;
that output is never an official qualification.

The current release-candidate policy qualifies only native `linux/amd64`.
Ubuntu 24.04 x64 with GCC is the build and validation baseline; it is not a guarantee of
broad compatibility across distributions. Release builds use `CGO_ENABLED=1` on a native
`linux/amd64` host.

ARM, macOS, and Windows targets are not release-qualified in this RC. Source builds on
those targets are best-effort, not a compatibility promise. Generic cross-compilation by
setting only `GOOS` and `GOARCH` is unsupported. A target gains support only after a native
build, the test suite and `go vet`, inspection, and a runtime smoke test.

There are not yet any distributed binaries or publication automation. The CI workflow exists, but
no remote green execution has been observed.

Smoke a promoted bundle without Maven:

```bash
./scripts/smoke-release-candidate.sh \
  v1.0.0-rc1 \
  /tmp/fluxos-v1.0.0-rc1
```

The smoke verifies `SHA256SUMS`, `BUILD_INFO.txt`, and `--version`, then checks the M3
`Workflow.start` golden, non-interactive M4 options, JSON/DOT runtime-context goldens, and the
Spring Petclinic target `org.springframework.samples.petclinic.owner.OwnerController.processFindForm`
at commit `c36452a2c34443ae26b4ecbba4f149906af14717` from the pinned repository URL.

The local release-candidate gate runs tests, race, vet, the reproducible build, and this smoke:

```bash
./scripts/verify-release-candidate.sh \
  --diagnostic \
  v1.0.0-rc1 \
  /tmp/fluxos-v1.0.0-rc1
```

It reports `local gate: passed` only when those checks pass, then reports
`remote CI: pending` and exits with status 3 (`result: pending-remote-ci`). A pending gate is
not RC approval or publication.

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

Use `./fluxos --help`, `./fluxos -h`, or `./fluxos help` for the global overview. Each
command also has help: `./fluxos trace --help` and `./fluxos index --help`. Help is printed
to stdout and does not require a project or target.

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

Truncations are readable note nodes in Mermaid and DOT, for example
`% truncation: node limit; omitted 3 while tracing com.example.Workflow.start()`.
Internal node and dispatch IDs are not placed in the human-readable label. JSON retains the
structured truncation record.

Unresolved terminals are included by default. To omit unresolved nodes and their incident
edges while keeping other terminal outcomes and truncation metadata, use:

```bash
./fluxos trace --include-unresolved=false Workflow.start ./project
```

For the complete machine-readable contract, see [JSON schema v1](docs/json-schema.md).

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

The grammar is `--pick-impls=<receiver-fqcn>=<implementation-fqcn|all|none>[,...]`.
`none` keeps the ambiguous terminal, `all` fans out up to `--max-impls`, and an explicit
implementation follows only that runtime type. Unmapped receivers retain the default
ambiguous terminal. Unknown receivers and candidates fail before graph output is written.
Declarative picks, `--all-impls`, and `--no-prompt` never prompt. With stdin, stdout,
and stderr all connected to a TTY, the CLI otherwise automatically offers the
implementation picker; non-TTY runs never prompt.

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

## License

MIT License. See [LICENSE](LICENSE).
