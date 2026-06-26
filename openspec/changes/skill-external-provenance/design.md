# Design: skill-external-provenance (Issue #29, slice 5)

## Outcome

Add a third `source.type` — `external` — that records **provenance metadata only**
(`repo` = origin URL, `ref` = vendored commit/ref) for a skill a human has already
reviewed and vendored into `skills/<id>/`. The tool NEVER fetches: `external` is pure
inert metadata, validated, round-tripped, and mapped to the `custom` manifest tag.
The slice's credibility anchor is a static import-allowlist test that makes a hidden
fetch impossible to land without a reviewer noticing.

## Architecture Approach

Pure additive metadata over the existing zero-dependency registry engine
(`engine/skills/`, 100% labdrian-owned per `architecture/overlay-gentle-ai-separation`).
No new package, no new import, no I/O surface. Every change is an extension of an
existing pure function or table:

- Data model: two optional fields on `Source`.
- Parser: one new `type` enum value + two new keys under the existing `source` mapping.
- Validator: one new branch in `validateEntry`, mirroring the `allowedProjects`-on-global
  fail-loud pattern already present.
- Serializer: emit `repo`/`ref` after `type`, omit when empty, extend the representability guard.
- Tag mapping: external→custom, unified in ONE helper consumed by both `validate.Diff`
  (`tagMatchesSourceType`) and `sync.SyncManifest` (`registryTag`).
- Lifecycle: thread optional `--repo`/`--ref` through `parseFlags` → `AddCore` → `AddEntry`.
- Guard: a new static test asserting the package imports stay within a fixed allowlist.

The layering and boundaries are unchanged: pure transforms (`AddEntry`, `Serialize`,
`ParseRegistry`, `Diff`, `SyncManifest`) stay pure and table-testable; the `*Core`
functions remain the only I/O boundary with all side effects injected.

## Component & Data-Flow Map

```
bin/labdrian-overlay (arg passthrough, usage only)
        │  skills add <id> [--repo <url>] [--ref <sha>]
        ▼
SkillsCore ── "add" ──► AddCore(args, readFile, statFile, …)   [I/O boundary]
        │                   │
        │                   ├─ parseFlags(args) → (…, id, repo, ref)   [extended]
        │                   ├─ AddEntry(reg, id, repo, ref)            [PURE, extended]
        │                   │     repo=="" ⇒ Source{Type:"custom"}
        │                   │     repo!="" ⇒ Source{Type:"external", Repo, Ref}
        │                   ├─ statFile(skills/<id>/SKILL.md)  (precondition, UNCHANGED — no fetch)
        │                   ├─ Serialize(newReg)               [extended emit + guard]
        │                   ├─ ParseRegistry(bytes)            [extended parse + validate]
        │                   │     reflect.DeepEqual round-trip check (R-063)
        │                   ├─ appendManifestLine(man, id)     (emits "custom" — already correct)
        │                   └─ Diff(reg2, mv)                  [external→custom via shared helper]
        ▼
skills.registry.yaml + overlay.manifest   (atomic dual-temp write, UNCHANGED)
```

YAML shape (flat — `repo`/`ref` live directly on `source`, sibling to `type`/`upstream`):

```yaml
- id: some-vendored-skill
  path: some-vendored-skill
  source:
    type: external
    repo: https://github.com/acme/cool-skill
    ref: 4f3c2a1
  install:
    defaultScope: global
    targets:
      - claude
  lifecycle:
    updateStrategy: overlay-only
```

## Decisions (ADR-style)

### ADR-10 — `Source` carries flat `Repo`/`Ref`, not a nested `provenance:` mapping

**Decision.** Add `Repo string` and `Ref string` directly to `Source`, siblings of
`Type` and `*Upstream`. YAML: `source: { type, repo, ref }`.

```go
type Source struct {
    Type     string    // "core" | "custom" | "external"
    Upstream *Upstream // optional; only valid when Type == "core"
    Repo     string    // optional; only valid when Type == "external" (origin URL)
    Ref      string    // optional; only valid when Type == "external" (vendored commit/ref)
}
```

**Rationale.** `upstream` is already an optional, type-gated sub-mapping; provenance is
the symmetric concept for `external`. Two scalar strings are simpler than a third nested
mapping and a third `parseX`/`Upstream`-style struct. Flat strings serialize and
round-trip with the existing `scalar`/`needsQuote` machinery — no new parser function
needed beyond two `case` arms inside `parseSource`.

**Rejected — nested `provenance:` mapping** (`source.provenance.{repo,ref}`). Mirrors
`upstream` structurally but costs a new `Provenance` struct, a new `parseProvenance`,
and a new serialize block, for zero added expressiveness. The flat form keeps the diff
small and the parser flat. If provenance ever grows (signatures, multiple refs) we can
promote to a mapping later; YAGNI now.

**Rejected — reuse `upstream.owner` for the URL.** Conflates two distinct trust models
(`core` = synced-from-upstream vs `external` = vendored-once-from-elsewhere) and would
break the existing `upstream only valid for core` invariant.

### ADR-11 — `repo` REQUIRED, `ref` OPTIONAL for `external`

**Decision.** An `external` entry MUST have a non-empty `repo`. `ref` is OPTIONAL.
`repo` or `ref` on a `core` or `custom` entry is a HARD error.

| `source.type` | `upstream`            | `repo`            | `ref`     |
|---------------|-----------------------|-------------------|-----------|
| `core`        | optional; owner≠"" if present | **forbidden** (hard error) | **forbidden** (hard error) |
| `custom`      | forbidden (existing)  | **forbidden** (hard error) | **forbidden** (hard error) |
| `external`    | **forbidden** (hard error) | **REQUIRED** (non-empty)   | optional  |

**Rationale.** `repo` is the whole point of `external` — provenance with no origin URL
is indistinguishable from `custom`, so it must be required to make the type meaningful.
`ref` is recommended-but-optional because a human may vendor loosely from a tag/branch
they noted informally, or from a snapshot whose exact SHA they did not record; forcing a
fake SHA would be worse than an honest empty `ref`. The forbidden-on-core/custom rules
mirror the existing `allowedProjects`-only-for-project-scope rejection (validate.go
`Install.DefaultScope == "global" && len(AllowedProjects) > 0`) and the existing
`upstream`-not-allowed-on-custom rule — same fail-loud shape, same error-message style.

**Validation lives in `validateEntry`** (post-parse, entry-level), not in `parseSource`,
because it is a cross-field invariant (type ↔ repo/ref) exactly like the existing
upstream/type checks. New branches, appended after the upstream checks:

```go
// add "external" to the enum
var validSourceTypes = map[string]bool{"core": true, "custom": true, "external": true}

// in validateEntry, after the existing core/custom upstream checks:
if e.Source.Type == "external" {
    if e.Source.Upstream != nil {
        return fmt.Errorf("skills: entry %q: source.upstream is not allowed when source.type is 'external'", e.ID)
    }
    if e.Source.Repo == "" {
        return fmt.Errorf("skills: entry %q: source.repo is required when source.type is 'external'", e.ID)
    }
}
if e.Source.Type != "external" && (e.Source.Repo != "" || e.Source.Ref != "") {
    return fmt.Errorf("skills: entry %q: source.repo/source.ref are only valid when source.type is 'external'", e.ID)
}
```

The enum error message in `validateEntry` updates to `"must be 'core', 'custom', or 'external'"`.

### ADR-12 — Serializer field order and round-trip guarantee

**Decision.** Under `source:`, emit in this fixed order: `type`, then `upstream` (if set),
then `repo` (if non-empty), then `ref` (if non-empty). `repo`/`ref` are emitted ONLY when
non-empty and ONLY meaningful for `external` (a non-external entry can never have them set
because `validateEntry` rejects that, and `AddEntry` never sets them for custom).

```go
b.WriteString("      type: ")
b.WriteString(scalar(e.Source.Type))
b.WriteByte('\n')
if e.Source.Upstream != nil { /* unchanged */ }
if e.Source.Repo != "" {
    b.WriteString("      repo: ")
    b.WriteString(scalar(e.Source.Repo))
    b.WriteByte('\n')
}
if e.Source.Ref != "" {
    b.WriteString("      ref: ")
    b.WriteString(scalar(e.Source.Ref))
    b.WriteByte('\n')
}
```

**Round-trip (`parse(serialize(r)) == r`).** Holds because: (a) empty `ref` is omitted on
write and parses back as empty — symmetric; (b) `repo` is always present for `external`
(required) so it always survives; (c) field order is deterministic and the parser is
order-tolerant within a mapping, so DeepEqual is stable; (d) `parseSource` gains `case
"repo"` / `case "ref"` arms reading `t.val` into `src.Repo` / `src.Ref`.

**Representability guard.** Extend `checkRepresentable` to test `e.Source.Repo` and
`e.Source.Ref` against `forbiddenChars = "{}[]&*!\t"`. A typical repo URL
(`https://github.com/acme/cool-skill`) contains `:` and `/` — NEITHER is in the forbidden
set (confirmed against the tokenizer: `:` and `/` are not in `{}[]&*!` nor a tab), so URLs
serialize bare or quoted fine. `needsQuote` already quotes a value containing `": "` or a
leading indicator, which a URL does not trigger; a URL with a space (malformed) would be
quoted and still round-trip. The guard only rejects genuinely unrepresentable bytes,
matching every other scalar field.

### ADR-13 — Single source of the external→custom tag mapping

**Decision.** Introduce ONE unexported helper as the single authority for source-type →
manifest-tag, and have BOTH the validate path and the sync path consume it.

```go
// manifestTagFor returns the manifest tag a registry entry's source.type maps to.
// core → managed; custom AND external → custom (vendored, labdrian-owned).
func manifestTagFor(sourceType string) string {
    if sourceType == "core" {
        return "managed"
    }
    return "custom" // custom and external are both labdrian-owned, not upstream-synced
}
```

- `sync.registryTag(e Entry)` becomes `return manifestTagFor(e.Source.Type)`.
- `validate.tagMatchesSourceType(tag, sourceType)` becomes `return tag == manifestTagFor(sourceType)`.

**Rationale.** Today the mapping is duplicated: `registryTag` (sync.go) hardcodes
`core→managed else custom`, and `tagMatchesSourceType` (validate.go) hardcodes
`managed↔core, custom↔custom`. Adding `external` to two places invites drift (exactly the
"validate/sync misalign external" risk in the proposal). Collapsing to one helper makes
`external→custom` true everywhere by construction, and a single Diff/round-trip test
covers both consumers. `manifestTagFor("external") == "custom"` means `appendManifestLine`
(which already hardcodes the `custom` tag) stays correct for `external` adds with no change.

**Note.** `tagMatchesSourceType`'s old form returned `false` for unknown tags; the new form
returns `tag == manifestTagFor(...)`. For an unknown manifest tag (e.g. `managed` vs an
`external` entry → expects `custom` → mismatch → `false`) behavior is preserved: any tag
that isn't the mapped one yields `false`, still flagged as `TAG_MISMATCH`.

### ADR-14 — Thread `--repo`/`--ref` through `parseFlags` → `AddCore` → `AddEntry`, keep `AddEntry` pure

**Decision.** Extend `parseFlags` to capture `--repo`/`--ref`, extend the `AddEntry`
signature to `AddEntry(reg Registry, id, repo, ref string)`, and branch on `repo`:

```go
func AddEntry(reg Registry, id, repo, ref string) (Registry, error) {
    // ... existing slug + duplicate guards (unchanged) ...
    src := Source{Type: "custom"} // repo=="" ⇒ custom (current behavior preserved)
    if repo != "" {
        src = Source{Type: "external", Repo: repo, Ref: ref}
    }
    newEntry := Entry{ID: id, Path: id, Source: src, Install: /*unchanged defaults*/, Lifecycle: /*unchanged*/}
    // ... copy + append (unchanged) ...
}
```

`parseFlags` returns `repo, ref` as two new trailing values (same `--flag value` loop
pattern as `--registry`/`--manifest`). `AddCore` passes them to `AddEntry`. Everything
downstream (Serialize, round-trip check, manifest append, Diff, atomic write) is unchanged.

**Defensive guard in `AddCore`.** Reject `--ref` supplied without `--repo`
(`ref != "" && repo == ""`) with a loud error before calling `AddEntry`, because a lone
`ref` would otherwise be silently dropped on a `custom` entry. This is a usability guard,
not a schema rule.

**Precondition UNCHANGED, ZERO fetch.** Step 3 of `AddCore` still requires
`skills/<id>/SKILL.md` to already exist on disk (`statFile`). `--repo` does NOT trigger any
download — it only labels an already-vendored local directory. This is the heart of the
slice: `add --repo` registers provenance for files a human already reviewed and committed.

**Rationale for keeping `AddEntry` pure.** A signature change (vs an options struct or a
package-level mutable default) keeps the function a pure data transform with no hidden
state, so the new external/custom branch is covered by a single table-driven test
(`repo=""` → custom; `repo="url"` → external with/without ref). Existing call sites
(`AddCore`, `lifecycle_test.go`) update to pass `"", ""` for the custom path — mechanical
churn, no behavior change.

**Rejected — options struct `AddEntry(reg, AddOpts{ID, Repo, Ref})`.** Cleaner for many
optional params, but overkill for two strings and would churn more existing tests for no
table-testability gain. Revisit only if a future slice adds more add-time options.

### ADR-15 — Zero-fetch guard: static import allowlist test (credibility anchor)

**Decision.** Add a Go test that statically parses every non-test `.go` file in
`engine/skills/` with `go/parser` + `go/ast`, collects the full set of imported package
paths, and asserts that set is a SUBSET of a fixed allowlist. Any import outside the
allowlist fails the test.

```go
// zero_fetch_test.go (package skills)
var allowedImports = map[string]bool{
    "bufio": true, "bytes": true, "fmt": true, "io": true,
    "os": true, "reflect": true, "regexp": true, "strings": true,
}
// Test parses ParseDir(".", filter: !strings.HasSuffix(name, "_test.go")),
// walks every *ast.ImportSpec, strips quotes, and for each path:
//   if !allowedImports[path] { t.Errorf("forbidden/unexpected import %q in %s", path, file) }
```

**Why an allowlist, not a denylist.** A denylist (`net`, `net/http`, `os/exec`, `git…`)
can be defeated by a creative new import the denylist never anticipated (e.g.
`golang.org/x/net`, a vendored git wrapper, `syscall`, `plugin`). An allowlist is
**unfoolable**: introducing ANY new import — fetch-related or not — turns the test red and
forces a reviewer to consciously widen `allowedImports`, making a stealth network/exec
dependency impossible to land silently. This is the slice's central promise made
mechanically enforceable, exactly the mitigation the proposal's "reader fears hidden
fetch" risk demands.

**Coverage detail.** `os` is allowlisted (the engine legitimately does file I/O), but
`os/exec` is a DISTINCT import path and is NOT in the allowlist → a subprocess call fails
the test. `net` and `net/http` are absent → any network import fails. The test excludes
`_test.go` files so test helpers may use richer imports without weakening the production
guard; the production guard is what ships in the engine binary.

**Why static parse over `go list`/build tags.** `go/parser` needs no build, no network,
no module resolution — it reads source text directly, so the guard itself honors the
zero-dependency, hermetic ethos and runs in the same `go test ./...` pass under Strict TDD.

## Test Strategy (Strict TDD, table-driven per go-testing skill)

| Target | Pattern | Key cases |
|--------|---------|-----------|
| `validateEntry` external rules | table-driven | external+repo ✓; external no repo ✗; external+upstream ✗; core/custom with repo or ref ✗; enum rejects bad type |
| parse external mapping | table-driven | `source: {type: external, repo, ref}` → struct; repo-only (no ref) ✓ |
| `Serialize` external | golden + round-trip | field order type→repo→ref; ref omitted when empty; `parse(serialize(r))==r` via `reflect.DeepEqual` |
| representability | table-driven | repo URL with `:` `/` ✓ representable; `repo` with `{` ✗ |
| `manifestTagFor` / Diff / Sync | table-driven | external entry vs `custom` manifest tag → no divergence; vs `managed` → TAG_MISMATCH |
| `AddEntry` provenance | table-driven (pure) | repo=="" → custom; repo set → external; repo+ref → external with ref |
| `AddCore` external | `t.TempDir()` I/O | `--repo` ⇒ external entry + `custom` manifest row; missing SKILL.md still fails (no fetch); `--ref` without `--repo` fails loud |
| zero-fetch guard | static `go/parser` | production imports ⊆ allowlist; net/http, os/exec, git absent |

## Risks & Alternatives

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Reader/reviewer fears a hidden fetch path | Med | ADR-15 static import-allowlist test; precondition still `statFile` only; RED LINE in proposal |
| Round-trip drift on `repo`/`ref` (esp. empty `ref`) | Low | ADR-12 omit-when-empty + DeepEqual round-trip test |
| validate/sync disagree on external→custom | Low | ADR-13 single `manifestTagFor` helper consumed by both paths; one Diff test covers both |
| Lone `--ref` silently dropped on custom add | Low | ADR-14 `AddCore` rejects `--ref` without `--repo` loudly |
| Existing `AddEntry(reg, id)` call sites break on signature change | Low (compile-time) | Mechanical update to `AddEntry(reg, id, "", "")`; caught immediately by the compiler/tests |
| URL with space or unrepresentable byte | Low | `checkRepresentable` extended to repo/ref; `needsQuote` already handles quoting |

## Non-Goals (explicit)

- NO fetch / clone / download / network / subprocess / git-library code — none, ever.
- NO `pinned-ref` auto-update, NO `vendor-skills/` auto-importer, NO signature verification.
- NO touching vendored `bin/overlay`; global `apply`/`capture` unchanged.
- `external` is inert provenance metadata only; the tool acts solely on local committed files.

## Affected Files (edit points)

| File | Change |
|------|--------|
| `engine/skills/types.go` | `Source` gains `Repo`, `Ref` strings |
| `engine/skills/parse.go` | `validSourceTypes` += external; `parseSource` += repo/ref cases; `validateEntry` += external rules + forbid-on-core/custom |
| `engine/skills/serialize.go` | emit repo/ref after type (omit empty); extend `checkRepresentable` |
| `engine/skills/validate.go` | `tagMatchesSourceType` delegates to `manifestTagFor` |
| `engine/skills/sync.go` | `registryTag` delegates to `manifestTagFor` (new shared helper lives here or validate.go) |
| `engine/skills/lifecycle.go` | `parseFlags` += `--repo`/`--ref`; `AddEntry` signature + branch; `AddCore` passes through + `--ref`-needs-`--repo` guard |
| `engine/skills/zero_fetch_test.go` | NEW static import-allowlist guard test |
| `bin/labdrian-overlay` | usage line: `add <id> [--repo <url>] [--ref <sha>]` (args already pass through) |
| `README` / skill doc | one note: external = vendored-and-reviewed, tool never fetches |
```

