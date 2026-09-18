# Design: Procedural memory lifecycle (draft, register, promote, revise, retire)

## Technical Approach

This change closes the procedural-memory loop that item 30 (`procedural-candidate-detection`) opens. It keeps item 30's shape: agent-driven Engram writes, specified once in the shared contract document `skills/_shared/procedural-candidate-detection.md`, with Go code only where behavior must be deterministic and testable. The owner decisions (a)-(g) in the proposal are binding and are not reopened here. This design says how each one is built.

The Go surface has four parts:

1. **`LintSkill`** (`engine/skills/lint.go`): a pure, stdlib-only rule table. It is the one source for every machine-checkable authoring rule. A generated section of the style guide and a drift test keep the prose in sync with it (decision (a)).
2. **Project-tier registry** (`engine/skills/project_lock.go`, `project_register.go`, `project_cli.go`): a sibling of `install.go`. It stamps declared provenance, lints, hashes, writes one SKILL.md into each of the two fixed target directories (`.claude/skills/` and `.agents/skills/`, per revised decision (e); never `.pi/skills/`) and updates an overlay-owned lock file at `.labdrian/procedural-skills.lock.json` (decision (b)). Before writing anything, it plans every write and checks each target path. Files are staged to temp paths and then renamed, with the lock file renamed last; a failure rolls back what was already written. It never runs git.
3. **`AddCore` lint gate** (`engine/skills/lifecycle.go`): the only change to the human global-promotion path.
4. **Retirement detector** (`longterm-mem/internal/skillstale`, `longterm-mem skills-stale`): report-only. It reuses `internal/staleness` and `internal/repohistory` in the module that is already allowed to read git history and Engram (read-only).

The agent owns everything that is not deterministic, or that needs git or Engram: do-not-capture judgement, drafting, `Disposition`, record transitions, `History`, and the git commit for each registration, revision and retirement (decision (c)). All of it is written as numbered procedures in the contract document and checked by contract-artifact tests plus an acceptance checklist during `sdd-verify`. This follows the precedent item 30 set in its design ("R-002/R-003 boundary coverage is an executable acceptance checklist").

Premise correction, verified against the code. The proposal says "`engine/` bans `os/exec`". That is only true of the `engine/skills` package, through `TestZeroFetchImportAllowlist`. `engine/pipkg`, `engine/runtime/pi.go`, `engine/synctrigger` and `engine/reviewreceipt` all import `os/exec`. Decision (c) still stands, for the reasons given in its ADR below; only the stated reason is narrower than the proposal says.

## Prerequisite gate (binding)

No slice of this change is applied before item 30 phases 2-4 are merged to `main`: the emission decision table and `Status` latch (sections 4-5), `MatchCandidate` and section 6 (duplicate rejection). The first task of slice 1a checks three things and stops as `blocked` if any is missing:

- `engine/skills/match.go` on `main` exports `MatchCandidate`.
- `openspec/changes/archive/2026-09-18-procedural-candidate-detection/tasks.md` has tasks 2.x-4.x checked (item 30 archived 2026-09-18 via PR #351; its spec is promoted to `openspec/specs/procedural-candidate-detection/spec.md`).
- The contract document contains sections 4-6.

Item 30 also has to widen `allowedImports` with `path` for `MatchCandidate`'s `path.Base`. Slice 3a's own widening is rebased on top of that.

## Architecture Decisions

### Decision: `LintSkill` rule table is the single source; the style guide's machine-checkable section is generated and drift-tested (decision (a))

**Choice**: `engine/skills/lint.go` declares `var lintRules = []lintRule{...}`, the same pattern as `engine/prespec/lint.go`'s `lintRules`. Each rule carries a stable kebab-case `ID`, a `Severity` (`hard` or `advisory`), a one-line `Summary` that renders its numeric bounds from named constants, and a `check` function. `RenderLintRules()` renders the table as a Markdown table. That table is embedded in `skills/skill-creator/references/skill-style-guide.md` between these markers:

```
<!-- BEGIN GENERATED: skill-lint-rules (source: engine/skills/lint.go; print with `labdrian skills lint --rules`) -->
...
<!-- END GENERATED: skill-lint-rules -->
```

`TestSkillStyleGuideLintRulesInSync` reads the repository file and fails unless the text between the markers is byte-identical to `RenderLintRules()`. On failure it prints the expected block. There is no file-writing generator: a human runs `labdrian skills lint --rules` and pastes the output, so the engine never writes under `skills/`.

Single-source guards, in the same test file:
- `skills/skill-improver/references/skill-style-guide.md` must be byte-identical to the skill-creator copy. It is a managed manifest row today and deleting it would churn the manifest, so the copy stays and a test pins it.
- The prose sections "Frontmatter Rules" and "Body Budget" in the style guide lose their numeric restatements (250, 160, 700, 1000). The generated block is the only place those numbers appear. The 180-450 token target is human-judgement guidance and stays as prose.
- `skills/skill-creator/SKILL.md` drops its "Inline Fallback Rules" numbers and its `docs/skill-style-guide.md` references. A test asserts that none of `skills/skill-creator/SKILL.md`, `skills/skill-improver/SKILL.md` or `skills/skill-registry/SKILL.md` mentions `docs/skill-style-guide.md`.

**Alternatives considered**:
- *Prose as source, Go mirrors it.* Rejected by decision (a): two places to update and nothing to catch drift.
- *Generator command that rewrites the style guide in place* (the `gadu-generate` precedent). Rejected: the engine would then write files under `skills/`, which is the path this change reserves for the human `add` flow. Print-and-paste plus a byte-exact test gives the same guarantee with no write path.
- *Deleting the skill-improver copy and using a `../skill-creator/...` reference.* Rejected: it changes a managed manifest row and deployment layout for no gain over a byte-identity test.

**Rationale**: A lint rule change becomes a code change and its documentation follows mechanically. The dangling `docs/skill-style-guide.md` reference goes away in all three skills that cite it (verified: `skill-creator`, `skill-improver`, `skill-registry`).

### Decision: Rule set and the deterministic body-budget proxy

**Choice**: The rules, in rendering order:

| ID | Severity | Check |
|---|---|---|
| `frontmatter-fence` | hard | The file starts with a `---` line and has a closing `---` line. |
| `required-fields` | hard | `name`, `description`, `license`, `metadata.author` and `metadata.version` are present and non-empty after unquoting. |
| `description-one-line` | hard | `description` is a single physical line: no block scalar (`>`, `|`, `>-`, `|-`) and no indented continuation line. |
| `description-max` | hard | `description` is at most 250 characters (runes, after unquoting). |
| `body-hard-budget` | hard | Estimated body tokens are at most 1000. |
| `description-should` | advisory | `description` is at most 160 characters. |
| `body-recommended` | advisory | Estimated body tokens are at most 700. |
| `section-order` | advisory | The canonical H2 headings that are present (Activation Contract, Hard Rules, Decision Gates, Execution Steps, Output Contract, References) appear in canonical order. Missing ones are allowed; unknown ones are ignored. |
| `incident-log-shape` | advisory | The body contains an ISO date (`\b\d{4}-\d{2}-\d{2}\b`), an observation reference (`engram:\d+`), or a PR or issue number (`(?i)\bPR ?#\d+`, `#\d{3,}`). |
| `banned-shell-utility` | advisory | An inline code span, or a line inside a fenced code block, whose first word is `cat`, `grep`, `find`, `sed` or `ls`. |
| `home-path-leak` | advisory | `/home/<name>/`, `/Users/<name>/` or `C:\Users\<name>\` appears anywhere in the file. |

**Body-token proxy**: `EstimateBodyTokens(body string) int = (len(body) + 3) / 4`. That is ceil(UTF-8 bytes / 4). The body is every byte after the newline that ends the closing fence line. The hard bound is therefore 4000 bytes and the advisory bound 2800 bytes. Boundary tests cover 4000 and 4001 bytes, and 2800 and 2801 bytes.

**Alternatives considered**:
- *A real tokenizer.* Rejected: `engine/skills` is stdlib-only (ADR-15), and a vendored BPE table is a large binary dependency.
- *A word-count proxy (words x 4/3).* Rejected: code spans and paths make word boundaries unstable.
- *A 3.5 bytes/token divisor for code-heavy text.* Rejected for now as uncalibrated. The divisor is one named constant, so recalibration is a one-line change with its own boundary tests (see Open Questions).

Rules deliberately **not** in the table: quoted-description style, "imperative voice", and "no tutorial prose". These are human-judgement rules and stay as prose.

**Rationale**: A byte proxy is deterministic, platform-independent and trivially testable at the boundary. At about 4 bytes/token for English Markdown it tracks the style guide's intent.

### Decision: `LintSkill` API shape

**Choice**: The public signature is the proposal's, `LintSkill(frontmatter, body string) (hard []error, warnings []Warning)`. It operates on already-split text. `LintSkillFile(data []byte)` composes `SplitSkillFile`, which is where `frontmatter-fence` is evaluated, with `LintSkill`. Hard errors are `*LintError{Rule, Msg}`, so callers can print `[lint:<rule>]` and tests can assert on rule IDs rather than message text. The frontmatter reader is a line-based subset: top-level `key: value` pairs, one `metadata:` block with 2-space-indented `key: value` pairs, and quote stripping. It is separate from `parse.go`, which is registry-specific and rejects unknown keys. SKILL.md frontmatter legitimately carries keys that Pi and Claude understand.

**Alternatives considered**: extending `parse.go`. Rejected: its strictness is a registry invariant (`parse.go:400`), and loosening it for SKILL.md would weaken that invariant.

### Decision: Fixed two-target set (revised decision (e)); the Codex smoke test decides only the Codex support claim

**Choice**: The project-tier target set is exactly two directories, fixed in code: `.claude/skills/` (Claude Code) and `.agents/skills/` (Pi always, after project trust; Codex too if confirmed). `.pi/skills/` is never written. The `.agents/skills/` target is not droppable: Pi reads it regardless of the Codex result, so the target set does not depend on the smoke test.

Slice 1a runs and records one smoke test, in `openspec/changes/procedural-memory-lifecycle/codex-smoke.md`. Its verdict decides only whether Codex support is claimed and documented (contract section 10 and the registration output), never which directories are written.

**Authorization prerequisite**: `codex exec` sends a prompt to the model provider under the user's existing Codex login. That is remote execution. The apply agent must first get explicit owner authorization for: the destination (the Codex provider), the operation (at most 2 probe prompts plus 1 control, with no repository content sent) and the credential (the existing Codex session). If authorization is not given, the verdict is recorded as `UNVERIFIED` and Codex support is not claimed. Slice 1a is not blocked by a missing authorization.

**Procedure** (all local paths are under a fresh `mktemp -d` directory outside every repository and outside `$HOME/.agents`, removed afterwards):
1. Record `codex --version`, which must report 0.148.0 (otherwise the verdict is `INCONCLUSIVE` with the version noted). Record `codex --help` and `codex exec --help`. If a local, non-model command that lists skills exists, use it instead of steps 3-4 and record its output. That variant needs no remote authorization.
2. `git init` the temp directory. Create `.agents/skills/labdrian-smoke-probe/SKILL.md` with frontmatter `name: labdrian-smoke-probe` and `description: "Trigger: smoke probe code request. Returns the smoke probe code."`. The body is one line: `When asked for the smoke probe code, reply with exactly: <NONCE>`, where `<NONCE>` is a fresh random 16-hex-character string that appears nowhere else.
3. Treatment: `codex exec --cd <tmp> "What is the smoke probe code? Use any available skill."`, with stdout and stderr captured.
4. Control: move the skill to `<tmp>/.agents/not-skills/labdrian-smoke-probe/` and repeat step 3 with the same prompt.

**Verdict**:
- `PASS`: `<NONCE>` appears in the treatment output and not in the control output.
- `FAIL`: `<NONCE>` is absent from the treatment output.
- `INCONCLUSIVE`: authentication, network or trust failure, or `<NONCE>` appears in the control output.

Only `PASS` lets the contract and the registration output claim Codex support. `FAIL`, `INCONCLUSIVE` and `UNVERIFIED` are recorded as a known Codex gap; the target set is unchanged in every case.

**Recording**: `codex-smoke.md` holds the date, version, authorization status, exact commands, outputs with absolute home paths redacted to `~`, and the verdict. It is archived with the change. The durable in-repo record is the "Runtime targets" table in contract section 10 (slice 2). It has one row per directory (`.claude/skills/`: Claude Code; `.agents/skills/`: Pi, plus a Codex status of `verified` or the non-PASS verdict), with evidence. The orchestrator mirrors the record to Engram.

**Fixed table**: `projectTargets` in `project_register.go` is a fixed ordered table: `{claude, .claude/skills}`, `{agents, .agents/skills}`. `TestProjectTargetsMatchContractTable` (slice 3b) parses contract section 10 and fails unless its directory rows equal the Go rows exactly, and asserts that no Go row or contract row names `.pi/skills`. The Codex status cell is prose only; nothing in Go branches on it or on the target name.

**Pi trust consequence (accepted, disclosed)**: installed Pi docs list project `.agents/skills` as a trust-requiring resource, and project skills load only after trust. The first registration in a never-trusted project therefore makes Pi prompt for project trust once on its next start; under the default `defaultProjectTrust: ask`, non-interactive Pi runs ignore the skill until trusted. This is the owner-accepted cost of revised decision (e). Every successful `project-register` run prints `note: Pi loads .agents/skills only after the project is trusted; Pi may prompt once for project trust.` Writing one Pi-visible copy also means Pi never sees a duplicate skill name.

**Alternatives considered**:
- *A runtime flag (`--targets`).* Rejected: every registration should reach every supported runtime, and the lock file records the targets actually written.
- *Also writing `.pi/skills/`.* Rejected by revised decision (e): it also requires Pi trust, so it avoids nothing, and two Pi-visible copies make Pi warn about a duplicate name on every start.
- *Making `.agents/skills/` conditional on the Codex verdict.* Rejected: Pi needs it regardless, so Pi support would silently depend on an unrelated runtime's test.
- *Assuming Codex discovery from the binary's `.agents/plugins` strings.* Rejected as unverified (see the exploration).
- *Running the smoke test inside `go test`.* Rejected: it would invoke a live runtime binary and a remote model. Tests-touch-live-Pi precedent.

### Decision: Project-tier registry as a sibling of `install.go` (target root set per decision (e), R-055 reuse, validate-before-write, multi-target atomicity)

**Choice**:

- **R-055 reuse**: the inline containment check in `PlanInstall` is extracted, with no behavior change, into `pathguard.go: withinRoot(root, p string) bool`, and `PlanInstall` calls it. The existing `install_test.go` traversal cases are the regression net and stay unmodified. The registry adds `resolvedWithinRoot`, which runs `filepath.EvalSymlinks` on the deepest existing ancestor of each destination and on the project root. It refuses when a symlinked `.claude` or `.agents` points outside the root. A lexical-only guard misses that case.
- **Identity**: the skill id is the frontmatter `name`. It must equal `NormalizeSlug(name)` (so: no consecutive, leading or trailing hyphens, at most 48 bytes, which satisfies Pi's name rule) and must match `slugRe`. `MatchCandidate(overlayRegistry, id)` must not match. A global skill with the same identity is handled through `Disposition: extend:<id>` on the human path, never shadowed by a project copy. The registry comes from `--registry`, which the `labdrian` wrapper always injects. An unreadable registry refuses (fail-closed).
- **Validate-before-write**, all in the pure planner `PlanProjectRegister`, before any filesystem mutation:
  1. `--project-root` is absolute, exists and is a directory. There is no cwd fallback (the R-002 precedent from `RenderValidateCore`).
  2. The draft path is outside the project root, so the draft can never be committed or used as a target.
  3. The draft contains no `\r`.
  4. The frontmatter holds only allowlisted keys (`name`, `description`, `license`, `metadata`). `allowed-tools`, `disable-model-invocation`, `model`, `hooks` or any other key refuses, because a procedural skill must not pre-approve tools.
  5. Provenance is stamped (next decision), then `LintSkillFile` runs with zero hard errors on the stamped bytes.
  6. The `--candidate` key validates.
  7. The lock parses and has version 1.
  8. No lock entry has this id.
  9. No target directory `<root>/<dir>/<id>` exists. If one does and the id is not in the lock, the result is a "foreign skill" refusal (for example the live `archify` layout).
  10. Every destination passes `withinRoot` and `resolvedWithinRoot`.
  11. No destination lies under `<root>/skills/` (decision (f)).
- **Execution** (`ExecuteProjectPlan`, over an injected `projectFS` interface so tests can inject failures):
  - Stage: `MkdirAll` each target directory, recording which directories this run created. Write every SKILL.md and the new lock to same-directory temp files (the `writeFileAtomic` pattern). Mode 0644, never executable.
  - Commit phase: rename the SKILL.md temps in `projectTargets` order, then rename the lock last. The lock is the commit marker, like registry-last in ADR-9.
  - Rollback on any failure: for each planned write, if it captured backup bytes at plan time (the destination already existed — including a pre-existing lock file that already carries other skills' entries), restore those backup bytes through temp plus rename; if it has no backup (a genuinely new file, including a lock file created for the very first registration in this project), remove it. Then remove leftover temps, then remove directories this run created, deepest first and only if empty. Any rollback failure is printed as `error: rollback incomplete: <rel-path>` and exits 1.
- **Crash window**: a process kill between renames can leave untracked SKILL.md files with no lock entry. The next run refuses them as foreign. The contract's recovery step lets the agent delete only paths that `git status --porcelain -- <paths>` reports as `??` **and** whose bytes hash to the sha256 the failed run printed. Anything else goes to the human.

**Alternatives considered**:
- *Extending `install.go` / `RenderInstallCore`.* Rejected: it is registry-driven and Claude-only, and the proposal requires its behavior to stay unchanged.
- *Duplicating the containment check instead of extracting it.* Rejected: two guards drift.
- *Writing the lock first.* Rejected: a crash would then record a hash for files that do not exist, and a later ownership check would call them human-deleted.
- *True cross-directory atomicity.* Not available on POSIX. The git commit is the real atomic unit; the engine's job is to leave either the full planned set or the pre-run state before the commit.

### Decision: Provenance is stamped by the engine, then linted, then hashed

**Choice**: `StampProvenance(draft []byte, candidateKey string) ([]byte, error)` rewrites the `metadata:` block deterministically. It removes any existing `author`, `provenance` and `candidate` lines in that block, then appends, in this order:

```yaml
  author: "labdrian-overlay procedural"
  provenance: procedural
  candidate: procedural/candidates/<kind>/<slug...>
```

Other metadata lines, such as `version`, are kept byte-for-byte in their original order. A missing `metadata:` block is a refusal (`required-fields` would fail anyway). `ProceduralAuthor = "labdrian-overlay procedural"` is an exported constant (decision (g)). The order is stamp, then lint the stamped bytes, then hash the stamped bytes, then write. Candidate-key validation accepts exactly the item-30 shapes: `procedural/candidates/repeated-success/<s>` and `procedural/candidates/failure-recovery/<s>/<s>`, with every `<s>` non-empty and equal to `NormalizeSlug(<s>)`.

**Alternatives considered**: have the agent declare the fields and the engine only verify them. Rejected: the spec requires the system to stamp, and verification alone leaves a recurring agent failure mode ("forgot the author constant"). The stamper is about 40 lines and fully table-testable.

### Decision: Lock file schema and path (decision (b))

**Choice**: `<project-root>/.labdrian/procedural-skills.lock.json`, written as `json.MarshalIndent` with 2-space indent and a trailing newline:

```json
{
  "version": 1,
  "skills": [
    {
      "id": "probe-engram-with-home-override",
      "provenance": "procedural",
      "candidate": "procedural/candidates/repeated-success/probe-engram-with-home-override",
      "sha256": "<64 lowercase hex>",
      "revision": 1,
      "targets": [
        ".claude/skills/probe-engram-with-home-override/SKILL.md",
        ".agents/skills/probe-engram-with-home-override/SKILL.md"
      ]
    }
  ]
}
```

Rules:
- `skills` is sorted by `id`. `targets` holds repo-relative slash paths in `projectTargets` order.
- Parsing uses `DisallowUnknownFields`, and `version != 1` refuses, so a newer or foreign writer is never silently clobbered.
- A missing file means an empty lock. An empty `skills` list is kept as a file after the last retirement, so `git revert` stays symmetric.
- No timestamps. Time comes from git, and the retirement rule does not need one (see the retirement ADR).
- `skills-lock.json` is never opened.

**Alternatives considered**: reuse `skills-lock.json` (rejected by decision (b)); YAML through `parse.go` (rejected: registry-specific parser); per-skill sidecar files (rejected: Hermes machinery the proposal rejects).

**Zero-dependency invariant (ADR-15)**: slice 3a widens `allowedImports` in `engine/skills/zero_fetch_test.go` by exactly `crypto/sha256`, `encoding/hex` and `encoding/json`. All three are stdlib, with no network, no exec and nothing git-related, and `engine/runtime/opencode.go` already uses `crypto/sha256`. The widening is the reviewer-visible approval point the test comment asks for. `os/exec` and `net/*` stay absent. A hand-rolled JSON parser was rejected because the bug surface is larger than one stdlib import.

### Decision: Hash definition (exactly which bytes)

**Choice**: `sha256` is the lowercase hex SHA-256 of the exact bytes written to each target `SKILL.md`, that is, the stamped draft. All targets receive identical bytes, so the lock holds one hash per skill. There is no normalization: no line-ending conversion, no whitespace trim and no frontmatter canonicalization. Only `SKILL.md` is hashed, because project-tier skills are single-file by construction (the input is one draft file, and `assets/`, `references/` and `scripts/` are never written).

**Ownership-by-hash** is computed by `EvaluateOwnership(lockEntry, readFile, readDir)`. A skill is **agent-owned** only when all of the following hold:
- The lock entry exists.
- Every recorded target file exists and hashes to `sha256`.
- Every target skill directory contains exactly one entry, `SKILL.md`.

Anything else makes it **human-owned**, reported with the first failing reason: `hash-mismatch <path>`, `missing <path>` or `extra-entry <path>`. The check reads only the lock's hash; it never reads or compares against the candidate record's `Registered`/`Promoted` hash line. That record line is an informational mirror for humans, kept in sync by the same write that updates the lock, but is not a second source the ownership check consults.

**Alternatives considered**: normalizing CRLF and trailing whitespace before hashing. Rejected: any byte change means someone other than the agent touched the file, and normalization would hide real edits. The false-positive direction (a `core.autocrlf` checkout reads as human-owned) is the safe direction: the agent stops.

### Decision: Git commit is performed by the agent procedure with an explicit pathspec and a staged-change refusal (decision (c))

**Choice**: The engine prints the exact path set. The agent performs the git steps from contract section 11 and runs every command as `git -C <project-root>`:

1. `git -C R rev-parse --show-toplevel` must equal `R`, otherwise refuse (a subdirectory, or a nested repo or submodule).
2. `git -C R symbolic-ref -q HEAD` must succeed (refuse on detached HEAD). Refuse if any of `MERGE_HEAD`, `CHERRY_PICK_HEAD`, `REVERT_HEAD`, `rebase-merge` or `rebase-apply` exists under `git -C R rev-parse --git-dir`.
3. `git -C R diff --cached --name-only` must be empty. Any staged path refuses and is named in the refusal, and the index is left untouched.
4. `labdrian skills project-register --dry-run ...` prints the `plan: <rel>` lines.
5. `git -C R check-ignore -- <plan paths>` must print nothing. An ignored target refuses, and `git add -f` is forbidden.
6. The real run prints `wrote: <rel>`, `sha256: <hex>` and `revision: <n>`.
7. `git -C R add -- <wrote paths>`, then `git -C R diff --cached --name-only` must equal the wrote set exactly. On a mismatch, run `git -C R restore --staged -- <wrote paths>` and refuse.
8. `git -C R commit -m "<conventional message>" -- <wrote paths>`. Messages: `feat(skills): register project skill <id>`, `feat(skills): revise project skill <id>` or `chore(skills): retire project skill <id>`. No AI attribution, no `-a`, no `--no-verify`, no amend, no push.
9. `labdrian skills project-status --project-root R <id>` must report `owner:agent`. A hook that rewrote the bytes makes the skill human-owned, and that is reported rather than fixed.
10. Record `git -C R rev-parse --short HEAD` in the record's `Registered` line and in `History`.

On failure after step 6 and before the commit: the newly written SKILL.md files are always untracked (each lives under a brand-new skill directory), and are deleted only when their bytes hash to the sha256 the failed run printed. The lock file needs separate handling because it may already be tracked with other skills' entries: when it existed in `HEAD` before this run, restore it byte-for-byte with `git -C R restore --source=HEAD -- <lock path>`; when this run created it for the first time in the repository (so it is untracked), delete it instead, after the same hash check. A revision or retirement runs `git -C R restore --source=HEAD --staged --worktree -- <wrote paths>`. `git clean` and `git reset --hard` are never used.

**Alternatives considered**:
- *An engine verb that runs git through `os/exec` in a new package.* Rejected: it opens a subprocess and process-integration boundary (repository selection, hooks, credential prompts, timeouts) in a component designed to be pure. The agent already runs git under the `work-unit-commits` conventions.
- *Leaving the change staged.* Rejected by decision (c).

**Rationale**: `engine/skills` stays inside its import allowlist. The commit pathspec plus the empty-index precondition gives two independent protections against sweeping in unrelated work.

### Decision: Status vocabulary, transition table, and `History` format (decision (d))

**Choice**: The candidate `Status` values are `observing | emitted | rejected | drafted | registered | promoted | retired`, and exactly one holds at a time. The transitions (normative, contract section 9):

| From | To | Actor | Tier | Event | Required `History` evidence |
|---|---|---|---|---|---|
| none | `observing` | agent | none | first occurrence (item 30) | `engram:<id>` |
| `observing` | `emitted` | agent | none | count reaches `Threshold`, no `MatchCandidate` match (item 30) | count |
| `observing` | `rejected` | agent | none | `MatchCandidate` match (item 30) | `MatchedSkillPath` |
| `observing` or `emitted` | `rejected` | agent | none | do-not-capture class matched at emission or at draft time | `RejectionReason: do-not-capture:<class>` |
| `emitted` | `drafted` | agent | project | draft record written, `LintSkill` hard = 0 (new) or diff applies cleanly (extend) | draft topic key, `Disposition`, lint counts |
| `drafted` | `registered` | agent | project | `project-register` and commit (new), or `project-revise` of an agent-owned target (extend) | skill id, `sha256`, commit, rev |
| `registered` | `registered` | agent | project | revision (ownership OK, trigger reached) | `sha256`, commit, rev |
| `drafted` or `registered` | `promoted` | human | global | `engine skills add` merged in the overlay | overlay commit, `sha256` |
| `registered` | `retired` | agent | project | `project-retire` and commit | `RetirementReason`, commit |
| `promoted` | `retired` | human | global | `engine skills remove` merged | `RetirementReason`, overlay commit |

Non-transition events append a same-state line (`registered -> registered`, or `promoted -> promoted`). These events are: ownership lost, disposition computed, a revision draft opened, a recurrence after retirement, and the agent's post-promotion removal of the now-redundant project-tier copies. That last one deserves its own note: once a human sets `promoted`, the agent may run `project-retire --reason promoted`, which deletes the project-tier files and their project lock entry and commits, but it is not the `registered -> retired` transition in the table above — it never changes `Status`, which stays `promoted`, and it only appends a `promoted -> promoted` `History` line recording the removal. Retirement never reopens automatically.

Only a human sets `promoted`. No engine verb writes `Status`, and every project-tier procedure in the contract ends at `registered` or `retired`.

**`History` format**. One line per entry, append-only, and always the last field in the record block:

```
**History**:
- 2026-09-20T10:00:00Z | observing -> emitted | agent | count 3/3, no registry match
- 2026-09-20T10:05:00Z | emitted -> drafted | agent | draft procedural/drafts/repeated-success/probe-engram-with-home-override; Disposition new; lint hard 0 warnings 1 (body-recommended)
- 2026-09-20T10:09:00Z | drafted -> registered | agent | skill probe-engram-with-home-override rev 1 sha256:3f2a...c1 commit a1b2c3d
```

Write rule: read the current record with `mem_get_observation`, copy the existing `History` lines byte-for-byte, append one or more lines, and upsert. After the write, the old lines must be a prefix of the new ones. Records that item 30 created with no `History` get a backfilled first line on their first write under this contract: `<FirstObserved> | none -> observing | agent | backfilled from FirstObserved`.

**New record fields** (appended to item 30's field block):
- `**Disposition**: new | extend:<skill-id>`
- `**Draft**: procedural/drafts/...`
- `**Registered**: <id> rev:<n> sha256:<hex> commit:<sha> at:<RFC3339>`. Replaced on revision; the old values survive in `History`.
- `**Promoted**: <id> path:skills/<id>/SKILL.md sha256:<hex> commit:<sha> at:<RFC3339>`
- `**OccurrencesSincePromotion**: <n>`
- `**RetirementReason**: stale-reference | superseded | absorbed | promoted | quiet | human-request`
- `**AbsorbedInto**: <skill-id>`

**Draft record** (`procedural/drafts/{kind}/{slug...}`, where `{slug...}` mirrors the candidate key tail; `type: pattern`, `capture_prompt: false`):

```
**Candidate**: <candidate topic key>
**Disposition**: new | extend:<skill-id>
**Status**: open | registered | abandoned
**ForRevision**: <n>
**Lint**: hard 0, warnings <k> (<rule-ids>)
**History**:
- ...
**Body**:
````markdown
<complete SKILL.md text>      (Disposition new)
````
or a ````diff fenced unified diff (Disposition extend)
```

A four-backtick fence is used so the SKILL.md can contain its own fences. `ForRevision` is the latch: at most one open draft exists per revision number.

**Alternatives considered**: a separate `procedural/history/...` record per transition (rejected: it doubles lookups and loses the one-record audit); rewriting `History` as a summary (rejected: it defeats append-only).

### Decision: Revision trigger semantics and the `OccurrencesSincePromotion` derivation

**Choice**: `OccurrencesSincePromotion` is **derived**, like item 30's `OccurrenceCount`. It is the number of `Occurrences` entries whose timestamp is strictly after the `at:` of the most recent `Registered` or `Promoted` line. A revision writes a new `at:`, which resets it. It applies at both tiers. At the project tier it counts since registration and opens a project revision. At the global tier it counts since promotion and opens a human-path draft.

A post-registration occurrence counts only if:
- **failure-recovery**: the same failure recurred (the skill did not prevent it), or
- **repeated-success**: the observation records that the skill's instruction was missing, wrong, or had to be rediscovered.

Simply following the skill is not an occurrence, because that is the skill working. When the count reaches 2 and no open draft with `ForRevision = rev+1` exists, the agent opens one. Otherwise it only appends.

**Alternatives considered**: a stored, incremented counter (rejected: it breaks item 30's "derived, never blindly incremented" rule); counting every post-registration occurrence (rejected: it would treat use as failure for `repeated-success`).

**Disclosure**: this is a proxy, not load telemetry, and no telemetry exists. The contract says so verbatim.

### Decision: Overlay-repo behavior (decision (f))

**Choice**: In the overlay repository the project root is the overlay root. The targets are the same `projectTargets` rows (`.claude/skills/`, `.agents/skills/`). The overlay repository already has an untracked `.agents/` directory, so the foreign-directory check (step 9) and the explicit pathspec matter here too. The planner refuses any destination under `<root>/skills/` (RED test with the overlay layout reproduced in a temp dir). `MatchCandidate` against the overlay registry refuses ids that would shadow a global skill. The ondisk gate stays untouched because it only scans `skills/`. The overlay repository's own `.claude/skills/` currently holds untracked workspace-install files. The explicit pathspec and the empty-index precondition keep them out of registration commits.

### Decision: Retirement detector placement and inputs

**Choice**: The work is split along module boundaries, because Go forbids `engine` from importing `longterm-mem/internal/...` and a reverse `require`/`replace` would add a new cross-module dependency:

- **`engine skills project-status`** (slice 6, extended in 7a). Its inputs are the lock, the target files and the overlay registry. It reports per skill: `owner:agent|human (<reason>)`, and `superseded-by:<path>` when `MatchCandidate(registry, id)` or `MatchCandidate(registry, <last candidate slug>)` now matches a global skill.
- **`longterm-mem skills-stale --project-root R [--project P]`** (slice 7b), report-only. Its inputs:
  1. The lock file, parsed independently. The schema is pinned by a shared fixture, `longterm-mem/internal/skillstale/testdata/procedural-skills.lock.json`, which the engine test asserts `SerializeProjectLock` reproduces byte-for-byte.
  2. The first target SKILL.md of each entry.
  3. The paths the body names: repo-shaped tokens inside inline code spans and fenced blocks, extracted by a new `staleness.ReferencedPaths(text)` that reuses the existing strict `filePath` pattern.
  4. The commands the body names: the first word of each fenced shell line, resolved by an `os.Stat` scan of `$PATH`, with no `os/exec`.
  5. The candidate record's `LastObserved` and `Status`, read read-only through `engram.Store.ListObservations` filtered by `TopicKey`.

  It reports:
  - `REMOVED <path> (by <commit>)`: absent from the tree and deleted in history, through a new `staleness.ClassifyPaths(repoRoot, paths)` that reuses `indexTree` and `repohistory.Inspect`.
  - `MOVED <path> -> <new>`: informational.
  - `UNRESOLVED-COMMAND <name>`: informational.
  - `QUIET since <LastObserved>`: informational, when older than 180 days.

  Unlike memories, the time-ordering rule is **not** applied. A skill is an instruction, not a record, so naming a deleted path is a defect whenever the deletion happened.
- **Removal is a decision.** Project tier: `engine skills project-retire`, gated on ownership, deletes the target files and the lock entry, and the agent commits. Global tier: human `engine skills remove`. `AbsorbedInto: B` is verified before it is written. If B is global, `MatchCandidate(registry, B)` must match. If B is a project skill, B must be an entry in the project lock. Otherwise the write is refused and the unverified target named.

**Alternatives considered**: the whole detector in `engine` (rejected: it needs git history, meaning `os/exec`, and Engram reads, and `engine/skills` has neither); widening `longterm-mem`'s `allowedExecImporters` (rejected: `repohistory` already owns the git read); an automatic retirement action (rejected: the proposal forbids time-based auto-archiving).

`Detect`'s behavior is unchanged: the new exported functions sit beside it and share its private helpers.

## Data Flow

Full lifecycle, from an emitted candidate to its end states:

    item 30: candidate Status=emitted
         │
         ▼
    [agent] do-not-capture check ──match──▶ Status=rejected (do-not-capture:<class>) + History
         │ pass
         ▼
    [agent] Disposition: MatchCandidate(registry) + project-status (lock) ──▶ new | extend:<id>
         │
         ▼
    [agent] draft record procedural/drafts/... (mem_save) ──▶ labdrian skills lint <tmp draft>
         │ hard=0                                                │ hard>0 → fix or abandon
         ▼
    candidate Status=drafted + History
         │
         ▼
    [agent] git preconditions (toplevel, HEAD, empty index)
         │
         ▼
    labdrian skills project-register --dry-run ──▶ plan paths ──▶ git check-ignore
         │
         ▼
    engine: stamp ─▶ lint ─▶ hash ─▶ guard ─▶ stage temps ─▶ rename SKILL.md ×N ─▶ rename lock
         │ wrote: paths, sha256                        (failure → rollback, exit 1)
         ▼
    [agent] git add -- paths; verify staged set; git commit -- paths; project-status owner:agent
         │
         ▼
    candidate Status=registered, Registered line, History (commit)
         │
         ├── recurrences after `at:` ≥ 2 ─▶ revision draft ─▶ project-revise (ownership gate) ─▶ commit
         ├── human promotion: copy ─▶ labdrian skills add (lint gate) ─▶ overlay PR merged ─▶ Status=promoted (human-set)
         │        └─▶ agent project-retire (reason promoted) ─▶ deletes project files+lock entry; Status stays promoted; History appended
         └── longterm-mem skills-stale / project-status report ─▶ decision ─▶ project-retire ─▶ commit

Trust boundaries: Go never writes Engram. Go never runs git. The only Go writer into a consumer repository is the planned registry path set.

## File Changes

| Slice | File | Action | Description |
|---|---|---|---|
| 1a `skill-lint-core` | `openspec/changes/procedural-memory-lifecycle/codex-smoke.md` | Create | Smoke-test record: authorization status, commands, redacted output, verdict (`UNVERIFIED` when not authorized); decides only the Codex support claim |
| 1a | `engine/skills/lint.go` | Create | `SplitSkillFile`, frontmatter subset reader, `lintRules`, `LintSkill`, `LintSkillFile`, `EstimateBodyTokens`, `LintError`, `Warning` |
| 1a | `engine/skills/lint_test.go` | Create | Boundary table for every rule, determinism, well-formed fixture |
| 1b `skill-lint-surface` | `engine/skills/lint_cli.go` | Create | `RenderLintCore` (`lint <path>`, `lint --rules`), tolerant of wrapper-injected flags; `RenderLintRules` |
| 1b | `engine/skills/lint_cli_test.go` | Create | Exit contract, printed rule ids, injected-flag tolerance |
| 1b | `engine/skills/skills.go`, `engine/cmd/main.go` | Modify | Dispatch `lint`; usage strings |
| 1b | `skills/skill-creator/references/skill-style-guide.md` | Modify | Generated block between markers; numeric restatements removed |
| 1b | `skills/skill-improver/references/skill-style-guide.md` | Modify | Byte-identical copy |
| 1b | `skills/skill-creator/SKILL.md`, `skills/skill-improver/SKILL.md`, `skills/skill-registry/SKILL.md` | Modify | Drop `docs/skill-style-guide.md`; skill-creator runs `labdrian skills lint` |
| 1b | `engine/skills/style_guide_contract_test.go` | Create | Drift, byte-identity, no-dangling-reference, no-second-numeric-source |
| 2 `drafting-contract` | `skills/_shared/procedural-candidate-detection.md` | Modify | Sec 7 do-not-capture; sec 8 lesson shape, `Disposition`, draft record; sec 9 status vocabulary, transition table, `History`, new fields; sec 10 runtime targets table (`.claude/skills/`, `.agents/skills/`, Codex status, Pi trust note; no `.pi/skills/`) |
| 2 | `engine/skills/procedural_candidate_contract_test.go` | Modify | Verbatim assertions for sections 7-10 |
| 3a `project-lock` | `engine/skills/project_lock.go` | Create | Lock types, `ParseProjectLock`, `SerializeProjectLock`, `HashSkill`, `EvaluateOwnership`, `StampProvenance`, `ValidateCandidateKey`, `ProceduralAuthor` |
| 3a | `engine/skills/project_lock_test.go` | Create | Round-trip, unknown-field and version refusal, stamping table, hash, ownership reasons, key shapes |
| 3a | `engine/skills/zero_fetch_test.go` | Modify | Add `crypto/sha256`, `encoding/hex`, `encoding/json` |
| 3b `project-register-core` | `engine/skills/pathguard.go` | Create | `withinRoot`, `resolvedWithinRoot` |
| 3b | `engine/skills/install.go` | Modify (mechanical) | `PlanInstall` calls `withinRoot`; no behavior change |
| 3b | `engine/skills/project_register.go` | Create | `projectTargets`, `PlanProjectRegister`, `ExecuteProjectPlan`, `projectFS` |
| 3b | `engine/skills/project_register_test.go` | Create | Temp-dir registration, traversal, symlink, foreign-dir, `skills/` refusal, frontmatter allowlist, rollback per failure point, targets-vs-contract test, no `.pi/skills` anywhere |
| 4 `project-register-cli` | `engine/skills/project_cli.go` | Create | `RenderProjectRegisterCore` (`project-register`, `--dry-run`); prints the Pi trust `note:` line after a successful write |
| 4 | `engine/skills/project_cli_test.go` | Create | Flags, missing `--project-root`, output format, trust note present on success and absent on refusal/dry-run, exit codes, `skills-lock.json` untouched |
| 4 | `engine/skills/skills.go`, `engine/cmd/main.go` | Modify | Dispatch and usage |
| 4 | `skills/_shared/procedural-candidate-detection.md` | Modify | Sec 11 registration and commit procedure, crash recovery, acceptance checklist |
| 4 | `engine/skills/procedural_candidate_contract_test.go` | Modify | Sec 11 assertions |
| 5 `global-promotion` | `engine/skills/lifecycle.go` | Modify | `AddCore` reads `<src>/<id>/SKILL.md`, runs `LintSkillFile`, prints `[lint:<id>]` errors and exits 1 before `Serialize` |
| 5 | `engine/skills/lifecycle_test.go` | Modify | Fixtures serve lint-clean SKILL.md; RED cases for a hard error (bytes unchanged) and warnings-only |
| 5 | `skills/_shared/procedural-candidate-detection.md` | Modify | Sec 12 human promotion procedure |
| 6 `revision` | `engine/skills/project_register.go`, `project_cli.go` | Modify | `project-revise` (ownership gate, rev bump, backup restore), `project-status` (ownership) |
| 6 | `engine/skills/project_register_test.go`, `project_cli_test.go` | Modify | Human-owned refusal per reason, revision rollback |
| 6 | `skills/_shared/procedural-candidate-detection.md` | Modify | Sec 13 revision trigger, emission-table rows for post-registration occurrences |
| 7a `retirement-engine` | `engine/skills/project_register.go`, `project_cli.go` | Modify | `project-retire`; `project-status` supersession via `MatchCandidate` |
| 7a | tests, contract sec 14 | Modify | Retirement decision, reasons, `AbsorbedInto` verification |
| 7b `retirement-detector` | `longterm-mem/internal/staleness/staleness.go` | Modify | Export `ReferencedPaths`, `ClassifyPaths`; `Detect` unchanged |
| 7b | `longterm-mem/internal/skillstale/skillstale.go`, `_test.go`, `testdata/procedural-skills.lock.json` | Create | Detector over the lock, SKILL.md and candidate records |
| 7b | `longterm-mem/cmd/longterm-mem/cmd_skills_stale.go`, `_test.go`, `main.go` | Create/Modify | Report-only subcommand |
| 7b | `engine/skills/project_lock_test.go` | Modify | Pin `SerializeProjectLock` to the shared fixture |
| none | `skills-lock.json`, `skills.registry.yaml`, `overlay.manifest`, `engine/skills/ondisk.go`, `parse.go`, `types.go` | Untouched | Proposal out of scope |

### Slice mapping (sequential; apply, review, PR, merge; never parallel)

The proposal's 7 slices are refined into 10 to hold the 400-line budget. The proposal's plan states that its estimates are for `sdd-tasks` to refine. The order and content are the proposal's. Revised decision (e) leaves the estimates materially unchanged: 3b drops the conditional-target mechanics but keeps a fixed two-row table and its contract test (~370), and 4 adds the Pi trust `note:` line and its tests (~310).

| # | Slice | Proposal slice | Estimate | Item-30 symbols and sections consumed |
|---|---|---|---|---|
| 1a | `skill-lint-core` | 1 | ~350 | none directly (prerequisite gate only) |
| 1b | `skill-lint-surface` | 1 | ~300 | none |
| 2 | `drafting-contract` | 2 | ~250 | sec 1 topic-key shapes, sec 2 record block, sec 4 emission table, sec 5 `Status` latch, sec 6 rejection fields and `MatchCandidate` call site |
| 3a | `project-lock` | 3 | ~350 | `NormalizeSlug`, sec 1 key shapes (in `ValidateCandidateKey`) |
| 3b | `project-register-core` | 3 | ~370 | `NormalizeSlug`, `MatchCandidate`, `ParseRegistry` |
| 4 | `project-register-cli` | 4 | ~310 | sec 2 record block (`Registered` line wiring in the procedure) |
| 5 | `global-promotion` | 5 | ~200 | sec 2 record block (`Promoted` line) |
| 6 | `revision` | 6 | ~300 | sec 3 occurrence rules, sec 4 emission table (new rows) |
| 7a | `retirement-engine` | 7 | ~250 | `MatchCandidate`, sec 6 |
| 7b | `retirement-detector` | 7 | ~350 | sec 2 field names (`LastObserved`, `Status`), read-only |

## Interfaces / Contracts

```go
// lint.go
type Severity string
const (SeverityHard Severity = "hard"; SeverityAdvisory Severity = "advisory")
const (
    DescriptionMaxRunes    = 250
    DescriptionShouldRunes = 160
    BodyHardTokens         = 1000
    BodyRecommendedTokens  = 700
    BytesPerTokenProxy     = 4
)
type LintError struct{ Rule, Msg string } // implements error
type Warning   struct{ Rule, Msg string }
func SplitSkillFile(data []byte) (frontmatter, body string, err error) // err is a *LintError{Rule:"frontmatter-fence"}
func LintSkill(frontmatter, body string) (hard []error, warnings []Warning)
func LintSkillFile(data []byte) (hard []error, warnings []Warning)
func EstimateBodyTokens(body string) int // (len(body)+3)/4
func RenderLintRules() string             // generated style-guide block (without markers)

// project_lock.go
const ProceduralAuthor = "labdrian-overlay procedural"
const ProjectLockRelPath = ".labdrian/procedural-skills.lock.json"
type ProjectLock struct { Version int `json:"version"`; Skills []ProjectLockEntry `json:"skills"` }
type ProjectLockEntry struct {
    ID, Provenance, Candidate, SHA256 string // json: id, provenance, candidate, sha256
    Revision int                             // json: revision
    Targets  []string                        // json: targets (repo-relative, slash)
}
func ParseProjectLock(data []byte) (ProjectLock, error)   // missing file handled by caller as empty
func SerializeProjectLock(l ProjectLock) ([]byte, error)  // sorted, 2-space indent, trailing newline
func HashSkill(data []byte) string                        // lowercase hex sha256
func ValidateCandidateKey(key string) error
func StampProvenance(draft []byte, candidateKey string) ([]byte, error)
type Ownership struct{ AgentOwned bool; Reason string } // Reason: "", "hash-mismatch <p>", "missing <p>", "extra-entry <p>", "not-in-lock"
func EvaluateOwnership(root string, e ProjectLockEntry, readFile func(string) ([]byte, error), readDir func(string) ([]fs.DirEntry, error)) Ownership

// project_register.go
type ProjectTarget struct{ Name, Dir string }
var projectTargets = []ProjectTarget{{"claude", ".claude/skills"}, {"agents", ".agents/skills"}} // fixed; never .pi/skills; Codex verdict does not change it
type ProjectWrite struct{ Rel, Abs string; Data []byte; Backup []byte /* nil when new */ }
type ProjectPlan struct{ ID, SHA256 string; Revision int; Writes []ProjectWrite; Deletes []string; Lock ProjectWrite }
func PlanProjectRegister(in RegisterInput) (ProjectPlan, error) // pure; RegisterInput carries pre-read state
func ExecuteProjectPlan(p ProjectPlan, fsys projectFS, stdout, stderr io.Writer) error
```

CLI, reached from any repository through the `labdrian` wrapper. The wrapper appends `--registry`, `--manifest` and `--source-root` after the verb. Every new verb consumes those flag pairs and ignores their values, except that `--registry` is used for `MatchCandidate`.

```
labdrian skills lint <path> | --rules                                           exit 0 (warnings ok) | 1 (hard or usage/read error)
labdrian skills project-register --project-root <abs> --candidate <key> [--dry-run] <draft-file>
labdrian skills project-revise   --project-root <abs> --candidate <key> [--dry-run] <draft-file>
labdrian skills project-status   --project-root <abs> [<id>]
labdrian skills project-retire   --project-root <abs> [--dry-run] <id>
longterm-mem skills-stale --project-root <abs> [--project <P>]                 report-only
```

Output lines: `plan: <rel>`, `wrote: <rel>`, `removed: <rel>`, `sha256: <hex>`, `revision: <n>`, `note: <text>` (the Pi trust disclosure after a successful `project-register`) and `<id> rev:<n> owner:agent|human[ (<reason>)] superseded-by:<path>|-`. The agent procedure relays `note:` lines to the user and never uses them as a pathspec. All paths are repo-relative with slashes and are ready to use as a git pathspec.

## Testing Strategy

Strict TDD for all Go code: RED first, then GREEN, then REFACTOR.
- Focused runs: `cd engine && go test ./skills/... ./cmd/...` and `cd longterm-mem && go test ./internal/staleness/... ./internal/skillstale/... ./cmd/...`.
- Broad runs: `cd engine && go vet ./... && go test ./...` and `cd longterm-mem && go vet ./... && go test ./...`.

**Isolation rule**: every filesystem test roots at `t.TempDir()`. No test reads or writes the live `.claude/skills/`, `.agents/skills/`, `.pi/`, `skills-lock.json`, `$HOME`, or a runtime binary, and none spawns git from engine code. Two structural guarantees back this:
- No new verb falls back to cwd or `$HOME`. A missing `--project-root` exits 1 with nothing written (RED test).
- `longterm-mem` detector tests build their git history in a temp repo, as the existing `repohistory` tests do, and use a fixture Engram database.

The one live check is the Codex smoke test. It is a recorded manual procedure, never a `go test`, and it runs only with explicit owner authorization at apply time (otherwise `UNVERIFIED`).

| Slice | Layer | What | Approach |
|---|---|---|---|
| 1a | Unit | Every rule at its boundary: fence missing or unclosed; each required field missing or empty; block-scalar and continuation description; 250/251 and 160/161 runes; 4000/4001 and 2800/2801 body bytes; section order in and out of order; each incident-log pattern; each banned utility inline and fenced, plus a near miss (`catalog`); home-path forms. Also determinism and a clean fixture with zero findings | Table tests on rule IDs |
| 1b | Unit/CLI | Exit 0 with warnings-only output, exit 1 on hard errors, missing path, `--rules` output equals `RenderLintRules()`, wrapper-injected trailing flags | `RenderLintCore` with injected I/O |
| 1b | Contract | Generated block in sync; skill-improver copy byte-identical; no `docs/skill-style-guide.md` references; no numeric restatement outside the block | Repo-file assertions (`readRepoFile`) |
| 2 | Contract | Sections 7-10 verbatim: do-not-capture classes, draft key shape, status vocabulary, transition table, `History` line format, runtime targets table | Extend `procedural_candidate_contract_test.go` |
| 2, 4, 6, 7a | Acceptance (executed in `sdd-verify`) | `History` only grows across a draft, register, revise and retire sequence. Do-not-capture refusal is recorded. The `extend` path. Commit procedure: clean repo gives exactly one commit whose paths equal the wrote set; an unrelated staged file refuses with the index unchanged; an ignored target refuses; detached HEAD refuses; `git revert` restores the files and lock. A human-edited skill refuses revision. Retirement is one revertable commit | Numbered checklist in the contract, run in a temp git repo with real Engram records, results recorded honestly |
| 3a | Unit | Lock round-trip and sorting; unknown field and version 2 refused; `StampProvenance` insert, replace and preserve-order table plus missing `metadata:`; `HashSkill` known vector; ownership: agent, hash-mismatch, missing, extra-entry, not-in-lock; candidate key valid, invalid and non-normalized | Table tests |
| 3b | Unit/FS | Registration over temp root writes every target and the lock with the correct hash. Refusals: traversal id (`../../etc`); symlinked `.claude` escaping the root; foreign existing dir; `allowed-tools` frontmatter; draft inside the project root; CR bytes; registry match; any destination under `skills/`. Rollback leaves the pre-run tree byte-identical, with an injected failure at each rename index and during staging. Files are 0644. `install_test.go` passes unmodified. Registration writes exactly `.claude/skills/<id>/SKILL.md` and `.agents/skills/<id>/SKILL.md` and never creates `.pi/`. The Go targets equal the contract sec 10 directory rows, and neither names `.pi/skills` | `projectFS` fake plus real temp dirs; tree snapshot before and after |
| 4 | CLI | `--dry-run` writes nothing and prints plan lines; a successful run prints the Pi trust `note:` line, refusals and `--dry-run` do not; missing `--project-root`; relative root refused; `skills-lock.json` bytes unchanged; wrapper flags tolerated | Injected I/O plus temp dirs |
| 5 | Unit | `AddCore` hard lint error exits 1 with registry and manifest bytes unchanged and the error names `metadata.version`; warnings-only exits 0; existing SC cases pass with lint-clean fixtures | Extend `lifecycle_test.go` |
| 6 | Unit/FS | Revise refused per ownership reason; revise bumps revision and hash; revision rollback restores backup bytes | Temp dirs |
| 7a | Unit/FS | Retire removes files and the lock entry; human-owned refused; `project-status` reports `superseded-by` on a registry match | Temp dirs |
| 7b | Unit/FS | A removed path is flagged `REMOVED` regardless of order; a renamed path gives `MOVED` only; a clean skill is not flagged; no file mtime or content changes (tree snapshot); the Engram fixture is opened read-only; the lock fixture parses; `os/exec` allowlist unchanged | Temp git repo, fixture DB |

## Threat Matrix

| Boundary | Minimum adversarial cases | Applicability | Design response | Planned RED tests |
|---|---|---|---|---|
| Documentation-like paths | Executable Markdown | **Applicable**: SKILL.md is instruction Markdown that Claude Code and Pi (after project trust), and possibly Codex, load and act on | Exactly two targets (`.claude/skills/`, `.agents/skills/`), never `.pi/skills/`. Exactly one `SKILL.md` per target, mode 0644, never scripts, assets or references. Frontmatter key allowlist refuses `allowed-tools` and similar privilege keys. Draft must live outside the root. Lint plus do-not-capture plus a git-visible commit plus revert. Prompt injection from a poisoned observation is recorded as a residual risk. Pi's trust prompt is a user-visible checkpoint, not a control this design relies on | 3b: no `.pi/` created; `allowed-tools` refused; unknown key refused; file mode 0644; only `SKILL.md` created per target dir; draft inside root refused |
| Git repository selection | `git -C`, relative paths, absolute paths | **Applicable** (agent-performed git, engine-planned paths) | Engine requires an absolute, existing `--project-root` with no cwd fallback and emits repo-relative paths only. Agent runs every git command as `git -C R` and refuses when `rev-parse --show-toplevel` is not `R` (subdirectory, nested repo, submodule) | 4: relative root refused; missing root refused; non-directory root refused; output paths are relative. Acceptance: subdirectory root refused; nested repo refused |
| Commit state | staged, `commit -a`, empty index | **Applicable** | Pre-existing staged paths refuse and the index is left untouched. `-a` is forbidden. After `git add -- <paths>` the staged set must equal the wrote set, otherwise it is unstaged and refused. Commit uses a pathspec. Detached HEAD or an operation in progress refuses. No `--no-verify`, no amend. Post-commit ownership check surfaces hook rewrites | Acceptance (sdd-verify, temp repo): unrelated staged file; staged set mismatch; ignored target; detached HEAD; merge in progress; hook rewrites bytes and is reported human-owned. 3b/4 Go: planned path set is exact and deterministic |
| Push state | tracking branch, first push, refspec | N/A: no push anywhere. Commits stay local and push is the user's decision under ordinary repository policy | none | none |
| PR commands | `--head`, env prefix, composition | N/A: no PR automation. The overlay promotion PR is created by a human under ordinary policy | none | none |

## Migration / Rollout

- Existing item-30 candidate records get a backfilled first `History` line on their first write under this contract. No bulk migration.
- Consumer repositories: the first registration creates `.labdrian/`. Registration refuses where the targets are gitignored, and does not change any ignore rules.
- Rollout is per slice, in the order above. Until slice 4 merges, no consumer repository is written. Until slice 5 merges, `AddCore` behaves exactly as before.
- Rollback: revert slice PRs in reverse order (7b to 1a). The `AddCore` gate reverts with slice 5. Consumer-side artifacts revert with `git revert <commit>`, lock entries included.

## Open Questions

- [x] **Pi trust premise behind decision (e)**: resolved. The owner revised decision (e) on 2026-09-18 (Engram #3435) after installed Pi docs showed `.pi/skills/` also requires project trust. Targets are now `.claude/skills/` and `.agents/skills/` only; the one-time trust prompt is an accepted, disclosed consequence. The earlier Pi duplicate-name risk no longer exists because only one Pi-visible copy is written.
- [x] **Spec alignment (targets)**: resolved 2026-09-18. Specs name exactly `.claude/skills/` and `.agents/skills/`; `.pi/skills/` appears only in MUST NOT assertions; `.agents/skills/` does not depend on the Codex verdict.
- [x] **Spec alignment (OccurrencesSincePromotion, AbsorbedInto)**: resolved 2026-09-18. The maintenance and detection specs now define `OccurrencesSincePromotion` as derived at both tiers and verify `AbsorbedInto` by existence of the absorbing skill, matching this design.
- [ ] **Proxy calibration**: `BytesPerTokenProxy = 4` is uncalibrated against a real tokenizer. Revisit once measured. It is one constant plus its boundary tests.
- [ ] **Inherited from item 30**: the contract document is not in `gateContractFiles` (`engine/pipkg/pipkg.go`), so Pi-packaged agents may not see sections 7-14. Revisit if Pi agents are expected to run registration.
- [ ] **Codex smoke authorization**: slice 1a needs explicit owner authorization at apply time for one remote Codex probe (`codex exec` contacts the model provider under the user's Codex login). Without it the verdict is recorded as `UNVERIFIED` and Codex support is not claimed. The target set is unaffected either way.
