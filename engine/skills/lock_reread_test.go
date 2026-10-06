package skills

// What a shared lock on a lock file that does not exist yet can and cannot cover is said at
// rereadsWhenTheLockFileAppears. validate, the reader that reads several files which must agree,
// is a use case now: its reads under the locks are held to ReadConsistently (lock_acquire_test.go)
// and to the adapter (engine/cmd/skills_validate_test.go). What remains here is the check on its
// own.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The check answers for several lock files, and each answer is independent of the
// others: a file that appeared is reported as appeared even when another file in the
// list cannot be inspected, and an inspection failure is reported even when another
// file appeared. The two signals used to overwrite each other, in an order that
// depended on where the paths sat in the list. Only the first kind of signal says the
// read is torn, only the second says the check cannot tell, and a caller that is
// handed both must be able to see both. validate asks about one file, so the verbs
// cannot reach a list of two; the contract is pinned on the function itself.
func TestRereadsWhenTheLockFileAppearsReportsEachSignalWhateverTheOrder(t *testing.T) {
	dir := t.TempDir()
	absent := filepath.Join(dir, "absent.lock")
	appeared := filepath.Join(dir, "appeared.lock")
	writeTestFile(t, appeared, "")
	loop := filepath.Join(dir, "loop.lock")
	makeSymlink(t, loop, loop) // a link that points at itself
	if _, err := os.Stat(loop); err == nil || os.IsNotExist(err) {
		t.Fatalf("a symbolic link that points at itself gave %v, want a stat failure that is not 'does not exist'", err)
	}

	for _, tc := range []struct {
		name         string
		paths        []string
		wantAppeared bool
		wantErrAbout string // a path the error must name; empty for no error
	}{
		{"nothing to check", nil, false, ""},
		{"every file still absent", []string{absent}, false, ""},
		{"one file appeared", []string{absent, appeared}, true, ""},
		{"one file cannot be inspected", []string{absent, loop}, false, loop},
		{"one appeared, then one cannot be inspected", []string{appeared, loop}, true, loop},
		{"one cannot be inspected, then one appeared", []string{loop, appeared}, true, loop},
		{"one appeared between two that cannot be inspected", []string{loop, appeared, loop}, true, loop},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotAppeared, err := rereadsWhenTheLockFileAppears(noopLocker{}, tc.paths)

			if gotAppeared != tc.wantAppeared {
				t.Errorf("appeared = %v, want %v: a signal was dropped", gotAppeared, tc.wantAppeared)
			}
			switch {
			case tc.wantErrAbout == "" && err != nil:
				t.Errorf("error = %v, want none", err)
			case tc.wantErrAbout != "" && err == nil:
				t.Errorf("no error, want one that names %s: an inspection failure was dropped", tc.wantErrAbout)
			case tc.wantErrAbout != "" && !strings.Contains(err.Error(), tc.wantErrAbout):
				t.Errorf("error = %v, want it to name %s", err, tc.wantErrAbout)
			}
		})
	}
}
