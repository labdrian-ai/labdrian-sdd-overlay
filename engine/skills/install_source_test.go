package skills

// The one rule that decides what a whole-directory copy of a skill's source leaves out.
// install and the Pi package builder (pipkg) both copy skill directories; they share
// SkipWhenCopying so that neither can start shipping the approval record, which is
// repository governance state, or half of a write another verb is doing. How install
// walks the tree is the file system adapter's, and its tests are in engine/skills/skillsfs.

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

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
