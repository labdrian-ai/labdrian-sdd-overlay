# Exploration: procedural-memory-lifecycle (roadmap items 31-35)

Date: 2026-09-18. Scope: one SDD change covering skill drafting (31), approval (32), registration (33), revision (34) and retirement (35), delivered as sequential slices.

Premises: Engram #3426 (two-tier autonomy: the agent registers skills autonomously only inside the project it works in; promotion to the global overlay `skills/` stays human-gated; mandatory guardrails: LintSkill hard gate, declared provenance + ownership-by-hash, git commit per registration with git revert as rollback, post-promotion occurrence revision trigger, do-not-capture list). Engram #3420 (Hermes-agent synthesis; its §3.4 human-only registration is superseded by #3426 at the project tier).

## Premise re-verification (against live code)

1. **Item 30 is incomplete — confirmed.** `openspec/changes/procedural-candidate-detection/tasks.md`: phase 1 (candidate store) done, phases 2-4 pending. `engine/skills/match.go` holds only `NormalizeSlug`/`truncateSlug`; `MatchCandidate` does not exist. This change consumes item 30's candidate record schema, topic-key contract and `MatchCandidate`. No apply of this change runs before item 30 is merged.
2. **`engine skills install` is reusable and Claude-only — confirmed.** `engine/skills/install.go` (`PlanInstall`/`ExecuteInstall`/`RenderInstallCore`) copies `defaultScope: project` entries to `<targetRoot>/.claude/skills/<id>` (hardcoded) with the R-055 traversal guard on src and dst. `RenderInstallCore` has no runtime-target parameter.
3. **`docs/skill-style-guide.md` is absent — confirmed.** `skills/skill-creator/SKILL.md` cites it as normative, with a bundled `references/skill-style-guide.md` fallback and inline fallback rules; none is machine-enforced.
4. **The ondisk CI gate covers only the global `skills/` tree — confirmed.** `engine/skills/ondisk.go` (`DiffOnDisk`/`DeployableManifestPaths`) never inspects `.claude/skills/`, `.agents/skills/` or `.pi/`. Drafts cannot live under `skills/`; project-local registration cannot trip this gate.
5. **Engram upsert overwrites — confirmed.** `skills/_shared/procedural-candidate-detection.md` already designs around it (append-only `Occurrences`, derived `OccurrenceCount`); the same shape is the precedent for an append-only `History`.

## Project-local skill discovery per runtime

| Runtime | Project-local skill dir | Evidence |
|---|---|---|
| Claude Code | `.claude/skills/<id>/SKILL.md` | `engine/skills/install.go`; live repo state |
| Pi | `.pi/skills/` and `.agents/skills/` (cwd and ancestors up to the git root) | VERIFIED in installed docs: `@earendil-works/pi-coding-agent/docs/skills.md` lines 27-39, `docs/sdk.md` 346-348. Note: Pi prompts for project trust when project `.agents/skills` exists (`docs/settings.md` line 14) |
| Codex (0.148.0) | `.agents/skills/<id>/` (assumed) | Structural precedent only: live `.agents/skills/archify/` + root `skills-lock.json`. Binary strings show `~/.codex/skills` and `<repo-root>/.agents/plugins/`, but no literal `.agents/skills`. UNVERIFIED; design must confirm with a live smoke test |

Consequence: if Codex confirms `.agents/skills/`, two target dirs (`.claude/skills/`, `.agents/skills/`) cover all three runtimes in use. opencode is not used.

## Current state

- `engine/skills/types.go`: `Registry`/`Entry` have no room for provenance or hash; `parse.go` (strict zero-dependency YAML subset) rejects unknown keys.
- `engine/skills/lifecycle.go`: pure `AddEntry`/`RemoveEntry` plus human-invoked `AddCore`/`RemoveCore` (atomic manifest-then-registry dual write, validate-before-write re-parse).
- `engine/skills/ondisk.go`: global-tree gate; its maintained route allowlist is a reusable pattern.
- `skills/_shared/procedural-candidate-detection.md`: sections 1-3 only (identity, record shape: `Kind`, `Candidate`, `Aliases`, `Status: observing|emitted|rejected`, `Threshold`, `OccurrenceCount`, `Occurrences`, `Summary`); stops before drafting.
- `longterm-mem/internal/staleness`: pure, report-only detector (deleted vs renamed vs unresolved root via git history), directly reusable for retirement.
- Root `skills-lock.json` with `computedHash` per skill: a live ownership-by-hash precedent for externally sourced project-local skills.
- `engine/` zero-dependency invariant (ADR-15, `TestZeroFetchImportAllowlist`) and the `os/exec` ban inside `engine/`: hashing and frontmatter code must be stdlib-only.

## Affected areas

- `skills/_shared/procedural-candidate-detection.md` (amend: do-not-capture list, lesson shape, Disposition, Status vocabulary extension, History).
- `engine/skills/match.go` (`MatchCandidate` is an item-30 input).
- New pure `LintSkill(frontmatter, body) (hard []error, warnings []Warning)` beside `parse.go`.
- `engine/skills/install.go` (extend or sibling: multi-target project-tier register with lock-file write).
- `engine/skills/lifecycle.go` (`AddCore`/`RemoveCore` reused for the global tier; LintSkill as a gate).
- `skills/skill-creator/SKILL.md` and `docs/skill-style-guide.md` (single source of truth for lint rules).
- `longterm-mem/internal/staleness` (reused for retirement).

## Approaches for project-tier registration

1. **Sibling of `install.go` with a per-project lock file (recommended).** Reuses the traversal guard and validate-before-write discipline; writes the SKILL.md into each runtime target dir; records id, provenance, candidate key and sha256 in a project lock file. Effort: medium.
2. **Engram plus per-runtime shell-out.** Rejected: `engine/` bans `os/exec`, adds indirection. Effort: medium-high.
3. **Global registry/manifest reuse for project skills.** Rejected: the manifest describes the overlay, not consumer projects; would couple every project to the overlay repo.

## Candidate slice decomposition (sequential, each independently mergeable)

- Slice 0 (prerequisite, owned by item 30): `MatchCandidate` and contract sections 4-6 merged.
- Slice 1: `LintSkill` + style-guide single source of truth.
- Slice 2: contract amendment: do-not-capture list, lesson shape, Disposition, draft record `procedural/drafts/{kind}/{slug}`, History.
- Slice 3: project-tier registration (multi-target, lock file, provenance frontmatter, hash, LintSkill gate, commit).
- Slice 4: project to overlay promotion (human; reuses `AddCore` with LintSkill gate).
- Slice 5: revision (ownership-by-hash check, `OccurrencesSincePromotion >= 2` trigger, draft path).
- Slice 6: retirement (report-only detector over `internal/staleness`, human removal, `AbsorbedInto` via `MatchCandidate`).

## Risks

- Hard dependency on item 30 phases 2-4.
- Codex project-local skill discovery unverified.
- Pi trust prompt fires when project `.agents/skills` appears: an agent-created dir changes the user's next Pi startup.
- `docs/skill-style-guide.md` disposition undecided.
- Where ownership-by-hash is recorded (lock file, Engram record, or both) undecided.
- No skill-load telemetry; `OccurrencesSincePromotion` is a proxy.
- Live `.claude/skills/`, `.agents/skills/archify` and `skills-lock.json` in this repo are developer state, not fixtures; tests must use temp dirs (see the tests-touch-live-Pi precedent).
- Registration writes into consumer repos: the git-commit guardrail must not commit unrelated staged work.

## Open product decisions for the human

- Style guide: doc as source vs Go rules as source with generated doc.
- Lock file: reuse the `skills-lock.json` schema (shared with external installers) or a separate overlay-owned file.
- Whether the agent's registration commit is automatic or left staged.
- Whether a project-tier skill needs `Status: registered` distinct from `promoted` (global).

## Ready for proposal

Yes.
