# OpenCode agent instructions

## Build prerequisites

- Module: `github.com/lucas-garcia-rubio/fluxos`; use Go `1.26.3` from `go.mod`.
- Tree-sitter uses native C code: a CGO-enabled build and `gcc`/C toolchain are required; `CGO_ENABLED=0` is unsupported.
- CI and release qualification use native Ubuntu 24.04 `linux/amd64`.

## Checks

```bash
go test ./...
go test -race ./...
go vet ./...
go build ./...
go test ./internal/...
go test ./cmd/fluxos -run 'Test(RuntimeContextGoldens|M4LimitsGoldens|M4InteractiveGoldens)'
go test ./cmd/fluxos -run TestScope
```

## Code path and analysis boundary

- `cmd/fluxos` is the CLI. The main flow is discovery → tree-sitter parse/extract → index → resolve/graph/trace → renderer.
- `internal/project` discovers Java from `src/main/java` by default and skips `build`, `generated`, `target`, and `out`; `--scope=all` also includes `src/test/java`.
- Analysis is deliberately project-source-only: do not assume dependency, JDK, bytecode, reflection, or Lombok resolution.

## Output and fixtures

- Keep graph and index output on stdout; prompts, warnings, and CLI errors belong on stderr.
- Graph output must remain deterministic. Mermaid is the default renderer.
- JSON schema v1 is stable: preserve ordered arrays and explicit empty values (`[]`, `""`, numeric zero, or `null` as specified).
- Golden tests and Java fixtures are under `testdata/m3` and `testdata/m4`; deliberate renderer behavior changes require updating the matching goldens.

## Tree-sitter ownership

- Parser and tree objects own C resources. When editing parsing or extraction, explicitly `Close()` both the parser and every returned tree.

## Release qualification

- Prefer `./scripts/verify-release-candidate.sh --diagnostic <version> <output-dir>` for local qualification.
- The gate requires a clean worktree and a new/empty artifacts directory outside the repo; it runs tests, race, vet, build, reproducibility, and smoke checks.
- Local success still returns exit code `3` as `pending-remote-ci` until remote CI qualifies it.
- Official builds are native Ubuntu 24.04 `linux/amd64`; Java fixture compilation requires JDK 21.
- Expected release artifacts are the binary, `SHA256SUMS`, and `BUILD_INFO.txt`; publication is gated by GitHub's `release` environment.

## Release workflow safety

- GitHub's tag lookup does not return draft releases: retain paginated draft discovery and the pinned release ID strategy.
- A published release is verify-only and must never be changed. Existing draft assets must be byte-checked against the build artifacts before publication.
