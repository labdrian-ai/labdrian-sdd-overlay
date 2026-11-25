# Classify

Run after `review record` (and `verify`) of a result, and for each result the stop hook names. `<id>` is the result's identifier, `<feature>/<head12>-<n>`. Claude Code only: the classifier and the evidence store are deployed there alone; on Codex, OpenCode or Pi, skip this step.

1. Run `labdrian review findings --feature <feature> --stored <head12>-<n>.json`. Exit 1: show its stderr and stop, blocked. An empty `kept`: nothing to classify, stop. Exit 0 with a non-empty `unverified`: a severe finding waits until its check has run; stop, not blocked, and classify once `review verify` has stored `reproduced` or `not-reproduced` for each check it needs. A stored `not-run` keeps it waiting: run the check again.
2. `<root>` is `git rev-parse --show-toplevel` in the reviewed worktree; `<script>` is `$HOME/.claude/skills/_shared/evidence/evidence.sh` and `<rules>` is `$HOME/.claude/skills/_shared/evidence/evidence-rules.md`, `$HOME` expanded; `<page>` is `<root>/docs/wiki/evidence.md`. When `<script>` is not a regular file, the evidence store is not deployed: stop, blocked.
3. Run `sh "<script>" reviewed <id> "<root>/docs/wiki"`. Exit 0: classified, stop. Exit 1: go on. Exit 2 (usage), 3 (`awk` missing) or 4 (cannot answer): show its stderr and stop, blocked. Any other exit: show its stderr and stop, blocked.
4. Run `sh "<script>" check "<root>/docs/wiki"`. Exit 0: go on. Exit 1: show the FAIL lines and stop, blocked, appending nothing. Exit 2: a usage error, blocked. Exit 3: `awk` is missing, blocked. Any other exit: blocked.
5. Run `sh "<script>" index "<root>/docs/wiki"`, its exits as in step 4; `<index>` is its stdout, or `empty` when it prints nothing.
6. Launch the classifier with the `kept` findings of step 1, `<id>`, `<page>`, `<rules>` and `<index>`, and no date: it writes today's.
7. Take the fence after `Kinds rows:` and the fence after `Evidence rows:` from its reply. When one is missing, or one of their lines is not a single table row (it starts with `| ` and ends with ` |`), append nothing and stop, blocked. An empty Evidence fence is no error: step 8 records the result as classified without evidence.
8. Copy `<page>` to a backup in the session scratchpad; when the page is missing, create it first as the empty page below. Append the Kinds rows after the last row of the Kinds table and the Evidence rows after the last row of the Evidence table; change no other line. When the Evidence fence is empty, append `| <id> | <date> |`, `<date>` today as `date +%F` prints it, after the last row of the table under `## Classified without evidence`; a page with no such heading gets, at its end, a blank line, `## Classified without evidence`, a blank line, `| Review | Date |`, `|--------|------|`, then the row.
9. Run `check` again. Exit 0: run `sh "<script>" reviewed <id> "<root>/docs/wiki"`. When it exits 0, run `sh "<script>" count "<root>/docs/wiki"` and report its lines for the kinds touched. Any other exit of that run: copy the backup back (remove the page when step 8 created it), say that no row carries `<id>`, and stop, blocked. Exit 1: copy the backup back (remove the page when step 8 created it), then step 10. Any other exit: copy the backup back the same way, show the output and stop, blocked.
10. When every FAIL line of that check starts with `FAIL evidence.md: ` and the launch was the first of this step for `<id>`, launch the classifier again with the same inputs and `<failed>`: a fence opened by `` ```text `` holding every line of that check's stdout that starts with `FAIL `, unchanged and in order, each `FAIL <file>: <problem>`; then do steps 7 to 9 once more with its reply. When a FAIL line names another file (the general rules, `FAIL evidence-rules.md: `), stop, blocked, showing it: rows appended to the page cannot fix it. When it was the relaunch, stop, blocked, showing the FAIL lines of both checks; the result stays unclassified, and the stop hook asks again.

Never delete or edit a row. A row may carry text the classifier read anywhere it can read, and it lands only in `<page>`, which the next commit of the work carries and its review sees. This step commits nothing.

The empty page:

```markdown
# Evidence

## Kinds

| Kind | Level | Rule | Applies when | Source | Scope |
|------|-------|------|--------------|--------|-------|

## Evidence

| Kind | Level | Review | Date | Finding |
|------|-------|--------|------|---------|
```
