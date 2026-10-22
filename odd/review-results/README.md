# Review results

The parent of the role chain (`skills/role-chain/SKILL.md`) stores what the `reviewer` agent returns here, one file per launch: `odd/review-results/<feature>/<head12>-<n>.json`.

- `<feature>` is the name of the feature document, `odd/tasks/<feature>.md`.
- `<head12>` is the first 12 hex characters of the reviewed `head`. `<n>` counts the launches stored for that head, from 1: a second blind launch takes the next number.
- The file is the reply as the reviewer gave it: one JSON object with `candidate` (`base`, `head`, `diff_sha256`), `inspection`, `findings` and `evidence`. `agents/reviewer.md` spells the shape out.
- When the verifier ran, its report is stored beside the result as `<head12>-<n>.verifier.md`.

## Binding to the chain

The record that closes the review is a RoleHandoff appended with `labdrian roles append`. Its `payload_kind` is `review-report` and its `payload_sha256` is the sha256 of the stored result. Its evidence binds the bytes under review:

| kind | ref | sha256 |
|---|---|---|
| `review-diff` | `<base>..<head>` | `diff_sha256`, the sha256 of `git diff base head` |
| `review-result` | the stored result | its sha256 |
| `verifier-report` | the stored verifier report, when the verifier ran | its sha256 |

Delivery recomputes the sha256 of `git diff base head` and trusts the record only on equality: other bytes give another digest, and the record does not apply to them.

A review that did not run (a refused launch, an unparseable result, an echo that differs, an incomplete inspection) is recorded `interrupted` with a `resume_reason`, never as approved.

## What a result is not

A result here is check evidence in the role chain, never review authority. It carries no `schema`, lineage, receipt or `GENTLE_AI_*` member. `odd/review-receipts/` and the `gentle-ai review` lifecycle stay as they are: both can run on the same commit, because they share nothing but its bytes.
