package vecindex

import (
	"testing"
	"time"
)

// A Builder keeps one lock for each index directory it builds, so that two builds of one directory made through it
// exclude each other and builds of two directories do not.
func TestBuilder_KeepsOneLockForEachDirectory(t *testing.T) {
	builder := &Builder{}

	first := builder.mutexFor("/state/index/a")

	if again := builder.mutexFor("/state/index/a"); again != first {
		t.Error("asked twice for the lock of one directory, the Builder answered two locks")
	}
	if other := builder.mutexFor("/state/index/b"); other == first {
		t.Error("the lock of a directory is the lock of another: unrelated projects would wait for each other")
	}
}

// The locks are the Builder's own and nothing else's: two Builders share none, and no package-level state holds
// the locks of a process. What a caller wants kept apart it builds through one Builder.
func TestBuilder_ShareNoLockWithAnotherBuilder(t *testing.T) {
	one, another := &Builder{}, &Builder{}

	if one.mutexFor("/state/index/a") == another.mutexFor("/state/index/a") {
		t.Error("two Builders answered the same lock for a directory: the locks are not the Builder's own")
	}
}

// lock is both locks together, the Builder's in-process one and the advisory file lock that keeps processes apart:
// held, it keeps another lock of the same directory waiting; released, it lets it through, and it does not hold
// back a directory of another project.
func TestBuilder_LockHoldsOutTheSameDirectoryUntilItIsReleased(t *testing.T) {
	builder := &Builder{}
	dir, otherDir := t.TempDir(), t.TempDir()

	unlock, err := builder.lock(dir)
	if err != nil {
		t.Fatalf("lock: %v", err)
	}

	acquired := make(chan struct{})
	go func() {
		release, err := builder.lock(dir)
		if err != nil {
			t.Errorf("second lock: %v", err)
			close(acquired)
			return
		}
		close(acquired)
		release()
	}()

	otherUnlock, err := builder.lock(otherDir)
	if err != nil {
		t.Fatalf("lock of another directory while the first is held: %v", err)
	}
	otherUnlock()

	select {
	case <-acquired:
		t.Fatal("a second lock of a directory was granted while the first was held")
	case <-time.After(100 * time.Millisecond):
	}

	unlock()

	select {
	case <-acquired:
	case <-time.After(2 * time.Second):
		t.Fatal("the second lock was not granted after the first was released")
	}
}

// A directory a lock cannot be taken in is the error of taking it, said with the package's words, and leaves the
// directory's in-process lock free for the next build.
func TestBuilder_LockOfADirectoryThatIsNotThereFailsAndLeavesNoLockHeld(t *testing.T) {
	builder := &Builder{}
	missing := t.TempDir() + "/not-made"

	if _, err := builder.lock(missing); err == nil {
		t.Fatal("lock of a directory that does not exist = nil, want an error")
	}

	mu := builder.mutexFor(missing)
	if !mu.TryLock() {
		t.Fatal("the in-process lock of the directory stayed held after the failed lock")
	}
	mu.Unlock()
}
