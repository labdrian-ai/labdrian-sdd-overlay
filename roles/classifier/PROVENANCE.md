# Provenance

The classifier role of this repository: `body.md`, adapted from the classify agent of playbook.

- Source repository: `andresfrei/playbook`
- Snapshot commit: `6de8566bf7f1150f49a75c1162c0f4cf87bf57c5` (tag `v3.6.0`)
- Source path: `plugins/playbook-agents/agents/classify.md`
- License: MIT, in `LICENSE` beside this file (the snapshot `LICENSE`, byte-identical).
- Status: the file is this repository's own code. No upstream is tracked.

## Changed since the snapshot

- The six lines of the core prefix of the role bodies come first, and the frontmatter is generated from the role table of `engine/agentgen`.
- The tools are `Read, Grep, Glob`, where the snapshot had `Read, Grep, Glob, Bash, Edit, Write`. The agent returns its rows and the parent appends them and runs the check, where the snapshot's agent wrote the page and ran the script.
- The general kinds come from the `<index>` the prompt gives and from the rules file read by the path `<rules>` the prompt gives, instead of `${CLAUDE_PLUGIN_ROOT}/evidence-rules.md`. The script path and the exit-3 rule are gone from the body.
- Step 3 takes the level of a general kind from its row in `<rules>`, where the snapshot copied it at "the same level".
- The output is the two fenced row lists, `Kinds rows:` and `Evidence rows:`, then the skipped findings.
- "The prompt gives the findings the review kept" names the feed of the evidence store.
- When the prompt gives `<failed>`, the FAIL lines of the parent's check, the agent returns its rows again, corrected, where the snapshot's agent ran the check and fixed the rows itself.
- The generated agent file carries the MIT notice of `LICENSE` as comment lines of its frontmatter.
