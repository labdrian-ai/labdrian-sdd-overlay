# Proposal: Project-Scoped Skill Install (issue #29, slice 2)

## Intent

Slice 1 shipped `skills.registry.yaml` + read-only `skills list/status/validate`, but every skill is still GLOBAL: `defaultScope` is locked to `global` and deploy hits only the three user-level `TARGET_PATHS`. Issue #29 wants per-project skills. Verification (engram `skill-package-manager/project-scope-feasibility`) confirmed Claude Code discovers project-local skills at `<repo>/.claude/skills/<name>/SKILL.md` (precedence Personal > Project > Plugin); the filesystem boundary IS the scoping mechanism. This is the FIRST slice that touches deployment — slice 1 deliberately left `cmd_apply` untouched. We add a project scope to the schema and a way to install project-scoped skills into a target repo, without changing the global pipeline.

## Scope

### In Scope
- Schema: allow `install.defaultScope: project` (alongside `global`); add optional `install.allowedProjects` (list of repo identifiers a project skill may install into).
- Parser/validator updates in `engine/skills/` (strict TDD): accept `project` scope; parse/validate `allowedProjects`; keep `global` entries valid unchanged.
- New `labdrian skills install` verb, run FROM WITHIN a target repo (cwd = project): copies each project-scoped, allowed skill from the overlay repo's `skills/<id>/` to `<repo>/.claude/skills/<id>/`.
- Bash passthrough in `bin/labdrian-overlay` `cmd_skills` (additive forward; vendored `bin/overlay` untouched).
- Fail-loud on unknown project, missing source dir, or non-project-scoped install attempt.

### Out of Scope (non-goals)
- opencode/codex project-local install paths (unverified — CLAUDE `.claude/skills/` only this slice).
- `skills add/remove`, external sources, pinned-ref, manifest generation from registry.
- Any change to global `cmd_apply` / `cmd_capture` deploy/sync.
- Uninstall/cleanup of project-installed skills (likely next slice — note only).

## Capabilities

### New Capabilities
- `skills-project-install`: `skills install` verb semantics — resolve project-scoped + allowed skills, copy overlay `skills/<id>/` → `<repo>/.claude/skills/<id>/`, fail-loud rules, idempotent re-install.

### Modified Capabilities
- `skills-registry`: schema gains `install.defaultScope: project` and optional `install.allowedProjects[]`; `global` semantics unchanged.
- `skills-cli`: `engine skills` / `labdrian skills` dispatch gains the `install` verb alongside list/status/validate.

## Approach

Option A (recommended; design decides). Run install FROM the target repo: cwd identifies the project, so no name→filesystem-path registry is needed (avoids Option B's project-path map and global-apply coupling). The engine resolves entries with `defaultScope: project` whose `allowedProjects` admits the current project, then copies each `skills/<id>/` tree from the overlay repo into `./.claude/skills/<id>/`. Source dir = overlay `skills/<id>/`; target = target repo `.claude/skills/<id>/`. Global skills are ignored by install; project skills are ignored by the global pipeline — the two paths stay fully additive. Engine logic is unit-tested (strict TDD, Go, zero-dep parser); bash forwarder verified with `bash -n` + e2e.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `engine/skills/types.go` | Modified | `Install.AllowedProjects []string`; scope accepts `project` |
| `engine/skills/parse.go` | Modified | Parse `allowedProjects`; relax `defaultScope` validation |
| `engine/skills/install.go` | New | Resolve + copy project skills into `<repo>/.claude/skills/` |
| `engine/skills/skills.go` | Modified | Dispatch `install` verb |
| `bin/labdrian-overlay` | Modified | `cmd_skills` forwards `install` (no global-pipeline change) |
| `skills.registry.yaml` | Modified | At least one `project` entry to exercise the path |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| `defaultScope` relax breaks global validation | Low | TDD: keep all `global` golden cases green |
| opencode/codex assumed but unverified | Med | Scope to `.claude/` only; defer as non-goal |
| Install copies into wrong cwd | Med | Resolve target as `$PWD/.claude/skills`; fail-loud if not a repo |
| Overwriting user-edited project skill on re-install | Med | Idempotent copy; document overwrite; uninstall deferred |

## Rollback

Remove `engine/skills/install.go`, revert the `install` dispatch and parser/validator scope changes, drop the `cmd_skills install` forward, revert any `project` entries in `skills.registry.yaml`. Global deploy/capture untouched → zero effect on `apply`/`capture`.

## Assumptions (automatic mode — no question round)

- Project identity for `allowedProjects` matching = git remote / repo name resolved from cwd (design to fix exact key).
- One entry per skill; `path`/`id` already kebab and stable from slice 1.
- `allowedProjects` empty/absent on a `project` skill → not installable anywhere (explicit allow-list), pending design confirmation.
- opencode/codex project paths deferred; confirm later before extending targets.
- Re-install overwrites; cleanup/uninstall is the next slice.

## Dependencies

- Slice 1 merged (registry + skills-cli). Verification memo `skill-package-manager/project-scope-feasibility` (done).

## Success Criteria

- [ ] Registry accepts `defaultScope: project` + `allowedProjects[]`; all slice-1 `global` cases still validate.
- [ ] `labdrian skills install` from a target repo copies allowed project skills into `<repo>/.claude/skills/<id>/`.
- [ ] Fail-loud on unknown project, missing source dir, or installing a non-project skill.
- [ ] `engine/skills/` install logic unit-tested (strict TDD); `bin/overlay` byte-identical; global pipeline unchanged.
