# Review results

The parent of the role chain (`skills/role-chain/SKILL.md`) stores what the `reviewer` agent returns here, one file per launch: `odd/review-results/<feature>/<head12>-<n>.json`.

- `<feature>` is the name of the feature document, `odd/tasks/<feature>.md`.
- `<head12>` is the first 12 hex characters of the reviewed `head`. `<n>` counts the launches stored for that head, from 1: a second blind launch takes the next number.
- The file is the reply as the reviewer gave it: one JSON object with `candidate` (`base`, `head`, `diff_sha256`), `inspection`, `findings` (each with an `id`, `F1`, `F2` and so on, unique within the reply, so that the verifier can answer per finding) and `evidence`. `agents/reviewer.md` spells the shape out.
- When the verifier ran, its report is stored beside the result as `<head12>-<n>.verifier.md`.

## What makes a result valid

A result is valid when `reviewresult.Parse` (the package `engine/reviewresult`) accepts it against the candidate the parent launched the reviewer on: the full `base` and `head`, the `diff_sha256` of `git diff base head`, and the paths of `git diff --name-only base head`. The parent computes those; the reviewer copies the first three from its prompt and never computes them. A reply that breaks a rule is refused with an error that names the rule, and it is a review that did not run (see below), never one that approved. The package applies the rules in this order and reports the first one broken, so the same reply is always refused for the same reason:

1. The binding names a candidate: `base` and `head` are full git object names and `diff_sha256` is a SHA-256 digest.
2. The reply is the object `agents/reviewer.md` spells out: valid UTF-8, no key written twice, and every member present and not null, in the reply, in `candidate`, in `inspection` and in each finding. `"findings":[]` says nothing was found; a missing or null `findings` says nothing at all, so it is never a clean review.
3. No object has a member the contract does not name. Names are matched exactly, so `Severity` is not `severity`.
4. `candidate` echoes the `base`, `head` and `diff_sha256` of the binding, each one.
5. `inspection.status` is `completed`. An `incomplete` inspection carries its `reason`.
6. `inspection.paths` are the paths of the diff, compared as sets: a path of the diff that the reviewer did not list is a file nobody read, and a path it lists that the diff does not change is not an inspection of this candidate.
7. Each finding has an `id` that is not blank and that no earlier finding has.
8. `severity` is `BLOCKER`, `CRITICAL`, `WARNING` or `SUGGESTION`; `evidence_class` is `deterministic` or `inferential`; `causal_disposition` is `introduced`, `behavior-activated`, `worsened`, `pre-existing`, `base-only` or `unknown`.
9. A `BLOCKER` or `CRITICAL` has at least one `proof_refs` entry that is not blank.

No command calls the package yet: `labdrian review record` will, before it stores a result (`odd/tasks/own-rdd.md`, T2).

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
