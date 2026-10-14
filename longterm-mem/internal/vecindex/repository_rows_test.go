package vecindex

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/embedindex"
)

// The adapter is also what the build use case asks the index to be kept by, and the rows it builds from are the
// use case's own.
var (
	_ embedindex.Builder = (*Repository)(nil)
	_                    = []Row(([]embedindex.Row)(nil))
)

var buildContract = embedindex.Contract{Model: testModel, Dimension: testDim, InputLimit: testLimit}

// BuildRows builds the project's index from the rows and the embedder it is given, under the contract it is given,
// at the project's directory under the state directory, with the clock it holds, and says what it did; a second
// build over the same rows reuses every entry.
func TestRepositoryBuildRowsBuildsFromTheRowsAndTheEmbedderItIsGiven(t *testing.T) {
	stateDir := t.TempDir()
	rows := []embedindex.Row{{EngramID: 1, Title: "a", Content: "alpha"}, {EngramID: 2, Title: "b", Content: "beta"}}
	embedder := &fakeEmbedder{dim: testDim}
	repository := &Repository{StateDir: stateDir, Now: fixedNow}

	first, err := repository.BuildRows(context.Background(), "p", buildContract, rows, embedder)
	if err != nil {
		t.Fatalf("BuildRows: %v", err)
	}
	second, err := repository.BuildRows(context.Background(), "p", buildContract, rows, embedder)
	if err != nil {
		t.Fatalf("BuildRows (again): %v", err)
	}

	if want := (embedindex.Result{Embedded: 2}); first != want {
		t.Errorf("the first build says %+v, want %+v", first, want)
	}
	if want := (embedindex.Result{Reused: 2}); second != want {
		t.Errorf("the second build says %+v, want %+v: every entry matched", second, want)
	}
	built, err := Load(filepath.Join(stateDir, "index", "p"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if built.Manifest.Model != testModel || built.Manifest.Dimension != testDim || built.Manifest.InputLimit != testLimit || built.Manifest.BuiltAt != fixedNow() {
		t.Errorf("the index was built as %+v, want the contract and the instant of the clock", built.Manifest)
	}
	if len(embedder.calls) != 2 {
		t.Errorf("the embedder was asked %d times, want twice: the second build reused", len(embedder.calls))
	}
}

// A build that removed a row's entry says so.
func TestRepositoryBuildRowsCountsTheEntriesItDrops(t *testing.T) {
	repository := &Repository{StateDir: t.TempDir(), Now: fixedNow}
	embedder := &fakeEmbedder{dim: testDim}
	all := []embedindex.Row{{EngramID: 1, Title: "a", Content: "alpha"}, {EngramID: 2, Title: "b", Content: "beta"}}
	if _, err := repository.BuildRows(context.Background(), "p", buildContract, all, embedder); err != nil {
		t.Fatalf("BuildRows: %v", err)
	}

	got, err := repository.BuildRows(context.Background(), "p", buildContract, all[:1], embedder)

	if err != nil || got != (embedindex.Result{Reused: 1, Removed: 1}) {
		t.Errorf("BuildRows = %+v, %v; want one reused and one removed", got, err)
	}
}

// What the build wrote the cache forgets, so the very query that asked reads it; one that failed changed nothing on
// disk and forgets nothing.
func TestRepositoryBuildRowsInvalidatesTheCacheOnlyWhenItWrote(t *testing.T) {
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
	repository := &Repository{StateDir: stateDir, Cache: cache, Now: fixedNow}

	failing := failingEmbedder{errors.New("backend down")}
	if _, err := repository.BuildRows(context.Background(), "p", buildContract, []embedindex.Row{{EngramID: 9, Title: "t", Content: "c"}}, failing); err == nil {
		t.Fatal("BuildRows = nil error, want the embedder's failure")
	}
	if _, held := cache.entries[dir]; !held {
		t.Fatal("a build that failed made the cache forget an index nothing changed")
	}

	if _, err := repository.BuildRows(context.Background(), "p", buildContract, []embedindex.Row{{EngramID: 1, Title: "a", Content: "alpha"}}, &fakeEmbedder{dim: testDim}); err != nil {
		t.Fatalf("BuildRows: %v", err)
	}
	if _, held := cache.entries[dir]; held {
		t.Fatal("the cache still holds the index a build just replaced")
	}
}

// The failure of a build is the build's own, untouched; a repository with no clock refuses before it reads or writes
// anything and names what is missing.
func TestRepositoryBuildRowsReportsTheBuildsFailureAndNamesAMissingClock(t *testing.T) {
	boom := errors.New("backend down")
	stateDir := t.TempDir()
	rows := []embedindex.Row{{EngramID: 1, Title: "a", Content: "alpha"}}

	if _, err := (&Repository{StateDir: stateDir, Now: fixedNow}).BuildRows(context.Background(), "p", buildContract, rows, failingEmbedder{boom}); !errors.Is(err, boom) {
		t.Errorf("error = %v, want the build's, wrapping %v", err, boom)
	}

	other := t.TempDir()
	if _, err := (&Repository{StateDir: other}).BuildRows(context.Background(), "p", buildContract, rows, &fakeEmbedder{dim: testDim}); !errors.Is(err, errNoClock) {
		t.Errorf("error = %v, want errNoClock", err)
	}
	if _, statErr := os.Stat(Dir(other, "p")); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("the refused build left %s behind (stat: %v)", Dir(other, "p"), statErr)
	}
}

// The rows of the use case and the rows of the index are one type: what the use case maps from the observations is
// what Build is given, with nothing in between to drift.
func TestRowIsTheRowOfTheBuildUseCase(t *testing.T) {
	var row Row = embedindex.Row{EngramID: 1, Title: "t", Content: "c"}

	if !reflect.DeepEqual(row, Row{EngramID: 1, Title: "t", Content: "c"}) {
		t.Errorf("Row = %+v", row)
	}
}
