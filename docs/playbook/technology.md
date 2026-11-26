# Technology

| Technology | Version | Why |
|---|---|---|
| Go, 12 separate modules (engine, tui, longterm-mem, identity, telemetry, archguard, 6 under `tools/`) | `go 1.21` in most; `1.26.1` in tui and longterm-mem; `1.26.0` in tools/lint | Open |
| Bubbletea, Bubbles, Lipgloss (TUI) | v1.3.10, v1.0.0, v1.1.0 | Open |
| Bash CLI: `bin/labdrian-overlay`, `bin/overlay`, `bin/lib/*.sh` | — | Open |

The architecture's reasons are in the ADRs under `docs/architecture/` and in [hexagonal-target.md](../architecture/hexagonal-target.md).

## How a change is checked

CI, [.github/workflows/ci.yml](../../.github/workflows/ci.yml), on each pull request: `bash -n` on the CLI; `go test ./... -cover` on every Go module except `tools/lint`, which only builds the linters; gofmt and `go vet` on most modules; staticcheck on engine, tui, longterm-mem, archguard, identity and telemetry; deadcode on engine and tui; errcheck and nilerr are built but not run as a gate; `skills validate` against `skills.registry.yaml`; `gadu-generate --check` for drift of the agents generated from `roles/`; `review anchor --merge HEAD`; the contract and fixture validators. Shellcheck runs but does not fail the build (`|| true`).

## Delivery

Versioning: semver computed from conventional commits on every push to `main`; `.github/workflows/release.yml` tags `vX.Y.Z`. The version exists only as the tag; `CHANGELOG.md` is not cut per release (audit-2026-11-25-3).

Delivery default: a `feat/*` branch, a pull request, a merge commit to `main` with CI green (audit-2026-11-25-3).
