# Proposal: skill-external-provenance (Issue #29, slice 5)

## Intent

Issue #29 originally proposed `source: external` backed by a git FETCH (`add <git-url>` pulling third-party code). The platform owner REJECTED auto-fetch: a SKILL.md is INSTRUCTIONS the LLM obeys, so auto-fetching + auto-trusting third-party skills is an unacceptable supply-chain / prompt-injection surface. This slice ships the SAFE reinterpretation: a human manually reviews and VENDORS an external skill (copies files in, reads the SKILL.md, commits) under the same trust model as any committed code. The registry merely RECORDS PROVENANCE (origin URL + vendored ref) for audit/traceability. The tool only ever acts on local, committed, reviewed files. ZERO fetch/network/git execution.

## Scope

### In Scope
- Schema: `source.type` accepts `core | custom | external`; `external` adds inert descriptive `source.repo` (origin URL) and `source.ref` (vendored commit/ref) strings.
- Parser: accept + validate `repo`/`ref`; serializer round-trips them.
- Validation: `external` requires `repo`; `ref` recommended; `repo`/`ref` INVALID on `core`/`custom` (fail-loud, mirroring `allowedProjects`-on-global rejection). `core` still requires upstream.
- Manifest mapping: `external` skills map to manifest tag `custom` (vendored, labdrian-owned, not upstream-synced) in validate/sync.
- `skills add` gains optional `--repo <url>` / `--ref <sha>`; with `--repo` the new entry is `external` with provenance recorded. Without them, `add` stays `custom` (current behavior). `add` STILL only registers an already-present local `skills/<id>/` dir — it never fetches.
- Docs: short note that external = vendored-and-reviewed, the tool never fetches.

### Out of Scope
- Any fetch/clone/update/network/subprocess mechanism.
- `pinned-ref` auto-update; `vendor-skills/` auto-importer; signature verification.
- Touching vendored `bin/overlay`; global apply/capture stays unchanged.

## Capabilities

### New Capabilities
None.

### Modified Capabilities
- `skill-package-manager`: registry schema + parser/serializer/validate add the `external` source type and inert `repo`/`ref` provenance fields, plus the external→custom manifest tag mapping.
- `skill-lifecycle`: `skills add` accepts optional `--repo`/`--ref` to register an existing local dir as `external` with recorded provenance.

## Approach

Pure additive metadata. Extend `Source` struct with `Repo`/`Ref` strings; add `external` to `validSourceTypes`; extend `parseSource`, `serialize`, and `validateEntry`. Map `external`→`custom` in `tagMatchesSourceType`. Extend `AddEntry`/`AddCore`/flag parsing with optional provenance; `--repo` present ⇒ `external`. No new imports — a reviewer must confirm no `net/http`, `os/exec`, or git import was added.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `engine/skills/types.go` | Modified | Add `Repo`/`Ref` to `Source` |
| `engine/skills/parse.go` | Modified | Parse + validate `external`, `repo`, `ref` |
| `engine/skills/serialize.go` | Modified | Round-trip `repo`/`ref` |
| `engine/skills/validate.go` | Modified | `external`→`custom` tag mapping |
| `engine/skills/lifecycle.go` | Modified | `add --repo/--ref` ⇒ external entry |
| `bin/labdrian-overlay` | Modified | Pass `--repo`/`--ref`; usage doc |
| `README`/skill doc | Modified | Vendored-not-fetched note |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Reader fears hidden fetch logic | Med | RED LINE: zero net/exec/git imports; reviewer-verifiable; tests assert no fetch |
| Round-trip drift on `repo`/`ref` | Low | Serialize↔parse round-trip test for external entries |
| validate/sync misalign external | Low | external→custom mapping covered by Diff tests |

## Rollback Plan

Revert the change commits/PR. Schema additions are additive and optional; no existing `core`/`custom` entry changes, so reverting leaves `skills.registry.yaml` and `overlay.manifest` valid and unchanged.

## Dependencies

Builds on merged slices 1-4 (registry parser/serializer, validate/sync, `skills add`). No external dependencies.

## Success Criteria

- [ ] Registry round-trips an `external` entry with `repo`/`ref` (serialize↔parse).
- [ ] `repo`/`ref` on `core`/`custom` fail loud; `external` without `repo` fails loud.
- [ ] `validate` aligns an `external` entry against a `custom` manifest tag.
- [ ] `skills add <id> --repo <url> [--ref <sha>]` registers an existing local dir as `external`; without flags stays `custom`; neither variant fetches.
- [ ] No `net/http`, `os/exec`, or git import added anywhere in the diff.
- [ ] Strict-TDD Go tests pass; `bin/overlay` and global apply/capture untouched.
