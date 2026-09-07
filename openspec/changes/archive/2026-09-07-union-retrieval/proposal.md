# Proposal: Union Retrieval Over Engram's Own Rows

Claim tags: **[M]** measured, **[C]** read from code, **[A]** assumed. Sources:
`openspec/decisions/vault-value.md`, `openspec/decisions/union-retrieval.md`.

## Intent

`query` runs two retrievers, and the semantic one is pointed at the wrong corpus:
it indexes 49 pages of upstream `claude-obsidian` documentation **[M]**, not this
human's 586 observations. Meanwhile trigram FTS is perfect on identifiers and
near-useless on paraphrase, and embeddings are the exact inverse **[M]**:

| class | trigram FTS | embeddings | FTS+rerank | **union** |
|---|---|---|---|---|
| identifier, single token (n=14) | 93/100 | 0/7 | 93/100 | **93/100** |
| identifier, multi-token (n=16) | 88/94 | 0/6 | 50/56 | **88/94** |
| paraphrase (n=10) | 10/10 | 40/70 | 30/30 | **40/70** |

hit@1 / hit@5. The union matches the better arm in **every** class **[M]**.

Success: an embedding index over Engram rows, merged with FTS so the result set
contains both arms' top-5 — a set-union property, not an experimental result.

## Scope

### In Scope

- **Slice 1 — merge.** Amend R-006's ordering; round-robin interleave with each
  source's native order preserved as a subsequence; dedup by `engram_id`;
  `sources` opt-in (default `engram-fts`, `engram-embed`); budget allocated
  before snippets render; quota-aware `capResponse`.
- **Slice 2 — index.** `internal/embed` (HTTP client + egress guard),
  `internal/vecindex` (fingerprints, incremental build), explicit
  `index --embeddings` pass, `doctor`/`status` wiring, the embedding arm.
- Coverage and named degradation in **every response**.

### Out of Scope

- Deleting the vault. It is retired from the default path, still reachable by
  name; deletion is a later decision taken after the union is measured live.
- `shared-project-vault` (frozen; do not touch).
- **Abstention / relevance floor.** Enabled by cosine, deliberately deferred: a
  threshold fitted to 24 queries fails as silent absence.
- ANN index. Break-even is ~50k rows **[A]**; the corpus is 586.
- Background or lazy indexing. No file locking exists anywhere in the module **[C]**.

## Capabilities

### New Capabilities
- `longterm-mem-embedding-index`: index location and format, fingerprint
  staleness, coverage reporting, build/incremental lifecycle, network egress
  boundary, named degradation codes.

### Modified Capabilities
- `longterm-mem-query`: R-006 ordering (below), `sources` parameter, per-response
  coverage, budget-before-render snippet allocation, unbiased cap.
- `longterm-mem-ops`: `doctor` checks and `status` fields for the new index.

## The spec amendment

`openspec/specs/longterm-mem-query/spec.md:67` mandates verbatim that results
appear as "vault matches (in vault rank order) followed by" Engram matches. With
`top` per source **[C]** and `capResponse` trimming from the end **[C]**, the
vault permanently owns ranks 1–5 and every row lost to budget pressure is
Engram's. `mergeResults` is not misbehaving — it implements R-006 faithfully.
**The defect is in the requirement.**

R-006 becomes: results are merged across requested sources by deterministic
round-robin, each source's native rank order preserved as a subsequence, deduped
by `engram_id`, with rank 1 routed by the token-shape gate; the vault-first
clause narrows to "when the vault source is requested". No cross-source score
arithmetic — D8 survives, restated as a testable invariant.

## Approach

| Decision | Rationale |
|---|---|
| Index at `<state-dir>/index/<project>/` | R-002 makes Engram's DB read-only **[C]**; the index is our state, in the existing `--state-dir` **[C]** |
| Fixed-stride blob + self-digesting JSON | 586 × 768 × 4 B = 1.79 MB; full scan is microseconds against a 0.137 s embed round trip **[M]** |
| Fingerprint over the exact embedded bytes, incl. model/dim/input_limit | `revision_count` is a *claim about* a row; the guarded failure is "bytes changed, metadata did not" |
| Rank-1 gate = identifier token shape **OR** FTS `MatchAll` | Fixed order fails one class **[M]**; degrades softly — a bad gate costs @1 only |

**Egress guard (new constraint this change introduces).** The module imports
`net/http` nowhere today, not even in tests **[C]**. R-021's exec allowlist does
not cover the network boundary. This change owes a mirror allowlist test, sibling
to `exec_allowlist_test.go`: exactly one file may import `net/http`; loopback by
default with **non-loopback refused, not warned**; every redirect refused;
explicit client timeout. Precedent: `rerank.py --allow-remote-ollama` **[C]**.

**Degradation.** Absent ollama and unpulled model are distinguished **by name**
(`embedding_backend_unreachable`, `embedding_model_missing`), and every detail
names the query consequence, not the system state. What must never happen is
FTS-only results that look like a working union — this module has just shipped a
fix for exactly that shape of bug **[C]**.

**Staleness.** Two checks: per-returned-row fingerprint verification (exact, drop
on mismatch) and corpus coverage `M − N_live` from one `COUNT(*)`. Coverage goes
in **every response**, not in a diagnostic nobody reads. This bit the measuring
harness itself: the corpus grew 584 → 586 mid-run and the index answered two rows
stale **[M]**.

**Budget.** Measured, the union sits at **7,060 bytes against 8,000 — 12%
headroom** **[M]**. One more diagnostic and `capResponse` starts silently
deleting the rows the union exists to deliver. Budget is allocated across rows
before snippets are rendered; `capResponse` remains a backstop and drops the
lowest-ranked row of whichever source holds the most slots.

## Affected Areas

| Area | Impact | Description |
|---|---|---|
| `openspec/specs/longterm-mem-query/spec.md` | Modified | R-006 ordering amendment |
| `internal/query/query.go` | Modified | merge, gate, `sources`, budget, cap, diagnostics |
| `internal/mcpserver/server.go` | Modified | `sources` field, mirroring `ExcludeTypes` **[C]** |
| `internal/embed/` | New | ollama HTTP client + egress guard + allowlist test |
| `internal/vecindex/` | New | fingerprints, manifest, incremental build |
| `internal/ops/{doctor,status}.go` | Modified | three checks, one `built_at` field |
| `cmd/longterm-mem/` | Modified | `index --embeddings` |

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Rank-1 gate is overfit (37 queries, 13 written after seeing 24 behave) | **High** | @5 guarantee is gate-independent. Blind-author 20+20 queries; below ~80% routing accuracy, ship FTS-first, take the paraphrase@1 loss, say so in the response |
| Byte ceiling deletes the union's own guarantee | Med | Budget-before-render; unbiased cap; measure real encoded responses before implementing |
| Non-loopback embedder egresses private memory | Low | Refuse, don't warn. Allowlist test as a regression gate |
| Union lands *between* the arms in a Go port | Low | Simulated as arm D and it did not **[M]**; port the harness to a golden test |
| Harness/production tokenizer drift | Med | `evaluate.py` uses a regex; production uses `strings.Fields` **[C]** — the Go port must use `strings.Fields` |

## What the validation does NOT establish

n was 14, 16 and 10. The paraphrase questions were written by the agent that ran
the experiment; lexical distance was verified, **relevance judgements were not
independently reviewed**. One project's corpus. Embeddings covered the first
2,000 characters, so long observations were judged on their openings. The gate is
fitted to 37 queries no human wrote. The 0.62 s/row embed cost was measured once.

## Review budget

Delivery strategy `ask-on-risk`, budget 800 changed lines. **It will bind.**
Slice 1 (~250–350 lines + tests) fits comfortably. Slice 2 (~500–700 lines +
tests, plus two new packages and a golden harness port) is expected to exceed 800
and should be planned as a chained PR — plausibly split again into
`internal/embed` + egress guard, then `internal/vecindex` + the arm.

## Rollback Plan

- Slice 1: revert the merge commit and the R-006 delta. `sources` is additive and
  `omitempty`; existing callers are unaffected while it is absent.
- Slice 2: revert, or set the default `sources` back to `engram-fts` — the
  embedding arm is opt-out by configuration without removing code.
- Index deletion is safe at any time: it holds no observation text, only ids,
  fingerprints and floats. Rebuild costs ~6 min for 586 rows **[M]**.
- The vault path is retired, not deleted, so it can be restored by default flip.

## Dependencies

- Local ollama with `nomic-embed-text` pulled. Optional at runtime by design:
  absent means named degradation, never silent FTS-only.
- No new module dependency. `net/http` is stdlib; no vector library is vendored.

## Success Criteria

- [ ] Union simulated as arm D reproduces `93/100 · 88/94 · 40/70` in a Go golden test
- [ ] Merged set ⊇ each requested arm's top-5, proven by a subsequence property test
- [ ] Gate scored on blind-authored queries; result published whichever way it lands
- [ ] R-006 amended; no shipped scenario left contradicting the new ordering
- [ ] `net/http` allowlist test fails when a second file imports it
- [ ] Non-loopback endpoint refused without explicit opt-in; redirects refused
- [ ] Every response carries coverage; absent backend and missing model are distinguished by name
- [ ] Encoded union response stays under 8,000 bytes with the full diagnostic set and `Standing` objects present
