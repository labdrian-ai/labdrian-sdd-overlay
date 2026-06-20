# Baseline — minimalism-contract-lite

Captured: 2026-06-19
Purpose: directional behavioral evidence for AC-4 (proposal). Records pre-change LOC and
abstraction counts from archived changes in this overlay, to compare against post-change
apply outputs and assess whether the minimalism contract produces measurable behavior change.

---

## Status: pending-first-archives

No changes have been completed and archived in this overlay yet. The overlay is nascent
(`minimalism-contract-lite` is the first change going through the full SDD pipeline here).

Once 3-5 changes have been archived, capture the following per change:

| Change | LOC added (approx) | Net new deps | Single-caller abstractions | Notes |
|--------|-------------------|--------------|---------------------------|-------|
| (first archived change) | — | — | — | pending |
| (second archived change) | — | — | — | pending |
| (third archived change) | — | — | — | pending |

### Instructions for capturing entries

1. `git diff --stat <merge-commit>` for the change's merge commit to approximate LOC.
2. Count new entries in dependency manifests (`package.json`, `go.mod`, `requirements.txt`, etc.).
3. Count functions/classes that appear in exactly one call site at time of merge.
4. Record the change name (kebab-case slug, from `openspec/changes/*/`).

---

## Notes

- Baseline is file-based (not Engram) to survive session restarts without ambiguity.
- Engram persistence is blocked in this overlay by project-ambiguity (multiple git repos in
  cwd); file is the recovery-safe store.
- This file is NOT a blocking gate for the current slice — T-07 (`overlay apply`) may
  proceed once authoring tasks T-01..T-05 pass sdd-verify.
