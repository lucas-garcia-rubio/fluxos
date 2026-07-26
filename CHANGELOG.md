# Changelog

## v1.0.0 — 2026-07-26

**Status:** `v1.0.0` is an existing annotated tag at the current HEAD, and green
remote CI was confirmed. This entry does not assert a published GitHub Release,
URLs, distributed assets, or concrete published checksums.

Delivered:

- Added iterative discovery and coordination for reachable polymorphic
  implementations, with deterministic choices carried into the final graph.
- Added TTY selection and non-interactive `--no-prompt`, `--all-impls`, and
  `--pick-impls` modes.
- Added source scopes, traversal limits and truncation metadata, Mermaid/DOT/JSON
  output, JSON schema v1, and unresolved-output filters.
- Added `--version` and linker-injected release versions.
- Defined the supported release policy as native Linux/amd64 with CGO/GCC,
  qualified against Ubuntu 24.04; other platforms are not promised.
- Added the GitHub Actions workflow and a same-host reproducible-build verifier
  using independent build caches.
- Added tooling to promote a release bundle containing the executable, `SHA256SUMS`, and
  `BUILD_INFO`.
- Added smoke coverage for M3/M4, JSON/DOT output, and the pinned Spring Petclinic
  target.
- Added the MIT license.

## v1.0.0-rc1 — 2026-07-25 (superseded)

**Historical status:** at this stage, the release candidate was in preparation;
it was not published and was not tagged.

This entry records the factual M4 and release-candidate work present at that
historical stage:

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
  RC qualification was asserted at that stage.
- Added the release-candidate artifact/checksum output path, while keeping concrete artifacts
  outside the repository.
- Added a release-candidate smoke (`scripts/smoke-release-candidate.sh`) that validates a promoted
  bundle, goldens M3/M4, JSON/DOT runtime-context output, and the Spring Petclinic target pinned by
  URL/SHA without running Maven; the clone is temporary and never persists in the repository.
- Added a release-candidate gate (`scripts/verify-release-candidate.sh`) that runs local test, race,
  vet, reproducible-build, and smoke checks, then printed `local gate: passed`, `remote CI: pending`,
  and exited 3 with `result: pending-remote-ci`. At that stage, the pending gate was not RC approval;
  observed green remote CI remained a separate blocker.

At that stage, the CI workflow existed, but no remote green execution had been observed. No
distributed binary, publication automation, release publication, remote release, Git tag, or
concrete checksum was part of this RC entry. These statements describe the superseded RC stage,
not the current v1.0.0 status.
