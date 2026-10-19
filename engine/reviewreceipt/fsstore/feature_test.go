package fsstore

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gitprov"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewreceipt"
)

var _ reviewreceipt.FeatureCatalog = Store{}

// A feature's receipts are in odd/review-receipts/<feature>/ under the root openspec/ is found
// under, beside the feature's task document odd/tasks/<feature>.md: Location says so, Write
// puts the exact bytes there privately, and Read gives them back or says none are persisted.
func TestAFeaturesReceiptsAreUnderOddReviewReceipts(t *testing.T) {
	root := t.TempDir()
	s := newStore(t, root, &locator{})
	dest := reviewreceipt.InFeature("consolidation-phase9")
	name := "review-939c22285467c2b0.review-state.json"

	want := filepath.Join(root, "odd", "review-receipts", "consolidation-phase9", name)
	if got := s.Location(dest, name); got != want {
		t.Fatalf("Location = %q, want %q", got, want)
	}
	if _, err := s.Read(dest, name); !errors.Is(err, reviewreceipt.ErrNotPersisted) {
		t.Errorf("Read before the write = %v, want ErrNotPersisted", err)
	}
	data := []byte("{\"exact\": \"bytes\"}\n")
	if err := s.Write(dest, name, data); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got, err := os.ReadFile(want); err != nil || string(got) != string(data) {
		t.Errorf("persisted %q, %v, want %q", got, err, data)
	}
	if info, err := os.Stat(want); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, %v, want 0600", info, err)
	}
	if got, err := s.Read(dest, name); err != nil || string(got) != string(data) {
		t.Errorf("Read = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(root, "openspec")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a feature's write touched openspec/: %v", err)
	}
}

// The feature folder is found under the same root as openspec/: the toplevel the locator
// reports, from a subdirectory of a project whose toplevel has openspec/changes.
func TestAFeatureFolderIsUnderTheRootOpenspecIsFoundUnder(t *testing.T) {
	top := t.TempDir()
	sub := filepath.Join(top, "sub")
	write(t, filepath.Join(top, "openspec", "changes", "a", "tasks.md"), "x")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	s := newStore(t, sub, &locator{obs: gitprov.Observation{Toplevel: top}})
	if err := s.Write(reviewreceipt.InFeature("f"), "review-a.json", []byte("{}")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(top, "odd", "review-receipts", "f", "review-a.json")); err != nil {
		t.Errorf("the receipt is not under the toplevel: %v", err)
	}
	if got, err := s.Features(); err != nil || !reflect.DeepEqual(got, []string{"f"}) {
		t.Errorf("Features = %v, %v, want [f]", got, err)
	}
}

// The features are the directories under odd/review-receipts, by name; a project without the
// folder has none, and a folder that is there and cannot be read is an error that names it.
func TestFeaturesAreTheDirectoriesUnderOddReviewReceipts(t *testing.T) {
	root := t.TempDir()
	s := newStore(t, root, &locator{})
	if got, err := s.Features(); err != nil || got != nil {
		t.Errorf("Features without the folder = %v, %v, want none", got, err)
	}

	base := filepath.Join(root, "odd", "review-receipts")
	for _, d := range []string{"zeta", "alpha"} {
		if err := os.MkdirAll(filepath.Join(base, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write(t, filepath.Join(base, "a-file.json"), "not a feature")
	if err := os.Symlink(filepath.Join(base, "alpha"), filepath.Join(base, "a-link")); err != nil {
		t.Logf("symlinks unavailable: %v", err)
	}
	if got, err := s.Features(); err != nil || !reflect.DeepEqual(got, []string{"alpha", "zeta"}) {
		t.Errorf("Features = %v, %v, want [alpha zeta]", got, err)
	}

	blocked := t.TempDir()
	write(t, filepath.Join(blocked, "odd", "review-receipts"), "a file where the folder should be")
	_, err := newStore(t, blocked, &locator{}).Features()
	if want := "read " + filepath.Join(blocked, "odd", "review-receipts") + ": "; err == nil || !strings.HasPrefix(err.Error(), want) {
		t.Errorf("Features = %v, want an error that starts with %q", err, want)
	}
}

// A destination of no kind the domain names is refused, never written somewhere guessed.
func TestADestinationOfNoKindIsRefused(t *testing.T) {
	root := t.TempDir()
	s := newStore(t, root, &locator{})
	if err := s.Write(reviewreceipt.Destination{Name: "x"}, "review-a.json", []byte("{}")); err == nil {
		t.Error("Write accepted a destination of no kind")
	}
	if _, err := s.Read(reviewreceipt.Destination{Name: "x"}, "review-a.json"); err == nil || errors.Is(err, reviewreceipt.ErrNotPersisted) {
		t.Errorf("Read of a destination of no kind = %v, want a refusal", err)
	}
	if entries, _ := os.ReadDir(root); len(entries) != 0 {
		t.Errorf("a refused destination left %v behind", entries)
	}
}
