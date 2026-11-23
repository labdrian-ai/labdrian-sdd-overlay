# Provenance

The evidence store of this repository: `evidence.sh` and `evidence-rules.md`.

- Source repository: `andresfrei/playbook`
- Snapshot commit: `6de8566bf7f1150f49a75c1162c0f4cf87bf57c5` (tag `v3.6.0`)
- Source paths: `plugins/playbook-agents/evidence.sh`, `plugins/playbook-agents/evidence-rules.md`
- License: MIT, in `LICENSE` beside this file (the snapshot `LICENSE`, byte-identical).
- Status: the files are this repository's own code. No upstream is tracked.

## Changed since the snapshot

- `evidence.sh`: with no `awk` on `PATH`, every mode but a usage error prints `evidence.sh: awk is not on PATH; <mode> cannot run` on stderr and exits 3; the snapshot printed nothing and exited 0 in `rules` and `index`, and `rule` reported the kind as missing (review `8e035ddb49dc-2`, F2).
- `evidence.sh`: a new mode `reviewed <identifier> [wiki directory]`, which prints nothing and exits 0 when the page passes every check of `check` and an Evidence row's Review cell is the identifier, 1 when none is, the page fails a check or there is no page, 2 with no identifier or one that holds a blank, 3 without `awk`, and 4, with one stderr line, when the page cannot be read or `awk` fails on it; the mode is written for the stop hook of S6 to learn whether a stored review result is classified (the snapshot has no such mode).
