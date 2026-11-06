# Design — skill-lifecycle (issue #29, slice 3)

Make `skills.registry.yaml` MUTABLE through `labdrian skills add <id>` / `remove <id>`.
Slices 1–2 built a strict-subset YAML **parser** and a registry↔manifest **validator** but no
writer — the registry has been read-only. This slice adds the missing inverse (a serializer) plus
crash-safe add/remove that keep `validate` green.

## Quick path (what gets built)

1. `engine/skills/serialize.go` — pure `Serialize(Registry) ([]byte, error)`, the exact inverse of `ParseRegistry`.
2. `engine/skills/lifecycle.go` — pure `AddEntry` / `RemoveEntry`, pure manifest-row ops, an atomic writer, and thin `RenderAddCore` / `RenderRemoveCore`.
3. `engine/skills/skills.go` — dispatch `add` / `remove` verbs; update usage strings.
4. `bin/labdrian-overlay` `cmd_skills` — already injects `--registry` / `--manifest` / `--source-root`; no change needed (verified).
5. Tests — round-trip, golden, manifest-row, atomic-write, core wiring.

`engine/cmd/main.go`, `bin/overlay`, `cmd_apply`, `cmd_capture` stay UNCHANGED (`main.go` already
forwards every verb through `SkillsCore`; `bin/overlay` is the inert upstream vendor copy).

## Architecture approach

Hexagonal split, same as slices 1–2: **pure model transforms** in the core, **I/O at the edges**,
**CLI cores** that wire flags to functions and inject all side effects. This keeps the high-risk
serializer and transforms fully table-testable, and confines filesystem and crash concerns to one
small atomic-writer seam.

```
cmd_skills (bash, injects paths)
   -> SkillsCore("add"/"remove", args)         skills.go  (dispatch)
        -> RenderAddCore / RenderRemoveCore     lifecycle.go (thin, parses flags + orchestrates)
             ParseRegistry  (read)              parse.go    (existing)
             AddEntry / RemoveEntry  (pure)     lifecycle.go
             Serialize  (pure)                  serialize.go
             addManifestRow / removeManifestRow (pure)  lifecycle.go
             Diff (cross-check, pure)           validate.go (existing)
             writeFileAtomic (I/O, temp+rename) lifecycle.go
```

Architecture rule honoured (`architecture/overlay-gentle-ai-separation`): every new file lives under
`engine/skills/` (100% labdrian, not on `upstream`), so a gentle-ai update can never conflict. No
upstream-owned file is touched. `cmd_skills` is in `bin/labdrian-overlay` (custom router), not
`bin/overlay`.

---

## ADR-5 — Manifest stays in sync (FORK 1 resolved: keep-in-sync, SKILL.md row only)

**Decision.** `add`/`remove` update `overlay.manifest` so `validate` stays green with no manual step.
They manage **exactly one row** per skill: `<id>/SKILL.md <tag>`.

- `add <id>` → append `"<id>/SKILL.md custom"` (add always infers `source.type=custom`, and
  `custom ↔ custom` per `tagMatchesSourceType`, so the tag is always `custom`).
- `remove <id>` → delete the line whose first field equals `<id>/SKILL.md`.

**Why only the SKILL.md row.** `LoadManifestView` derives a skill directory **only** from a
`*/SKILL.md` row; `references/`, `evals/`, `assets/` rows are skipped for discovery and are invisible
to `Diff`. Therefore the SKILL.md row is the *exact and complete* surface that keeps `validate`
aligned. Managing it is necessary and sufficient.

**Rows we never touch (inert / infra / capture-owned):**

| Row class | Example | Why untouched |
|---|---|---|
| engine tracking | `engine/skills/parse.go managed` | Inert metadata (gentle-ai manifest doesn't even list engine). |
| `_shared/*` | `_shared/pre-sdd-contracts.md custom` | Infra prefix, excluded from `ManifestView`. |
| registry self-row | `skills.registry.yaml custom` | Inert tracking of the registry file itself. |
| reference/eval/asset | `<id>/references/x.md custom` | Track real on-disk files; invisible to `Diff`; owned by `overlay capture`. |

**Consequence / documented boundary.** On `remove`, the on-disk `skills/<id>/` dir is NOT deleted
(non-goal), and its non-SKILL.md tracking rows remain — they still describe files that still exist,
so they are not wrong and `validate` does not flag them. Full file-level (de)registration remains
`overlay capture`'s job. This is symmetric: `add` writes one row, `remove` deletes one row.

**Rejected alternative — registry-only (let `validate` report divergence).** Cheaper code, but every
`add`/`remove` would immediately make `validate` fail until a manual manifest edit — exactly the
hand-editing hazard this slice exists to remove. Rejected.

**Rejected alternative — enumerate and write every file under `skills/<id>/`.** Crosses into
capture's domain, is fragile (file sets change), and is content-adjacent (non-goal). Rejected.

---

## ADR-6 — Serializer strategy: full re-emit (FORK 2 resolved)

**Decision.** `Serialize` does a **full re-emit from the parsed model** — it ignores the prior file
bytes entirely and renders canonical subset YAML from `Registry`.

**Why.** The registry is a machine-managed artifact whose only reader is our own strict parser.
Full re-emit is the *inverse function* of `ParseRegistry`, which is exactly what makes the round-trip
invariant `parse(serialize(r)) == r` provable and table-testable. Surgical editing would have to
re-implement the parser's tokenizer to locate insertion points, re-introducing the very grammar
coupling we are trying to test away, and would risk silent drift on entries it didn't touch.

**Asymmetry note (intentional).** The **registry** is full re-emit (machine-owned, single strict
reader). The **manifest** is a line-set, partly maintained by `capture`/humans, so its edits are
**surgical line ops** (append-if-absent / delete-matching-line) — see ADR-5. Different ownership →
different write strategy. This is deliberate, not an inconsistency.

**Rejected alternative — surgical registry edit.** Re-derives parser internals, drift-prone, harder
to prove round-trip. Rejected.

---

## ADR-7 — Serializer grammar (the exact emission rules)

The invariant: for any `Registry` that `ParseRegistry` could have produced (i.e. already
`validateEntry`-clean and slice-normalized), `ParseRegistry(Serialize(r)) ` deep-equals `r`.

### Field order (deterministic)

- Document: `version`, then `skills:`.
- Entry: `id`, `path`, `source`, `install`, `lifecycle`.
- `source`: `type`, then `upstream` (when present).
- `upstream`: `owner`.
- `install`: `defaultScope`, `targets`, then `allowedProjects` (when present).
- `lifecycle`: `updateStrategy`.

Order is irrelevant to model equality (the parser accepts any order) but fixed for deterministic
golden output. Entries are emitted in `reg.Skills` slice order; sequence items in slice order.

### Indentation (2 spaces per level; the dash sits at the parent indent)

```
version: "1"
skills:
  - id: <id>
    path: <path>
    source:
      type: <type>
      upstream:
        owner: <owner>
    install:
      defaultScope: <scope>
      targets:
        - <target>
      allowedProjects:
        - <project>
    lifecycle:
      updateStrategy: <strategy>
```

This mirrors the parser exactly: entry keys at indent 4 (`seqIndent 2 + 2`), `source`/`install`/
`lifecycle` children at 6, `upstream`/`owner` at 8, sequence scalars (`targets`/`allowedProjects`) at 8.

### Quoting (minimal, to keep diffs clean and human-readable)

A scalar is emitted **bare** unless `needsQuote(v)` is true, in which case it is wrapped in double
quotes. `needsQuote(v)` returns true when bare emission would not round-trip or would be misparsed:

- `v == ""` (empty).
- `v != strings.TrimSpace(v)` (leading/trailing whitespace — the parser trims).
- first byte is a YAML indicator that changes meaning: one of `" ' | > # - ? : ,` or a space.
- `v` contains `": "`, or ends with `:` (would be read as a key boundary).
- `v` contains `" # "` (the parser's inline-comment trigger when unbare).

**Special case:** the `version` value is **always** double-quoted (`version: "1"`) to preserve the
documented form and avoid any external reader treating it as an integer. (Bare `1` would still parse
to `"1"` here, but quoting matches the canonical file.)

Double-quoting is naive-but-consistent with `unquoteScalar` (strip outer matching pair, no escape
processing), so any `needsQuote` value round-trips through the parser's unquote logic.

### Representability guard (fail-loud — ADR-2 fail-loud lineage)

The tokenizer rejects `{ } [ ] & * !` and tab **anywhere on a line, even inside quotes**. Such a
value can therefore never round-trip. `Serialize` calls `representable(v)` on every scalar and
returns an error (no partial bytes) if any of those characters or a tab is present. In practice
`AddEntry`'s slug guard (ADR-8) prevents this for new ids; the check is defense-in-depth for any
hand-edited entry already in the file.

### Omission rules (so nil round-trips to nil)

| Field | Emit when | Omit when |
|---|---|---|
| `path` | non-empty | `""` (parser yields `""`) |
| `source.upstream` block | `Source.Upstream != nil` | nil (custom skills) |
| `install.allowedProjects` block | `len > 0` | empty/nil (global skills) |

`source`, `install`, `lifecycle` are never empty (each has a required child), so their headers are
always emitted. The parser produces **nil** (not empty) slices for absent sequences; `Serialize`
omits empty slices, so `parse(serialize(x))` yields nil → matches. `AddEntry` MUST set
`AllowedProjects: nil` (never `[]string{}`) so the synthetic entry already matches its round-trip image.

### Round-trip invariant test seam

- `serialize_test.go` — `TestSerializeRoundTrip`: table of synthetic registries (core+upstream,
  custom, multi-target, single-target, project-scoped+allowedProjects, path==""), assert
  `reflect.DeepEqual(r, mustParse(Serialize(r)))`.
- `TestSerializeRealFileRoundTrip` — load `skills.registry.yaml`, serialize, re-parse, DeepEqual.
  This breaks CI if parser grammar and serializer ever drift apart.
- `serialize_golden_test.go` — golden bytes of `Serialize(realRegistry)` behind `-update`; assert the
  golden itself re-parses clean.
- `TestSerializeRejectsUnrepresentable` — ids/values containing `[ ] { } & * !`/tab → error, no bytes.

---

## ADR-8 — add/remove logic, defaults, preconditions

### `AddEntry(reg, id) (Registry, error)` — pure

Inferred defaults (per proposal):

| Field | Value |
|---|---|
| `Path` | `id` |
| `Source.Type` | `custom` (→ `Upstream = nil`) |
| `Install.DefaultScope` | `global` |
| `Install.Targets` | `["claude","opencode","codex"]` |
| `Install.AllowedProjects` | `nil` |
| `Lifecycle.UpdateStrategy` | `overlay-only` |

Preconditions (fail-loud, return error, registry unchanged):

- **Slug guard:** `id` matches `^[a-z0-9][a-z0-9-]*$`. Rejects empty, path separators, `..`, and any
  forbidden/unrepresentable char before it can reach the serializer or a filesystem stat.
- **Uniqueness:** `id` not already in `reg.Skills` → else `"skill \"<id>\" already registered"`.

Returns a copied registry with the new entry appended (declaration order preserved). The built entry
passes `validateEntry` by construction.

### `RemoveEntry(reg, id) (Registry, error)` — pure

- **Presence:** `id` must exist → else `"skill \"<id>\" not found"`.
- Returns a copied registry with that entry filtered out; order of the rest preserved.
- Does NOT stat or delete `skills/<id>/` (non-goal).

### I/O-side preconditions (in the cores, not the pure transforms)

- `add` only: `<source-root>/<id>/SKILL.md` must exist (stat) → else fail-loud. The dir must already
  hold a real skill; we never scaffold content (non-goal).
- `remove`: no disk stat (the dir intentionally survives).

### Path injection (reuse existing wiring)

`cmd_skills` already appends `--registry $OVERLAY_DIR/skills.registry.yaml`,
`--manifest $OVERLAY_DIR/overlay.manifest`, `--source-root $OVERLAY_DIR/skills` for **every** verb
(verified at `bin/labdrian-overlay:1020-1027`). So:

- `add` reads `--registry`, `--manifest`, `--source-root` (all three injected — free).
- `remove` reads `--registry`, `--manifest` (source-root unused, harmless if present).
- `--project-id` / install target-root are NOT used here (not an install). No bash change required.

The positional `<id>` is the first non-flag token in `args` (cores parse flags, take the first bare
arg as id, error if missing — mirrors `RenderInstallCore`'s flag loop).

---

## ADR-9 — Atomic, validate-before-write, recoverable two-file ordering

Goal: a failed op leaves **both** files byte-unchanged. Single-file writes are atomic via temp+rename
in the **same directory** (`os.CreateTemp(dir, ...)`, write, `Sync`, `Rename` over target — rename is
atomic within a filesystem).

### Operation pipeline (all validation BEFORE any write)

1. Read + `ParseRegistry` the registry (fail-loud).
2. `AddEntry` / `RemoveEntry` → `newReg` (fail-loud on precondition).
3. `add` only: stat `<source-root>/<id>/SKILL.md` (fail-loud if absent).
4. `Serialize(newReg)` → `regBytes` (fail-loud if unrepresentable).
5. **Validate-before-write:** `ParseRegistry(regBytes)` → `reg2`; assert `reflect.DeepEqual(newReg, reg2)`
   (re-parse runs `validateEntry` per entry; DeepEqual proves the round-trip). Fail-loud on mismatch.
6. Read manifest bytes; `addManifestRow` / `removeManifestRow` → `manBytes` (pure).
7. **Cross-check:** `Diff(reg2, loadManifestViewReader(manBytes))` must be empty. Fail-loud otherwise.
   This proves the synced pair is `validate`-green before touching disk.
8. Write **manifest first**, then **registry** — both temp+rename.

If any of steps 1–7 fails, nothing is written → both files unchanged.

### Why manifest-first ordering (crash recovery)

Two files cannot be renamed as one atomic transaction without a journal (out of scope). The residual
risk is a crash *between* the two renames. We order writes so that **re-running the identical command
recovers cleanly**, because the registry is the precondition gate and the manifest op is idempotent:

| Op | Crash after manifest write, before registry | `validate` sees | Re-run identical command |
|---|---|---|---|
| `add` | manifest has row, registry lacks entry | `MISSING_IN_REGISTRY` | `AddEntry` succeeds (id absent); `addManifestRow` dedups → consistent. |
| `remove` | manifest lacks row, registry has entry | `MISSING_IN_MANIFEST` | `RemoveEntry` succeeds (id present); `removeManifestRow` no-ops → consistent. |

So `addManifestRow` is **append-if-absent** and `removeManifestRow` is **delete-if-present**
(idempotent). The crash window is two adjacent renames; any divergence is detected by `validate` and
healed by re-running the same command. Documented honestly as residual, recoverable risk.

### Required small refactor

`LoadManifestView(path)` currently opens a file. Extract `loadManifestViewReader(io.Reader)` and have
`LoadManifestView` wrap `os.Open` + the reader form, so step 7 can cross-check the in-memory
`manBytes` without writing first. Pure, behavior-preserving.

---

## Go package surface (new + changed)

| Symbol | File | Kind | Notes |
|---|---|---|---|
| `Serialize(Registry) ([]byte, error)` | serialize.go (new) | pure | inverse of `ParseRegistry` |
| `needsQuote`, `representable`, `emitScalar`, `emitSeq` | serialize.go | pure unexported | grammar helpers |
| `AddEntry(Registry, id) (Registry, error)` | lifecycle.go (new) | pure | defaults + slug + dup guard |
| `RemoveEntry(Registry, id) (Registry, error)` | lifecycle.go | pure | presence guard |
| `addManifestRow([]byte, id, tag) []byte` | lifecycle.go | pure | append-if-absent |
| `removeManifestRow([]byte, id) []byte` | lifecycle.go | pure | delete-if-present |
| `writeFileAtomic(path, data) error` | lifecycle.go | I/O | temp+rename same dir |
| `RenderAddCore(...)`, `RenderRemoveCore(...)` | lifecycle.go | I/O core | thin, mirrors `RenderInstallCore` |
| `loadManifestViewReader(io.Reader)` | manifest.go (refactor) | pure | enables byte cross-check |
| `SkillsCore` add `"add"`,`"remove"` cases + usage | skills.go (mod) | dispatch | update both error strings |

Test files: `serialize_test.go`, `serialize_golden_test.go`, `lifecycle_test.go`
(table tests for AddEntry/RemoveEntry/manifest-row ops), plus `t.TempDir()` tests for
`writeFileAtomic` and the cores (per go-testing skill: pure→table, file ops→TempDir).

---

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| Serializer ↔ parser grammar drift | `TestSerializeRealFileRoundTrip` + golden; both are the inverse of each other, co-located; any drift fails CI. |
| Two-file non-atomicity (crash between renames) | Validate-all-before-write + manifest-first ordering + idempotent row ops → re-run heals; `validate` detects. Full journaling is a non-goal. |
| Forbidden chars in an id/value | `AddEntry` slug guard + `Serialize` `representable` guard → fail-loud, no bytes written. |
| nil vs empty-slice DeepEqual breaks round-trip | `AddEntry` sets `AllowedProjects: nil`; `Serialize` omits empty slices; parser yields nil. |
| Manifest cosmetic churn (append at EOF) | Order-independent for `LoadManifestView`/`Diff`; purely cosmetic; documented. |
| Quoting edge cases | `needsQuote` rules + dedicated tests; `version` always quoted. |

## Non-goals (reaffirmed)

External source fetch/network; deleting `skills/<id>/` on remove; scaffolding new skill **content**;
project-scope add flags (`--allowed-projects`); generating the manifest FROM the registry (Approach B);
managing reference/eval/asset manifest rows; pinned refs; full two-file transactional journaling.

## Next step

`sdd-spec` (R-050+/SC-20+ requirements) and `sdd-design` together feed `sdd-tasks`. This design is the
HOW; tasks will sequence the WHAT-to-do.
