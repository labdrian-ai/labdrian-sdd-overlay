---
name: odd-feature-doc
description: "Trigger: odd/tasks/<feature>.md, feature document, Specs, Tasks, Log. Format of the ODD feature document and the part each role plays in it."
license: MIT
metadata:
  author: labdrian
  version: "1.0"
---

# ODD feature document

## Activation Contract

Load when the prompt locates `odd/tasks/<feature>.md`. The shaper writes it, the builder works from it, delivery closes it.

## Hard Rules

- Read the document up to `## Log` before acting, and report which `S#` you covered.
- Four parts, in this order and no other: a header, `## Specs`, `## Tasks`, `## Log`.
- Header, two or three lines: the objective, the branch, the delivery strategy, the test runner.
- Each spec is `S<n>`: its scope, its acceptance checks, and the user's strings, messages and examples quoted verbatim, in the user's language. Never summarize a quote, never add a requirement the user did not ask for, and mark an assumption as an assumption.
- One line per task: `- [ ] T1 — S1-S2 — inline — what it delivers — commit: -`. The box is the status, the route is `inline` or `delegated`, and the commit is `-` until the task is committed.
- The Log is append-only. `L1` is the user's request, verbatim; each later entry is `- **L<n>** (<date>, <who>): ...` and holds a user correction verbatim, a decision or evidence. Never edit, reorder or delete an entry.
- A requirement change: append the user's words to the Log, rewrite only the affected `S#`, reopen only its task.
- Check a task off only after you observed its outcome and its checks.

## Decision Gates

Write only the part of your role.

| Role | Writes | Never |
|---|---|---|
| shaper | the header, `## Specs`, `## Tasks` with every box open, and `L1`, the request exactly as the prompt quotes it | check a box, write code |
| builder | `[x]` and the commit on each finished task, and one Log entry: the `S#` covered, the commands run with their exact results, the `test-first` table, anything found that the task did not ask for | edit a spec, a task it was not given, or an earlier entry |
| delivery | one final Log entry with the state at close: the tasks `[x]` and not, each with its commit; unfinished tasks and missing artifacts named as unfinished | copy, move or rewrite the document |

## Output Contract

Return the path and the `S#` and task IDs you changed.
