package fsscan

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skillregistry"
)

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestIsProjectIsADirectoryThatHoldsGitOrAtl(t *testing.T) {
	root := t.TempDir()
	for name, make := range map[string]func(dir string){
		"a .git directory":  func(dir string) { os.MkdirAll(filepath.Join(dir, ".git"), 0o755) },
		"a .git file":       func(dir string) { write(t, filepath.Join(dir, ".git"), "gitdir: /elsewhere\n") },
		"an .atl directory": func(dir string) { os.MkdirAll(filepath.Join(dir, ".atl"), 0o755) },
	} {
		dir := filepath.Join(root, name)
		os.MkdirAll(dir, 0o755)
		make(dir)
		if !(Scanner{}).IsProject(dir) {
			t.Errorf("%s: IsProject = false, want true", name)
		}
	}
	plain := filepath.Join(root, "plain")
	os.MkdirAll(filepath.Join(plain, "skills"), 0o755)
	if (Scanner{}).IsProject(plain) {
		t.Error("a directory with only a skills directory is not a project")
	}
	if (Scanner{}).IsProject(filepath.Join(root, "missing")) {
		t.Error("a directory that is not there is not a project")
	}
}

func TestScanReturnsTheSkillsOfASourceInTheOrderOfTheirDirectories(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "skills")
	write(t, filepath.Join(dir, "b-dir", "SKILL.md"), "b")
	write(t, filepath.Join(dir, "a-dir", "SKILL.md"), "a")
	write(t, filepath.Join(dir, "Z-dir", "SKILL.md"), "z")
	write(t, filepath.Join(dir, ".hidden", "SKILL.md"), "hidden")

	got, exists := (Scanner{}).Scan(skillregistry.Source{Scope: skillregistry.ScopeUser, Label: "x", Dir: dir})

	if !exists {
		t.Fatal("exists = false for a directory")
	}
	want := []skillregistry.Candidate{
		{Scope: skillregistry.ScopeUser, DirName: ".hidden", Path: filepath.Join(dir, ".hidden", "SKILL.md"), Text: "hidden"},
		{Scope: skillregistry.ScopeUser, DirName: "Z-dir", Path: filepath.Join(dir, "Z-dir", "SKILL.md"), Text: "z"},
		{Scope: skillregistry.ScopeUser, DirName: "a-dir", Path: filepath.Join(dir, "a-dir", "SKILL.md"), Text: "a"},
		{Scope: skillregistry.ScopeUser, DirName: "b-dir", Path: filepath.Join(dir, "b-dir", "SKILL.md"), Text: "b"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Scan = %+v\nwant %+v", got, want)
	}
}

func TestScanSkipsWhatHoldsNoSkillFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "skills")
	write(t, filepath.Join(dir, "good", "SKILL.md"), "good")
	write(t, filepath.Join(dir, "afile.md"), "not a directory")
	write(t, filepath.Join(dir, "lower", "skill.md"), "wrong case")
	write(t, filepath.Join(dir, "nested", "inner", "SKILL.md"), "one level only")
	os.MkdirAll(filepath.Join(dir, "emptydir"), 0o755)
	os.MkdirAll(filepath.Join(dir, "asdir", "SKILL.md"), 0o755)

	got, exists := (Scanner{}).Scan(skillregistry.Source{Scope: skillregistry.ScopeProject, Dir: dir})

	if !exists || len(got) != 1 || got[0].DirName != "good" {
		t.Fatalf("Scan = %+v, %v, want only the good skill", got, exists)
	}
}

func TestScanFollowsALinkToADirectory(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "target", "linked", "SKILL.md"), "via link")
	dir := filepath.Join(root, "skills")
	os.MkdirAll(dir, 0o755)
	if err := os.Symlink(filepath.Join(root, "target", "linked"), filepath.Join(dir, "linked")); err != nil {
		t.Skip("no symlinks here:", err)
	}

	got, _ := (Scanner{}).Scan(skillregistry.Source{Scope: skillregistry.ScopeProject, Dir: dir})

	if len(got) != 1 || got[0].Text != "via link" || got[0].Path != filepath.Join(dir, "linked", "SKILL.md") {
		t.Fatalf("Scan = %+v, want the skill behind the link, listed under the link's own path", got)
	}
}

func TestScanSaysASourceThatIsNotADirectoryDoesNotExist(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "afile"), "x")
	for name, dir := range map[string]string{"missing": filepath.Join(root, "missing"), "a file": filepath.Join(root, "afile")} {
		if got, exists := (Scanner{}).Scan(skillregistry.Source{Dir: dir}); exists || len(got) != 0 {
			t.Errorf("%s: Scan = %+v, %v, want none", name, got, exists)
		}
	}
}
