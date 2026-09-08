package query

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/embed"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vecindex"
)

// EmbedFunc embeds one query string into a vector for the embedding arm
// (R-058). Production wires embed.Client.Embed; it is invoked only when a
// caller actually names engram-embed in Sources, so a query that never
// asks for it never reaches the network (R-071).
type EmbedFunc func(ctx context.Context, text string) ([]float32, error)

// BuildIndexFunc incrementally (re)builds one project's embedding index
// under an explicit model/dimension/inputLimit contract (Deps.BuildIndex's
// own doc comment explains why the contract is passed in rather than
// assumed). Production wires it to read the project's live rows
// (engram.Store.ListObservations, the same source cmd_index_embeddings.go
// already reads) and call vecindex.Build.
type BuildIndexFunc func(ctx context.Context, project, model string, dimension, inputLimit int) error

// TopUpMaxRows bounds runEmbeddingArm's inline top-up: a gap of at most
// this many unindexed rows is closed with one incremental vecindex.Build
// call before scanning; a larger gap is left to `longterm-mem index
// --embeddings`, exactly as before this change.
//
// The bound is measured, not guessed (issue #286, this repository,
// 2026-09-08): a single embedding request costs ~50ms for a short row and
// ~350ms at the 2000-character input limit (internal/vecindex's own
// DefaultInputLimit), and the backend serializes requests rather than
// batching or parallelizing them usefully. 25 rows at the worst-case
// 350ms/row is therefore under 9 seconds -- one query call's worth of
// added latency, paid once to close a small gap, never accumulating
// because the gap it is bounded against is exactly the amount left after
// the top-up itself just closed it.
const TopUpMaxRows = 25

// Coverage reports how much of a source-backed index a query actually saw
// (design: "coverage is a response field, not a diagnostic"). It is a
// caller-facing statement of fact, not a diagnostic a caller may not
// iterate: an absent field and "the index is complete" must never look
// alike, which is why Result.Coverage carries no `omitempty`.
type Coverage struct {
	// Source names the sourced index this entry describes, e.g.
	// "engram-embed". Only a source that depends on an index this module
	// owns gets an entry; FTS gets none, since observations_fts is
	// Engram's own and reporting a count for it would report an
	// assumption in the shape of a measurement.
	Source string `json:"source"`
	// Live is how many of the project's observations are not soft-deleted
	// right now (R-020's own scoping).
	Live int `json:"live"`
	// Indexed is how many of those live observations the on-disk index
	// currently vouches for: a manifest entry naming a live id whose
	// fingerprint is not evaluated here (a stale entry is still counted
	// as "indexed" for this field; the embedding arm itself drops it at
	// query time and that drop is not reflected in this count, which
	// describes what the index CLAIMS to cover, not what it has just
	// proven current).
	Indexed int `json:"indexed"`
	// Unindexed is Live minus Indexed, precomputed rather than left for a
	// caller to subtract: a caller who forgets the subtraction gets a
	// silent wrong answer, and that is this module's unforgivable
	// failure.
	Unindexed int `json:"unindexed"`
	// BuiltAt is the index's own recorded build time (RFC3339), or the
	// literal "never" when no index has ever been built for this project.
	BuiltAt string `json:"built_at"`
}

// embeddingIndexNeverBuilt is Coverage.BuiltAt's value when no index
// exists yet -- distinct from an empty string, which could be mistaken for
// a genuine (if malformed) timestamp rather than an honest "never".
const embeddingIndexNeverBuilt = "never"

// runEmbeddingArm retrieves the embedding source's own top-`top` rows
// (R-058), in its own native cosine-rank order, alongside this request's
// Coverage entry (design) and any degradation diagnostics (R-070).
//
// It never resolves a candidate id through engram.Store.ObservationByID:
// that method deliberately returns a soft-deleted row (store.go's own doc
// comment), and reusing it here would let an observation deleted after the
// index was built surface from a months-old index -- R-020 defeated by
// convenience. Every candidate is resolved through CoverageSnapshot
// instead, which simply omits a row that is missing, soft-deleted, or
// belongs to another project.
func runEmbeddingArm(ctx context.Context, store *engram.Store, stateDir, project, queryText string, top int, embedFn EmbedFunc, buildIndex BuildIndexFunc) ([]ResultRow, Coverage, []Diagnostic) {
	coverage := Coverage{Source: SourceEngramEmbed, BuiltAt: embeddingIndexNeverBuilt}
	dir := vecindex.Dir(stateDir, project)

	// The index is loaded FIRST so its manifest ids can go into the same
	// read that counts live observations. Coverage's two numbers used to
	// come from two separate calls, which is what let them disagree and
	// forced a clamp, a diagnostic and a branch no test could reach.
	idx, loadErr := vecindex.Load(dir)

	var diags []Diagnostic
	live, liveByID, err := store.CoverageSnapshot(project, manifestEngramIDs(idx))
	if err != nil {
		// One failure, named once. Coverage's counts stay zero and the
		// diagnostic says they are not measurements -- the previous shape
		// discarded the lookup's error into an empty map, which reported
		// Indexed as 0 and so every live observation as unindexed, telling
		// the operator to rebuild an index that was never the problem.
		return nil, coverage, append(diags, Diagnostic{
			Code:   DiagnosticCoverageUnreadable,
			Detail: fmt.Sprintf("could not read project %q's coverage from engram, so the live and indexed counts below are zeros, not measurements: %v", project, err),
		})
	}
	coverage.Live = live

	if loadErr != nil {
		// ErrNoIndex (nothing built yet) and ErrCorrupted (a corrupted
		// on-disk index) both degrade to "no rows, coverage says so" --
		// neither is a call failure, since the union's @5 guarantee is
		// unaffected by one arm having nothing to contribute (R-058).
		coverage.Unindexed = coverage.Live
		return nil, coverage, diags
	}
	coverage.BuiltAt = idx.Manifest.BuiltAt
	coverage.Indexed = len(liveByID)
	// No clamp, and none needed: both counts came from one snapshot, in
	// which every indexed row is by definition one of the live rows
	// counted, so this subtraction cannot go negative.
	coverage.Unindexed = coverage.Live - coverage.Indexed

	if coverage.Unindexed > 0 && coverage.Unindexed <= TopUpMaxRows && buildIndex != nil {
		idx, coverage, liveByID, diags = topUpEmbeddingIndex(ctx, store, dir, project, idx, coverage, liveByID, buildIndex, diags)
	}

	if embedFn == nil {
		return nil, coverage, diags
	}
	queryVector, embedErr := embedFn(ctx, queryText)
	if embedErr != nil {
		return nil, coverage, append(diags, embeddingDegradationDiagnostic(embedErr))
	}

	type scoredCandidate struct {
		score float64
		entry vecindex.ManifestEntry
	}
	candidates := make([]scoredCandidate, len(idx.Manifest.Entries))
	for i, e := range idx.Manifest.Entries {
		candidates[i] = scoredCandidate{score: cosineSimilarity(queryVector, idx.Vectors[i]), entry: e}
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })

	var rows []ResultRow
	for _, c := range candidates {
		if len(rows) >= top {
			break
		}
		obs, ok := liveByID[c.entry.EngramID]
		if !ok {
			continue
		}
		if vecindex.Fingerprint(idx.Manifest.Model, idx.Manifest.Dimension, idx.Manifest.InputLimit, obs.Title, obs.Content) != c.entry.Fingerprint {
			// A stale entry: the observation's title or content changed
			// since it was embedded (R-069). Serving it would show a
			// snippet under a vector that no longer describes it, so it
			// is dropped rather than served under a false pretense.
			continue
		}
		rows = append(rows, ResultRow{
			EngramID: obs.ID, Title: obs.Title, Content: obs.Content, FullLength: len(obs.Content),
		})
	}
	return rows, coverage, diags
}

// manifestEngramIDs collects idx's own manifest entry ids, or nil when idx
// is nil (Load failed): store.CoverageSnapshot treats a nil/empty id slice
// as "nothing is indexed yet", which is exactly true when there is no
// index to read ids from.
func manifestEngramIDs(idx *vecindex.Index) []int64 {
	if idx == nil {
		return nil
	}
	ids := make([]int64, len(idx.Manifest.Entries))
	for i, e := range idx.Manifest.Entries {
		ids[i] = e.EngramID
	}
	return ids
}

// topUpEmbeddingIndex closes a small, bounded gap between dir's manifest
// and project's live corpus (runEmbeddingArm has already checked the gap
// is within TopUpMaxRows) by running one incremental build BEFORE the
// caller scans, so this query's own results already reflect it.
//
// A build failure degrades exactly as an untopped-up stale index already
// does -- the query never fails because a top-up could not run -- plus a
// diagnostic naming what went wrong, appended to diags and returned
// alongside the unchanged idx/coverage/liveByID the caller already had.
func topUpEmbeddingIndex(ctx context.Context, store *engram.Store, dir, project string, idx *vecindex.Index, coverage Coverage, liveByID map[int64]engram.Observation, buildIndex BuildIndexFunc, diags []Diagnostic) (*vecindex.Index, Coverage, map[int64]engram.Observation, []Diagnostic) {
	toppedUp := coverage.Unindexed
	if err := buildIndex(ctx, project, idx.Manifest.Model, idx.Manifest.Dimension, idx.Manifest.InputLimit); err != nil {
		diags = append(diags, Diagnostic{
			Code:   DiagnosticEmbeddingTopUpFailed,
			Detail: fmt.Sprintf("tried to embed %d unindexed rows before answering this query, but the build failed, so this answer still comes from the index as it stood before: %v", toppedUp, err),
		})
		return idx, coverage, liveByID, diags
	}

	reloaded, err := vecindex.Load(dir)
	if err != nil {
		// The build itself reported success but the index it wrote cannot
		// be read back -- treated the same as a build failure: the query
		// still answers from what it had, and the diagnostic names the
		// (now stranger) reason why the top-up did not take effect.
		diags = append(diags, Diagnostic{
			Code:   DiagnosticEmbeddingTopUpFailed,
			Detail: fmt.Sprintf("a top-up build for %d unindexed rows reported success, but the resulting index could not be reloaded: %v", toppedUp, err),
		})
		return idx, coverage, liveByID, diags
	}

	live, newLiveByID, err := store.CoverageSnapshot(project, manifestEngramIDs(reloaded))
	if err != nil {
		diags = append(diags, Diagnostic{
			Code:   DiagnosticCoverageUnreadable,
			Detail: fmt.Sprintf("could not re-read project %q's coverage after a top-up build, so the counts below describe the index before it: %v", project, err),
		})
		return idx, coverage, liveByID, diags
	}

	coverage.BuiltAt = reloaded.Manifest.BuiltAt
	coverage.Live = live
	coverage.Indexed = len(newLiveByID)
	coverage.Unindexed = coverage.Live - coverage.Indexed
	diags = append(diags, Diagnostic{
		Code:   DiagnosticEmbeddingToppedUp,
		Detail: fmt.Sprintf("%d previously unindexed rows were embedded and folded into the index before answering this query", toppedUp),
	})
	return reloaded, coverage, newLiveByID, diags
}

// embeddingDegradationDiagnostic names which of the two conditions R-070
// requires distinctly: the backend never answered at all, or it answered
// but the configured model is not pulled. Both collapse to "no embedding
// rows this call" for a caller who does not read diagnostics; this is the
// one place that says which happened.
func embeddingDegradationDiagnostic(err error) Diagnostic {
	var modelMissing *embed.ModelMissingError
	if errors.As(err, &modelMissing) {
		return Diagnostic{Code: DiagnosticEmbeddingModelMissing, Detail: modelMissing.Error()}
	}
	var unreachable *embed.BackendUnreachableError
	if errors.As(err, &unreachable) {
		return Diagnostic{Code: DiagnosticEmbeddingBackendUnreachable, Detail: unreachable.Error()}
	}
	// An error neither typed error names (e.g. a redirect refusal, or a
	// non-loopback endpoint refused at construction) is still a backend
	// that did not produce a vector -- reported under the same code rather
	// than invented a third one for a case R-070 does not distinguish.
	return Diagnostic{Code: DiagnosticEmbeddingBackendUnreachable, Detail: err.Error()}
}

// coverageIncompleteDiagnostic names the exact rebuild command a caller
// needs when Coverage says the index is behind the live corpus, rather
// than leaving it to be inferred from a bare count (design's own table:
// "the fix is `longterm-mem index --embeddings`, named verbatim in the
// detail"). It returns nil when there is nothing incomplete to report.
func coverageIncompleteDiagnostic(c Coverage) *Diagnostic {
	if c.BuiltAt == embeddingIndexNeverBuilt {
		return &Diagnostic{
			Code:   DiagnosticEmbeddingIndexIncomplete,
			Detail: "no embedding index has ever been built for this project, so paraphrase questions are unanswerable from it right now; run `longterm-mem index --embeddings` to build one",
		}
	}
	if c.Unindexed > 0 {
		return &Diagnostic{
			Code:   DiagnosticEmbeddingIndexIncomplete,
			Detail: fmt.Sprintf("%d of %d live observations are not in the embedding index, so a thin or empty paraphrase result is not evidence of absence; run `longterm-mem index --embeddings` to rebuild it", c.Unindexed, c.Live),
		}
	}
	return nil
}

// cosineSimilarity returns the cosine similarity of a and b. It returns 0
// when either vector has zero magnitude rather than dividing by zero; a
// production embedding is never the zero vector, so this only guards
// malformed input, never a real degradation path.
func cosineSimilarity(a, b []float32) float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var dot, normA, normB float64
	for i := 0; i < n; i++ {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
