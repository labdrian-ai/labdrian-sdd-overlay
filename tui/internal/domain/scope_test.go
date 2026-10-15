package domain

// The invariant behind T1: the TUI never acts on a target the operator did
// not see. `--target all` is the one argument that can name more than the
// selection, so it stands for the selection only when the selection is the
// whole catalog that was shown, and only while the backend still answers with
// that same catalog. The use case that runs the invocations is tested over a
// backend in package app; these are the rules it asks the scope.

import (
	"errors"
	"strings"
	"testing"
)

func TestTargetScope_CoversAll(t *testing.T) {
	shown := fourTargets()
	tests := []struct {
		name     string
		shown    []Target
		selected []Target
		want     bool
	}{
		{"the whole catalog", shown, shown, true},
		{"the whole catalog in another order", shown, []Target{shown[3], shown[1], shown[0], shown[2]}, true},
		{"a one-target catalog fully selected", shown[:1], shown[:1], true},
		{"one target fewer than the catalog", shown, shown[:3], false},
		{"the three targets the TUI used to call all", shown, threeCopyTargets(), false},
		{"a target the catalog does not list", shown[:3], []Target{shown[0], shown[1], {Name: "gemini", Kind: KindCopy}}, false},
		{"a name repeated in the selection stands for no catalog", shown[:2], []Target{shown[0], shown[0]}, false},
		{"an empty selection of an empty catalog: two empty sets are equal, and all would act on every target", nil, nil, false},
		{"an empty selection of a catalog", shown, nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scope := TargetScope{Shown: tc.shown}
			if got := scope.CoversAll(tc.selected); got != tc.want {
				t.Errorf("CoversAll(%v) over %v = %t, want %t", tc.selected, tc.shown, got, tc.want)
			}
		})
	}
}

func TestTargetScope_ConfirmUnchanged(t *testing.T) {
	shown := threeCopyTargets()

	t.Run("the backend still lists the catalog shown, in any order", func(t *testing.T) {
		catalog := &fakeCatalog{targets: []Target{shown[2], shown[0], shown[1]}}
		if err := (TargetScope{Shown: shown, Catalog: catalog}).ConfirmUnchanged(); err != nil {
			t.Errorf("ConfirmUnchanged() = %v, want nil", err)
		}
		if catalog.asked != 1 {
			t.Errorf("the catalog was asked %d times, want once", catalog.asked)
		}
	})

	t.Run("the backend now lists another target", func(t *testing.T) {
		err := (TargetScope{Shown: shown, Catalog: &fakeCatalog{targets: fourTargets()}}).ConfirmUnchanged()
		if err == nil || !strings.Contains(err.Error(), "catálogo de destinos del backend cambió") {
			t.Errorf("ConfirmUnchanged() = %v, want the refusal that says the catalog changed", err)
		}
	})

	t.Run("the backend lists one target fewer", func(t *testing.T) {
		err := (TargetScope{Shown: shown, Catalog: &fakeCatalog{targets: shown[:2]}}).ConfirmUnchanged()
		if err == nil {
			t.Error("ConfirmUnchanged() = nil, want a refusal: all would no longer mean what was shown")
		}
	})

	t.Run("the backend cannot be asked", func(t *testing.T) {
		err := (TargetScope{Shown: shown, Catalog: &fakeCatalog{err: errCatalogUnreadable}}).ConfirmUnchanged()
		if !errors.Is(err, errCatalogUnreadable) {
			t.Errorf("ConfirmUnchanged() = %v, want an error that wraps the cause", err)
		}
		if err != nil && !strings.Contains(err.Error(), "no se pudo releer el catálogo de destinos del backend") {
			t.Errorf("ConfirmUnchanged() = %q, want the words that say the catalog could not be read again", err)
		}
	})

	t.Run("there is no catalog to ask", func(t *testing.T) {
		err := (TargetScope{Shown: shown}).ConfirmUnchanged()
		if err == nil || !strings.Contains(err.Error(), "no hay catálogo de destinos") {
			t.Errorf("ConfirmUnchanged() = %v, want the refusal that says there is no catalog", err)
		}
	})
}
