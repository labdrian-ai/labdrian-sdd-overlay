# ODD Task — Canonical standalone platform roadmap

## Objective
Persist the approved general-to-deep TDD roadmap for Goal/Workflow contracts, shaper, simplified roles, standalone lifecycle, procedural memory, runtime adapters, skills lifecycle, observability, and release gates.

## Authorized scope
- Create an OpenSpec decision artifact for the approved roadmap.
- Create an Engram canonical mirror under the project scope.
- Promote the Engram observation through the existing longterm-mem workflow.
- Do not modify implementation code, runtime state, foreign configuration, or existing historical OpenSpec artifacts.

## Acceptance criteria
- OpenSpec artifact exists and identifies itself as the canonical roadmap decision without overwriting `openspec/project/roadmap.md`.
- Engram observation contains the complete roadmap and references the OpenSpec artifact.
- Longterm-mem promotion succeeds with project confinement and provenance.
- All failures or partial outcomes are recorded honestly.

## Checks
- Read back the OpenSpec artifact.
- Retrieve the Engram observation.
- Run longterm-mem status/query or promotion verification as available.

## Progress
- [x] OpenSpec artifact — `openspec/decisions/standalone-platform-roadmap.md`
- [x] Engram mirror — topic `project/labdrian-sdd-overlay/standalone-platform-roadmap`, observation 3597
- [x] Longterm-mem promotion — vault page `c-000122`
- [x] Read-back and evidence — longterm-mem query returned observation 3597; embedding index reported partial coverage (247/900 live observations unindexed)

## Evidence

- Promotion command: `cd longterm-mem && go run ./cmd/longterm-mem promote --project labdrian-sdd-overlay --id 3597`
- Promotion result: `longterm-mem: promoted c-000122 (created)`
- Verification query: `cd longterm-mem && go run ./cmd/longterm-mem query --project labdrian-sdd-overlay --top 5 --json "standalone platform roadmap"`
- Verification result: observation 3597 returned; diagnostics reported an incomplete embedding index, which does not invalidate the exact FTS hit.

## Phase 0 baseline reconciliation

- Branch: `feat/procedural-memory-lifecycle-7b-retirement-detector`, tracking origin.
- Worktree: intentionally dirty; procedural-memory lifecycle files are deleted from the working tree as part of the existing transition, while archived artifacts, standalone docs, `.agents/`, `.claude/skills/`, `.pi/`, and `skills-lock.json` remain unrelated/uncommitted state. No existing changes were altered.
- Go modules identified: `longterm-mem`, `engine`, `tui`, and five `tools/*` modules.
- Active OpenSpec changes: `longterm-mem-knowledge-ingestion`, `overlay-versioned-releases`, `runtime-roster`, `skill-lifecycle`, `skill-manifest-gen`, `skill-package-manager`, and `skill-project-scope`.
- Verification: longterm-mem, engine, tui, and all five tools passed `go test ./...` and `go vet ./...`; shell syntax and ShellCheck passed for `bin/overlay`; `git diff --check` passed. Exact report: Linux 7.0.0-31-generic x86_64, Go 1.26.5 linux/amd64 from `/home/linuxbrew/.linuxbrew/bin/go`, Git 2.43.0, Bash 5.2.21, ShellCheck available. All requested paths/tools were available; no failures or unavailable checks.
- Known evidence gaps: authenticated Claude workflow, clean isolated Codex support, Laya/OS distribution, and public release smoke evidence remain unverified or blocked as documented in the standalone roadmap.
- Delegated mapping attempt timed out during grep; existing prior mapping plus direct repository state and verification evidence were used instead. No implementation files or runtime state were modified in Phase 0.
