# Review results

The parent of the role chain (`skills/role-chain/SKILL.md`) binds a review to its commits with `labdrian review bind`, stores what the `reviewer` agent returns with `labdrian review record`, one file per launch: `odd/review-results/<feature>/<head12>-<n>.json`, and stores what the `verifier` agent returns about the findings of a result with `labdrian review verify`, beside it.

- `<feature>` is the name of the feature document, `odd/tasks/<feature>.md`, and one safe path segment: 1 to 100 characters, each a letter or digit of ASCII, `.`, `_` or `-`, the first a letter or a digit, with no `..`. `record` refuses any other name, and refuses to write through a symbolic link on the way to the file, so no input names a path outside `odd/review-results/`.
- `<head12>` is the first 12 hex characters of the reviewed `head`. `<n>` counts the launches stored for that head, from 1: a second blind launch takes the next number, one more than the highest stored for that head (a verdict counts). `record` creates the file and never writes over one that exists; the file is published whole or not at all (a temporary file in the same directory, flushed, then linked to its name).
- The file is one JSON object, `version` 1: the `binding` the reply was held to, and the `reply` exactly as the reviewer gave it (white space aside). `agents/reviewer.md` spells the reply out: `candidate` (`base`, `head`, `diff_sha256`), `inspection`, `findings` (each with an `id`, `F1`, `F2` and so on, unique within the reply, so that the verifier can answer per finding) and `evidence`.

  ```json
  {
    "version": 1,
    "binding": {"base": "<full sha>", "head": "<full sha>", "diff_sha256": "<64 hex>", "paths": ["<changed path>"]},
    "reply": {"candidate": {}, "inspection": {}, "findings": [], "evidence": []}
  }
  ```

  The reviewed head is `binding.head`. `reply.candidate.head` is the same value, because a reply that echoes another is refused.
- When the verifier ran, its verdict is stored beside the result it answers, under the same `<head12>-<n>`, as `<head12>-<n>.verifier.json`: the reply of the verifier exactly as it gave it (white space aside), one JSON object (see "The verdict of the verifier" below). A result has one verdict, and `verify` never writes over a file.

## The three commands

`labdrian review bind --base <rev> --head <rev> [--cwd <repo>]` resolves both revisions to full commit names and prints one JSON object: `base`, `head`, `diff_sha256` and `paths` (the binding, defined below), `worktree`, `worktree_created`, `diff_bytes`, `diff_limit_bytes`, `over_limit` and, when the diff is over the limit, `note`. It writes nothing but the worktree.

- The worktree is the detached one the reviewer reads: `<repo-parent>/<repo-name>-worktrees/review-<head12>`, beside the main checkout of the repository (`~/labdrian-sdd-overlay-worktrees/review-<head12>` for this one) and never inside it, whichever worktree of it the command runs in. It is made at `head` with git's hooks switched off. One already there at `head` is reused; a path that is a worktree of another commit, or that is not a worktree of this repository, is refused and left as it is. A reused worktree is not checked for edits.
- A diff over 150000 bytes (`diff_limit_bytes`) is bound all the same: `over_limit` is `true` and `note` says so. Splitting the candidate into one launch per commit is the parent's job (`skills/role-chain/SKILL.md`); the command does not fail.

`labdrian review record --feature <name> --base <rev> --head <rev> [--result <file>|-] [--cwd <repo>]` computes the binding again from git, never from the reply, holds the reply (the file, or stdin) to it with `reviewresult.Parse` (below), and stores it. A reply above 1 MiB is refused. Exit 0 stored, 2 reply refused (the error names the rule, and nothing was written), 1 usage error or any other failure.

- stderr says where the result was stored and the sha256 of the stored file, and names the members of the skeleton below that the parent fills.
- stdout is the skeleton of the record that closes the review, for `labdrian roles append --stdin`: `from_role` `reviewer`, `payload_kind` `review-report`, `payload_sha256` the sha256 of the stored file, `status` `completed`, and the evidence `review-diff` (`<base>..<head>`, `diff_sha256`) and `review-result` (the stored path relative to the repository, the sha256 of the stored file). The members only the parent of the chain knows are left empty: `project_id`, `goal_id`, `chain_id`, `seq`, `prev_sha256`, `to_role` (`builder` when a severe finding blocks, `delivery` otherwise) and `context.summary`. `labdrian roles validate` refuses the skeleton, naming the first of them, until they are filled, so it is never appended by accident. When the verifier ran, `verify` prints the same skeleton with the `verifier-report` evidence added.

`labdrian review verify --feature <name> --stored <result> [--verdicts <file>|-] [--cwd <repo>]` holds the reply of the verifier (the file, or stdin) to the stored result called `<result>`, the name of a file of `odd/review-results/<feature>/` such as `<head12>-<n>.json`, with `reviewverdict.Parse` (below), decides whether the review blocks (below), and stores the reply beside the result. A reply above 1 MiB is refused. Exit 0 stored, 2 reply refused (the error names the rule, and nothing was written), 1 usage error or any other failure, among them a result that already has a verdict and a correction round that was spent (below).

- The candidate is the one stored with the result, which names both commits in full: git is asked only where the repository is. The result is read as `record` wrote it (one JSON object of `version` 1) and its reply is held to its binding again, so a file that is not a result, a name that is not one of `<head12>-<n>.json`, a file that is not a regular file and a file above 8 MiB are refused before the verdict is looked at.
- stderr says where the verdict was stored and the sha256 of the stored file, what was decided, which findings are blocking, unverified, to fix before delivery or to carry, and names the members of the skeleton that the parent fills.
- stdout is the skeleton `record` printed for the same result, with the evidence `verifier-report` (the stored verdict relative to the repository, the sha256 of the stored file) added and `to_role` filled: `builder` when the review blocks, `delivery` when it is clear, and none when a check did not run (the review is neither clear nor blocked, so the parent fills `to_role` once the check has run).

## The verdict of the verifier

The `verifier` agent (`agents/verifier.md`) is launched on a stored result and replies with one JSON object and no prose, one verdict for each finding of the result:

```json
{"verdicts": [{"id": "F1", "result": "reproduced", "command": "go test ./engine/...", "output": "--- FAIL: TestGuard", "base_result": "not-reproduced"}]}
```

- `id` is the `id` of a finding of the result, and each finding is answered once.
- `result` is what the check found on `head`, the commit under review, and `base_result` what the same check found on `base`: `reproduced`, `not-reproduced` or `not-run`. `not-run` says the check could not run or was not tried, and is never read as `not-reproduced`: a check that cannot run proves nothing.
- `command` is the command the verifier ran, on head and on base. It is empty only when no check ran, that is when `result` and `base_result` are both `not-run`. `output` is what the command printed, or why no check ran, and is never blank.

`reviewverdict.Parse` (the package `engine/reviewverdict`) applies these rules in this order and reports the first one broken, so the same reply is always refused for the same reason. A reply that breaks one is a verification that did not run, never a clear review.

1. The reply is one JSON value in valid UTF-8, with no key written twice (`ErrUnparseable`).
2. The reply has the shape above. Its objects are looked at one at a time, in the order the reply holds them (the reply, then each verdict), and the first fault is the one reported. Each must be an object; it has no member the contract does not name (`ErrUnknownMember`; names are matched exactly); it has every member the contract names, none of them `null`; `verdicts` is an array; and every value is of its type (`ErrUnparseable`).
3. Each verdict, in order and one at a time, answers a finding of the result (`ErrUnknownID`) that no earlier verdict answered (`ErrDuplicateID`); has a `result` and a `base_result` that are values listed above (`ErrOutsideEnum`); and carries its evidence: an `output`, and the `command` that produced it whenever a check ran, on head or on base (`ErrMissingEvidence`).
4. Every finding of the result is answered (`ErrUnanswered`): a finding nobody checked is never read as one that did not reproduce.

### Whether the review blocks

`reviewverdict.Decide` is the one pure function that turns the result and its verdicts into the decision of `skills/role-chain/SKILL.md`. It reads nothing and keeps nothing:

| finding | what the verifier found | it becomes |
|---|---|---|
| `BLOCKER` or `CRITICAL`, and `introduced`, `behavior-activated` or `worsened` | `result` `reproduced` | blocking |
| the same | `result` `not-reproduced` | nothing |
| the same | `result` `not-run` | unverified |
| `BLOCKER` or `CRITICAL`, and `pre-existing` or `base-only` | anything | carried as a follow-up |
| `BLOCKER` or `CRITICAL`, and `unknown` | `result` `not-reproduced` | nothing |
| the same | `base_result` `reproduced` | carried: the diff did not cause it |
| the same | `base_result` `not-run` | unverified |
| the same | `base_result` `not-reproduced` | decided like a finding the diff caused |
| `WARNING` | anything | to fix before delivery |
| `SUGGESTION` | anything | carried |

For `unknown` the rows are read in order, and the first that matches decides. The outcome is `blocks` when anything is blocking, else `unverified` when a finding that could block was not checked, else `clear`. A finding with no verdict cannot occur after `Parse`, and `Decide` reads one as a check that did not run.

### One correction round

A candidate is corrected at most once. The candidate is the feature and the `base`; each `head` reviewed against it is one try. A verdict that blocks sends the result back to the builder, and the fix is reviewed against the same `base`, as another `head`. If that review is blocked too, `verify` refuses its verdict (exit 1, `the correction round of the candidate was spent`, naming the verdict that spent it) and stores nothing: the owner decides.

- The round is recorded by the stored verdict itself. `verify` reads each verdict stored in the directory of the feature, decides it again from its own result, and counts one that blocks and whose result is for another `head` of the same `base`; no counter is kept beside the files, so none can drift from them. A result or a verdict that cannot be read to tell is a refusal, never an unspent round.
- A verdict that blocks is checked and stored under a lock of the directory of the feature, so two `verify` calls at the same time on two heads of one base cannot both find the round unspent: the second waits, then finds the first verdict and is refused. On linux the lock is an exclusive `flock(2)` on the directory, opened without following a link; it is given back when the verdict is stored, or when the process ends. Elsewhere no lock is held, and the calls are run one after another.
- A second launch on the same `head` is no correction, and a range from another `base` is another candidate. A fix that is reviewed alone, as the range from the reviewed `head` to the fix, has another `base` and so is another candidate: the rule does not see it.
- Reopening the round is the owner's act: removing the verdict that spent it.

## The digest and the paths of a candidate

`diff_sha256` is the sha256 of the bytes this command prints, run in the root of a working tree of the repository, with `<base>` and `<head>` the full object names:

```
git -c core.quotePath=true -c diff.suppressBlankEmpty=false -c core.attributesFile=/dev/null -c core.compression=0 -c core.looseCompression=0 --attr-source=4b825dc642cb6eb9a060e54bf8d69288fbee4904 diff-tree -p -r --full-index --binary --no-renames --no-color --no-ext-diff --no-textconv --ignore-submodules=none --src-prefix=a/ --dst-prefix=b/ -U3 <base> <head>
```

It runs under the environment of the process without any variable whose name begins with `GIT_`, with `GIT_ATTR_NOSYSTEM=1` and `LC_ALL=C` added. `--attr-source` is the empty tree of the hash function of the repository: `4b825dc642cb6eb9a060e54bf8d69288fbee4904` where objects are named with 40 characters, and `6ef19b41225c5369f1c104d45d8d85efa9b057b53b14b4b9b939dd74decc5321` where they are named with 64. It needs git 2.40 or later, the first with `--attr-source`; an older git refuses the command, and `bind` and `record` fail with what git said.

`paths` are the paths the diff changes, from the NUL-terminated output of

```
git diff-tree -r --name-only -z --no-renames --ignore-submodules=none <base> <head>
```

byte for byte, in the order git prints them, one entry per path. A rename is the removal of the old path and the creation of the new one, which is how the diff above shows it too (`git diff --name-only` would list only the new name).

Why the command is spelled out: the same two commits must give the same bytes on every run, so that `record` and the delivery check recompute the digest a reply was bound to. `diff-tree` is plumbing that ignores the settings a person changes for `git diff` (`diff.algorithm`, `diff.context`, `diff.noprefix`, `diff.renames`, `diff.external`, `color.*`, `core.abbrev` and the like). Five parts are there because the plumbing itself reads what they pin: `core.quotePath` (how a name that is not ASCII is quoted), `diff.suppressBlankEmpty` (whether a blank context line keeps its space), `core.attributesFile` with `--attr-source` (which keep every file of attributes out, so that no `.gitattributes` of the candidate, of the commit checked out or of the working tree decides what is binary or names a diff driver), and `core.compression` with `core.looseCompression` (the level of the zlib stream of a binary patch; 0 stores it). The other flags write the same definition out so that a later git cannot change it. The variables of git are removed because they win over the command line (`GIT_DIFF_OPTS` changes the context, `GIT_CONFIG_*` the configuration, `GIT_DIR` and `GIT_INDEX_FILE` the repository), and the messages are not translated. The tests of `engine/review/gitexec` run the real git against each of these parts.

What is not pinned, and so what can make two machines disagree: `$GIT_DIR/info/attributes` of the repository (a file kept by hand, outside any commit), and the zlib of the machine, whose output for a binary patch is the same on the zlibs we know and is not promised to be on all. A digest that does not agree is refused, never accepted: a reply bound to a digest the check does not recompute is not found.

## What makes a result valid

A result is valid when `reviewresult.Parse` (the package `engine/reviewresult`) accepts it against the candidate that `labdrian review bind` computes: the full `base` and `head`, the `diff_sha256` of the canonical diff, and its `paths` (both defined above). The parent runs `bind` and sends the reviewer the candidate; the reviewer copies the first three from its prompt and never computes them; `record` computes them again from git and trusts nothing the reply says about them. A reply that breaks a rule is refused with an error that names the rule, and it is a review that did not run (see below), never one that approved. The package applies the rules in this order and reports the first one broken, so the same reply is always refused for the same reason:

1. The binding names a candidate: `base` and `head` are full git object names, `diff_sha256` is a SHA-256 digest, and the paths of the diff list each path once.
2. The reply is one JSON value in valid UTF-8, with no key written twice.
3. The reply has the shape `agents/reviewer.md` spells out. Its objects are looked at one at a time, in the order the reply holds them (the reply, `candidate`, `inspection`, then each finding), and the first fault is the one reported. Each must be an object; it has no member the contract does not name (names are matched exactly, so `Severity` is not `severity`); it has every member the contract names, none of them `null`; and no array in it (`evidence`, `inspection.paths` and the `proof_refs` of a finding) has a `null` among its elements. `"findings":[]` says nothing was found; a missing or null `findings` says nothing at all, so it is never a clean review. The `id` of a finding is the one member this rule does not ask for: rule 8 does.
4. Every value is of its type, checked once every object has been looked at.
5. `candidate` echoes the `base`, `head` and `diff_sha256` of the binding, each one.
6. `inspection.status` is `completed`. An `incomplete` inspection carries its `reason`.
7. `inspection.paths` are the paths of the diff, compared as sets in which each path is listed once: a path of the diff that the reviewer did not list is a file nobody read, a path it lists that the diff does not change is not an inspection of this candidate, and a path it lists twice makes a list that is not the list of the diff.
8. Each finding has an `id` that is present, not `null` and not blank, and that no earlier finding has.
9. `severity` is `BLOCKER`, `CRITICAL`, `WARNING` or `SUGGESTION`; `evidence_class` is `deterministic` or `inferential`; `causal_disposition` is `introduced`, `behavior-activated`, `worsened`, `pre-existing`, `base-only` or `unknown`.
10. A `BLOCKER` or `CRITICAL` has at least one `proof_refs` entry that is not blank.

Rules 8 to 10 are applied finding by finding, in the order the reviewer wrote them: a finding is held to all three before the next one is looked at.

`labdrian review record` calls the package before it stores a result (`odd/tasks/own-rdd.md`, T2), and `labdrian review verify` calls `reviewverdict.Parse` before it stores a verdict (T3), so no file of this directory is a reply that broke a rule.

## Binding to the chain

The record that closes the review is a RoleHandoff appended with `labdrian roles append`; `record` and `verify` print its skeleton. Its `payload_kind` is `review-report` and its `payload_sha256` is the sha256 of the stored result. Its evidence binds the bytes under review:

| kind | ref | sha256 |
|---|---|---|
| `review-diff` | `<base>..<head>` | `diff_sha256`, the sha256 of the canonical diff above |
| `review-result` | the stored result | its sha256 |
| `verifier-report` | the stored verdict, when the verifier ran | its sha256 |

Delivery recomputes the sha256 of the canonical diff of the range (`labdrian review bind` prints it) and trusts the record only on equality: other bytes give another digest, and the record does not apply to them.

A review that did not run (a refused launch, an unparseable result, an echo that differs, an incomplete inspection) is recorded `interrupted` with a `resume_reason`, never as approved. `record` stores nothing for it.

## What a result is not

A result here is check evidence in the role chain, never review authority. It carries no `schema`, lineage, receipt or `GENTLE_AI_*` member. `odd/review-receipts/` and the `gentle-ai review` lifecycle stay as they are: both can run on the same commit, because they share nothing but its bytes.
