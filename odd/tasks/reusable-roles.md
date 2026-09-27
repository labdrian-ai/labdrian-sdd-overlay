# ODD Task — Phase 4: Reusable roles

> **Roadmap authority:** [Standalone Platform Master Roadmap](../../openspec/decisions/standalone-platform-roadmap.md), canonical **Phase 4 — Reusable roles**: typed reusable role handoffs (`prototyper → shaper → estimator → builder → sweeper → polisher → reviewers → delivery`), preserving context/evidence and interruption recovery without duplicating runtime-specific phase agents. Phases 2 and 3 are verified on `main` (`5d9cb1d`).

## Approved design (user, 2026-09-26)

1. **Closed role vocabulary**, canonical order: `prototyper`, `shaper`, `estimator`, `builder`, `sweeper`, `polisher`, `reviewer`, `delivery`. `sweeper` and `polisher` are optional (skippable). The only backward transition is `reviewer → builder` (rework). Each role is data: purpose, what it consumes, what it produces. Roles grant no authority.
2. **`RoleHandoff` v1 record**, parsed strictly through `jsonstrict`: `{version, project_id, goal_id, chain_id, seq, from_role, to_role, prev_sha256, payload_kind, payload_sha256, evidence[{kind, ref, sha256}], context{summary, decisions[], open_questions[]}, status: completed|interrupted, resume_reason?}`. The transition must be valid for the vocabulary; `prev_sha256` chains each record to the previous record's bytes so any history edit is detected.
3. **Chain and recovery:** an append-only ledger per `chain_id` in the XDG state store. `resume` derives the current role from the last record; an `interrupted` record resumes in the same role.
4. **CLI** `labdrian roles validate | next | resume`, with **no dispatcher**: launching agents belongs to Phase 7. Roles stay data, so runtime-specific agents are not duplicated.
5. **Relationships:** the free-form `roles` in Shaper handoff v3 are not broken; a check reports which of them match the vocabulary. WorkflowProfile roles stay separate: they are participants within a stage, not lifecycle roles.

## Engineering defaults (parent; user may overturn)

- Store: `$XDG_STATE_HOME/labdrian/role-chains/<project_id>/<goal_id>/<chain_id>/<seq>.json` (with the `$HOME/.local/state` fallback), one immutable file per record created exclusively (no overwrite), 0700 directories and 0600 files, refusing symlinked or non-regular components, mirroring the Shaper clearance store.
- The first record of a chain has `seq` 1, `prev_sha256` set to the empty-chain sentinel (64 zeros), and `from_role` `prototyper` or `shaper` (a chain may start without prototyping). Each later record's `from_role` equals the previous record's `to_role`, except that an `interrupted` record is followed by a record with the same `from_role` and `to_role`. `seq` increases by exactly 1.
- Digests are lowercase hex SHA-256 over raw record bytes.

## Scope and constraints

- Branch `feat/reusable-roles` from `main` (`5d9cb1d`), worktree `~/labdrian-sdd-overlay-shaper`.
- In scope: new `engine/roles` package, `engine/cmd` `roles` verbs, `bin/labdrian-overlay` dispatch, README, and the v3 role-match report.
- Out of scope: dispatching or launching agents, runtime adapters (Phase 7), memory integration (Phase 5), changing Shaper handoff v3 semantics.
- TDD: strict. Runner: `go test` from `engine/`. Checks: `gofmt -l`, `go build ./...`, `go vet ./...`, `go test -count=1 ./...`, `staticcheck ./...`. Tests isolate HOME and XDG state.
- RDD on; delivery `ask-on-risk`, `stacked-to-main`, one issue per slice.

## Tasks

- [x] R1 — `engine/roles`: vocabulary, allowed transitions, `RoleHandoff` v1 strict parse and validation. *Route: delegated writer.* **Evidence:** commit `4758a1d` (822 lines). RED was a compile failure. Native review `review-507a539c5d6aac40` approved with no correction and was acknowledged; three informational test-gap findings.
- [x] R2 — Chain store and recovery. *Route: delegated writer.* **Evidence:** commit `20587e1` (930 lines). RED was a compile failure. Re-appending identical bytes for an existing `seq` is idempotent. Native review `review-de36751bceda7107` approved with no correction and was acknowledged; eight informational test-gap findings (symlinked record file, link EEXIST race, parse-failure path, 6-digit `seq` boundary).
- [x] R3 — CLI `roles validate | next | resume | append --stdin | match-shaper`, `labdrian roles` dispatch, README, and the Shaper v3 role-match report. *Route: delegated writer.* **Evidence:** commit `5c7101a` (756 lines). **Writer decision beyond the spec, accepted by the parent:** `append` refuses when the `--project`, `--goal`, or `--chain` flags do not match the record's own identifiers (defense in depth). Native review `review-77ba7840d17f0ed7` (four lenses) approved with no correction and was acknowledged; fifteen informational findings, mostly unproven CLI refusal paths.
- [x] R4 — Native reviews done. With user authorization, `status:approved` was applied to #425–#427 and `size:exception` to #428–#430, with read-back. PRs #428, #429, and #430 merged in order with green CI (`8887056`, `aedc0c0`, `7b26798`). The primary checkout was fast-forwarded and the engine rebuilt from `7b26798`. **Live smoke** with the deployed `labdrian roles`, in an isolated XDG state: an 8-record chain (prototyper → shaper → estimator → builder, interrupted, → reviewer, rework to builder, → reviewer → delivery) was accepted and resolved to terminal `delivery` with the no-authority statement. Editing record 3 on disk made `resume` refuse on the prev_sha256 mismatch, and `builder → delivery` was refused. `match-shaper` on the real v3 fixture reported `shaper` as a vocabulary role and `human` as free-form. The roadmap marks Phase 4 **verified**.

## Acceptance criteria

- Invalid transitions, broken hash chains, `seq` gaps, duplicate or edited records, and unknown fields are refused.
- A chain interrupted mid-role resumes in that role; a completed chain reports `delivery` as terminal.
- No command launches an agent or grants authority; runtime-specific agent files are not duplicated.

## Progress and evidence

(none yet)

## Next step

Phase 4 is closed. Next roadmap outcome: Phase 5, procedural memory integration.
