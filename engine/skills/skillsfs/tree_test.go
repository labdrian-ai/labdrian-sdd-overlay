package skillsfs_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/skillsfs"
)

// tempPrefix begins the name of a temporary file the writers of an overlay make
// (skills.SkipWhenCopying names what a copy leaves out).
const tempPrefix = ".tmp-skills-"

// put writes content to path, making the directories on the way.
func put(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// --- ScanSkillFiles ---------------------------------------------------------------------------

func TestScanSkillFilesListsTheRegularFilesSorted(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{"alpha/SKILL.md", "alpha/references/one.md", "_shared/contract.md", "beta/SKILL.md"} {
		put(t, filepath.Join(root, filepath.FromSlash(f)), "x")
	}
	got, err := skillsfs.Tree{}.ScanSkillFiles(root)
	if err != nil {
		t.Fatalf("ScanSkillFiles: %v", err)
	}
	want := []string{"_shared/contract.md", "alpha/SKILL.md", "alpha/references/one.md", "beta/SKILL.md"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ScanSkillFiles = %v, want %v: sorted, slash-separated, relative, no directories", got, want)
	}
}

// The order is the order of the paths, not the order the walk meets them in: a directory
// 'a' is walked before a file 'a.c', and 'a.c' sorts before 'a/b'.
func TestScanSkillFilesSortsByPathNotByWalkOrder(t *testing.T) {
	root := t.TempDir()
	put(t, filepath.Join(root, "a", "b.md"), "x")
	put(t, filepath.Join(root, "a.c"), "x")
	got, err := skillsfs.Tree{}.ScanSkillFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a.c", "a/b.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ScanSkillFiles = %v, want %v", got, want)
	}
}

func TestScanSkillFilesLeavesOutDotNamesLinksAndPipes(t *testing.T) {
	root := t.TempDir()
	put(t, filepath.Join(root, "alpha", "SKILL.md"), "x")
	put(t, filepath.Join(root, "alpha", ".scratch"), "an editor's file")
	put(t, filepath.Join(root, ".git", "config"), "[core]")
	put(t, filepath.Join(root, "alpha", ".hidden", "inner.md"), "x")
	if err := os.Symlink("SKILL.md", filepath.Join(root, "alpha", "link.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../beta", filepath.Join(root, "alpha", "linkdir")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := skillsfs.Tree{}.ScanSkillFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"alpha/SKILL.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ScanSkillFiles = %v, want %v", got, want)
	}
}

func TestScanSkillFilesDoesNotWalkARootThatIsALink(t *testing.T) {
	real := t.TempDir()
	put(t, filepath.Join(real, "alpha", "SKILL.md"), "x")
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	got, err := skillsfs.Tree{}.ScanSkillFiles(link)
	if err != nil {
		t.Fatalf("ScanSkillFiles: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ScanSkillFiles through a link = %v, want nothing: a link is not walked", got)
	}
}

func TestScanSkillFilesRefusesWhatItCannotScan(t *testing.T) {
	dir := t.TempDir()
	put(t, filepath.Join(dir, "a-file"), "x")
	for name, tc := range map[string]struct{ root, want string }{
		"a root that is not there": {filepath.Join(dir, "absent"), "ondisk: stat skills dir " + filepath.Join(dir, "absent") + ": stat "},
		"a root that is a file":    {filepath.Join(dir, "a-file"), "ondisk: " + filepath.Join(dir, "a-file") + " is not a directory"},
		"a root below a file":      {filepath.Join(dir, "a-file", "x"), "ondisk: stat skills dir "},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := skillsfs.Tree{}.ScanSkillFiles(tc.root)
			if err == nil || !strings.HasPrefix(err.Error(), tc.want) {
				t.Errorf("ScanSkillFiles = %v, %v, want an error that begins %q", got, err, tc.want)
			}
			if got != nil {
				t.Errorf("ScanSkillFiles returned %v with an error, want nothing", got)
			}
		})
	}
}

func TestScanSkillFilesSaysWhichDirectoryItCouldNotRead(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads every directory")
	}
	root := t.TempDir()
	put(t, filepath.Join(root, "alpha", "SKILL.md"), "x")
	if err := os.Chmod(filepath.Join(root, "alpha"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(root, "alpha"), 0o755) })
	_, err := skillsfs.Tree{}.ScanSkillFiles(root)
	if err == nil || !strings.HasPrefix(err.Error(), "ondisk: walk "+root+": ") || !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("ScanSkillFiles = %v, want a walk error naming the root and the permission", err)
	}
}

// --- ReadSkillSource --------------------------------------------------------------------------

func TestReadSkillSourceReturnsTheRegularFilesSortedWithTheirModes(t *testing.T) {
	src := t.TempDir()
	put(t, filepath.Join(src, "SKILL.md"), "the skill")
	put(t, filepath.Join(src, "references", "guide.md"), "a reference")
	put(t, filepath.Join(src, "assets", "deep", "x.txt"), "deep")
	script := filepath.Join(src, "scripts", "run.sh")
	put(t, script, "#!/bin/sh\n")
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}

	files, err := skillsfs.Tree{}.ReadSkillSource(src)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	modes := map[string]fs.FileMode{}
	for _, f := range files {
		paths = append(paths, f.Rel)
		modes[f.Rel] = f.Mode
	}
	if want := []string{"SKILL.md", "assets/deep/x.txt", "references/guide.md", "scripts/run.sh"}; !reflect.DeepEqual(paths, want) {
		t.Errorf("files = %v, want %v", paths, want)
	}
	if modes["scripts/run.sh"] != 0o755 || modes["SKILL.md"] != 0o644 {
		t.Errorf("modes = %v, want the source permission bits", modes)
	}
	if string(files[0].Data) != "the skill" {
		t.Errorf("SKILL.md bytes = %q", files[0].Data)
	}
}

// The approval record and a writer's temporary file are not skill content.
func TestReadSkillSourceSkipsTheApprovalRecordAndAWritersTemporaryFile(t *testing.T) {
	src := t.TempDir()
	for path, content := range map[string]string{
		"SKILL.md":                "the skill",
		"references/guide.md":     "a reference",
		".gitkeep":                "",
		skills.ApprovalRecordName: `{"version":1}`,
		"references/" + skills.ApprovalRecordName: "content that happens to share the name",
		tempPrefix + "123456789":                  "half a record",
		"references/" + tempPrefix:                "not a writer's file: a longer name is needed",
		"references/" + tempPrefix + "42":         "half of another write",
	} {
		put(t, filepath.Join(src, filepath.FromSlash(path)), content)
	}
	files, err := skillsfs.Tree{}.ReadSkillSource(src)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range files {
		got = append(got, f.Rel)
	}
	want := []string{".gitkeep", "SKILL.md", "references/.approval.json", "references/.tmp-skills-", "references/guide.md"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("read %v, want %v", got, want)
	}
}

func TestReadSkillSourceLeavesOutLinksPipesAndEmptyDirectories(t *testing.T) {
	src := t.TempDir()
	put(t, filepath.Join(src, "SKILL.md"), "the skill")
	outside := filepath.Join(t.TempDir(), "secret.txt")
	put(t, outside, "not part of the skill")
	if err := os.Symlink(outside, filepath.Join(src, "link.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(outside), filepath.Join(src, "linkdir")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "empty", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	files, err := skillsfs.Tree{}.ReadSkillSource(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Rel != "SKILL.md" {
		t.Errorf("files = %+v, want only SKILL.md", files)
	}
}

func TestReadSkillSourceFailsOnAMissingDirectory(t *testing.T) {
	if _, err := (skillsfs.Tree{}).ReadSkillSource(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("a missing source directory was read without an error")
	}
}

func TestReadSkillSourceNamesTheFileItCouldNotRead(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads every file")
	}
	src := t.TempDir()
	put(t, filepath.Join(src, "SKILL.md"), "x")
	unreadable := filepath.Join(src, "references", "notes.md")
	put(t, unreadable, "x")
	if err := os.Chmod(unreadable, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o644) })
	files, err := skillsfs.Tree{}.ReadSkillSource(src)
	if err == nil || !strings.HasPrefix(err.Error(), "reading "+unreadable+": ") {
		t.Errorf("ReadSkillSource = %v, %v, want an error that names the file", files, err)
	}
}

// The order is the order of the paths, not the order the walk meets them in.
func TestReadSkillSourceSortsByPathNotByWalkOrder(t *testing.T) {
	src := t.TempDir()
	put(t, filepath.Join(src, "a", "b.md"), "x")
	put(t, filepath.Join(src, "a.c"), "x")
	files, err := skillsfs.Tree{}.ReadSkillSource(src)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range files {
		got = append(got, f.Rel)
	}
	if want := []string{"a.c", "a/b.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ReadSkillSource = %v, want %v", got, want)
	}
}
