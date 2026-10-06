package skills

// Tests of how the verbs that write an overlay use the StagedWrites port they are given: which
// files they stage, at which mode, in which order they commit them, what they put back when a
// step fails, and the words of each failure. The port answers from the real file system over a
// temporary directory, with a failure injected at one call; what the real adapter does with a
// real disk is tested in engine/skills/skillsfs, and what the program leaves on disk is pinned
// by the golden files of engine/cmd.

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stagedSpy records every call to the writes it wraps and can turn one into an error.
type stagedSpy struct {
	real  StagedWrites
	fail  func(op, path string, n int) error // n counts the calls of this op so far, from 1
	ops   []string
	perms []fs.FileMode
	n     map[string]int
}

func newStagedSpy(fail func(op, path string, n int) error) *stagedSpy {
	return &stagedSpy{real: testProjectFS(), fail: fail, n: map[string]int{}}
}

func (s *stagedSpy) check(op, path string) error {
	s.n[op]++
	s.ops = append(s.ops, op+" "+path)
	if s.fail != nil {
		return s.fail(op, path, s.n[op])
	}
	return nil
}

func (s *stagedSpy) WriteTemp(dir string, data []byte, perm fs.FileMode) (string, error) {
	s.perms = append(s.perms, perm)
	if err := s.check("writetemp", dir); err != nil {
		return "", err
	}
	return s.real.WriteTemp(dir, data, perm)
}

func (s *stagedSpy) Rename(oldPath, newPath string) error {
	if err := s.check("rename", newPath); err != nil {
		return err
	}
	return s.real.Rename(oldPath, newPath)
}

func (s *stagedSpy) Remove(name string) error {
	if err := s.check("remove", name); err != nil {
		return err
	}
	return s.real.Remove(name)
}

var errInjectedWrite = errors.New("injected failure")

// writeWorld is an overlay with a registry and a manifest that agree on one skill, a second skill
// ready to be added, and the paths of the files.
type writeWorld struct {
	dir, reg, man, skills string
}

func newWriteWorld(t *testing.T) writeWorld {
	t.Helper()
	dir := t.TempDir()
	reg, man, skillsRoot := setupFixture(t, dir, minimalRegistry("existing"), minimalManifest("existing"), []string{"existing", "foo"})
	return writeWorld{dir: dir, reg: reg, man: man, skills: skillsRoot}
}

func (w writeWorld) add(files StagedWrites) (code int, stderr string) {
	var out, errBuf bytes.Buffer
	code = -1
	AddCore([]string{"--registry", w.reg, "--manifest", w.man, "--source-root", w.skills, "foo"},
		os.ReadFile, testRegistries(os.ReadFile), os.Stat, files, &out, &errBuf, func(c int) { code = c })
	return code, errBuf.String()
}

func (w writeWorld) remove(files StagedWrites) (code int, stderr string) {
	var out, errBuf bytes.Buffer
	code = -1
	RemoveCore([]string{"--registry", w.reg, "--manifest", w.man, "existing"},
		os.ReadFile, testRegistries(os.ReadFile), files, &out, &errBuf, func(c int) { code = c })
	return code, errBuf.String()
}

// unchanged says the registry and the manifest are as the fixture made them, and that no
// temporary file is left in the directory.
func (w writeWorld) unchanged(t *testing.T, wantRegistry, wantManifest string) {
	t.Helper()
	if got, _ := os.ReadFile(w.reg); string(got) != wantRegistry {
		t.Errorf("the registry is %q, want %q", got, wantRegistry)
	}
	if got, _ := os.ReadFile(w.man); string(got) != wantManifest {
		t.Errorf("the manifest is %q, want %q", got, wantManifest)
	}
	w.noLitter(t)
}

func (w writeWorld) noLitter(t *testing.T) {
	t.Helper()
	entries, _ := os.ReadDir(w.dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), atomicTempPrefix) {
			t.Errorf("the temporary file %s was left behind", e.Name())
		}
	}
}

func TestAddStagesTheManifestThenTheRegistryAndCommitsThemInThatOrder(t *testing.T) {
	w := newWriteWorld(t)
	spy := newStagedSpy(nil)
	if code, stderr := w.add(spy); code != 0 {
		t.Fatalf("add = exit %d, stderr %q", code, stderr)
	}
	kinds := make([]string, len(spy.ops))
	for i, op := range spy.ops {
		kinds[i] = strings.Fields(op)[0]
	}
	if got, want := strings.Join(kinds, " "), "writetemp writetemp rename rename"; got != want {
		t.Errorf("the calls were %q, want %q: both files staged, then the manifest, then the registry", got, want)
	}
	if got, want := spy.ops[2], "rename "+w.man; got != want {
		t.Errorf("the first commit was %q, want %q", got, want)
	}
	if got, want := spy.ops[3], "rename "+w.reg; got != want {
		t.Errorf("the second commit was %q, want %q", got, want)
	}
	for _, p := range spy.perms {
		if p != 0o600 {
			t.Errorf("a file was staged at the mode %v, want 0600: the registry and the manifest are the owner's", p)
		}
	}
	for _, op := range spy.ops[:2] {
		if op != "writetemp "+w.dir+string(filepath.Separator) {
			t.Errorf("%q: a file is staged in the directory of its destination", op)
		}
	}
	w.noLitter(t)
}

func TestAddPutsBackWhatItStagedWhenAStepFails(t *testing.T) {
	for name, tc := range map[string]struct {
		fail       func(w writeWorld) func(op, path string, n int) error
		wantStderr func(w writeWorld) string
		// what the registry and the manifest are afterwards: "same" or "new"
		manifest, registry string
	}{
		"staging the manifest": {
			fail: func(w writeWorld) func(string, string, int) error {
				return func(op, _ string, n int) error {
					if op == "writetemp" && n == 1 {
						return errors.New("create temp: no room")
					}
					return nil
				}
			},
			wantStderr: func(writeWorld) string {
				return "error: writing manifest: writeFileAtomic: create temp: no room\n"
			},
			manifest: "same", registry: "same",
		},
		"staging the registry": {
			fail: func(w writeWorld) func(string, string, int) error {
				return func(op, _ string, n int) error {
					if op == "writetemp" && n == 2 {
						return errors.New("sync: disk gone")
					}
					return nil
				}
			},
			wantStderr: func(writeWorld) string {
				return "error: writing registry: writeFileAtomic: sync: disk gone\n"
			},
			manifest: "same", registry: "same",
		},
		"committing the manifest": {
			fail: func(w writeWorld) func(string, string, int) error {
				return func(op, path string, _ int) error {
					if op == "rename" && path == w.man {
						return errInjectedWrite
					}
					return nil
				}
			},
			wantStderr: func(writeWorld) string { return "error: finalizing manifest: injected failure\n" },
			manifest:   "same", registry: "same",
		},
		// The two files are renamed one after the other: a failure of the second leaves the
		// manifest as it was written and the registry as it was. The temporary file of the
		// registry is removed. This is how the verb has always behaved; the registry and the
		// manifest are not committed as one.
		"committing the registry": {
			fail: func(w writeWorld) func(string, string, int) error {
				return func(op, path string, _ int) error {
					if op == "rename" && path == w.reg {
						return errInjectedWrite
					}
					return nil
				}
			},
			wantStderr: func(writeWorld) string { return "error: finalizing registry: injected failure\n" },
			manifest:   "new", registry: "same",
		},
	} {
		t.Run(name, func(t *testing.T) {
			w := newWriteWorld(t)
			spy := newStagedSpy(tc.fail(w))
			code, stderr := w.add(spy)
			if code != 1 || stderr != tc.wantStderr(w) {
				t.Errorf("add = exit %d, stderr %q, want exit 1 and %q", code, stderr, tc.wantStderr(w))
			}
			wantManifest, wantRegistry := minimalManifest("existing"), minimalRegistry("existing")
			gotManifest, _ := os.ReadFile(w.man)
			gotRegistry, _ := os.ReadFile(w.reg)
			if (string(gotManifest) == wantManifest) != (tc.manifest == "same") {
				t.Errorf("the manifest is %q, want it %s", gotManifest, tc.manifest)
			}
			if (string(gotRegistry) == wantRegistry) != (tc.registry == "same") {
				t.Errorf("the registry is %q, want it %s", gotRegistry, tc.registry)
			}
			w.noLitter(t)
		})
	}
}

func TestRemoveStagesAndCommitsThroughTheWritesAndPutsBackWhenAStepFails(t *testing.T) {
	w := newWriteWorld(t)
	spy := newStagedSpy(nil)
	if code, stderr := w.remove(spy); code != 0 {
		t.Fatalf("remove = exit %d, stderr %q", code, stderr)
	}
	if len(spy.ops) != 4 || spy.ops[2] != "rename "+w.man || spy.ops[3] != "rename "+w.reg {
		t.Errorf("the calls were %q, want both files staged and then the manifest and the registry committed", spy.ops)
	}

	for name, fail := range map[string]func(w writeWorld) func(op, path string, n int) error{
		"staging the manifest": func(writeWorld) func(string, string, int) error {
			return func(op, _ string, n int) error {
				if op == "writetemp" && n == 1 {
					return errors.New("create temp: no room")
				}
				return nil
			}
		},
		"committing the manifest": func(w writeWorld) func(string, string, int) error {
			return func(op, path string, _ int) error {
				if op == "rename" && path == w.man {
					return errInjectedWrite
				}
				return nil
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			w := newWriteWorld(t)
			code, stderr := w.remove(newStagedSpy(fail(w)))
			if code != 1 || !strings.HasPrefix(stderr, "error: ") {
				t.Errorf("remove = exit %d, stderr %q, want a refusal", code, stderr)
			}
			w.unchanged(t, minimalRegistry("existing"), minimalManifest("existing"))
		})
	}
}

func TestSyncManifestStagesTheManifestAtTheOwnerOnlyModeAndCommitsIt(t *testing.T) {
	dir := t.TempDir()
	reg, man, _ := setupFixture(t, dir, minimalRegistry("existing", "other"), minimalManifest("existing"), []string{"existing", "other"})
	run := func(files StagedWrites) (int, string) {
		var out, errBuf bytes.Buffer
		code := -1
		SyncCore([]string{"--registry", reg, "--manifest", man}, os.ReadFile, testRegistries(os.ReadFile), files, &out, &errBuf, func(c int) { code = c })
		return code, errBuf.String()
	}

	failing := newStagedSpy(func(op, _ string, _ int) error {
		if op == "writetemp" {
			return errors.New("create temp: no room")
		}
		return nil
	})
	if code, stderr := run(failing); code != 1 || stderr != "error: writing manifest: writeFileAtomic: create temp: no room\n" {
		t.Errorf("sync-manifest with a file system that cannot stage = exit %d, stderr %q", code, stderr)
	}

	failing = newStagedSpy(func(op, path string, _ int) error {
		if op == "rename" {
			return errInjectedWrite
		}
		return nil
	})
	if code, stderr := run(failing); code != 1 || stderr != "error: finalizing manifest: injected failure\n" {
		t.Errorf("sync-manifest with a file system that cannot commit = exit %d, stderr %q", code, stderr)
	}
	if got, _ := os.ReadFile(man); string(got) != minimalManifest("existing") {
		t.Errorf("the manifest is %q after a failed commit, want it as it was", got)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), atomicTempPrefix) {
			t.Errorf("the temporary file %s was left behind", e.Name())
		}
	}

	working := newStagedSpy(nil)
	if code, stderr := run(working); code != 0 {
		t.Fatalf("sync-manifest = exit %d, stderr %q", code, stderr)
	}
	if len(working.ops) != 2 || working.perms[0] != 0o600 {
		t.Errorf("the calls were %q at the modes %v, want one staging at 0600 and one commit", working.ops, working.perms)
	}
}

func TestApproveStagesTheRecordWorldReadableAndPutsBackWhenItCannotCommit(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "foo", "SKILL.md"), lintCleanSkillMD("foo"))
	run := func(files StagedWrites) (int, string, string) {
		var out, errBuf bytes.Buffer
		code := -1
		RenderApproveCore([]string{"--id", "foo", "--approver", "reviewer", "--source-root", root},
			os.ReadFile, fixedClock(approveFixedNow), files, &out, &errBuf, func(c int) { code = c })
		return code, out.String(), errBuf.String()
	}
	record := ApprovalRecordPath(root, "foo")

	failing := newStagedSpy(func(op, _ string, _ int) error {
		if op == "writetemp" {
			return errors.New("create temp: no room")
		}
		return nil
	})
	code, _, stderr := run(failing)
	want := fmt.Sprintf("error: skills approve: skill %q: writing approval record: writeFileAtomic: create temp: no room\n", "foo")
	if code != 1 || stderr != want {
		t.Errorf("approve with a file system that cannot stage = exit %d, stderr %q, want exit 1 and %q", code, stderr, want)
	}

	failing = newStagedSpy(func(op, _ string, _ int) error {
		if op == "rename" {
			return errInjectedWrite
		}
		return nil
	})
	code, _, stderr = run(failing)
	want = fmt.Sprintf("error: skills approve: skill %q: writing approval record: finalizing %q: injected failure\n", "foo", record)
	if code != 1 || stderr != want {
		t.Errorf("approve with a file system that cannot commit = exit %d, stderr %q, want exit 1 and %q", code, stderr, want)
	}
	if _, err := os.Stat(record); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a record is there after a failed commit: %v", err)
	}
	entries, _ := os.ReadDir(filepath.Join(root, "foo"))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), atomicTempPrefix) {
			t.Errorf("the temporary file %s was left behind", e.Name())
		}
	}

	working := newStagedSpy(nil)
	if code, _, stderr := run(working); code != 0 {
		t.Fatalf("approve = exit %d, stderr %q", code, stderr)
	}
	if len(working.perms) != 1 || working.perms[0] != 0o644 {
		t.Errorf("the record was staged at %v, want 0644: it is committed with the skill", working.perms)
	}
	if info, err := os.Stat(record); err != nil || info.Mode().Perm() != 0o644 {
		t.Errorf("the record is %v, %v, want a file at 0644", info, err)
	}
}

// A file is staged in the directory of its destination, and a destination that names no
// directory is staged in the working directory ("."), never in the directory of temporary files.
func TestWriteFileAtomicStagesInTheDirectoryOfThePath(t *testing.T) {
	for path, want := range map[string]string{
		"/overlay/skills.registry.yaml": "/overlay/",
		"overlay/overlay.manifest":      "overlay/",
		"overlay.manifest":              ".",
		"./overlay.manifest":            "./",
	} {
		t.Run(path, func(t *testing.T) {
			var got string
			spy := newStagedSpy(func(op, dir string, _ int) error {
				got = dir
				return errors.New("stop here")
			})
			if _, err := writeFileAtomic(spy, filepath.FromSlash(path), []byte("x"), 0o600); err == nil {
				t.Fatal("writeFileAtomic = nil, want the failure the port was told to return")
			}
			if got != filepath.FromSlash(want) {
				t.Errorf("staged in %q, want %q", got, filepath.FromSlash(want))
			}
		})
	}
}
