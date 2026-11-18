---
name: issue-creation
description: "Create or comment on a GitHub issue for the labdrian flow. Trigger: the owner asks for an issue, a bug report or a feature request."
license: Apache-2.0
metadata:
  author: labdrian
  version: "2.0"
---

## Activation Contract

Load this skill when the owner asks for a GitHub issue or comment.

## Hard Rules

- A write runs only on the owner's current direct instruction that names the target and the action.
- Resolve the exact target as `[HOST/]OWNER/REPO`; never assume the current repository. A stale or ambiguous target or form is a stop at the gate.
- The body follows a YAML Issue Form: the target repository's own `.github/ISSUE_TEMPLATE/*.yml` that fits the request. If none fits, stop and ask. The YAML form is the only body format: no Markdown fallback, no browser route.
- One duplicate search over open and closed issues runs before a write. Read only the candidates whose title matches the request, at most 10. If more match, stop: report the candidates and ask the owner. A candidate that cannot be read is `unknown`: stop.
- Never invent a fact, label or policy; ask for the smallest missing fact.
- Body files live in a private temporary directory, removed on every exit; never print them.
- Run the privacy scan on title and body immediately before the write: paths, usernames, hostnames, secrets, private names. Replace each with `<project-name>`, `<user>`, `<hostname>` or `<token>`.
- Make one attempt, no retry, then read it back from the target host.
- `unknown` stops every later write and retry.
- An issue that turns into work reaches a role only through the foreman and `inception-pipeline` (feature document), never straight to a builder.

## Decision Gates

| Condition | Action |
|---|---|
| Target or form missing, stale or ambiguous | Stop at the gate; resolve it first. |
| A conforming duplicate exists | Comment there; create nothing. |
| The search is saturated or incomplete | Narrow it or stop. |
| A duplicate does not conform to the form | Report it and ask the owner; never edit it unasked. |
| The owner gave no direct instruction | Draft only; no write. |

## Procedure

Commands, variables and the read-back comparison: `references/procedure.md`.

## Outcomes

- `confirmed`: the write returned a stable identity and the read-back matches.
- `no_write`: an authoritative rejection, or a refusal before the write.
- `unknown`: a timeout, a lost response, a missing identity, a read-back that is unavailable, or a mismatch.

## Output Contract

Return the target, form, duplicate decision, identity read back, and exactly one of `confirmed | no_write | unknown`. A stopped run names the missing fact and says no write occurred.
