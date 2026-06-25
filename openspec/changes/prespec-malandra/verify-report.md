# Verify Report: prespec-malandra — PR-2

**Branch**: `prespec-malandra/pr-2-lint-brief-dispatch`  
**Scope**: T-04 (lint), T-05 (brief+ULID), T-06 (dispatch)  
**Date**: 2026-06-24  
**Verdict**: CONDITIONAL GO — 0 CRITICAL, 3 WARNING, 2 SUGGESTION

---

## Test Output

```
ok  github.com/labdrian-ai/labdrian-sdd-overlay/engine/cmd        0.013s
ok  github.com/labdrian-ai/labdrian-sdd-overlay/engine/gate       0.003s
ok  github.com/labdrian-ai/labdrian-sdd-overlay/engine/prespec    0.003s
ok  github.com/labdrian-ai/labdrian-sdd-overlay/engine/propagator 0.002s
ok  github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings   0.004s
```

`go vet ./...`: clean (no output).

---

## CRITICAL Issues

None.

---

## WARNING Issues

### W-1: `bundles-concerns` regex is over-broad — false-positive boundary untested

**Rule**: `(?i)\band\b` fires on ANY occurrence of the word "and" in a question.

**Evidence**: The following legitimate single-topic Socratic probes are all rejected:
- "What happens when a user logs in and the token is expired?" → rejected (bundles-concerns)
- "What is the relationship between the team and their deadline pressure?" → rejected
- "What are the costs and benefits?" → rejected

These are single-information-seeking questions that naturally use "and" as a connector within a clause, not to join two independent interrogative intents. The SKILL will need to reformulate any such question before asking it.

**Test gap**: The existing test suite has zero cases where a question containing "and" is expected to pass lint. The false-positive behavior is entirely untested and may be mistaken for an oversight rather than a deliberate trade-off.

**Recommendation**: One of:
- (a) Accept the aggressive rule and add explicit test cases that confirm single-topic questions with embedded "and" ARE rejected, with a comment explaining the intentional choice (enforces the skill to always reformulate to pure single-clause probes).
- (b) Narrow the pattern to target multi-clause bundling more precisely (e.g. require "and" to be preceded and followed by full interrogative sub-clauses, or require two "?" indicators).

Option (a) is lower-risk for PR-2 since no production behavior changes. Option (b) requires new test coverage and a regex update.

**Severity rationale**: WARNING not CRITICAL because the rule is mechanically consistent with the spec's literal scenarios, no test currently fails, and the SKILL can compensate. But the untested boundary is a real correctness risk at the skill-authoring layer.

---

### W-2: R-007a / R-011a API contract diverges from spec without annotation

**Spec R-007a says**:
```go
type Violation struct { Rule string; Detail string }
func Check(question string) []Violation
```

**Implementation exports**:
```go
type LintResult struct { Accepted bool; Rule string; Reason string }
func Lint(question string) LintResult
```

The design is authoritative over the spec on this (tasks artifact explicitly states this). The dispatch wires correctly. But a SKILL.md author reading the spec verbatim will write calls to `Check()` and `Violation.Detail` that do not compile.

**Recommendation**: Add a comment in `lint.go` citing R-007a and noting that `Lint`/`LintResult` is the design-authoritative API shape, superseding the spec's `Check`/`Violation` names.

---

### W-3: R-017 / R-018 partial — empty Project not guarded, Assumptions field absent

**R-017**: `TopicKey()` panics on `Project` starting with `"sdd/"` but NOT on empty `Project`. An empty project produces a valid-looking but malformed key: `"project//prespec/<ULID>"`. There is no test for this case. `Validate()` does not check `Project` emptiness.

**R-018**: Specifies `Validate` must reject: empty/invalid DiscoveryID, empty Project, `len(Assumptions) > 3`, and `ReadinessScore < 0.6`. Current `Validate()` only checks: readiness (via `cells`), empty `Job`, empty `Transcript`. The `Brief` struct has no `Assumptions` field and no `ReadinessScore` field.

**Assessment**: The `Assumptions` and `ReadinessScore` fields may be intentionally omitted (they are skill-layer concerns; the SKILL enforces the 3-assumption cap). The empty-Project gap is the concrete risk: a skill bug sending `""` as project produces a structurally broken TopicKey that would corrupt engram namespace.

**Recommendation**: Add an empty-Project guard to `TopicKey()` (or `Validate()`) with a test.

---

## SUGGESTION Issues

### S-1: Brief struct field names diverge from spec R-011a without cross-reference

Spec names: `JobStatement`, `Section1`..`Section6`, `Assumptions`, `ReadinessScore`.  
Implementation: `Job`, `Sections [6]string` (array), no `Assumptions`, no `ReadinessScore`.  
The array form is superior Go ergonomics and the design is authoritative. A comment mapping spec field names to implementation names would help future readers.

### S-2: Local `min()` helper in prespec_test.go is redundant under Go 1.21

`go.mod` specifies `go 1.21`, where `min()` is a built-in. The local definition at line 253 of `prespec_test.go` is harmless but can be removed. Minor cleanup only.

---

## Per-Requirement Coverage Map (PR-2 scope)

| Req | Description | Covering Test(s) | Status |
|-----|-------------|------------------|--------|
| R-007 / T-04 | Lint: 3 rules, first-match wins | TestLintAccepted, TestLintSmuggleAnswer, TestLintPresupposesSolution, TestLintBundlesConcerns, TestLintFirstFailingRuleWins, TestLintReasonNonEmpty | PASS |
| R-014 / T-05 | ULID format, uniqueness, deterministic seam | TestNewIDFormat, TestNewIDFromDeterministic, TestNewIDUniqueness | PASS |
| R-011 / T-05 | Brief schema, render, golden file | TestRenderBriefGolden | PASS |
| R-017 / T-05 | TopicKey sdd/ guard | TestTopicKeySDDPanic | PASS (partial — empty project untested) |
| R-017 / T-05 | TopicKey correct format | TestTopicKeyFormat | PASS |
| R-018 / T-05 | Validate: gate, empty Job, empty Transcript | TestValidatePassesGate, TestValidateFailsGate, TestValidateRequiresJob, TestValidateRequiresTranscript | PASS (partial — empty Project, DiscoveryID format not validated) |
| R-012 / T-05 | No ChangeName field | Struct definition (compiler-checked) | PASS |
| T-06 dispatch | 4 verbs, JSON-over-stdin, fail-loud | TestPrespecRankVerb, TestPrespecLintVerb, TestPrespecLintVerbRejects, TestPrespecReadinessVerb, TestPrespecBriefVerb, TestPrespecBriefVerbFailsGate, TestPrespecUnknownVerb, TestPrespecRankVerbMalformedJSON | PASS |
| T-06 cmd layer | prespec subcommand in main | TestRunPrespec_LintHappyPath, TestRunPrespec_UnknownVerbExitsOne, TestRunPrespec_MalformedJSONExitsOne | PASS |
| ADR-2 / T-06 | Engine stateless, no persistence imports | Import list inspection | PASS |

---

## Confirmed Passing Properties

- **ULID**: Independent encoding verification confirms `NewIDFrom(fixedTime, zeroBits{})` produces `066GSAY4800000000000000000`, matching the golden file. Matches `^[0-9A-HJKMNP-TV-Z]{26}$`. Not kebab-compatible (uppercase letters, no hyphens). Zero new deps: `go.mod` is unchanged (stdlib only, `go 1.21`).
- **Dispatch statelessness**: `engine/prespec/prespec.go` imports only `encoding/json`, `fmt`, `io`, `strings`, `time`. No engram, no file I/O.
- **Golden file determinism**: Render output is byte-stable; time injected via `Brief.CreatedAt`; random component injected via `NewIDFrom` seam. `UPDATE_GOLDEN=1` path documented and tested.
- **T-04/T-05/T-06 tasks**: All three marked `[x]` complete in apply-progress with matching commit messages.
- **TopicKey namespace guard**: `sdd/` prefix in Project panics — R-017 positive path covered with test.

---

## GO / NO-GO

**CONDITIONAL GO for PR-2.**

No blockers that prevent opening the PR. The 3 warnings should be tracked as follow-up items (ideally addressed in PR-2 via a review comment or small fix before merge). W-3 (empty Project guard) is the highest-priority fix since it is a concrete namespace corruption risk, not just a documentation gap.

PR-3 (SKILL.md) is out of scope for this verification as specified.
