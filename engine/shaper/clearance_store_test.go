package shaper

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain isolates every state and config location this package could
// resolve, so no test can reach the real user store or a real Pi binary even
// if it forgets its own overrides.
func TestMain(m *testing.M) {
	os.Exit(runIsolated(m))
}

func runIsolated(m *testing.M) int {
	dir, err := os.MkdirTemp("", "shaper-test-env-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create isolated test env: %v\n", err)
		return 1
	}
	defer os.RemoveAll(dir)
	for key, value := range map[string]string{
		"HOME":             filepath.Join(dir, "home"),
		"XDG_STATE_HOME":   filepath.Join(dir, "state"),
		"XDG_CONFIG_HOME":  filepath.Join(dir, "config"),
		"LABDRIAN_PI_BIN":  filepath.Join(dir, "fake-pi-not-present"),
		"LABDRIAN_TESTING": "1",
	} {
		if err := os.Setenv(key, value); err != nil {
			fmt.Fprintf(os.Stderr, "set %s: %v\n", key, err)
			return 1
		}
	}
	return m.Run()
}

// isolatedStore points XDG_STATE_HOME at a fresh temp dir and returns a store
// resolved from it.
func isolatedStore(t *testing.T) (FileStore, string) {
	t.Helper()
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	s, err := NewFileStore()
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	return s, state
}

func storedRecord(t *testing.T) (ClearanceRecord, []byte) {
	t.Helper()
	r := recordFor(t, freshAssessment(t, overlapInput(t)))
	return r, marshalRecord(t, r)
}

func mode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat %s: %v", path, err)
	}
	return info.Mode()
}

func TestFileStoreResolvesDecidedPath(t *testing.T) {
	sha := strings.Repeat("a", 64)
	t.Run("xdg state home", func(t *testing.T) {
		s, state := isolatedStore(t)
		got, err := s.Path("proj", "goal-alpha", sha)
		if err != nil {
			t.Fatalf("Path: %v", err)
		}
		want := filepath.Join(state, "labdrian", "shaper-clearance", "proj", "goal-alpha", sha+".json")
		if got != want {
			t.Errorf("Path = %q, want %q", got, want)
		}
	})
	t.Run("home fallback", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("XDG_STATE_HOME", "")
		s, err := NewFileStore()
		if err != nil {
			t.Fatalf("NewFileStore: %v", err)
		}
		got, err := s.Path("proj", "goal-alpha", sha)
		if err != nil {
			t.Fatalf("Path: %v", err)
		}
		want := filepath.Join(home, ".local", "state", "labdrian", "shaper-clearance", "proj", "goal-alpha", sha+".json")
		if got != want {
			t.Errorf("Path = %q, want %q", got, want)
		}
	})
	t.Run("relative xdg state home refused", func(t *testing.T) {
		t.Setenv("XDG_STATE_HOME", "relative/state")
		if _, err := NewFileStore(); err == nil {
			t.Errorf("NewFileStore accepted a relative XDG_STATE_HOME")
		}
	})
	t.Run("no home refused", func(t *testing.T) {
		t.Setenv("XDG_STATE_HOME", "")
		t.Setenv("HOME", "")
		if _, err := NewFileStore(); err == nil {
			t.Errorf("NewFileStore accepted an empty HOME with no XDG_STATE_HOME")
		}
	})
}

func TestFileStoreRefusesUnsafePathComponents(t *testing.T) {
	s, _ := isolatedStore(t)
	sha := strings.Repeat("b", 64)
	for _, tc := range []struct {
		name, project, goal, sha string
	}{
		{name: "empty project", project: "", goal: "g", sha: sha},
		{name: "dot project", project: ".", goal: "g", sha: sha},
		{name: "dotdot goal", project: "p", goal: "..", sha: sha},
		{name: "slash project", project: "a/b", goal: "g", sha: sha},
		{name: "backslash goal", project: "p", goal: `a\b`, sha: sha},
		{name: "nul goal", project: "p", goal: "a\x00b", sha: sha},
		{name: "short sha", project: "p", goal: "g", sha: "abc"},
		{name: "uppercase sha", project: "p", goal: "g", sha: strings.Repeat("B", 64)},
		{name: "traversing sha", project: "p", goal: "g", sha: "../" + sha[3:]},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if p, err := s.Path(tc.project, tc.goal, tc.sha); err == nil {
				t.Errorf("Path accepted (%q, %q, %q) as %q", tc.project, tc.goal, tc.sha, p)
			}
			if _, err := s.Get(tc.project, tc.goal, tc.sha); err == nil {
				t.Errorf("Get accepted (%q, %q, %q)", tc.project, tc.goal, tc.sha)
			}
		})
	}
}

func TestFileStorePutWritesPrivateImmutableRecord(t *testing.T) {
	s, state := isolatedStore(t)
	r, data := storedRecord(t)

	path, err := s.Put(data)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	want := filepath.Join(state, "labdrian", "shaper-clearance", r.Subject.ProjectID, r.Subject.GoalID, r.Subject.HandoffSHA256+".json")
	if path != want {
		t.Errorf("Put path = %q, want %q", path, want)
	}
	if m := mode(t, path); !m.IsRegular() || m.Perm() != 0o600 {
		t.Errorf("record mode = %v, want regular 0600", m)
	}
	for _, dir := range []string{
		filepath.Join(state, "labdrian"),
		filepath.Join(state, "labdrian", "shaper-clearance"),
		filepath.Join(state, "labdrian", "shaper-clearance", r.Subject.ProjectID),
		filepath.Join(state, "labdrian", "shaper-clearance", r.Subject.ProjectID, r.Subject.GoalID),
	} {
		if m := mode(t, dir); !m.IsDir() || m.Perm() != 0o700 {
			t.Errorf("%s mode = %v, want directory 0700", dir, m)
		}
	}
	got, err := s.Get(r.Subject.ProjectID, r.Subject.GoalID, r.Subject.HandoffSHA256)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("Get returned %q, want the stored bytes", got)
	}

	again, err := s.Put(data)
	if err != nil {
		t.Fatalf("Put identical bytes again: %v", err)
	}
	if again != path {
		t.Errorf("idempotent Put path = %q, want %q", again, path)
	}

	declined := r
	declined.Decision = DecisionDecline
	if _, err := s.Put(marshalRecord(t, declined)); err == nil {
		t.Fatalf("Put replaced a stored record with different bytes")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read stored record: %v", err)
	}
	if !bytes.Equal(after, data) {
		t.Errorf("stored record changed after a refused Put")
	}

	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("read store dir: %v", err)
	}
	if len(entries) != 1 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("store dir holds %v, want only the record (no temp leftovers)", names)
	}
}

func TestFileStorePutRefusesInvalidRecord(t *testing.T) {
	s, state := isolatedStore(t)
	for _, data := range [][]byte{[]byte(`{}`), []byte(`not json`), []byte(`{"version":1,"extra":true}`)} {
		if _, err := s.Put(data); err == nil {
			t.Errorf("Put accepted invalid record %q", data)
		}
	}
	if _, err := os.Lstat(filepath.Join(state, "labdrian")); !os.IsNotExist(err) {
		t.Errorf("Put created store directories for an invalid record (err %v)", err)
	}
}

func TestFileStoreGetMissingRecordFails(t *testing.T) {
	s, _ := isolatedStore(t)
	if _, err := s.Get("proj", "goal-alpha", strings.Repeat("c", 64)); err == nil {
		t.Errorf("Get returned no error for a missing record")
	}
}

func TestFileStoreRefusesSymlinkedComponents(t *testing.T) {
	r, data := storedRecord(t)
	recordRel := []string{"labdrian", "shaper-clearance", r.Subject.ProjectID, r.Subject.GoalID, r.Subject.HandoffSHA256 + ".json"}
	for _, tc := range []struct {
		name string
		// link returns the store-relative path of the component replaced by
		// a symlink to an outside directory (or file).
		link []string
		file bool
	}{
		{name: "labdrian dir", link: []string{"labdrian"}},
		{name: "store root", link: []string{"labdrian", "shaper-clearance"}},
		{name: "project dir", link: []string{"labdrian", "shaper-clearance", r.Subject.ProjectID}},
		{name: "goal dir", link: []string{"labdrian", "shaper-clearance", r.Subject.ProjectID, r.Subject.GoalID}},
		{name: "record file", link: []string{"labdrian", "shaper-clearance", r.Subject.ProjectID, r.Subject.GoalID, r.Subject.HandoffSHA256 + ".json"}, file: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, state := isolatedStore(t)
			outside := t.TempDir()
			target := outside
			if tc.file {
				target = filepath.Join(outside, "record.json")
				if err := os.WriteFile(target, data, 0o600); err != nil {
					t.Fatalf("write outside record: %v", err)
				}
			}
			linkPath := filepath.Join(append([]string{state}, tc.link...)...)
			if err := os.MkdirAll(filepath.Dir(linkPath), 0o700); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.Symlink(target, linkPath); err != nil {
				t.Fatalf("symlink: %v", err)
			}
			if _, err := s.Put(data); err == nil {
				t.Errorf("Put followed a symlinked %s", tc.name)
			}
			if !tc.file {
				entries, err := os.ReadDir(outside)
				if err != nil {
					t.Fatalf("read outside dir: %v", err)
				}
				if len(entries) != 0 {
					t.Errorf("Put wrote %d entries through the symlink", len(entries))
				}
				// Place a valid record behind the symlinked directory, so
				// Get can only fail by refusing the symlink itself rather
				// than because nothing exists at the resolved path.
				rest := recordRel[len(tc.link):]
				behind := filepath.Join(append([]string{outside}, rest...)...)
				if err := os.MkdirAll(filepath.Dir(behind), 0o700); err != nil {
					t.Fatalf("mkdir behind symlink: %v", err)
				}
				if err := os.WriteFile(behind, data, 0o600); err != nil {
					t.Fatalf("write record behind symlink: %v", err)
				}
				if _, err := os.Stat(filepath.Join(append([]string{state}, recordRel...)...)); err != nil {
					t.Fatalf("record is not reachable through the symlink: %v", err)
				}
			}
			if _, err := s.Get(r.Subject.ProjectID, r.Subject.GoalID, r.Subject.HandoffSHA256); err == nil {
				t.Errorf("Get followed a symlinked %s", tc.name)
			}
		})
	}
}

func TestFileStoreRefusesNonRegularRecord(t *testing.T) {
	s, _ := isolatedStore(t)
	r, data := storedRecord(t)
	path, err := s.Path(r.Subject.ProjectID, r.Subject.GoalID, r.Subject.HandoffSHA256)
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatalf("mkdir at record path: %v", err)
	}
	if _, err := s.Put(data); err == nil {
		t.Errorf("Put accepted a directory at the record path")
	}
	if _, err := s.Get(r.Subject.ProjectID, r.Subject.GoalID, r.Subject.HandoffSHA256); err == nil {
		t.Errorf("Get accepted a directory at the record path")
	}
}
