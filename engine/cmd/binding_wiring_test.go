package main

import (
	"errors"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
)

// recordingBindings is a binding store that remembers the operations it was asked for.
type recordingBindings struct{ calls []string }

func (r *recordingBindings) Load(string) (projection.Loaded, error) {
	r.calls = append(r.calls, "Load")
	return projection.Loaded{Classification: projection.ClassificationAbsent}, nil
}
func (r *recordingBindings) Bind(string, string, string, time.Time, bool) error {
	r.calls = append(r.calls, "Bind")
	return nil
}
func (r *recordingBindings) BindIfUnchanged(string, string, string, time.Time, projection.Binding) error {
	r.calls = append(r.calls, "BindIfUnchanged")
	return nil
}
func (r *recordingBindings) Unbind(string) (bool, error) {
	r.calls = append(r.calls, "Unbind")
	return true, nil
}
func (r *recordingBindings) UnbindIfUnchanged(string, projection.Binding) (bool, error) {
	r.calls = append(r.calls, "UnbindIfUnchanged")
	return true, nil
}

func TestLazyBindingsOpensTheStoreOnFirstUseAndOnlyOnce(t *testing.T) {
	inner := &recordingBindings{}
	opened := 0
	lazy := &lazyBindings{open: func() (projection.BindingStore, error) { opened++; return inner, nil }}
	if opened != 0 {
		t.Fatalf("the store was opened %d times before any use, want 0", opened)
	}

	if _, err := lazy.Load("key"); err != nil {
		t.Fatal(err)
	}
	if err := lazy.Bind("key", "p", "w", time.Time{}, false); err != nil {
		t.Fatal(err)
	}
	if err := lazy.BindIfUnchanged("key", "p", "w", time.Time{}, projection.Binding{}); err != nil {
		t.Fatal(err)
	}
	if _, err := lazy.Unbind("key"); err != nil {
		t.Fatal(err)
	}
	if _, err := lazy.UnbindIfUnchanged("key", projection.Binding{}); err != nil {
		t.Fatal(err)
	}

	if opened != 1 {
		t.Errorf("the store was opened %d times, want once", opened)
	}
	want := []string{"Load", "Bind", "BindIfUnchanged", "Unbind", "UnbindIfUnchanged"}
	if len(inner.calls) != len(want) {
		t.Fatalf("the store was asked %v, want %v", inner.calls, want)
	}
	for i := range want {
		if inner.calls[i] != want[i] {
			t.Errorf("operation %d was %s, want %s", i, inner.calls[i], want[i])
		}
	}
}

func TestLazyBindingsAnswersEveryOperationWithTheReasonTheStoreCouldNotBeOpened(t *testing.T) {
	reason := errors.New("projection store: XDG_STATE_HOME is unset")
	opened := 0
	lazy := &lazyBindings{open: func() (projection.BindingStore, error) { opened++; return nil, reason }}

	calls := map[string]func() error{
		"Load":              func() error { _, err := lazy.Load("key"); return err },
		"Bind":              func() error { return lazy.Bind("key", "p", "w", time.Time{}, false) },
		"BindIfUnchanged":   func() error { return lazy.BindIfUnchanged("key", "p", "w", time.Time{}, projection.Binding{}) },
		"Unbind":            func() error { _, err := lazy.Unbind("key"); return err },
		"UnbindIfUnchanged": func() error { _, err := lazy.UnbindIfUnchanged("key", projection.Binding{}); return err },
	}
	for name, call := range calls {
		if err := call(); err != reason {
			t.Errorf("%s = %v, want the reason the store could not be opened, as it was said", name, err)
		}
	}
	if opened != 1 {
		t.Errorf("the store was opened %d times, want once: the failure is kept", opened)
	}
}

func TestSeamedBindingsRunsEachSeamJustBeforeItsOperation(t *testing.T) {
	inner := &recordingBindings{}
	var order []string
	beforeStaleReplace = func() { order = append(order, "beforeStaleReplace") }
	beforeHookUnbind = func() { order = append(order, "beforeHookUnbind") }
	t.Cleanup(func() { beforeStaleReplace, beforeHookUnbind = nil, nil })
	store := seamedBindings{BindingStore: inner, beforeLoad: func() { order = append(order, "beforeLoad") }}

	_, _ = store.Load("key")
	_ = store.BindIfUnchanged("key", "p", "w", time.Time{}, projection.Binding{})
	_, _ = store.UnbindIfUnchanged("key", projection.Binding{})
	_ = store.Bind("key", "p", "w", time.Time{}, false)
	_, _ = store.Unbind("key")

	wantSeams := []string{"beforeLoad", "beforeStaleReplace", "beforeHookUnbind"}
	if len(order) != len(wantSeams) {
		t.Fatalf("the seams ran %v, want %v and no other operation to run one", order, wantSeams)
	}
	for i := range wantSeams {
		if order[i] != wantSeams[i] {
			t.Errorf("seam %d was %s, want %s", i, order[i], wantSeams[i])
		}
	}
	if len(inner.calls) != 5 {
		t.Errorf("the store was asked %v, want the five operations", inner.calls)
	}
}
