package skills_test

// The in-package tests of the skills package cannot import the adapters (they import it), so they
// use a double of the tree and of the project file system, made of os calls. These tests hold each
// double to the adapter it imitates, on the same files: when the rules of an adapter change, the
// double is told here, not by a domain test that quietly tests a stale copy.

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/skillsfs"
)

func putFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestTheTreeDoubleScansAsTheTreeAdapterDoes(t *testing.T) {
	root := t.TempDir()
	putFile(t, filepath.Join(root, "a", "SKILL.md"), "a")
	putFile(t, filepath.Join(root, "a", "ref", "x.md"), "x")
	putFile(t, filepath.Join(root, "a", ".swp"), "scratch")
	putFile(t, filepath.Join(root, ".git", "HEAD"), "ref")
	putFile(t, filepath.Join(root, ".skills.registry.yaml.lock"), "")
	putFile(t, filepath.Join(root, "b.txt"), "b")
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("a", filepath.Join(root, "link")); err != nil {
		t.Logf("no symbolic link here (%v): the rule for links is not held to the adapter", err)
	}

	fromAdapter, err := skillsfs.Tree{}.ScanSkillFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	fromDouble, err := skills.ScanSkillFilesOfTheDouble(root)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"a/SKILL.md", "a/ref/x.md", "b.txt"}
	if !reflect.DeepEqual(fromAdapter, want) {
		t.Errorf("the adapter lists %v, want %v", fromAdapter, want)
	}
	if !reflect.DeepEqual(fromDouble, fromAdapter) {
		t.Errorf("the double lists %v, the adapter %v", fromDouble, fromAdapter)
	}
}

func TestTheProjectFSDoubleWritesATempFileAsTheProjectAdapterDoes(t *testing.T) {
	for name, fsys := range map[string]skills.ProjectFS{"the adapter": skillsfs.Project{}, "the double": skills.ProjectFSOfTheDouble()} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()

			tmp, err := fsys.WriteTemp(dir, []byte("data"), 0o640)
			if err != nil {
				t.Fatal(err)
			}

			if filepath.Dir(tmp) != dir {
				t.Errorf("the temp file is %s, want one in %s", tmp, dir)
			}
			if got, err := os.ReadFile(tmp); err != nil || string(got) != "data" {
				t.Errorf("the temp file holds %q, %v", got, err)
			}
			if info, err := os.Stat(tmp); err != nil || info.Mode().Perm() != 0o640 {
				t.Errorf("the temp file has mode %v, %v, want 0640 whatever the mask", info.Mode().Perm(), err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 {
				t.Fatalf("the directory holds %v, %v", entries, err)
			}
			if !skills.SkipWhenCopying(entries[0].Name(), entries[0]) {
				t.Errorf("a copier of a tree would take %s for content: its name is not the one of half a write", entries[0].Name())
			}
			if _, err := fsys.WriteTemp(filepath.Join(dir, "absent"), []byte("x"), 0o600); err == nil {
				t.Error("a temp file was written in a directory that is not there")
			}
		})
	}
}

func TestTheProjectFSDoubleResolvesAPathAsTheProjectAdapterDoes(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real", filepath.Join(base, "link")); err != nil {
		t.Skipf("no symbolic link here: %v", err)
	}
	missing := filepath.Join(base, "link", "not", "there")

	fromAdapter, err := skillsfs.Project{}.ResolvePath(missing)
	if err != nil {
		t.Fatal(err)
	}
	fromDouble, err := skills.ProjectFSOfTheDouble().ResolvePath(missing)
	if err != nil {
		t.Fatal(err)
	}
	resolvedBase, _ := filepath.EvalSymlinks(base)
	if want := filepath.Join(resolvedBase, "real", "not", "there"); fromAdapter != want || fromDouble != want {
		t.Errorf("the adapter resolves to %q, the double to %q, want %q", fromAdapter, fromDouble, want)
	}
}
