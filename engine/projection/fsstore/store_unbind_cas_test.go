package fsstore_test

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
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection/fsstore"
)

// These tests pin UnbindIfUnchanged, the removal a hook makes when the bound
// workflow is closed. The hook reads the binding, then reads the workflow, then
// removes the binding: a gap in which a user can bind the next workflow. The
// removal must then leave that fresh binding alone, which a plain Unbind cannot
// promise because it removes whatever is bound at that moment.

func TestUnbindIfUnchangedRemovesTheBindingItWasGiven(t *testing.T) {
	s, root := isolatedStore(t)
	current := boundBinding(t, s, hex64("a"), "proj-1", "wf-1", t0)

	removed, err := s.UnbindIfUnchanged(hex64("a"), current)
	if !removed || err != nil {
		t.Fatalf("UnbindIfUnchanged() = %v, %v, want true, nil", removed, err)
	}
	if _, statErr := os.Lstat(bindingPath(root, hex64("a"))); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("the binding file is still there: %v", statErr)
	}
}

// TestUnbindIfUnchangedLeavesAFreshBindingAlone is the race this method exists
// for: the caller saw wf-1 bound (and closed), and before it removed the
// binding somebody bound wf-3. That binding is not the caller's to remove.
func TestUnbindIfUnchangedLeavesAFreshBindingAlone(t *testing.T) {
	s, root := isolatedStore(t)
	seen := boundBinding(t, s, hex64("a"), "proj-1", "wf-1", t0)
	if err := s.Bind(hex64("a"), "proj-3", "wf-3", t0.Add(time.Minute), true); err != nil {
		t.Fatal(err)
	}
	before := readBytes(t, bindingPath(root, hex64("a")))

	removed, err := s.UnbindIfUnchanged(hex64("a"), seen)
	if removed || !errors.Is(err, projection.ErrBindingChanged) {
		t.Fatalf("UnbindIfUnchanged() = %v, %v, want false and ErrBindingChanged", removed, err)
	}
	for _, want := range []string{"proj-3", "wf-3"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name what is bound now (missing %q)", err, want)
		}
	}
	if after := readBytes(t, bindingPath(root, hex64("a"))); !bytes.Equal(after, before) {
		t.Fatalf("the fresh binding was changed:\n%s\nvs\n%s", after, before)
	}
}

func TestUnbindIfUnchangedLeavesTheSameWorkflowBoundAgainAlone(t *testing.T) {
	s, root := isolatedStore(t)
	seen := boundBinding(t, s, hex64("a"), "proj-1", "wf-1", t0)
	if removed, err := s.Unbind(hex64("a")); !removed || err != nil {
		t.Fatalf("Unbind() = %v, %v, want true, nil", removed, err)
	}
	mustBind(t, s, hex64("a"), "proj-1", "wf-1", t0.Add(time.Minute))
	before := readBytes(t, bindingPath(root, hex64("a")))

	removed, err := s.UnbindIfUnchanged(hex64("a"), seen)
	if removed || !errors.Is(err, projection.ErrBindingChanged) {
		t.Fatalf("UnbindIfUnchanged() = %v, %v, want false and ErrBindingChanged: the binding was made again after it was seen", removed, err)
	}
	if after := readBytes(t, bindingPath(root, hex64("a"))); !bytes.Equal(after, before) {
		t.Fatalf("the binding made again was changed:\n%s\nvs\n%s", after, before)
	}
}

// TestUnbindIfUnchangedOfAnAbsentBindingIsANoOp: the binding is already gone,
// which is the state the caller wanted, so this is not an error, and nothing is
// created to say so.
func TestUnbindIfUnchangedOfAnAbsentBindingIsANoOp(t *testing.T) {
	t.Run("the binding was removed meanwhile", func(t *testing.T) {
		s, root := isolatedStore(t)
		seen := boundBinding(t, s, hex64("a"), "proj-1", "wf-1", t0)
		if removed, err := s.Unbind(hex64("a")); !removed || err != nil {
			t.Fatal(removed, err)
		}
		if removed, err := s.UnbindIfUnchanged(hex64("a"), seen); removed || err != nil {
			t.Fatalf("UnbindIfUnchanged() = %v, %v, want false, nil", removed, err)
		}
		if got := names(t, bindingsDir(root)); len(got) != 0 {
			t.Fatalf("bindings directory holds %v, want nothing", got)
		}
	})
	t.Run("the store never existed", func(t *testing.T) {
		s, root := isolatedStore(t)
		if removed, err := s.UnbindIfUnchanged(hex64("a"), validBinding()); removed || err != nil {
			t.Fatalf("UnbindIfUnchanged() = %v, %v, want false, nil", removed, err)
		}
		if got := names(t, root); len(got) != 0 {
			t.Fatalf("UnbindIfUnchanged created %v under the state home, want nothing", got)
		}
	})
}

func TestUnbindIfUnchangedLeavesWhatIsNotOursAlone(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"an unrelated JSON object", `{"hello":"world"}` + "\n", "foreign"},
		{"plain text", "hand-written notes\n", "malformed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, root := isolatedStore(t)
			path := plant(t, root, hex64("a"), tt.content)
			before := readBytes(t, path)

			removed, err := s.UnbindIfUnchanged(hex64("a"), validBinding())
			if removed || !errors.Is(err, projection.ErrBindingChanged) || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("UnbindIfUnchanged() = %v, %v, want false and ErrBindingChanged naming %q", removed, err, tt.want)
			}
			if after := readBytes(t, path); !bytes.Equal(after, before) {
				t.Fatalf("the file changed:\n%q\nvs\n%q", after, before)
			}
		})
	}
	for _, tc := range unavailableCases {
		t.Run(tc.name, func(t *testing.T) {
			s, watched := tc.setup(t)
			before, err := os.Lstat(watched)
			if err != nil {
				t.Fatalf("lstat %s: %v", watched, err)
			}
			removed, err := s.UnbindIfUnchanged(hex64("a"), validBinding())
			if removed || !errors.Is(err, projection.ErrBindingChanged) || !strings.Contains(err.Error(), "unavailable") {
				t.Fatalf("UnbindIfUnchanged() = %v, %v, want false and ErrBindingChanged naming the unavailable state", removed, err)
			}
			after, err := os.Lstat(watched)
			if err != nil || after.Mode() != before.Mode() || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
				t.Errorf("%s changed: %v -> %v (%v)", watched, before.Mode(), after, err)
			}
		})
	}
}

func TestUnbindIfUnchangedRefusesInvalidInputBeforeAnythingIsRead(t *testing.T) {
	otherRepo := validBinding()
	otherRepo.RepoKey = hex64("b")
	tests := []struct {
		name     string
		key      string
		expected projection.Binding
	}{
		{"a short repo key", strings.Repeat("a", 63), validBinding()},
		{"a repo key that climbs out", "../" + strings.Repeat("a", 61), validBinding()},
		{"a zero expected binding", hex64("a"), projection.Binding{}},
		{"an expected binding of another repository", hex64("a"), otherRepo},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, root := isolatedStore(t)
			removed, err := s.UnbindIfUnchanged(tt.key, tt.expected)
			if removed || err == nil || errors.Is(err, projection.ErrBindingChanged) {
				t.Fatalf("UnbindIfUnchanged() = %v, %v, want false and an error that is not ErrBindingChanged", removed, err)
			}
			if got := names(t, root); len(got) != 0 {
				t.Fatalf("a refused UnbindIfUnchanged created %v under the state home, want nothing", got)
			}
		})
	}
}

func TestUnbindIfUnchangedOnAZeroStoreIsRefused(t *testing.T) {
	var s fsstore.Store
	if removed, err := s.UnbindIfUnchanged(hex64("a"), validBinding()); removed || !errors.Is(err, fsstore.ErrStoreNotInitialized) {
		t.Fatalf("UnbindIfUnchanged() = %v, %v, want false and ErrStoreNotInitialized", removed, err)
	}
}

// TestExactlyOneOfManyConcurrentUnbindIfUnchangedCallsRemoves: every caller saw
// the same binding and wants it gone. The binding is removed once; the others
// find it already gone, which is success, not a removal.
func TestExactlyOneOfManyConcurrentUnbindIfUnchangedCallsRemoves(t *testing.T) {
	s, _ := isolatedStore(t)
	seen := boundBinding(t, s, hex64("a"), "proj-1", "wf-1", t0)
	const callers = 8

	start := make(chan struct{})
	var wg sync.WaitGroup
	removals := make([]bool, callers)
	errs := make([]error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			removals[i], errs[i] = s.UnbindIfUnchanged(hex64("a"), seen)
		}(i)
	}
	close(start)
	wg.Wait()

	removed := 0
	for i := range removals {
		if errs[i] != nil {
			t.Errorf("caller %d: UnbindIfUnchanged() = %v, want no error", i, errs[i])
		}
		if removals[i] {
			removed++
		}
	}
	if removed != 1 {
		t.Fatalf("%d callers reported removing the binding, want exactly 1 (%v)", removed, fmt.Sprint(removals))
	}
	if loaded := mustLoad(t, s, hex64("a")); loaded.Classification != projection.ClassificationAbsent {
		t.Fatalf("Load() = %+v, want absent", loaded)
	}
}
