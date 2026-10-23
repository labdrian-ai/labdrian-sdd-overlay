# Role agents — v3 slice 4: one source projected to Claude, Codex and Pi (Phase 9, PR 4 of `docs/architecture/role-design/per-role-design-v3.md` §6)

Objective: generalize `engine/gadu` into `engine/agentgen`, which projects every role from `roles/<role>/body.md` to Claude, Codex and Pi. Wire the Codex agent route and the Pi package agents. Branch `feat/phase9-agentgen` from `main` (`b5ade353`). Delivery: `ask-on-risk`, one PR, two work-unit commits: T1 generator, T2 deploy. Runners: `cd engine && go test ./...`, CI-form `skills validate`, and a differential of `labdrian apply --target <t>` against a base copy in an isolated HOME.

## Specs

Read v3 in full (D10, D11, D14, §2, §4, §5, §6, §9), `odd/tasks/role-agents-slice1.md` and `odd/tasks/role-agents-slice2.md` before any edit.

- **S1** — v3 §6 row 4, verbatim: "Projection: `engine/gadu` → `engine/agentgen` (N roles, Pi + Codex templates, `Check` in CI), `codex-agent` route + `AGENT_TARGET_PATHS[codex]`, pipkg directory copy, `pi_gadu.go` → package agents loop; capability table: Codex/Pi Skills claim detail gains one sentence about agents; live check per runtime (`codex` lists the agent, `subagent_list_agents` in Pi) recorded as evidence, not test". Risk named there: "item 5 (installer/deploy), item 3 (route domain error strings are pinned: `ondisk.go:102-104`, `routes.go:135`)". §6 trailer, verbatim: "differential check of `labdrian apply --target <t>` output against the base copy for PR 4-5".
- **S2** — Generator, v3 §5 first bullet, verbatim: "Source per role: `roles/<role>/body.md` (core prefix + delta) + the per-runtime frontmatter in one table in the generator (`tools`, `model`, `thinking`), as `gadu.go` keeps its model constants. Generator = `engine/gadu` generalized to N roles (rename to `engine/agentgen`, keep `Generate`/`Check`/pins), emitting `agents/<role>.md` (Claude), `pi/agents/<role>.md` (Pi: provider-prefixed model, Pi tool names `read, grep, find, bash, edit, write, codegraph`, `thinking`), `codex/agents/<role>.toml` (`name`, `description`, `developer_instructions = """<body>"""`, `model_reasoning_effort`; no `model`, see OD3). OpenCode variant not generated (not a runtime in use, memory `agent-runtimes-in-use`)." D14: "Codex `.toml` files omit `model` (the parent passes it at spawn)". v3 §2: "no stamped gentle blocks (v2 §1: they are gentle's sync markers; the generator never emits them)". GADU stays one of the projected agents until PR 5 deletes the persona.
- **S3** — Deploy, v3 §5 second bullet, verbatim: "Deploy: Claude unchanged (`agent` rows). Codex: add `AGENT_TARGET_PATHS[codex]=$HOME/.codex/agents` (`targets.sh:22-25`) and route `codex-agent` → `capability.TargetCodex` in `agentRouteTargets` (`routes.go:162-165`); rows `codex/agents/<role>.toml managed codex-agent`. [Amended by L3: the rows are `custom codex-agent`.] Pi: pipkg keeps copying `pi/agents/*.md` into the package (`build.go:141-147` generalized from GADU to the directory); `pi_gadu.go` generalized from one fixed name to every `agents/*.md` in the package, same ownership proof (Readlink equality), same conflict/stale/missing states, same Status lines; `linkGaduAgent` becomes `linkPackageAgents`. The `"agents"` key in the package manifest stays as the overlay's own inventory (harmless; Pi ignores it)."
- **S4** — Tool names, v3 §5 third bullet, verbatim: "Tool-name map per runtime is data in the generator; the exact MCP tool names (Engram, CodeGraph) on each runtime are **tested at implementation**, not assumed (carried unverified fact; gentle-pi drops unknown names silently)." Determine them from installed sources and record what was verified. Never probe them by running a live Pi or Codex.
- **S5** — Carried from slice 2 (L3 there): "the core prefix tells the reviewer to read Engram topics, but it has no Engram tool. The projection must drop that line for tool-less roles."
- **S6** — Consumers and safety:
  - Pinned route error strings and installer/manifest tests keep passing, or change only where the new route requires it, with the change stated.
  - The existing hand-written `agents/<role>.md` files become generator output. Any byte change to them must be intended (S5) and stated.
  - Tests never touch a live Pi, Codex or Claude install (memory `tests-touch-live-pi`). They use a temp HOME and STATE_DIR, with `LABDRIAN_PI_BIN` set.
  - The live check per runtime is a separate owner-authorized step after the PR's checks, not part of the writer's work.
- **S7** — Scope fence:
  - No deletion of `agents/sdd-*`, `review-*`, `jd-*` or the GADU persona (PR 5).
  - No CI ratchets (PR 6), apart from adding the `Check` step S1 names.
  - No D11 Codex `codex exec` launcher (not in row 4).
  - No OpenCode role variants.
  - No change to role bodies beyond S5.

## Tasks

| ID | Specs | Route | Status | Commit / evidence |
|---|---|---|---|---|
| T1 | S1, S2, S4, S5, S6, S7 | delegated workflow (owner: "sonnet max para implementaciones"); generator, templates, generated files, `Check` in CI | done | `61a31b70` |
| T2 | S1, S3, S6, S7 | same workflow, after T1; Codex route, pipkg directory copy, Pi package agents loop, capability sentence | done | `84349dcd`; review fixes `c8b32dcd`, `f76113dd` |
| T3 | S1 (live check) | owner-authorized, after merge-ready | pending | — |

## Log

- **L1** (2026-10-21, owner): "bueno cotinua en ese orden ve en ultracode con opus high para las verify y sonnet max para implementaciones".
- **L2** (2026-10-22, owner): after #667 merged, I asked "¿Borro el worktree `role-agents-s3` y arranco el PR 4?" Owner: "si porfa".
- **L3** (2026-10-23): workflow `wf_834e68bc-27e` ran a Sonnet max writer and two Opus high verifiers (spec and adversarial), with one correction and a recheck.
  - **Verdicts:** S2 pass. S1 and S3 partial: T3 (live check) is pending, plus the blockers below.
  - **Differential:** `labdrian apply --target claude|codex|pi|opencode|all` against base in an isolated HOME. Every run exits 0. Every difference is spec-required:
    - `reviewer.md` loses 1 line (S5);
    - codex gains 8 `.toml` files;
    - pi gains 8 package agents and their links;
    - the `engine/agentgen` manifest rows are renamed.
  - **B1 (parent fixed):** 4 tests were red until two out-of-surface companion patches were applied. They touch `engine/architecture_test.go`, `bin/lib/usage.sh`, `engine/cmd/{manifest_test,usage,memory_acceptance_test}.go` and `engine/shelltest/overlay_pi_package_build_test.go`. After them, `go test ./...` passes 70 packages; only the 3 known environmental failures remain.
  - **B2 (correction):** the Pi Skills capability detail had dropped "so the real pi never loads the package". The clause is restored and pinned. Recheck pass.
  - **Owner blocker (codex rows `managed` vs `custom`):** I asked whether to keep `custom` or go back to `managed`. Owner: "Aceptar custom (Recomendado)". The reason: capture and bootstrap walk only `managed` rows, and every other overlay agent row is already `custom`. S3 is amended, and the correction note in v3 §5 stays.
  - **Name kept:** the CLI command keeps the name `gadu-generate`; only the package is renamed.
- **L4** (2026-10-23): native review.
  - **Both commits together:** `b5ade353..84349dcd` was refused with `lens_context_budget_exceeded`, so each commit was reviewed as its own candidate.
  - **T2:** `review-1b287747549001ca` (risk, resilience, readability, reliability), approved and acknowledged. Findings:
    - R3/R4 (uninstall leaves owned links when the package `agents/` is gone): fixed in `c8b32dcd`, test-first, with the new test RED then GREEN.
    - R2 (the GADU label): documented in `agentLabel`.
    - R2 (the route domain listed in three places): pre-existing, and tests pin the agreement. Follow-up.
  - **T1:** `review-31d072b1220c45e2` (risk, readability, reliability), approved and acknowledged, reviewed in a temporary detached worktree. Findings:
    - The Check header named only GADU: fixed in `f76113dd`; it now reads "generated agent artifacts are stale or missing:", and the pins are updated.
    - The README sentence about the old header path is simplified.
    - The opus and haiku Pi model refs: confirmed in Pi 1.3.0's anthropic catalog (`chat:claude-opus-5`, `chat:claude-haiku-4-5`). That they actually run stays T3's proof.
    - The Engram-line drop needs the exact line: it fails loudly in Check, so this is accepted.
  - **Receipts:** `odd/review-receipts/role-agents-slice4/`.
  - **Suite:** the full engine suite after the fixes shows only the 3 known environmental failures.
- **L5** (2026-10-23): review of the fix commits (`84349dcd..ebc586b9`).
  - `review-af65599034467589` (resilience, readability, reliability), approved and acknowledged, no blockers.
  - R3/R4 (a listing error is swallowed when only one side fails): fixed in the next commit, test-first. Uninstall now tells an unreadable package agents directory, or an unreadable links directory, after the links it still removed. A missing package `agents/` stays silent.
  - R3 suggestion (a rebuild that drops one agent): pinned by a test, which already passed.
  - R2 suggestion (the partial-failure branch): documented in the comment of `unlinkPackageAgents`.
