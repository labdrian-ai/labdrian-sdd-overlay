# Role agents — v3 slice 2 (Phase 9, PR 2 of `docs/architecture/role-design/per-role-design-v3.md` §6)

Objective: the reviewer and verifier role agents, the orchestrator-run `role-chain` skill with the parent duties, and the `odd/review-results/` convention, per the approved v3 design. Branch `feat/phase9-role-agents-s2` from `main` (`24f5a7ad`). Delivery: `ask-on-risk`, one PR. Runners: `cd engine && go test ./...`, CI-form `skills validate`, skills lint, structural readback for prose.

## Specs

Every spec quotes the approved design (v3); read it, `per-role-reviewer.md` (R) §2-7 and v2 §2 before any edit. Slice 1 (`odd/tasks/role-agents-slice1.md`) is the pattern to follow: `roles/<role>/body.md` + `agents/<role>.md`, `custom agent` manifest rows, pins in `engine/skills/role_agents_artifact_test.go`.

- **S1** — v3 §6 row 2, verbatim: "`reviewer.md`, `verifier.md`, `skills/role-chain/SKILL.md` (parent duties §3), `odd/review-results/` convention; sdd-* and gentle files untouched". Risk named there: "none (additive); verifier runs nothing the parent did not authorize".
- **S2** — Reviewer and verifier agents, v3 §2 rows verbatim: "reviewer | `reviewer.md` | `Read, Grep, Glob` | sonnet (alias) | R §6 reviewer (4 lines) | after sweeper/polisher; second blind launch (other model) when the Risk line names an item; readability row by trigger" and "verifier | `verifier.md` | `Read, Grep, Glob, Bash, mem_get_observation` | sonnet | R §6 verifier (3 lines) | reproduces every severe finding; runs the authorized checks". D4 verbatim: "`review-*` → our `reviewer.md` (Read/Grep/Glob over a detached worktree at `head` + diff in prompt, bound by `base`/`head`/`diff_sha256`) and `verifier.md` (runs checks, reproduces severe findings); `model: sonnet` alias + per-launch override on refusal; default one launch reliability+resilience, readability by trigger, risk as a checklist line; no refuter; second blind launch with another model when the Risk line names a high-risk item; results under `odd/review-results/<feature>/<head12>-<n>.json`; gentle's eight managed files and lifecycle untouched (compatible mode)".
- **S3** — `role-chain` skill, v3 §3 verbatim: "one orchestrator-run skill `skills/role-chain/SKILL.md` within the lint budget (`engine/skills/lint.go:42-44`), `disable-model-invocation: true`, deployed to all three runtimes by the existing skill routes". Its content is the §3 parent duties (Appends, Triggers, Worktrees, Binding, Persistence) and the §3 order "`[prototyper] → shaper → estimator → builder → [sweeper] → [polisher] → reviewer → delivery`". "If the lint refuses the size, the sweeper trigger splits into `skills/sweeper/SKILL.md` as S drafted; nothing else splits."
- **S4** — Carried from slice 1's native review (L6 there): R1-001/R1-002 — prototyper and delivery path confinement is prompt-only; the `role-chain` skill owns it (§3 Worktrees: spikes in `…/spike-<goal>`, reviews in `…/review-<head12>`, never under the repo or the scratchpad; delivery writes only the paths the parent names).
- **S5** — Scope fence: no estimator rewrite (PR 3), no generator/projection (PR 4), no deletions (PR 5), no CI ratchets (PR 6). Existing agents, the gentle review lifecycle, and their manifest rows stay untouched. The minimalism contract already excludes reviewer/verifier (slice 1); keep it.
- **S6** — Consumers keep working (slice 1 lesson: the gate, `engine propagate`, the Pi gate `engine/pipkg/labdrian-gate.ts` and the OpenCode prompt config all read shared contracts). A new global skill needs an owner approval record: the writer must not create `.approval.json`; it reports the digest for `labdrian skills approve --id role-chain --approver <name> --source-root skills`.

## Tasks

| ID | Specs | Route | Status | Commit / evidence |
|---|---|---|---|---|
| T1 | S1-S6 | delegated (owner: "sonnet max para implementaciones"), workflow implement → verify (opus high) → one correction | done | wf_601cedcd-8f4 (L3), commit L4 |

## Log

- **L1** (2026-10-21, owner): "bueno cotinua en ese orden ve en ultracode con opus high para las verify y sonnet max para implementaciones".
- **L2** (2026-10-22, owner): after #664/#665 merged, asked "¿Lo arranco?" for PR 2 — owner: "si arranca porfa".
- **L3** (2026-10-22, workflow wf_601cedcd-8f4): Sonnet writer reported S1-S6 covered: RED 19 tests before the files, GREEN after. Engine binaries are byte-identical to base, and the `manifest routes` and `gate-task` differentials only add rows. `role-chain` lint is clean at 699 of 700 advisory tokens. Two Opus verifiers (spec, adversarial) passed S1-S6 with 0 blockers, so no correction ran. Mutations M1-M6 were each caught. A scratch approval turned CI validate and the two `APPROVAL_MISSING` tests green. Owner step: approve `role-chain`, sha256 `4af6c0a5c57f52a999360fe2bb88a82226f173ddca70df608153c7689ee778a2`. Advisory dispositions:
  - Accepted:
    - Reviewer delta line 3 inlines the R §4 shape instead of the dangling "(the shape in §4)" pointer. The meaning is unchanged.
    - `SKILL.md` retry-with-another-model also covers an echo or paths mismatch. R §4 re-runs every `interrupted` review; the model switch is harmless there.
    - The `<head12>-<n>.verifier.md` filename is the convention this PR defines.
    - Verifier lists only the `mcp__plugin_engram_engram__` prefix. The manual `engram` server was removed (memory `sdd-agent-engram-tool-prefix`).
    - `disable-model-invocation: true` matches v3 §3 verbatim.
  - Carried to PR 4 (generator projects the prefix per role): the core prefix tells the reviewer to read Engram topics, but it has no Engram tool. The projection must drop that line for tool-less roles.
  - Dropped to fit the lint budget, outside v3 §3's duty text: R §6 "one Agent call per launch in a single message" and R §3 `git show head:path` spot check.
- **L4** (2026-10-22): owner ran `labdrian skills approve --id role-chain --approver labdrian-ai --source-root skills` ("approved: role-chain", sha256 `4af6c0a5…ee778a2`). CI-form `skills validate` rc=0 (40 skills, 8 approved); `go test ./skills/ ./gate/` ok. Committed as the feature's single work unit. `gentle-ai review assess --base-ref 24f5a7ad --committed-only`: risk `medium`, `review_due: false` (`under_budget`); no native review started, both Opus verifiers already covered it.
