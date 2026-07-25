# Changelog

## v1.0.0-rc1 — 2026-07-25

**Status:** release candidate in preparation; not published and not tagged.

This entry records the factual M4 and release-candidate work already present in the repository:

- Added iterative discovery and coordination for reachable polymorphic implementation sites,
  with deterministic choices carried into the final graph.
- Added batched TTY selection, non-interactive behavior, `--no-prompt`, `--all-impls`, and
  declarative `--pick-impls` handling.
- Added source-scope selection, traversal limits, visible truncation metadata, deterministic
  Mermaid/DOT/JSON rendering, JSON schema v1, and unresolved-output filtering.
- Added CLI help and documentation for the supported commands, formats, options, and limits.
- Added `version`/`--version`, linker-injected release versions, and the native Linux/amd64
  CGO/GCC release policy with Ubuntu 24.04 as its qualification baseline.
- Added the GitHub Actions validation workflow and a reproducible-build verifier that compares two
  builds of the same source and version on one host with independent temporary `GOCACHE`
  directories. This is same-host repeatability evidence, not RC qualification.
- Kept the qualification scope limited to native Linux/amd64 with the Ubuntu 24.04 baseline; no
  RC qualification is asserted here.
- Added the release-candidate artifact/checksum output path, while keeping concrete artifacts
  outside the repository.
- Added a release-candidate smoke (`scripts/smoke-release-candidate.sh`) that validates a promoted
  bundle, goldens M3/M4, JSON/DOT runtime-context output, and the Spring Petclinic target pinned by
  URL/SHA without running Maven; the clone is temporary and never persists in the repository.
- Added a release-candidate gate (`scripts/verify-release-candidate.sh`) that runs local test, race,
  vet, reproducible-build, and smoke checks, then prints `local gate: passed`, `remote CI: pending`,
  and exits 3 with `result: pending-remote-ci`. A pending gate is not RC approval; observed green
  remote CI remains a separate blocker.

The CI workflow exists, but no remote green execution has been observed. No distributed binary,
publication automation, release publication, remote release, Git tag, or concrete checksum is
part of this entry.
