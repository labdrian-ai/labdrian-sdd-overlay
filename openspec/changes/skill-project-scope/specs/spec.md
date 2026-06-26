# Delta Spec: Project-Scoped Skill Install (issue #29, slice 2)

change: `skill-project-scope`
status: draft
created: 2026-06-26

---

## Scope Statement

This spec covers only what MUST be true after the change is applied. It describes
observable behaviour (inputs → outputs, error conditions, invariants); it does NOT
prescribe implementation structure — that is design's domain.

All existing slice-1 requirements (R-001 through R-032) remain in force. This file
adds the delta (R-040 onward, SC-10 onward).

---

## 1. Schema — `install` Block

### R-040 — `defaultScope` accepts `"global"` or `"project"`

WHEN `ParseRegistry` processes an entry whose `install.defaultScope` value is `"project"`,
THEN it MUST accept the entry without error; the returned `Entry.Install.DefaultScope`
MUST equal `"project"`.

WHEN `install.defaultScope` is any value other than `"global"` or `"project"`,
THEN `ParseRegistry` MUST return a non-nil error whose message includes the line number
and the invalid value.

> Precision: the existing line 571 guard `!= "global"` is the exact point where
> `"project"` must be admitted.

### R-041 — `install.allowedProjects` is an optional string sequence

WHEN an `install` block contains an `allowedProjects` key followed by a YAML block
sequence of plain-scalar strings,
THEN `ParseRegistry` MUST populate `Entry.Install.AllowedProjects` with those strings
in declaration order.

WHEN the `allowedProjects` key is absent from an `install` block,
THEN `Entry.Install.AllowedProjects` MUST be `nil` (not an empty slice — the zero
value for `[]string`). No error is produced.

WHEN the `allowedProjects` sequence is present but empty (no items),
THEN `Entry.Install.AllowedProjects` MUST be `nil` or an empty slice (both are
acceptable); no error is produced.

### R-042 — `allowedProjects` is forbidden on `global` entries

WHEN an entry has `install.defaultScope: global` AND `install.allowedProjects` is
present with at least one item,
THEN `ParseRegistry` MUST return a non-nil error. The message MUST include the entry
id and a clear reason (e.g., `"allowedProjects is only valid for project-scoped entries"`).

### R-043 — Slice-1 global cases remain valid (non-regression)

All test cases that passed under SC-01 through SC-05 and R-001 through R-032 MUST
continue to pass without modification after the parser change.

### R-044 — Parser never returns partial data on error (inherited)

`ParseRegistry` MUST return a zero-value `Registry{}` whenever it returns a non-nil
error. This is unchanged from R-011 and is restated here as a reminder for the new
code paths.

### R-045 — `targets` still required for all entries (non-regression)

Entries with `install.defaultScope: project` MUST still satisfy the existing R-007
constraint: `install.targets` MUST be non-empty and contain only valid values
(`"claude"`, `"opencode"`, `"codex"`). The `install` verb does not consult `targets`
when deciding what to copy, but the schema constraint is not relaxed.

---

## 2. Project Identity Resolution

### R-046 — Identity derived from git remote origin repo name

WHEN the `install` verb runs, it MUST resolve the current project identity from the
git remote named `origin` of the cwd repository. The identity string is the last
path segment of the remote URL with any `.git` suffix stripped.

Examples:
- `https://github.com/labdrian/labdrian-sdd-overlay.git` → `labdrian-sdd-overlay`
- `git@github.com:labdrian/my-project.git` → `my-project`
- `https://github.com/org/repo` (no `.git`) → `repo`

This same format is what registry authors MUST use in `allowedProjects` entries.

### R-047 — Fail-loud when no project identity is resolvable

WHEN cwd has no git remote named `origin` (or running `git remote get-url origin`
fails for any reason),
THEN `engine skills install` MUST exit non-zero and write an error to stderr that
includes the cwd path and the reason identity resolution failed. No files are written.

---

## 3. `skills-project-install` Behaviour

### R-048 — Install set selection

WHEN `engine skills install` is invoked from `<cwd>`,
THEN the engine MUST compute the install set as all `Entry` values where:
  1. `Entry.Install.DefaultScope == "project"`, AND
  2. the resolved project identity is present in `Entry.Install.AllowedProjects`
     (case-sensitive string match).

Entries that do not satisfy BOTH conditions are silently excluded from the install
set. In particular:
- `global`-scoped entries are always excluded.
- `project`-scoped entries whose `allowedProjects` does not contain the current
  project identity are excluded.
- `project`-scoped entries with a nil or empty `allowedProjects` are excluded.

### R-049 — Copy: source and destination paths

For each entry `e` in the install set:

- Source: `<overlay_root>/skills/<e.ID>/` (entire directory tree)
- Destination: `<cwd>/.claude/skills/<e.ID>/`

The overlay root is the directory from which the engine was invoked (design decides
exact resolution; spec asserts that the source path starts from the directory
containing the registry file, i.e., the overlay repo root).

### R-050 — Idempotent overwrite on re-install

WHEN the destination `<cwd>/.claude/skills/<e.ID>/` already exists,
THEN the engine MUST overwrite all regular files from source without error. No merge
is performed; the destination ends up identical to the source after the operation.

Files not present in the source but already in the destination directory: the engine
MAY leave them in place in this slice (uninstall is deferred to a future slice).
Spec does not require their removal.

### R-051 — Output: one `installed:` line per skill

WHEN the install set is non-empty and all copies succeed,
THEN the engine MUST print exactly one line per installed skill to stdout in the form:

```
installed: <id>
```

where `<id>` is `Entry.ID`. Order: the engine MUST process and report entries in the
order they appear in the registry (declaration order from `Registry.Skills`).

### R-052 — Empty install set → exit 0 with notice

WHEN the install set is empty (no project-scoped skills are admitted for the current
project),
THEN `engine skills install` MUST exit 0 and print a human-readable notice to stdout
that includes the resolved project identity (e.g.,
`no project-scoped skills admitted for project "my-project"`).

### R-053 — Fail-loud: source skill directory missing

WHEN an entry is in the install set but `<overlay_root>/skills/<e.ID>/` does not
exist as a directory,
THEN the engine MUST exit non-zero and write to stderr a message that includes `e.ID`
and the full expected source path. No partial install is performed for entries
processed after the first failure; the engine SHOULD report all missing source dirs
before exiting (full-scan fail-loud, same pattern as `Diff`).

### R-054 — Fail-loud: write failure during copy

WHEN any file write to `<cwd>/.claude/skills/<e.ID>/` fails (permissions, disk full,
or any other I/O error),
THEN the engine MUST exit non-zero and write to stderr a message including the
destination path and the OS error. The partially-written directory is left in place
(no rollback in this slice).

### R-055 — Install writes ONLY under `<cwd>/.claude/skills/`

The engine MUST NOT write any file outside `<cwd>/.claude/skills/` during an install
run. This is an absolute invariant; violation is a critical bug.

---

## 4. CLI Dispatch

### R-056 — `SkillsCore` dispatches `"install"` verb

WHEN `SkillsCore` is called with `verb == "install"`,
THEN it MUST call the install core function (design names it, spec refers to it as
`RunInstallCore`) and MUST NOT fall through to the default error branch.

### R-057 — Unknown-verb error message lists `"install"`

WHEN `SkillsCore` is called with an unknown verb,
THEN the error message written to stderr MUST include `"install"` among the listed
supported verbs (e.g., `"list, status, validate, install"`).

### R-058 — `bin/labdrian-overlay cmd_skills` forwards `install`

WHEN `labdrian skills install [args...]` is invoked,
THEN `cmd_skills` in `bin/labdrian-overlay` MUST forward the `install` verb and all
remaining arguments to `engine skills install`. No other branch of `cmd_skills` is
modified.

---

## 5. Red Lines (Non-Regression Invariants)

### R-059 — `cmd_apply` and `cmd_capture` byte-identical

The `cmd_apply` and `cmd_capture` functions in `bin/labdrian-overlay` MUST remain
byte-identical to their pre-change state. No modifications, not even whitespace.

### R-060 — Vendored `bin/overlay` byte-identical

The vendored `bin/overlay` file MUST remain byte-identical to its pre-change state.

### R-061 — Registry fixture: at least one `project` entry

`skills.registry.yaml` MUST contain at least one entry with `install.defaultScope:
project` and at least one item in `install.allowedProjects`, so the install code path
is exercised by the existing integration smoke test.

---

## 6. Acceptance Scenarios

All scenarios are designed to map directly to `go test` table-driven cases in the
`engine/skills` package. Filesystem scenarios use `t.TempDir()`.

---

### SC-10 — Parser accepts `project` scope with `allowedProjects`

```
Given: a registry YAML with one entry:
  id: my-skill, install.defaultScope: project,
  install.allowedProjects: [labdrian-sdd-overlay, other-repo]
  (all other required fields present and valid)
When:  ParseRegistry(r) is called
Then:
  - err == nil
  - entry.Install.DefaultScope == "project"
  - entry.Install.AllowedProjects == ["labdrian-sdd-overlay", "other-repo"]
```

### SC-11 — Parser rejects unknown `defaultScope` value

```
Given: a registry YAML with install.defaultScope: workspace
When:  ParseRegistry(r) is called
Then:
  - err != nil
  - error message contains the line number of the defaultScope line
  - error message contains "workspace" or the invalid value
```

### SC-12 — `allowedProjects` forbidden on `global` entry

```
Given: a registry YAML with defaultScope: global AND allowedProjects: [some-repo]
When:  ParseRegistry(r) is called
Then:
  - err != nil
  - error message references the entry id
  - error message indicates allowedProjects is invalid for global scope
```

### SC-13 — `allowedProjects` absent on `project` entry → valid

```
Given: a registry YAML with defaultScope: project and no allowedProjects key
When:  ParseRegistry(r) is called
Then:
  - err == nil
  - entry.Install.AllowedProjects == nil
```

### SC-14 — All slice-1 global golden cases pass (non-regression)

```
Given: the existing test fixtures valid_core_and_custom.yaml (SC-01) and
       all R-001..R-012 error-case fixtures
When:  TestParseRegistry runs
Then:
  - every previously-passing case still passes
  - no new failures introduced
```

### SC-15 — Install copies project skill into `<cwd>/.claude/skills/`

```
Given:
  overlayDir := t.TempDir()
  os.MkdirAll(overlayDir+"/skills/my-skill", 0755)
  os.WriteFile(overlayDir+"/skills/my-skill/SKILL.md", []byte("# My Skill"), 0644)
  registry has my-skill: defaultScope=project, allowedProjects=[target-repo]
  cwdDir := t.TempDir() with a fake git remote origin → "https://example.com/org/target-repo.git"
When:  RunInstallCore runs with overlayDir + cwdDir
Then:
  - exit code 0
  - file cwdDir+"/.claude/skills/my-skill/SKILL.md" exists
  - its content equals "# My Skill"
  - stdout contains "installed: my-skill"
```

### SC-16 — Install is idempotent (re-run overwrites)

```
Given:  SC-15 precondition with SKILL.md already present in cwdDir
        (first install has already run)
When:   RunInstallCore runs a second time
Then:
  - exit code 0
  - SKILL.md content matches the overlay source (overwrite succeeded)
  - no error produced
```

### SC-17 — Install skips skills not in `allowedProjects`

```
Given:
  registry has:
    skill-A: defaultScope=project, allowedProjects=[other-repo]
    skill-B: defaultScope=project, allowedProjects=[target-repo]
  current project identity resolves to "target-repo"
When:  RunInstallCore runs
Then:
  - cwdDir+"/.claude/skills/skill-B/..." exists
  - cwdDir+"/.claude/skills/skill-A/" does NOT exist
```

### SC-18 — Install ignores global-scoped skills

```
Given:
  registry has:
    global-skill: defaultScope=global
    project-skill: defaultScope=project, allowedProjects=[target-repo]
  current project identity resolves to "target-repo"
When:  RunInstallCore runs
Then:
  - cwdDir+"/.claude/skills/project-skill/..." exists
  - cwdDir+"/.claude/skills/global-skill/" does NOT exist
```

### SC-19 — Fail-loud on missing source directory

```
Given:
  registry has my-skill: defaultScope=project, allowedProjects=[target-repo]
  overlayDir+"/skills/my-skill/" does NOT exist
When:  RunInstallCore runs
Then:
  - exit code != 0
  - stderr contains "my-skill"
  - stderr contains the expected source path
```

### SC-20 — Fail-loud when project identity cannot be resolved

```
Given:
  cwdDir := t.TempDir() (no git repo, no remote)
When:  RunInstallCore runs
Then:
  - exit code != 0
  - stderr contains a message about failing to resolve project identity
  - no files written under cwdDir+"/.claude/"
```

### SC-21 — Install writes only under `<cwd>/.claude/skills/`

```
Given: a valid install invocation (SC-15 preconditions)
When:  RunInstallCore runs and succeeds
Then:
  - a recursive walk of overlayDir and any path outside cwdDir+"/.claude/skills/"
    shows no newly-created files attributable to this run
  (implementation note: use a synthetic writeSpy or verify by walking cwdDir)
```

### SC-22 — Empty install set → exit 0 with notice

```
Given:
  registry has no project-scoped skills whose allowedProjects includes "target-repo"
  current project identity resolves to "target-repo"
When:  RunInstallCore runs
Then:
  - exit code 0
  - stdout contains "target-repo" and a notice (e.g., "no project-scoped skills")
  - cwdDir+"/.claude/skills/" is not created or is empty
```

### SC-23 — `SkillsCore` dispatches `install` verb without error

```
Given: SkillsCore called with verb="install" and valid args
When:  dispatch runs
Then:
  - RunInstallCore is called (not the default error branch)
  - no "unknown verb" error appears in stderr
```

### SC-24 — Unknown-verb error message includes `"install"`

```
Given: SkillsCore called with verb="frobnicate"
When:  dispatch runs
Then:
  - stderr contains "install" among the listed supported verbs
  - exit code != 0
```

---

## 7. Spec Assumptions (forced by proposal gaps)

The following were not fully resolved by the proposal and are treated as working
assumptions in this spec. Design MUST confirm or correct them:

| # | Assumption | Impact if wrong |
|---|------------|-----------------|
| A-1 | Project identity = last path segment of `origin` remote URL, `.git` stripped | SC-15 / SC-20 fixture setup |
| A-2 | `allowedProjects` absent on a `project` entry is valid (not an error); entry is simply not installable anywhere | R-041, SC-13 |
| A-3 | `allowedProjects` forbidden on `global` entries (error, not warn) | R-042, SC-12 |
| A-4 | Overlay root for install is the directory containing `skills.registry.yaml`, resolved by the engine at startup | R-049 |
| A-5 | Re-install leaves extra destination files in place (no removal); uninstall is deferred | R-050 |
| A-6 | `targets` field is still required for `project`-scoped entries (R-045 / R-007 non-regression) | parse_test golden cases |
| A-7 | Install processes all failing source-dir misses before exiting (full-scan fail-loud) | R-053 |

---

## 8. Out of Scope (explicit non-goals for this spec)

- opencode / codex project-local install paths
- `skills add/remove`, external sources, pinned-ref
- Any change to `cmd_apply` or `cmd_capture`
- Uninstall / cleanup of project-installed skills
- Conflict detection for user-edited files in destination (overwrite is unconditional)
- Multi-remote or non-`origin` remote resolution
