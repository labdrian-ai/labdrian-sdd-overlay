# Provenance

The evidence store of this repository: `evidence.sh` and `evidence-rules.md`.

- Source repository: `andresfrei/playbook`
- Snapshot commit: `6de8566bf7f1150f49a75c1162c0f4cf87bf57c5` (tag `v3.6.0`)
- Source paths: `plugins/playbook-agents/evidence.sh`, `plugins/playbook-agents/evidence-rules.md`
- License: MIT, in `LICENSE` beside this file (the snapshot `LICENSE`, byte-identical).
- Status: the files are this repository's own code. No upstream is tracked.

## Changed since the snapshot

- `evidence.sh`: with no `awk` on `PATH`, every mode but a usage error prints `evidence.sh: awk is not on PATH; <mode> cannot run` on stderr and exits 3; the snapshot printed nothing and exited 0 in `rules` and `index`, and `rule` reported the kind as missing (review `8e035ddb49dc-2`, F2).
- `evidence.sh`: a new mode `reviewed <identifier> [wiki directory]`, which prints nothing and exits 0 when the page passes every check of `check` and an Evidence row's Review cell is the identifier, 1 when none is, the page fails a check or nothing is at the page's path under a directory that can be searched, 2 with no identifier or one that holds a blank, 3 without `awk`, and 4, with one stderr line, when something other than a readable regular file is at the path, a directory on the way cannot be searched, or `awk` fails or gives no answer (its own stderr is dropped); the mode is written for a stop hook that asks whether a stored review result is classified (the snapshot has no such mode).
- `evidence.sh`: a new mode `reviewed-each <wiki directory> <identifier>...`, which reads the page once and prints one line for each identifier, in the order given, `yes` where `reviewed` would exit 0 and `no` where it would exit 1, then exits 0; it exits 4, 3 and 2 where `reviewed` does, with the same stderr lines, naming `reviewed-each` as the mode they name, and nothing on stdout; the identifiers reach `awk` in one environment string, so past the system's limit for one string (128 KiB on Linux) `awk` cannot start and the mode exits 4; and a missing or empty wiki directory, no identifier or a blank identifier is a usage error; it is written for a stop hook that asks about many stored review results at once (the snapshot has no such mode).
- `evidence.sh`: `rule` and `reviewed` compare the kind or identifier they are given with a cell as text, never as a number, whatever the `awk`, so a cell `01` is not `1`.
- `evidence.sh`: `cd` takes `--` like `dirname` and `basename`, so a script path that starts with `-` is read as a path.
