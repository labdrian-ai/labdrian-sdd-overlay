package query

import (
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vecindex"

	_ "modernc.org/sqlite"
)

// unionFixturePath is the golden fixture (design: "the golden fixture must
// not call ollama"): a real, gzipped snapshot of this project's own Engram
// corpus at the time the published table was measured -- 584 observations'
// title/content/type, their nomic-embed-text vectors (the harness's
// title+newline+content shape, matching what produced the published
// numbers this test reproduces), the recorded live observations_fts DDL
// (trigram tokenizer, captured verbatim rather than assumed), and the 40
// authored identifier/paraphrase queries with their ground truth and their
// own pre-computed query embeddings (so no query embedding call is made at
// test time either).
const unionFixturePath = "testdata/union/fixture.json.gz"

// unionFixtureQuery is one query the fixture carries: its ground truth
// engram id(s) and its own pre-computed embedding vector.
type unionFixtureQuery struct {
	Query     string    `json:"query"`
	Truth     []int64   `json:"truth"`
	Kind      string    `json:"kind,omitempty"`
	Embedding []float32 `json:"embedding"`
}

// unionFixtureRow is one observation the fixture carries.
type unionFixtureRow struct {
	ID      int64  `json:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
	Type    string `json:"type"`
}

// unionFixture is the golden fixture's whole decoded shape.
type unionFixture struct {
	SchemaSQL string                         `json:"schema_sql"`
	Rows      []unionFixtureRow              `json:"rows"`
	Vectors   map[string][]float32           `json:"vectors"`
	Queries   map[string][]unionFixtureQuery `json:"queries"`
	// Blind is the frozen blind-authored gate-validation set
	// (openspec/changes/union-retrieval/validation/queries.json, "dropped"
	// entries already excluded), keyed by class ("identifier"/
	// "paraphrase"), each with its own pre-computed query embedding.
	Blind map[string][]unionFixtureQuery `json:"blind"`
}

// loadUnionFixture reads and gunzips unionFixturePath. It is skipped, not
// failed, when the fixture is absent -- the fixture is a large generated
// binary excluded from the review line count (per the phase contract's
// own rule), and a checkout that has not fetched it should not report a
// spurious failure on every other package's `go test ./...`.
func loadUnionFixture(t *testing.T) *unionFixture {
	t.Helper()
	f, err := os.Open(unionFixturePath)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skipf("golden fixture %s not present", unionFixturePath)
		}
		t.Fatalf("open %s: %v", unionFixturePath, err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader for %s: %v", unionFixturePath, err)
	}
	defer gz.Close()

	var fixture unionFixture
	if err := json.NewDecoder(gz).Decode(&fixture); err != nil {
		t.Fatalf("decode %s: %v", unionFixturePath, err)
	}
	return &fixture
}

// unionGoldenProject is the project name every golden-fixture row and
// query is scoped to inside the temp DB this test builds -- unrelated to
// any real project name; it exists only inside this test's own temp
// database.
const unionGoldenProject = "union-golden"

// buildUnionGoldenStore materializes fixture's rows into a fresh temp
// SQLite database created from fixture.SchemaSQL -- the recorded live
// schema, trigram tokenizer included, not the FTS5 default -- through a
// writable setup connection, then opens it via the real production
// engram.Open path so the golden test exercises the shipped Store.Search,
// not a re-implementation of it.
func buildUnionGoldenStore(t *testing.T, fixture *unionFixture) (*engram.Store, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "engram.db")
	setup, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open fixture setup connection: %v", err)
	}
	if _, err := setup.Exec(fixture.SchemaSQL); err != nil {
		setup.Close()
		t.Fatalf("apply fixture schema_sql: %v", err)
	}
	for _, r := range fixture.Rows {
		if _, err := setup.Exec(
			`INSERT INTO observations (id, session_id, type, title, content, project) VALUES (?, ?, ?, ?, ?, ?)`,
			r.ID, "sess-golden", r.Type, r.Title, r.Content, unionGoldenProject,
		); err != nil {
			setup.Close()
			t.Fatalf("insert fixture row %d: %v", r.ID, err)
		}
	}
	setup.Close()

	store, err := engram.Open(dbPath)
	if err != nil {
		t.Fatalf("engram.Open(%q): %v", dbPath, err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, dbPath
}

// buildUnionGoldenIndex builds an on-disk vecindex over every fixture row
// that carries a precomputed vector, under a fresh temp state directory,
// with each entry's fingerprint computed the same way runEmbeddingArm
// recomputes it at query time -- over the row's title/content as stored in
// the database above -- so a fingerprint check inside the query path
// passes for every indexed row.
//
// The fixture's rows are deliberately the FULL live corpus at capture time
// (592 rows, matching what the FTS arm's bm25 statistics were measured
// against), while its vectors cover only the subset that had been embedded
// (584) -- an honest partial-coverage index, not a smaller corpus: a
// fixture built from only the embedded subset was found, empirically, to
// shift bm25 rank order on close ties relative to the published
// measurement, because bm25 depends on the whole corpus's document
// statistics and the FTS arm evaluate.py measured always saw the full
// table.
func buildUnionGoldenIndex(t *testing.T, fixture *unionFixture) string {
	t.Helper()
	stateDir := t.TempDir()

	manifest := vecindex.Manifest{
		Model: vecindex.DefaultModel, Dimension: vecindex.DefaultDimension, InputLimit: vecindex.DefaultInputLimit,
		BuiltAt: "2026-01-01T00:00:00Z",
	}
	var vectors [][]float32
	for _, r := range fixture.Rows {
		vec, ok := fixture.Vectors[strconv.FormatInt(r.ID, 10)]
		if !ok {
			continue
		}
		if len(vec) != manifest.Dimension {
			t.Fatalf("row %d vector has dimension %d, want %d", r.ID, len(vec), manifest.Dimension)
		}
		fp := vecindex.Fingerprint(manifest.Model, manifest.Dimension, manifest.InputLimit, r.Title, r.Content)
		manifest.Entries = append(manifest.Entries, vecindex.ManifestEntry{EngramID: r.ID, Fingerprint: fp})
		vectors = append(vectors, vec)
	}
	if len(manifest.Entries) == 0 {
		t.Fatalf("fixture has no rows with a vector")
	}

	idx := &vecindex.Index{Manifest: manifest, Vectors: vectors}
	if err := idx.Save(vecindex.Dir(stateDir, unionGoldenProject)); err != nil {
		t.Fatalf("save golden index: %v", err)
	}
	return stateDir
}

// unionGoldenEmbedFunc answers embedFn with fixture's own pre-computed
// query embeddings, keyed by exact query text -- the golden test's own
// guarantee that it never reaches a network.
func unionGoldenEmbedFunc(fixture *unionFixture) EmbedFunc {
	byQuery := make(map[string][]float32)
	for _, items := range fixture.Queries {
		for _, q := range items {
			byQuery[q.Query] = q.Embedding
		}
	}
	return func(_ context.Context, text string) ([]float32, error) {
		vec, ok := byQuery[text]
		if !ok {
			return nil, fmt.Errorf("golden fixture: no cached embedding for query %q", text)
		}
		return vec, nil
	}
}

// TestUnionGoldenFixtureUsesLiveFTSSchema (design: "one fidelity trap the
// port must not fall into"): the fixture's recorded schema_sql must build
// an observations_fts using the trigram tokenizer, not FTS5's default
// (unicode61). A trigram index matches a substring inside a single word
// that was never its own token; the default tokenizer does not. This
// queries exactly such a substring and requires a hit -- a fixture built
// on the default tokenizer would report zero rows here, silently, rather
// than erroring, which is exactly the failure mode this test exists to
// catch before it reaches the numbers-reproduction test below.
func TestUnionGoldenFixtureUsesLiveFTSSchema(t *testing.T) {
	fixture := loadUnionFixture(t)
	if !strings.Contains(fixture.SchemaSQL, "tokenize='trigram'") {
		t.Fatalf("fixture schema_sql does not record the trigram tokenizer: %s", fixture.SchemaSQL)
	}

	store, _ := buildUnionGoldenStore(t, fixture)
	// "ActionKind" is one of the fixture's own known identifier titles
	// (identifier_single's first ground-truth row); "tionKi" is a byte
	// substring straddling its two halves that is never a whole token
	// under any word-based tokenizer. Only trigram indexing -- every
	// three-character window is its own token -- can match it.
	search, err := store.Search(unionGoldenProject, "tionKi", 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(search.Rows) == 0 {
		t.Fatalf("substring query found nothing; the fixture's observations_fts is not using the trigram tokenizer it claims to record")
	}
}

// TestUnionArmDReproducesPublishedTable is the golden test itself
// (design's "arm D reproduces 93/100 · 88/94 · 40/70"): for each of the
// three authored query classes, it runs the real production Run() with
// both Engram sources requested (Branch A: routeRank1 wired live) and
// computes hit@1 (does the merged rank-1 row carry the truth) and hit@5
// (does ANY row in the merged set carry it, over the FULL merged set --
// R-058's actual guarantee, up to 2×top rows, not a top-5-only slice).
//
// All six numbers reproduce the published table exactly.
//
// This test caught a real gate defect once already, and is left as the
// record of it: an earlier `routeRank1` used `if matchMode == "any" {
// return SourceEngramFTS }` as an early return that pre-empted the
// token-shape rule below it, rather than being ORed with it (gate.go's own
// doc comment carries the full history). Because every one of these 10
// paraphrase queries fails FTS's exact AND-match and widens to
// engram.MatchAny, that defect forced all 10 to the lexical arm regardless
// of shape, and this test's paraphrase hit@1 read 10%, not 40% -- a real,
// reproducible regression this golden table exists to catch, not a
// harness artifact. TestGateRoutingAccuracyOnBlindSet
// (gate_blind_test.go) pins the routing-accuracy regression directly,
// against the frozen blind-authored set rather than this table's own
// small, non-blind query set.
func TestUnionArmDReproducesPublishedTable(t *testing.T) {
	fixture := loadUnionFixture(t)
	store, _ := buildUnionGoldenStore(t, fixture)
	stateDir := buildUnionGoldenIndex(t, fixture)
	embedFn := unionGoldenEmbedFunc(fixture)

	cases := []struct {
		class              string
		wantHit1, wantHit5 int // published percentages
	}{
		{"identifier_single", 93, 100},
		{"identifier_multi", 88, 94},
		{"paraphrase", 40, 70},
	}

	for _, tc := range cases {
		t.Run(tc.class, func(t *testing.T) {
			items := fixture.Queries[tc.class]
			if len(items) == 0 {
				t.Fatalf("fixture has no %q queries", tc.class)
			}

			hit1, hit5 := 0, 0
			for _, q := range items {
				deps := Deps{Engram: store, ResolveLink: NoLinkResolver, StateDir: stateDir, Embed: embedFn}
				result, err := Run(context.Background(), deps, Request{
					Project: unionGoldenProject, Query: q.Query, Top: 5,
					Sources: []string{SourceEngramFTS, SourceEngramEmbed},
				})
				if err != nil {
					t.Fatalf("Run(%q): %v", q.Query, err)
				}
				truth := make(map[int64]bool, len(q.Truth))
				for _, id := range q.Truth {
					truth[id] = true
				}
				if len(result.Results) > 0 && truth[result.Results[0].EngramID] {
					hit1++
				}
				for _, r := range result.Results {
					if truth[r.EngramID] {
						hit5++
						break
					}
				}
			}

			n := len(items)
			gotHit1 := (hit1*100 + n/2) / n
			gotHit5 := (hit5*100 + n/2) / n
			if gotHit1 != tc.wantHit1 || gotHit5 != tc.wantHit5 {
				t.Errorf("%s (n=%d): hit@1=%d%% hit@5=%d%%, want hit@1=%d%% hit@5=%d%%", tc.class, n, gotHit1, gotHit5, tc.wantHit1, tc.wantHit5)
			}
		})
	}
}
