package skillsfs_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/skillsfs"
)

func names(entries []fs.DirEntry) []string {
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func TestProjectStatFollowsLinksAndSaysWhyItCannot(t *testing.T) {
	dir := t.TempDir()
	put(t, filepath.Join(dir, "real", "f.md"), "x")
	if err := os.Symlink("real", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	info, err := skillsfs.Project{}.Stat(filepath.Join(dir, "link"))
	if err != nil || !info.IsDir() {
		t.Errorf("Stat through a link = %v, %v, want the directory it points to", info, err)
	}
	if _, err := (skillsfs.Project{}).Stat(filepath.Join(dir, "absent")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat of a path that is not there = %v, want an error that is fs.ErrNotExist", err)
	}
	if _, err := (skillsfs.Project{}).Stat(filepath.Join(dir, "real", "f.md", "below")); err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat of a path below a file = %v, want an error that is not 'not there'", err)
	}
}

func TestProjectReadDirIsSortedByName(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"b", "a.c", "a", "C"} {
		put(t, filepath.Join(dir, n), "x")
	}
	entries, err := skillsfs.Project{}.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := names(entries), []string{"C", "a", "a.c", "b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ReadDir = %v, want %v", got, want)
	}
	if _, err := (skillsfs.Project{}).ReadDir(filepath.Join(dir, "b")); err == nil {
		t.Error("ReadDir of a file succeeded")
	}
}

func TestProjectMkdirAllMakesEveryMissingDirectory(t *testing.T) {
	dir := t.TempDir()
	deep := filepath.Join(dir, "a", "b", "c")
	if err := (skillsfs.Project{}).MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(deep); err != nil || !info.IsDir() {
		t.Errorf("%s was not made: %v", deep, err)
	}
	put(t, filepath.Join(dir, "file"), "x")
	if err := (skillsfs.Project{}).MkdirAll(filepath.Join(dir, "file", "sub"), 0o755); err == nil {
		t.Error("MkdirAll below a file succeeded")
	}
}

func TestProjectWriteTempWritesNextToTheDestinationAtTheModeAsked(t *testing.T) {
	dir := t.TempDir()
	for _, perm := range []fs.FileMode{0o600, 0o644, 0o755} {
		name, err := skillsfs.Project{}.WriteTemp(dir, []byte("the bytes"), perm)
		if err != nil {
			t.Fatalf("WriteTemp(%v): %v", perm, err)
		}
		if filepath.Dir(name) != dir || !strings.HasPrefix(filepath.Base(name), ".tmp-skills-") || len(filepath.Base(name)) == len(".tmp-skills-") {
			t.Errorf("WriteTemp named %q, want a file of the directory whose name begins '.tmp-skills-' and goes on", name)
		}
		got, err := os.ReadFile(name)
		if err != nil || string(got) != "the bytes" {
			t.Errorf("the temporary file holds %q, %v, want the bytes", got, err)
		}
		if info, _ := os.Stat(name); info.Mode().Perm() != perm {
			t.Errorf("the temporary file has the mode %v, want %v whatever the mask of the process", info.Mode().Perm(), perm)
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 3 {
		t.Errorf("%d files in the directory, want the three temporary files and nothing else: %v", len(entries), names(entries))
	}
}

func TestProjectWriteTempSaysTheStepItFailedAt(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "no", "such", "dir")
	name, err := skillsfs.Project{}.WriteTemp(missing, []byte("x"), 0o644)
	if err == nil || !strings.HasPrefix(err.Error(), "create temp: ") {
		t.Errorf("WriteTemp in a directory that is not there = %q, %v, want an error that begins 'create temp: '", name, err)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("WriteTemp error = %v, want it to carry the failure of the system", err)
	}
	if name != "" {
		t.Errorf("WriteTemp returned the name %q with an error", name)
	}
}

func TestProjectWriteTempLeavesNothingWhenTheDirectoryCannotBeWrittenTo(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes everywhere")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if _, err := (skillsfs.Project{}).WriteTemp(dir, []byte("x"), 0o644); err == nil || !strings.HasPrefix(err.Error(), "create temp: ") || !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("WriteTemp in a directory that cannot be written to = %v, want 'create temp: ... permission denied'", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("%v left behind", names(entries))
	}
}

func TestProjectRenameReplacesAndRemoveDeletes(t *testing.T) {
	dir := t.TempDir()
	put(t, filepath.Join(dir, "old"), "old")
	put(t, filepath.Join(dir, "new"), "new")
	if err := (skillsfs.Project{}).Rename(filepath.Join(dir, "new"), filepath.Join(dir, "old")); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "old")); string(got) != "new" {
		t.Errorf("old holds %q after the rename, want 'new'", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "new")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the renamed file is still there: %v", err)
	}
	if err := (skillsfs.Project{}).Remove(filepath.Join(dir, "old")); err != nil {
		t.Fatal(err)
	}
	if err := (skillsfs.Project{}).Remove(filepath.Join(dir, "old")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Remove of a file that is gone = %v, want fs.ErrNotExist", err)
	}
	put(t, filepath.Join(dir, "full", "f"), "x")
	if err := (skillsfs.Project{}).Remove(filepath.Join(dir, "full")); err == nil {
		t.Error("Remove deleted a directory that is not empty")
	}
	if err := os.Mkdir(filepath.Join(dir, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := (skillsfs.Project{}).Remove(filepath.Join(dir, "empty")); err != nil {
		t.Errorf("Remove of an empty directory = %v", err)
	}
}

func TestProjectResolvePathResolvesLinksAndKeepsWhatIsNotThere(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(dir, "real", "f.md"), "x")
	if err := os.Symlink("real", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	got, err := skillsfs.Project{}.ResolvePath(filepath.Join(dir, "link", "not", "there.md"))
	if want := filepath.Join(dir, "real", "not", "there.md"); err != nil || got != want {
		t.Errorf("ResolvePath through a link to a path that is not there = %q, %v, want %q", got, err, want)
	}
	loop := filepath.Join(dir, "loop")
	if err := os.Symlink("loop", loop); err != nil {
		t.Fatal(err)
	}
	if got, err := (skillsfs.Project{}).ResolvePath(filepath.Join(loop, "x")); err == nil {
		t.Errorf("ResolvePath through a link to itself = %q, nil, want an error", got)
	}
}
