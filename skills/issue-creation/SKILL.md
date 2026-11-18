---
name: issue-creation
description: "Create or comment on a GitHub issue for the labdrian flow. Trigger: the owner asks for an issue, a bug report or a feature request."
license: Apache-2.0
metadata:
  author: labdrian
  version: "2.0"
---

## Activation Contract

Load this skill when the owner asks you to create or comment on a GitHub issue. The selected YAML Issue Form is the authority on the body.

## Hard Rules

- A write runs only on the owner's current direct instruction that names the target and the action.
- Resolve the exact target as `[HOST/]OWNER/REPO`; never assume the current repository.
- The body follows the YAML Issue Form (this repository: `.github/ISSUE_TEMPLATE/feature.yml`). No Markdown fallback, blank body or browser route.
- One duplicate search over open and closed issues runs before a write.
- Never invent a fact, answer, label or policy; ask for the smallest missing fact.
- Body files live in a private temporary directory, removed on every exit; never print their contents.
- Run the privacy scan on title and body immediately before the write: local paths, usernames, hostnames, secrets, private project names. Replace each finding with `<project-name>`, `<user>`, `<hostname>` or `<token>`.
- Make one attempt, no blind retry, then read it back from the target host.
- `unknown` stops every later write and retry.
- An issue that turns into work reaches a role only through the foreman and `inception-pipeline` (a feature document), never straight to a builder.

## Decision Gates

| Condition | Action |
|---|---|
| Target or form missing, stale or ambiguous | Resolve it; stop if it stays unknown. |
| A conforming duplicate exists | Comment there; create nothing. |
| The search is saturated or incomplete | Narrow it or stop without writing. |
| The owner gave no direct instruction | Draft only; no write. |

## Execution Steps

1. Search: `gh issue list --repo "$TARGET" --state all --search "$QUERY" --limit 1000`.
2. Fill the body from the form's controls and the reviewed answers.
3. Privacy scan, then one attempt: `gh issue create --repo "$TARGET" --title "$TITLE" --body-file "$BODY_FILE"`, or `gh issue comment "$NUMBER" --repo "$TARGET" --body-file "$BODY_FILE"`.
4. Read back with `gh issue view "$NUMBER" --repo "$TARGET" --json number,url,title,body`; title and body must match.

## Output Contract

Return the target, the form, the duplicate decision, the identity read back, and exactly one of `confirmed | no_write | unknown`. When stopping before a write, name the missing fact and say no write occurred.
