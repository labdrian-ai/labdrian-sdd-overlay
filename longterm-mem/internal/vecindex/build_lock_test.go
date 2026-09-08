package vecindex

import (
	"context"
	"errors"
	"hash/fnv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// vectorFor deterministically derives a testDimSized vector from text, so a
// concurrency test can assert the persisted index holds exactly what the
// embedder would have produced for each row's content, independent of call
// order or how many times a racing goroutine happened to call Embed for it.
func vectorFor(dim int, text string) []float32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(text))
	seed := float32(h.Sum32()%1000) + 1
	v := make([]float32, dim)
	for i := range v {
		v[i] = seed + float32(i)
	}
	return v
}

// trackingEmbedder is a fakeEmbedder replacement that (a) returns the same
// deterministic vector for the same text regardless of which goroutine or
// how many times it is called, and (b) records the maximum number of Embed
// calls that were ever in flight at once, so a test can assert Build's
// per-directory locking actually serializes embedding work rather than just
// serializing Save.
type trackingEmbedder struct {
	dim      int
	inFlight int32
	maxSeen  int32
}

func (e *trackingEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	n := atomic.AddInt32(&e.inFlight, 1)
	for {
		max := atomic.LoadInt32(&e.maxSeen)
		if n <= max || atomic.CompareAndSwapInt32(&e.maxSeen, max, n) {
			break
		}
	}
	// Hold the "in flight" state briefly so a racing, unlocked second Build
	// call has a real window to observe more than one call in flight.
	time.Sleep(5 * time.Millisecond)
	atomic.AddInt32(&e.inFlight, -1)
	return vectorFor(e.dim, text), nil
}

// erroringEmbedder always fails, to exercise Build's error return path.
type erroringEmbedder struct{}

func (erroringEmbedder) Embed(context.Context, string) ([]float32, error) {
	return nil, errors.New("embed: forced failure")
}

// TestBuildSerializesConcurrentBuildsOnSameDirectory is JD286-1's regression
// test: N goroutines call Build concurrently on one temp directory. Without
// per-directory locking inside Build, two of them can interleave their
// Save calls -- each an independent atomic rename of vectors.blob followed
// by manifest.json -- and pair one build's manifest with another's blob.
// With locking, at most one Embed call is ever in flight, and the index
// left behind after every goroutine returns is internally consistent: every
// manifest entry's vector matches what the embedder deterministically
// returns for that row's content.
func TestBuildSerializesConcurrentBuildsOnSameDirectory(t *testing.T) {
	dir := t.TempDir()
	embedder := &trackingEmbedder{dim: testDim}
	rows := []Row{
		{EngramID: 1, Title: "a", Content: "alpha"},
		{EngramID: 2, Title: "b", Content: "beta"},
		{EngramID: 3, Title: "c", Content: "gamma"},
	}

	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, err := Build(context.Background(), dir, testModel, testDim, testLimit, rows, embedder, fixedNow)
			errs[i] = err
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d Build: %v", i, err)
		}
	}

	if max := atomic.LoadInt32(&embedder.maxSeen); max != 1 {
		t.Errorf("max in-flight Embed calls = %d, want 1 (Build must serialize per directory)", max)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load after concurrent Build calls: %v", err)
	}
	if len(loaded.Manifest.Entries) != len(rows) {
		t.Fatalf("loaded index holds %d entries, want %d", len(loaded.Manifest.Entries), len(rows))
	}

	wantByID := make(map[int64][]float32, len(rows))
	for _, row := range rows {
		wantByID[row.EngramID] = vectorFor(testDim, EmbedInput(row.Title, row.Content, testLimit))
	}
	for i, entry := range loaded.Manifest.Entries {
		want, ok := wantByID[entry.EngramID]
		if !ok {
			t.Fatalf("loaded index has unexpected engram id %d", entry.EngramID)
		}
		got := loaded.Vectors[i]
		if len(got) != len(want) {
			t.Fatalf("engram id %d: vector length %d, want %d", entry.EngramID, len(got), len(want))
		}
		for j := range want {
			if got[j] != want[j] {
				t.Fatalf("engram id %d: vector[%d] = %v, want %v (manifest/blob pairing is corrupted)", entry.EngramID, j, got[j], want[j])
			}
		}
	}
}

// TestBuildReleasesLockOnEmbedderError proves Build's lock (both the
// in-process mutex and the cross-process flock) is released on every return
// path, including the error path: a Build call that fails because the
// embedder errors must not leave the directory locked for a later Build.
func TestBuildReleasesLockOnEmbedderError(t *testing.T) {
	dir := t.TempDir()
	rows := []Row{{EngramID: 1, Title: "a", Content: "alpha"}}

	if _, _, err := Build(context.Background(), dir, testModel, testDim, testLimit, rows, erroringEmbedder{}, fixedNow); err == nil {
		t.Fatal("Build with an erroring embedder: got nil error, want an error")
	}

	done := make(chan error, 1)
	go func() {
		_, _, err := Build(context.Background(), dir, testModel, testDim, testLimit, rows, &fakeEmbedder{dim: testDim}, fixedNow)
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("second Build after failed Build: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second Build after failed Build blocked -- lock was not released on the error path")
	}
}
