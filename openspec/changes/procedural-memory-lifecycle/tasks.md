# Tasks: Procedural Memory Lifecycle (draft, register, promote, revise, retire)

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~3030 total across 10 slices (design's per-slice estimates, refined below): 1a ≈350, 1b ≈300, 2 ≈250, 3a ≈350, 3b ≈370, 4 ≈310, 5 ≈200, 6 ≈300, 7a ≈250, 7b ≈350 |
| Per-slice estimate | See table above; every slice is individually ≲400, several (3b, 4, 1a) sit close to the budget and should not accrete unplanned scope |
| Chained PRs recommended | Yes — 10 sequential PRs in a feature-branch chain, each gated on the previous slice merging into its base; only the tracker merges to `main` |
| 400-line budget risk | Medium overall (not per slice): 3b (~370) and 4 (~310) are the closest to the ceiling; any RED-phase scope creep (extra refusal cases, extra CLI flags) pushes either over 400 and forces a further split (e.g. 3b into 3b-i lock-write / 3b-ii traversal-and-refusals) |
| Size exceptions | **Slice 1a: `size:exception` granted by the owner on 2026-09-18** — actual 1024 insertions / 9 deletions (lint.go 544, lint_test.go 470) against a ~350 forecast. Reason: one cohesive 11-rule table with per-rule boundary tests; the only coherent split (hard vs advisory rules) leaves both halves near 500 lines. |
| Decision needed before apply | **Resolved 2026-09-18** — owner chose `chain_strategy: feature-branch-chain`. A tracker branch holds the feature; PR 1 targets the tracker; each later PR targets the immediately previous slice branch; only the tracker merges to `main`, after PR 10. Slices remain strictly one at a time (apply → review → PR → merge into its base). `delivery_strategy: auto-chain` |
| Reliability-review deltas (2026-09-18) | The reliability lens's approved, non-blocking findings added: one RED test each to 3b (rollback-of-rollback failure, task 3b.5a) and 7a (retirement rollback and rollback-of-rollback failure), plus non-Go acceptance-checklist cases to 2.4 (lesson-shape rejection) and 4.8 (staged-set mismatch, merge/rebase in progress, subdirectory/nested-root, hook-rewrite, second-registration crash recovery). These stay within each slice's existing headroom language and do not change the per-slice line estimates above |

Ten sequential slices are used instead of the proposal's seven because revised decision (e) and the item-30 prerequisite gate add fixed per-slice overhead (contract-test wiring, target-table drift tests, the Codex smoke record) that the proposal's coarser 7-slice estimate did not itemize; the design's slice-mapping table is authoritative and is reproduced task-by-task below.

## Phase 0: Prerequisite Gate (blocks every slice below)

- [x] 0.1 Verify `main` at `engine/skills/match.go` exports `MatchCandidate` (item 30, phases 2-4)
- [x] 0.2 Verify `openspec/changes/archive/2026-09-18-procedural-candidate-detection/tasks.md` (read-only) has every task in phases 2-4 (2.x, 3.x, 4.x) checked `[x]` (item 30 was archived 2026-09-18, PR #351)
- [x] 0.3 Verify `skills/_shared/procedural-candidate-detection.md` on `main` contains sections 4, 5 and 6 (emission decision table, `Status` latch, `MatchCandidate`/rejection fields)
- [x] 0.4 If any of 0.1-0.3 fails, STOP: report `status: blocked`, name the missing item, and do not start slice 1a (none failed; proceeding to slice 1a)
- [x] 0.5 Verify `engine/skills/zero_fetch_test.go`'s `allowedImports` already includes `path` (item 30's own widening for `MatchCandidate`'s `path.Base`); slice 3a's widening (§3a.3) is additive on top of this, not a replacement

## Phase 1a: `skill-lint-core` (PR 1, ~350 lines)

Item-30 symbols consumed: none directly — this slice only depends on the Phase 0 gate having passed.

- [x] 1a.1 Codex smoke test record (non-Go; requires explicit owner authorization at apply time for the `codex exec` remote probe — design §"Fixed two-target set", steps 1-4)
  - [x] 1a.1.1 At apply time, ask the owner for explicit authorization: destination (Codex provider), operation (≤2 probe prompts + 1 control, no repository content sent), credential (existing Codex login session)
  - [x] 1a.1.2 If authorized: run the procedure in a fresh `mktemp -d` outside every repo and outside the user's home .agents directory — record `codex --version` (expect `0.148.0`, else verdict `INCONCLUSIVE`), `codex --help`, `codex exec --help`; `git init` the temp dir; create `.agents/skills/labdrian-smoke-probe/SKILL.md` with the fresh-nonce body; treatment run (`codex exec --cd <tmp> "..."`); control run (skill moved to `.agents/not-skills/...`)
  - [x] 1a.1.3 (verdict PASS via a disclosed probe-2 variant with the nonce in the description; probe 1 INCONCLUSIVE on a host sandbox limitation — see codex-smoke.md) Determine verdict per design rule: `PASS` (nonce in treatment only), `FAIL` (nonce absent from treatment), `INCONCLUSIVE` (auth/network/trust failure or nonce leaks into control)
  - [x] 1a.1.4 (not applicable: authorization was granted) If not authorized: record verdict `UNVERIFIED` and continue without blocking this slice
  - [x] 1a.1.5 Write `openspec/changes/procedural-memory-lifecycle/codex-smoke.md`: date, version, authorization status, exact commands, outputs (absolute home paths redacted to a tilde), verdict
- [x] 1a.2 RED `engine/skills/lint_test.go`: boundary table for every hard rule (`frontmatter-fence` missing/unclosed; each of `name`/`description`/`license`/`metadata.author`/`metadata.version` missing or empty; `description-one-line` with a block scalar and with an indented continuation; `description-max` at 250/251 runes; `body-hard-budget` at 4000/4001 bytes) and every advisory (`description-should` at 160/161; `body-recommended` at 2800/2801; `section-order` in/out of canonical order with a missing heading allowed and an unknown heading ignored; `incident-log-shape` for an ISO date, an `engram:\d+` ref, and a PR/issue number, each independently; `banned-shell-utility` for each of `cat`/`grep`/`find`/`sed`/`ls` inline and fenced, plus a near-miss `catalog` that must NOT match; `home-path-leak` for a Linux home-directory prefix, a macOS Users prefix and a Windows Users prefix, each followed by a user name (exact fixture strings are in design.md, lint rule table; they are test data, not edit paths); determinism (two calls, byte-identical output); a clean fixture with zero hard errors and zero warnings
- [x] 1a.2a RED `engine/skills/lint_test.go`: `LintSkillFile` boundary tests for a missing fence and an unclosed fence, each surfaced as a hard `frontmatter-fence` error, plus a test proving `LintSkill` is never invoked when the split fails (observable via zero warnings and exactly one hard error on garbage input that would otherwise trigger many `required-fields` findings) (carry-forward, owner-accepted, review-42be59cf3243a7ff advisory R3-lintskillfile-coverage: this scenario moved from `LintSkill` to the new `LintSkillFile Composes SplitSkillFile and LintSkill` requirement in skill-lint/spec.md)
- [x] 1a.3 GREEN `engine/skills/lint.go`: `Severity`, `LintError`, `Warning`, `lintRules []lintRule` table, `SplitSkillFile` (frontmatter-fence check lives here), a line-based frontmatter subset reader (top-level `key: value`, one `metadata:` block with 2-space-indented pairs, quote stripping — separate from `parse.go`), `LintSkill(frontmatter, body string) (hard []error, warnings []Warning)`, `LintSkillFile(data []byte)`, `EstimateBodyTokens(body string) int = (len(body)+3)/4`, named constants `DescriptionMaxRunes=250`, `DescriptionShouldRunes=160`, `BodyHardTokens=1000`, `BodyRecommendedTokens=700`, `BytesPerTokenProxy=4`, `RenderLintRules() string`
- [x] 1a.4 REFACTOR: confirm `LintSkill`/`LintSkillFile` perform no filesystem/network/`os/exec` access and import stdlib only; confirm `RenderLintRules()` output renders every rule's numeric bound from the named constants (no hardcoded literals in the rendered text)
- [x] 1a.5 Verify: `cd engine && go test ./skills/...`

## Phase 1b: `skill-lint-surface` (PR 2, ~300 lines)

Item-30 symbols consumed: none.

- [ ] 1b.1 RED `engine/skills/lint_cli_test.go`: `lint <path>` exit 0 with warnings-only output; exit 1 on any hard error with the missing field named in output; missing/unreadable path handling; `lint --rules` output equals `RenderLintRules()` byte-for-byte; wrapper-injected trailing flags (`--registry`, `--manifest`, `--source-root`) are tolerated and ignored
- [ ] 1b.2 GREEN `engine/skills/lint_cli.go`: `RenderLintCore` implementing `lint <path>` and `lint --rules`
- [ ] 1b.3 GREEN `engine/skills/skills.go`, `engine/cmd/main.go`: dispatch the `lint` verb; usage strings
- [ ] 1b.4 RED `engine/skills/style_guide_contract_test.go`: `TestSkillStyleGuideLintRulesInSync` (byte-identical between the generated-block markers and `RenderLintRules()`, printing the expected block on failure); `skill-improver`'s copy is byte-identical to `skill-creator`'s; none of `skills/skill-creator/SKILL.md`, `skills/skill-improver/SKILL.md`, `skills/skill-registry/SKILL.md` mentions `docs/skill-style-guide.md`; the "Frontmatter Rules"/"Body Budget" prose sections in the style guide contain no numeric restatement of 250/160/700/1000 outside the generated block
- [ ] 1b.5 GREEN `skills/skill-creator/references/skill-style-guide.md`: insert the generated block between the BEGIN GENERATED skill-lint-rules and END GENERATED HTML-comment markers (exact marker text in design.md) (paste the output of `labdrian skills lint --rules`, run manually — no engine write path); strip the numeric restatements from prose, keeping the 180-450 token human-judgement guidance as prose
- [ ] 1b.6 GREEN `skills/skill-improver/references/skill-style-guide.md`: byte-identical copy of the skill-creator file
- [ ] 1b.7 GREEN `skills/skill-creator/SKILL.md`: drop "Inline Fallback Rules" numbers and the `docs/skill-style-guide.md` reference; document that it runs `labdrian skills lint`
- [ ] 1b.8 GREEN `skills/skill-improver/SKILL.md`, `skills/skill-registry/SKILL.md`: drop the `docs/skill-style-guide.md` reference
- [ ] 1b.9 REFACTOR: confirm no engine code path writes under `skills/` (print-and-paste only, per decision (a) alternatives-considered)
- [ ] 1b.10 Verify: `cd engine && go test ./skills/... ./cmd/...`

## Phase 2: `drafting-contract` (PR 3, ~250 lines)

Item-30 symbols/sections consumed: sec 1 topic-key shapes, sec 2 record block, sec 4 emission table, sec 5 `Status` latch, sec 6 rejection fields and `MatchCandidate` call site (read-only reference, not modified).

- [ ] 2.1 RED `engine/skills/procedural_candidate_contract_test.go` (extend, following the item-30 pattern): assert sections 7-10 appear verbatim — do-not-capture list (5 categories), lesson-shape rule (imperative + one why-clause, no ids/dates/PR numbers), `Disposition: new | extend:<skill-id>`, draft record topic-key shape `procedural/drafts/{kind}/{slug}` and its field block (`Candidate`, `Disposition`, `Status`, `ForRevision`, `Lint`, `History`, `Body`), extended `Status` vocabulary (`observing | emitted | rejected | drafted | registered | promoted | retired`), the full transition table (10 rows), the `History` line format and write rule (read-copy-append-upsert, prefix invariant), the new candidate-record fields (`Disposition`, `Draft`, `Registered`, `Promoted`, `OccurrencesSincePromotion`, `RetirementReason`, `AbsorbedInto`), and the runtime-targets table (`.claude/skills/`, `.agents/skills/`, Codex status cell, Pi trust note; asserts no row names `.pi/skills`)
- [ ] 2.2 GREEN `skills/_shared/procedural-candidate-detection.md`: write sections 7-10 exactly as asserted in 2.1, sourced from design decisions (linked spec reqs: `procedural-skill-drafting` — Do-Not-Capture, Lesson Shape, Disposition, Draft Records Live Only in Engram, Extended Status Vocabulary, History Is Append-Only; `procedural-candidate-detection` delta — Do-Not-Capture List Gates Candidate Quality, Extended Status Vocabulary, Disposition Field, Append-Only History)
- [ ] 2.3 REFACTOR: confirm sections 7-10 cross-reference item 30's sections 1-6 by name rather than duplicating their text
- [ ] 2.4 Acceptance checklist (non-Go, executed during `sdd-verify`, per `procedural-skill-drafting`'s lesson-shape requirement): a Summary containing an `engram:<id>` reference is rejected before any draft record is saved; a Summary containing a literal date is rejected before any draft record is saved; a Summary containing a PR/issue number is rejected before any draft record is saved; a clean imperative-rule-plus-one-why-clause Summary passes and its draft record is saved; confirm the advisory `incident-log-shape` lint warning is a separate, non-blocking signal that never gates drafting
- [ ] 2.5 Verify: `cd engine && go test ./skills/... ./cmd/...`

## Phase 3a: `project-lock` (PR 4, ~350 lines)

Item-30 symbols consumed: `NormalizeSlug`; sec 1 key shapes (in `ValidateCandidateKey`).

- [ ] 3a.1 RED `engine/skills/project_lock_test.go`: `ParseProjectLock`/`SerializeProjectLock` round-trip and `id`-sort determinism; unknown-field refusal (`DisallowUnknownFields`); `version != 1` refusal; missing file handled as empty by the caller (documented, not implemented here); `StampProvenance` table — insert into an existing `metadata:` block, replace an existing `author`/`provenance`/`candidate` line, preserve other metadata lines (e.g. `version`) byte-for-byte in original order, refusal on a missing `metadata:` block; `HashSkill` known-vector test; `EvaluateOwnership` reasons — agent-owned, `hash-mismatch <path>`, `missing <path>`, `extra-entry <path>`, `not-in-lock`; `ValidateCandidateKey` — valid `repeated-success/<s>` and `failure-recovery/<s>/<s>` shapes, empty segment, non-normalized segment (fails `NormalizeSlug` idempotence)
- [ ] 3a.2 GREEN `engine/skills/project_lock.go`: `ProceduralAuthor` constant (`"labdrian-overlay procedural"`), `ProjectLockRelPath = ".labdrian/procedural-skills.lock.json"`, `ProjectLock`/`ProjectLockEntry` types, `ParseProjectLock`, `SerializeProjectLock` (2-space indent, trailing newline, sorted by `id`), `HashSkill`, `ValidateCandidateKey`, `StampProvenance`, `Ownership` type, `EvaluateOwnership(root, e, readFile, readDir)`
- [ ] 3a.3 GREEN `engine/skills/zero_fetch_test.go`: widen `allowedImports` by exactly `crypto/sha256`, `encoding/hex`, `encoding/json`
- [ ] 3a.4 REFACTOR: confirm `os/exec` and `net/*` remain absent from `engine/skills`'s reachable import set after the widening
- [ ] 3a.5 Verify: `cd engine && go vet ./... && go test ./skills/...`

## Phase 3b: `project-register-core` (PR 5, ~370 lines)

Item-30 symbols consumed: `NormalizeSlug`, `MatchCandidate`, `ParseRegistry`.

- [ ] 3b.1 RED `engine/skills/pathguard_test.go`: extract `withinRoot` behavior from the existing `install_test.go` traversal cases (regression net, unmodified) into a focused test for the new file
- [ ] 3b.2 GREEN `engine/skills/pathguard.go`: `withinRoot(root, p string) bool`, extracted from `PlanInstall`'s inline containment check with no behavior change; `resolvedWithinRoot` (runs `filepath.EvalSymlinks` on the deepest existing ancestor of each destination and the project root; refuses a symlinked `.claude`/`.agents` escaping the root)
- [ ] 3b.3 GREEN `engine/skills/install.go`: `PlanInstall` calls `withinRoot` (mechanical, no behavior change); confirm `install_test.go` passes unmodified
- [ ] 3b.4 RED `engine/skills/project_register_test.go`: registration over `t.TempDir()` writes every target (`.claude/skills/<id>/SKILL.md`, `.agents/skills/<id>/SKILL.md`) and the lock with the correct hash, mode 0644, never `.pi/`; refusals — traversal id (a parent-escaping id fixture, see design.md testing row 3b); symlinked `.claude` escaping root; foreign existing target dir not in lock; `allowed-tools`/`disable-model-invocation`/`model`/`hooks` frontmatter key present; draft path inside the project root; `\r` bytes in draft; registry match via `MatchCandidate`; any destination under `<root>/skills/`; rollback leaves the pre-run tree byte-identical with an injected failure at each rename index and during staging (tree snapshot before/after); Go `projectTargets` table equals the design's fixed two rows and neither names `.pi/skills`
- [ ] 3b.5 GREEN `engine/skills/project_register.go`: `ProjectTarget`, `projectTargets = []ProjectTarget{{"claude", ".claude/skills"}, {"agents", ".agents/skills"}}`, `ProjectWrite`, `ProjectPlan`, `PlanProjectRegister(in RegisterInput) (ProjectPlan, error)` implementing the validate-before-write order (§design "Validate-before-write", items 1-11), `ExecuteProjectPlan(p ProjectPlan, fsys projectFS, stdout, stderr io.Writer) error` implementing stage/commit/rollback over the injected `projectFS` interface
- [ ] 3b.5a RED `engine/skills/project_register_test.go`: using the injected `projectFS` fake, make its remove call fail during rollback itself (a rollback-of-rollback failure), asserting the process prints `error: rollback incomplete: <rel-path>` and exits 1, distinct from the ordinary rollback-succeeds cases in 3b.4
- [ ] 3b.6 REFACTOR: confirm the rollback path removes renamed new files, leftover temps, and only-if-empty created directories (deepest first) on every injected failure point, and prints `error: rollback incomplete: <rel-path>` + exit 1 on a rollback failure, as proven by 3b.5a
- [ ] 3b.7 Verify: `cd engine && go vet ./... && go test ./skills/...`

## Phase 4: `project-register-cli` (PR 6, ~310 lines)

Item-30 symbols consumed: sec 2 record block (`Registered` line wiring in the procedure, prose only).

- [ ] 4.1 RED `engine/skills/project_cli_test.go`: `project-register --project-root <abs> --candidate <key> [--dry-run] <draft>` flag parsing; missing `--project-root` exits 1 with nothing written; relative `--project-root` refused; `--dry-run` prints `plan: <rel>` lines and writes nothing; a successful non-dry-run write prints `wrote: <rel>`, `sha256: <hex>`, `revision: <n>`, and the exact `note: Pi loads .agents/skills only after the project is trusted; Pi may prompt once for project trust.` line; the `note:` line is present only on a successful write and absent on refusal or `--dry-run`; `skills-lock.json` bytes are unchanged in every case; wrapper-injected trailing flags tolerated
- [ ] 4.2 GREEN `engine/skills/project_cli.go`: `RenderProjectRegisterCore` implementing `project-register` and `--dry-run`, printing the Pi trust `note:` line after a successful write
- [ ] 4.3 GREEN `engine/skills/skills.go`, `engine/cmd/main.go`: dispatch `project-register`; usage strings
- [ ] 4.4 GREEN `skills/_shared/procedural-candidate-detection.md`: section 11 — registration and commit procedure (the 10-step agent git sequence: toplevel check, HEAD/in-progress-op check, empty-index check, `--dry-run` plan, `check-ignore`, real run, `add` + staged-set verification, `commit` with the exact conventional messages, `project-status` ownership confirmation, `rev-parse --short HEAD` recorded to `Registered`/`History`); crash-window recovery step (delete only `??`-and-hash-verified paths); acceptance checklist for `sdd-verify`
- [ ] 4.5 RED `engine/skills/procedural_candidate_contract_test.go`: extend to assert section 11 appears verbatim (the 10 steps, the three exact commit messages, the crash-recovery rule)
- [ ] 4.6 REFACTOR: confirm the CLI's printed paths are repo-relative with slashes and usable directly as a git pathspec
- [ ] 4.7 Verify: `cd engine && go vet ./... && go test ./skills/... ./cmd/...`
- [ ] 4.8 Acceptance checklist (non-Go, executed during `sdd-verify`, temp git repo with real Engram records): clean repo → exactly one commit whose changed paths equal the wrote set; unrelated staged file → refusal, index untouched; ignored target → refusal; detached HEAD → refusal; `git revert` restores files and lock; `History` only grows across draft→register; a staged-set mismatch after `git add -- <wrote paths>` (an unexpected path staged alongside them) → refusal, staged set unstaged via `git restore --staged`; a merge or rebase in progress (`MERGE_HEAD` or `rebase-merge`/`rebase-apply` present) → refusal; running the procedure from a subdirectory of the project root or from a nested repository root → resolved or refused per the design's `rev-parse --show-toplevel` check, never silently registering against the wrong root; a commit hook that rewrites the committed bytes so the post-commit hash differs from what was written → reported as human-owned by `project-status`, not auto-corrected; a second registration (for a different skill) that fails after `wrote:`/`sha256:` output but before its own commit → the crash-recovery step deletes only that run's untracked, hash-verified SKILL.md files and restores the lock file from `HEAD`, and a prior skill's lock entry and committed files survive unchanged

## Phase 5: `global-promotion` (PR 7, ~200 lines)

Item-30 symbols consumed: sec 2 record block (`Promoted` line, prose only).

- [ ] 5.1 RED `engine/skills/lifecycle_test.go`: extend — `AddCore` given a `t.TempDir()` fixture whose `skills/foo/SKILL.md` has a hard lint error (missing `metadata.version`) exits non-zero, names the hard error, and leaves registry/manifest bytes byte-for-byte unchanged; `AddCore` given a lint-clean-but-warnings-only fixture exits 0 and writes the registry entry exactly as before; every existing `AddCore` test (SC-65 through SC-68 and prior) still passes once fixtures are made lint-clean
- [ ] 5.2 GREEN `engine/skills/lifecycle.go`: `AddCore` reads `<src>/<id>/SKILL.md`, runs `LintSkillFile`, prints `[lint:<id>]` for each hard error and exits 1 before any `Serialize` call, runs after the existing SKILL.md-existence precondition and before both the manifest append and the registry serialize
- [ ] 5.3 GREEN `skills/_shared/procedural-candidate-detection.md`: section 12 — human promotion procedure over unmodified `engine skills add`, no new CLI, silence-is-not-consent statement, `Promoted` hash-line and `History` wiring
- [ ] 5.4 REFACTOR: confirm no partial write (manifest-only or registry-only) can occur on a lint refusal
- [ ] 5.5 Verify: `cd engine && go vet ./... && go test ./skills/...`

## Phase 6: `revision` (PR 8, ~300 lines)

Item-30 symbols consumed: sec 3 occurrence rules, sec 4 emission table (new rows, prose only).

- [ ] 6.1 RED `engine/skills/project_register_test.go`, `project_cli_test.go` (extend): `project-revise` refused per each `EvaluateOwnership` reason (`hash-mismatch`, `missing`, `extra-entry`) with the reason named in output and nothing written; a hash-matching revision bumps `revision`, recomputes `sha256`, and restores backup bytes on an injected mid-revision failure (rollback); `project-status` reports `owner:agent|human (<reason>)`
- [ ] 6.2 GREEN `engine/skills/project_register.go`: `project-revise` planning/execution path — ownership gate via `EvaluateOwnership`, revision-number bump, backup-capture-then-restore-on-failure (temp+rename pattern, same as new registration)
- [ ] 6.3 GREEN `engine/skills/project_cli.go`: `project-status` (ownership reporting) and `project-revise` CLI wiring
- [ ] 6.4 GREEN `skills/_shared/procedural-candidate-detection.md`: section 13 — revision trigger (`OccurrencesSincePromotion >= 2`, derived not stored), new emission-table rows for post-registration occurrences, the "qualifying occurrence" rule (failure-recovery recurrence, or repeated-success where the instruction was missing/wrong/rediscovered — simply following the skill does not count)
- [ ] 6.5 REFACTOR: confirm `project-revise` reuses the exact same stamp→lint→hash→write ordering as new registration (no divergent code path)
- [ ] 6.6 Verify: `cd engine && go vet ./... && go test ./skills/...`

## Phase 7a: `retirement-engine` (PR 9, ~250 lines)

Item-30 symbols consumed: `MatchCandidate`, sec 6.

- [ ] 7a.1 RED `engine/skills/project_register_test.go`, `project_cli_test.go` (extend): `project-retire` removes target files and the lock entry in one operation for an agent-owned skill; refused for a human-owned skill (reason named); `project-status` reports `superseded-by:<path>` when `MatchCandidate(registry, id)` or `MatchCandidate(registry, <last candidate slug>)` matches a global skill; `AbsorbedInto` write succeeds only when the named id exists (global: `MatchCandidate` against the overlay registry as an existence lookup, not coverage proof; project: an entry in the project lock) and is refused with the unverified target named otherwise; a failure injected mid-retirement (via the `projectFS` fake) triggers rollback of the partial delete, leaving the pre-retirement tree byte-identical, and a failure during that rollback itself prints `error: rollback incomplete: <rel-path>` and exits 1
- [ ] 7a.2 GREEN `engine/skills/project_register.go`: `project-retire` planning/execution (delete targets + lock entry, ownership-gated); `AbsorbedInto` existence verification helper
- [ ] 7a.3 GREEN `engine/skills/project_cli.go`: `project-retire` CLI wiring; `project-status` supersession reporting via `MatchCandidate`
- [ ] 7a.4 GREEN `skills/_shared/procedural-candidate-detection.md`: section 14 — retirement decision procedure, `RetirementReason` vocabulary (`stale-reference | superseded | absorbed | promoted | quiet | human-request`), `AbsorbedInto` verification rule, global-tier `RemoveCore` path (human, unmodified)
- [ ] 7a.5 REFACTOR: confirm `project-retire` never invokes removal automatically from a detector result — it is always a separate, explicitly invoked command
- [ ] 7a.6 Verify: `cd engine && go vet ./... && go test ./skills/...`

## Phase 7b: `retirement-detector` (PR 10, ~350 lines)

Item-30 symbols consumed: sec 2 field names (`LastObserved`, `Status`), read-only.

- [ ] 7b.1 RED `longterm-mem/internal/staleness/staleness_test.go` (extend): `ReferencedPaths(text)` extracts repo-shaped tokens from inline code spans and fenced blocks using the existing strict `filePath` pattern; `ClassifyPaths(repoRoot, paths)` reuses `indexTree`/`repohistory.Inspect` to classify each as removed-in-history vs still-present vs moved; `Detect`'s existing behavior and tests are unchanged
- [ ] 7b.2 GREEN `longterm-mem/internal/staleness/staleness.go`: export `ReferencedPaths`, `ClassifyPaths`, sitting beside `Detect` and sharing its private helpers, with no change to `Detect` itself
- [ ] 7b.3 RED `longterm-mem/internal/skillstale/skillstale_test.go` (new), fixture `longterm-mem/internal/skillstale/testdata/procedural-skills.lock.json`: detector over a temp git repo + fixture Engram DB — a skill naming a git-removed path is flagged `REMOVED <path> (by <commit>)` regardless of ordering; a renamed path yields `MOVED <path> -> <new>` only, never `REMOVED`; a clean skill (all paths present, all commands resolvable, `LastObserved` recent, no supersession) is not flagged; an unresolvable fenced command's first word yields `UNRESOLVED-COMMAND <name>` (resolved via `os.Stat` scan of `$PATH`, no `os/exec`); `LastObserved` older than 180 days yields `QUIET since <LastObserved>`; the detector performs zero file/record mutation (tree snapshot identical before/after, Engram store opened read-only); the fixture lock parses and `engine/skills/project_lock_test.go`'s `SerializeProjectLock` reproduces the shared fixture byte-for-byte (cross-module pinning, verified in 7b.6)
- [ ] 7b.4 GREEN `longterm-mem/internal/skillstale/skillstale.go`: the detector over the lock, the first target SKILL.md of each entry, `staleness.ReferencedPaths`/`ClassifyPaths`, `os.Stat`-based `$PATH` command resolution, and `engram.Store.ListObservations` filtered by `TopicKey` for `LastObserved`/`Status` (read-only)
- [ ] 7b.5 GREEN `longterm-mem/cmd/longterm-mem/cmd_skills_stale.go`, `main.go`: `longterm-mem skills-stale --project-root <abs> [--project <P>]`, report-only subcommand
- [ ] 7b.6 GREEN `engine/skills/project_lock_test.go`: add the pinning test asserting `SerializeProjectLock` reproduces `longterm-mem/internal/skillstale/testdata/procedural-skills.lock.json` byte-for-byte
- [ ] 7b.7 REFACTOR: confirm no new `os/exec`-importing package was introduced in `longterm-mem` (the module's existing `allowedExecImporters`/equivalent guard, if any, is unchanged) and that `skillstale` and `cmd_skills_stale.go` perform no write to the lock, the SKILL.md files, or Engram
- [ ] 7b.8 Verify: `cd longterm-mem && go vet ./... && go test ./internal/staleness/... ./internal/skillstale/... ./cmd/...`

## Phase Final: Full Verification (after PR 10 merges)

- [ ] F.1 `cd engine && go vet ./... && go test ./...`
- [ ] F.2 `cd longterm-mem && go vet ./... && go test ./...`
- [ ] F.3 Confirm no file was written under `skills/` by any project-tier code path outside `AddCore`'s own tests (`git diff --stat` review across all 10 merged PRs)
- [ ] F.4 Confirm no test in either module touched a live `.claude/skills/`, `.agents/skills/`, `.pi/`, `skills-lock.json`, the user's home directory, or spawned a live runtime binary (temp-dir-only rule)
- [ ] F.5 Run the full acceptance checklist from sections 11 (Phase 4), and the sdd-verify checklists implied by Phases 2, 6, 7a against real Engram records and a temp git repo; record results honestly, including any scenario that could not be exercised (e.g. the rollback-failure print path, or the Codex smoke test if `UNVERIFIED`)
- [ ] F.6 Confirm every Success Criteria checkbox in `proposal.md` is satisfied or explicitly recorded as a known gap (in particular the Codex-support claim, gated strictly by the recorded smoke-test verdict)
