package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vecindex"
)

// embeddingIndexDir is where the embedding index of the golden project lives in the world's state directory.
func (w *retrievalWorld) embeddingIndexDir() string {
	return vecindex.Dir(w.stateDir, queryGoldenProject)
}

func embeddingIndexGoldenSteps() []retrievalStep {
	build := []string{"index", "--project", queryGoldenProject, "--embeddings", "--embed-dimension", "3"}
	query := func(rest ...string) []string {
		return append([]string{"query", "--project", queryGoldenProject}, rest...)
	}
	return []retrievalStep{
		{name: "01-build-the-embedding-index", args: build},
		{name: "02-build-it-again-and-reuse-every-row", args: build},
		{name: "03-query-reads-both-engram-sources-once-an-index-exists", args: query("--json", "dragonscale")},
		{name: "04-the-text-form-names-the-sources-of-a-row", args: query("cache")},
		{
			name: "05-an-edited-row-and-a-deleted-one-are-not-served-from-the-index",
			before: func(t *testing.T, w *retrievalWorld) {
				w.execSQL(t, `UPDATE observations SET content = 'We cache with strategy gamma-cache now.' WHERE id = 8`)
				w.execSQL(t, `UPDATE observations SET deleted_at = '2026-09-01 00:00:00' WHERE id = 3`)
			},
			args: query("--json", "cache"),
		},
		{
			name: "06-one-new-row-is-folded-into-the-index-before-the-query-answers",
			before: func(t *testing.T, w *retrievalWorld) {
				w.execSQL(t, newObservationSQL(20, "Fresh dragonscale note", "dragonscale fresh body"))
			},
			args: query("--json", "dragonscale"),
		},
		// The backend answers 500 with a plain-text body, and the diagnostics carry it as the program words it:
		// a 500 is "unreachable" (the backend produced no vector; only a 404 or a "not found" body is a missing
		// model, step 08) and the detail ends with the body's own newline. Both are recorded as they are;
		// trimming the body would change the detail, which is the owner's decision, not a golden's.
		{
			name: "07-a-top-up-the-backend-cannot-serve-is-named-beside-the-answer",
			before: func(t *testing.T, w *retrievalWorld) {
				w.execSQL(t, newObservationSQL(21, "Second dragonscale note", "dragonscale second body"))
				w.backend.mode.Store(int32(backendFails))
			},
			args: query("--json", "dragonscale"),
		},
		{
			name: "08-a-backend-without-the-model-is-named-apart-from-an-unreachable-one",
			before: func(t *testing.T, w *retrievalWorld) {
				w.backend.mode.Store(int32(backendLacksTheModel))
			},
			args: query("--json", "dragonscale"),
		},
		{
			name: "09-more-rows-than-a-top-up-takes-leave-the-index-incomplete",
			before: func(t *testing.T, w *retrievalWorld) {
				w.backend.mode.Store(int32(backendAnswers))
				for id := 30; id < 30+26; id++ {
					w.execSQL(t, newObservationSQL(id, fmt.Sprintf("Bulk note %d", id), "bulk body"))
				}
			},
			args: query("--json", "dragonscale"),
		},
		{name: "10-the-index-builds-what-the-query-did-not", args: build},
		// The query over a manifest it cannot read answers from full-text search, as for an index nobody built
		// (coverage null, the same rows, exit 0), and says so beside the answer with the load error (owner decision
		// of 2026-10-12). The build of the next step refuses the same index.
		{
			name: "11-a-corrupted-manifest-is-reported-and-the-query-falls-back",
			before: func(t *testing.T, w *retrievalWorld) {
				if err := os.WriteFile(filepath.Join(w.embeddingIndexDir(), "manifest.json"), []byte("{not json"), 0o600); err != nil {
					t.Fatalf("corrupt the manifest: %v", err)
				}
			},
			args: query("--json", "dragonscale"),
		},
		{name: "12-the-build-refuses-a-corrupted-index", args: build},
		{
			name: "13-an-index-removed-is-built-from-nothing",
			before: func(t *testing.T, w *retrievalWorld) {
				if err := os.RemoveAll(w.embeddingIndexDir()); err != nil {
					t.Fatalf("remove the corrupted index: %v", err)
				}
			},
			args: build,
		},
		{
			name: "14-a-remote-endpoint-is-refused-for-the-build",
			args: append(append([]string{}, build...), "--embed-endpoint", "http://example.com:11434"),
		},
		{name: "15-another-project-has-its-own-index", args: []string{"index", "--project", "otherproj", "--embeddings", "--embed-dimension", "3"}},
		{name: "16-and-the-golden-project-is-untouched-by-it", args: query("--json", "dragonscale")},
	}
}

// newObservationSQL is the statement that adds one live observation to the golden project.
func newObservationSQL(id int, title, content string) string {
	return fmt.Sprintf(`INSERT INTO observations (id, sync_id, session_id, type, title, content, project, topic_key, revision_count, pinned, created_at, updated_at, deleted_at)
		VALUES (%d, 'sy-%d', 's', 'discovery', '%s', '%s', 'goldenproj', NULL, 1, 0, '2026-09-10 00:00:00', '2026-09-10 00:00:00', NULL)`, id, id, title, content)
}

// TestEmbeddingIndexGolden walks one world through the life of an embedding index: built, reused, queried by the
// union of both Engram sources, behind the corpus, topped up, topped up and failing, unreachable, without its
// model, past the bound of a top-up, corrupted, rebuilt, removed and built again. Each step's golden holds the command's output and the
// files the state directory holds after it.
func TestEmbeddingIndexGolden(t *testing.T) {
	w := newRetrievalWorld(t)
	for _, step := range embeddingIndexGoldenSteps() {
		if step.before != nil {
			step.before(t, w)
		}
		code, stdout, stderr := w.run(t, step.args)
		got := w.normalise(fmt.Sprintf("args: %s\nexit: %d\n--- stdout ---\n%s--- stderr ---\n%s--- state ---\n%s",
			strings.Join(step.args, " "), code, stdout, stderr, w.renderState(t)))
		w.check(t, "embedding-"+step.name, got)
	}
}

func (w *retrievalWorld) execSQL(t *testing.T, statement string, args ...any) {
	t.Helper()
	db, err := sql.Open("sqlite", w.dbPath)
	if err != nil {
		t.Fatalf("open the fixture database: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(statement, args...); err != nil {
		t.Fatalf("execute %q: %v", statement, err)
	}
}

// summariseManifest says what a manifest holds in a line each: its contract, when it was built, and every entry
// with the first twelve characters of its fingerprint. A manifest that is not JSON is printed as it is.
func summariseManifest(data []byte) string {
	var m struct {
		Model      string `json:"model"`
		Dimension  int    `json:"dimension"`
		InputLimit int    `json:"input_limit"`
		BuiltAt    string `json:"built_at"`
		Revision   string `json:"revision"`
		Entries    []struct {
			EngramID    int64  `json:"engram_id"`
			Fingerprint string `json:"fingerprint"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return string(data) + "\n"
	}
	var b strings.Builder
	builtAt := m.BuiltAt
	if instantInOutput.MatchString(builtAt) {
		builtAt = "<instant>"
	}
	fmt.Fprintf(&b, "model %s, dimension %d, input limit %d, built at %s, revision <digest>\n", m.Model, m.Dimension, m.InputLimit, builtAt)
	for _, e := range m.Entries {
		fmt.Fprintf(&b, "  engram:%d %.12s\n", e.EngramID, e.Fingerprint)
	}
	return b.String()
}

// renderState lists every file under the world's state directory, sorted, with its content: a text file as it
// is, the manifest summarised, the vector blob by its size and digest.
func (w *retrievalWorld) renderState(t *testing.T) string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(w.stateDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			rel, relErr := filepath.Rel(w.stateDir, path)
			if relErr != nil {
				return relErr
			}
			paths = append(paths, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk the state directory: %v", err)
	}
	sort.Strings(paths)
	var b strings.Builder
	for _, rel := range paths {
		data, err := os.ReadFile(filepath.Join(w.stateDir, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		fmt.Fprintf(&b, "=== %s ===\n", filepath.ToSlash(rel))
		switch {
		case strings.HasSuffix(rel, ".blob"):
			fmt.Fprintf(&b, "%d bytes, sha256 %x\n", len(data), sha256.Sum256(data))
		case strings.HasSuffix(rel, "manifest.json"):
			b.WriteString(summariseManifest(data))
		case len(data) == 0:
			b.WriteString("(empty)\n")
		default:
			b.Write(data)
			if !strings.HasSuffix(string(data), "\n") {
				b.WriteString("\n")
			}
		}
	}
	if b.Len() == 0 {
		return "(nothing)\n"
	}
	return b.String()
}
