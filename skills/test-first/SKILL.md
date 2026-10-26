---
name: test-first
description: "Trigger: implementing a task, RED GREEN REFACTOR, test first, TDD evidence. No code before a failing test; the evidence table the builder reports."
license: MIT
metadata:
  author: labdrian
  version: "1.0"
---

# Test first

## Activation Contract

Load when you implement a task. The prompt names the test runner; without one, take the last row of the table below.

## Hard Rules

- Before you change an existing file, run its tests; that run is the safety net.
- No code before a failing test. RED is a run you saw fail for the reason the `S#` gives; a compile error on the symbol you are about to write counts, a typo does not.
- Write no more test than it takes to fail, and no more code than it takes to pass.
- One RED test per rule of the `S#`: through the public interface, asserting every observable effect of the rule (output, exit code, stored data), covering the cases the rule names.
- For each existing command, option or behavior your change touches, one test that its previous behavior still holds, passing before you change anything. Add no other cases.
- GREEN: fix the code, never the test. Run only the relevant tests during the cycle and the whole suite once, at the end of the task.
- REFACTOR: run the tests after every step and revert the step that turns them red.
- Every assertion calls production code, checks a specific expected value, and would fail if the code were wrong: no tautology, no empty-result check without a setup that makes empty the right answer, no loop over a collection you did not show to be non-empty, no assertion on style or internals.
- Tests never touch real state: temp directories and a fake HOME only. Scratch goes in a `mktemp -d` directory outside the worktree; report its path and leave it for the parent to remove.
- Never invent RED or GREEN evidence, or a runner.

## Decision Gates

| Situation | Do |
|---|---|
| New or changed behavior | RED, GREEN, REFACTOR |
| Refactor, behavior must not change | Pin the current behavior with a passing test first; the same test passes after |
| Tests fail before you touch anything | Stop; report them as pre-existing, except the ones the prompt lists as known; fix none |
| Runner fails for an infrastructure reason | Report the task `blocked` with the output; go on to the next task |
| Documentation only, no runner, or no meaningful RED | Name the exception; run the structural or functional check that fits; write `n/a` in the RED cell |

## Output Contract

Put this table in the Log entry of the task and in the summary of your envelope:

| Task | Test | Safety net | RED | GREEN | REFACTOR |
|---|---|---|---|---|---|

Each cell is the command you ran and its exact result, or `not run`. A task finished without a failing test first is FAILED, unless the last row applies.
