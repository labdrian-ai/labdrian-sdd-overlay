# Tasks: gadu-portable-operator

> Strict TDD active. Runner: `cd engine && go test ./... && cd tui && go test ./...`
> Task ordering: RED → GREEN for every new unit. Tests travel in the same commit as
> the behavior they verify (work-unit-commits rule). The staleness-check scenario in
> `gadu_test.go` becomes GREEN only after task A5 (generated artifacts committed).
>
> Delivery: two chained PRs (stacked-to-main).
> - PR Slice A — `feat/gadu-generator`: canonical source + generator + tests + generated artifacts
> - PR Slice B — `feat/overlay-agent-route`: route_resolve helper + 5-function installer surgery + tests

---

## Prerequisite — E1 Durability Probe (no code)

**Must complete and document BEFORE writing any guard code. R-013, D4.**

- [x] **A0**: Run experiment E1.
  - Back up `~/.claude/agents/` to a local scratch directory.
  - Plant sentinel file `~/.claude/agents/_overlay-durability-probe.md` (record its
    sha256 + mtime).
  - Snapshot `~/.claude/agents/GADU.md` sha256 + mtime.
  - Run `gentle-ai sync` (and observe any pending `gentle-ai upgrade` or its dry-run).
  - Re-inspect `~/.claude/agents/`: is the sentinel still present and unmodified?
    Is `GADU.md` unchanged?
  - Classify behavior into exactly one of: `append-only/replace-catalog-only` or
    `clear-and-rewrite`.
  - Write the result to `docs/e1-durability-probe.md` (create file) with the
    classification and the raw snapshot evidence. This document is the gate for D4
    guard sizing — no guard code may be written until it exists.
  - **Requirements traced**: R-013, D4, AC-5.
  - **Commit**: `docs(gadu): E1 durability probe result and classification`

---

## PR Slice A — Canonical Source + Generator + Tests + Generated Artifacts

Branch: `feat/gadu-generator` → target `main`

### A1 — RED: Failing generator tests

- [x] **A1**: Create `engine/gadu/gadu_test.go` with all scenarios. Tests MUST fail
  because `engine/gadu` package does not yet exist. Use `t.TempDir()` for all
  file-emission assertions.
  - `TestGenerate_BothFilesEmitted`: call `Generate` with a tempdir as repo root;
    assert both `agents/GADU.md` and `skills/gadu-operator/SKILL.md` are created.
  - `TestGenerate_BodyIsIdentical`: compare persona body sections of both emitted
    files byte-for-byte; assert they are equal.
  - `TestGenerate_AgentFrontmatter`: parse YAML frontmatter of emitted
    `agents/GADU.md`; assert `name == "GADU"`, non-empty description, `model == "opus"`,
    `tools` field present (R-004).
  - `TestGenerate_SkillFrontmatter`: inspect `skills/gadu-operator/SKILL.md`; assert
    it has a valid overlay skill frontmatter block with `name: gadu-operator` and a
    `description` containing `Trigger:` (R-005).
  - `TestGenerate_DoNotEditHeader`: assert both emitted files contain the do-not-edit
    comment string `GENERATED — DO NOT EDIT` (R-012).
  - `TestCheck_FailsWhenMissing`: run `Check` against a tempdir missing
    `skills/gadu-operator/SKILL.md`; assert error is non-nil (R-003).
  - `TestCheck_FailsWhenStale`: run `Generate` into a tempdir, modify one artifact,
    run `Check`; assert error is non-nil (R-003).
  - `TestCheck_PassesWhenInSync` (will remain RED until A5): run `Check` against the
    real repo root (`../..` from `engine/gadu/`); assert nil error. Skip via
    `t.Skip` when the generated files are absent so the suite is not hard-broken
    during A1–A4 development.
  - Table-driven subtests where multiple cases apply.
  - **Requirements traced**: R-001, R-002, R-003, R-004, R-005, R-012, D1, D8, AC-2.

### A2 — Canonical persona body

- [x] **A2**: Create `engine/gadu/persona/body.md`.
  - Port verbatim (in substance) from existing `~/.claude/agents/GADU.md` (mtime
    2026-06-23): the six non-negotiable defining traits (Judgment, Red-team, No
    sycophancy, Highest-probability path, Autonomy/agent orchestration,
    Source-grounded), Voice section, Signature capabilities (adversarial review +
    parallel fan-out, loaded as portable skills), Safety baseline, Memory section.
  - This file is BODY ONLY — no frontmatter, no do-not-edit header, no delivery-shape
    preamble. Frontmatter is generator-owned (D7).
  - This is the ONLY place persona content is authored. No other file holds persona
    body content (R-001).
  - **Requirements traced**: R-001, R-002, D7.
  - _(No test step; this is content. Tests in A1 assert the body appears in both outputs.)_

### A3 — GREEN: Implement generator package

- [x] **A3**: Create `engine/gadu/gadu.go` (package `gadu`) to make A1's tests pass.
  - `//go:embed persona/body.md` (path relative to `engine/gadu/gadu.go`).
  - Frontmatter template for the agent file (`agents/GADU.md`):
    ```
    ---
    name: GADU
    description: <single-line description>
    model: opus
    tools: '*'
    ---
    ```
  - Frontmatter template for the skill file (`skills/gadu-operator/SKILL.md`):
    standard overlay skill frontmatter (`name: gadu-operator`, description with
    `Trigger:` line, `license: Apache-2.0`, metadata block) plus a short
    load-into-native-subagent preamble paragraph (R-005). NOT auto-spawned (R-009).
  - Do-not-edit comment immediately after frontmatter close:
    ```
    <!-- GENERATED — DO NOT EDIT. Source: engine/gadu/persona/body.md. Run: gentle-ai-overlay gadu-generate -->
    ```
  - `Generate(repoRoot string) error`: writes both output files, creating parent dirs
    as needed. Accepts repo root so the caller (CLI + tests) can control paths.
  - `Check(repoRoot string) error`: calls Generate into a `os.MkdirTemp`, diffs the
    result against on-disk files at `<repoRoot>/agents/GADU.md` and
    `<repoRoot>/skills/gadu-operator/SKILL.md`; returns non-nil with a descriptive
    message on any mismatch or missing file.
  - **Requirements traced**: R-001, R-002, R-003, R-004, R-005, R-012, D1, D7.

### A4 — Wire gadu-generate subcommand

- [x] **A4**: Modify `engine/cmd/main.go`.
  - Add `"gadu-generate"` to the main switch.
  - `runGaduGenerate(args []string)`: parses optional `--check` flag; calls
    `gadu.Generate(overlayRoot)` or `gadu.Check(overlayRoot)` where `overlayRoot` is
    `../..` relative to the binary (or `os.Getenv("OVERLAY_DIR")` if set, for tests).
    Exits non-zero on error.
  - Update `usage()` to list: `engine gadu-generate [--check]`.
  - Import `engine/gadu` package.
  - **Requirements traced**: R-002, R-003, D1.
  - **Commit (A1 + A2 + A3 + A4)**: `feat(gadu): canonical persona body, generator package, and CLI subcommand (R-001..R-005 R-012)`

### A5 — Run generator; commit generated artifacts

- [x] **A5**: Execute `cd engine && go run ./cmd gadu-generate` from the overlay repo root.
  - Confirm `agents/GADU.md` and `skills/gadu-operator/SKILL.md` are written.
  - Verify YAML frontmatter in `agents/GADU.md` is valid (parse it; assert `name`,
    `model`, `description`, `tools` fields present).
  - Run `cd engine && go test ./gadu/...` — ALL subtests must pass, including
    `TestCheck_PassesWhenInSync` (which was skipped or RED in A1; now the generated
    files exist on disk).
  - Run full suite: `cd engine && go test ./...` — no regressions.
  - **Requirements traced**: R-002, R-003, AC-1, AC-2, D8.
  - **Commit**: `feat(gadu): generated GADU.md (agent) and SKILL.md (skill) from canonical source (AC-1 AC-2)`

### A6 — Manifest GADU rows

- [x] **A6**: Add two rows to `overlay.manifest`:
  ```
  gadu-operator/SKILL.md   custom
  GADU.md                  custom   agent
  ```
  - `gadu-operator/SKILL.md` has no route column (defaults to `skill` → 3 skills
    destinations) — same convention as existing custom skill rows (D3).
  - `GADU.md` has route `agent` (→ `~/.claude/agents` only) (D3, R-006).
  - Both are `custom` (gentle-ai ships neither; capture will not touch them) (R-010, D5).
  - **Requirements traced**: R-006, R-007, R-010, D3.
  - **Commit**: `feat(gadu): manifest entries for GADU agent and skill rows (R-006 R-007 D3)`

> **PR Slice A boundary**: open PR `feat/gadu-generator → main`. Includes commits for
> A0 (docs), A1+A2+A3+A4 (generator), A5 (generated artifacts), A6 (manifest rows).
> Slice B targets a branch off main (after Slice A merges) or off Slice A (stacked).

---

## PR Slice B — Installer Route Surgery + Tests

Branch: `feat/overlay-agent-route` → target `main` (after Slice A merges)

Depends on: Slice A merged (manifest GADU rows present).

### B0 — RED: Failing installer route tests

- [x] **B0**: Create `engine/installer/route_test.go` (package `installer_test`). All
  tests MUST fail because `route_resolve` does not yet exist in `bin/labdrian-overlay`.
  Use `exec.Command("bash", "-c", "source … && route_resolve …")` to shell into the
  function. Use `t.TempDir()` for sandbox HOME in integration tests. Mark integration
  tests `testing.Short()`-skippable.

  **Unit tests (fast, always run):**
  - `TestRouteResolve_LegacySkillRow`: source `route_resolve` from
    `bin/labdrian-overlay`; call with a legacy bare-skill manifest path (e.g.
    `sdd-spec/SKILL.md`); assert emitted record has route=`skill`, repo_source ends in
    `skills/sdd-spec/SKILL.md`, and three target pairs for claude/opencode/codex.
    Backward-compat invariant (D2).
  - `TestRouteResolve_GADUSkillRow`: call with `gadu-operator/SKILL.md` (no route
    col in manifest); assert same 3-skills structure as legacy.
  - `TestRouteResolve_GADUAgentRow`: call with `GADU.md` (manifest route=`agent`);
    assert route=`agent`, repo_source ends in `agents/GADU.md`, exactly ONE target
    pair `claude:<HOME>/.claude/agents/GADU.md`, no opencode/codex targets (R-006).

  **Target-flag unit tests (fast):**
  - `TestRouteResolve_TargetFlag_AgentRowOpencode`: intersect agent row's target set
    with `--target opencode`; assert zero applicable targets (R-007, D2).
  - `TestRouteResolve_TargetFlag_AgentRowClaude`: intersect with `--target claude`;
    assert exactly `claude:~/.claude/agents/GADU.md` (R-007).
  - `TestRouteResolve_TargetFlag_SkillRowOpencode`: intersect skill row with
    `--target opencode`; assert exactly one opencode skills dest (R-007).

  **Integration tests (skip on `-short`):**
  - `TestApply_AgentLandsInClaudeAgents`: sandbox HOME + fixture manifest + fixture
    `agents/GADU.md` + fixture `skills/gadu-operator/SKILL.md`; run
    `cmd_apply --target all`; assert `<HOME>/.claude/agents/GADU.md` exists and skill
    is in all three skills dirs; assert NO skill file is in `<HOME>/.claude/agents`
    except GADU.md (AC-3).
  - `TestStatus_ReportsAgentFile`: after apply, run `cmd_status`; assert output
    references agent file path under `.claude/agents`, not under `.claude/skills` (R-007, AC-4).
  - `TestSyncCheck_DetectsMissingAgentFile`: after apply, delete
    `<HOME>/.claude/agents/GADU.md`; run `cmd_sync_check`; assert output contains
    `OVERLAY_NOT_DEPLOYED` for the agent row (R-007, AC-4).
  - `TestUnrelatedSkillUnchanged`: after apply, compare an unrelated skill in all
    three destinations; assert it matches the repo source byte-for-byte (D2
    backward-compat invariant).

  - **Requirements traced**: R-006, R-007, R-008, R-011, D2, D8, AC-3, AC-4.

### B1 — AGENT_TARGET_PATHS declaration

- [x] **B1**: In `bin/labdrian-overlay`, add immediately below the existing
  `TARGET_PATHS` declaration (after the closing parenthesis):
  ```bash
  declare -A AGENT_TARGET_PATHS=( [claude]="$HOME/.claude/agents" )
  ```
  This is the only place the agent destination is defined (D2). No other code reads
  it directly — only `route_resolve` emits target pairs using it.
  - **Requirements traced**: R-006, D2.

### B2 — GREEN: Implement route_resolve

- [x] **B2**: Add `route_resolve()` bash function to `bin/labdrian-overlay` (after the
  helper block, before `cmd_bootstrap`). Side-effect-free: only echoes one line.
  Contract (from D2):
  ```
  route_resolve <manifest_path>
  # Reads overlay.manifest for the row whose $1 == <manifest_path>,
  # extracts optional $3 (route column, default "skill").
  # Emits ONE tab-separated line:
  #   <route>  <repo_source_abs>  <target_name>:<dest_abs> [...]
  #
  # route=skill: repo_source=$OVERLAY_DIR/skills/<path>
  #              targets: claude:<CLAUDE_SKILLS>/<path> opencode:<OC_SKILLS>/<path> codex:<CODEX_SKILLS>/<path>
  # route=agent: repo_source=$OVERLAY_DIR/agents/<path>
  #              targets: claude:$HOME/.claude/agents/<path>
  ```
  Implementation:
  - `grep` the manifest for the row matching `$1` (first field); extract `$3` via
    `awk '{print $3}'`; default to `skill` if empty.
  - Construct `repo_source` based on route.
  - Emit target pairs by iterating `TARGET_PATHS` (skill) or `AGENT_TARGET_PATHS`
    (agent).
  - Run unit tests after this task: `cd engine && go test ./installer/... -run TestRoute`
    — unit tests (B0 non-integration) should now pass. Integration tests still RED
    until B3–B7.
  - **Requirements traced**: R-006, R-007, D2, D3.
  - **Commit (B0 + B1 + B2)**: `feat(installer): AGENT_TARGET_PATHS + route_resolve helper with unit tests (R-006 D2 D3)`

### B3 — Wire route_resolve into cmd_apply

- [x] **B3**: Modify `cmd_apply` in `bin/labdrian-overlay`.
  - In the inner `while IFS= read -r rel_path` loop (currently: `src="$OVERLAY_DIR/skills/${rel_path}"` and
    `dest="$tpath/${rel_path}"`):
    - Call `route_resolve "$rel_path"` to get the record.
    - Parse `repo_source` and the applicable target pairs from the record.
    - Intersect emitted targets with the current `$t` iteration; skip if the current
      target is not in the route's applicable set.
    - Replace the hardcoded `src`/`dest` with the route-resolved values.
    - Update the warning echo to use the route-relative path (e.g.
      `agents/$rel_path` for agent rows, `skills/$rel_path` for skill rows).
  - Skill rows must deploy identically to pre-change behavior (backward-compat, D2).
  - **Requirements traced**: R-006, R-007, D2, AC-3.

### B4 — Wire route_resolve into cmd_status

- [x] **B4**: Modify `cmd_status` in `bin/labdrian-overlay`.
  - Replace `repo_file="$OVERLAY_DIR/skills/${rel_path}"` and `live_file="$tpath/${rel_path}"`
    with values from `route_resolve`.
  - For targets not applicable to the row's route (e.g. agent row when target is
    opencode), skip the comparison (or emit a "not applicable" note — do not report
    MISSING for a non-applicable target).
  - **Requirements traced**: R-007, AC-4.

### B5 — Wire route_resolve into cmd_sync_check (CORRECTNESS-CRITICAL)

- [x] **B5**: Modify `cmd_sync_check` in `bin/labdrian-overlay`.
  - Replace `local live_file="$tpath/${rel_path}"` with the route-resolved live path
    (from `route_resolve`).
  - Replace **both** git-ref lookups for the `main` branch:
    - `git cat-file -e "main:skills/${rel_path}"` → `git cat-file -e "main:<route_subdir>/${rel_path}"`
    - `git show "main:skills/${rel_path}" … | sha256sum` → `git show "main:<route_subdir>/${rel_path}" … | sha256sum`
      **This second substitution (main_hash) is CORRECTNESS-CRITICAL**: if left as
      `main:skills/` for an agent row, `main_hash` is always empty and every agent
      row is permanently reported `OVERLAY_NOT_DEPLOYED`. (Design: D2 table, L511.)
  - Replace `upstream:skills/${rel_path}` at both the `git cat-file` and `git show`
    calls for upstream hash with `upstream:<route_subdir>/${rel_path}`.
  - Replace all human-readable echo lines that embed `skills/$rel_path` with the
    route-resolved prefix (e.g. `agents/GADU.md` for the agent row).
  - Skip non-applicable targets for agent rows (agent is claude-only).
  - **Requirements traced**: R-007, AC-4.

### B6 — Wire route_resolve into cmd_capture

- [x] **B6**: Modify `cmd_capture` in `bin/labdrian-overlay`.
  - In the `from_backup` branch and the live-copy branch, replace hardcoded
    `skills/${rel_path}` source/dest/tar_path with route-resolved values.
  - GADU rows are `custom`, so `managed_files()` already excludes them from capture
    iteration — this change is a no-op for GADU (D5, R-010). The route-awareness
    ensures a hypothetical future managed agent row would resolve correctly.
  - **Requirements traced**: R-007, R-010, D5.

### B7 — Wire route_resolve into cmd_bootstrap + git add agents/

- [x] **B7**: Modify `cmd_bootstrap` in `bin/labdrian-overlay`.
  - In the tarball-extract loop (`managed_files()` loop):
    - Replace `tar_path="files/home/labdrian/.claude/skills/${rel_path}"` and
      `dest="$OVERLAY_DIR/skills/${rel_path}"` with route-resolved values.
  - In the layer-custom loop (`parse_manifest` loop):
    - Replace `src="${TARGET_PATHS[claude]}/${rel_path}"` and
      `dest="$OVERLAY_DIR/skills/${rel_path}"` with route-resolved values (agent
      rows source from `${AGENT_TARGET_PATHS[claude]}/${rel_path}` and write to
      `$OVERLAY_DIR/agents/${rel_path}`).
    - A missing live source is benign: the existing "source not found, skipping"
      warn path handles it (D6).
  - Add `git add agents/` alongside the existing `git add skills/`:
    - At the upstream-baseline commit step (currently `git add skills/`).
    - At the overlay commit step (currently `git add skills/`).
  - **Requirements traced**: R-007, R-011, D6, AC-3.
  - **Commit (B3 + B4 + B5 + B6 + B7)**: `feat(installer): wire route_resolve into all five installer commands (R-006 R-007 R-011 D2 D4 D5 D6)`

### B8 — Known limitations note (documentation only, no code)

- [x] **B8**: Add a comment block near the top of `bin/labdrian-overlay` (below the
  `AGENT_TARGET_PATHS` declaration) documenting the following as known limitations
  that are explicitly OUT OF SCOPE for this change (per design):
  - The `bootstrap` init block's human-readable info echoes (dir creation) and the
    `git add skills/` in the initial scaffold commit are skills-only display lines.
    They remain skills-only and are a no-op for GADU (`custom` rows) — they only
    matter for a hypothetical future `managed` agent row.
  - No native opencode/Codex agent-definition files are produced (R-009).
  - No SDD orchestrator auto-spawn wiring (R-009).
  - **Requirements traced**: R-009.

### B9 — Verify full suite passes (no new task, checkpoint)

- [x] **B9**: Run `cd engine && go test ./... && cd tui && go test ./...`.
  - All installer unit tests (TestRouteResolve_*) pass.
  - All integration tests (TestApply_*, TestStatus_*, TestSyncCheck_*) pass or are
    skipped with `testing.Short()`.
  - No regressions in existing engine or tui tests.
  - **Requirements traced**: all; final validation before PR Slice B.
  - **Commit (if needed)**: `test(installer): full installer route test suite passing (AC-3 AC-4)`

> **PR Slice B boundary**: open PR `feat/overlay-agent-route → main`. Includes commits
> for B0+B1+B2 (route_resolve + unit tests), B3+B4+B5+B6+B7 (five-function surgery),
> B8 (known limitations). Passes `cd engine && go test ./...` including integration tests.

---

## PR Slice A — Commit Plan

| Commit | Files | Message |
|---|---|---|
| 1 | `docs/e1-durability-probe.md` | `docs(gadu): E1 durability probe result and classification` |
| 2 | `engine/gadu/gadu_test.go`, `engine/gadu/persona/body.md`, `engine/gadu/gadu.go`, `engine/cmd/main.go` | `feat(gadu): canonical persona body, generator package, and CLI subcommand (R-001..R-005 R-012)` |
| 3 | `agents/GADU.md`, `skills/gadu-operator/SKILL.md` | `feat(gadu): generated GADU.md (agent) and SKILL.md (skill) from canonical source (AC-1 AC-2)` |
| 4 | `overlay.manifest` | `feat(gadu): manifest entries for GADU agent and skill rows (R-006 R-007 D3)` |

## PR Slice B — Commit Plan

| Commit | Files | Message |
|---|---|---|
| 1 | `engine/installer/route_test.go`, `bin/labdrian-overlay` (AGENT_TARGET_PATHS + route_resolve) | `feat(installer): AGENT_TARGET_PATHS + route_resolve helper with unit tests (R-006 D2 D3)` |
| 2 | `bin/labdrian-overlay` (five-function surgery + git add agents/) | `feat(installer): wire route_resolve into all five installer commands (R-006 R-007 R-011 D2 D4 D5 D6)` |
| 3 | `bin/labdrian-overlay` (known-limit comment) | `docs(installer): known-limitation note for out-of-scope echo lines (R-009)` |

---

## Requirements Traceability

| Req | Tasks |
|---|---|
| R-001 (single canonical source) | A2, A3 |
| R-002 (generator emits both shapes) | A1-RED, A3-GREEN, A4, A5 |
| R-003 (staleness test) | A1-RED, A3-GREEN (`Check`), A5 (staleness goes GREEN) |
| R-004 (valid agent frontmatter) | A1-RED, A3-GREEN |
| R-005 (valid skill, loadable on demand) | A1-RED, A3-GREEN |
| R-006 (route by manifest column) | A6, B0-RED, B1, B2-GREEN |
| R-007 (five commands honor route) | B0-RED, B3, B4, B5, B6, B7 |
| R-008 (survive sync/upgrade) | A0 (E1), D4 guard sized after A0 |
| R-009 (no auto-spawn / no native agent files) | A3 (skill preamble), B8 |
| R-010 (capture skips custom) | A6 (both rows `custom`), B6 |
| R-011 (bootstrap route-aware) | B0-RED, B7 |
| R-012 (do-not-edit header) | A1-RED, A3-GREEN |
| R-013 (E1 before guard code) | A0 (must precede any guard implementation) |

---

## Parallel vs Sequential

```
A0 (E1 probe) ─────────────────────── sequential (gate for guard code)
A1-RED ──► A2 ──► A3-GREEN ──► A4 ──► sequential (standard TDD chain)
A5 (run generator) ──► A6 ─────────── sequential (artifacts must exist before manifest)

SLICE A PR opens after A6 ◄────────── Slice B can start on a branch in parallel,
                                       but its integration tests need A6 manifest rows
B0-RED ──► B1 ──► B2-GREEN ────────── sequential within Slice B
B3 ──────────────────────────────────┐
B4 ──────────────────────────────────┤ CAN RUN IN PARALLEL within a single session
B5 ──────────────────────────────────┤ (independent functions, no shared state)
B6 ──────────────────────────────────┤
B7 ──────────────────────────────────┘
B8, B9 (verification + doc) ─────────── after B3–B7
```

Tasks B3, B4, B5, B6, B7 touch independent functions in `bin/labdrian-overlay` and
may be implemented in parallel in a single working session, but MUST all be committed
before PR Slice B opens (no partial function wiring in a PR).
