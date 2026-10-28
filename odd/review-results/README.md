# Review results

The parent of the role chain (`skills/role-chain/SKILL.md`) binds a review to its commits with `labdrian review bind`, stores what the `reviewer` agent returns with `labdrian review record`, one file per launch: `odd/review-results/<feature>/<head12>-<n>.json`, and stores what the `verifier` agent returns about the findings of a result with `labdrian review verify`, beside it. A head whose work takes no review is exempted from it instead, with `labdrian review exempt`, on the report of the `verifier` agent and the risk the agent declares: `odd/review-results/<feature>/<head12>-<n>.exempt.json`. The delivery role holds a head to those files with `labdrian review check` before it closes the feature.

- `<feature>` is the name of the feature document, `odd/tasks/<feature>.md`, and one safe path segment: 1 to 100 characters, each a letter or digit of ASCII, `.`, `_` or `-`, the first a letter or a digit, with no `..`. `record` refuses any other name, and refuses to write through a symbolic link on the way to the file, so no input names a path outside `odd/review-results/`.
- `<head12>` is the first 12 hex characters of the reviewed `head`. `<n>` counts the launches stored for that head, from 1: a second blind launch takes the next number, one more than the highest stored for that head (a verdict and an exemption count). `record` creates the file and never writes over one that exists; the file is published whole or not at all (a temporary file in the same directory, flushed, then linked to its name).
- The file is one JSON object, `version` 1: the `binding` the reply was held to, the `rows` the launch was sent (see "The lens rows of a launch" below), and the `reply` exactly as the reviewer gave it (white space aside). `agents/reviewer.md` spells the reply out: `candidate` (`base`, `head`, `diff_sha256`), `inspection`, `findings` (each with an `id`, `F1`, `F2` and so on, unique within the reply, so that the verifier can answer per finding) and `evidence`.

  ```json
  {
    "version": 1,
    "binding": {"base": "<full sha>", "head": "<full sha>", "diff_sha256": "<64 hex>", "paths": ["<changed path>"]},
    "rows": ["default", "readability"],
    "reply": {"candidate": {}, "inspection": {}, "findings": [], "evidence": []}
  }
  ```

  The reviewed head is `binding.head`. `reply.candidate.head` is the same value, because a reply that echoes another is refused.
- When the verifier ran, its verdict is stored beside the result it answers, under the same `<head12>-<n>`, as `<head12>-<n>.verifier.json`: the reply of the verifier exactly as it gave it (white space aside), one JSON object (see "The verdict of the verifier" below). A result has one verdict, and `verify` never writes over a file.
- When a head is exempted from review, the exemption is stored under the next number of the head as `<head12>-<n>.exempt.json`: one JSON object, `version` 1, with the `binding`, the `risk` that was declared and the `report` of the verifier exactly as it gave it (white space aside); see "The exemption of a head from review" below. `exempt` never writes over a file either.

## The commands `bind`, `record` and `verify`

`labdrian review bind --base <rev> --head <rev> [--cwd <repo>]` resolves both revisions to full commit names and prints one JSON object: `base`, `head`, `diff_sha256` and `paths` (the binding, defined below), `worktree`, `worktree_created`, `diff_bytes`, `diff_limit_bytes`, `over_limit` and, when the diff is over the limit, `note`. It writes nothing but the worktree.

- The worktree is the detached one the reviewer reads: `<repo-parent>/<repo-name>-worktrees/review-<head12>`, beside the main checkout of the repository (`~/labdrian-sdd-overlay-worktrees/review-<head12>` for this one) and never inside it, whichever worktree of it the command runs in. It is made at `head` with git's hooks switched off. One already there at `head` is reused; a path that is a worktree of another commit, or that is not a worktree of this repository, is refused and left as it is. A reused worktree is not checked for edits.
- A diff over 150000 bytes (`diff_limit_bytes`) is bound all the same: `over_limit` is `true` and `note` says so. Splitting the candidate into one launch per commit is the parent's job (`skills/role-chain/SKILL.md`); the command does not fail.
- A range whose base is its head covers no commit and is refused (exit 1, `base and head are the same commit`, `review.ErrInvalidCandidate`) by `bind`, and so by `record`, `exempt` and `check`, which bind a range in the same way: nothing reviewed or exempted over an empty diff clears a head, and nothing is made for it, not even the worktree.

`labdrian review record --feature <name> --base <rev> --head <rev> --rows <id,id> [--result <file>|-] [--cwd <repo>]` computes the binding again from git, never from the reply, holds the reply (the file, or stdin) to it with `reviewresult.Parse` (below), and stores it with the lens rows the reviewer was sent (`--rows`, required, defined in "The lens rows of a launch" below). A reply above 1 MiB is refused. Exit 0 stored, 2 reply refused (the error names the rule, and nothing was written), 1 usage error or any other failure, rows that are not rows of the canonical file included.

- stderr says where the result was stored and the sha256 of the stored file, and names the members of the skeleton below that the parent fills.
- stdout is the skeleton of the record that closes the review, for `labdrian roles append --stdin`: `from_role` `reviewer`, `payload_kind` `review-report`, `payload_sha256` the sha256 of the stored file, `status` `completed`, and the evidence `review-diff` (`<base>..<head>`, `diff_sha256`) and `review-result` (the stored path relative to the repository, the sha256 of the stored file). The members only the parent of the chain knows are left empty: `project_id`, `goal_id`, `chain_id`, `seq`, `prev_sha256`, `to_role` (`builder` when a severe finding blocks, `delivery` otherwise) and `context.summary`. `labdrian roles validate` refuses the skeleton, naming the first of them, until they are filled, so it is never appended by accident. When the verifier ran, `verify` prints the same skeleton with the `verifier-report` evidence added.

`labdrian review verify --feature <name> --stored <result> [--verdicts <file>|-] [--cwd <repo>]` holds the reply of the verifier (the file, or stdin) to the stored result called `<result>`, the name of a file of `odd/review-results/<feature>/` such as `<head12>-<n>.json`, with `reviewverdict.Parse` (below), decides whether the review blocks (below), and stores the reply beside the result. A reply above 1 MiB is refused. Exit 0 stored, 2 reply refused (the error names the rule, and nothing was written), 1 usage error or any other failure, among them a result that already has a verdict and a correction round that was spent (below).

- The candidate is the one stored with the result, which names both commits in full: git is asked only where the repository is. The result is read as `record` wrote it (one JSON object of `version` 1) and its reply is held to its binding again, so a file that is not a result (a file whose `rows` are missing or are not a list of the rows of a launch included), a name that is not one of `<head12>-<n>.json`, a file that is not a regular file and a file above 8 MiB are refused before the verdict is looked at.
- stderr says where the verdict was stored and the sha256 of the stored file, what was decided, which findings are blocking, unverified, to fix before delivery or to carry, and names the members of the skeleton that the parent fills.
- stdout is the skeleton `record` printed for the same result, with the evidence `verifier-report` (the stored verdict relative to the repository, the sha256 of the stored file) added and `to_role` filled: `builder` when the review blocks, `delivery` when it is clear, and none when a check did not run (the review is neither clear nor blocked, so the parent fills `to_role` once the check has run).

## The lens rows of a launch

The reviewer is sent the lens rows of its launch in its prompt (`agents/reviewer.md`: "Scope is the lens rows in this prompt"). The sentences of the rows are written once, in `roles/reviewer/rows.json`, and the parent of the chain copies the ones a launch needs from there:

| id | the row |
|---|---|
| `default` | reliability and resilience, and a risk line (secrets, permissions, path escapes, exec or net imports) |
| `readability` | added when docs, a public API, messages or help change |
| `risk-line` | the second blind launch, with another `model`, that a Risk line which names an item asks for |

- `record` takes the rows the launch was sent from `--rows`: their ids separated by commas with no space and none written twice, in the order the parent sent them (`default,readability`). It refuses a list that is empty, that holds an id which is not in the table, or that writes an id twice (exit 1, nothing written, and the error names the ids that are accepted). The table is the program's (`review.RowIDs`), and a test of `engine/review` holds its ids and their order to `roles/reviewer/rows.json`, so a row is added in both.
- The stored result records them, in the member `rows` and in that order. `verify` and `check` read them back with the result and refuse a stored file whose `rows` are missing or are not such a list, as they refuse any file `record` does not write. A result stored before the rows were recorded must be recorded again with `--rows`; none is kept in this repository, and none was released.
- The rows are what the parent declares it sent, as the risk of an exemption is what the agent declares: the program cannot see the prompt, so it holds the declaration to the table and nothing more.
- `review_lens_count` of the actuals record (`skills/_shared/actuals-record.schema.json`) is the number of rows recorded in the stored results of the change, one for each row of each launch, and is omitted when no stored result records one. It is no longer read from the `selected_lenses` of the receipt of another tool.

## The count of the lens rows

The number is checked when the actuals record is written, not trusted (`odd/tasks/own-rdd.md`, T10, S13, L29). `CountLenses` of `engine/review/app` is the one function that makes it, and `tools/actuals-record-validator` calls it instead of making a count of its own; any other reader of the number calls it too.

- `CountLenses` lists `odd/review-results/<change_name>/` under the repository root it is given (the change name of the record is the name of the feature document, and a directory that is not there holds no result), reads each file named like a result as `verify` and `check` read one, and adds the length of the `rows` of each. It returns the number of results and the number of rows. A verdict, an exemption and a file of any other name are not results and are not read. It reads files and asks nothing of git.
- It refuses what it cannot count, with an error and no number: a change name that is not one safe path segment, a directory that cannot be listed, a file named like a result that cannot be read (a link, a file that is not a regular file, a file above 8 MiB), and one that is not a result any more (`rows` missing or not the rows of a launch, a reply that no longer holds to its binding). A count that left such a file out would be a count of the files that happened to read.
- The validator takes the repository root as `--repo`, which is required and is an absolute path: `go run -C` changes the working directory, so the callers pass one (`skills/_shared/pre-sdd-contracts.md`, `skills/inception-pipeline/SKILL.md`). Without it, or with a relative path, the validator exits 2 and checks nothing. It is also the root of a repository, a directory that holds a `.git` entry (a file in a linked worktree): any other directory holds none of the stored results, and an omitted count would pass as "nothing to compare" while the repository records rows, so it is exit 7. The checks of the shape and of the other rules run first, so a record that breaks one of them is refused for it as before.

| the record | the stored results of the change | the validator |
|---|---|---|
| `review_lens_count` N, above 0 | record N rows | exit 0, and a second line says what was compared: `review_lens_count: N equals the N rows recorded in K stored review results of "<change>"` |
| `review_lens_count` N | record M rows, M above 0 and other than N | exit 6, `review_lens_count is N but the stored review results of "<change>" record M rows (K stored results in odd/review-results/<change>)`, and says to write M |
| `review_lens_count` N, 0 included | none: the directory is missing or holds no result | exit 6, `review_lens_count is N but the stored review results of "<change>" record 0 rows (no stored review result in odd/review-results/<change>): nothing backs a count for this change`, and says to omit it |
| omitted | record M rows, M above 0 | exit 6, `review_lens_count is omitted but the stored review results of "<change>" record M rows`, and says to write M |
| omitted | record no row | exit 0, and a second line says that nothing was compared: `review_lens_count: omitted, and no stored review result of "<change>" records a row: there was nothing to compare` |
| any | cannot be counted (`--repo` is not a directory or holds no `.git` entry, or a file named like a result is not one) | exit 7, naming the repository or the file, and that `review_lens_count` cannot be checked |

- A change reviewed before the rows were stored (a gentle receipt and no stored result: nothing in `odd/review-results/` was ever written for it) has no row to count, so its record omits `review_lens_count`. A count taken from the receipt of another tool is the third row of the table: it is refused and never passed, because the validator cannot tell it from a count that was made up. A count of 0 is a count like any other: with no stored result it is the third row too, and only omitting the field is the outcome for such a change.
- Exit 7 is new. The exit codes 0, 2, 3, 4, 5 and 6 keep their meanings: 2 is now also a missing or relative `--repo`. A pass prints two lines on stdout where it printed one (it is the `--help` text of the validator that says so too): `ok: actuals record satisfies <schema> and every semantic invariant`, then the line of the table above that says what the count check compared. A caller that reads the output must not take the first line for the whole of it; the exit code stays what authorises the write.

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
- A verdict that blocks is checked and stored under a lock of the directory of the feature, so two `verify` calls at the same time on two heads of one base cannot both find the round unspent: the second waits, then finds the first verdict and is refused. On every unix (linux, macOS, the BSDs) the lock is an exclusive `flock(2)` on the directory, opened without following a link; it is given back when the verdict is stored, or when the process ends. A holder that keeps it more than 30 seconds is taken to be stuck, and the waiting `verify` stops with `another holder kept the lock of the directory of the feature` (exit 1, nothing stored); so does a file system that cannot lock. Windows has no `flock(2)`: there no lock is held, and the calls are run one after another.
- A second launch on the same `head` is no correction, and a range from another `base` is another candidate. A fix that is reviewed alone, as the range from the reviewed `head` to the fix, has another `base` and so is another candidate: the rule does not see it.
- Reopening the round is the owner's act: removing the verdict that spent it.

## The exemption of a head from review

Work that takes no review is delivered on the evidence of the verifier alone, as long as its risk is validated too (`odd/tasks/own-rdd.md`, T4, S5). `labdrian review exempt --feature <name> --base <rev> --head <rev> --risk <none|items> [--cwd <repo>]` holds the risk the agent declares and the report of the verifier to the rules below, computes the binding again from git, never from the report, and stores the exemption. The report is read from stdin and from nowhere else: there is no flag for a file. A report above 1 MiB is refused. Exit 0 stored, 2 report refused (the error names the rule, and nothing was written), 1 usage error, a risk that is refused or any other failure.

The rules are applied in this order, and the first one broken is the refusal, with nothing stored:

1. `<feature>` is one safe path segment, as for `record`.
2. The declared risk is `none`. `--risk` is `none` or a list of the numbers 1 to 6 of the high-risk items of the orchestrator's Task Size list, separated by commas with no space and none written twice (`2,4`): 1 data or irreversible effects, 2 security, 3 contracts others consume, 4 concurrency, 5 delivery or environment, 6 no test would catch a regression. The engine does not compute the risk from the diff: it holds the declaration to this grammar, and a head that names any item is never exempt, because that is what a review is for (`review.ErrHighRisk`; the error names every item written). A declaration that is anything else (an empty one, `None`, a number outside the list, a space after a comma, an item written twice) is not taken for the absence of risk: it is refused as one the engine cannot read (`review.ErrInvalidRisk`). A declaration is not a report, so both refusals are exit 1.
3. The report is the object below and is clear. A report that is missing, is not that object or is not clear is refused with exit 2.
4. The revisions are ones git can read and are two commits (a range whose base is its head is refused, above), and the binding is the candidate git gives for them.
5. The exemption is for one commit: `--base` is the first parent of `--head` as git gives it (`review.CheckOneCommit`; for a merge, the parent it was made on). A base that is further back, a base that is not an ancestor, or a head with no parent is refused (`review.ErrNotOneCommit`, exit 1, `an exemption is for one commit, so its base must be the first parent of its head`, naming both commits) and nothing is stored. Reviews are per commit, and a chain walks back from the head over the base of the exemption that binds each commit: one that spanned several commits would step over their reviews, a blocked one included. A result is not held to this: the fix of a commit whose review blocked is reviewed against the same base as that review (one correction round, above), so its range may hold the commit and the fix.

The report is the verifier's, one JSON object and no prose, with the checks it ran in the order it ran them (`agents/verifier.md` spells it out):

```json
{"checks": [{"command": "go test ./engine/review/...", "result": "pass", "output": "ok  github.com/labdrian-ai/labdrian-sdd-overlay/engine/review 0.009s"}]}
```

- `command` is the command the verifier ran, or would have run when the check did not run. `output` is what the command printed, or why it did not run, and is never blank.
- `result` is what the check found: `pass`, `fail` or `not-run`. `not-run` says the check could not run or was not tried, and is never read as `pass`. The result is the verifier's, because whether a command passed is not always its exit code (a search that finds nothing exits 1 and is the check passing).

`reviewverdict.ParseReport` (the package `engine/reviewverdict`) applies these rules in this order and reports the first one broken, so the same report is always refused for the same reason:

1. The report is one JSON value in valid UTF-8, with no key written twice (`ErrUnparseable`). An empty report is one that is missing.
2. The report has the shape above. Its objects are looked at one at a time, in the order the report holds them (the report, then each check); each has no member the contract does not name (`ErrUnknownMember`; names are matched exactly), has every member the contract names, none of them `null`, `checks` is an array, and every value is of its type (`ErrUnparseable`).
3. Each check, in order and one at a time, has a `result` that is a value listed above (`ErrOutsideEnum`) and carries its evidence: a `command` and an `output`, neither blank (`ErrMissingEvidence`).
4. The report is clear (`ErrNotClear`): it names at least one check and every check is `pass`. A report that names no check proves nothing, and a check that failed or did not run never passes.

`exempt` stores the exemption as `odd/review-results/<feature>/<head12>-<n>.exempt.json`, with `<n>` one more than the highest stored for that head, results and verdicts included (from 1). The file is published whole or not at all and never over one that exists. It is one JSON object, `version` 1:

```json
{
  "version": 1,
  "binding": {"base": "<full sha>", "head": "<full sha>", "diff_sha256": "<64 hex>", "paths": ["<changed path>"]},
  "risk": "none",
  "report": {"checks": [{"command": "<command>", "result": "pass", "output": "<output>"}]}
}
```

The `risk` is the declaration that was made and the `report` the report exactly as the verifier gave it (white space aside), so that any later reader holds the exemption to the rules it was made under again (`review.CheckExemption`) and compares its `binding` with the one git gives (`review.CheckBinding`), without asking the verifier or the parent for anything. A file with a member it does not have, a key written twice, another version, no `risk` or no `report` is not an exemption.

- stderr says where the exemption was stored and the sha256 of the stored file, says that the head is exempted from review because the declared risk is `none` and the report is clear, and names the members of the skeleton below that the parent fills.
- stdout is the skeleton of the record that closes the exemption, for `labdrian roles append --stdin`: `from_role` `reviewer`, `to_role` `delivery`, `payload_kind` `review-exemption`, `payload_sha256` the sha256 of the stored file, `status` `completed`, and the evidence `review-diff` (`<base>..<head>`, `diff_sha256`) and `review-exemption` (the stored path relative to the repository, the sha256 of the stored file). The members only the parent of the chain knows are left empty: `project_id`, `goal_id`, `chain_id`, `seq`, `prev_sha256` and `context.summary`. `labdrian roles validate` refuses the skeleton, naming the first of them, until they are filled, so it is never appended by accident.

An exemption grants nothing by itself. It is evidence that a head was checked and a risk was declared; `labdrian review check` is what clears a head (below), and it holds the exemption to the file and to git again every time. `exempt` does not look at the results of the head: which of the two stands is the check's to say (below).

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

The rows of the launch are not part of the reply, so the package does not read them: `record` holds them to the table of the lens rows (above) right after the feature and before it looks at the reply, and a stored file is a valid result for `verify` and `check` only with rows that hold to it too.

`labdrian review record` calls the package before it stores a result (`odd/tasks/own-rdd.md`, T2), and `labdrian review verify` calls `reviewverdict.Parse` before it stores a verdict (T3), so no file of this directory is a reply that broke a rule.

## Binding to the chain

The record that closes the review is a RoleHandoff appended with `labdrian roles append`; `record` and `verify` print its skeleton. Its `payload_kind` is `review-report` and its `payload_sha256` is the sha256 of the stored result. Its evidence binds the bytes under review:

| kind | ref | sha256 |
|---|---|---|
| `review-diff` | `<base>..<head>` | `diff_sha256`, the sha256 of the canonical diff above |
| `review-result` | the stored result | its sha256 |
| `verifier-report` | the stored verdict, when the verifier ran | its sha256 |

`labdrian roles append` holds a completed `review-report` record to the files `record` and `verify` stored, in the repository of its working directory, before it stores the record (`odd/tasks/own-rdd.md`, T4). It refuses the record (exit 1, nothing stored, and the message names the evidence kind and its ref) when:

- it lacks the `review-diff` or the `review-result` evidence, or carries a kind twice, so that none of the pieces is the evidence of its kind (the `verifier-report` is optional: the verifier may not have run);
- the ref of the `review-result` is not exactly one `odd/review-results/<feature>/<head12>-<n>.json` (no other path, none that leaves the directory, no verdict, no temporary file), or the file is not stored, or its sha256 is not the one of the evidence, or the file is not a result. It is read as `verify` reads it, through the store that refuses a link on the way, a file that is not a regular file and a file above 8 MiB;
- the `review-diff` is not the range `<base>..<head>` and the `diff_sha256` of the binding stored in that result;
- the `verifier-report`, when there is one, is not the verdict stored beside that result, `<head12>-<n>.verifier.json`, or its sha256 is not that of the stored file, or the file is not a verdict of that result (`reviewverdict.Parse`, as `verify` holds it). What the verdict makes of the review is not decided here: `check` decides.

Only a completed `review-report` is held to this, and a completed `review-exemption` to the exemption (below). One that is `interrupted` stored nothing, and a record of any other kind is not about a review; both are stored as before. This compares the record with the stored files and computes no diff.

The record that closes an exemption has `payload_kind` `review-exemption` and a `payload_sha256` that is the sha256 of the stored exemption. Its evidence:

| kind | ref | sha256 |
|---|---|---|
| `review-diff` | `<base>..<head>` | `diff_sha256`, the sha256 of the canonical diff above |
| `review-exemption` | the stored exemption | its sha256 |

`labdrian roles append` holds a completed `review-exemption` record to the exemption `exempt` stored, in the repository of its working directory, before it stores the record (`odd/tasks/own-rdd.md`, T4, S5). It refuses the record (exit 1, nothing stored, and the message names the evidence kind and its ref) when:

- it lacks the `review-diff` or the `review-exemption` evidence, or carries a kind twice, so that none of the pieces is the evidence of its kind;
- the ref of the `review-exemption` is not exactly one `odd/review-results/<feature>/<head12>-<n>.exempt.json` (a result and a verdict are not exemptions, and neither is a path that leaves the directory or a temporary file), or the file is not stored, or its sha256 is not the one of the evidence, or the file is not an exemption;
- the stored exemption no longer holds to the rules it was made under (`review.CheckExemption`: its `binding` names a candidate, its `risk` is `none` and its `report` is clear), even when the record carries the sha256 of the file as it is now, so an exemption that was edited after it was made is refused for what it now says;
- the `review-diff` is not the range `<base>..<head>` and the `diff_sha256` of the binding stored in that exemption.

An `interrupted` `review-exemption` says the exemption was not made and names no file; it is stored as before.

Delivery recomputes the sha256 of the canonical diff of the range (`labdrian review bind` prints it, and `labdrian review check` recomputes it, below) and trusts the record only on equality: other bytes give another digest, and the record does not apply to them.

A review that did not run (a refused launch, an unparseable result, an echo that differs, an incomplete inspection) is recorded `interrupted` with a `resume_reason`, never as approved. `record` stores nothing for it. An exemption that was not made (a risk that is refused, a report that is missing or not clear) is recorded `interrupted` in the same way, and `exempt` stores nothing for it.

## The check before delivery

`labdrian review check --feature <name> [--head <rev>] [--base <rev>] [--cwd <repo>]` decides whether the head being delivered (`--head`, and `HEAD` when it is not given; an empty `--head` is a usage error) was reviewed, or exempted from review, and nothing that was stored keeps it from delivery (`odd/tasks/own-rdd.md`, T4, S5). With `--base` it decides it for every commit from that base to the head (below). It stores nothing. The delivery role runs it before it closes the feature (`roles/delivery/body.md`). Exit 0 cleared, 1 usage error, refusal or any other failure.

For each result of the feature that is about the head, in the order of its launches:

1. The result is the file `record` wrote, and its reply is held to its binding again with `reviewresult.Parse`, as `verify` holds it. A result is about a head by its binding and not by its name: a file named for the head (`<head12>`) whose binding is another commit is left out. A file named for the head that cannot be read, or that is not a result any more, refuses the check: what cannot be told is not cleared.
2. The candidate is recomputed from git, by the code `bind` and `record` use, for the `base` the result names and the head being delivered, and the stored binding must equal it: `base`, `head`, `diff_sha256` and `paths`. The digest in the file is never taken on its word.
3. The verdict stored beside the result, when there is one, is read and held to the result with `reviewverdict.Parse`, and `reviewverdict.Decide` makes the decision from the findings and the verdicts. A verdict that is stored and cannot be read is a refusal, never read as no verdict. Without a verdict the verifier did not run, and `Decide` reads every finding as a check that did not run: a result with only `WARNING` and `SUGGESTION` findings, or whose severe findings are `pre-existing` or `base-only`, is clear without one, and a result with a severe finding the diff may have caused (`introduced`, `behavior-activated`, `worsened` or `unknown`) is not clear until a verdict answers it. A check that did not run never passes.

For each exemption of the feature that is about the head, in the order of its numbers:

1. The exemption is the file `exempt` wrote (`review.DecodeExemption`), and it is held to the rules it was made under again (`review.CheckExemption`): its `binding` names a candidate before its base is given to git, its `risk` is still `none` and its `report` is still clear (`reviewverdict.ParseReport` and its clearness), so a file that was edited to say something else is refused. An exemption is about a head by its binding and not by its name, as a result is. A file named for the head that cannot be read, or that is not an exemption any more, refuses the check.
2. The candidate is recomputed from git for the `base` the exemption names and the head being delivered, and the stored binding must equal it, as a result's must. The report is never taken on its word either, and no verdict is looked for beside an exemption: the report is the whole of what the verifier said.
3. The `base` the exemption names is still the first parent git gives for the head (`review.CheckOneCommit`, after the binding is compared). An exemption of several commits or of a head with no parent, which `exempt` never makes, is refused, and its file is named in the error: a chain from the delivery base would step over the reviews of the commits in between.

The head is cleared when at least one result binds it and every one is clear, or when an exemption binds it and no result does. A result that blocks wins over any exemption, and so does a result that is not clear: an exemption is for work that takes no review, and is no way around a review that left a finding unchecked. Otherwise the check is refused: one line on stderr, `error: review check: head <head>: ...`, that names the result, its verdict and the findings, and nothing on stdout. An exemption that no longer holds refuses the head, even when another file would clear it, as a result that no longer holds does.

| the head has | the error says |
|---|---|
| no result and no exemption: it was neither reviewed nor exempted, or only another head or another feature was | `no stored result binds the head` |
| a result whose decision is `blocks` | `a stored result blocks the head` |
| a result that is neither `clear` nor `blocks`, with an exemption or not: its decision is `unverified` (a check did not run), or it has severe findings and no verdict was stored for it | `a stored result is not clear` |
| a result whose `base`, `head`, `diff_sha256` or `paths` are not what git gives now | `the stored binding differs from the candidate git gives` |
| an exemption whose `binding` differs from what git gives now | `the stored binding differs from the candidate git gives` |
| an exemption whose `risk` names an item | `a change that names a high-risk item is never exempt from review` |
| an exemption whose `report` is not clear | `the report is not clear` |
| an exemption whose `base` is not the first parent of its `head` | `an exemption is for one commit, so its base must be the first parent of its head` |
| a result or an exemption whose `base` is its `head`, which no verb makes | `base and head are the same commit` |

A result that blocks keeps the head blocked whatever was recorded after it: a verdict that reproduced a severe finding is not undone by another launch on the same head, and a blocked head goes back to the builder and comes back as another head. A result that is not clear keeps the head from delivery whichever launch it is, one before a clear result or one after it, and a head whose review could not be checked is reviewed again as another head.

A stored verdict that is edited in place (from `reproduced` to `not-reproduced`, say) clears a head it blocked, because `check` does not compare the stored files with the digests that the records of the role chain hold, and `roles append` checks them only when it appends a record.

On exit 0, stdout is one JSON object: `head` (the full name), `outcome` (`clear`), `results`, one for each result used (an empty array for a head that an exemption alone clears), with `result` (the stored path relative to the repository), `verdict` (the stored verdict, left out when the verifier did not run), `base`, `diff_sha256` and, when there are any, `fix` and `carry`: the ids of the findings to be fixed before delivery and to be carried as follow-ups, and `exemptions`, one for each exemption used, with `exemption` (the stored path relative to the repository), `base` and `diff_sha256` (the member is left out when there is no exemption, so that the output for a head with none is the one it always was).

Without `--base` the check is by head: the base of each result and of each exemption is the one it names. A candidate that was split into one launch per commit has a result for each head, and the check clears the head it is given by the results, or the exemption, of that head alone.

### The chain of a delivery

Reviews are per commit (`odd/tasks/own-rdd.md`, L18): each result binds the range of one launch, from the head of the last reviewed commit, and each exemption binds exactly one commit (its base is the first parent of its head, above), so the walk cannot step over a commit whose review blocked. With `--base <rev>`, the integration base of the delivery (`main`, a tracker branch or a dev branch, which the delivery role chooses to suit the project), the check clears the head only when clear results or exemptions cover `base..head` as an unbroken chain, each range starting where the one before ended. The base should be the commit the first reviewed range starts from, which is the merge base when the integration branch moved on after the branch was made. The walk goes back from the head:

1. The commit is held as a single head is held, by the same code: every result and every exemption that binds it is read from the files and recomputed from git, and `review.Cleared` decides. A commit with no result and no exemption (a gap), a result that blocks or is not clear, a binding that differs and a failure of git or of the store refuse the chain.
2. The base the range names is the next commit: the base of the first result that binds the commit, or of the first exemption when it has no result. The other results of that commit must be clear too, but they do not choose the way.
3. The walk is done when that base is `--base`. It is refused when no result or exemption ends at a commit (a range that begins before `--base` ends there as well, and so does a chain that begins after it), and when it comes back to a commit it already passed (`review.ErrChainLoops`): each step takes a commit not passed before, so it always ends.

A refusal is one line on stderr, `error: review check: the chain from <base> to <head>: ` and the refusal of the single-head check for the commit where the walk stopped (`head <commit>: review: no stored result binds the head`). On exit 0, stdout is one JSON object: `head` and `base` (full names), `outcome` (`clear`) and `ranges`, from the range that starts at `--base` to the one that ends at the head, each with `base`, `head`, `results` and `exemptions` as above (`results` is an empty array for a range that an exemption alone clears). Nothing is stored.

## What a result is not

A result here is check evidence in the role chain, never review authority. It carries no `schema`, lineage, receipt or `GENTLE_AI_*` member. `odd/review-receipts/` and the `gentle-ai review` lifecycle stay as they are: both can run on the same commit, because they share nothing but its bytes. An exemption is check evidence in the same way, and no review: it carries no `schema`, lineage, receipt or `GENTLE_AI_*` member either, and it does not stand in for the `gentle-ai review` lifecycle, which neither reads nor writes it.
