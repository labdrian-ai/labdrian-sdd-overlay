# Role agents — v3 slice 5: retire the superseded agents and GADU (Phase 9, PR 5 of `docs/architecture/role-design/per-role-design-v3.md` §6)

Objective: delete the agents and skills the eight role agents replace, together with their manifest rows, and every consumer that names them. Branch `feat/phase9-retire-agents` from `main` (`fd41c0c0`). Delivery: `ask-on-risk`, one PR, work-unit commits per task. Runners: `cd engine && go test ./...`, CI-form `skills validate`, `gadu-generate --check`, and a differential of `labdrian apply --target <t>` against a base copy in an isolated HOME.

## Specs

Read v3 in full (D5, D10, §2, §4, §5, §6, §9), v2 §1-§2 and §5, and `odd/tasks/role-agents-slice4.md` before any edit.

- **S1** — v3 §6 row 5, verbatim: "Deletion: `agents/sdd-*.md`, repo copies of `review-*`/`jd-*` (byte-identical to gentle's; rows + files together, R §7), `agents/GADU.md`, `opencode/agents/GADU.md`, `pi/agents/GADU.md`, `skills/gadu-operator`, `gadu-orchestrate` (or keep as orchestrator-only reference, v2 OD4), GADU engine persona and README mentions; `sdd-phase-common.md` shrunk to §B-D". Risk named there, verbatim: "item 3 (manifest rows and names others consume), item 5, item 1 (uninstall of the Pi link)". §6 trailer, verbatim: "differential check of `labdrian apply --target <t>` output against the base copy for PR 4-5".
- **S2** — OD4 is resolved by the owner (L3): `skills/gadu-orchestrate` is retired, deleted like `gadu-operator`.
- **S3** — Consumers (memory `consumer-impact-rule`):
  - Enumerate every reader of each deleted name and path: skills, the registry, the manifest, engine code and tests, routing pins, capability declarations, the Pi package build, CI, README and docs, and the orchestrator and skill text that launches `sdd-*` agents.
  - Each reader is updated to the role agents or the deletion, or is listed with a reason if it must stay.
  - The list goes in the Log with path:line evidence.
- **S4** — Deployed copies and the Pi link (v3 item 1):
  - State what `labdrian apply` does with a deployed file whose manifest row is gone, for each target: `~/.claude/agents/sdd-*.md`, `GADU.md`, the OpenCode GADU, and the Pi package plus the `~/.pi/agent/agents/GADU.md` link.
  - After `apply --target pi`, no overlay-owned link may point to a file the package no longer ships. The ownership proof is the existing Readlink equality.
  - A file the overlay does not own is never removed or rewritten. That covers gentle-ai's own `review-*`, `jd-*` and `gentle-ai-*` agents.
  - Any removal of deployed files is shown in the differential and stated.
- **S5** — Engine persona: GADU's projection leaves `engine/agentgen` (the persona, its constants, its artifacts and its pins). The eight roles' outputs stay byte-identical. The CLI command name `gadu-generate` is not renamed in this slice: no spec asks for it. Report it as a follow-up.
- **S6** — Global skills:
  - Deleted skills leave the registry and the manifest through the repository's own retire or removal path. Find that path; do not hand-edit around it.
  - Every remaining global skill whose bytes change needs the owner's `labdrian skills approve`, which an agent cannot run. List those ids as `needs_owner`.
- **S7** — Safety:
  - Tests never touch a live Pi, Codex or Claude install (memory `tests-touch-live-pi`). They use a temp HOME, STATE_DIR and GOCACHE, with `LABDRIAN_PI_BIN` set.
  - The live deploy of this PR is a separate owner-authorized step after merge.
- **S8** — Scope fence:
  - No CI ratchets (PR 6).
  - No change to the eight role bodies or their generated outputs.
  - No change to the gentle review lifecycle.
  - No new agents.

## Tasks

| ID | Specs | Route | Status | Commit / evidence |
|---|---|---|---|---|
| T1 | S1, S3, S4, S5, S7, S8 | delegated workflow (owner, L1: "sonnet max para implementaciones"); agents and GADU: files, rows, engine persona, the Pi link, consumers | pending | — |
| T2 | S1, S2, S3, S6, S8 | same workflow, after T1; skills: `gadu-operator`, `gadu-orchestrate`, `sdd-phase-common.md` shrunk to §B-D, registry, consumers | pending | — |
| T3 | S4 (live) | owner-authorized after merge: `labdrian apply` on the real targets | pending | — |
| T4 | slice 4 carry | record slice 4's T3 live check in `odd/tasks/role-agents-slice4.md` | done | first commit of the branch |

## Log

- **L1** (2026-10-21, owner): "bueno cotinua en ese orden ve en ultracode con opus high para las verify y sonnet max para implementaciones".
- **L2** (2026-10-23, owner): after the slice 4 live check I asked "¿Borro el worktree `role-agents-s4` y arranco el PR 5, el de las eliminaciones?" Owner: "si porfa".
- **L3** (2026-10-23, owner): I asked "En el PR 5, ¿qué hacemos con `skills/gadu-orchestrate` (v2 OD4)?" Owner: "Retirarlo (Recomendado)".
