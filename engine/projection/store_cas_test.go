package projection_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
)

// These tests pin BindIfUnchanged, the compare-and-swap a caller uses when it
// has read a binding, judged it stale, and now wants to replace exactly that
// binding. Everything is deterministic: "another process" is the test itself
// changing the store between the read and the swap, except in the last test,
// which races real callers and asserts only what the lock guarantees.

// boundBinding binds the repository to project/wf and returns the binding the
// store now holds, which is what a caller that read the store would have seen.
func boundBinding(t *testing.T, s projection.Store, key, project, wf string, at time.Time) projection.Binding {
	t.Helper()
	mustBind(t, s, key, project, wf, at)
	loaded := mustLoad(t, s, key)
	if loaded.Classification != projection.ClassificationOwned {
		t.Fatalf("Load() = %+v, want an owned binding", loaded)
	}
	return loaded.Binding
}

func TestBindIfUnchangedReplacesTheBindingItWasGiven(t *testing.T) {
	s, root := isolatedStore(t)
	stale := boundBinding(t, s, hex64("a"), "proj-1", "wf-1", t0)

	if err := s.BindIfUnchanged(hex64("a"), "proj-2", "wf-9", t0.Add(time.Hour), stale); err != nil {
		t.Fatalf("BindIfUnchanged() = %v, want nil: the binding is still the one that was read", err)
	}
	want := projection.Binding{Version: 1, RepoKey: hex64("a"), ProjectID: "proj-2", WorkflowID: "wf-9", BoundAt: "2026-09-29T11:00:00Z"}
	if loaded := mustLoad(t, s, hex64("a")); loaded.Classification != projection.ClassificationOwned || loaded.Binding != want {
		t.Fatalf("Load() = %+v, want owned with %+v", loaded, want)
	}
	if got := names(t, bindingsDir(root)); fmt.Sprint(got) != fmt.Sprint([]string{hex64("a") + ".json"}) {
		t.Fatalf("bindings directory holds %v, want only the binding (no temporary file)", got)
	}
}

// TestBindIfUnchangedRefusesABindingAnotherProcessReplacedMeanwhile is the race
// this method exists for: the caller judged wf-1 stale, and before it swapped,
// another process bound a live workflow. Replacing it would silently move
// every session of the repository off a workflow somebody just chose.
func TestBindIfUnchangedRefusesABindingAnotherProcessReplacedMeanwhile(t *testing.T) {
	s, root := isolatedStore(t)
	stale := boundBinding(t, s, hex64("a"), "proj-1", "wf-1", t0)
	if err := s.Bind(hex64("a"), "proj-3", "wf-3", t0.Add(30*time.Minute), true); err != nil {
		t.Fatal(err)
	}
	before := readBytes(t, bindingPath(root, hex64("a")))

	err := s.BindIfUnchanged(hex64("a"), "proj-2", "wf-9", t0.Add(time.Hour), stale)
	if !errors.Is(err, projection.ErrBindingChanged) {
		t.Fatalf("BindIfUnchanged() = %v, want ErrBindingChanged", err)
	}
	for _, want := range []string{"proj-3", "wf-3"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name what is bound now (missing %q)", err, want)
		}
	}
	if after := readBytes(t, bindingPath(root, hex64("a"))); !bytes.Equal(after, before) {
		t.Fatalf("a refused BindIfUnchanged changed the file:\n%s\nvs\n%s", after, before)
	}
}

// TestBindIfUnchangedRefusesTheSameWorkflowBoundAgain pins that "unchanged"
// means the same record, not the same workflow: an unbind followed by a bind of
// the same workflow is a decision somebody made after the caller read it.
func TestBindIfUnchangedRefusesTheSameWorkflowBoundAgain(t *testing.T) {
	s, root := isolatedStore(t)
	stale := boundBinding(t, s, hex64("a"), "proj-1", "wf-1", t0)
	if removed, err := s.Unbind(hex64("a")); !removed || err != nil {
		t.Fatalf("Unbind() = %v, %v, want true, nil", removed, err)
	}
	mustBind(t, s, hex64("a"), "proj-1", "wf-1", t0.Add(time.Minute))
	before := readBytes(t, bindingPath(root, hex64("a")))

	err := s.BindIfUnchanged(hex64("a"), "proj-2", "wf-9", t0.Add(time.Hour), stale)
	if !errors.Is(err, projection.ErrBindingChanged) {
		t.Fatalf("BindIfUnchanged() = %v, want ErrBindingChanged: the binding was made again after it was read", err)
	}
	if after := readBytes(t, bindingPath(root, hex64("a"))); !bytes.Equal(after, before) {
		t.Fatalf("the file changed:\n%s\nvs\n%s", after, before)
	}
}

// TestBindIfUnchangedRefusesABindingThatWasRemovedAndCreatesNothing pins the
// other half of "write nothing": a removed binding is not brought back with
// the caller's workflow, and the refusal does not even create the store.
func TestBindIfUnchangedRefusesABindingThatWasRemovedAndCreatesNothing(t *testing.T) {
	t.Run("the binding was unbound", func(t *testing.T) {
		s, root := isolatedStore(t)
		stale := boundBinding(t, s, hex64("a"), "proj-1", "wf-1", t0)
		if removed, err := s.Unbind(hex64("a")); !removed || err != nil {
			t.Fatalf("Unbind() = %v, %v, want true, nil", removed, err)
		}

		err := s.BindIfUnchanged(hex64("a"), "proj-2", "wf-9", t0.Add(time.Hour), stale)
		if !errors.Is(err, projection.ErrBindingChanged) || !strings.Contains(err.Error(), "removed") {
			t.Fatalf("BindIfUnchanged() = %v, want ErrBindingChanged saying the binding was removed", err)
		}
		if got := names(t, bindingsDir(root)); len(got) != 0 {
			t.Fatalf("bindings directory holds %v, want nothing: a removed binding must not be recreated", got)
		}
	})
	t.Run("the store never existed", func(t *testing.T) {
		s, root := isolatedStore(t)
		err := s.BindIfUnchanged(hex64("a"), "proj-2", "wf-9", t0, validBinding())
		if !errors.Is(err, projection.ErrBindingChanged) {
			t.Fatalf("BindIfUnchanged() = %v, want ErrBindingChanged", err)
		}
		if got := names(t, root); len(got) != 0 {
			t.Fatalf("a refused BindIfUnchanged created %v under the state home, want nothing", got)
		}
	})
}

// TestBindIfUnchangedLeavesWhatIsNotOursAlone: a foreign or malformed file is
// not the binding that was read, and, as for Bind, it is never overwritten.
func TestBindIfUnchangedLeavesWhatIsNotOursAlone(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"an unrelated JSON object", `{"hello":"world"}` + "\n", "foreign"},
		{"a binding that names another repository", rawBinding(hex64("b")), "foreign"},
		{"plain text", "hand-written notes\n", "malformed"},
		{"an empty file", "", "malformed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, root := isolatedStore(t)
			path := plant(t, root, hex64("a"), tt.content)
			before := readBytes(t, path)

			err := s.BindIfUnchanged(hex64("a"), "proj-2", "wf-9", t0, validBinding())
			if !errors.Is(err, projection.ErrBindingChanged) || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("BindIfUnchanged() = %v, want ErrBindingChanged naming %q", err, tt.want)
			}
			if after := readBytes(t, path); !bytes.Equal(after, before) {
				t.Fatalf("the file changed:\n%q\nvs\n%q", after, before)
			}
			if got := names(t, bindingsDir(root)); fmt.Sprint(got) != fmt.Sprint([]string{hex64("a") + ".json"}) {
				t.Fatalf("bindings directory holds %v, want only the original file", got)
			}
		})
	}
}

func TestBindIfUnchangedLeavesUnavailableStatesAlone(t *testing.T) {
	for _, tc := range unavailableCases {
		t.Run(tc.name, func(t *testing.T) {
			s, watched := tc.setup(t)
			before, err := os.Lstat(watched)
			if err != nil {
				t.Fatalf("lstat %s: %v", watched, err)
			}

			err = s.BindIfUnchanged(hex64("a"), "proj-2", "wf-9", t0, validBinding())
			if !errors.Is(err, projection.ErrBindingChanged) || !strings.Contains(err.Error(), "unavailable") {
				t.Fatalf("BindIfUnchanged() = %v, want ErrBindingChanged naming the unavailable state", err)
			}
			after, err := os.Lstat(watched)
			if err != nil {
				t.Fatalf("lstat %s after the refusal: %v", watched, err)
			}
			if after.Mode() != before.Mode() || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
				t.Errorf("%s changed: mode %v -> %v, size %d -> %d", watched, before.Mode(), after.Mode(), before.Size(), after.Size())
			}
		})
	}
}

// TestBindIfUnchangedRefusesInvalidInputBeforeAnythingIsRead: a bad key, a bad
// identifier, or an expected binding that cannot be a stored one is a bug in
// the caller, not a changed store, and is reported as such.
func TestBindIfUnchangedRefusesInvalidInputBeforeAnythingIsRead(t *testing.T) {
	otherRepo := validBinding()
	otherRepo.RepoKey = hex64("b")
	tests := []struct {
		name                     string
		key, project, workflowID string
		expected                 projection.Binding
	}{
		{"a short repo key", strings.Repeat("a", 63), "proj-2", "wf-9", validBinding()},
		{"a repo key that climbs out", "../" + strings.Repeat("a", 61), "proj-2", "wf-9", validBinding()},
		{"an empty project id", hex64("a"), "", "wf-9", validBinding()},
		{"a hidden workflow id", hex64("a"), "proj-2", ".wf", validBinding()},
		{"a zero expected binding", hex64("a"), "proj-2", "wf-9", projection.Binding{}},
		{"an expected binding of another repository", hex64("a"), "proj-2", "wf-9", otherRepo},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, root := isolatedStore(t)
			err := s.BindIfUnchanged(tt.key, tt.project, tt.workflowID, t0, tt.expected)
			if err == nil {
				t.Fatal("BindIfUnchanged() = nil, want an error")
			}
			if errors.Is(err, projection.ErrBindingChanged) {
				t.Errorf("BindIfUnchanged() = %v: invalid input must not be reported as a changed binding", err)
			}
			if got := names(t, root); len(got) != 0 {
				t.Fatalf("a refused BindIfUnchanged created %v under the state home, want nothing", got)
			}
		})
	}
}

// TestBindIfUnchangedOfTheSameWorkflowIsAnIdempotentNoOp: Bind never rewrites a
// binding to the workflow it already names, and BindIfUnchanged keeps that.
func TestBindIfUnchangedOfTheSameWorkflowIsAnIdempotentNoOp(t *testing.T) {
	s, root := isolatedStore(t)
	current := boundBinding(t, s, hex64("a"), "proj-1", "wf-1", t0)
	before := readBytes(t, bindingPath(root, hex64("a")))

	if err := s.BindIfUnchanged(hex64("a"), "proj-1", "wf-1", t0.Add(24*time.Hour), current); err != nil {
		t.Fatalf("BindIfUnchanged() = %v, want nil", err)
	}
	if after := readBytes(t, bindingPath(root, hex64("a"))); !bytes.Equal(after, before) {
		t.Fatalf("binding the workflow it already names rewrote the file (bound_at must keep the first time):\n%s\nvs\n%s", after, before)
	}
}

// TestExactlyOneOfManyConcurrentBindIfUnchangedCallsWins races callers that all
// read the same binding and judged it stale. The lock spans the comparison and
// the write, so exactly one swap can find the binding unchanged; every other
// caller must be told it changed, and none may overwrite the winner.
func TestExactlyOneOfManyConcurrentBindIfUnchangedCallsWins(t *testing.T) {
	s, _ := isolatedStore(t)
	stale := boundBinding(t, s, hex64("a"), "proj-1", "wf-0", t0)
	const callers = 8

	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			errs[i] = s.BindIfUnchanged(hex64("a"), "proj-1", fmt.Sprintf("wf-%d", i+1), t0, stale)
		}(i)
	}
	close(start)
	wg.Wait()

	winner := -1
	for i, err := range errs {
		switch {
		case err == nil:
			if winner != -1 {
				t.Fatalf("callers %d and %d both replaced a binding only one of them could have found unchanged", winner, i)
			}
			winner = i
		case !errors.Is(err, projection.ErrBindingChanged):
			t.Errorf("caller %d: BindIfUnchanged() = %v, want nil or ErrBindingChanged", i, err)
		}
	}
	if winner == -1 {
		t.Fatalf("no caller replaced the binding: %v", errs)
	}
	want := fmt.Sprintf("wf-%d", winner+1)
	if loaded := mustLoad(t, s, hex64("a")); loaded.Classification != projection.ClassificationOwned || loaded.Binding.WorkflowID != want {
		t.Fatalf("Load() = %+v, want the winner's binding to %s", loaded, want)
	}
}
