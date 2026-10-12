package vecindex

import (
	"errors"
	"reflect"
	"testing"
)

// noEmbeddingIndex and embeddingIndexReader are what the diagnostics (internal/ops) ask of this adapter,
// restated in terms of the standard library alone: this package does not import the package that consumes it,
// so the shape of its port is held here by what the methods say, and the composition root holds the adapter to
// the port itself (cmd/longterm-mem, ops_ports_test.go).
type noEmbeddingIndex interface {
	error
	NoEmbeddingIndex()
}

type embeddingIndexReader interface {
	EmbeddingIndexLocation(project string) string
	EmbeddingIndexBuiltAt(project string) (string, error)
	IndexedEngramIDs(project string) ([]int64, error)
}

var _ embeddingIndexReader = (*Repository)(nil)

// The diagnostics name the directory they looked in, and it is the directory the index is kept in.
func TestRepositoryTellsWhereTheIndexOfAProjectIsKept(t *testing.T) {
	stateDir := t.TempDir()

	if got, want := (&Repository{StateDir: stateDir}).EmbeddingIndexLocation("my/project"), Dir(stateDir, "my/project"); got != want {
		t.Errorf("location = %q, want %q", got, want)
	}
	// A repository with no state directory keeps its indexes where Dir puts them for an empty one: the
	// diagnostics have always named that path, and still do.
	if got, want := (&Repository{}).EmbeddingIndexLocation("p"), Dir("", "p"); got != want {
		t.Errorf("location without a state directory = %q, want %q", got, want)
	}
}

// A built index answers when it was built and which observations it vouches for, in the order it holds them,
// and the ids are the caller's own slice: the repository may share the vectors of what it caches, but it hands
// nobody the manifest's entries to change.
func TestRepositoryAnswersWhenAnIndexWasBuiltAndWhichRowsItHolds(t *testing.T) {
	stateDir := t.TempDir()
	if err := testIndex().Save(Dir(stateDir, "p")); err != nil {
		t.Fatalf("save: %v", err)
	}
	repository := &Repository{StateDir: stateDir}

	builtAt, err := repository.EmbeddingIndexBuiltAt("p")
	if err != nil || builtAt != "2026-01-01T00:00:00Z" {
		t.Errorf("built at = (%q, %v), want (\"2026-01-01T00:00:00Z\", nil)", builtAt, err)
	}
	ids, err := repository.IndexedEngramIDs("p")
	if err != nil || !reflect.DeepEqual(ids, []int64{1, 2}) {
		t.Errorf("ids = (%v, %v), want ([1 2], nil)", ids, err)
	}
	ids[0] = 99
	again, _ := repository.IndexedEngramIDs("p")
	if !reflect.DeepEqual(again, []int64{1, 2}) {
		t.Errorf("ids after the caller changed the first answer = %v, want [1 2]", again)
	}
}

// A project nothing was built for is told by what the error says it is, and the error is the load's own with
// its own words; an index that is there and cannot be trusted is the load's error untouched, not the other kind.
func TestRepositoryTellsAnIndexNobodyBuiltFromOneThatCannotBeRead(t *testing.T) {
	stateDir := t.TempDir()
	repository := &Repository{StateDir: stateDir}
	if err := testIndex().Save(Dir(stateDir, "broken")); err != nil {
		t.Fatalf("save: %v", err)
	}
	corruptManifest(t, Dir(stateDir, "broken"))
	_, loadErr := Load(Dir(stateDir, "broken"))

	reads := map[string]func(project string) error{
		"built at": func(project string) error { _, err := repository.EmbeddingIndexBuiltAt(project); return err },
		"ids":      func(project string) error { _, err := repository.IndexedEngramIDs(project); return err },
	}
	for name, read := range reads {
		t.Run(name, func(t *testing.T) {
			err := read("never-built")
			var nothing noEmbeddingIndex
			if !errors.As(err, &nothing) || !errors.Is(err, ErrNoIndex) || err.Error() != ErrNoIndex.Error() {
				t.Errorf("error = %v, want one that says it is an index nobody built, is ErrNoIndex and has its words", err)
			}

			err = read("broken")
			if errors.As(err, &nothing) || !errors.Is(err, ErrCorrupted) || err.Error() != loadErr.Error() {
				t.Errorf("error = %v, want the load's own error (%v), which is not an index nobody built", err, loadErr)
			}
		})
	}
}

// A repository that caches reads the diagnostics' questions through its cache, as it reads the query's: asking
// both of one project reads the disk once.
func TestRepositoryReadsTheDiagnosticsQuestionsThroughItsCache(t *testing.T) {
	stateDir := t.TempDir()
	if err := testIndex().Save(Dir(stateDir, "p")); err != nil {
		t.Fatalf("save: %v", err)
	}
	cache := NewLoadCache()
	diskReads := 0
	cache.load = func(dir string) (*Index, error) { diskReads++; return Load(dir) }
	repository := &Repository{StateDir: stateDir, Cache: cache}

	if _, err := repository.EmbeddingIndexBuiltAt("p"); err != nil {
		t.Fatalf("built at: %v", err)
	}
	if _, err := repository.IndexedEngramIDs("p"); err != nil {
		t.Fatalf("ids: %v", err)
	}

	if diskReads != 1 {
		t.Errorf("the disk was read %d times for two questions about one index, want once", diskReads)
	}
}
