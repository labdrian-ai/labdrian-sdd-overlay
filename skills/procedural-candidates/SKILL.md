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
2. **T2:** At session close, beside `mem_session_summary`, sweep for qualifying candidates T1 missed.
3. Before creating or updating a candidate, run `skills candidates` to inspect the existing candidates, drafts, aliases, and reconciliation anomalies. Resolve source errors and anomalies; do not treat unavailable data as an empty list.
4. Follow the shared contract's candidate record shape and topic-key rules. Record candidate changes using Engram `mem_save`; do not write a draft file under `skills/` or create, revise, or register a skill as part of detection.
5. Exclude every category in the shared contract's do-not-capture list. Detection and candidate recording do not draft a skill or request approval.
