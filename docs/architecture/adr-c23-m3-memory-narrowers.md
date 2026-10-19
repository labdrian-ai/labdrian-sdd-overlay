# ADR: goal and handoff narrowers stored in the workflow log (C23, M3)

- **Status:** Accepted, 2026-10-19; implementation in batch 54 of phase 9.
- **Decided by:** the owner, on 2026-10-18 and 2026-10-19.
- **Ledger:** `odd/tasks/consolidation-phase9.md`, task C23, unit M3 (gap G6). M1's record is
  `docs/architecture/adr-c23-m1-memory-sources.md`; M2's is
  `docs/architecture/adr-c23-m2-project-identity.md`.

## Context

A memory plan is a profile's ceiling narrowed by directives (`memoryscope.Resolve`,
`engine/memoryscope/memoryscope.go:184`): a narrower may lower the scope (`none < goal < project`)
and keep a subset of the sources, never more. Before M3 only `labdrian memory plan` applied
narrowers, read from the files named by `--goal-directive` and `--handoff-directive`
(`engine/cmd/memory.go:160`). Nothing stored a narrower:

- The Goal's `memory_scope` is free text, checked only for being non-blank
  (`engine/goal/goal.go:32`, `:99`); no memory code reads it.
- The shaper handoff v3 has the same free-text `memory_scope` (`engine/shaper/handoff_v3.go:178`).
- The role handoff (`engine/roles/handoff.go:49`) has no memory field at all.
- The runtime plan, which the context line and the memory gate share, is the profile's ceiling with
  no narrower (`engine/projection/context.go:110`).

So a goal or a handoff could not narrow the memory a workflow reads.

## Decision

### The owner's six decisions (and a seventh on `memory plan`)

1. **The Goal carries a typed directive.** A Goal v3 replaces `memory_scope` with
   `memory_directive`, the same strict object as a MemoryDirective v1 (`version`, `scope`,
   `sources`, `write`). Goals v1 and v2 are unchanged and narrow nothing.
2. **Both handoffs carry one.** A shaper handoff v4 replaces `memory_scope` with
   `memory_directive`. A role handoff v2 may carry an optional `memory_directive`: the role that
   hands off narrows what the next role reads.
3. **The log holds every narrower.** `workflow create` copies the Goal's directive, the shaper
   handoff's directive (from a new `--handoff <file>`) and the directive of the role chain's head
   into the created event. Each later role handoff enters as a new `narrowed` event, which a new
   command, `workflow narrow`, appends after reading the bound chain. The gate and the context read
   only the log, so a workflow stays self-contained, as with the profile snapshot (Q6).
4. **The latest pass rules, over the base.** The base is the profile's ceiling narrowed by the
   Goal, then by the shaper handoff. The role directive in force is the newest one (the created
   event's, replaced by each `narrowed`), applied over the base. A later role can get back to the
   base, never past it. A role handoff with no directive means the base.
5. **A widening directive is refused when written.** `workflow create` and `workflow narrow`
   resolve the plan before writing and refuse with an error that names the directive and the scope
   or source it widens. The log never holds a directive the plan cannot apply.
6. **EventVersion 3.** The new created fields and the `narrowed` kind exist only in a v3 log; an
   older engine refuses it by its version. Logs v1 and v2 are unchanged and keep the ceiling as
   their plan; `workflow narrow` refuses them and says to create the workflow again.
7. **`memory plan` reads a workflow.** With `--project` and `--workflow`, `labdrian memory plan`
   prints the plan the gate applies, from the log. The directive flags stay, for trying a
   directive before writing it; the two forms do not combine.

## Consequences

- The workflow log format changes one way again (item 1 of the risk list): a v3 log is refused by
  an engine older than batch 54.
- The Goal, the shaper handoff and the role handoff each gain a version; older versions still
  parse.
- The plan the gate enforces can now be narrower than the profile's ceiling. M4 makes the gate
  enforce the plan's sources; M5 uses `Filters.GoalID` for the goal filter.

## Implementation

Batch 54 of phase 9: M3a (Goal v3, shaper handoff v4, `f22e1081`), M3b (role handoff v2,
`42a90199`) and M3c (decisions 3 to 7, below). Paths are under `engine/`.

- **One nested-directive reader.** `memoryscope.ParseDirectiveMember`
  (`memoryscope/memoryscope.go:101`) reads an optional directive member strictly (exact-case keys,
  no unknown or repeated key; absent or null is none). The Goal v3 (`goal/goal.go`), the shaper
  handoff v4 (`shaper/handoff_v4.go`) and the workflow log use it; the role handoff keeps its own
  check, since a null directive is an error there and not "none". `memoryscope.CheckNarrows`
  (`memoryscope/memoryscope.go:232`) is the step `Resolve` applies to each narrower, exported so a
  caller can name the narrower that widens; `Resolve`'s refusals keep their words.
- **EventVersion 3 (decision 6).** `EventVersionNarrowers = 3` (`workflow/workflow.go:45`) is what
  `Create` writes. Version 3 is version 2 plus the created event's optional `goal_directive`,
  `handoff_digest` and `handoff_directive` (together or not at all) and `role_directive` (needs a
  role chain), and the kind `narrowed` with its payload `narrowed: {role_chain_head,
  memory_directive?}` (`workflow/workflow.go:196`). The nested objects are read strictly
  (`checkNestedNarrowerFields`, `workflow/workflow.go:382`); their shapes and the refusal of all of
  them in versions 1 and 2 are `validateCreatedNarrowers` and `validateNarrowedPayload`
  (`workflow/workflow.go:650`, `:683`). Logs of versions 1 and 2 replay, verify and plan as before.
- **The state (decisions 3, 4).** `State` carries the stored directives, the handoff digest and the
  latest narrowed head and directive apart from `RoleChainHead`, which stays the head bound at
  creation, so `Verify` and its messages are unchanged. `narrowed` is legal while created, running
  or paused, needs a role chain and changes no status (`workflow/statemachine.go:195`); a completed
  close still needs a verified event right before it. `LastRoleChainHead` and
  `RoleDirectiveInForce` (`workflow/statemachine.go:104`, `:114`) say which head and which role
  directive are current.
- **One plan (decisions 4, 5).** `workflow.PlanOf` (`workflow/plan.go:38`) is the profile's ceiling
  narrowed by the Goal's directive, the shaper handoff's, then the role directive in force, for the
  recorded project identity (or the slug) and the goal. A widening directive is refused by name:
  "the goal", "the shaper handoff" or "the role handoff memory directive widens the plan: ...".
  The projection's `resolvePlan` (`projection/context.go:113`) calls it, so the context line and
  the memory gate apply the stored narrowers; a plan it cannot compute is still a warning.
- **Create (decisions 3, 5).** `Lifecycle.CreateWithHandoff` (`workflow/lifecycle.go:507`) takes a
  parsed `ShaperHandoff` and the SHA-256 of its bytes; `Create` keeps its signature and records the
  Goal's and the role chain head's directives. `checkShaperHandoff` (`workflow/lifecycle.go:512`)
  refuses (`ErrHandoffRefused`) a handoff that is not a valid version 4, is of another project or
  goal, or does not pair with the Goal's version as `shaper.BindGoal` pairs them
  (`shaper.PairedGoalVersion`, `shaper/handoff_v4.go:88`). The plan of the created payload is
  resolved before anything is probed or written (`workflow/lifecycle.go:598`).
- **Narrow (decision 3).** `Lifecycle.Narrow` (`workflow/lifecycle.go:639`) refuses a log of
  version 1 or 2 (`ErrNarrowersNeedVersion3`: create the workflow again), a closed workflow, one
  with no role chain, a chain that does not verify or no longer holds the last recorded head
  (`ErrRoleChainInvalid`), a chain whose head is that last head (`ErrNothingToNarrow`) and a
  widening directive (`workflow/lifecycle.go:672`); otherwise it appends one `narrowed` event with
  the head and its record's directive.
- **CLI (decisions 3, 7).** `workflow create --handoff <path>` reads the file as `--goal` is read
  (`readShaperHandoff`, `cmd/workflow.go:220`); `workflow narrow` (`cmd/workflow.go:73`) prints the
  state, which shows the narrowers only when there are any. `memory plan --project --workflow`
  (`cmd/memory.go:97`, `workflowMemoryPlan` at `:116`) prints `PlanOf` of the log and refuses to
  combine with the directive flags.
