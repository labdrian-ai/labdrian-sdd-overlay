## SDD Workflow (Spec-Driven Development)

SDD is the structured planning layer for substantial changes. This file is the lazy-loaded Claude Code workflow surface; read it before handling `/sdd-*`, SDD meta-commands, SDD/Judgment-Day phase delegation, or SDD continuation/routing.

### Artifact Store Policy

- `engram` — default when available; persistent memory across sessions.
- `openspec` — file-based artifacts; use only when the user explicitly requests it or a change already exists there.
- `hybrid` — both backends; useful for team-shareable files plus cross-session recovery.
- `none` — return results inline only; recommend enabling engram or openspec.

### Commands

Skills and slash commands:

- `/sdd-init` → initialize SDD context; detects stack and testing capabilities.
- `/sdd-explore <topic>` → investigate an idea; no implementation.
- `/sdd-status [change]` → read-only structured status.
- `/sdd-apply [change]` → implement pending tasks in batches.
- `/sdd-verify [change]` → validate implementation against specs/tasks.
- `/sdd-archive [change]` → close a completed change.
- `/sdd-onboard` → guided end-to-end walkthrough.

Meta-commands are handled by the orchestrator directly and do not appear in autocomplete:

- `/sdd-new <change>` → run exploration then proposal.
- `/sdd-continue [change]` → run the next dependency-ready phase.
- `/sdd-ff <name>` → fast-forward proposal → specs → design → tasks.

### Native SDD Dispatcher Guard

Before routing, continuing, applying, verifying, or archiving an SDD change, first determine this session's artifact store. The native dispatcher (`gentle-ai sdd-continue [change] --cwd <repo>` or `gentle-ai sdd-status [change] --cwd <repo> --json --instructions`) reads only OpenSpec file artifacts and always emits `artifactStore: openspec`; it cannot observe Engram-backed changes.

- For `engram`, do NOT invoke the dispatcher. Resolve status from Engram topic keys with `mem_search` followed by `mem_get_observation`.
- For `openspec` or `hybrid`, use the dispatcher when available and treat its JSON as authoritative over prompt inference.
- Route only by structured `nextRecommended`, dependency states, and `blockedReasons`; never infer from free text.
- If blocked, stop and report the blocker. Do not proceed to apply, archive, or terminal work.

### SDD Session Preflight (HARD GATE)

Before executing ANY SDD command or natural-language SDD request, ensure this session has an explicit `SDD Session Preflight` decision block.

This applies to `/sdd-new`, `/sdd-ff`, `/sdd-continue`, `/sdd-explore`, `/sdd-status`, `/sdd-apply`, `/sdd-verify`, `/sdd-archive`, and natural-language equivalents such as "use SDD to add dark mode" / "do it with SDD".

Required preflight choices:

1. **Execution mode**: `interactive` or `auto`.
2. **Artifact store**: `openspec`, `engram`, or `hybrid` when Engram is callable. If Engram is unavailable, offer only file/inline-safe choices.
3. **Chained PR strategy**: the canonical `delivery_strategy` — `ask-on-risk`, `auto-chain`, `single-pr`, or `exception-ok`. The preflight menu offers the first three; `exception-ok` is reachable only when the user explicitly accepts `size:exception`.
4. **Review budget**: maximum changed lines before stopping for reviewer-burden approval.

User-facing preflight question format:

Use the built-in `AskUserQuestion` tool for SDD Session Preflight only when it is available in the current interactive runtime and all four groups are exactly representable. While that native route is usable, do NOT render a duplicate plain-chat menu. If the tool is unavailable, denied, the runtime is noninteractive, or the prompt is unrepresentable, follow the Lossless Blocking Prompts fallback in the orchestrator rule and STOP.

When the native route is representable, ask all four preflight groups in one single `AskUserQuestion` tool call so Claude Code can render the groups as one interactive prompt. Do NOT run this as a sequential wizard. Do NOT issue four separate `AskUserQuestion` tool calls.

The single `AskUserQuestion` tool call must contain these four localized groups in this order:

1. Pace: Interactive, Automatic.
2. Artifacts: OpenSpec, Engram, Both.
3. PRs: Ask me, Single PR, Auto.
4. Review: 400 lines, 800 lines, Other.

Match the user's current language and active persona for question labels and descriptions. Treat the preflight UI as direct orchestrator conversation, not as a generated technical artifact. Technical artifacts still default to English, but this UI follows the user's conversation language/persona. Do NOT mix languages inside one grouped question.

Do NOT show option codes in the interactive UI. Do NOT show canonical values or other internal values in the interactive UI labels or descriptions.

After the single grouped `AskUserQuestion` tool call returns, map the selected human labels to canonical values internally. Do not reveal the canonical values in the UI.

If Other is selected for review budget, ask one follow-up question for the numeric budget.

Only after all four preflight choices are collected, summarize them as the `SDD Session Preflight` decision block and continue with the SDD init guard/requested phase.

Map answers to canonical values:

- Pace: Interactive -> `interactive`; Automatic -> `auto`.
- Artifacts: OpenSpec -> `openspec`; Engram -> `engram`; Both -> `hybrid`.
- PRs: Ask me -> `ask-on-risk`; Single PR -> `single-pr`; Auto -> `auto-chain`.
- Review: 400 lines -> `review_budget_lines: 400`; 800 lines -> `review_budget_lines: 800`; Other -> ask one follow-up for the number.

The PR canonical values are exactly the `delivery_strategy` domain `sdd-tasks` and `sdd-apply` accept; never emit a value outside it. The preflight offers no separate chained option because `delivery_strategy` is only consulted once the tasks forecast flags review-budget risk: below that line there is nothing to chain, and above it `Auto` already resolves to `auto-chain` without asking again.

Hard gate rules:

- `openspec/config.yaml`, existing SDD artifacts, previous `sdd-init` results, or installed SDD assets do NOT satisfy session preflight. That exclusion is unchanged and absolute: none of them records a user decision, so none of them can stand in for one.
- A **validated entry contract** DOES satisfy session preflight — but only for the four values it caches, and only under the operational definition of "validated" below. It is the single exception to the bullet above.
- If the session has no preflight block and no validated entry contract, ask the single grouped `AskUserQuestion` preflight above. Do not run init, delegate phases, edit files, or apply tasks until all four choices are collected.
- Cache the choices for this session and include them in later phase prompts.
- If the user explicitly provided all four choices in the current conversation, summarize them as the session preflight block and continue.

#### Validated Entry Contract as Preflight Evidence

`inception-pipeline` already collects these four values, normalizes them, and persists them at `sdd/{change-name}/entry`. Asking again is not extra safety — it invites the user to answer differently from the contract the change was validated against, and leaves the engine running on a value the artifacts do not record.

"Validated" is a mechanical result, never a reading of the file. An entry contract satisfies preflight only when ALL of these hold:

1. The object is the one persisted at topic key `sdd/{change-name}/entry` (writer: `inception-pipeline`; see Topic Keys), or the exact candidate bytes that were persisted there.
2. Its `contract_version` is one the installed contract bundle supports. The bundle is a compatibility set, not an exact-match lock: the installed `skills/_shared/entry-contract.schema.json` accepts any supported version and new contracts declare the current one. An unsupported version fails closed; an older but supported one does not. `skills/_shared/pre-sdd-contracts.md` is the authority on which versions are currently supported — read it rather than assuming.
3. `labdrian validate-entry-contract --schema skills/_shared/entry-contract.schema.json --instance <candidate-path>` exited `0` for those exact bytes — in this session, or as an inception-pipeline result carried into this session with the exit code stated. Non-zero exit, a missing validator, or an unstated exit code all fail closed.

Reading the JSON, checking that the fields look present, or accepting a claim that inception validated it is NOT validation. If no exit-0 result is available for the persisted bytes, re-run the validator against them before using the contract as preflight evidence. Schema shape alone is also not enough: the validator enforces the ordering, path, range, and delivery invariants the schema cannot express.

When all three hold, map the contract to the preflight block instead of asking:

| Preflight choice     | Entry contract field                       | Canonical value                                    |
| -------------------- | ------------------------------------------ | -------------------------------------------------- |
| Execution mode       | `interaction_mode`                         | `interactive` \| `auto`                             |
| Artifact store       | `artifact_store_mode`                      | `openspec` \| `engram` \| `hybrid` \| `none`        |
| Chained PR strategy  | `delivery_strategy`                        | `single-pr` \| `auto-chain` \| `exception-ok`       |
| Review budget        | `review_budget.max_changed_lines_per_slice`| integer, lines                                      |

Then summarize them as the `SDD Session Preflight` decision block exactly as if the user had answered, and add one provenance line so the source is auditable: `source: sdd/{change-name}/entry, contract_version <version>, validator exit 0`. Continue with the init guard / requested phase.

Scope limits on this exception:

- It satisfies **only** those four rows. Every other preflight or scope decision is still asked.
- `delivery_strategy` from the contract is already resolved, so `ask-on-risk` can never arrive this way — see Delivery Strategy for why that is correct and not a missing value.
- `chain_strategy` is cached by the same contract and satisfies the Chain Strategy ask under the identical validation rule.
- One change's entry contract satisfies preflight for that change only. Switching to a different change re-runs this gate against that change's own entry contract.
- If the contract is present but fails any of the three conditions, fall through to the `AskUserQuestion` preflight and report why the contract was rejected. Do NOT silently prefer the contract, and do NOT silently prefer the user's answer over a valid contract without saying the two disagree.

### SDD Entry Routing (MANDATORY)

For a new product/code change request that says to use SDD, start at preflight -> init guard -> explore/proposal (`/sdd-new` equivalent). Never launch `sdd-apply` just because the user asked to implement a feature.

Only launch `sdd-apply` when all are true:

1. Session preflight is complete.
2. The active change has existing spec, design, and tasks artifacts.
3. The user explicitly asked to apply/continue implementation, or the prior SDD planning phase completed and the orchestrator has passed the review workload guard.

If any dependency is missing, STOP and propose `/sdd-new` or `/sdd-ff`; do not implement.

### SDD Init Guard (MANDATORY)

Before executing any SDD command or meta-command, check whether `sdd-init` has run for this project:

1. Search Engram: `mem_search(query: "sdd-init/{project}", project: "{project}")`.
2. If found, proceed normally.
3. If not found, run `sdd-init` first, then continue with the requested command.

This ensures testing capabilities, Strict TDD mode, and project context are available to later phases.

### Execution Mode

This is collected by `SDD Session Preflight`. If missing, enforce the hard gate before any phase work. Cache the collected mode for the session:

- **Automatic** (`auto`): phases run back-to-back without pausing, but the orchestrator gatekeeper validates after each phase before launching the next.
- **Interactive** (`interactive`): after each phase, show a concise summary and ask whether to adjust or continue.

If the user doesn't specify, default to **Automatic**. After scope approval, expect zero further prompts on the happy path and at most one actionable prompt per recoverable failure; the gatekeeper summarizes phase progress instead of interrupting except on a second consecutive gate failure or a genuine scope/product decision. Interactive approval is phase-scoped; words like "continue", "dale", or "go on" approve only the immediate next phase.

Before the `sdd-propose` phase in interactive mode, offer the user a proposal question round focused on business/product understanding, business problem, business rules, outcomes, implications and impact, edge cases, scope boundaries, non-goals, constraints, and product tradeoffs. Do not ask about test commands, PR shape, changed-line budget, or other harness mechanics unless the user explicitly asks.

### Automatic Mode Gatekeeper (MANDATORY)

In Automatic mode, the orchestrator validates every delegated phase result before launching the next phase. The gatekeeper runs after every phase and before launching the next sub-agent.

Gate checks:

- **Contract conformance:** returned `status`, `executive_summary`, `artifacts`, `next_recommended`, `risks`, and `skill_resolution`; status is not partial/failed/blocked.
- **Artifact existence:** declared artifact is readable in the active backend.
- **No hallucination:** claimed files, symbols, commands, and artifacts exist.
- **No drift from inputs:** proposal/spec/design/tasks/apply outputs stay consistent with their dependencies.
- **Routing coherence:** `next_recommended` follows the dependency graph and no unaddressed CRITICAL risk remains.

Hybrid validation:

- Inline for low-risk phases: `sdd-explore`, `sdd-spec`, `sdd-tasks`, `sdd-archive`.
- Fresh-context phase-contract validator for `sdd-design` and `sdd-apply`: validate only the phase artifact against its inputs. This is not adversarial implementation review, inspects no code diff, and creates no 4R/Judgment-Day budget.
- Escalate to fresh-context review when an inline gate smells wrong.

On gate failure, re-run the same phase exactly once with specific corrective feedback. If the second result fails, STOP the automatic chain and report; do not advance dependent phases.

### Native Runtime Attempt Authority (MANDATORY)

Use the provider-owned Git-common-dir runtime ledger for every runtime-bearing `sdd-apply`, `sdd-verify`, or remediation continuation. It is the single attempt/budget authority for both OpenSpec and Engram; never persist caller-authored counters in OpenSpec files, Engram topics, prompts, or Pi state.

1. Before an actor or harness launch, call `gentle-ai sdd-attempt acquire --cwd <repo> --change <change> --request-id <id> --work-unit <label> --evidence-goal <goal> --max-attempts <count> --max-changed-lines <count>`.
   - Exception: when this launch is a phase actor started BY a parent that already ran this exact acquire and got `state: proceed`, do not acquire blind — pass the parent's returned token as `--token <token>` on the actor's own acquire call. A matching token proves the actor is continuing that SAME attempt and returns `proceed` with zero ledger mutation; acquiring without it collides with the parent's own active attempt and deadlocks on `blocked: active_attempt` (#2291).
2. Launch only when acquire returns `state: proceed`, and retain its opaque `token`. `blocked` or `complete` stops the launch.
3. After the external run, call `gentle-ai sdd-attempt settle --cwd <repo> --change <change> --token <token> --request-id <settle-id> ...` with a request ID distinct from the acquire operation's request ID, outcome, and bounded evidence. Reuse each operation's own ID only for its idempotent replay. Settle derives native binding/remediation inputs; pass `--successor-lineage` only for a distinct approved successor, otherwise the bound lineage remains its own successor.
4. Route only from settle's `proceed`, `blocked`, or `complete` state. Full `status|begin|finish|reset` operations are diagnostic/compatibility surfaces; reset requires an explicit maintainer scope decision and is never automatic.

### Artifact Store Mode

This is collected by `SDD Session Preflight`. If missing, enforce the hard gate before any phase work. Cache the collected store (`engram`, `openspec`, `hybrid`, or `none`) for the session. If unspecified, default to `engram` when Engram is available; otherwise use `none` and explain the persistence limitation.

Pass the artifact store mode to every SDD phase agent.

### Delivery Strategy

On the first SDD chain request in a session, ask once for delivery strategy and cache it:

- `ask-on-risk` — default; ask only when the tasks forecast detects review-budget risk.
- `auto-chain` — automatically split into chained/stacked PR slices when needed.
- `single-pr` — proceed as one PR only if the size is within budget.
- `exception-ok` — user accepts `size:exception` when over budget. The preflight menu cannot select this; it is reached only when the user explicitly accepts `size:exception`, either up front or when `ask-on-risk` stops to ask.

These four are the whole domain **of the workflow's own `delivery_strategy`**. Pass `delivery_strategy` to `sdd-tasks` and `sdd-apply`.

#### Delivery and Chain Vocabulary Map (MANDATORY)

Four separate vocabularies describe delivery and chaining across this system. They are not synonyms and they do not have equal domains. Never assume a token means the same thing on another surface; map it here first.

**Delivery — caller intent to resolved outcome.** `requested_pr_strategy` is what the caller asked for; `delivery_strategy` is what that resolved to after the review budget was applied. Only the resolved value is stored in the entry contract, and only the resolved value reaches `sdd-tasks` and `sdd-apply`.

| `requested_pr_strategy` (schema, caller intent) | Resolves to `delivery_strategy` (schema, resolved)                                          | Workflow branch used by the Review Workload Guard |
| ----------------------------------------------- | -------------------------------------------------------------------------------------------- | -------------------------------------------------- |
| `auto`                                          | `single-pr` when every slice is within budget; `auto-chain` when chaining is required; `exception-ok` only with an approved size exception | the resolved token, one of the three                |
| `force-chained`                                 | `auto-chain`, with `chaining_required: true` and a non-`none` `chain_strategy`                 | `auto-chain`                                        |
| `force-single`                                  | `single-pr`, or `exception-ok` only with an approved size exception                            | `single-pr` or `exception-ok`                       |
| *(no counterpart — see below)*                  | *(never stored)*                                                                               | `ask-on-risk`                                       |

**Why `ask-on-risk` has no schema counterpart.** It is an UNRESOLVED policy, not a delivery outcome. It says "if the tasks forecast flags review-budget risk, stop and ask the user" — it names a question still to be asked, not a shape the PRs will take. The entry contract stores only RESOLVED values: by the time it validates, that question has already been answered, and the answer is one of `single-pr`, `auto-chain`, or `exception-ok`. A contract carrying `ask-on-risk` would assert that a decision it claims to have made is still open. So the schema's three-token domain and the workflow's four-token domain are both correct, and the missing token is not a gap.

Two consequences follow: a `delivery_strategy` arriving from a validated entry contract can never be `ask-on-risk`, and `ask-on-risk` can only ever be a live session choice that must resolve to one of the other three before `sdd-apply` runs.

**Chaining — one concept, four spellings.**

| Surface                                                    | Domain                                                                 | How to read it                                                                                                |
| ---------------------------------------------------------- | ---------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- |
| `chain_strategy` in `entry-contract.schema.json`            | `none` \| `feature-branch-chain` \| `stacked-to-main`                    | Authoritative stored value. `none` is correct and required when `chaining_required: false`.                     |
| `chain_strategy` in this workflow (Chain Strategy, below)   | `stacked-to-main` \| `feature-branch-chain`                              | The two topologies offered to the user. Never asked when chaining is not required — that case is schema `none`. |
| `Chain strategy:` literal in the `sdd-tasks` forecast       | `stacked-to-main` \| `feature-branch-chain` \| `size-exception` \| `pending` | Only the first two are topologies. See the reconciliation rule in Chain Strategy before routing on this line.    |
| `chained-pr` reference prose (`references/chaining-details.md`) | narrative only, no machine tokens                                      | Human guidance. Never parse it and never route from it.                                                          |

`stacked-to-main` is the single token both machine vocabularies share, and it is the only reason this map has never produced a routing failure in practice: all three real contracts to date chose it. That is luck, not a guard.

### Chain Strategy

When delivery planning yields chained PRs, ask once for chain strategy and cache it:

- `stacked-to-main` — each PR targets the previous PR branch or main in sequence.
- `feature-branch-chain` — PR #1 targets the tracker branch; child PRs target the immediate previous PR branch; only the tracker merges to main.

A third value exists but is never asked: `none`, which the entry contract stores when `chaining_required` is `false`. It is a legal `chain_strategy`, so treat `none` as "not chaining" and route by `delivery_strategy` alone. Do NOT ask the user for a topology when the value is `none`, and do NOT pass `none` to `sdd-apply` as if it were a topology — `sdd-apply` has no branch for it.

**Unknown-value guard (MANDATORY).** Any `chain_strategy` value outside `stacked-to-main`, `feature-branch-chain`, and `none` is invalid. Do NOT pick the nearest branch, do NOT default to `stacked-to-main` because it is the common case, and do NOT proceed: STOP, report the unrecognised value and where it came from (entry contract, tasks forecast, or session cache), and re-collect the chain strategy before launching `sdd-apply`. This mirrors the identical rule for `delivery_strategy` in the Review Workload Guard, which this section previously lacked. The exposure is real and only latent: `sdd-apply` branches on `stacked-to-main` and `feature-branch-chain` and nothing else, so any other value reaches implementation with no branch to take.

**Reconciling the `sdd-tasks` forecast literal.** The `Chain strategy:` line `sdd-tasks` emits admits two extra tokens that are not chain topologies:

- `size-exception` is a **delivery** fact, not a topology. It means the change ships as one PR with an approved budget overrun — already fully expressed by `delivery_strategy: exception-ok`, `chaining_required: false`, `chain_strategy: none`, and `review_budget.size_exception.state: approved`. Carrying it in the chain field duplicates a delivery decision in a topology slot and makes the field unmappable to the schema.
- `pending` is a **null state**, not a value. It means the decision has not been made, which is what an absent field already means.

`sdd-tasks` SHOULD stop emitting either token in that field: the chain field's job is to answer "which branch does each PR target", and neither token answers it. Until that emission is fixed, apply this transitional read so today's artifacts still route — and treat every application of it as a defect to remove, not as a supported mapping:

| Forecast literal  | Read as                                                                         | Action                                                                            |
| ------------------ | -------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------- |
| `stacked-to-main`  | `chain_strategy: stacked-to-main`                                                 | Route normally.                                                                       |
| `feature-branch-chain` | `chain_strategy: feature-branch-chain`                                        | Route normally.                                                                       |
| `size-exception`   | `chain_strategy: none` + `delivery_strategy: exception-ok`                        | Require the recorded `size:exception` approval before apply; never treat as chaining. |
| `pending`          | no value                                                                          | Decision outstanding: ask for the chain strategy; never pass `pending` downstream.     |

Never forward `size-exception` or `pending` to `sdd-apply` or into an entry contract. Both fail the schema, and the unknown-value guard above will stop the run.

When chained PRs are selected, treat `chained-pr` (registry skill `gentle-ai-chained-pr`) as a required skill match. Resolve and forward it by registry path to `sdd-tasks` and `sdd-apply`; do not hardcode its path.

Pass it as `chain_strategy` to `sdd-tasks` and `sdd-apply` prompts alongside `delivery_strategy`.

### Dependency Graph

```text
proposal -> specs --> tasks -> apply -> verify -> archive
             ^
             |
           design
```

### Result Contract

Every SDD phase returns: `status`, `executive_summary`, `artifacts`, `next_recommended`, `risks`, and `skill_resolution`.

### Review Workload Guard (MANDATORY)

After `sdd-tasks` completes and before launching `sdd-apply`, inspect `Review Workload Forecast`.

If it says `Chained PRs recommended: Yes`, `400-line budget risk: High`, estimated changed lines exceed 400, or `Decision needed before apply: Yes`, apply cached `delivery_strategy`:

- `ask-on-risk`: stop and ask whether to split or proceed with `size:exception`.
- `auto-chain`: split automatically; ask for `chain_strategy` only if missing.
- `single-pr`: stop and require/record `size:exception` before apply.
- `exception-ok`: continue and tell `sdd-apply` this run uses `size:exception`.

Any other `delivery_strategy` value is invalid. Do NOT pick the nearest branch and do NOT proceed: STOP, report the unrecognised value, and re-collect the delivery strategy before launching `sdd-apply`.

Always pass the resolved `delivery_strategy`, `chain_strategy`, and PR boundary/exception to `sdd-apply`.

When launching `sdd-apply`, always include the resolved `delivery_strategy`, `chain_strategy`, and any chosen PR boundary/exception in the prompt.

#### Plan vs Realized Slice Count (MANDATORY)

The forecast check above compares an estimate against a budget. This check compares the **plan against reality**, which nothing in the engine did before: `review_slices` in the entry contract has a producer (`inception-pipeline`), a validator (`entry-contract-validator`), and a human reader (`openspec/project/roadmap.md`), but until this rule it had zero consumers inside the SDD engine. Nobody noticed a plan diverging from delivery while the work was still running.

Define, for the active change:

- **P** = `len(review_slices)` in the validated entry contract at `sdd/{change-name}/entry`. Use it only when that contract satisfies the validation conditions in SDD Session Preflight. If there is no validated entry contract, P is undefined and this check is skipped — record the skip, because a chained change running without a slice plan is itself worth reporting.
- **R** = realized slices delivered so far: PRs opened for this change under the chosen `chain_strategy`, or, when not delivering via PRs, `sdd-apply` batches that carry their own review boundary. Count what exists, never what was intended.

Recompute R at every `sdd-apply` batch boundary, and again before `sdd-verify` and before `sdd-archive`.

| Condition                                       | Verdict           | Action                                                                                                                                                                                                                       |
| ------------------------------------------------ | ------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `R <= P + max(1, ceil(0.2 * P))`                 | Within tolerance   | Continue. Record `slices planned=P realized=R` in `apply-progress`.                                                                                                                                                              |
| Above tolerance and `R <= 2 * P`                 | Drift              | Continue, but record `slice drift: planned=P realized=R` in `apply-progress` at the batch where it crossed, and repeat it in the archive report. Re-check the review budget: more slices than planned usually means slices were split.       |
| `R > 2 * P`                                      | Plan invalidated   | **STOP before launching the next batch.** Report P, R, and where the crossing happened. Resume only after either (a) `sdd-tasks` re-plans the remaining scope and a re-validated entry contract rewrites `review_slices`, or (b) the user explicitly accepts the new count and that acceptance is recorded in the session. |
| At `sdd-verify` or `sdd-archive`, `R < P`        | Under-delivery     | Name every planned slice with no realized counterpart in the verify or archive report. A plan whose slices were never delivered is drift in the other direction, not success.                                                     |

The `±20%` band (floored at ±1) exists so the ordinary case — one slice turning out to be two — never interrupts the run, while the 2× line marks the point where the plan has stopped describing the work rather than merely mis-sizing it. A single re-plan is the correct response there; repeatedly widening the tolerance is not.

Worked example — the `longterm-mem` change. Planned P = 18 (`review_slices`, orders 1..18, contiguous). Realized R = 82 PRs (one draft plus 81 in a contiguous range). Tolerance was 22 and the hard-stop line was 36; both were crossed long before the change finished, and the run never paused. The drift **was** recorded — in `tasks.md`, in several places in `apply-progress.md`, and in the archive report — but every one of those was human-authored prose written after the fact. No code, validator, guard, or CI job ever compared the planned slice count to the delivered one. This rule is the first thing that does, and it is prose in a file with no test coverage: it holds only as long as the orchestrator honors it.

<!-- gentle-ai:sdd-model-assignments -->
## Model Assignments

Read this table at session start (or before first SDD/Judgment-Day delegation), cache it for the session, and use the mapped alias only for SDD/Judgment-Day phase agents. If an SDD/Judgment-Day phase is missing, use the `default` fallback row. If you do not have access to the assigned model (for example, no Opus access), substitute `sonnet` and continue.

The Claude Code session model is controlled by Claude Code itself; Gentle AI does not configure the main orchestrator model. This table applies only to Agent tool calls for SDD/Judgment-Day phase sub-agents, not generic delegation.

**Mandatory phase model gate:** Agent tool calls for SDD/Judgment-Day phase agents MUST include `model`. Generic/non-SDD delegation MUST NOT use this table; omit `model` unless the user explicitly requested an override. Before each SDD/Judgment-Day Agent call, resolve the target phase to an alias from this table.

| Phase | Default Model | Effort | Reason |
|-------|---------------|--------|--------|
| sdd-explore | sonnet | high | Reads code, structural - not architectural |
| sdd-propose | fable | default | Architectural decisions |
| sdd-spec | sonnet | high | Structured writing |
| sdd-design | fable | default | Architecture decisions |
| sdd-tasks | sonnet | high | Mechanical breakdown |
| sdd-apply | sonnet | high | Implementation |
| sdd-verify | opus | default | Validation against spec |
| sdd-archive | haiku | default | Copy and close |
| sdd-onboard | haiku | default | Guided walkthrough, pedagogical |
| jd-judge-a | opus | default | Adversarial review — blind judge A |
| jd-judge-b | opus | default | Adversarial review — blind judge B |
| jd-fix-agent | opus | default | Surgical fixes from confirmed issues |
| default | sonnet | max | SDD/JD phase fallback |

<!-- /gentle-ai:sdd-model-assignments -->

### Sub-Agent Launch Deduplication (MANDATORY)

Maintain a session-scoped launch log of `(phase, task-fingerprint)` pairs. If the same pair already exists, do NOT launch again. Emit exactly one launch per distinct task and append the pair after launch.

### Sub-Agent Launch Protocol

ALL sub-agent launch prompts that involve reading, writing, or reviewing code MUST include pre-resolved skill paths from the skill registry. Follow `~/.claude/skills/_shared/skill-resolver.md`.

Pre-flight before every SDD/Judgment-Day Agent call:

1. Identify the phase key (`sdd-apply`, `sdd-verify`, `jd-judge-a`, etc.).
2. Look up the model alias in the Model Assignments table.
3. Include `model: "<alias>"` in SDD/Judgment-Day Agent calls.
4. For generic/non-SDD delegation, omit `model` unless the user explicitly requested one.

Resolve skills once per session, cache the registry, and pass exact `SKILL.md` paths. If a delegated result reports `skill_resolution` as `fallback-registry`, `fallback-path`, or `none`, re-read the registry before subsequent delegations.

**Key Learnings closing (generic delegations):** When delegating to generic agents (Explore, general-purpose), instruct the sub-agent to close its final message with a `## Key Learnings` section containing 1–5 numbered items, each a standalone factual sentence of ≥4 words and ≥20 characters. This enables engram passive capture of learnings across delegation boundaries. SDD phase agents load this requirement from `~/.claude/skills/_shared/sdd-phase-common.md` section F automatically.

### Context Protocol

Sub-agents start with fresh context. The orchestrator controls what context they receive.

For non-SDD delegation:

- Orchestrator searches Engram for relevant prior context and passes it in the prompt.
- Sub-agent saves significant discoveries, decisions, and bug fixes to Engram before returning.
- Orchestrator forwards exact skill paths.

For SDD phases, sub-agents read/write the active backend directly using artifact references, not copied artifact bodies.

| Phase          | Reads                                                                 | Writes           |
| -------------- | --------------------------------------------------------------------- | ---------------- |
| orchestrator   | `entry` (validated, optional) — preflight and routing, never written   | nothing          |
| `sdd-explore`  | nothing                                                               | `explore`        |
| `sdd-propose`  | exploration (optional)                                                | `proposal`       |
| `sdd-spec`     | proposal (required)                                                   | `spec`           |
| `sdd-design`   | proposal (required)                                                   | `design`         |
| `sdd-tasks`    | spec + design (required) + `entry` (optional)                         | `tasks`          |
| `sdd-apply`    | tasks + spec + design + apply-progress if present + `entry` (optional)| `apply-progress` |
| `sdd-verify`   | spec + tasks + apply-progress + `entry` (optional)                    | `verify-report`  |
| `sdd-archive`  | all artifacts + `entry` (optional)                                    | `archive-report` |

The `entry` contract is written by `inception-pipeline`, never by an SDD phase — the engine is a reader only, and must not create, edit, or re-validate it in place. Read it for the four cached preflight values, `review_slices` (Plan vs Realized Slice Count), and `chain_strategy`; treat it as absent unless it satisfies the validation conditions in SDD Session Preflight. `actuals` is likewise engine-external: it is written after archive by `inception-pipeline` closure-feedback, which is its only writer, so no SDD phase reads or writes it.

### Strict TDD Forwarding (MANDATORY)

When launching `sdd-apply` or `sdd-verify`, search for testing capabilities (`sdd-init/{project}`). If `strict_tdd: true`, add: `STRICT TDD MODE IS ACTIVE. Test runner: {test_command}. You MUST follow strict-tdd.md. Do NOT fall back to Standard Mode.`

### Apply-Progress Continuity (MANDATORY)

When launching `sdd-apply` after prior batches, search for `sdd/{change-name}/apply-progress`. If it exists, tell the sub-agent to read it first, merge new progress into it, and save the combined result. Do not overwrite.

### Archive Final-State Handoff (MANDATORY)

When launching `sdd-archive`, forward explicit final-state facts for any work completed after `apply-progress` or `verify-report` were persisted — verify warnings fixed in later commits, blockers resolved, tasks finished, updated test or issue counts — with commit or evidence references where available. Those two artifacts are intermediate snapshots, valid at the time they were written; the archive report records the state at close, and explicit final-state facts in the `sdd-archive` launch prompt outrank stale snapshot claims.

### Topic Keys

| Artifact        | Topic Key                          |
| --------------- | ---------------------------------- |
| Project context | `sdd-init/{project}`               |
| Exploration     | `sdd/{change-name}/explore`        |
| Proposal        | `sdd/{change-name}/proposal`       |
| Spec            | `sdd/{change-name}/spec`           |
| Design          | `sdd/{change-name}/design`         |
| Tasks           | `sdd/{change-name}/tasks`          |
| Apply progress  | `sdd/{change-name}/apply-progress` |
| Verify report   | `sdd/{change-name}/verify-report`  |
| Archive report  | `sdd/{change-name}/archive-report` |
| DAG state       | `sdd/{change-name}/state`          |
| Entry contract  | `sdd/{change-name}/entry`          |
| Closure actuals | `sdd/{change-name}/actuals`        |

Sub-agents retrieve full Engram content in two steps: `mem_search(query: "{topic_key}", project: "{project}")`, then `mem_get_observation(id)`.

The last two keys are **engine-external**: every other key in this table is written by the SDD phase that owns it, but `entry` is written by `inception-pipeline` before handoff and `actuals` by `inception-pipeline` closure-feedback after archive. The engine reads `entry` (preflight values, `review_slices`, `chain_strategy`) and neither reads nor writes `actuals`. Never write either key from an SDD phase, and never re-derive one from the other artifacts. They are listed here because they are addressed by this workflow — a key the orchestrator resolves but the table omits is a key nobody maintains, which is how both stayed invisible to the engine.

### State and Conventions

Convention files live under the agent's global skills directory, including `engram-convention.md`, `persistence-contract.md`, and `openspec-convention.md`.

### Recovery

- `engram` → `mem_search(...)` → `mem_get_observation(...)`.
- `openspec` → read `openspec/changes/*/state.yaml` and artifacts.
- `none` → state is not persisted; explain the limitation.
