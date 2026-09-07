# Tasks: One Runtime Roster, And A Test That Fails When It Drifts

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | PR-1 ~330 · PR-2 ~190 · PR-3 ~260 |
| 400-line budget risk | Low — every slice fits with headroom |
| Chained PRs recommended | Yes (3) |
| Suggested split | PR-1 roster + runtime derivation → PR-2 skills derivation + registry guard → PR-3 contract block + prose correction + sweep |
| Delivery strategy | ask-on-risk |
| Chain strategy | pending — ask the maintainer: stacked-to-main or feature-branch chain |

Decision needed before apply: **Yes** (chain strategy, and the vendored-file
question in `design.md`'s Open Question).

### Where the boundaries fall, and why

- **PR-1 / PR-2** splits at the package boundary. PR-1 is `engine/runtimes` +
  `engine/runtime` + the one `engine/cmd` call site; PR-2 is `engine/skills`.
  They are separable because `engine/skills` keeps its literal `validTargets`
  through PR-1 — behavior identical, just not yet derived. Merging them gives
  ~520 lines, over budget, and mixes two review questions ("is the roster the
  right shape?" with "is the registry correctly constrained?").
- **PR-2 / PR-3** splits at code-versus-document. PR-3 is the only slice that
  edits a `managed` vendored file and the only one whose tests read markdown.
  A reviewer of PR-3 is checking prose claims; a reviewer of PR-2 is checking a
  parser. Those are different reviews.
- **No slice needs to exceed 400.** PR-3 is the largest test file (~150 lines
  for two guards plus their mutation cases). If review finds it over budget
  once written, it splits cleanly at `TestContractRosterBlockMatchesRoster`
  (exact check) versus `TestSharedContractsNeverClaimADormantRuntime`
  (heuristic sweep) — but the prose correction must ship with the sweep, since
  the sweep is red until the correction lands. Splitting them would mean
  merging a knowingly-red test.

### Suggested Work Units

| Unit | Goal | PR | Focused test command | Rollback boundary |
|---|---|---|---|---|
| 1 | Roster package, derived target domain, adapter registry | PR-1 | `go test ./runtimes/... ./runtime/... ./cmd/...` | revert PR-1; `engine/runtimes` has no other importer yet |
| 2 | `validTargets` derived + shipped-registry guard | PR-2 | `go test ./skills/...` | revert PR-2; PR-1's literal-free runtime domain is unaffected |
| 3 | Contract roster block, prose correction, `_shared` sweep | PR-3 | `go test ./skills/... -run RuntimeRoster` | revert PR-3; restores the vendored contract text verbatim |

## Phase 1: Roster, Target Domain, Adapter Registry (PR-1) — R-135, R-137, R-141

- [ ] 1.1 RED `TestRosterLivenessDomainIsClosed`, `TestActiveAndDormantAreSeparatelyEnumerable`, `TestPiAndKiloAreDormant` (`engine/runtimes/roster_test.go`). Failure before the fix: `no required module provides package github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtimes` — the package does not exist.
- [ ] 1.2 GREEN: create `engine/runtimes/roster.go` — `Liveness`, `Entry`, the private `roster` slice with claude/opencode/codex active and pi/kilo dormant, and `All`/`ActiveNames`/`DormantNames`/`LivenessOf`. `All` returns a copy.
- [ ] 1.3 RED `TestRosterIsNotMutableByCallers` (`engine/runtimes/roster_test.go`) — take `All()`, overwrite element 0's `Liveness`, call `All()` again and want the original. Failure before the fix: `roster mutated through All(): claude liveness = "dormant", want "active"`. Red if 1.2 returns the backing slice.
- [ ] 1.4 GREEN: return a defensive copy from `All`.
- [ ] 1.5 RED `TestParseTargetRefusesDormantRuntimeByName` (`engine/runtime/runtime_test.go`) — `ParseTarget("pi")` must error with text containing both `pi` and `dormant`. Failure before the fix: `ParseTarget("pi"): want error naming dormancy, got "runtime: unknown target \"pi\""`.
- [ ] 1.6 PIN (not a RED — it starts green and must stay green) extend `TestParseTargetRejectsUnknownTarget` with an assertion that the unknown-target error does **not** contain `dormant`. This is the anti-cheat pin: an implementation that appends "dormant" to every rejection passes 1.5 and fails here. It cannot fail before 1.7, by construction; it exists to fail *after* a careless 1.7. A task whose test cannot fail is not a TDD task, so this is booked as a pin on 1.7 rather than counted as its own RED.
- [ ] 1.7 GREEN: rewrite `ParseTarget` in `engine/runtime/runtime.go` to consult `runtimes.LivenessOf` — active name or `all` returns a `Target`, dormant name returns the dormancy error, anything else keeps today's unknown error verbatim.
- [ ] 1.8 GREEN: rewrite `ExpandTarget(TargetAll)` to build from `runtimes.ActiveNames()` rather than the literal triple at `runtime.go:201`. Existing `TestExpandTargetAndFoundationAdapters` must stay green unmodified — it asserts order claude/opencode/codex, which pins that the roster's declaration order is load-bearing.
- [ ] 1.9 RED `TestEveryActiveRuntimeHasATargetAndAnAdapter`, `TestNoDormantRuntimeHasATargetOrAnAdapter` (`engine/runtime/roster_parity_test.go`). Failure before the fix: `undefined: engineRuntime.HasNativeAdapter`.
- [ ] 1.10 GREEN: add `nativeAdapters map[Target]func(string) Adapter`, `AdapterFor(target, configRoot) Adapter`, `HasNativeAdapter(target) bool` in `engine/runtime/runtime.go`; reimplement `NewFoundationAdapter` over `AdapterFor` with the per-target default config root. `foundationAdapter` and its `unsupported` answers are unchanged.
- [ ] 1.11 GREEN: replace `runtimeAdapterForTarget` (`engine/cmd/main.go:359-370`) with a call to `runtimepkg.AdapterFor`. `engine/cmd/runtime_test.go` and `main_test.go` must pass unmodified — if either needs editing, the refactor changed behavior and that is a finding, not a fixup.
- [ ] 1.12 Load-bearing proof for 1.9: temporarily delete `TargetCodex` from `nativeAdapters`, run `go test ./runtime/...`, record that `TestEveryActiveRuntimeHasATargetAndAnAdapter` fails naming `codex`, restore. Record the observed message in `apply-progress.md`. Without this, 1.9 is a test nobody has seen fail.

## Phase 2: Skills-Side Derivation And The Registry Guard (PR-2) — R-136

- [ ] 2.1 RED `TestValidTargetsAreExactlyTheActiveRoster` (`engine/skills/parse_test.go` or a new `registry_targets_test.go`) — asserts the parser's accepted target set equals `runtimes.ActiveNames()`. Failure before the fix: `undefined: skills.ActiveTargetNames` (the exported accessor the test needs does not exist).
- [ ] 2.2 RED `TestEntryNamingADormantRuntimeIsRefusedWithTheRosterListed` — an in-memory entry with `install.targets: [pi]`; want a validation error containing the entry id, the value `pi`, and the active roster names. Failure before the fix: the error is today's fixed literal `must be one of: claude, opencode, codex`, which happens to list the right names but is not derived — assert on derivation by also feeding a stub roster in the same test table where possible, or assert the message is built from `ActiveTargetNames()` output.
- [ ] 2.3 GREEN: in `engine/skills/parse.go`, build `validTargets` from `runtimes.ActiveNames()` at package init and derive the error text at `parse.go:621` from the same source. Delete the literal map at `parse.go:590`.
- [ ] 2.4 RED/GREEN `TestShippedRegistryTargetsAreActiveRuntimes` — parse the real `skills.registry.yaml` through the production parser and assert every entry's `install.targets` values are active roster names. Green on arrival for the shipped data (all 36 entries already comply); its RED is 2.1's missing accessor, and its real proof is 2.5.
- [ ] 2.5 Load-bearing proof for 2.4: temporarily rewrite one entry's `targets:` to `pi` in `skills.registry.yaml`, run `go test ./skills/... -run ShippedRegistry`, record that it fails naming that entry id and `pi`, restore the file, confirm `git status` clean. Record the observed message in `apply-progress.md`.
- [ ] 2.6 Confirm `engine/skills/lifecycle.go:48`'s `Targets: []string{"claude","opencode","codex"}` literal — decide in-PR whether it is a default that should derive from the roster or a fixture that should not. If it derives, it is covered by 2.1; if it stays literal, add a one-line comment saying why and note the decision in `apply-progress.md`. Do not leave it undecided.

## Phase 3: Contract Block, Prose Correction, Shared Sweep (PR-3) — R-138, R-139, R-140

- [ ] 3.1 RED `TestContractRosterBlockMatchesRoster` (`engine/skills/runtime_roster_contract_test.go`) — reads `skills/_shared/review-ledger-contract.md` from the repo root (helper pattern: `engine/assets/assets_test.go:14`), parses the roster block delimited by the `runtime-roster:start` and `runtime-roster:end` HTML comment markers, compares to `runtimes.All()` both directions. Failure before the fix: `skills/_shared/review-ledger-contract.md: no runtime-roster:start block found`.

  Note: the markers are named here without their HTML comment delimiters on purpose. archive-reconcile refuses to classify a line carrying both a task checkbox and a comment delimiter rather than guessing which it is, and it is right to: the marker's literal form belongs in the design and the test, not on a checkbox line.
- [ ] 3.2 RED (same file) three comparator cases over an in-memory parsed block — one extra name, one missing name, one flipped liveness — each asserting the comparison names the runtime and the side that disagrees. These prove the comparator, not the file; without them a comparator that always returns "equal" passes 3.1.
- [ ] 3.3 GREEN: add the `runtime-roster` block to `skills/_shared/review-ledger-contract.md`, immediately after the paragraph at line 42, listing claude/opencode/codex active and pi/kilo dormant.
- [ ] 3.4 RED `TestSharedContractsNeverClaimADormantRuntime` — sweeps every `skills/_shared/*.md`; a line naming a dormant runtime as a case-sensitive whole word must also contain `dormant`. Failure before the fix, two lines named: `skills/_shared/review-ledger-contract.md:42: line names dormant runtime "Pi" without the word "dormant"` and `skills/_shared/sdd-orchestrator-workflow.md:180: line names dormant runtime "Pi" without the word "dormant"`.
- [ ] 3.5 RED (same file) `TestDormantNameMatchIsWholeWordAndCaseSensitive` — synthetic lines: `pipeline`, `capability`, `PI`, `π` must not match; `Pi` and `Kilo` must. Failure before the fix: the matcher does not exist.
- [ ] 3.6 GREEN: implement the sweep with a doc comment stating R-140's three limits verbatim — it cannot detect a claim about a runtime absent from the roster; it cannot detect a claim spanning two lines; a legitimate whole-word use is a false positive whose fix is rewording, never an exemption list.
- [ ] 3.7 GREEN: correct `skills/_shared/review-ledger-contract.md:42` — remove Pi from the four-host enumeration and from the per-host description clause; extend the existing dormancy sentence to read that Pi and Kilo remain dormant because neither has an equivalent native path. Do not add any new claim about Pi.
- [ ] 3.8 GREEN: correct `skills/_shared/sdd-orchestrator-workflow.md:180` — drop `, prompts, or Pi state` down to `, or prompts`. Pi is not a persistence surface; nothing replaces it.
- [ ] 3.9 Load-bearing proof for 3.4: after 3.7/3.8 are green, temporarily reinstate the original line 42 sentence, run `go test ./skills/... -run RuntimeRoster`, record that the sweep fails naming that line, restore. Record the observed message in `apply-progress.md`. This is the single most important evidence in the change: it is the proof that the recurrence guard actually fires on the exact defect it was written for.
- [ ] 3.10 Load-bearing proof for 3.1: temporarily flip `pi` to `active` in the contract block only, run the same command, record that `TestContractRosterBlockMatchesRoster` fails naming `pi` and both liveness values, restore.

## Phase 4: Cross-Cutting

- [ ] 4.1 Confirm `skills/sdd-init/references/init-details.md` is untouched across all three PRs (`git diff --stat` names it nowhere). Its `~/.pi/agent/skills/` scan-list entry is deliberately preserved — see `design.md` §"The one file we do not touch".
- [ ] 4.2 Confirm no new `overlay.manifest` row was added or needed for the new `engine/` Go files, consistent with the nine engine sources already unregistered.
- [ ] 4.3 Run the full suite (`go test ./...`) in `engine/`, `tui/` and `longterm-mem/` after PR-3 lands.
- [ ] 4.4 Record in `apply-progress.md` whether the maintainer accepted the vendored-file divergence from `design.md`'s Open Question, and under which PR it was answered.
