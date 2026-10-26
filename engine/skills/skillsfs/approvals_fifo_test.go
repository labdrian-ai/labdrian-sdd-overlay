//go:build linux || darwin

package skillsfs_test

import (
	"io/fs"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/skillsfs"
)

// A named pipe in a skill is reported as one and never opened: opening a FIFO for reading blocks
// until a writer appears, so a store that read every entry naively would hang `skills validate`
// (and a session that runs it) on a directory anybody could put a pipe in.
func TestApprovalsReportANamedPipeAndNeverBlockOnIt(t *testing.T) {
	root := t.TempDir()
	put(t, filepath.Join(root, "alpha", "SKILL.md"), "skill bytes")
	if err := syscall.Mkfifo(filepath.Join(root, "alpha", "pipe"), 0o600); err != nil {
		t.Skipf("mkfifo unavailable: %v", err)
	}

	type outcome struct {
		entries []skills.SkillEntry
		err     error
	}
	done := make(chan outcome, 1)
	go func() {
		entries, err := skillsfs.Approvals{}.ReadSkillTree(root, "alpha")
		done <- outcome{entries, err}
	}()

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("ReadSkillTree: %v, want the pipe reported, not a failure", got.err)
		}
		var pipe *skills.SkillEntry
		for i := range got.entries {
			if got.entries[i].Rel == "pipe" {
				pipe = &got.entries[i]
			}
		}
		if pipe == nil || pipe.Type&fs.ModeNamedPipe == 0 || len(pipe.Data) != 0 {
			t.Fatalf("entries = %v, want the pipe listed as a named pipe with no bytes", describe(got.entries))
		}
		if _, err := skills.SkillDigest(got.entries); err == nil {
			t.Error("SkillDigest covered a skill with a named pipe in it")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ReadSkillTree blocked on a named pipe in the skill")
	}
}
