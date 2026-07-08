# Tasks: Overlay Coherence Mini-Fix

> Strict TDD active (`openspec/config.yaml`): RED → GREEN for each new behavior.
> Runner: `cd engine && go test ./...` and `cd tui && go test ./...`

## Phase 1 — Wrapper Dispatch Foundations

- [x] 1.1 **RED**: In `engine/installer/route_test.go`, add an integration test using `runOverlay` that runs `bin/labdrian-overlay gadu-generate --check` with a fake engine binary and asserts:
  - missing binary exits non-zero,
  - successful fake execution receives `gadu-generate --check` and uses repo-root `OVERLAY_DIR`.
- [x] 1.2 **GREEN**: In `bin/labdrian-overlay`, add `gadu-generate` to `usage()` and add a `cmd_gadu_generate` dispatch path.
  - Fail loudly if `$ENGINE_BINARY` is missing with actionable `labdrian install-hooks` guidance.
  - Forward with `exec env OVERLAY_DIR="$OVERLAY_DIR" "$ENGINE_BINARY" gadu-generate "$@"`.
- [x] 1.3 **RED**: In `engine/installer/route_test.go`, add a regression test for `status-hooks` that confirms a missing engine binary returns non-zero and prints `install-hooks` guidance, with no write to `~/.claude/bin`.
- [x] 1.4 **GREEN**: In `bin/labdrian-overlay`, make `cmd_status_hooks` read-only.
  - remove binary build/deploy fallbacks,
  - require executable `$ENGINE_BINARY` before `exec "$ENGINE_BINARY" status`,
  - keep behavior safe for repeated CI runs.

## Phase 2 — Canonical GADU Portability

- [x] 2.1 **RED**: In `engine/gadu/gadu_test.go`, add assertions that generated outputs:
  - are present for all three generated files,
  - contain `engine/gadu/persona/body.md` bytes,
  - do not include runtime-specific wording like `You run on Claude`.
- [x] 2.2 **GREEN**: Update `engine/gadu/persona/body.md` to remove runtime-specific Claude-only wording while preserving the canonical traits and structure.
- [x] 2.3 **GREEN**: Run `(cd engine && OVERLAY_DIR=/home/labdrian/labdrian-sdd-overlay go run ./cmd gadu-generate)` and regenerate:
  - `agents/GADU.md`
  - `opencode/agents/GADU.md`
  - `skills/gadu-operator/SKILL.md`

## Phase 3 — Documentation + Verification

- [x] 3.1 **RED**: Add focused help assertions in `engine/installer/route_test.go` that `bin/labdrian-overlay --help` contains:
  - `gadu-generate [--check]`,
  - read-only `status-hooks` guidance semantics.
- [x] 3.2 **GREEN**: Update `README.md` command reference to document:
  - `labdrian`/`bin/labdrian-overlay gadu-generate [--check]` dispatch behavior,
  - all three generated artifacts: `agents/GADU.md`, `opencode/agents/GADU.md`, `skills/gadu-operator/SKILL.md`.
- [x] 3.3 **GREEN**: Verify `proposal.md`, `design.md`, and `openspec/changes/overlay-coherence-mini-fix/specs/overlay-coherence/spec.md` still match implemented behavior; update only if wording drift appears.
- [x] 3.4 **Verification**: Run and capture module-scoped checks:
  - `bash -n bin/labdrian-overlay`
  - `cd engine && go test ./...`
  - `cd tui && go test ./...`
  - `(cd engine && OVERLAY_DIR=/home/labdrian/labdrian-sdd-overlay go run ./cmd gadu-generate --check)`
  - `bin/labdrian-overlay --help`
  - `cd engine && go test ./gadu -run TestGenerate -count=1`

## Review Workload Forecast

decision_needed_before_apply: no
chained_prs_recommended: no
budget_risk: medium
estimated_changed_lines: 700-800 (actual PR footprint including archived OpenSpec artifacts)
