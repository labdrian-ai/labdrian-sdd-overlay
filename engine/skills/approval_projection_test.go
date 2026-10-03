package skills

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// The approval record sits inside the skill directory it approves, so every
// whole-tree copier must leave it behind: it is repository governance state,
// not skill content, and a runtime that loaded it would treat it as part of
// the skill. These tests pin the copiers that live in this package; the Pi
// package builder pins its own in engine/pipkg.

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %q: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}

// installedTree runs `skills install` for one skill whose source holds files and
// returns the project it installed into. The source tree is written to disk as given,
// the approval record included.
func installedTree(t *testing.T, files map[string]string) (project string) {
	t.Helper()
	overlay, project := t.TempDir(), t.TempDir()
	for name, content := range files {
		writeTestFile(t, filepath.Join(overlay, "my-skill", filepath.FromSlash(name)), content)
	}
	var out, errBuf bytes.Buffer
	code := -1
	RenderInstallCore(
		[]string{"--registry", "reg.yaml", "--source-root", overlay, "--project-id", "target-repo"},
		testRegistries(func(string) ([]byte, error) {
			return []byte(makeInstallRegistryYAML("my-skill", []string{"target-repo"})), nil
		}),
		func() (string, error) { return project, nil },
		&out, &errBuf, func(c int) { code = c },
	)
	if code != 0 {
		t.Fatalf("install: exit %d, stderr %q", code, errBuf.String())
	}
	return project
}

func TestInstall_DoesNotProjectTheApprovalRecord(t *testing.T) {
	project := installedTree(t, map[string]string{
		"SKILL.md":            "body\n",
		"references/notes.md": "notes\n",
		ApprovalRecordName:    string(goodRecordJSON("my-skill", abcDigest)),
	})

	for _, runtime := range []string{".claude", ".agents"} {
		dst := filepath.Join(project, runtime, "skills", "my-skill")
		for _, want := range []string{"SKILL.md", filepath.Join("references", "notes.md")} {
			if _, err := os.Stat(filepath.Join(dst, want)); err != nil {
				t.Errorf("skill content %q must still be projected into %s: %v", want, runtime, err)
			}
		}
		if _, err := os.Stat(filepath.Join(dst, ApprovalRecordName)); err == nil {
			t.Errorf("the approval record must not be projected into %s", dst)
		}
	}
}

func TestInstall_OnlyTheRootLevelRecordIsSkipped(t *testing.T) {
	// A file that merely shares the record's name deeper in the tree belongs to
	// the skill's own content and is copied as usual.
	project := installedTree(t, map[string]string{
		"SKILL.md":                         "body\n",
		"references/" + ApprovalRecordName: "skill-owned\n",
	})
	if _, err := os.Stat(filepath.Join(project, ".claude", "skills", "my-skill", "references", ApprovalRecordName)); err != nil {
		t.Errorf("a nested file sharing the record name is skill content and must be copied: %v", err)
	}
}

func TestInstall_ADirectoryInTheRecordsPlaceIsNotProjectedEither(t *testing.T) {
	project := installedTree(t, map[string]string{
		"SKILL.md":                        "body\n",
		ApprovalRecordName + "/inner.txt": "not skill content\n",
	})
	if _, err := os.Stat(filepath.Join(project, ".claude", "skills", "my-skill", ApprovalRecordName)); err == nil {
		t.Errorf("nothing at the record's path may be projected, directory included")
	}
}
