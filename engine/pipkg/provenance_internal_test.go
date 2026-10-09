package pipkg

import (
	"errors"
	"testing"
)

// scriptedSource answers the three value questions provenance asks from fixed results.
type scriptedSource struct {
	NoRepository
	changes    bool
	changesErr error
	resolveID  string
	resolveErr error
	tag        string
	tagErr     error
}

func (s scriptedSource) HasChanges(string, ...string) (bool, error) { return s.changes, s.changesErr }
func (s scriptedSource) Resolve(string, string) (string, error)     { return s.resolveID, s.resolveErr }
func (s scriptedSource) LatestTag(string, string, string) (string, error) {
	return s.tag, s.tagErr
}

type stopped struct{}

func (stopped) Error() string     { return "context deadline exceeded" }
func (stopped) Unavailable() bool { return true }

// A value comes back with a nil error, or the zero value with an error, never both.
func TestProvenanceNeverReturnsAValueWithAnError(t *testing.T) {
	none := errors.New("exit status 128")

	t.Run("version: no tag is the development version", func(t *testing.T) {
		got, err := Packages{Source: scriptedSource{tagErr: none}}.resolvePackageVersion("/o", "abc")
		if got != developmentVersion || err != nil {
			t.Fatalf("= %q, %v, want the development version and no error", got, err)
		}
	})
	t.Run("version: a git that could not answer is the zero value and an error", func(t *testing.T) {
		got, err := Packages{Source: scriptedSource{tagErr: stopped{}}}.resolvePackageVersion("/o", "abc")
		if got != "" || err == nil || !errors.Is(err, stopped{}) {
			t.Fatalf("= %q, %v, want no value and an error wrapping the stop", got, err)
		}
	})
	t.Run("version: a tag loses its v", func(t *testing.T) {
		if got, err := (Packages{Source: scriptedSource{tag: "v1.2.3"}}).resolvePackageVersion("/o", ""); got != "1.2.3" || err != nil {
			t.Fatalf("= %q, %v", got, err)
		}
	})
	t.Run("rev: HEAD that names nothing records no commit", func(t *testing.T) {
		got, err := Packages{Source: scriptedSource{resolveErr: none}}.resolveBuildRev("/o")
		if got != "" || err != nil {
			t.Fatalf("= %q, %v, want no commit and no error", got, err)
		}
	})
	t.Run("rev: a git that could not answer is the zero value and an error", func(t *testing.T) {
		got, err := Packages{Source: scriptedSource{resolveErr: stopped{}}}.resolveBuildRev("/o")
		if got != "" || err == nil {
			t.Fatalf("= %q, %v, want no commit and an error", got, err)
		}
	})
	t.Run("rev: a status that could not be taken is the zero value and an error, and HEAD is not asked", func(t *testing.T) {
		got, err := Packages{Source: scriptedSource{changesErr: stopped{}, resolveID: "abc"}}.resolveBuildRev("/o")
		if got != "" || err == nil {
			t.Fatalf("= %q, %v, want no commit and an error", got, err)
		}
	})
	t.Run("rev: a dirty tree records no commit and is not an error", func(t *testing.T) {
		got, err := Packages{Source: scriptedSource{changes: true, resolveID: "abc"}}.resolveBuildRev("/o")
		if got != "" || err != nil {
			t.Fatalf("= %q, %v", got, err)
		}
	})
	t.Run("dirty: an answer of error from git is a clean tree, a stop is an error", func(t *testing.T) {
		if dirty, err := (Packages{Source: scriptedSource{changesErr: none}}).isSourceDirty("/o"); dirty || err != nil {
			t.Errorf("answered error = %v, %v, want clean and no error", dirty, err)
		}
		if dirty, err := (Packages{Source: scriptedSource{changes: true, changesErr: stopped{}}}).isSourceDirty("/o"); dirty || err == nil {
			t.Errorf("stopped = %v, %v, want the zero value and an error", dirty, err)
		}
	})
}
