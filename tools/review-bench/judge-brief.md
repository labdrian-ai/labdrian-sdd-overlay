No finding ids. Labeling task: read the claims below about one code change and label each one against the code. There is one check to report: that you wrote the labels file.

Code (read-only, outside any repository):
- `{{CASE_DIR}}/head/` is the code after the change, `{{CASE_DIR}}/base/` the code before it, and `{{CASE_DIR}}/diff.patch` the change itself.
- Read only under `{{CASE_DIR}}`. There is no git history there; do not look for one.

Claims: the file `{{CASE_DIR}}/claims.json`, a JSON array in which each claim has `key`, `location` (a path and lines in the change), `claim` (what fails) and `proof_refs` (where its author looked). Label every claim in it, in its order.

For each claim, read the code at `head/` (and `base/` or `diff.patch` when the claim needs them) and decide:
- `real`: the failure the claim states happens at head as stated.
- `false`: it does not happen, or the code shows the claim is wrong.
- `unsure`: reading the code cannot decide.

Also set `failure_stated`: `false` when the claim names no failure (for example a remark on test coverage or style), `true` otherwise. Judge only whether the claimed failure holds, not how serious it is.

Give each label one code citation, `head/<path>:<line>` or `head/<path>:<start>-<end>` (or `base/...`), naming a file that exists under `{{CASE_DIR}}`, and a one-sentence reason. A `real` or `false` label always has a citation.

Write exactly one file, `{{OUT}}`, and nothing else anywhere:
`{"labels":[{"key":"...","label":"real|false|unsure","failure_stated":true,"citation":"head/...","reason":"..."}]}`
with one entry per key of `claims.json`, none twice and none missing. Run no command that changes any other file.

Your reply is the exemption report of one check: the command that wrote `{{OUT}}` and how many labels it holds.
