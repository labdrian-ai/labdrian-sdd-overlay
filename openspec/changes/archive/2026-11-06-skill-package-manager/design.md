# Design: Scoped Skill Package Manager — First Slice (Read-Only Semantic Layer)

Architectural HOW for the read-only `skills.registry.yaml` descriptor plus the
`engine skills` / `labdrian skills` command group. Grounded in the proposal
(`sdd/skill-package-manager/proposal`) and the binding separation rule
(`architecture/overlay-gentle-ai-separation`, #1561).

## Quick path (what we are building)

1. A new repo-root file `skills.registry.yaml` — a read-only semantic descriptor,
   one entry per skill, never consumed by deploy.
2. A new zero-dep Go package `engine/skills/` — parse + list + status + validate,
   pure and unit-tested (strict TDD).
3. Dispatch `engine skills <verb>` from `engine/cmd/main.go` (labdrian-custom, safe).
4. Forward `labdrian skills <verb>` → engine from `bin/labdrian-overlay`
   (the prespec passthrough pattern). Never `bin/overlay`.
5. `labdrian skills validate` — fail-loud cross-check of registry vs manifest.

## Architecture approach

Layered, additive, read-only. The registry is a **descriptor layer** that sits
on top of `overlay.manifest` (which stays the sole deploy ground truth). The Go
package is pure and dependency-free; the bash router only forwards. No existing
deploy/capture path is touched, so rollback is deletion.

```
labdrian skills <verb>            (bin/labdrian-overlay — custom router)
        │  exec, forwards --registry/--manifest paths
        ▼
engine skills <verb>              (engine/cmd/main.go — labdrian-custom dispatch)
        │  SkillsCore(verb, deps, stdout, stderr, exit)
        ▼
engine/skills/                    (pure, zero-dep, strict TDD)
  parse.go   ParseRegistry(io.Reader) (Registry, error)   ← strict YAML subset
  load.go    Load(path) (Registry, error)                 ← os.Open + ParseRegistry
  validate.go Diff(Registry, ManifestView) []Divergence   ← pure reconciler
             Validate(reg, manifestPath) error            ← loader + Diff, fail-loud
  list.go    RenderList(w, Registry)                       ← golden-tested table
  status.go  RenderStatus(w, Registry, ManifestView)       ← list + alignment summary
  types.go   Skill, Source, Install, Lifecycle, Registry, Divergence
        ▲
        │ reads (never writes)
skills.registry.yaml  +  overlay.manifest
```

Data flow is one-directional and read-only: files in → structs → rendered text
or a divergence error out. No file is ever written by this feature.

## Decisions (ADR-style)

### ADR-1 — On-disk format: keep YAML, parse a strict hand-rolled subset (zero-dep preserved)

**Decision.** Keep the human-friendly `skills.registry.yaml`, but parse it with a
small, **strict, fail-loud** subset parser inside `engine/skills/`. Do NOT add
`gopkg.in/yaml.v3`. Do NOT switch to JSON/TSV.

**Why.** `engine/` has a hard-won **zero external dependency** invariant (`go.mod`
declares only `go 1.21`, no `require` block). That property — instant builds,
trivially auditable binary, zero supply-chain surface — is part of the engine's
identity and is consistent with its fail-loud ethos. The registry in this slice
is a ~20-entry, schema-we-own, read-only descriptor. Pulling a general YAML
library to read a file whose grammar we fully control is disproportionate.

**Critically, this is NOT "reinventing YAML."** We define a tiny DSL that *is* a
strict subset of YAML and **reject every construct outside it with a line-numbered
error**. The classic trap of hand-rolled YAML (silently misreading valid YAML)
is converted into fail-loud behavior, which is exactly the engine's contract.

**The supported subset (`skills.registry.yaml` grammar v1):**

| Allowed | Rejected (fail-loud, with line number) |
|---|---|
| UTF-8, LF line endings | Tabs in indentation (spaces only) |
| Full-line comments (`^\s*#`) and blank lines | Inline/trailing comments (v1 limitation) |
| `key: value` scalar mappings | Flow mappings `{ ... }` |
| `key:` then deeper-indented nested mapping | Flow sequences `[ a, b ]` |
| Block sequences (`- ` items) of mappings or scalars | Anchors/aliases `&` `*`, tags `!` |
| Plain or single/double-quoted scalars | Block scalars `\|` `>`, multi-doc `---` / `...` |

Indentation is space-based and structural (2 spaces per level by convention; the
parser tracks indent columns, not a fixed width). This subset covers the current
schema **and** the foreseeable next-slice nesting (`source.upstream`,
`install.allowedProjects[]` as a scalar block list), so the boundary holds.

**Rejected alternatives.**
- *(a) `gopkg.in/yaml.v3`* — correct and battle-tested, but breaks the zero-dep
  invariant for a read-only descriptor. **Deferred, not discarded:** the day the
  schema needs true YAML richness (e.g. `external` sources with nested
  `upstream: {owner, repo, ref}` maps, or multi-line scalars), adding yaml.v3
  becomes justified. This is the documented escape hatch and a clean evolution
  boundary.
- *(b naïve) permissive hand-rolled parser* — rejected; a lenient subset parser
  silently misreads real YAML. We take the strict variant instead.
- *(c) JSON / TSV* — JSON keeps zero-dep via stdlib but loses comments and
  human-friendliness during the hand-maintained slice; TSV cannot express the
  nesting. Both fight issue #29's explicit `skills.registry.yaml` intent.

**Test seams.** `ParseRegistry(io.Reader) (Registry, error)` is the pure seam —
table-driven tests feed in-memory strings; no filesystem needed. One table case
per rejected construct asserts the specific line-numbered error.

### ADR-2 — Go package layout: pure functions, no global state

**Decision.** New package `engine/skills/`, mirroring the `engine/prespec/`
testable-core pattern (`PrespecCore(verb, stdin, stdout, stderr, exit)`).

| File | Responsibility | Test pattern |
|---|---|---|
| `types.go` | `Skill`, `Source`, `Install`, `Lifecycle`, `Registry`, `Divergence` + enums | — |
| `parse.go` | `ParseRegistry(io.Reader) (Registry, error)` strict subset parser + schema validation | Table-driven |
| `load.go` | `Load(path string) (Registry, error)` = `os.Open` + `ParseRegistry` | `t.TempDir` |
| `validate.go` | `Diff(reg Registry, mv ManifestView) []Divergence` (pure) + `Validate(reg, manifestPath) error` (loader + Diff) | Table-driven (Diff), `t.TempDir` (loader) |
| `manifest.go` | `LoadManifestView(path) (ManifestView, error)` — derives skill-dir → tag map | `t.TempDir` |
| `list.go` | `RenderList(w io.Writer, reg Registry)` aligned table | Golden file |
| `status.go` | `RenderStatus(w, reg, mv)` = list + alignment summary | Golden file |
| `skills.go` | `SkillsCore(verb string, deps Deps, stdout, stderr io.Writer, exit func(int))` dispatch | Injected exit/stdout/stderr |

No global state. All I/O is injected (`io.Reader`/`io.Writer`, path args,
`exit func(int)`), so every branch is unit-testable without touching `$HOME`.
Rendered output is sorted by `id` for deterministic golden files (never file order).

### ADR-3 — Manifest cross-check: bidirectional, skill-dir granularity, fail-loud

**Decision.** `Validate` reconciles the registry against `overlay.manifest` at
**skill-directory granularity** and fails loud (exit 1) on any divergence.

**Manifest → skill view.** `overlay.manifest` is per-*file* (e.g.
`genesis-design-system/SKILL.md` + `genesis-design-system/references/*.md`).
`LoadManifestView` collapses rows to their **first path segment** (the skill dir)
and records the tag, after excluding an explicit infra allowlist:

```
INFRA_PREFIXES = { "engine/", "_shared/" }   // not skills; never expected in registry
```

`engine/` rows are deploy-inert anyway (apply resolves `skills/${path}`, and
`engine/` lives at repo root → skipped); `_shared/` holds shared contracts, not
skills. Both are excluded from the cross-check.

**Tag mapping.** `managed ⇔ source.type: core`, `custom ⇔ source.type: custom`.

**Divergence classes** (each emitted to stderr, all collected, then exit 1):

| Class | Condition |
|---|---|
| `MISSING_IN_MANIFEST` | registry entry whose `path` has no manifest skill-dir |
| `MISSING_IN_REGISTRY` | manifest skill-dir (non-infra) with no registry entry |
| `TAG_MISMATCH` | registry `source.type` disagrees with manifest tag |
| `MIXED_TAG` | a manifest skill-dir whose rows carry both `managed` and `custom` |

**Schema-internal invariants** (checked in `ParseRegistry`, also fail-loud):
`type: core ⇔ updateStrategy: vendor-merge`, `type: custom ⇔ overlay-only`;
`source.upstream` required iff `core`; `targets ⊆ {claude, opencode, codex}` and
non-empty; `defaultScope: global` only; unique `id`.

**Fail-loud mechanics.** `Validate` returns a `*ValidationError` carrying the
`[]Divergence`. The CLI prints one line per divergence to **stderr** and calls
`exit(1)`. Zero divergence → `exit(0)` with `registry and manifest aligned (N skills)`
on stdout. This is the proposal's "fail loud on drift" made concrete.

### ADR-4 — CLI wiring: custom dispatch only, never the vendored bin/overlay

**Decision.**
- `engine/cmd/main.go`: add `case "skills": runSkills(os.Args[2:])`. `runSkills`
  parses the verb + `--registry`/`--manifest` flags and calls `skills.SkillsCore`.
  **Confirmed safe** against #1561: `engine/` is 100% labdrian, NOT on the
  `upstream` branch — editing `main.go` cannot conflict with a gentle-ai update.
- `bin/labdrian-overlay`: add `cmd_skills()` mirroring `cmd_prespec()` — assert the
  engine binary exists, then `exec "$ENGINE_BINARY" skills "$@"`, defaulting
  `--registry "$OVERLAY_DIR/skills.registry.yaml"` and
  `--manifest "$OVERLAY_DIR/overlay.manifest"`. Register in the dispatch `case`
  and `usage()`.
- **`bin/overlay` is NEVER touched** — it is the byte-identical vendored upstream
  reference (invariant: `diff <(git show upstream:bin/overlay) bin/overlay` empty).

**Verb split.**

| Verb | Reads | Output | Exit |
|---|---|---|---|
| `list` | registry | sorted table (golden) | 0 |
| `status` | registry + manifest | table + alignment summary (informational) | 0 |
| `validate` | registry + manifest | divergence lines or "aligned" | 0 aligned / 1 diverged |

Paths are passed as explicit flags (testable; no hidden env), defaulted by the
bash router from `$OVERLAY_DIR` (already exported for the TUI).

### ADR-5 — registry.yaml ownership: repo-root, NOT in overlay.manifest

**Decision.** `skills.registry.yaml` lives at **repo root** and is **NOT** added
to `overlay.manifest`. It is git-tracked on `main`, like `README.md`,
`.gitignore`, and `overlay.manifest` itself — none of which are manifest rows.

**Why this also solves "core entries must survive upstream merge."** The vendor
sync machinery only ever touches:
- `managed_files()` on **capture** (copies FROM `~/.claude/skills/<path>`), and
- `git merge upstream` on **apply** (upstream branch content).

`skills.registry.yaml` is a path **gentle-ai upstream does not have**, lives only
on `main`, and is never under `skills/`. Therefore capture never reads it, the
upstream branch never carries it, and a merge can never clobber it. The `core`
*entries inside* the file survive automatically because the **whole file is
invisible to the vendor machinery** — exactly the golden rule from #1561
("nothing labdrian-owned lives in a file gentle-ai owns").

**Proposal refinement (flagged).** The proposal's Affected Areas said "add
`skills.registry.yaml` rows" to `overlay.manifest`. The design corrects this: a
manifest row implies the file lives under `skills/` and is deployed to each
target (`apply` copies `skills/${path}` → `TARGET/skills/${path}`). Deploying a
repo-management descriptor into every agent's skills dir is wrong; the file is
not a skill. Root-level git tracking is the correct home.

**Amendment (reconciles spec R-044).** The implementation keeps a single inert
`skills.registry.yaml custom` row in `overlay.manifest`, matching spec R-044. This
is ACCEPTED and does NOT weaken the survival guarantee above: `custom` files are
never read by `capture` (it only touches `managed_files()`), and the repo-root
path is skipped by `apply` (it resolves `skills/skills.registry.yaml`, which does
not exist). So the file stays invisible to the vendor machinery either way. The
row therefore serves purely as ownership/tracking documentation, consistent with
the already-inert `engine/*` rows. The original "NOT in manifest" stance and this
row are functionally equivalent; the row is the conventional, lower-churn choice.

**engine/skills/*.go manifest rows (cosmetic).** Existing `engine/*` rows are
tagged `managed` but are **inert** (apply/capture skip them — `engine/` is not
under `skills/`). Adding `engine/skills/*.go` rows is optional documentation. If
added, tag them `custom` to reflect truth (engine is labdrian-owned); leaving the
legacy `managed` engine rows as-is is out of scope. The cross-check excludes
`engine/` regardless, so the tag has no functional effect.

## Schema (skills.registry.yaml v1)

```yaml
# Read-only semantic descriptor of overlay-managed skills.
# DEPLOY ground truth remains overlay.manifest. Never consumed by apply/capture.
skills:
  - id: sdd-spec
    source:
      type: core            # core | custom
      upstream: gentle-ai   # required iff type: core
    path: sdd-spec
    install:
      defaultScope: global  # global only (this slice)
      targets:
        - claude
        - opencode
        - codex
    lifecycle:
      updateStrategy: vendor-merge   # core ⇔ vendor-merge
  - id: prespec-malandra
    source:
      type: custom
    path: prespec-malandra
    install:
      defaultScope: global
      targets:
        - claude
        - opencode
        - codex
    lifecycle:
      updateStrategy: overlay-only   # custom ⇔ overlay-only
```

Initial population (≈20 skills) derived from `overlay.manifest`, first path
segment as skill dir, excluding `engine/` and `_shared/`:
- **core** (manifest `managed`): `sdd-spec`, `sdd-tasks`, `sdd-verify`.
- **custom**: `gadu-orchestrate`, `requirements-from-transcripts`,
  `prespec-malandra`, `kadia-content-guard`, `kadia-ui-fix`, `kadia-visual-qa`,
  `genesis-delivery-workflow`, `genesis-design-system`, `chat-thread-analyzer`,
  `project-inception`, `inception-pipeline`, `project-manifest`,
  `project-architect`, `roadmap-maker`, `sdd-time-estimation`.

## Testability (strict TDD)

| Target | Pattern |
|---|---|
| `ParseRegistry` valid + each rejected construct + schema invariants | Table-driven, in-memory strings, assert line-numbered errors |
| `Diff` divergence classes (aligned / MISSING_* / TAG_MISMATCH / MIXED_TAG) | Table-driven, in-memory `Registry` + `ManifestView` |
| `Load` / `LoadManifestView` | `t.TempDir` fixtures |
| `RenderList` / `RenderStatus` | Golden files in `engine/skills/testdata/golden/`, `-update` flow |
| `SkillsCore` dispatch (unknown verb fails loud; each verb routes) | Injected `exit`/`stdout`/`stderr`, mirror `PrespecCore` tests |

Every Go unit is test-first. Pure cores (`ParseRegistry`, `Diff`, `Render*`) need
no filesystem; only the loaders and the golden tests touch disk (via `t.TempDir`
or committed `testdata/`).

## Risks & mitigations

| Risk | Sev | Mitigation |
|---|---|---|
| Hand-rolled parser drifts from real YAML | Med | Strict whitelist + line-numbered fail-loud on every out-of-subset construct; subset documented; escape hatch to yaml.v3 (ADR-1) if schema outgrows it |
| Two sources of truth (registry vs manifest) diverge | Med | `validate` is the reconciler, fail-loud; wiring into apply/CI deferred but available |
| Infra allowlist (`engine/`, `_shared/`) goes stale | Low | Single named constant; new infra prefix = one-line change; documented |
| Schema overfits today | Med | Encode only today's dimensions (`defaultScope: global` fixed; no external/project) |
| registry.yaml absent at runtime | Low | `Load` fails loud with a clear "registry not found" message |

## Deferred (non-goals, restated)

`skills add` / `skills remove`; `external` and `project` source types; `project`
scope + `allowedProjects[]`; `updateStrategy: pinned-ref`; manifest generation
from registry (Approach B); wiring `validate` into `apply`/`capture` (deploy
stays byte-identical); the TUI skills screen. The yaml.v3 dependency is deferred
to whichever later slice first needs true YAML richness.

## Rollback

Delete `skills.registry.yaml`, remove `engine/skills/`, revert the `skills`
dispatch in `engine/cmd/main.go`, drop `cmd_skills` from `bin/labdrian-overlay`.
No deploy/capture path changed → zero effect on apply/capture; the binary returns
to its prior subcommand set.
