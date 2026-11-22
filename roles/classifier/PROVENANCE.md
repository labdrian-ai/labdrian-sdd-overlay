# Provenance

The classifier role of this repository: `body.md`, adapted from the classify agent of playbook.

- Source repository: `andresfrei/playbook`
- Snapshot commit: `6de8566bf7f1150f49a75c1162c0f4cf87bf57c5` (tag `v3.6.0`)
- Source path: `plugins/playbook-agents/agents/classify.md`
- License: MIT, in `LICENSE` beside this file (the snapshot `LICENSE`, byte-identical).
- Status: the file is this repository's own code. No upstream is tracked.

## Changed since the snapshot

- The six lines of the core prefix of the role bodies come first, and the frontmatter is generated from the role table of `engine/agentgen`.
- `<script>` is the deployed path `$HOME/.claude/skills/_shared/evidence/evidence.sh`, run as `sh "<script>" ...`, where the snapshot named `${CLAUDE_PLUGIN_ROOT}/evidence.sh`. A `~` inside quotes would not expand.
- Step 2 reads the general kinds through the `index` and `rule <kind>` modes of the script instead of reading `evidence-rules.md`, so this repository keeps one parser.
- Step 3 takes the level of a general kind from `rule`, where the snapshot copied it at "the same level".
- New rule: when the script exits 3 (no `awk`), stop with status blocked.
- "You receive the findings the review kept" names the feed of the evidence store.
