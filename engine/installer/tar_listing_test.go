package installer_test

// capture --from-backup and bootstrap look for the path of each managed row in the listing of the
// snapshot they extract from. The listing is a shell variable, and it used to be offered to grep by
// `echo "$listing" | grep -qF "$path"`: under pipefail, when the listing is larger than a pipe holds
// and the path is near its start, grep leaves as soon as it has a match, echo is killed by SIGPIPE
// while it is still writing, the pipeline fails, and the file is reported "not found in tarball"
// and skipped. A snapshot of a real home is such a listing. The listing is a here-string now.

import (
	"fmt"
	"strings"
	"testing"
)

// largeSnapshotFiles is the files of the standard snapshot (writeSnapshot) among about 400 KB of
// listing, six times what a pipe holds.
func largeSnapshotFiles() map[string]string {
	files := map[string]string{
		"files/home/labdrian/.claude/skills/skill-a/SKILL.md":      "# skill-a backup\n",
		"files/home/labdrian/.claude/skills/skill-a/refs/notes.md": "# notes backup\n",
		"files/home/labdrian/.claude/skills/shared/helper.md":      "# helper backup\n",
		"files/home/labdrian/.claude/agents/GADU.md":               "# GADU claude backup\n",
		"files/home/labdrian/.config/opencode/agents/GADU.md":      "# GADU opencode backup\n",
		"files/home/labdrian/.claude/weird/SKILL.md":               "# weird backup\n",
		"files/home/labdrian/.claude/skills/dup/SKILL.md":          "# dup backup\n",
	}
	for i := 0; i < 3000; i++ {
		files[fmt.Sprintf("files/home/labdrian/.claude/skills/filler/%s/%04d.md", strings.Repeat("x", 90), i)] = ""
	}
	return files
}

// checkEverythingInTheListingWasFound holds that each file of the standard snapshot was extracted,
// and that the only files said not to be in the tarball are the two rows that have none.
func checkEverythingInTheListingWasFound(t *testing.T, run pinRun) {
	t.Helper()
	if run.Exit != 0 {
		t.Fatalf("the command = %s", pinRunText(run))
	}
	for _, extracted := range []string{
		"skills/skill-a/SKILL.md", "skills/skill-a/refs/notes.md", "skills/shared/helper.md",
		"agents/GADU.md", "opencode/agents/GADU.md", "weird/SKILL.md", "skills/dup/SKILL.md",
	} {
		if !strings.Contains(run.Stdout, "extracted: "+extracted+"\n") {
			t.Errorf("%s is in the snapshot and was not extracted:\n%s", extracted, run.Stdout)
		}
	}
	if got := strings.Count(run.Stdout, "WARNING: not found in tarball"); got != 2 {
		t.Errorf("%d files were said not to be in the tarball, want the 2 that are not:\n%s", got, run.Stdout)
	}
}

func TestCaptureFromABackupFindsTheFilesOfAListingLargerThanAPipe(t *testing.T) {
	w := fullPinWorld(t)
	snapshot := writeSnapshotOf(t, w, "snapshot.tar.gz", largeSnapshotFiles())

	checkEverythingInTheListingWasFound(t, w.run("capture", "--from-backup", snapshot))
}

func TestBootstrapFindsTheFilesOfAListingLargerThanAPipe(t *testing.T) {
	w := newBootstrapWorld(t)
	writeSnapshotOf(t, w, ".gentle-ai/backups/upgrade-20260615T175529Z/snapshot.tar.gz", largeSnapshotFiles())

	checkEverythingInTheListingWasFound(t, w.run("bootstrap"))
}
