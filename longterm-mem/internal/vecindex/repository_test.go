package vecindex

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/query"
)

// The adapter is what the query package's two ports ask for.
var (
	_ query.EmbeddingIndexRepository = (*Repository)(nil)
	_ query.EmbeddingIndexBuilder    = (*Repository)(nil)
)

func TestRepositoryAnswersTheIndexOfTheProjectInTheQuerysTerms(t *testing.T) {
	stateDir := t.TempDir()
	saved := testIndex()
	if err := saved.Save(Dir(stateDir, "my/project")); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := (&Repository{StateDir: stateDir}).LoadEmbeddingIndex("my/project")
	if err != nil {
		t.Fatalf("LoadEmbeddingIndex: %v", err)
	}

	want := &query.EmbeddingIndex{
		Model: "nomic-embed-text", Dimension: 4, InputLimit: 2000, BuiltAt: "2026-01-01T00:00:00Z",
		Entries: []query.IndexedObservation{{EngramID: 1, Fingerprint: "fp1"}, {EngramID: 2, Fingerprint: "fp2"}},
		Vectors: [][]float32{{1, 2, 3, 4}, {5, 6, 7, 8}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("index = %+v, want %+v", got, want)
	}
}

// An index nobody built and a corrupted one are refused with the errors the package has always given them, so
// that a caller who cares can still tell which; the query treats both as no index.
func TestRepositoryRefusesWhatLoadRefuses(t *testing.T) {
	stateDir := t.TempDir()
	repository := &Repository{StateDir: stateDir}

	if _, err := repository.LoadEmbeddingIndex("never-built"); !errors.Is(err, ErrNoIndex) || !errors.Is(err, query.ErrNoEmbeddingIndex) {
		t.Errorf("error = %v, want ErrNoIndex and, for the query, ErrNoEmbeddingIndex for a project nothing was built for", err)
	}

	corrupted := testIndex()
	if err := corrupted.Save(Dir(stateDir, "broken")); err != nil {
		t.Fatalf("save: %v", err)
	}
	corruptManifest(t, Dir(stateDir, "broken"))
	if _, err := repository.LoadEmbeddingIndex("broken"); !errors.Is(err, ErrCorrupted) || errors.Is(err, query.ErrNoEmbeddingIndex) {
		t.Errorf("error = %v, want ErrCorrupted and not ErrNoEmbeddingIndex for a manifest that does not match its digest", err)
	}
}

// A repository built without what a top-up needs (the live rows, the embedder of a model, the clock, the builder
// that owns the locks) answers the build with an error that names what is missing, before it reads, embeds or
// writes anything, and does not panic. A read needs none of the four.
func TestRepositoryBuiltWithoutWhatABuildNeedsFailsTheBuildNamingIt(t *testing.T) {
	wired := func(stateDir string) *Repository {
		return &Repository{
			StateDir:    stateDir,
			LiveRows:    func(string) ([]Row, error) { return nil, nil },
			NewEmbedder: func(string) (Embedder, error) { return nil, errors.New("no embedder was needed") },
			Now:         func() string { return "2026-01-01T00:00:00Z" },
			Builder:     &Builder{},
		}
	}
	for name, tc := range map[string]struct {
		unwire func(*Repository)
		want   error
	}{
		"no live rows":        {func(r *Repository) { r.LiveRows = nil }, errNoLiveRows},
		"no embedder factory": {func(r *Repository) { r.NewEmbedder = nil }, errNoEmbedderFactory},
		"no clock":            {func(r *Repository) { r.Now = nil }, errNoClock},
		"no builder":          {func(r *Repository) { r.Builder = nil }, errNoBuilder},
	} {
		t.Run(name, func(t *testing.T) {
			stateDir := t.TempDir()
			repository := wired(stateDir)
			tc.unwire(repository)

			err := repository.BuildEmbeddingIndex(context.Background(), "p", "m", 3, 100)

			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if _, statErr := os.Stat(Dir(stateDir, "p")); !errors.Is(statErr, os.ErrNotExist) {
				t.Errorf("the failed build left %s behind (stat: %v)", Dir(stateDir, "p"), statErr)
			}
		})
	}
}

// A repository given a cache reads through it, so a long-lived process reads a project's index from disk once per
// change; one without a cache reads the disk every time.
func TestRepositoryReadsThroughItsCacheWhenItHasOne(t *testing.T) {
	stateDir := t.TempDir()
	if err := testIndex().Save(Dir(stateDir, "p")); err != nil {
		t.Fatalf("save: %v", err)
	}
	loads := 0
	cache := newLoadCacheForTest(&loads)
	cached := &Repository{StateDir: stateDir, Cache: cache}

	for i := 0; i < 3; i++ {
		if _, err := cached.LoadEmbeddingIndex("p"); err != nil {
			t.Fatalf("LoadEmbeddingIndex: %v", err)
		}
	}
	if loads != 1 {
		t.Errorf("the disk was read %d times through the cache, want once", loads)
	}
}

// The fingerprint the query asks for is the package's own, under the contract it is given.
func TestRepositoryFingerprintIsThePackages(t *testing.T) {
	got := (&Repository{}).Fingerprint("m", 3, 10, "title", "content that is longer than ten")
	if want := Fingerprint("m", 3, 10, "title", "content that is longer than ten"); got != want {
		t.Errorf("Fingerprint = %q, want %q", got, want)
	}
}

// corruptManifest leaves a manifest in dir that is not one.
func corruptManifest(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, manifestFileName), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("corrupt the manifest: %v", err)
	}
}

// newBuildRepository is a Repository whose rows, embedder factory and clock are the test's: the factory notes the
// model it is asked for in models.
func newBuildRepository(stateDir string, rows []Row, rowsErr error, models *[]string, embedder Embedder, embedderErr error, cache *LoadCache) *Repository {
	return &Repository{
		StateDir: stateDir,
		Cache:    cache,
		LiveRows: func(string) ([]Row, error) { return rows, rowsErr },
		NewEmbedder: func(model string) (Embedder, error) {
			*models = append(*models, model)
			return embedder, embedderErr
		},
		Now:     fixedNow,
		Builder: &Builder{},
	}
}

// A build reads the project's live rows, makes the embedder of the model it is asked for, builds under the
// contract it is given, at the project's directory under the state directory, with the clock it was handed.
func TestRepositoryBuildsTheProjectsIndexUnderTheContractItIsGiven(t *testing.T) {
	stateDir := t.TempDir()
	rows := []Row{{EngramID: 1, Title: "a", Content: "alpha"}, {EngramID: 2, Title: "b", Content: "beta"}}
	var models []string
	embedder := &fakeEmbedder{dim: testDim}
	repository := newBuildRepository(stateDir, rows, nil, &models, embedder, nil, nil)

	if err := repository.BuildEmbeddingIndex(context.Background(), "p", "other-model", testDim, 50); err != nil {
		t.Fatalf("BuildEmbeddingIndex: %v", err)
	}

	if !reflect.DeepEqual(models, []string{"other-model"}) {
		t.Errorf("the embedder was made for %v, want the model of the call", models)
	}
	built, err := Load(filepath.Join(stateDir, "index", "p"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if built.Manifest.Model != "other-model" || built.Manifest.Dimension != testDim || built.Manifest.InputLimit != 50 {
		t.Errorf("the index was built under %q/%d/%d, want other-model/%d/50", built.Manifest.Model, built.Manifest.Dimension, built.Manifest.InputLimit, testDim)
	}
	if built.Manifest.BuiltAt != fixedNow() {
		t.Errorf("BuiltAt = %q, want the instant of the clock it was given, %q", built.Manifest.BuiltAt, fixedNow())
	}
	if len(built.Manifest.Entries) != 2 || len(embedder.calls) != 2 {
		t.Errorf("the index holds %d entries after %d embeddings, want 2 and 2", len(built.Manifest.Entries), len(embedder.calls))
	}
}

// What a build wrote is forgotten by the cache, so the very query that asked for it reads it; a build that
// failed changed nothing on disk and so forgets nothing.
func TestRepositoryBuildInvalidatesTheCacheOnlyWhenItWrote(t *testing.T) {
	stateDir := t.TempDir()
	dir := Dir(stateDir, "p")
	if err := testIndex().Save(dir); err != nil {
		t.Fatalf("save: %v", err)
	}
	loads := 0
	cache := newLoadCacheForTest(&loads)
	if _, err := cache.Load(dir); err != nil {
		t.Fatalf("warm the cache: %v", err)
	}
	var models []string

	failing := newBuildRepository(stateDir, nil, errors.New("rows offline"), &models, nil, nil, cache)
	if err := failing.BuildEmbeddingIndex(context.Background(), "p", testModel, 4, testLimit); err == nil {
		t.Fatal("BuildEmbeddingIndex = nil error, want the rows' failure")
	}
	if _, held := cache.entries[dir]; !held {
		t.Fatal("a build that failed made the cache forget an index nothing changed")
	}
	if _, err := cache.Load(dir); err != nil || loads != 1 {
		t.Fatalf("after a failed build the disk was read %d times (err %v), want the cached one only", loads, err)
	}

	working := newBuildRepository(stateDir, []Row{{EngramID: 1, Title: "a", Content: "alpha"}}, nil, &models, &fakeEmbedder{dim: 4}, nil, cache)
	if err := working.BuildEmbeddingIndex(context.Background(), "p", testModel, 4, testLimit); err != nil {
		t.Fatalf("BuildEmbeddingIndex: %v", err)
	}
	// Asked before anything reads again: a changed size or mtime of the manifest would make the next Load read
	// the disk whether or not the build forgot the index, so what the cache holds is what proves it did.
	if _, held := cache.entries[dir]; held {
		t.Fatal("the cache still holds the index a build just replaced")
	}
	if _, err := cache.Load(dir); err != nil || loads != 2 {
		t.Fatalf("after a build that wrote, the disk was read %d times (err %v), want a second read", loads, err)
	}
}

// Each stage's failure reaches the caller as it is, and the stages after it do not run.
func TestRepositoryBuildReportsEachStagesFailureUntouched(t *testing.T) {
	rowsErr := errors.New("rows offline")
	embedderErr := errors.New("no embedder for this model")
	embedFailure := errors.New("backend down")

	t.Run("the rows cannot be read", func(t *testing.T) {
		var models []string
		repository := newBuildRepository(t.TempDir(), nil, rowsErr, &models, &fakeEmbedder{dim: testDim}, nil, nil)
		if err := repository.BuildEmbeddingIndex(context.Background(), "p", testModel, testDim, testLimit); err != rowsErr {
			t.Errorf("error = %v, want the rows' own", err)
		}
		if len(models) != 0 {
			t.Errorf("an embedder was made for %v after the rows failed", models)
		}
	})
	t.Run("the embedder cannot be made", func(t *testing.T) {
		repository := newBuildRepository(t.TempDir(), []Row{{EngramID: 1}}, nil, new([]string), nil, embedderErr, nil)
		if err := repository.BuildEmbeddingIndex(context.Background(), "p", testModel, testDim, testLimit); err != embedderErr {
			t.Errorf("error = %v, want the factory's own", err)
		}
	})
	t.Run("the build fails", func(t *testing.T) {
		repository := newBuildRepository(t.TempDir(), []Row{{EngramID: 1, Title: "a", Content: "alpha"}}, nil, new([]string), failingEmbedder{embedFailure}, nil, nil)
		if err := repository.BuildEmbeddingIndex(context.Background(), "p", testModel, testDim, testLimit); !errors.Is(err, embedFailure) {
			t.Errorf("error = %v, want the build's, wrapping %v", err, embedFailure)
		}
	})
}

type failingEmbedder struct{ err error }

func (f failingEmbedder) Embed(context.Context, string) ([]float32, error) { return nil, f.err }
