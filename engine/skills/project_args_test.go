package skills

// The lock layer must lock exactly the directory the verb then works in. They used
// to read --project-root separately, each with its own idea of a flag's value, so a
// spelling one of them took differently locked a directory the verb never used (or
// did not lock the one it used). Now both read it with parseProjectArgs. These tests
// drive the real verb and the real lock layer with the spellings that used to
// diverge, and compare the directories locked with the ones the verb read its
// project lock file from.

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
)

// rootsOfProjectLockReads returns a readFile that records the project root of every
// project lock file the verb reads, and the function that lists them.
func rootsOfProjectLockReads() (readFileFn, func() []string) {
	var mu sync.Mutex
	seen := map[string]bool{}
	read := func(name string) ([]byte, error) {
		if strings.HasSuffix(filepath.ToSlash(name), "/"+ProjectLockRelPath) {
			mu.Lock()
			seen[filepath.Dir(filepath.Dir(name))] = true
			mu.Unlock()
		}
		return os.ReadFile(name)
	}
	roots := func() []string {
		mu.Lock()
		defer mu.Unlock()
		out := make([]string, 0, len(seen))
		for r := range seen {
			out = append(out, r)
		}
		sort.Strings(out)
		return out
	}
	return read, roots
}

// lockedProjectRoots lists the directories a recordingLocker was asked to lock.
func lockedProjectRoots(l *recordingLocker) []string {
	var out []string
	for _, e := range l.log() {
		if dir, ok := strings.CutPrefix(e, "lockdir exclusive "); ok {
			out = append(out, dir)
		}
	}
	sort.Strings(out)
	return out
}

// Every verb that reads --project-root is in projectArgSpecs, the table the lock layer
// looks verbs up in, and is locked on that root: a verb added to the table without a
// lock, or locked without being in the table, would run beside writers of the project.
func TestEveryProjectVerbIsLockedOnItsOwnRoot(t *testing.T) {
	root := t.TempDir()
	want := map[string]LockMode{
		"project-register": LockExclusive,
		"project-revise":   LockExclusive,
		"project-retire":   LockExclusive,
		"project-status":   LockShared,
	}
	if len(projectArgSpecs) != len(want) {
		t.Errorf("projectArgSpecs has %d verbs, this test knows %d", len(projectArgSpecs), len(want))
	}
	for verb, mode := range want {
		if _, ok := projectArgSpecs[verb]; !ok {
			t.Errorf("%s is not in projectArgSpecs", verb)
			continue
		}
		requests := lockRequestsFor(verb, []string{verb, "--project-root", root}, "")
		if len(requests) != 1 || !requests[0].Dir || requests[0].Path != root || requests[0].Mode != mode {
			t.Errorf("%s: lock requests = %+v, want one %v lock on %s", verb, requests, modeName(mode), root)
		}
	}
}

func TestTheLockLayerAndTheProjectVerbsReadTheSameProjectRoot(t *testing.T) {
	e := newProjectCLIEnv(t, "tidy-worktree", projectCLIRegistry)
	if _, errOut, code := runProjectRegister(t, registerArgs(e)); code != 0 {
		t.Fatalf("setup registration: exit %d, stderr %q", code, errOut)
	}
	other := t.TempDir()
	reg := []string{"--registry", e.registryPath}
	id := "tidy-worktree"

	for _, tc := range []struct {
		name string
		args []string
		// reads says whether the verb gets as far as reading the project's lock file,
		// which is what the case is for; a spelling the verb refuses reads nothing,
		// and must then lock nothing.
		reads bool
	}{
		{"one root", append(append([]string{"--project-root", e.root}, reg...), "--dry-run", id), true},
		{"the last of two roots is the one used", append(append([]string{"--project-root", other, "--project-root", e.root}, reg...), "--dry-run", id), true},
		{"an unclean spelling of the root", append(append([]string{"--project-root", e.root + string(filepath.Separator) + ".." + string(filepath.Separator) + filepath.Base(e.root)}, reg...), "--dry-run", id), true},
		{"the end of options after the root", append(append([]string{"--project-root", e.root}, reg...), "--dry-run", "--", id), true},
		{"a flag that swallows the next one as its value", append(append([]string{"--reason", "--project-root", e.root}, reg...), "--dry-run", id), false},
		{"a root only after the end of options", append(append([]string{"--"}, "--project-root", e.root), append(reg, id)...), false},
		{"a relative root", append(append([]string{"--project-root", "relative/dir"}, reg...), "--dry-run", id), false},
		{"a root spelled with an equals sign", append(append([]string{"--project-root=" + e.root}, reg...), "--dry-run", id), false},
		{"a root given as a flag's value", append(append([]string{"--project-root", "--dry-run"}, reg...), id), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			read, readRoots := rootsOfProjectLockReads()
			locker := &recordingLocker{}

			r := runAt("project-retire", tc.args, read, nil, locker)

			locked, read2 := lockedProjectRoots(locker), readRoots()
			if tc.reads {
				if r.code != 0 || len(read2) != 1 {
					t.Fatalf("the verb should have read one project lock: exit %d, read %v, stderr %q", r.code, read2, r.stderr)
				}
			} else if len(read2) != 0 {
				t.Fatalf("the verb should have refused before reading a project lock, but read %v", read2)
			}
			want := read2
			for i := range want {
				want[i] = filepath.Clean(want[i])
			}
			if len(locked) == 0 && len(want) == 0 {
				return
			}
			if !reflect.DeepEqual(locked, want) {
				t.Errorf("locked %v, but the verb worked in %v (exit %d, stderr %q)", locked, want, r.code, r.stderr)
			}
		})
	}
}
