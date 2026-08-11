# Technical Design: gadu-portable-operator

> Status: design (HOW at architecture level). Inputs: `proposal.md` and engram
> `sdd/gadu-portable-operator/proposal`. This document makes concrete, justified
> architecture decisions and resolves every propose-phase gate finding (F1–F6).
> It does NOT enumerate implementation tasks — that is the `sdd-tasks` phase.

## 1. Context and constraints

GADU is an orphan persona that exists only as a hand-made `~/.claude/agents/GADU.md`.
The overlay already distributes *skills* durably to three runtimes via
`bin/labdrian-overlay`, but it has no concept of an *agent* file and no
`~/.claude/agents` destination. We must make GADU reproducible, durable, and
multi-runtime WITHOUT touching the stripped `gentle-ai` binary and WITHOUT
auto-spawn wiring.

Hard constraints carried from the proposal (locked decisions are requirements):

- ONE canonical persona source; a generator emits BOTH delivery shapes (R-001/R-002).
- Skill rides the EXISTING skills route into 3 dirs; the agent file needs a NEW
  `agents/` route landing ONLY in `~/.claude/agents` (R-006/R-007).
- Both artifacts overlay-managed and survive `gentle-ai sync`/`upgrade` (R-008).
- No native opencode/Codex agent-definition files; no SDD auto-spawn (R-009).
- Strict TDD active; the only confirmed runner is Go
  (`cd engine && go test ./... && cd tui && go test ./...`).

### Spot-check findings that constrain the design

These were verified against the real repo before deciding (not re-derived):

1. **Manifest paths are route-relative + bare, with no consistent prefix.**
   `overlay.manifest` stores skill rows as bare paths (`sdd-spec/SKILL.md`); the
   `skills/` prefix is added in CODE (`src="$OVERLAY_DIR/skills/${rel_path}"`).
   It ALSO contains root-relative `engine/...` rows that `cmd_apply` silently
   skips (it would look for `skills/engine/go.mod`, warn, continue). There is
   therefore NO reliable `skills/` vs `agents/` path prefix to infer a route
   from. This directly rules out naive prefix inference for F4.
2. **gentle-ai already coexists with the orphan.** `~/.claude/agents/` currently
   holds `GADU.md` (mtime 2026-06-23) alongside gentle-ai's own `sdd-*.md`,
   `jd-*.md`, `review-*.md`. GADU has survived since 2026-06-23. This is the
   primary empirical signal that gentle-ai writes individual catalog files and
   does NOT clear the directory — but it is "suggests," not "proven" (F1).
3. **Go is a real runner for both layers.** `engine/` is a Go 1.21 module with an
   established pattern of integration tests that assert externally-verified
   behavior (`engine/gate/gate_e2e_test.go`). The `go-testing` skill explicitly
   sanctions `t.TempDir()` filesystem tests, golden files, and
   `testing.Short()`-skippable integration tests that run external commands.

## 2. Architecture approach

A **generate-then-deploy** pipeline with a single canonical author, layered on
the overlay's existing capture/apply model:

```
  engine/gadu/persona/body.md   (CANONICAL — the only place the persona body is authored)
            │
            ▼  gadu.Generate()  (Go; embeds frontmatter templates + generated header)
   ┌────────┴─────────────────────────────────┐
   ▼                                            ▼
 agents/GADU.md                       skills/gadu-operator/SKILL.md
 (Claude Code agent file)             (persona skill, all runtimes)
   │  route=agent                       │  route=skill (default)
   ▼                                    ▼
 ~/.claude/agents/GADU.md     ~/.claude/skills/… , ~/.config/opencode/skills/… , ~/.codex/skills/…
            ▲                                    ▲
            └──── bin/labdrian-overlay apply, driven by overlay.manifest ────┘
                  via the SINGLE route_resolve() helper (F3)
```

Two well-separated concerns, mirroring the existing `install-hooks` vs `apply`
split:

- **Build step (generator).** Reads the canonical body, emits both committed
  artifacts. Runs in dev/CI, NOT inside `apply`. A Go test enforces the committed
  artifacts are never stale (golden-style).
- **Deploy step (installer).** `overlay apply` deploys the COMMITTED artifacts to
  their routed destinations. Routing is centralized in one helper so the skills
  route stays byte-identical and the agent route is the only new behavior.

### Component map

| Component | Path | Kind | Owns |
|---|---|---|---|
| Canonical persona body | `engine/gadu/persona/body.md` | new, embedded | The ONLY authored persona content (6 traits, Voice, Signature capabilities, Safety, Memory). |
| Generator library | `engine/gadu/gadu.go` (`Generate`, `Check`) | new Go | Wrap body with per-shape frontmatter + generated header; emit both files; staleness check. |
| Generator subcommand | `engine/cmd/main.go` → `gadu-generate [--check]` | modified Go | Human/CI entry point; forwards to `gadu.Generate`/`Check`. |
| Generator test | `engine/gadu/gadu_test.go` | new Go | R-003/AC-2: missing/stale/divergent fails. |
| Generated agent file | `agents/GADU.md` | new (generated, committed) | Deployed by agent route. |
| Generated skill | `skills/gadu-operator/SKILL.md` | new (generated, committed) | Deployed by skill route. |
| Route helper | `route_resolve()` in `bin/labdrian-overlay` | new bash | Single source of truth: manifest entry → source dir + per-target dests + applicable targets. |
| Installer surgery | `cmd_apply`, `cmd_status`, `cmd_sync_check`, `cmd_capture`, `cmd_bootstrap` | modified bash | Consume `route_resolve` instead of hardcoded `skills/`. |
| Installer tests | `engine/installer/route_test.go` | new Go | Unit the bash helper + `-short`-skippable apply/status/sync-check integration in a sandbox HOME. |
| Manifest | `overlay.manifest` | modified | GADU rows + optional `route` column. |

## 3. Decisions (ADR-style)

### D1 — Generator language: GO (resolves F2 + locked decision a)

**Decision.** Write the generator as a Go package `engine/gadu` plus a thin
`gadu-generate` subcommand on the existing engine binary. The canonical persona
body lives at `engine/gadu/persona/body.md` and is `//go:embed`-ed. `Generate`
emits both `agents/GADU.md` and `skills/gadu-operator/SKILL.md`; `Check`
regenerates in memory and diffs against the on-disk committed copies (exit
non-zero on drift).

**Rationale.** Strict TDD needs a REAL runner. Go is the only confirmed runner
(`go test ./...`). The engine is already Go with established table-driven +
golden-file patterns. A bash generator would force a second test harness (bats or
Go-wrapping-bash) for the persona-emission logic itself, adding a runner with no
upside. Putting the generator in Go means its test runs under the EXISTING
canonical command with zero new harness, and the staleness check is a natural
golden test.

**Rejected.** Bash generator (`sed`/`cat` templating): no native test runner under
the canonical command; templating multi-section markdown in bash is brittle and
hard to assert structurally. Make/heredoc script: same harness gap.

### D2 — Single route-resolution helper (resolves F3 + locked decision b)

**Decision.** Add ONE pure bash function `route_resolve` to `bin/labdrian-overlay`
that is the single source of truth for mapping a manifest entry to its physical
routing. It has NO side effects (echoes a record), so it is independently
unit-testable. Contract:

```
# route_resolve <manifest_path>
#   Reads the entry's optional route column (default "skill") and emits ONE line:
#     <route>\t<repo_source_abs>\t<target1>:<dest1_abs>[ <target2>:<dest2_abs> ...]
#
#   route=skill  (default; ALL existing rows)
#     repo_source = $OVERLAY_DIR/skills/<path>
#     targets     = claude:~/.claude/skills/<path>
#                   opencode:~/.config/opencode/skills/<path>
#                   codex:~/.codex/skills/<path>
#   route=agent
#     repo_source = $OVERLAY_DIR/agents/<path>
#     targets     = claude:~/.claude/agents/<path>
#
#   Source SUBDIR (for git refs in sync-check) derives from route:
#     skill -> "skills/<path>"   agent -> "agents/<path>"
```

Supporting state: a parallel `declare -A AGENT_TARGET_PATHS=( [claude]="$HOME/.claude/agents" )`
next to the existing `TARGET_PATHS`. Callers intersect the emitted target set with
the user's `--target` selection (so `--target opencode` simply yields no work for
an agent row). The route name maps to a source subdir (`skills`|`agents`) used for
both the repo source path and the `main:`/`upstream:` git refs in sync-check.

**Lookup mechanism (no caller refactor).** `route_resolve` determines the route
INTERNALLY: it greps `overlay.manifest` for the row whose first field equals the
given `<manifest_path>` and reads that row's optional third `route` column
(`awk '{print $3}'`, defaulting to `skill` when the field is empty). That single
route value drives the source subdir, the destination set, and the git-ref subdir
in the emitted record. Every call site keeps its EXISTING loop structure
(`while IFS= read -r … < <(parse_manifest | all_tracked_files | managed_files)`);
the only per-site change is swapping the hardcoded `skills/${rel_path}` strings for
fields parsed out of the one `route_resolve "$rel_path"` record. No caller is
refactored to read the manifest's third column itself — that knowledge stays
encapsulated in `route_resolve`.

**Call sites (complete list).** All hardcoded `skills/` prefixes are replaced by
`route_resolve` output:

| Function | Lines today | What changes |
|---|---|---|
| `cmd_apply` | L386 (`src`), L387 (`dest`), L398 warn | `src`/`dest` from `route_resolve`; skip targets not in the route's applicable set. |
| `cmd_status` | L443 (`repo_file`), **L444 (`live_file`)** | `repo_file`/`live_file` from `route_resolve`; per-target applicability. |
| `cmd_sync_check` | **L500 (`live_file`)**, **L510 AND L511 (`main:skills/…` — `git cat-file -e` AND `git show`)**, L530 + L537 (`upstream:skills/…`), display echoes **L516, L524, L542, L546, L551** (hardcoded `skills/$rel_path`) | `live_file` from `route_resolve`; the `main:` git ref is dereferenced at BOTH L510 (`git cat-file -e`) and **L511 (`git show … | sha256sum`)** — **L511 is CORRECTNESS-CRITICAL**: if it keeps `main:skills/…` for an agent row, `main_hash` stays empty and the agent row is reported `OVERLAY_NOT_DEPLOYED` forever; `upstream:` refs at L530/L537 likewise use the route subdir; the human-readable echoes at L516/L524/L542/L546/L551 must print the resolved route prefix (`agents/` for agent rows) instead of the literal `skills/`. |
| `cmd_capture` | L293 (tar_path), L294 (dest), L307 (src), L308 (dest) | route-aware source/dest/tar_path (see D5 — no-op for GADU because it is `custom`). |
| `cmd_bootstrap` | L186 (tar_path), L187 (dest), **L213 (src), L214 (dest)** | route-aware layer-custom loop (see D6); the layer-custom loop sources the agent row from `${TARGET_PATHS[claude]}/${rel_path}` (L213) and writes it to `$OVERLAY_DIR/agents/${rel_path}` (L214); commit scope must `git add agents/` next to the existing `git add skills/` at L199 and L226. |

**Backward-compatibility invariant.** For any row WITHOUT a route column,
`route_resolve` emits exactly `skill` + `skills/<path>` + the three current
destinations. Therefore every existing skill deploys/statuses/sync-checks
byte-identically. This invariant is pinned by a characterization test in
`engine/installer/route_test.go` (D8). The pre-existing `engine/...` rows keep
their current inert behavior (resolved as `skill` → `skills/engine/...` → skipped
with the same warning); fixing that wart is explicitly out of scope.

**Rationale.** The proposal's own risk section asks for one helper rather than
copy-paste branching across five functions. A side-effect-free echo helper is the
smallest unit that can be tested in isolation and reused everywhere, keeping the
skills route provably unchanged.

**Rejected.** Per-function `if [[ $path == agents/* ]]` branches: duplicates
routing logic five times, regresses easily, untestable as a unit.

### D3 — Manifest schema: optional third `route` column (resolves F4)

> **Status: RATIFIED by the user.** The optional third `route ∈ {skill, agent}`
> column (default `skill`) is the approved schema; path-prefix inference is REJECTED.
> Backward-compatible with existing two-column rows; both GADU rows are `custom`.

**Decision.** Extend the row format to an OPTIONAL third column
`route ∈ {skill, agent}`, defaulting to `skill` when absent:

```
gadu-operator/SKILL.md   custom            # route omitted → skill → 3 skills dirs
GADU.md                  custom   agent    # agent route   → ~/.claude/agents only
```

Both rows are `custom` (gentle-ai ships neither). Paths are route-relative bare
names, consistent with how existing skill rows are skills-relative bare names.

**Rationale.** Path-prefix inference is unsafe here: existing skill rows are bare
(no `skills/` prefix) and `engine/...` rows are root-relative, so there is no
consistent prefix to key on (spot-check finding 1). An explicit column is
unambiguous, decouples routing from path shape, and is trivially
backward-compatible — the awk helpers read `$1`/`$2` only, so every existing
two-column row parses unchanged; only the new `route_resolve` reads `$3`. It also
generalizes (a future route is a new value, not a new prefix parser).

**Rejected.** Infer route from an `agents/` path prefix: would create a THIRD path
convention atop the inconsistent existing two, and force the helper to also
special-case `engine/`. Fragile and surprising.

### D4 — Durability: reproducible re-apply, sized by an empirical experiment (resolves F1)

**Decision.** Durability is delivered by REPRODUCIBILITY, not a filesystem lock:
GADU is overlay-managed (manifest rows), regenerable from the canonical source,
and re-deployable by `overlay apply`. Experiment **E1** is the MANDATORY FIRST STEP
of `sdd-apply` — it MUST run and be documented BEFORE any durability-guard code is
written or committed. No guard branch may be implemented until E1's result is
recorded; the D4 guard branch below is SELECTED from E1's documented classification,
never chosen up front.

**Experiment E1 (durability probe) — must run first.**

1. *Snapshot.* Record `ls -la ~/.claude/agents/`, the sha256 + mtime of
   `GADU.md`, and plant an unmanaged sentinel `~/.claude/agents/_overlay-durability-probe.md`
   (content + mtime known, a name NOT in gentle-ai's catalog). Back up the whole
   `~/.claude/agents/` dir first.
2. *Act.* Run `gentle-ai sync`; separately observe the next `gentle-ai upgrade`
   (or its dry-run if available). Prefer an isolated `HOME` copy; if not possible,
   the pre-step backup is the safety net.
3. *Re-snapshot + classify* gentle-ai's behavior toward UNMANAGED files into one of:
   - **append-only / replace-catalog-only** — sentinel + GADU survive untouched
     (hash + mtime unchanged). Predicted by spot-check finding 2.
   - **clear-and-rewrite** — sentinel and/or GADU deleted.
   - **name-collision overwrite** — only catalog names change (GADU/sentinel names
     do not collide, so safe).

**Guard sizing from E1 outcome.**

- *append-only / replace-catalog-only (expected):* the MINIMAL guard is the
  existing apply mechanism extended to the agent route — `overlay apply`
  re-deploys `agents/GADU.md`, and `overlay sync-check`/`apply` after a sync
  restores any drift. NO pre-sync backup, NO new destructive-protection code.
  Document durability as "reproducibility + re-apply." This satisfies R-008.
- *clear-and-rewrite (worst case only):* add ONE minimal safeguard — a pre-sync
  copy of `agents/GADU.md` into the overlay's managed backup dir, plus the
  documented "run `overlay apply` after every `gentle-ai sync`/`upgrade`."
  Re-apply remains the core recovery; the backup is the only addition.

**Rationale.** F1 is explicit that the guard must be evidence-based and minimal.
The live coexistence already indicates we likely need NOTHING beyond re-apply;
E1 confirms that before we spend code, so we neither over-engineer a backup we
don't need nor under-protect if gentle-ai actually clears the dir. AC-5
(simulated sync → reapply → both artifacts present + managed) is the regression
test regardless of E1's branch.

**Rejected.** Filesystem immutability / chattr locks: hostile to the user's own
edits and to gentle-ai's legitimate catalog writes. A blanket pre-sync backup
unconditionally: wasteful if E1 shows append-only behavior.

### D5 — cmd_capture: narrow for GADU, route-aware for correctness (resolves F5)

**Decision.** GADU's agent file and skill are `custom` GENERATED artifacts with NO
upstream pristine version, so capture must NOT reach them. Because both rows are
`custom`, the `managed_files()`-driven capture loop already excludes them — for
GADU, capture is a no-op BY DESIGN. We still make capture's source/dest/tar_path
go through `route_resolve` so the agent route is honored (R-007) and a
hypothetical future MANAGED agent file would resolve correctly instead of being
mis-routed to `skills/`.

**Rationale.** Capturing a generated file from a live target is backwards: it
would risk pulling a hand-edited or clobbered live copy back into the repo,
breaking the DRY guarantee (the canonical source + generator is the truth, not
the deployed copy). Narrowing the capture requirement for GADU is therefore
correct, not a gap. The route-awareness keeps the single-helper invariant whole.

**Rejected.** Extend capture to pull GADU from `~/.claude/agents`: inverts the
source of truth and reintroduces drift.

### D6 — cmd_bootstrap: add to the route-aware set (resolves F6)

> **Status: RATIFIED by the user.** `cmd_bootstrap` is an APPROVED FIFTH route-aware
> function — installer surgery spans cmd_apply, cmd_capture, cmd_status,
> cmd_sync_check, cmd_bootstrap — and is no longer treated as a deviation from the
> locked set. **Git-add scope:** bootstrap MUST run `git add agents/` in addition to
> the existing `git add skills/` (L199 upstream baseline, L226 overlay commit) so the
> generated agent file is committed on both the upstream-baseline and overlay commits.

**Decision.** Add `cmd_bootstrap` to the route-aware function set. Its
layer-custom loop (iterating `parse_manifest`) must resolve the agent row through
`route_resolve` so a fresh-machine bootstrap sources the agent file from
`~/.claude/agents/GADU.md` (not `~/.claude/skills/`) and writes it to
`agents/GADU.md` in the repo (not `skills/GADU.md`). The tarball-extract loop
(`managed_files()`) is a no-op for GADU (custom). A MISSING live source is benign:
the committed, generated `agents/GADU.md` already in the repo stands (the existing
"source not found, skipping" warn path).

**Rationale.** This is a CORRECTION, not scope creep. The proposal's own "Why" and
the engram record already list `cmd_bootstrap L186` among the hardcoded `skills/`
sites; locked decision 4 under-named the function set. Without this, a fresh
machine bootstrapped before the agent file is generated would mis-handle the
agent row (wrong source dir, wrong repo dir). Because GADU's repo artifacts are
generated and committed, bootstrap's real job is reconstruction, and routing the
agent row correctly (with missing-source treated as benign) closes F6.

**Rejected.** Leave bootstrap skills-only: a fresh machine silently mis-places or
warns on the agent row — an avoidable latent bug.

### D7 — DRY split: persona body authored once, frontmatter is generator-owned (R-001/R-002)

**Decision.** The canonical source is ONLY the persona BODY (everything from the
six traits through Memory). The two frontmatters are small generator-owned
TEMPLATES, not persona content:

- Agent (`agents/GADU.md`): `name: GADU`, `description: …`, `model: opus`,
  `tools: '*'` — matching the existing hand-made file (A-2, R-004), invocable as
  `@GADU`.
- Skill (`skills/gadu-operator/SKILL.md`): standard skill frontmatter
  (`name: gadu-operator`, a `description` with a `Trigger:` line, license/metadata)
  plus a short "load into a native subagent on demand" preamble (R-005, locked
  decision 2). NOT auto-spawned (R-009).

Immediately after each file's closing frontmatter, the generator writes a
generated-file header comment:
`<!-- GENERATED — DO NOT EDIT. Source: engine/gadu/persona/body.md. Run: gentle-ai-overlay gadu-generate -->`.
The same body bytes appear in both outputs.

**Rationale.** Frontmatter legitimately differs per delivery shape; treating it as
template (not authored content) keeps the persona body single-authored, so the
two outputs cannot drift (R-002). The header makes hand-edits obvious and is
asserted by the generator test.

### D8 — Test strategy: one real runner for Go + bash (resolves F2 for the installer)

**Decision.** ALL new automated tests live under `engine/` so the canonical
`go test ./...` runs them:

- `engine/gadu/gadu_test.go` (pure Go, fast): both artifacts produced; both carry
  the identical persona body; agent frontmatter has `name/description/model: opus/
  tools`; skill frontmatter is a valid skill; both carry the generated header;
  STALENESS — `Check` against the committed files fails if missing/stale/divergent
  (R-003/AC-2). Use `t.TempDir()` for emission; treat committed files as the
  golden.
- `engine/installer/route_test.go`:
  - *Unit* (fast): `exec.Command("bash","-c", …)` sources `route_resolve` from
    `bin/labdrian-overlay` and asserts the emitted record for: a legacy bare skill
    row → 3 skills dests (back-compat invariant from D2); the GADU skill row → 3
    skills dests; the GADU agent row → exactly `claude:~/.claude/agents/GADU.md`.
  - *Target-flag unit* (fast, AC-4): intersect `route_resolve`'s emitted target set
    with a simulated `--target` selection and assert **`--target opencode` on the GADU
    agent row yields ZERO applicable targets (no work)** — the agent route is
    claude-only — while `--target claude` on that row yields exactly
    `claude:~/.claude/agents/GADU.md`, and `--target opencode` on a skill row still
    yields the single opencode skills dest. This pins the call-site `--target`
    intersection rule from D2 so the flag can never mis-deploy an agent row to a
    skills runtime.
  - *Integration* (`testing.Short()`-skippable): run `bin/labdrian-overlay
    apply|status|sync-check` against a `t.TempDir()` sandbox `HOME` + a fixture
    manifest and fixture `skills/`/`agents/` source dirs; assert the agent file
    lands ONLY in `<HOME>/.claude/agents`, skills land in all three skills dirs,
    and an unrelated skill still deploys unchanged (AC-3/AC-4).

The SDD spec writes acceptance criteria runner-AGNOSTICALLY (assert observable
files/outputs); this design simply guarantees every AC has a Go test that proves
it regardless of whether the producing code is Go (generator) or bash (installer).

**Rationale.** `gate_e2e_test.go` is the established precedent for encoding
externally-verified behavior in a Go test. Shelling to the bash helper/installer
from Go gives Strict TDD a single real runner across both languages with no new
harness, and the `go-testing` skill explicitly sanctions `t.TempDir()` +
`-short`-skippable external-command tests.

**Rejected.** A separate bats suite for bash: adds a runner the canonical command
doesn't invoke, so Strict TDD wouldn't actually gate it.

## 4. Data flow (apply, end to end)

1. Dev edits `engine/gadu/persona/body.md` → runs `gadu-generate` → regenerates
   `agents/GADU.md` + `skills/gadu-operator/SKILL.md` → commits. `go test ./...`
   fails the build if the committed artifacts are stale (D1/D8).
2. `overlay apply` checks out `main`, merges `upstream`, then for each tracked
   row calls `route_resolve` (D2): the skill row deploys to the 3 skills dirs; the
   agent row deploys to `~/.claude/agents/GADU.md` only.
3. `overlay status`/`sync-check` report both rows under their routes (L444/L500
   now route-derived).
4. After a `gentle-ai sync`/`upgrade`, `overlay sync-check` flags any drift and
   `overlay apply` re-deploys — the durability path (D4), validated by AC-5.

## 5. Requirements traceability

| Requirement | Satisfied by |
|---|---|
| R-001 single source | D1, D7 (`engine/gadu/persona/body.md`) |
| R-002 both shapes, no drift | D1, D7 (shared body, golden test) |
| R-003 generator test | D1, D8 (`gadu_test.go` staleness) |
| R-004 valid agent frontmatter | D7 (agent template) |
| R-005 skill loadable on demand | D7 (skill template + preamble) |
| R-006 agent vs skill routing | D2, D3 |
| R-007 honored in apply/capture/status/sync-check + manifest | D2 (call sites), D3, D5 |
| R-008 survive sync/upgrade | D4 (re-apply + E1) |
| R-009 no auto-spawn / no native agent files | D7 (skill preamble), scope |
| AC-1..AC-6 | D1/D7 (1,2), D8 (2,3,4), D4 (5), scope (6) |

### Locked-decision traceability

| Locked decision | Design decision(s) |
|---|---|
| LD1 — ONE canonical persona source | D1 (Go generator from `engine/gadu/persona/body.md`), D7 (body authored once; frontmatter is generator-owned template) |
| LD2 — persona AVAILABLE TO INVOKE in each runtime | D2 (`route` column + `route_resolve` deploy to agent/skill destinations) + D7 (agent & skill frontmatter templates: `@GADU` agent, on-demand skill) |
| LD3 — SURVIVE `gentle-ai sync`/`upgrade` | D4 (reproducibility + re-apply, guard sized by E1) |
| LD4 — NO SDD auto-spawn / no native agent-definition files | D7 (skill preamble: load on demand, not auto-spawned) + scope (R-009: no opencode/Codex agent files) |

## 6. Open items handed to later phases

- E1 (durability probe) is the MANDATORY FIRST STEP of `sdd-apply`: it MUST run and
  its classification be documented BEFORE any durability-guard code is written. The
  D4 guard branch (append-only → re-apply only; clear-and-rewrite → add pre-sync
  backup) is selected STRICTLY from E1's documented result — implementing any guard
  before E1 is recorded is a process violation.
- `sdd-tasks` should expect >400 changed lines (canonical body + generator + tests
  + two generated artifacts + five-function installer surgery + manifest), which
  triggers the chained/stacked-PR decision. Natural slice boundary: (slice 1)
  canonical source + generator + its test + two generated artifacts;
  (slice 2) installer route helper + five call sites + manifest + installer tests.
