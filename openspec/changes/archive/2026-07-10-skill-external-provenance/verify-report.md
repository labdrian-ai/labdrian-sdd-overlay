# Verify Report: skill-external-provenance — PR-2

**Verdict: GO — 0 CRITICAL, 1 WARNING, 1 SUGGESTION**
**Branch:** skill-external-provenance/pr-2-wiring
**Date:** 2026-06-26
**Scope:** T-05..T-07 + WARNING-1 fix + SUGGESTION-1 fix

---

## Test run (uncached)

Command: `cd /home/labdrian/labdrian-sdd-overlay/engine && go test ./... -count=1`

| Package | Result |
|---------|--------|
| engine/assets | no test files |
| engine/cmd | ok |
| engine/gate | ok |
| engine/prespec | ok |
| engine/propagator | ok |
| engine/settings | ok |
| engine/skills | ok (383 test entries, 0 FAIL) |

`go vet ./...` — clean.
`bash -n bin/labdrian-overlay` — syntax OK.

---

## Safety invariants

All real production files unchanged vs `main` branch (confirmed via `git diff --stat`):
- `overlay.manifest` — UNCHANGED
- `skills.registry.yaml` — UNCHANGED
- `bin/overlay` — UNCHANGED
- `engine/go.mod` / `engine/go.sum` — UNCHANGED
- `cmd_apply` / `cmd_capture` — UNCHANGED

---

## Zero-fetch guard (SC-69, R-131, R-132)

`TestZeroFetchImportAllowlist` PASS.

Unfoolability re-proof:
1. Added `import _ "net/http"` to a scratch production file.
2. Test turned RED: `forbidden/unexpected import "net/http" in zz_forbidden_test_scratch.go`.
3. Removed scratch file. Test returned GREEN.

No forbidden import (net/http, net, os/exec, git) present in production `engine/skills/*.go`.

---

## Spec requirement sweep — PR-2 scope (R-122..R-134)

| Req | Description | Result |
|-----|-------------|--------|
| R-122 | tagMatchesSourceType "external" matches "custom" | PASS |
| R-123 | registryTag "external" → "custom" | PASS |
| R-124 | Validate: external + manifest "custom" → zero divergences | PASS |
| R-125 | SyncManifest: external emits `<path>/SKILL.md custom` | PASS |
| R-126 | parseFlags: --repo and --ref extracted | PASS |
| R-127 | AddEntry repo!="" → Source.Type="external", Repo set | PASS |
| R-128 | AddEntry repo="" → Source.Type="custom" | PASS |
| R-129 | AddCore --repo: SKILL.md precondition unchanged, no fetch | PASS |
| R-130 | AddCore --repo: manifest emits "custom" tag | PASS |
| R-131 | No net/http, net, os/exec, git import in engine/skills | PASS |
| R-132 | No new non-stdlib import introduced | PASS |
| R-133 | bin/labdrian-overlay passes --repo/--ref to engine | PASS |
| R-134 | cmd_apply and capture NOT modified | PASS |

---

## Scenario sweep (SC-62..SC-69)

| SC | Description | Result |
|----|-------------|--------|
| SC-62 | tagMatchesSourceType("custom","external")==true; ("managed","external")==false | PASS |
| SC-63 | Diff: external + "custom" tag → zero divergences | PASS |
| SC-64 | SyncManifest external → "my-ext-skill/SKILL.md custom" | PASS |
| SC-65 | AddCore --repo → exit 0, external entry, custom tag, Validate nil | PASS |
| SC-66 | AddCore --repo --ref → Source.Ref set, round-trip DeepEqual | PASS |
| SC-67 | AddCore no --repo → custom (non-regression) | PASS |
| SC-68 | --repo + missing SKILL.md → exit!=0, files unchanged | PASS |
| SC-69 | Import scan clean (zero_fetch_test) | PASS |

---

## WARNING-1 fix

`engine/skills/parse.go` — `validateEntry`: external + upstream block → hard error.
Test `TestParseRegistry/WARNING1_external_with_upstream_errors` PASS.

## SUGGESTION-1 fix

Comment added in `parse.go` `validateEntry` explaining the external+upstream guard (ADR-11).

---

## Task completion (PR-2)

| Task | Status | Commit |
|------|--------|--------|
| T-05 manifestTagFor + wire validate/sync + tests | DONE | 2fc3b84 |
| T-06 AddEntry provenance + AddCore flags + tests | DONE | ea8cc8e |
| T-07 Docs note (README + usage block) | DONE | bdf3e63 |
| WARNING-1 fix | DONE | df76516 |
| SUGGESTION-1 fix | DONE | df76516 |

---

## Findings

### WARNING

**W-01 — Stale `unknown_source_type.yaml` fixture (maintenance debt, PR-1 origin)**

File: `engine/skills/testdata/unknown_source_type.yaml`

The fixture contains `source.type: external` without a `repo` field. Before PR-1, `external` was an unknown type. After PR-1, `external` is valid but requires `repo`, so the test still passes — but for the wrong reason (error: "source.repo is required" instead of "unknown source type"). The comment `// SC-03: source.type: external → error (R-003)` is stale.

The SC-57 scenario (external-without-repo) is correctly covered by `TestParseRegistry/SC-57_parse_external_missing_repo_errors`.

Fix: change fixture to `type: bogus` to restore the original "unknown type" intent.

### SUGGESTION

**S-01 — WARNING-1 error message lacks line number**

File: `engine/skills/parse.go` — `validateEntry` ~line 605.

The external+upstream guard error has no line number, consistent with the analogous custom+upstream check but diverging from the parseSource cross-field errors (repo/ref on non-external) which do carry line numbers. The apply phase documented this as a conscious consistency choice.

Fix: add `lineNum` tracking to validateEntry (non-trivial refactor; acceptable as follow-up).

---

## Verdict

**GO for PR-2 and the full skill-external-provenance change.**
0 CRITICAL | 1 WARNING | 1 SUGGESTION

W-01 is a maintenance debt item originating from PR-1; it does not block PR-2 or archive.
S-01 is cosmetic; it does not affect correctness.

next_recommended: sdd-archive
