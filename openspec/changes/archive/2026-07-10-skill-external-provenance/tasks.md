# Tasks: skill-external-provenance (Issue #29, Slice 5)

**Change:** skill-external-provenance
**Spec IDs covered:** R-112–R-134 | SC-55–SC-69
**Delivery strategy:** ask-on-risk | **Chain strategy:** feature-branch-chain
**TDD mode:** STRICT — every task ships tests + code as one work unit

---

## Dependency graph

```
T-01 (zero-fetch guard)
  └─> T-02 (types.go)
        ├─> T-03 (parse.go)  ─┐
        └─> T-04 (serialize.go)┤
              T-03 required ──>T-05 (manifestTagFor + validate + sync)
                                └─> T-06 (AddEntry + lifecycle + skills.go)

T-07 (docs) — independent, can run any time after T-01
```

---

## Tasks

### T-01 — Zero-fetch import-allowlist test (ADR-15)
**Parallel:** standalone (write first; no dependencies)
**Spec:** R-131, R-132, SC-69
**File(s):** `engine/skills/zero_fetch_test.go` (NEW)

Write a static-analysis test using `go/parser` + `go/ast` that:
1. Walks every non-`_test.go` `.go` file under `engine/skills/`.
2. Collects all import paths.
3. Asserts the set is a SUBSET of the fixed allowlist:
   `{bufio, bytes, fmt, io, os, reflect, regexp, strings}`.
4. Any import outside the allowlist → `t.Errorf` naming the file and import.

Constraints:
- Use stdlib only (`go/ast`, `go/parser`, `go/token`, `os`, `path/filepath`, `strings`, `testing`).
- Excludes `_test.go` files from the walk (production guard only).
- Must be GREEN against the current `engine/skills/` source on first commit (no forbidden imports exist today).
- Static parse only — no build, network, or module resolution.

Work-unit commit: `test(skills): add zero-fetch import-allowlist guard (ADR-15)`

---

### T-02 — Add Repo/Ref fields to Source struct
**Sequential:** after T-01
**Spec:** R-112 (partial), R-113 (partial), A-8
**File(s):** `engine/skills/types.go`

Add two exported fields to the `Source` struct:
```go
Repo string // origin URL; non-empty only for external entries
Ref  string // vendored commit SHA or tag; optional, non-empty only for external entries
```

Zero-value for `core` and `custom` entries (not emitted unless set).
No test file changes required for this task alone — compile-time correctness is the gate.

Work-unit commit: `feat(types): add Repo/Ref fields to Source (ADR-10)`

---

### T-03 — Parser: external type, repo/ref fields, and validateEntry cross-field rules
**Sequential:** after T-02
**Spec:** R-112, R-113, R-114, R-115, R-116, R-117 | SC-55, SC-56, SC-57, SC-58, SC-59
**File(s):** `engine/skills/parse.go`, `engine/skills/parse_test.go`

**Code changes in parse.go:**
1. Add `"external"` to `validSourceTypes` (enum string updated to `"core, custom, or external"`).
2. `parseSource`: add `case "repo":` → set `s.Repo` and `case "ref":` → set `s.Ref`.
3. `validateEntry` cross-field rules (mirroring allowedProjects-on-global fail-loud pattern):
   - `core` or `custom` entry with `source.repo` or `source.ref` present → line-numbered error naming entry id, offending key, and actual source type.
   - `external` entry with empty `source.repo` → line-numbered error naming entry id, stating repo is required.
   - `external` entry with absent `source.ref` → parse succeeds; `Source.Ref == ""`.
   - `core` upstream.owner non-empty requirement unchanged.

**Tests in parse_test.go (SC-55..SC-59):**
- SC-55: Parse external with repo+ref → fields set, err nil.
- SC-56: Parse external with repo only (no ref) → `Source.Ref == ""`, err nil.
- SC-57: Parse external missing repo → error containing id, `"repo"`, and line number.
- SC-58: Parse core with repo field → error containing id, `"repo"`, `"core"`, and line number.
- SC-59: Parse custom with ref field → error containing id, `"ref"`, `"custom"`, and line number.

Work-unit commit: `feat(parse): external source type, repo/ref fields, cross-field validation (R-112..R-117)`

---

### T-04 — Serializer: emit repo/ref + extend checkRepresentable + round-trip
**Sequential:** after T-02 | **Parallel with T-03**
**Spec:** R-118, R-119, R-120, R-121 | SC-60, SC-61
**File(s):** `engine/skills/serialize.go`, `engine/skills/serialize_test.go`

**Code changes in serialize.go:**
1. Under `source:` block emit in order: `type` → `upstream` (if set) → `repo` (if non-empty, indent 6) → `ref` (if non-empty for external, indent 6).
2. Use existing `scalar()`/`needsQuote()` for repo and ref values.
3. `parseSource` sibling in serializer: add `case "repo":` and `case "ref":` handling (if the serializer has its own parse layer — otherwise ensure `parse.go` handles it, covered by T-03).
4. `checkRepresentable`: extend to cover `source.repo` and `source.ref` against `forbiddenChars`. URL chars `:` and `/` are NOT forbidden (confirmed by tokenizer — do not add them to the forbidden set).

**Tests in serialize_test.go (SC-60, SC-61):**
- SC-60: `parse(serialize(r)) == r` (reflect.DeepEqual) for Registry containing an external entry with repo+ref.
- SC-61: `Serialize` with a repo value containing a forbidden char → error naming id and `"source.repo"`.

Work-unit commit: `feat(serialize): emit repo/ref fields, extend checkRepresentable (R-118..R-121)`

---

### T-05 — manifestTagFor helper + wire validate.go and sync.go
**Sequential:** after T-02 and T-03
**Spec:** R-122, R-123, R-124, R-125 | SC-62, SC-63, SC-64
**File(s):** `engine/skills/sync.go`, `engine/skills/validate.go`, `engine/skills/validate_test.go`, `engine/skills/sync_test.go`

**Code changes:**
1. Add `manifestTagFor(sourceType string) string` helper in `sync.go` (ADR-13):
   - `"core"` → `"managed"`
   - `"custom"` OR `"external"` → `"custom"`
2. Replace `sync.registryTag` implementation to call `manifestTagFor(e.Source.Type)`.
3. Replace `validate.tagMatchesSourceType` to check `tag == manifestTagFor(sourceType)`.
4. `appendManifestLine` already hardcodes `"custom"` for external adds → confirm it stays correct (no change needed).

**Tests:**
- validate_test.go (SC-62): `tagMatchesSourceType("custom","external")==true`; `tagMatchesSourceType("managed","external")==false`.
- validate_test.go (SC-63): Diff with external entry + manifest tag `"custom"` → zero divergences.
- sync_test.go (SC-64): SyncManifest with external entry → output row `"my-ext-skill/SKILL.md custom"`.

Work-unit commit: `feat(validate,sync): manifestTagFor helper unifies external→custom mapping (ADR-13, R-122..R-125)`

---

### T-06 — AddEntry signature, AddCore, parseFlags --repo/--ref, skills.go, and call-site updates
**Sequential:** after T-02, T-03, T-04, T-05 (all prior tasks)
**Spec:** R-126, R-127, R-128, R-129, R-130, R-133 | SC-65, SC-66, SC-67, SC-68
**File(s):** `engine/skills/lifecycle.go`, `engine/skills/lifecycle_test.go`, `engine/skills/skills.go`, `bin/labdrian-overlay`

**Code changes in lifecycle.go (ADR-14):**
1. Change `AddEntry` signature: `AddEntry(reg Registry, id, repo, ref string) (Registry, error)`.
   - `repo == ""` → `Source.Type = "custom"` (backward-compatible default, R-128).
   - `repo != ""` → `Source{Type: "external", Repo: repo, Ref: ref}` (R-127).
2. `parseFlags`: add `--repo <url>` and `--ref <sha>` extraction in the existing flag-parsing loop (same pattern as `--registry`), R-126.
3. `AddCore`: pass `repo` and `ref` through to `AddEntry`. Add guard: `--ref` without `--repo` → error exit, R-129.
4. Precondition UNCHANGED: `statFile(skills/<id>/SKILL.md)` still checked before any write (R-129, R-060).
5. `bin/labdrian-overlay`: update usage/help line to include `--repo` and `--ref`; argument passthrough already works (R-133).
6. Update ALL existing `AddEntry(reg, id)` call sites to `AddEntry(reg, id, "", "")` (compile-time breakage guarantees exhaustiveness).

**Tests in lifecycle_test.go (SC-65..SC-68) + call-site fixes:**
- Update existing test call sites: `AddEntry(reg, id)` → `AddEntry(reg, id, "", "")`.
- SC-65: `AddCore --repo` → exit 0, `Source.Type == "external"`, manifest has `"foo/SKILL.md custom"`, `Validate nil`.
- SC-66: `AddCore --repo --ref` → exit 0, `Source.Ref` set, round-trip holds.
- SC-67: `AddCore` without `--repo` → `Source.Type == "custom"` (non-regression).
- SC-68: `AddCore --repo` with missing `SKILL.md` dir → exit != 0, files unchanged, no network I/O.

Work-unit commit: `feat(lifecycle,skills): AddEntry provenance params, --repo/--ref flags, update call sites (R-126..R-130, R-133)`

---

### T-07 — Documentation: external = vendored, never fetched
**Parallel:** independent; can run at any time after T-01
**Spec:** R-131 (documentation aspect), R-132 (no new imports — already enforced by T-01)
**File(s):** `README.md` (or skill doc if present), skill YAML/metadata if applicable

Add a short note (≤ 5 lines) in the relevant documentation section:
- `external` source type records PROVENANCE METADATA only (repo = origin URL, ref = vendored commit).
- The tool NEVER fetches, clones, or executes any remote resource.
- Vendoring is a human responsibility; `--repo`/`--ref` are inert labels.

Work-unit commit: `docs: note external source = vendored provenance, zero fetch (R-131)`

---

## Execution order summary

| Order | Task | Depends on | Can parallel with |
|-------|------|-----------|-------------------|
| 1 | T-01 zero-fetch guard | — | T-07 |
| 2 | T-02 types.go | T-01 | — |
| 3 | T-03 parse.go + tests | T-02 | T-04 |
| 3 | T-04 serialize.go + tests | T-02 | T-03 |
| 4 | T-05 manifestTagFor + validate + sync | T-02, T-03 | — |
| 5 | T-06 AddEntry + lifecycle + skills.go | T-02–T-05 | — |
| any | T-07 docs | T-01 | all |

**Parallelism pairs:** T-03 ∥ T-04 (after T-02); T-07 ∥ everything.

---

## Review Workload Forecast

| Metric | Estimate |
|--------|----------|
| zero_fetch_test.go (new) | ~40 lines |
| types.go | ~5 lines |
| parse.go + parse_test.go | ~130 lines |
| serialize.go + serialize_test.go | ~70 lines |
| validate.go + sync.go + tests | ~90 lines |
| lifecycle.go + lifecycle_test.go + skills.go + bin | ~130 lines |
| docs | ~10 lines |
| **Total estimated changed lines** | **~475 lines** |

**400-line budget risk: High**
**Chained PRs recommended: Yes**
**Decision needed before apply: Yes**

### Proposed slice boundaries (feature-branch-chain)

**PR 1 — provenance foundation** (targets tracker branch):
T-01, T-02, T-03, T-04 — zero-fetch guard + types + parse + serialize.
Estimated: ~245 lines. Self-contained; no behaviour visible to users yet.

**PR 2 — manifest wiring + full lifecycle** (targets PR 1 branch):
T-05, T-06, T-07 — manifestTagFor + AddEntry + flags + docs.
Estimated: ~230 lines. Completes the feature end-to-end.
