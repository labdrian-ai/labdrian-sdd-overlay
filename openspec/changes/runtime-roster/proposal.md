# Proposal: One Runtime Roster, And A Test That Fails When It Drifts

Claim tags: **[C]** read from code (path:line), **[M]** measured in this repo,
**[A]** assumed. Every **[C]** below was verified before this proposal was
written; nothing here needs re-verifying to be acted on.

## Intent

`skills/_shared/review-ledger-contract.md:42` says, in the present indicative,
that Pi is an operational runtime **[C]**:

> "Claude Code, OpenCode, Codex, and **Pi** advertise immutable reviewer
> execution through one shared Go provider contract … and **Pi's**
> gentle-pi-owned host relay forwards the Go-issued opaque prompt to a
> brand-new print-mode `pi` subprocess in an empty scratch directory with
> every discovery surface disabled, returning raw final bytes through the
> exact capture operation."

There is no Pi anywhere in this repository:

| Surface | What it actually contains |
|---|---|
| `engine/runtime/runtime.go:16-19` | `Target` is exactly `claude`, `opencode`, `codex`, `all` **[C]** |
| `engine/skills/parse.go:590` | `validTargets = {claude, opencode, codex}` **[C]** |
| `engine/skills/parse.go:620` | that map **rejects** any other target — a `pi` registry entry fails validation today **[C]** |
| `rg -in 'gentle-pi\|TargetPi' engine/ tools/` | zero hits **[M]** |
| `engine/runtime/runtime_test.go:43` | `TestParseTargetRejectsUnknownTarget` pins the rejection **[C]** |
| this machine | no `pi` on `PATH`, no `~/.pi`, no `~/.config/pi` **[M]** |

**The decisive detail is not the absence — it is the word chosen.** The same
sentence ends "Kilo remains **dormant** because it has no equivalent native
path" **[C]**. The document owns a word for not-yet-supported, uses it for
Kilo in the same breath, and for Pi picks the word for working. That is an
assertion, not an omission.

`skills/_shared/sdd-orchestrator-workflow.md:180` compounds it, naming "Pi
state" as a persistence surface parallel to OpenSpec and Engram **[C]**. Also
nonexistent.

This matters because `_shared/review-ledger-contract.md` is loaded by every
SDD phase agent and every reviewer capture flow. The suite is telling every
agent that Pi works.

### This is the repository's recurring defect class

Every review round in this repo has re-found the same shape somewhere new: **a
record that no longer describes what it records.** The vault indexed upstream
documentation while claiming to index the human's memory. `mergeResults`
implemented an R-006 that no longer described the retrieval it ordered. Here
the contract is the record and the runtime roster is what it records, and they
came apart — silently, because nothing in the build compares them.

Correcting the sentence fixes this instance and guarantees nothing about the
next one. The load-bearing deliverable is therefore not the correction; it is
the test that makes the next divergence fail loud.

## Scope

### In Scope

- **One authoritative roster.** Today "which runtimes are live" is asserted
  independently in four places that can drift apart: `Target` constants
  (`engine/runtime/runtime.go:16-19`), `validTargets`
  (`engine/skills/parse.go:590`), the 36 `targets:` lists in
  `skills.registry.yaml` **[M]**, and the prose at
  `review-ledger-contract.md:42`. A single Go declaration becomes the source;
  the first two derive from it and the last two are checked against it.
- **A drift test.** Deleting a runtime from the roster, or naming a dormant
  runtime as working in a `_shared` contract, turns a test red.
- **The prose corrected.** Pi becomes `dormant`, in the same clause shape Kilo
  already uses; "or Pi state" leaves `sdd-orchestrator-workflow.md:180`.
- **Adapter coverage made total.** `runtimeAdapterForTarget`
  (`engine/cmd/main.go:359-370`) silently falls through to
  `NewFoundationAdapter`, and `NewFoundationAdapter`
  (`engine/runtime/runtime.go:204-215`) falls through to an inert
  `foundationAdapter` **[C]**. A roster name with no adapter therefore
  compiles, runs, and answers — it does not fail. Roster names get an explicit
  registry; the inert fallback survives only for non-roster input.

### Out of Scope

- **A real Pi adapter.** Deliberately, and not as an apology: Pi's frontmatter
  schema, its model-id convention, and its print-mode CLI flags exist nowhere
  in this repository and nowhere on this machine **[M]**. Designing the adapter
  now would mean inventing all three and then writing a contract sentence about
  the invention — which is the exact failure this change exists to close.
  Declaring Pi dormant is the honest state; the adapter is a separate change
  that starts by reading a real `pi` binary.
- **`skills/sdd-init/references/init-details.md:12-13`.** It names
  `~/.pi/agent/skills/` inside a ~15-entry scan list alongside Kimi, Gemini,
  Cursor, Copilot, Qwen and Kiro **[C]**. A discovery scan that looks for a
  competitor's directory claims nothing about supporting it — the surrounding
  list makes that unambiguous. Touching it would make the roster mean two
  different things in two files. Left alone, deliberately; see design §"The
  one file we do not touch".
- Kilo. Already dormant, already correct **[C]**.
- Any change to the 36 registry entries' data. All 36 already name only active
  runtimes **[M]**; this change adds the test that keeps that true, not an edit.

## Capabilities

### New Capabilities

- `runtime-roster`: the single roster declaration, what "active" and "dormant"
  each oblige, the derivation of the two validation surfaces, and the drift
  guard's exact obligations and its stated blind spots.

### Modified Capabilities

- `runtime-lifecycle`: adapter coverage over the active roster becomes total
  and explicit rather than fallback-shaped.

## Approach

| Decision | Rationale |
|---|---|
| Roster lives in a new zero-dependency leaf package `engine/runtimes` | `engine/skills` must read it, and `engine/runtime` already imports `settings`, `assets` and `propagator` **[C]**. A leaf keeps the arrow pointing one way and keeps the roster unit-testable with no filesystem |
| Roster carries `active`/`dormant`, not a bare list of live names | A bare list cannot express "Pi is known and deliberately off". The gap this change closes is precisely a missing word for that state — encoding it as a list would reproduce the defect in Go |
| Contract carries a delimited machine-readable roster block | The file already uses `<!-- authority-first-terminal-procedure:start -->` **[C]**; this is the house pattern, not a new mechanism. A block can be compared exactly; a sentence cannot |
| The prose guard is a per-line heuristic, scoped to `skills/_shared/` | Stated as a heuristic in the spec, with its false-positive and false-negative modes named. See below — this is the claim most at risk of being overstated |

**What the drift test proves, exactly.** Three checks of decreasing strength,
and the change is only worth doing if the third is described honestly:

1. **Exact, both directions.** The contract's roster block and
   `runtimes.All()` are the same set with the same liveness. Adding, removing
   or reclassifying a runtime in either place, and not the other, fails.
2. **Exact, code-internal.** Every `active` name has a `Target` constant that
   `ParseTarget` accepts and an entry in the adapter registry. Every `dormant`
   name has neither, and `ParseTarget` refuses it *naming its dormancy* rather
   than as an unknown string. Every `active` name is in `validTargets`; every
   `targets:` value across `skills.registry.yaml` is an active name.
3. **Heuristic.** In every `skills/_shared/*.md`, a line naming a dormant
   runtime must also contain the word `dormant`.

**What check 3 cannot do, said plainly.** It cannot read English. It cannot
catch a support claim about a runtime the roster has never heard of — a
sentence inventing "Zed" passes. It cannot catch a claim split across two
lines. It matches case-sensitive whole words, so a legitimate `Pi` (the
constant, a filename) would fail it as a false positive, and the fix would be
to reword rather than to open an allowlist — an allowlist would reopen exactly
the hole being closed. Checks 1 and 2 are the guarantees; check 3 is a tripwire
for the specific failure observed, not a proof of prose correctness. The spec
states this limit as a requirement, so a later reader cannot mistake a green
suite for a verified document.

## Affected Areas

| Area | Impact | Description |
|---|---|---|
| `engine/runtimes/` | New | roster declaration + accessors, zero deps |
| `engine/runtime/runtime.go` | Modified | `ParseTarget`, `ExpandTarget`, explicit adapter registry |
| `engine/skills/parse.go` | Modified | `validTargets` derived; error message lists the roster |
| `engine/cmd/main.go` | Modified | `runtimeAdapterForTarget` reads the registry |
| `skills/_shared/review-ledger-contract.md` | Modified | Pi → dormant; roster block added |
| `skills/_shared/sdd-orchestrator-workflow.md` | Modified | "or Pi state" removed |
| `openspec/specs/runtime-lifecycle/spec.md` | Modified (at archive) | adapter coverage requirement |

## Risks

| Risk | Mitigation |
|---|---|
| `_shared/review-ledger-contract.md` is tagged `managed` in `overlay.manifest:118` **[C]** — a vendored file under `vendor-merge`. A later upstream sync can reintroduce the Pi sentence | This is the strongest argument for check 1, not against the edit: after this change a vendor-merge that restores the claim fails the suite instead of landing quietly. Named as an explicit risk so the maintainer knows the edit is a local divergence they now own |
| Check 3 false-positives on a legitimate `Pi` | Reword the line. No allowlist — see above |
| Deriving `validTargets` from `engine/runtimes` couples two packages that were independent | The coupling is the point; they were independent and therefore free to disagree. The leaf package has no imports, so the arrow cannot invert |
| Reviewers read the change as "adding Pi support" | The proposal's Out of Scope is explicit and the roster marks Pi `dormant` in code, where a reader looking for support will actually look |

## Delivery

`ask-on-risk`. Three PRs are **proposed**, not assumed — see `tasks.md` for the
boundaries, the line estimates, and why each boundary falls where it does. The
maintainer chooses stacked-to-main or a feature-branch chain before apply.
