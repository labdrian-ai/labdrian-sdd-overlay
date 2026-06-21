# Minimalism Contract — Pre-Activation Baseline

> **Purpose**: Provide a measurable "before" reference so AC-4 (the minimalism contract reduces LOC added and over-engineering per change) can be evaluated after the contract is active. All measurements are read-only samples from existing git history.
>
> **Captured**: 2026-06-21
> **Contract activation branch**: `feature/minimalism-contract-activation`

---

## 1. Metric Definitions

| Metric | Definition | Source | Determinism |
|--------|-----------|--------|-------------|
| **LOC added** | `git show --numstat` additions (`$1`) summed across all files in a commit | `git log --numstat` | Deterministic — exact count |
| **LOC deleted** | `git show --numstat` deletions (`$2`) summed | `git log --numstat` | Deterministic — exact count |
| **Net LOC** | LOC added minus LOC deleted | Derived | Deterministic |
| **New deps** | Number of new package entries in `package.json`, `requirements.txt`, `go.mod`, or equivalent manifest files (lines prefixed `+` in the dep section, excluding version bumps) | `git show -- <manifest>` | Deterministic |
| **Single-caller abstractions** | New files or functions introduced where a single call site exists at commit time and a simpler inline alternative would have served. Scored as a count of candidate files/constructs. | Manual inspection of new-file list and call-graph evidence | **Judgment metric** — not deterministic; annotated best-effort |

> **Judgment metric caveat**: The single-caller abstraction count is inherently subjective and probabilistic when assessed by an AI reviewer. It is a signal, not a measurement. Two reviewers may score the same change ±1. Use it directionally, not for pass/fail.

---

## 2. Sampled Repos

| Repo | Path | Status |
|------|------|--------|
| genesis | `/home/labdrian/genesis` | Sampled |
| empresa-servicios | `/home/labdrian/empresa-servicios` | Sampled |
| etl_pipeline | `/home/labdrian/etl_pipeline` | Sampled |
| genesis-cutover | `/home/labdrian/genesis-cutover` | **Skipped** — directory does not exist as a git repo |
| labdrian-sdd-overlay | `/home/labdrian/labdrian-sdd-overlay` | Sampled (reference only — this overlay, not a consumer) |

---

## 3. Before-Activation Sample Data

### 3.1 genesis (TypeScript / Node monorepo — SDD-governed)

Five most recent non-merge commits sampled.

| # | Commit | Description | LOC Added | LOC Deleted | Net LOC | New Deps | Single-caller abstractions |
|---|--------|-------------|-----------|-------------|---------|----------|---------------------------|
| G-1 | `ad36c817` | chore(openspec): archive service-desk-inline-image-alias | 263 | 0 | +263 | 0 | 0 (spec archive — docs only) |
| G-2 | `6a2f36d6` | test(service-desk): tighten alias form-submit assertion | 38 | 19 | +19 | 0 | 0 (test tightening) |
| G-3 | `bc8db188` | refactor(service-desk): remove dead makeResolvedToken | 8 | 11 | −3 | 0 | −1 (removed dead abstraction) |
| G-4 | `4e079d59` | fix(service-desk): canonicalize aliases at modal-save | 234 | 6 | +228 | 0 | 1 (new `propuesta/page.test.tsx` guards a single page) |
| G-5 | `f6a8c8d8` | feat(service-desk): expand image aliases at all submit seams | 295 | 36 | +259 | 0 | 0 (logic spread across real call sites, not abstracted away) |

**genesis averages**: LOC added per change = **168**, Net LOC = **+153**, New deps = **0**, Single-caller abstractions = **~0.2**

> Note: Commit G-1 (`+263`) is a documentation-only archive operation. Excluding it, the feature/fix average is **+144 LOC added**, **+126 net**.
>
> Additional context: a nearby commit `7d1729ee` (notification delivery platform) added +2108 LOC with 1 new dep (`web-push`) and 26 new files for a full backend module. This is a clear feature build-out — included here for context but not in the primary 5-sample window since it precedes the inline-image feature arc.

---

### 3.2 empresa-servicios (Next.js marketing site — not SDD-governed)

Five most recent commits sampled.

| # | Commit | Description | LOC Added | LOC Deleted | Net LOC | New Deps | Single-caller abstractions |
|---|--------|-------------|-----------|-------------|---------|----------|---------------------------|
| E-1 | `4c36916` | security: ensure .env files properly gitignored | 7 | 1 | +6 | 0 | 0 |
| E-2 | `0cede78` | feat: integrate Apollo.io website tracker via next/script | 28 | 0 | +28 | 0 (used next/script already installed; tracker injected via inline `<Script>`) | 1 (inline script in layout.tsx — a direct embed, no new abstraction layer) |
| E-3 | `28c2d09` | fix: redirect primary CTA | 4 | 7 | −3 | 0 | 0 |
| E-4 | `1e2a0b3` | fix(layers): add text-white to section | 1 | 1 | 0 | 0 | 0 |
| E-5 | `c3c7843` | fix(globals): dark-section uses !important | 2 | 2 | 0 | 0 | 0 |

**empresa-servicios averages**: LOC added per change = **8**, Net LOC = **+6**, New deps = **0**, Single-caller abstractions = **~0.2**

> Context: This is a small marketing site. Changes are micro-edits and CSS fixes. The Apollo tracker commit (E-2) integrated a third-party script directly into `layout.tsx` without creating a wrapper component or custom hook — borderline but defensible for a single tracker.

---

### 3.3 etl_pipeline (Python ETL — not SDD-governed)

Five most recent substantive commits sampled (merge commit `6e58d88` excluded).

| # | Commit | Description | LOC Added | LOC Deleted | Net LOC | New Deps | Single-caller abstractions |
|---|--------|-------------|-----------|-------------|---------|----------|---------------------------|
| P-1 | `5007ef4` | feat(pipeline): batch execution, parity check, full-reload | 165 | 68 | +97 | 0 | 0 (new `dbmaker_repository.py` is called from orchestrator) |
| P-2 | `12101e1` | agregar querys de captura | 97 | 61 | +36 | 0 | 0 |
| P-3 | `fc73e54` | fix paginacion y manejo de tablas vivas | 611 | 187 | +424 | 0 | **2** (>55 strategy files each modified ~8–10 lines for 2-line fix; the strategy pattern itself is a pre-existing abstraction applied across all tables — the fix was correct but mechanical replication across 55+ files is a signal) |
| P-4 | `1847f50` | cambios para delete | 1364 | 277 | +1087 | 0 | **2** (same 55+ strategy file pattern for delete support; each file is ~20 lines for a feature that could have been a base-class method) |
| P-5 | `03cd206` | add strat para dw_bolelectr | 103 | 3 | +100 | 0 | 1 (new strategy file follows established pattern — acceptable) |

**etl_pipeline averages**: LOC added per change = **468**, Net LOC = **+349**, New deps = **0**, Single-caller abstractions = **~1.0**

> Context: etl_pipeline shows the highest LOC-per-change by far, driven by commits P-3 and P-4 where a single behavioral change (pagination fix; delete support) was replicated mechanically across 55+ strategy files. This is a pre-existing architectural decision (Strategy pattern without a shared base class) rather than per-commit over-engineering, but it is measurable and the minimalism contract should flag it. No new runtime dependencies were introduced across all sampled commits.

---

### 3.4 labdrian-sdd-overlay (Go — reference only, not a consumer project)

Five most recent commits sampled for reference.

| # | Commit | Description | LOC Added | LOC Deleted | Net LOC | New Deps | Single-caller abstractions |
|---|--------|-------------|-----------|-------------|---------|----------|---------------------------|
| O-1 | `5eafe3d` | ci: enforce engine test suite on push/PR | 31 | 0 | +31 | 0 | 0 |
| O-2 | `6f5bb4d` | fix(settings): close null-array and relative-registry warnings | 208 | 2 | +206 | 0 | 0 |
| O-3 | `332189b` | feat(overlay): wire deterministic scoping hooks | 994 | 11 | +983 | 0 | 0 (new settings + wiring with tests) |
| O-4 | `d637dd1` | test(engine/cmd): tighten TC-CLI-3 and error branch | 52 | 7 | +45 | 0 | 0 |
| O-5 | `8677a84` | test(engine): close adversarial-review gaps | 516 | 22 | +494 | 0 | 0 |

**overlay averages**: LOC added per change = **360**, Net LOC = **+352**, New deps = **0**, Single-caller abstractions = **0**

> Context: The overlay is under active SDD-governed development with TDD. LOC counts are higher but dominated by test files (O-3 and O-5 are primarily tests). The minimalism contract has not been active yet on this repo.

---

## 4. Summary Table

| Repo | Avg LOC Added / change | Avg Net LOC / change | Avg New Deps / change | Avg Single-caller abstractions / change | SDD-governed |
|------|----------------------|---------------------|----------------------|----------------------------------------|-------------|
| genesis | 168 | +153 | 0 | 0.2 | Yes |
| empresa-servicios | 8 | +6 | 0 | 0.2 | No |
| etl_pipeline | 468 | +349 | 0 | 1.0 | No |
| overlay (ref) | 360 | +352 | 0 | 0 | Yes (in progress) |

---

## 5. Measurement Protocol Going Forward

### 5.1 When to Capture

After every substantive commit or merge PR that touches production or pipeline code (not docs-only, not reformats), capture the following within the same session or PR review.

### 5.2 What to Capture Per Change

Run these commands (read-only) against the commit SHA or PR merge commit:

```bash
# LOC added, deleted, net
git show --numstat <sha> | awk 'NF==3 {added+=$1; del+=$2} END {print "added="added, "deleted="del, "net="added-del}'

# New deps (package.json example — adapt per manifest type)
git show <sha> -- package.json | grep '^+' | grep -v '^+++' | grep -v '"version"' | grep -E '"[^"]+": "[\^~]?' | wc -l

# New files count
git show --numstat <sha> | awk '$1!="0" && $2=="0"' | wc -l
```

For single-caller abstractions: inspect the new-files list. Flag any new file whose name implies a single concept (e.g., `use-X.ts`, `X-service.ts`, `X-repository.ts`) and verify it has more than one call site in the same commit. Record count of files with exactly one call site.

### 5.3 How to Compare (AC-4 Evaluation Criteria)

Collect the same 5-metric tuple for the next 3–5 changes made **after** the minimalism contract is active in the relevant repo.

**Success signal (AC-4 met)**:
- Average LOC added per change trends down or stays flat with documented behavioral justification.
- Average new deps per change = 0 for changes where deps were not architecturally necessary.
- Single-caller abstractions per change trends toward 0 or is accompanied by a documented justification in the commit/PR body.

**Failure signal (AC-4 not working)**:
- LOC added per change is flat or rising with no documented scope justification.
- New single-caller abstractions appear without justification in commit messages.
- Dependency count grows per change without a documented architectural decision.

**Comparison format** (add to this file or a `baseline-after.md`):

```
| Repo | Change | LOC Added | Net LOC | New Deps | Single-caller abstractions | Notes |
|------|--------|-----------|---------|----------|---------------------------|-------|
| genesis | <sha> | <n> | <n> | <n> | <n> | contract active |
```

Compare each after-row to the per-repo "before" averages in Section 4.

### 5.4 Determinism Boundary

| Environment | Behavior |
|-------------|----------|
| Claude Code (direct) | Contract rules enforced deterministically per session; Claude will flag and refuse tasks violating minimalism rules. |
| OpenCode / Codex (background agents) | Probabilistic — model may not consistently apply the contract without explicit prompt injection. |
| CI gate (overlay engine) | Deterministic — the engine gate enforces contract rules via hook at `sdd-tasks`/`sdd-apply` phases when the overlay is installed. |

---

## 6. Honest Limitations

1. **Small sample size**: 5 commits per repo is a weak statistical basis. The numbers are directionally useful but should not be treated as stable means until 20+ changes are sampled.

2. **Judgment metric is not reproducible**: The single-caller abstraction count will vary between reviewers and sessions. It is a qualitative signal, not a KPI.

3. **Repo heterogeneity**: genesis (TypeScript monorepo with SDD) and etl_pipeline (Python scripts without SDD) are not comparable on raw LOC. Compare each repo only to its own post-activation numbers.

4. **Skewed commits**: etl_pipeline's P-3 and P-4 are outliers driven by a pre-existing architectural pattern (55+ strategy files), not per-change over-engineering. Removing them gives an average of +78 LOC added — more representative of normal changes.

5. **Tests inflate LOC counts**: In the overlay and genesis, a large fraction of "LOC added" is test code. This is healthy and should not be penalized. Future measurements should optionally separate `src/` lines from `test/` lines.

6. **empresa-servicios baseline is effectively zero-noise**: With avg +8 LOC per change, this repo's changes are too small to show meaningful contract impact. It will only matter if a larger feature is built.

7. **No dep changes observed**: Zero new runtime dependencies were introduced across all 20 sampled commits. This means the "new deps" metric has no before-signal to compare against. AC-4 applicability for deps will only be visible if a future change introduces one.
