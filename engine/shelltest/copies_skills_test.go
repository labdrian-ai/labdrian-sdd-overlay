package shelltest

// apply.sh, sync_check.sh and skill_registry.sh ask copies_skills before they touch the skills directory
// of a target. A target that reads the skills of ~/.claude/skills (OpenCode) gets no copy of its own, and
// the three scripts must all see it the same way: the integration tests of apply and sync-check pin what
// they do with it, and this pins the answer they all read.

import (
	"os"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

func TestCopiesSkillsSaysWhichTargetsGetTheirOwnCopy(t *testing.T) {
	overlaytest.RequireBash(t)
	script := overlaytest.Script(t)
	const snippet = `for target in claude opencode codex pi unknown; do
  if copies_skills "$target"; then echo "$target copies"; else echo "$target reads-claude"; fi
done
echo "callers: $(declare -f cmd_apply cmd_sync_check repair_sdd_registry_file | grep -c copies_skills)"`
	shell := overlaytest.Shell{Env: []string{"HOME=" + t.TempDir(), "PATH=" + os.Getenv("PATH")}, Dir: t.TempDir()}

	out, err := overlaytest.Source(t, script, shell, snippet)

	want := "claude copies\nopencode reads-claude\ncodex copies\npi copies\nunknown copies\ncallers: 3\n"
	if err != nil || out != want {
		t.Errorf("copies_skills answers, then the number of the commands that ask it = %q, %v, want %q", out, err, want)
	}
}
