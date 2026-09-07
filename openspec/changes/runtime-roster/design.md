# Design: One Runtime Roster, And A Test That Fails When It Drifts

Claim tags: **[C]** read from code (path:line), **[M]** measured, **[A]** assumed.
This document does not re-derive the proposal's evidence; it decides the
mechanisms and states, for each, what its test can and cannot prove.

> **Size note.** The `sdd-design` word budget is deliberately exceeded. The
> load-bearing deliverable here is a guard, and a guard whose limits are not
> written down is a guard that will be trusted for something it does not do.

## Technical Approach

One Go declaration names every runtime the overlay knows and marks each
`active` or `dormant`. Two validation surfaces stop restating that list and
start deriving from it. Two data surfaces — the skill registry and the shared
contract — stop being independent assertions and start being *checked against*
it. The correction to the Pi sentence is then a consequence of the guard rather
than a separate act of maintenance.

## Architecture Decisions

### Decision: the roster is a leaf package, not a member of `engine/runtime`

**Choice**: a new package `engine/runtimes` with no non-stdlib imports,
exporting the roster and its accessors.

**Alternatives considered**: (a) put the roster in `engine/runtime` beside the
`Target` constants; (b) put it in `engine/skills`; (c) duplicate a small list
in both and test the two against each other.

**Rationale**: `engine/skills` needs the roster, and `engine/runtime` already
imports `engine/settings`, `engine/assets` and `engine/propagator` **[C]**.
(a) drags all three into `engine/skills`. (b) inverts the dependency for the
runtime package. (c) is the defect this change exists to remove, re-expressed
in Go — two lists and a test is strictly worse than one list and no test,
because the test only proves they agree, not that either is right. A leaf
package has one arrow and one truth, and it is unit-testable with no
filesystem and no HOME.

`engine/runtime` does not import `engine/skills` today **[C]**, so
`engine/skills → engine/runtimes` introduces no cycle.

**Shape**:

```go
package runtimes

type Liveness string

const (
    Active  Liveness = "active"
    Dormant Liveness = "dormant"
)

type Entry struct {
    Name     string   // wire name: --target, install.targets, contract block
    Liveness Liveness
}

// The roster. Adding a name here and nowhere else must fail the suite.
var roster = []Entry{
    {Name: "claude", Liveness: Active},
    {Name: "opencode", Liveness: Active},
    {Name: "codex", Liveness: Active},
    {Name: "pi", Liveness: Dormant},
    {Name: "kilo", Liveness: Dormant},
}

func All() []Entry
func ActiveNames() []string
func DormantNames() []string
func LivenessOf(name string) (Liveness, bool)
```

`All` returns a copy; the roster is package-private so no caller can mutate the
source of truth in a test and leave it mutated for the next one.

**RED test** (`engine/runtimes/roster_test.go`):
`TestRosterLivenessDomainIsClosed` — asserts every entry is `Active` or
`Dormant`. Red because the package does not exist:
`no required module provides package .../engine/runtimes`.
`TestPiAndKiloAreDormant` — the assertion this whole change is about, pinned
where a future reader will look for it.

### Decision: `Target` stays typed in `engine/runtime`; its *domain* is derived

**Choice**: `TargetClaude/TargetOpenCode/TargetCodex/TargetAll` stay exactly as
they are **[C]**. `ParseTarget` and `ExpandTarget` stop switching over literals
and consult `runtimes.LivenessOf`.

**Alternatives considered**: generate the constants from the roster; drop the
typed `Target` and use plain strings.

**Rationale**: the constants are referenced across `engine/cmd`,
`engine/runtime` and their tests **[C]**; generating them buys nothing and
costs a generator. The typed `Target` is what makes `ExpandTarget` and the
adapter registry type-safe. What was actually wrong is the *domain check*
being a literal switch, and that is what moves.

`ParseTarget` gains a third outcome. Today it returns a value or an "unknown"
error **[C]**. It now returns:

| input | outcome |
|---|---|
| an active roster name, or `all` | the `Target` |
| a **dormant** roster name | an error naming the runtime **and its dormancy** |
| anything else | today's unknown error, unchanged |

This is not decoration. A user who reads the contract, sees Pi, and types
`--target pi` deserves to be told Pi is dormant — being told "unknown target"
would send them looking for a typo. R-137 pins that the two errors differ.

**RED tests** (`engine/runtime/runtime_test.go`):
`TestParseTargetRefusesDormantRuntimeByName` — want an error containing both
`pi` and `dormant`. Red today with
`ParseTarget("pi"): error should name dormancy, got "runtime: unknown target \"pi\""`.
`TestParseTargetRejectsUnknownTarget` (existing) must stay green and must
**not** contain `dormant` — an added assertion, so a lazy implementation that
appends "dormant" to every error fails.

### Decision: adapter construction becomes an explicit registry

**Choice**: replace the two parallel if-chains — `NewFoundationAdapter`
(`engine/runtime/runtime.go:204-215`) and `runtimeAdapterForTarget`
(`engine/cmd/main.go:359-370`) **[C]** — with one package-level map in
`engine/runtime`:

```go
var nativeAdapters = map[Target]func(configRoot string) Adapter{
    TargetClaude:   func(root string) Adapter { return NewClaudeAdapter(root) },
    TargetOpenCode: func(root string) Adapter { return NewOpenCodeAdapter(root) },
    TargetCodex:    func(root string) Adapter { return NewCodexAdapter(root) },
}

func AdapterFor(target Target, configRoot string) Adapter // registry, else foundationAdapter
func HasNativeAdapter(target Target) bool                 // registry membership, no side effects
```

`NewFoundationAdapter` keeps its signature and behavior and becomes
`AdapterFor(target, defaultConfigRootFor(target))`. `engine/cmd/main.go` calls
`AdapterFor`. The inert `foundationAdapter` survives untouched for non-roster
input — R-141's third scenario pins that, because deleting a working honest
fallback under cover of a refactor is its own defect.

**Alternatives considered**: (a) leave both if-chains and have the drift test
call `NewFoundationAdapter` per active name and type-assert; (b) a
`func (t Target) Adapter()` method.

**Rationale**: (a) works but the check has side effects — `NewFoundationAdapter`
reads `HOME` **[C]**, so the guard would need a temp HOME to answer a question
about a data structure. `HasNativeAdapter` is a map lookup. (a) also leaves the
duplication that let the two chains disagree in the first place. (b) puts
construction on the enum and makes the registry unenumerable, which is the one
property the guard needs.

Why this matters beyond tidiness: today a roster name with no adapter does not
fail. It falls through to `foundationAdapter`, which answers `unsupported` for
every action **[C]** — honest at run time, invisible at build time. The guard
must fail at test time, so the registry must be enumerable.

**RED test** (`engine/runtime/roster_parity_test.go`):
`TestEveryActiveRuntimeHasATargetAndAnAdapter` — for each `runtimes.ActiveNames()`,
`ParseTarget` succeeds and `HasNativeAdapter` is true. Red because
`HasNativeAdapter` does not exist: `undefined: engineRuntime.HasNativeAdapter`.
`TestNoDormantRuntimeHasATargetOrAnAdapter` — the inverse; this is the test
that would fail the day someone adds `TargetPi` to make the contract true
without writing an adapter.

### Decision: the contract carries a delimited roster block

**Choice**: add to `skills/_shared/review-ledger-contract.md`:

```
<!-- runtime-roster:start -->
| Runtime | Liveness |
| --- | --- |
| claude | active |
| opencode | active |
| codex | active |
| pi | dormant |
| kilo | dormant |
<!-- runtime-roster:end -->
```

**Alternatives considered**: (a) parse the existing enumeration sentence;
(b) generate the whole contract from the roster; (c) keep prose only and rely
on the R-139 sweep.

**Rationale**: (a) is the trap. The sentence is one 90-word clause carrying
four host descriptions and a dormancy note **[C]**; any regex over it is a
second, undocumented specification of the sentence's grammar, and rewording the
prose for clarity would then break the build for no semantic reason. (b) is
disproportionate — one fact in a 69-line document. (c) gives no both-directions
check: a sweep can catch a bad claim, it cannot catch an *omission*, and the
Pi failure was in part an omission (Pi never got the clause Kilo got).

The delimiter pattern is already in this exact file
(`<!-- authority-first-terminal-procedure:start -->`) **[C]** — this is the
house convention, not a new mechanism.

**RED test** (`engine/skills/runtime_roster_contract_test.go`):
`TestContractRosterBlockMatchesRoster` — red with
`review-ledger-contract.md: no <!-- runtime-roster:start --> block found`.
Three further cases mutate the parsed block in memory (add a name, drop a name,
flip a liveness) and assert the comparison reports each, so the comparator's
own both-directions behavior is proven rather than assumed.

### Decision: the prose sweep is a tripwire, and says so in its own source

**Choice**: for every `skills/_shared/*.md`, every line containing a dormant
runtime name as a case-sensitive whole word must also contain `dormant`.

**Alternatives considered**: (a) a support-verb proximity heuristic
(`advertise`, `launches`, `relays`, `forwards` near the name); (b) a sweep over
the whole repository; (c) an exemption allowlist for false positives.

**Rationale**: (a) overfits to the sentence we already found and would miss the
next sentence's verb. The line-level rule is cruder and therefore harder to
evade. (b) would sweep `skills/sdd-init/references/init-details.md`, which
legitimately names `~/.pi/agent/skills/` — see below — so the whole-repo sweep
would have to carve out exactly the file whose carve-out is contested. Scoping
to `_shared/` is defensible on its own terms: those files are loaded by every
phase agent, which is the reason this defect had reach. (c) is refused. An
allowlist turns "this line is a false positive" into "this line is exempt from
the guard", and the two are indistinguishable a year later.

**What it cannot do**, stated in the guard's own doc comment and pinned by
R-140: it cannot catch a support claim about a runtime absent from the roster;
it cannot catch a claim spanning two lines; a legitimate whole-word `Pi` is a
false positive whose fix is rewording, never an exemption. Checks against the
roster block and the code are the guarantees. This one is a tripwire for the
specific shape observed.

**RED test** (`engine/skills/runtime_roster_contract_test.go`):
`TestSharedContractsNeverClaimADormantRuntime` — red on arrival naming two
lines:
`skills/_shared/review-ledger-contract.md:42: line names dormant runtime "Pi" without the word "dormant"`
`skills/_shared/sdd-orchestrator-workflow.md:180: line names dormant runtime "Pi" without the word "dormant"`
A second case feeds a synthetic in-memory line to prove the matcher is
whole-word and case-sensitive: `pipeline`, `capability` and `PI` must not
match; `Pi` must.

### Decision: the registry check reads the shipped file, not a fixture

**Choice**: `TestShippedRegistryTargetsAreActiveRuntimes` parses the real
`skills.registry.yaml` through the production parser and asserts every
`install.targets` value is an active roster name.

**Rationale**: all 36 entries already comply **[M]**, so this test is green the
moment it compiles — which makes its RED phase a fair question. The RED is not
the data; it is `skills.ActiveTargetNames()` not existing, and the test is
written to fail for that reason first. Its load-bearing proof is the mutation
in task 2.5: flip one entry's target to `pi` and watch it fail naming the entry.
A guard whose failure has never been observed is a guard nobody has tested.

## The one file we do not touch

`skills/sdd-init/references/init-details.md:12-13` names `~/.pi/agent/skills/`
inside a scan list of roughly fifteen competitor tool directories — Kimi,
Gemini, Cursor, Copilot, Qwen, Kiro among them **[C]**. None of those is a
supported runtime and nobody would read the list as claiming they are. The
list's purpose is discovery: find skills a user already has, wherever they came
from. Removing Pi from it would *reduce* what the overlay can discover, on the
strength of a word it never used.

This is the distinction the whole change turns on: a scan path is a place we
look, a roster entry is a promise we make. The contract made a promise. That
file does not. Extending the sweep to it would force one of the two to change
meaning, and the meaning worth protecting is the roster's.

## File Changes

| File | Change | Est. lines |
|---|---|---|
| `engine/runtimes/roster.go` | new | ~70 |
| `engine/runtimes/roster_test.go` | new | ~80 |
| `engine/runtime/runtime.go` | `ParseTarget`, `ExpandTarget`, `nativeAdapters`, `AdapterFor`, `HasNativeAdapter` | +55 / −25 |
| `engine/runtime/runtime_test.go` | dormant-vs-unknown cases | ~+40 |
| `engine/runtime/roster_parity_test.go` | new | ~70 |
| `engine/cmd/main.go` | `runtimeAdapterForTarget` → `AdapterFor` | +4 / −12 |
| `engine/skills/parse.go` | `validTargets` derived, error text | +12 / −4 |
| `engine/skills/registry_targets_test.go` | new | ~90 |
| `engine/skills/runtime_roster_contract_test.go` | new | ~150 |
| `skills/_shared/review-ledger-contract.md` | Pi → dormant, roster block | +12 / −2 |
| `skills/_shared/sdd-orchestrator-workflow.md` | drop "or Pi state" | +1 / −1 |

New Go files under `engine/` need no `overlay.manifest` row: nine existing
engine sources, including the whole `engine/runtime/longtermmem*.go` set, are
already unregistered **[M]**, and the on-disk cross-check scans `skills/` only
(`openspec/specs/skills-ondisk-validation/spec.md` R-005) **[C]**.

## Open Question For The Maintainer

`skills/_shared/review-ledger-contract.md` is `managed` in
`overlay.manifest:118` **[C]** — vendored, `vendor-merge`. Editing it makes the
overlay carry a deliberate local divergence from upstream. This change treats
that as acceptable and, in fact, as the reason the guard matters: after this
lands, a vendor-merge that restores the Pi sentence fails
`TestSharedContractsNeverClaimADormantRuntime` instead of landing silently.
The alternative — leave the vendored text alone and record the divergence
somewhere else — would leave every phase agent still reading that Pi works.
Flagged, not decided unilaterally; it is the one place a maintainer may
reasonably rule the other way.
