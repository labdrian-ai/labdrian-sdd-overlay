---
name: branch-pr
description: "Branch names, commits and pull requests for the labdrian flow. Trigger: naming a branch, writing a commit, or opening or merging a PR."
license: MIT
metadata:
  author: labdrian
  version: "3.0"
---

## Activation Contract

Load this skill when you name a branch, write a commit message, or the owner asks for a push, a pull request or a merge.

## Hard Rules

- `git push`, `gh pr create` and `gh pr merge` run only on an owner message that asks for that step; one message asks for one step.
- A pull request needs delivery's closing entry: the last `## Log` entry of `odd/tasks/<f>.md`, written by the delivery role, whose `labdrian review check --feature <f> --onto <base>` exited 0 (`<f>` is the feature, `<base>` the branch the PR targets).
- A merge needs `gh pr checks` showing no failing or pending line.
- Branch name: `^(feat|fix|chore|docs|style|refactor|perf|test|build|ci|revert)\/[a-z0-9._-]+$`.
- Commit subject: `^(build|chore|ci|docs|feat|fix|perf|refactor|revert|style|test)(\([a-z0-9\._-]+\))?!?: .+`.
- Every commit ends with its `Risk:` trailer (`Risk: none` or the item numbers). No Co-Authored-By line, no AI attribution.
- Slice by about 400 changed lines; for a larger change load `chained-pr` and `work-unit-commits`.

## Decision Gates

| Condition | Action |
|---|---|
| Owner asks for a PR, no closing entry or `review check` not 0 | Stop; report what is missing. |
| Owner asks for a push and nothing else | Push only. |
| Owner asks for a merge, `gh pr checks` shows a failing or pending line | Stop; report the line. |
| Branch or subject fails its pattern | Rename or amend before any push. |

## Output Contract

Return the branch, the commit subjects with their `Risk:` trailers, and the one step done. For a PR the body names the feature document and its task ids, the `review check` result, and the checks run with their exact results.
