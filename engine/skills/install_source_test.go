package skills

// The source tree of a skill as install reads it, and the one rule that decides what
// a whole-directory copy leaves out. install and the Pi package builder (pipkg) both
// copy skill directories; they share SkipWhenCopying so that neither can start
// shipping the approval record, which is repository governance state, or half of a
// write another verb is doing.

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestReadSkillSource_ReturnsTheRegularFilesSortedWithTheirModes(t *testing.T) {
	src := t.TempDir()
	writeTestFile(t, filepath.Join(src, "SKILL.md"), "the skill")
	writeTestFile(t, filepath.Join(src, "references", "guide.md"), "a reference")
	writeTestFile(t, filepath.Join(src, "assets", "deep", "x.txt"), "deep")
	script := filepath.Join(src, "scripts", "run.sh")
	writeTestFile(t, script, "#!/bin/sh\n")
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}

	files, err := readSkillSource(src)
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

// The approval record and a writer's temporary file are not skill content. This is
// the test that used to watch copyTree; install reads the tree itself now, through
// the same rule.
func TestReadSkillSource_SkipsTheApprovalRecordAndAWritersTemporaryFile(t *testing.T) {
	src := t.TempDir()
	for path, content := range map[string]string{
		"SKILL.md":            "the skill",
		"references/guide.md": "a reference",
		".gitkeep":            "",
		ApprovalRecordName:    `{"version":1}`,
		// A nested file with the record's name is content: only the record at the
		// root of the skill is governance state.
		"references/" + ApprovalRecordName:      "content that happens to share the name",
		atomicTempPrefix + "123456789":          "half a record",
		"references/" + atomicTempPrefix:        "not a writer's file: a longer name is needed",
		"references/" + atomicTempPrefix + "42": "half of another write",
	} {
		writeTestFile(t, filepath.Join(src, filepath.FromSlash(path)), content)
	}

	files, err := readSkillSource(src)
	if err != nil {
		t.Fatal(err)
	}

	var got []string
	for _, f := range files {
		got = append(got, f.Rel)
	}
	// A name that is exactly the prefix has no unique suffix and is not something
	// writeFileAtomic makes; it is content.
	want := []string{".gitkeep", "SKILL.md", "references/.approval.json", "references/.tmp-skills-", "references/guide.md"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("read %v, want %v", got, want)
	}
}

func TestReadSkillSource_LeavesOutSymlinksAndEmptyDirectories(t *testing.T) {
	src := t.TempDir()
	writeTestFile(t, filepath.Join(src, "SKILL.md"), "the skill")
	outside := filepath.Join(t.TempDir(), "secret.txt")
	writeTestFile(t, outside, "not part of the skill")
	if err := os.Symlink(outside, filepath.Join(src, "link.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(outside), filepath.Join(src, "linkdir")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "empty", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}

	files, err := readSkillSource(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Rel != "SKILL.md" {
		t.Errorf("files = %+v, want only SKILL.md", files)
	}
}

func TestReadSkillSource_FailsOnAMissingDirectory(t *testing.T) {
	if _, err := readSkillSource(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("a missing source directory was read without an error")
	}
}

func TestSkipWhenCopying(t *testing.T) {
	src := t.TempDir()
	writeTestFile(t, filepath.Join(src, "SKILL.md"), "x")
	writeTestFile(t, filepath.Join(src, ApprovalRecordName), "x")
	writeTestFile(t, filepath.Join(src, atomicTempPrefix+"7"), "x")
	writeTestFile(t, filepath.Join(src, "sub", ApprovalRecordName), "x")
	if err := os.MkdirAll(filepath.Join(src, atomicTempPrefix+"dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	entries := map[string]fs.DirEntry{}
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err == nil && p != src {
			rel, _ := filepath.Rel(src, p)
			entries[filepath.ToSlash(rel)] = d
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for rel, want := range map[string]bool{
		"SKILL.md":                  false,
		ApprovalRecordName:          true,
		atomicTempPrefix + "7":      true,
		"sub/" + ApprovalRecordName: false,
		"sub":                       false,
		// A directory that merely starts with the temporary prefix is walked.
		atomicTempPrefix + "dir": false,
	} {
		if got := SkipWhenCopying(filepath.FromSlash(rel), entries[rel]); got != want {
			t.Errorf("SkipWhenCopying(%q) = %v, want %v", rel, got, want)
		}
	}
}
