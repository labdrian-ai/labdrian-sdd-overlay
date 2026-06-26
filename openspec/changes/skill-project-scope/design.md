# Technical Design: Project-Scoped Skill Install (issue #29, slice 2)

One paragraph: Slice 1 shipped a read-only registry + `skills list/status/validate` with
`defaultScope` hard-locked to `global`. This slice adds a **project** scope and a new
`skills install` verb that, run **from inside a target repo**, copies allowed project-scoped
skills from the overlay's `skills/<id>/` tree into `<repo>/.claude/skills/<id>/` — the
filesystem boundary Claude Code uses for project-local skill discovery (verified, engram
`skill-package-manager/project-scope-feasibility`). The global deploy pipeline
(`cmd_apply`/`cmd_capture`) is untouched; everything here is additive.

## Quick path (what gets built)

1. **Schema**: `Install.DefaultScope` becomes a typed enum (`global` | `project`); add
   `Install.AllowedProjects []string`. Relax `parse.go` validation to accept `project`,
   parse `allowedProjects`, and fail loud on any other scope value.
2. **Planner (pure)**: `PlanInstall(reg, projectID, sourceRoot, targetRoot) ([]CopyOp, error)` —
   no I/O, table-testable. Filters to project-scoped + allowed entries; returns `{src,dst}` pairs.
3. **Executor (I/O)**: `ExecuteInstall(plan) error` — recursive directory copy; `t.TempDir()` golden tests.
4. **CLI**: `install` verb in `SkillsCore`; `cmd_skills` passthrough injects `--source-root`
   (= `$OVERLAY_DIR/skills`) so the engine binary at `~/.claude/bin` finds the source from any cwd.
5. **Registry**: at least one `project`-scoped entry to exercise the path end to end.

`bin/overlay` is NEVER touched. `cmd_apply`/`cmd_capture` are NEVER touched.

---

## Architecture context (binding constraints)

| Constraint | Source | Consequence for this slice |
|---|---|---|
| Nothing labdrian-owned may live in an upstream-owned file | `architecture/overlay-gentle-ai-separation` | All edits land in `engine/skills/*`, `engine/cmd/main.go` (100% labdrian), `bin/labdrian-overlay`, `skills.registry.yaml`. `bin/overlay` stays byte-identical. |
| Engine is zero-dependency; registry parsed by a strict hand-rolled YAML subset | slice-1 ADR-1 | No new deps. New `allowedProjects` field parsed by the existing tokenizer (block scalar sequence, same shape as `targets`). |
| Engine core is pure; all I/O injected | slice-1 ADR-2 | Planner is pure; executor isolates FS writes; both unit-tested without a real `$HOME`. |
| Filesystem location IS the scoping mechanism | `skill-package-manager/project-scope-feasibility` | "Install" = a recursive directory copy into `<repo>/.claude/skills/`. No runtime/registry indirection needed. |
| The engine binary runs from `~/.claude/bin`, not the overlay repo | `bin/labdrian-overlay:25` | The engine cannot assume its cwd is the overlay. The overlay source root must be passed IN (flag), mirroring how `cmd_skills` already injects `--registry`/`--manifest`. |

---

## ADR-1 (KEY): Install runs FROM the target repo (Option A), source root passed by flag

**Decision.** Adopt **Option A**. `labdrian skills install` is run with the **target repo as cwd**.
The engine resolves:
- **target root** = the current working directory (`os.Getwd()`), copying into `<cwd>/.claude/skills/<id>/`;
- **source root** = the overlay's `skills/` directory, supplied via a new `--source-root` flag that
  `cmd_skills` defaults to `$OVERLAY_DIR/skills` (same default-injection mechanism already used for
  `--registry`/`--manifest`).

**Why A over B.**

| Dimension | A — from-project (chosen) | B — global apply + project-path registry |
|---|---|---|
| Name→path mapping | None needed; cwd *is* the project | Requires a `projectId → absolute path` map maintained somewhere |
| Coupling to global pipeline | Zero; new verb is isolated | Couples into `cmd_apply`, the exact file we must not destabilize |
| Offline / portable | Yes; pure FS + cwd | Map can go stale when repos move |
| Matches verified model | Yes — "deploy into `<repo>/.claude/skills`" is literally a copy into cwd | Indirect |
| Slice-1 invariant ("apply untouched") | Preserved | Violated |

Option B's only advantage (install many projects from one place) is a non-goal this slice and
reintroduces the brittle name→path registry the proposal explicitly set out to avoid.

**How the engine learns the source root (concrete).** The binary is at
`~/.claude/bin/gentle-ai-overlay`; it cannot infer the overlay location from its own path or cwd.
`cmd_skills` already injects defaults for flags the caller didn't pass. We extend that block:

```bash
# bin/labdrian-overlay cmd_skills (additive; same shape as --registry/--manifest)
[[ "$has_source_root" -eq 0 ]] && defaults+=("--source-root" "$OVERLAY_DIR/skills")
```

This keeps the engine core pure (no `os.Getenv` inside `engine/skills/`): the bash layer owns
environment resolution, the engine receives explicit paths. `$OVERLAY_DIR` is already computed at
the top of the script (`bin/labdrian-overlay:13`), so no new discovery logic is required.

**Rejected alternative — read `OVERLAY_DIR` env directly in Go.** Would put environment knowledge
inside the otherwise-pure core, break the "all I/O/config injected" invariant (ADR-2 of slice 1),
and make the planner harder to table-test. The flag default in bash gives the identical result with
no purity cost.

---

## ADR-2: Project identity = cwd directory basename (offline), git-remote deferred

**Decision.** The `projectId` matched against `allowedProjects` is the **basename of the target
repo's root directory** (e.g. cwd `/home/x/labdrian-sdd-overlay` → `labdrian-sdd-overlay`). It is
computed in the bash/CLI boundary from cwd and passed to the pure planner as a string.

**Why basename.**
- **Offline and dependency-free** — no `git` invocation, consistent with the zero-dep ethos.
- **Deterministic and table-testable** — the planner takes a plain `projectId string`; tests pass
  any value with no git fixture.
- **Good enough for an explicit allow-list** — `allowedProjects` is an opt-in list the skill author
  controls; matching a human-chosen repo folder name is predictable.

**Tradeoff acknowledged.** Basename collides if two different repos share a folder name, and changes
if a user renames the checkout. A **git-remote identity** (`origin` URL `owner/repo` slug) is more
robust against renames and is the natural upgrade.

**Mitigation / escape hatch.** Keep identity resolution at the boundary, not in the planner, so the
upgrade is local. Provide an explicit override flag `--project-id <name>` (also used by tests) that
short-circuits basename derivation. A future slice can add git-remote derivation behind the same
override without touching planner signatures. **Non-goal this slice:** automatic git-remote derivation.

**Where derived.** `RenderInstallCore` calls `os.Getwd()`, takes `filepath.Base`, unless
`--project-id` was supplied. The planner never sees cwd.

---

## ADR-3: Pure planner + I/O executor split

Mirrors slice-1's testable-core discipline. New file `engine/skills/install.go`.

**Planner (pure, no I/O):**

```go
// CopyOp is one source→destination directory copy in an install plan.
type CopyOp struct {
    SkillID string // entry id, for messages
    Src     string // <sourceRoot>/<path>      (overlay skills/<id>/)
    Dst     string // <targetRoot>/.claude/skills/<id>/
}

// PlanInstall is PURE: given a parsed registry, the resolved project id, the overlay
// source root, and the target repo root, it returns the ordered copy plan.
// It performs NO filesystem access. Deterministic order (sorted by SkillID) for goldens.
func PlanInstall(reg Registry, projectID, sourceRoot, targetRoot string) ([]CopyOp, error)
```

Planner responsibilities (all pure, all unit-tested):
1. Select entries with `Install.DefaultScope == "project"`.
2. Keep only entries whose `AllowedProjects` contains `projectID` (exact match).
3. Build `Src = filepath.Join(sourceRoot, entry.Path)` and
   `Dst = filepath.Join(targetRoot, ".claude", "skills", entry.ID)`.
4. Sort by `SkillID`; return.
5. Fail loud (typed errors) when the *named* project matches an entry whose `AllowedProjects` is
   empty/absent (see ADR-4 rule), or when no project-scoped entry admits the project (empty plan →
   caller decides; see below).

**Executor (I/O only):**

```go
// ExecuteInstall performs the filesystem side of an install plan.
// It verifies each Src exists (fail loud on missing source dir, R), then recursively
// copies Src into Dst. Tested with t.TempDir().
func ExecuteInstall(plan []CopyOp) error
```

The executor is the *only* place that touches the filesystem, so `t.TempDir()` golden tests fully
cover it without mocks. The planner needs no filesystem at all.

**Empty plan semantics.** An empty plan (no project skill admits this repo) is **not** an error by
itself; `RenderInstallCore` prints an informational "no project-scoped skills allowed for
'<projectId>'" and exits 0. Fail-loud is reserved for *malformed intent* (unknown verb, missing
source dir, a non-project skill targeted explicitly), per the proposal's success criteria.

---

## ADR-4: Schema changes and fail-loud rules

**Types (`engine/skills/types.go`):**

```go
type Install struct {
    DefaultScope    string   // "global" | "project"  (enum; validated in parse.go)
    Targets         []string // unchanged
    AllowedProjects []string // NEW; required & non-empty when DefaultScope == "project"
}
```

`DefaultScope` stays a `string` field but is treated as a closed enum by the validator (consistent
with how `Source.Type` and `Lifecycle.UpdateStrategy` are already validated via maps). No new Go
enum type is introduced — it would add ceremony without buying type safety across the YAML boundary,
where the value arrives as a string anyway.

**Parser (`engine/skills/parse.go`):**
- In `parseInstall`, add a `case "allowedProjects"` that calls the existing `parseScalarSequence`
  (identical shape to `targets`) → `inst.AllowedProjects`.
- In `validateEntry`, replace the hard lock at line ~571:

| Old | New |
|---|---|
| `DefaultScope != "global"` → error | `validScopes = {global, project}`; reject anything else, line-numbered in `parseInstall` |
| (none) | `DefaultScope == "global"` ⇒ `AllowedProjects` MUST be empty (cross-scope misuse fails loud, in `validateEntry`) |
| (none) | `DefaultScope == "project"` with absent/empty `allowedProjects` → VALID, entry is simply never installable (R-041/SC-13); not an error |

**Fail-loud matrix (proposal success criteria → enforcement point):**

| Condition | Where | Result |
|---|---|---|
| Unknown scope value (`workspace`, typo) | `parseInstall` (line-numbered) | exit 1, line number + invalid value |
| `global` scope with non-empty `allowedProjects` | `validateEntry` | exit 1 |
| Source dir `<sourceRoot>/<path>` missing | `ExecuteInstall` | exit 1, names the skill + path |
| Install requested but no project skill admits cwd | `RenderInstallCore` | exit 0, informational |
| Not a recognizable repo cwd (optional guard) | `RenderInstallCore` | exit 1 (see ADR-2 override) |

All slice-1 global entries remain valid: they keep `defaultScope: global` with no `allowedProjects`,
which the new rules accept unchanged. Strict TDD keeps every slice-1 golden green.

---

## ADR-5: Copy semantics — clean overwrite, predictable and idempotent

**Decision.** Re-install is a **clean overwrite**: for each `CopyOp`, the executor removes `Dst`
(if present) and recursively copies the entire `Src` tree fresh. Recursive copy includes
`SKILL.md` plus any `references/`, examples, and nested files under `skills/<id>/`.

**Why clean overwrite (remove-then-copy) over merge-copy.**
- **Idempotent and deterministic** — the installed tree is always exactly the overlay source; no
  orphan files survive when the source drops a reference file. A plain merge-copy would leave stale
  files behind on re-install.
- **Registry is the single source of truth** — a project-installed skill is a *deployed artifact*,
  not an editing surface. The overlay `skills/<id>/` is canonical.

**User-edited project skill on re-install.** It is **overwritten**. This is the safe-by-clarity
choice given uninstall is deferred:
- *Why not skip-if-exists*: silently diverges installed content from the registry; the user thinks
  they updated, but got nothing.
- *Why not error-if-modified*: requires content hashing/state we don't keep this slice, and blocks
  the happy path.
- *Mitigation*: document loudly that `<repo>/.claude/skills/<id>/` is overlay-managed and not an
  edit target; the executor prints each overwritten skill id. Edits belong in the overlay repo
  `skills/<id>/`, then re-installed. A future slice may add a `--no-overwrite` / hash guard.

**Copy implementation.** A small internal `copyTree(src, dst string) error` using
`filepath.WalkDir` + `os.MkdirAll` + `io.Copy`, preserving file mode bits. No symlink following
this slice (overlay skills are plain files/dirs). Remove-then-copy via `os.RemoveAll(dst)` before
the walk.

---

## ADR-6: CLI wiring

**Engine core (`engine/skills/skills.go`):** add one case.

```go
case "install":
    RenderInstallCore(args, readFile, stdout, stderr, exit)
```

Update the empty/unknown-verb messages to list `install`. `RenderInstallCore` (in `install.go`):
1. Parse flags: `--registry` (default `skills.registry.yaml`), `--source-root`, `--target-root`
   (default cwd), `--project-id` (default `filepath.Base(cwd)`).
2. `readFile(registry)` → `ParseRegistry` (fail loud on parse error, like `RenderValidateCore`).
3. `PlanInstall(reg, projectID, sourceRoot, targetRoot)`.
4. `ExecuteInstall(plan)`; print one line per copied skill; exit 0, or fail loud per ADR-4.

`main.go` needs **no change** — `runSkillsCore` already forwards every verb into
`skills.SkillsCore`; `install` is just another case. (The verb-empty guard in `main.go:163`
already covers the missing-verb path.)

**Bash passthrough (`bin/labdrian-overlay cmd_skills`):** add `--source-root` default injection
only (mirrors `--registry`/`--manifest`). `install` runs in the user's cwd, so NO `cd` — that is the
whole point of Option A. `--target-root`/`--project-id` are left unset so the engine derives them
from cwd; tests pass them explicitly.

```bash
local has_source_root=0
for arg in "$@"; do
  ...
  [[ "$arg" == "--source-root" ]] && has_source_root=1
done
[[ "$has_source_root" -eq 0 ]] && defaults+=("--source-root" "$OVERLAY_DIR/skills")
```

`bin/overlay`, `cmd_apply`, `cmd_capture` are untouched. The dispatch line
`skills) cmd_skills "$@" ;;` already routes the verb.

---

## Component & data-flow map

```
user (cwd = target repo) ── labdrian skills install ──► cmd_skills (bash)
                                                          │ injects --source-root=$OVERLAY_DIR/skills
                                                          │ (no cd; cwd stays = target repo)
                                                          ▼
                          ~/.claude/bin/gentle-ai-overlay skills install --source-root … --registry …
                                                          ▼
                                      main.go runSkillsCore → SkillsCore("install", …)
                                                          ▼
                                              RenderInstallCore (install.go)
                          ┌───────────────────────────────┼───────────────────────────────┐
                          ▼                                ▼                                ▼
                  ParseRegistry (parse.go)         PlanInstall (PURE)              ExecuteInstall (I/O)
                  + relaxed scope validation   filter project+allowed → []CopyOp   removeAll+copyTree
                                                                                   into <cwd>/.claude/skills/<id>/
```

Data shapes crossing boundaries:
- bash → engine: argv flags only (`--source-root`, `--registry`, optional `--target-root`/`--project-id`).
- registry file → `Registry` (now with `AllowedProjects`).
- planner → executor: `[]CopyOp{SkillID, Src, Dst}` (pure value, the golden-test seam).

---

## Test matrix (strict TDD)

| Target | Pattern | Key cases |
|---|---|---|
| `validateEntry` scope rules | table-driven (`parse_test.go`) | accept `global` (no allowed); accept `project` (with allowed); reject unknown scope; reject `project` empty-allowed; reject `global` with allowed; all slice-1 goldens still pass |
| `parseInstall` allowedProjects | table-driven | single/multi entries; absent field → nil; duplicate-key guard still fires |
| `PlanInstall` | table-driven (pure) | project skill admits id → 1 op; not in allowed → skipped; global skills ignored; multiple admit → sorted; empty result; correct Src/Dst join (`.claude/skills/<id>`) |
| `ExecuteInstall` | `t.TempDir()` golden | fresh install copies SKILL.md + references/; re-install overwrites (stale file in dst removed); missing Src dir → fail loud; mode bits preserved |
| `RenderInstallCore` | core test w/ injected writers + TempDir | flag parsing; `--project-id` override; informational empty-plan exit 0; parse-error exit 1 |
| `cmd_skills` install forward | `bash -n` + e2e | `--source-root` injected when absent, not overridden when present; no `cd`; dispatch reaches engine |

Golden determinism: planner output sorted by `SkillID`; executor walks sorted dir entries.

---

## Affected files

| File | Change |
|---|---|
| `engine/skills/types.go` | `Install.AllowedProjects []string` |
| `engine/skills/parse.go` | `parseInstall` case `allowedProjects`; relax scope lock (~L571) + new fail-loud rules |
| `engine/skills/install.go` | **NEW** — `CopyOp`, `PlanInstall` (pure), `ExecuteInstall` + `copyTree` (I/O), `RenderInstallCore` |
| `engine/skills/skills.go` | dispatch `install`; update verb-list messages |
| `engine/skills/*_test.go` | new tables + TempDir goldens |
| `bin/labdrian-overlay` | `cmd_skills`: inject `--source-root` default (additive) |
| `skills.registry.yaml` | ≥1 `project`-scoped entry with `allowedProjects` |
| `engine/cmd/main.go` | none (verb auto-forwards) |

---

## Risks & alternatives

| Risk | Likelihood | Mitigation |
|---|---|---|
| Scope relax breaks global validation | Low | TDD: keep all slice-1 golden cases green; new rules are strictly additive for `global` entries |
| Install copies into wrong cwd | Med | Target = `os.Getwd()`/`<cwd>/.claude/skills`; optional repo-sanity guard; `--target-root` override only for tests; no `cd` in bash |
| Overwriting a user-edited project skill | Med | Clean overwrite documented loudly; installed tree declared overlay-managed; uninstall + `--no-overwrite` deferred to next slice |
| Basename collision / repo rename breaks match | Low–Med | `--project-id` escape hatch; git-remote identity is the documented upgrade behind the same seam |
| opencode/codex project paths assumed | Med | Out of scope; `.claude/skills/` only this slice (only Claude path verified) |
| Engine purity erosion (env in core) | Low | Source root injected as a flag by bash, never read from env in `engine/skills/` |

**Non-goals (explicit):** opencode/codex project-local install paths; uninstall/cleanup of
project-installed skills; `skills add`/`remove`; external sources / pinned refs; manifest generation
from registry; any change to global `cmd_apply`/`cmd_capture`; automatic git-remote project identity.

---

## Checklist (reviewer can confirm)

- [ ] `Install.AllowedProjects` added; `DefaultScope` validated as `{global, project}`
- [ ] Parser accepts `project` + `allowedProjects`; fails loud on unknown scope and scope/allowed mismatch
- [ ] `PlanInstall` is pure (no FS); `ExecuteInstall` is the only FS writer
- [ ] Re-install overwrites cleanly (no stale files); missing source dir fails loud
- [ ] `cmd_skills` injects `--source-root=$OVERLAY_DIR/skills`; no `cd`; `bin/overlay` byte-identical
- [ ] `cmd_apply`/`cmd_capture` unchanged; all slice-1 goldens green
