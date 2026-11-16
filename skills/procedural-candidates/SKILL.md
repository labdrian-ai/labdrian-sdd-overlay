---
name: procedural-candidates
description: Detect, reconcile, and record procedural promotion candidates from repeated successes or recurring failure-recovery patterns.
license: MIT
metadata:
  author: labdrian
  version: "1.0"
---

# Procedural candidates

Use this procedure when following the T1/T2 triggers in `skills/_shared/procedural-candidate-detection.md` section 5. That shared document is normative; load and follow its applicable sections, especially its record shape, alias matching, exclusions, and candidate lifecycle.

## Procedure

1. **T1:** Immediately after a proactive `mem_save` of type `bugfix`, `pattern`, `discovery`, or `decision`, determine whether the saved occurrence qualifies under section 5.
2. **T2:** At session close, beside `mem_session`, sweep for qualifying candidates T1 missed.
3. Before creating or updating a candidate, run `skills candidates` to inspect the existing candidates, drafts, aliases, and reconciliation anomalies. Resolve source errors and anomalies; do not treat unavailable data as an empty list.
4. Follow the shared contract's candidate record shape and topic-key rules. Record candidate changes using memorycard `mem_save`, and never write to Engram; do not write a draft file under `skills/` or create, revise, or register a skill as part of detection.
5. New occurrence ids are `memorycard:<id>`. A legacy `engram:<n>` that `memorycard resolve --source engram --key <n> --json` maps to `<m>` is the same occurrence as `memorycard:<m>` (shared contract section 3): appending one while the other is listed is a no-op, and `OccurrenceCount` and `OccurrencesSincePromotion` count distinct translated ids. An id reported unknown stays as written and counts as itself.
6. On every save of a candidate record, rewrite each legacy `engram:<n>` in `Occurrences` that `memorycard resolve` maps to `<m>` as `memorycard:<m>`; an id reported unknown stays as written, and `History` lines are never rewritten. Never migrate records in bulk. Never call `resolve` when the record lists no legacy id. Ask `resolve` in one call, with each distinct legacy number once, sorted as strings and as written. If `resolve` fails or answers a different source, count or order, stop occurrence work and report it, but on a resolve failure a `History` line whose external action already happened is still saved with `Occurrences` exactly as stored; when a rewrite makes two entries one id, keep the earliest-timestamp entry (shared contract section 3).
7. Exclude every category in the shared contract's do-not-capture list. Detection and candidate recording do not draft a skill or request approval.
