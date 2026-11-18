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
- Resolve the exact target as `[HOST/]OWNER/REPO`; never assume the current repository.
- The body follows a YAML Issue Form: the target repository's own `.github/ISSUE_TEMPLATE/*.yml` that fits the request. If none fits, stop and ask. No Markdown or blank body.
- One duplicate search over open and closed issues runs before a write.
- Never invent a fact, label or policy; ask for the smallest missing fact.
- Body files live in a private temporary directory, removed on every exit.
- Run the privacy scan on title and body immediately before the write: paths, usernames, hostnames, secrets, private names. Replace each with `<project-name>`, `<user>`, `<hostname>` or `<token>`.
- Make one attempt, no retry, then read it back from the target host.
- `unknown` stops every later write and retry.
- An issue that turns into work reaches a role only through the foreman and `inception-pipeline` (feature document), never straight to a builder.

## Decision Gates

| Condition | Action |
|---|---|
| Target or form missing or ambiguous | Resolve it or stop. |
| A conforming duplicate exists | Comment there; create nothing. |
| The search is saturated or incomplete | Narrow it or stop. |
| A duplicate does not conform to the form | Report it and ask the owner; never edit it unasked. |
| The owner gave no direct instruction | Draft only; no write. |

## Execution Steps

1. Search: `gh issue list --repo "$TARGET" --state all --search "$QUERY" --limit 1000`. Read each candidate from the target host with `gh issue view "$N" --repo "$TARGET" --json number,url,title,body` and compare it with the selected form. A candidate that cannot be read is `unknown`: stop.
2. Fill the body from the form and reviewed answers.
3. Privacy scan, then one attempt: `gh issue create --repo "$TARGET" --title "$TITLE" --body-file "$BODY_FILE"`, or `gh issue comment "$NUMBER" --repo "$TARGET" --body-file "$BODY_FILE"`.
4. Read an issue back with `gh issue view "$NUMBER" --repo "$TARGET" --json number,url,title,body`. Read a comment back with `gh api --hostname "$HOST" "repos/$REPO/issues/comments/$COMMENT_ID"` and compare its body.

## Outcomes

- `confirmed`: the write returned a stable identity and the read-back matches.
- `no_write`: an authoritative rejection, or a refusal before the write.
- `unknown`: a timeout, a lost response, a missing identity, a read-back that is unavailable, or a mismatch.

## Output Contract

Return the target, form, duplicate decision, identity read back, and exactly one of `confirmed | no_write | unknown`. Before a write, a stop names the missing fact.
