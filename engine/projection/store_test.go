package projection_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

var t0 = time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)

// isolatedStore points XDG_STATE_HOME at a fresh temporary directory and
// returns a store over it with that directory. The package TestMain already
// isolates HOME; this gives each test a state home of its own.
func isolatedStore(t *testing.T) (projection.Store, string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_STATE_HOME", root)
	s, err := projection.NewStore()
	if err != nil {
		t.Fatalf("NewStore() = %v, want nil", err)
	}
	return s, root
}

func bindingsDir(root string) string { return filepath.Join(root, "labdrian", "bindings") }

func bindingPath(root, key string) string { return filepath.Join(bindingsDir(root), key+".json") }

// plant writes content as the binding file of key, creating the directories
// with the modes the store itself uses, and returns its path.
func plant(t *testing.T, root, key, content string) string {
	t.Helper()
	if err := os.MkdirAll(bindingsDir(root), 0o700); err != nil {
		t.Fatalf("create bindings directory: %v", err)
	}
	path := bindingPath(root, key)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("plant %s: %v", path, err)
	}
	return path
}

func readBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

// names lists the entries of dir, sorted, so a test can assert exactly what
// the store left behind (in particular, that it left no temporary file).
func names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read directory %s: %v", dir, err)
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".lock") { // persistent by design (see Store)
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

func mustBind(t *testing.T, s projection.Store, key, project, workflowID string, at time.Time) {
	t.Helper()
	if err := s.Bind(key, project, workflowID, at, false); err != nil {
		t.Fatalf("Bind(%s/%s) = %v, want nil", project, workflowID, err)
	}
}

func mustLoad(t *testing.T, s projection.Store, key string) projection.Loaded {
	t.Helper()
	loaded, err := s.Load(key)
	if err != nil {
		t.Fatalf("Load() = %v, want nil (a state that cannot be read is reported through Classification)", err)
	}
	return loaded
}

func skipIfPermissionsAreNotEnforced(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not meaningful on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses permission checks")
	}
}

// --- the state home and the store's location ------------------------------

func TestNewStoreWritesUnderXDGStateHome(t *testing.T) {
	s, root := isolatedStore(t)
	mustBind(t, s, hex64("a"), "proj-1", "wf-1", t0)

	want := filepath.Join(root, "labdrian", "bindings", hex64("a")+".json")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("binding not at %s: %v", want, err)
	}
	// The workflow store resolves the same variable through the same helper,
	// so the two can never disagree about where the state home is.
	if home, err := workflow.StateHome(); err != nil || home != root {
		t.Fatalf("workflow.StateHome() = %q, %v, want %q", home, err, root)
	}
}

func TestNewStoreFallsBackToHomeLocalState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", home)
	s, err := projection.NewStore()
	if err != nil {
		t.Fatalf("NewStore() = %v, want nil", err)
	}
	mustBind(t, s, hex64("a"), "proj-1", "wf-1", t0)

	want := filepath.Join(home, ".local", "state", "labdrian", "bindings", hex64("a")+".json")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("binding not at %s: %v", want, err)
	}
}

func TestNewStoreRefusesAnUnusableEnvironment(t *testing.T) {
	tests := []struct {
		name string
		xdg  string
		home string
		want string
	}{
		{"relative XDG_STATE_HOME", "relative/path", "/home/someone", "XDG_STATE_HOME"},
		{"relative HOME fallback", "", "relative/home", "HOME"},
		{"unset HOME fallback", "", "", "HOME"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", tt.xdg)
			t.Setenv("HOME", tt.home)
			s, err := projection.NewStore()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("NewStore() = %v, want an error naming %s", err, tt.want)
			}
			// A refused store must not be usable by accident.
			if _, loadErr := s.Load(hex64("a")); !errors.Is(loadErr, projection.ErrStoreNotInitialized) {
				t.Errorf("Load() on the store returned with the error = %v, want ErrStoreNotInitialized", loadErr)
			}
		})
	}
}

func TestZeroStoreIsNotInitialized(t *testing.T) {
	var s projection.Store
	if _, err := s.Load(hex64("a")); !errors.Is(err, projection.ErrStoreNotInitialized) {
		t.Errorf("Load() = %v, want ErrStoreNotInitialized", err)
	}
	if err := s.Bind(hex64("a"), "proj-1", "wf-1", t0, false); !errors.Is(err, projection.ErrStoreNotInitialized) {
		t.Errorf("Bind() = %v, want ErrStoreNotInitialized", err)
	}
	if _, err := s.Unbind(hex64("a")); !errors.Is(err, projection.ErrStoreNotInitialized) {
		t.Errorf("Unbind() = %v, want ErrStoreNotInitialized", err)
	}
}

// --- Load ------------------------------------------------------------------

func TestLoadIsAbsentAndWritesNothing(t *testing.T) {
	t.Run("no state directory at all", func(t *testing.T) {
		s, root := isolatedStore(t)
		loaded := mustLoad(t, s, hex64("a"))
		if loaded.Classification != projection.ClassificationAbsent || loaded.Detail != "" {
			t.Fatalf("Load() = %+v, want absent with no detail", loaded)
		}
		if got := names(t, root); len(got) != 0 {
			t.Fatalf("Load() created %v under the state home, want nothing: it is read-only", got)
		}
	})
	t.Run("an empty bindings directory", func(t *testing.T) {
		s, root := isolatedStore(t)
		if err := os.MkdirAll(bindingsDir(root), 0o700); err != nil {
			t.Fatal(err)
		}
		if loaded := mustLoad(t, s, hex64("a")); loaded.Classification != projection.ClassificationAbsent {
			t.Fatalf("Load() = %+v, want absent", loaded)
		}
	})
	t.Run("a binding for another repository is not this one's", func(t *testing.T) {
		s, _ := isolatedStore(t)
		mustBind(t, s, hex64("b"), "proj-1", "wf-1", t0)
		if loaded := mustLoad(t, s, hex64("a")); loaded.Classification != projection.ClassificationAbsent {
			t.Fatalf("Load(a) = %+v, want absent while only b is bound", loaded)
		}
	})
}

func TestLoadRefusesMalformedRepoKeys(t *testing.T) {
	s, root := isolatedStore(t)
	for _, key := range []string{
		"",
		strings.Repeat("a", 63),
		strings.Repeat("a", 65),
		strings.Repeat("A", 64),
		strings.Repeat("g", 64),
		"../../etc/passwd",
		"../" + strings.Repeat("a", 61),
		strings.Repeat("a", 63) + "/",
		strings.Repeat("a", 63) + "\n",
	} {
		loaded, err := s.Load(key)
		if err == nil {
			t.Errorf("Load(%q) = %+v, want an error: a key that is not 64 lowercase hex digits must never build a path", key, loaded)
		}
	}
	if got := names(t, root); len(got) != 0 {
		t.Fatalf("refused keys touched the state home: %v", got)
	}
}

func TestLoadReturnsTheStoredBindingAsOwned(t *testing.T) {
	s, _ := isolatedStore(t)
	mustBind(t, s, hex64("a"), "proj-1", "wf-1", t0)
	loaded := mustLoad(t, s, hex64("a"))
	want := projection.Binding{Version: 1, RepoKey: hex64("a"), ProjectID: "proj-1", WorkflowID: "wf-1", BoundAt: "2026-09-29T10:00:00Z"}
	if loaded.Classification != projection.ClassificationOwned || loaded.Binding != want || loaded.Detail != "" {
		t.Fatalf("Load() = %+v, want owned with %+v and no detail", loaded, want)
	}
}

func TestLoadClassifiesWhatIsOnDisk(t *testing.T) {
	valid := rawBinding(hex64("a"))
	indented, err := validBinding().Marshal()
	if err != nil {
		t.Fatal(err)
	}
	atCap := valid + strings.Repeat(" ", projection.MaxBindingBytes-len(valid))

	tests := []struct {
		name    string
		content string
		want    projection.Classification
		detail  string // a fragment of the detail, when the classification has one worth pinning
	}{
		// Owned: our schema, naming the key in its file name.
		{"the compact document", valid, projection.ClassificationOwned, ""},
		{"the document Marshal writes", string(indented), projection.ClassificationOwned, ""},
		{"a document with surrounding whitespace", "\n  " + valid + "  \n\n", projection.ClassificationOwned, ""},
		{"a document of exactly the size cap", atCap, projection.ClassificationOwned, ""},

		// Foreign: valid JSON that is not our schema, or names another key.
		{"an unrelated object", `{"hello":"world"}` + "\n", projection.ClassificationForeign, ""},
		{"an empty object", "{}\n", projection.ClassificationForeign, ""},
		{"a newer version", strings.Replace(valid, `"version":1`, `"version":2`, 1), projection.ClassificationForeign, "version"},
		{"an unknown field", strings.Replace(valid, `"version":1,`, `"version":1,"extra":true,`, 1), projection.ClassificationForeign, `"extra"`},
		{"a duplicate key", strings.Replace(valid, `"version":1,`, `"version":1,"version":1,`, 1), projection.ClassificationForeign, "duplicate"},
		{"a case-variant field name", strings.Replace(valid, `"project_id"`, `"Project_ID"`, 1), projection.ClassificationForeign, ""},
		{"a wrongly typed field", strings.Replace(valid, `"version":1`, `"version":"1"`, 1), projection.ClassificationForeign, ""},
		{"an invalid identifier", strings.Replace(valid, `"proj-1"`, `"../escape"`, 1), projection.ClassificationForeign, "project_id"},
		{"a binding for another repository", rawBinding(hex64("b")), projection.ClassificationForeign, hex64("b")},
		{"a JSON array", "[]\n", projection.ClassificationForeign, ""},
		{"a JSON string", `"binding"` + "\n", projection.ClassificationForeign, ""},
		{"a JSON number", "42\n", projection.ClassificationForeign, ""},
		{"JSON null", "null\n", projection.ClassificationForeign, ""},

		// Malformed: not one valid UTF-8 JSON document within the size cap.
		{"an empty file", "", projection.ClassificationMalformed, "empty"},
		{"a whitespace-only file", " \n\t\n", projection.ClassificationMalformed, ""},
		{"plain text", "not json\n", projection.ClassificationMalformed, ""},
		{"a truncated document", valid[:len(valid)-12], projection.ClassificationMalformed, ""},
		{"trailing data after the document", valid + "\n{}\n", projection.ClassificationMalformed, ""},
		{"two documents", valid + "\n" + valid + "\n", projection.ClassificationMalformed, ""},
		{"trailing garbage", valid + "\nx", projection.ClassificationMalformed, ""},
		{"invalid UTF-8", strings.Replace(valid, "proj-1", "proj-\xff", 1), projection.ClassificationMalformed, "UTF-8"},
		{"a byte order mark", string(rune(0xFEFF)) + valid, projection.ClassificationMalformed, ""},
		{"a document one byte over the size cap", atCap + " ", projection.ClassificationMalformed, "maximum"},
		{"a file far over the size cap", strings.Repeat("x", 1<<20), projection.ClassificationMalformed, "maximum"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, root := isolatedStore(t)
			plant(t, root, hex64("a"), tt.content)

			loaded := mustLoad(t, s, hex64("a"))
			if loaded.Classification != tt.want {
				t.Fatalf("Load() = %+v, want classification %q", loaded, tt.want)
			}
			if tt.want == projection.ClassificationOwned {
				if loaded.Binding != validBinding() || loaded.Detail != "" {
					t.Fatalf("Load() = %+v, want the parsed binding and no detail", loaded)
				}
				return
			}
			if loaded.Binding != (projection.Binding{}) {
				t.Errorf("Load() carries the binding %+v for a %s file, want the zero Binding", loaded.Binding, tt.want)
			}
			if loaded.Detail == "" {
				t.Errorf("Load() has no detail for a %s file, want the reason", tt.want)
			}
			if tt.detail != "" && !strings.Contains(loaded.Detail, tt.detail) {
				t.Errorf("Load() detail = %q, want it to contain %q", loaded.Detail, tt.detail)
			}
		})
	}
}

// unavailableCases are the states in which nothing can be known about the
// file: it cannot be read, or the path to it is not the plain directory and
// regular file the store writes. Each returns the store and the path whose
// state the test must find unchanged afterwards.
var unavailableCases = []struct {
	name  string
	setup func(t *testing.T) (projection.Store, string)
}{
	{"the binding path is a directory", func(t *testing.T) (projection.Store, string) {
		s, root := isolatedStore(t)
		path := bindingPath(root, hex64("a"))
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
		return s, path
	}},
	{"the binding path is a symlink to a valid binding", func(t *testing.T) (projection.Store, string) {
		s, root := isolatedStore(t)
		target := filepath.Join(t.TempDir(), "elsewhere.json")
		if err := os.WriteFile(target, []byte(rawBinding(hex64("a"))), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(bindingsDir(root), 0o700); err != nil {
			t.Fatal(err)
		}
		path := bindingPath(root, hex64("a"))
		if err := os.Symlink(target, path); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		return s, path
	}},
	{"the binding path is a dangling symlink", func(t *testing.T) (projection.Store, string) {
		s, root := isolatedStore(t)
		if err := os.MkdirAll(bindingsDir(root), 0o700); err != nil {
			t.Fatal(err)
		}
		path := bindingPath(root, hex64("a"))
		if err := os.Symlink(filepath.Join(t.TempDir(), "missing.json"), path); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		return s, path
	}},
	{"the bindings directory is a symlink", func(t *testing.T) (projection.Store, string) {
		s, root := isolatedStore(t)
		real := t.TempDir()
		if err := os.WriteFile(filepath.Join(real, hex64("a")+".json"), []byte(rawBinding(hex64("a"))), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "labdrian"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(real, bindingsDir(root)); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		return s, bindingsDir(root)
	}},
	{"the labdrian directory is a symlink", func(t *testing.T) (projection.Store, string) {
		s, root := isolatedStore(t)
		real := t.TempDir()
		if err := os.Symlink(real, filepath.Join(root, "labdrian")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		return s, filepath.Join(root, "labdrian")
	}},
	{"the state home is a regular file", func(t *testing.T) (projection.Store, string) {
		file := filepath.Join(t.TempDir(), "not-a-directory")
		if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("XDG_STATE_HOME", file)
		s, err := projection.NewStore()
		if err != nil {
			t.Fatalf("NewStore() = %v, want nil: the state home is only checked when the store is used", err)
		}
		return s, file
	}},
	{"the binding file cannot be read", func(t *testing.T) (projection.Store, string) {
		skipIfPermissionsAreNotEnforced(t)
		s, root := isolatedStore(t)
		path := plant(t, root, hex64("a"), rawBinding(hex64("a")))
		if err := os.Chmod(path, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(path, 0o600) })
		return s, path
	}},
}

func TestLoadReportsUnavailableWhenTheStateCannotBeRead(t *testing.T) {
	for _, tc := range unavailableCases {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := tc.setup(t)
			loaded, err := s.Load(hex64("a"))
			if err != nil {
				t.Fatalf("Load() = %v, want nil: unavailable is reported through Classification", err)
			}
			if loaded.Classification != projection.ClassificationUnavailable || loaded.Detail == "" || loaded.Binding != (projection.Binding{}) {
				t.Fatalf("Load() = %+v, want unavailable with a detail and no binding", loaded)
			}
		})
	}
}

// --- Bind ------------------------------------------------------------------

func TestBindWritesTheBindingMarshalProduces(t *testing.T) {
	s, root := isolatedStore(t)
	mustBind(t, s, hex64("a"), "proj-1", "wf-1", t0)

	want, err := validBinding().Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if got := readBytes(t, bindingPath(root, hex64("a"))); !bytes.Equal(got, want) {
		t.Fatalf("file =\n%s\nwant\n%s", got, want)
	}
}

func TestBindStoresTheTimeAsUTCWithoutFractions(t *testing.T) {
	tests := []struct {
		name string
		at   time.Time
		want string
	}{
		{"a UTC time", t0, "2026-09-29T10:00:00Z"},
		{"a time in another zone", time.Date(2026, 9, 29, 12, 0, 0, 0, time.FixedZone("CEST", 2*3600)), "2026-09-29T10:00:00Z"},
		{"a time with sub-second precision", t0.Add(123456789 * time.Nanosecond), "2026-09-29T10:00:00Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := isolatedStore(t)
			mustBind(t, s, hex64("a"), "proj-1", "wf-1", tt.at)
			if got := mustLoad(t, s, hex64("a")).Binding.BoundAt; got != tt.want {
				t.Fatalf("bound_at = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBindCreatesTheDirectoriesAndFileWithPrivateModes(t *testing.T) {
	// A state home that does not exist yet, so the store creates every level.
	root := filepath.Join(t.TempDir(), "state")
	t.Setenv("XDG_STATE_HOME", root)
	s, err := projection.NewStore()
	if err != nil {
		t.Fatal(err)
	}
	mustBind(t, s, hex64("a"), "proj-1", "wf-1", t0)

	for _, dir := range []string{root, filepath.Join(root, "labdrian"), bindingsDir(root)} {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
			t.Errorf("%s: mode %v, err %v, want a directory with mode 0700", dir, info, err)
		}
	}
	info, err := os.Stat(bindingPath(root, hex64("a")))
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		t.Errorf("binding file: mode %v, err %v, want a regular file with mode 0600", info, err)
	}
	info, err = os.Stat(strings.TrimSuffix(bindingPath(root, hex64("a")), ".json") + ".lock")
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		t.Errorf("lock file: mode %v, err %v, want a regular file with mode 0600", info, err)
	}
}

func TestBindLeavesNoTemporaryFileBehind(t *testing.T) {
	s, root := isolatedStore(t)
	want := []string{hex64("a") + ".json"}

	mustBind(t, s, hex64("a"), "proj-1", "wf-1", t0)
	if got := names(t, bindingsDir(root)); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("after Bind: bindings directory holds %v, want %v", got, want)
	}
	if err := s.Bind(hex64("a"), "proj-1", "wf-2", t0.Add(time.Hour), true); err != nil {
		t.Fatalf("replacing Bind() = %v, want nil", err)
	}
	if got := names(t, bindingsDir(root)); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("after a replacing Bind: bindings directory holds %v, want %v", got, want)
	}
}

func TestBindIsIdempotentForTheSameWorkflow(t *testing.T) {
	for _, replace := range []bool{false, true} {
		t.Run(fmt.Sprintf("replace=%v", replace), func(t *testing.T) {
			s, root := isolatedStore(t)
			mustBind(t, s, hex64("a"), "proj-1", "wf-1", t0)
			before := readBytes(t, bindingPath(root, hex64("a")))

			if err := s.Bind(hex64("a"), "proj-1", "wf-1", t0.Add(24*time.Hour), replace); err != nil {
				t.Fatalf("Bind() again = %v, want nil: binding the same workflow twice is a no-op", err)
			}
			if after := readBytes(t, bindingPath(root, hex64("a"))); !bytes.Equal(after, before) {
				t.Fatalf("a repeated Bind rewrote the file (bound_at must keep the first time):\n%s\nvs\n%s", after, before)
			}
		})
	}
}

func TestBindRefusesADifferentWorkflowUnlessReplacing(t *testing.T) {
	tests := []struct {
		name          string
		project, wfID string
		wantInMessage []string
	}{
		{"another workflow of the same project", "proj-1", "wf-2", []string{"proj-1", "wf-1"}},
		{"the same workflow id in another project", "proj-2", "wf-1", []string{"proj-1", "wf-1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, root := isolatedStore(t)
			mustBind(t, s, hex64("a"), "proj-1", "wf-1", t0)
			before := readBytes(t, bindingPath(root, hex64("a")))

			err := s.Bind(hex64("a"), tt.project, tt.wfID, t0.Add(time.Hour), false)
			if !errors.Is(err, projection.ErrAlreadyBound) {
				t.Fatalf("Bind() = %v, want ErrAlreadyBound", err)
			}
			for _, want := range tt.wantInMessage {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not name the bound workflow (missing %q)", err, want)
				}
			}
			if after := readBytes(t, bindingPath(root, hex64("a"))); !bytes.Equal(after, before) {
				t.Fatalf("a refused Bind changed the file:\n%s\nvs\n%s", after, before)
			}
		})
	}
}

func TestBindReplacesADifferentWorkflowWhenAsked(t *testing.T) {
	s, root := isolatedStore(t)
	mustBind(t, s, hex64("a"), "proj-1", "wf-1", t0)

	if err := s.Bind(hex64("a"), "proj-2", "wf-9", t0.Add(time.Hour), true); err != nil {
		t.Fatalf("Bind(replace) = %v, want nil", err)
	}
	loaded := mustLoad(t, s, hex64("a"))
	want := projection.Binding{Version: 1, RepoKey: hex64("a"), ProjectID: "proj-2", WorkflowID: "wf-9", BoundAt: "2026-09-29T11:00:00Z"}
	if loaded.Classification != projection.ClassificationOwned || loaded.Binding != want {
		t.Fatalf("Load() = %+v, want owned with %+v", loaded, want)
	}
	if got := names(t, bindingsDir(root)); len(got) != 1 {
		t.Fatalf("bindings directory holds %v, want exactly the one binding", got)
	}
}

func TestBindKeepsRepositoriesIndependent(t *testing.T) {
	s, _ := isolatedStore(t)
	mustBind(t, s, hex64("a"), "proj-1", "wf-1", t0)
	mustBind(t, s, hex64("b"), "proj-1", "wf-2", t0)

	if got := mustLoad(t, s, hex64("a")).Binding.WorkflowID; got != "wf-1" {
		t.Errorf("repository a is bound to %q, want wf-1", got)
	}
	if got := mustLoad(t, s, hex64("b")).Binding.WorkflowID; got != "wf-2" {
		t.Errorf("repository b is bound to %q, want wf-2", got)
	}
}

func TestBindRefusesInvalidInputWithoutTouchingTheDisk(t *testing.T) {
	tests := []struct {
		name                     string
		key, project, workflowID string
	}{
		{"a short repo key", strings.Repeat("a", 63), "proj-1", "wf-1"},
		{"an uppercase repo key", strings.Repeat("A", 64), "proj-1", "wf-1"},
		{"a repo key that climbs out", "../" + strings.Repeat("a", 61), "proj-1", "wf-1"},
		{"an empty project id", hex64("a"), "", "wf-1"},
		{"a project id with a slash", hex64("a"), "a/b", "wf-1"},
		{"an empty workflow id", hex64("a"), "proj-1", ""},
		{"a hidden workflow id", hex64("a"), "proj-1", ".wf"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, root := isolatedStore(t)
			if err := s.Bind(tt.key, tt.project, tt.workflowID, t0, false); err == nil {
				t.Fatal("Bind() = nil, want an error")
			}
			if got := names(t, root); len(got) != 0 {
				t.Fatalf("a refused Bind created %v under the state home, want nothing", got)
			}
		})
	}
}

func TestBindFailsWithoutLeavingATemporaryFileWhenTheDirectoryIsNotWritable(t *testing.T) {
	skipIfPermissionsAreNotEnforced(t)
	s, root := isolatedStore(t)
	dir := bindingsDir(root)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) })

	if err := s.Bind(hex64("a"), "proj-1", "wf-1", t0, false); err == nil {
		t.Fatal("Bind() = nil, want an error when the directory cannot be written")
	}
	if got := names(t, dir); len(got) != 0 {
		t.Fatalf("a failed Bind left %v behind, want nothing", got)
	}
}

// --- refusing to overwrite what the store does not own ---------------------

// TestBindAndUnbindLeaveForeignAndMalformedFilesUntouched pins the store's
// central promise: a file it does not own is never overwritten or removed,
// not even when the caller asks to replace.
func TestBindAndUnbindLeaveForeignAndMalformedFilesUntouched(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		sentinel error
	}{
		{"an unrelated JSON object", `{"hello":"world"}` + "\n", projection.ErrRefuseForeignBinding},
		{"a newer binding version", strings.Replace(rawBinding(hex64("a")), `"version":1`, `"version":2`, 1), projection.ErrRefuseForeignBinding},
		{"a binding that names another repository", rawBinding(hex64("b")), projection.ErrRefuseForeignBinding},
		{"a JSON array", "[1,2,3]\n", projection.ErrRefuseForeignBinding},
		{"plain text", "hand-written notes\n", projection.ErrRefuseMalformedBinding},
		{"a truncated document", rawBinding(hex64("a"))[:40], projection.ErrRefuseMalformedBinding},
		{"a document followed by more data", rawBinding(hex64("a")) + "\n{}\n", projection.ErrRefuseMalformedBinding},
		{"an empty file", "", projection.ErrRefuseMalformedBinding},
		{"an oversized file", strings.Repeat("x", projection.MaxBindingBytes+1), projection.ErrRefuseMalformedBinding},
	}
	for _, tt := range tests {
		for _, replace := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/replace=%v", tt.name, replace), func(t *testing.T) {
				s, root := isolatedStore(t)
				path := plant(t, root, hex64("a"), tt.content)
				before := readBytes(t, path)

				if err := s.Bind(hex64("a"), "proj-1", "wf-1", t0, replace); !errors.Is(err, tt.sentinel) {
					t.Fatalf("Bind() = %v, want %v", err, tt.sentinel)
				}
				if removed, err := s.Unbind(hex64("a")); removed || !errors.Is(err, tt.sentinel) {
					t.Fatalf("Unbind() = %v, %v, want false and %v", removed, err, tt.sentinel)
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
}

func TestBindAndUnbindRefuseUnavailableStatesAndLeaveThemAlone(t *testing.T) {
	for _, tc := range unavailableCases {
		t.Run(tc.name, func(t *testing.T) {
			s, watched := tc.setup(t)
			before, beforeErr := os.Lstat(watched)
			if beforeErr != nil {
				t.Fatalf("lstat %s: %v", watched, beforeErr)
			}
			var linkTarget string
			if before.Mode()&os.ModeSymlink != 0 {
				linkTarget, _ = os.Readlink(watched)
			}

			if err := s.Bind(hex64("a"), "proj-1", "wf-1", t0, true); !errors.Is(err, projection.ErrBindingUnavailable) {
				t.Errorf("Bind() = %v, want ErrBindingUnavailable", err)
			}
			if removed, err := s.Unbind(hex64("a")); removed || !errors.Is(err, projection.ErrBindingUnavailable) {
				t.Errorf("Unbind() = %v, %v, want false and ErrBindingUnavailable", removed, err)
			}

			after, afterErr := os.Lstat(watched)
			if afterErr != nil {
				t.Fatalf("lstat %s after the refusals: %v", watched, afterErr)
			}
			if after.Mode() != before.Mode() || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
				t.Errorf("%s changed: mode %v -> %v, size %d -> %d", watched, before.Mode(), after.Mode(), before.Size(), after.Size())
			}
			if before.Mode()&os.ModeSymlink != 0 {
				if got, _ := os.Readlink(watched); got != linkTarget {
					t.Errorf("symlink now points at %q, was %q", got, linkTarget)
				}
			}
		})
	}
}

// --- Unbind ----------------------------------------------------------------

func TestUnbindAbsentIsANoOpAndWritesNothing(t *testing.T) {
	s, root := isolatedStore(t)
	removed, err := s.Unbind(hex64("a"))
	if removed || err != nil {
		t.Fatalf("Unbind() = %v, %v, want false, nil", removed, err)
	}
	if got := names(t, root); len(got) != 0 {
		t.Fatalf("Unbind on an absent binding created %v, want nothing", got)
	}
}

func TestUnbindRemovesAnOwnedBindingAndIsIdempotent(t *testing.T) {
	s, root := isolatedStore(t)
	mustBind(t, s, hex64("a"), "proj-1", "wf-1", t0)

	removed, err := s.Unbind(hex64("a"))
	if !removed || err != nil {
		t.Fatalf("Unbind() = %v, %v, want true, nil", removed, err)
	}
	if _, statErr := os.Lstat(bindingPath(root, hex64("a"))); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("the binding file is still there: %v", statErr)
	}
	if loaded := mustLoad(t, s, hex64("a")); loaded.Classification != projection.ClassificationAbsent {
		t.Fatalf("Load() after Unbind = %+v, want absent", loaded)
	}

	removed, err = s.Unbind(hex64("a"))
	if removed || err != nil {
		t.Fatalf("second Unbind() = %v, %v, want false, nil", removed, err)
	}

	// Unbinding frees the repository to bind a different workflow.
	mustBind(t, s, hex64("a"), "proj-2", "wf-2", t0)
}

func TestUnbindOnlyRemovesTheNamedRepository(t *testing.T) {
	s, _ := isolatedStore(t)
	mustBind(t, s, hex64("a"), "proj-1", "wf-1", t0)
	mustBind(t, s, hex64("b"), "proj-1", "wf-2", t0)

	if removed, err := s.Unbind(hex64("a")); !removed || err != nil {
		t.Fatalf("Unbind(a) = %v, %v, want true, nil", removed, err)
	}
	if loaded := mustLoad(t, s, hex64("b")); loaded.Classification != projection.ClassificationOwned || loaded.Binding.WorkflowID != "wf-2" {
		t.Fatalf("Load(b) = %+v, want b still bound to wf-2", loaded)
	}
}

func TestUnbindRefusesAMalformedRepoKey(t *testing.T) {
	s, _ := isolatedStore(t)
	if removed, err := s.Unbind("../" + strings.Repeat("a", 61)); removed || err == nil {
		t.Fatalf("Unbind() = %v, %v, want false and an error", removed, err)
	}
}

// --- restart and concurrency -----------------------------------------------

// TestAFreshStoreReadsWhatAnotherWrote is the restart guarantee: nothing is
// held in memory, so a new process (here, a new Store value over the same
// environment) sees exactly what an earlier one bound, and can change it.
func TestAFreshStoreReadsWhatAnotherWrote(t *testing.T) {
	first, root := isolatedStore(t)
	mustBind(t, first, hex64("a"), "proj-1", "wf-1", t0)

	second, err := projection.NewStore()
	if err != nil {
		t.Fatalf("NewStore() = %v, want nil", err)
	}
	loaded := mustLoad(t, second, hex64("a"))
	want := projection.Binding{Version: 1, RepoKey: hex64("a"), ProjectID: "proj-1", WorkflowID: "wf-1", BoundAt: "2026-09-29T10:00:00Z"}
	if loaded.Classification != projection.ClassificationOwned || loaded.Binding != want {
		t.Fatalf("a fresh store loaded %+v, want owned with %+v", loaded, want)
	}

	if removed, err := second.Unbind(hex64("a")); !removed || err != nil {
		t.Fatalf("Unbind() from the fresh store = %v, %v, want true, nil", removed, err)
	}
	if loaded := mustLoad(t, first, hex64("a")); loaded.Classification != projection.ClassificationAbsent {
		t.Fatalf("the first store still sees %+v after the second unbound, want absent (root %s)", loaded, root)
	}
}

// TestConcurrentBindsAreLastWriterWins documents the store's concurrency
// contract: writers take turns on the lock, every write is atomic, and a binding
// is only a pointer, so whichever writer renames last wins and either outcome
// is a valid binding.
func TestConcurrentBindsAreLastWriterWins(t *testing.T) {
	s, root := isolatedStore(t)
	const writers = 8

	var wg sync.WaitGroup
	errs := make([]error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = s.Bind(hex64("a"), "proj-1", fmt.Sprintf("wf-%d", i), t0, true)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("writer %d: Bind(replace) = %v, want nil", i, err)
		}
	}
	loaded := mustLoad(t, s, hex64("a"))
	if loaded.Classification != projection.ClassificationOwned || !strings.HasPrefix(loaded.Binding.WorkflowID, "wf-") {
		t.Fatalf("after %d concurrent writers Load() = %+v, want an owned binding to one of them", writers, loaded)
	}
	if got := names(t, bindingsDir(root)); fmt.Sprint(got) != fmt.Sprint([]string{hex64("a") + ".json"}) {
		t.Fatalf("bindings directory holds %v, want only the binding (no temporary files)", got)
	}
}

func TestConcurrentBindsWithoutReplaceEndInAValidBinding(t *testing.T) {
	s, _ := isolatedStore(t)
	const writers = 8

	var wg sync.WaitGroup
	errs := make([]error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = s.Bind(hex64("a"), "proj-1", fmt.Sprintf("wf-%d", i), t0, false)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil && !errors.Is(err, projection.ErrAlreadyBound) {
			t.Errorf("writer %d: Bind() = %v, want nil or ErrAlreadyBound", i, err)
		}
	}
	loaded := mustLoad(t, s, hex64("a"))
	if loaded.Classification != projection.ClassificationOwned || !strings.HasPrefix(loaded.Binding.WorkflowID, "wf-") {
		t.Fatalf("Load() = %+v, want an owned binding to one of the writers", loaded)
	}
}

// TestReadersNeverSeeAPartialBindingWhileItIsReplaced pins the atomicity of a
// write: a reader running while the binding is replaced over and over sees
// the old binding or the new one, never an empty or half-written file.
func TestReadersNeverSeeAPartialBindingWhileItIsReplaced(t *testing.T) {
	s, _ := isolatedStore(t)
	mustBind(t, s, hex64("a"), "proj-1", "wf-0", t0)

	stop := make(chan struct{})
	var mu sync.Mutex
	var problems []string
	var wg sync.WaitGroup
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				loaded, err := s.Load(hex64("a"))
				if err != nil || loaded.Classification != projection.ClassificationOwned {
					mu.Lock()
					problems = append(problems, fmt.Sprintf("Load() = %+v, %v", loaded, err))
					mu.Unlock()
					return
				}
			}
		}()
	}
	for i := 0; i < 300; i++ {
		if err := s.Bind(hex64("a"), "proj-1", fmt.Sprintf("wf-%d", i%2), t0, true); err != nil {
			t.Errorf("Bind(replace) #%d = %v, want nil", i, err)
			break
		}
	}
	close(stop)
	wg.Wait()

	if len(problems) > 0 {
		t.Fatalf("a reader saw a state other than an owned binding during replacement: %s", problems[0])
	}
}
