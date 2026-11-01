package main

// The user documents of `review launch` (own-rdd T8c, S9 d and e): the engine usage, the help of the wrapper, the README
// entry, the header comment of review.go, the CHANGELOG entry and the two lines of per-role-design-v3.md that the verb
// reconciles. A fragment is written on one line with no code marks, because the texts are compared after normalizeDocText.

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// launchFacts are what every help of the verb says, in the words they share.
var launchFacts = []string{
	"the one verb that starts a reviewer, on Codex read-only and on Claude Code with no tool",
	"codex exec --sandbox read-only --model <model> --cd <worktree>",
	"--ignore-user-config",
	"--runtime must be claude or codex: on Pi the parent launches the reviewer agent natively",
	"run: labdrian-overlay apply --target codex",
	"run: labdrian-overlay apply --target claude",
	"## Frozen context",
	"git diff --no-renames --function-context",
	"over the 150000-byte limit of one launch",
	"does not name the full base, the full head, the diff_sha256 and the worktree path of the binding",
	"names codex and repeats what it printed on stderr",
}

func TestUsageDocumentsReviewLaunch(t *testing.T) {
	var out bytes.Buffer
	usage(&out)
	section := docSection(t, out.String(), "  engine review launch", "  engine review stop-hook")

	for _, want := range append([]string{
		"engine review launch --runtime claude|codex --feature <name> --base <rev> --head <rev> --rows <id,id> --model <model> [--cwd <repo>]",
		"exit 0 stored, 2 reply refused, 1 usage error or any other failure",
	}, launchFacts...) {
		if !strings.Contains(section, want) {
			t.Errorf("the engine usage of review launch does not say %q", want)
		}
	}
	if text := normalizeDocText(out.String()); strings.Contains(text, "No verb launches an agent or dispatches work") {
		t.Errorf("the engine usage still says that no verb launches an agent")
	}
}

func TestWrapperUsageOfReviewLaunchSaysWhatTheEngineUsageSays(t *testing.T) {
	section := docSection(t, readRepositoryFile(t, "bin/lib/usage.sh"), "                                   launch --runtime", "                                   stop-hook\n")

	for _, want := range append([]string{
		"launch --runtime claude|codex --feature NAME --base REV --head REV --rows ID,ID --model MODEL [--cwd REPO]",
		"(exit 0 stored, 2 reply refused, 1 usage error or any other failure)",
	}, launchFacts...) {
		if !strings.Contains(section, want) {
			t.Errorf("bin/lib/usage.sh does not say %q about review launch", want)
		}
	}
	if text := normalizeDocText(readRepositoryFile(t, "bin/lib/usage.sh")); strings.Contains(text, "No verb launches an agent or dispatches work.") {
		t.Errorf("bin/lib/usage.sh still says that no verb launches an agent")
	}
}

func TestReadmeDocumentsReviewLaunch(t *testing.T) {
	readme := readRepositoryFile(t, "README.md")
	section := docSection(t, readme, "    launch --runtime", "    stop-hook\n")

	for _, want := range append([]string{
		"launch --runtime claude|codex --feature <name> --base <rev> --head <rev> --rows <id,id> --model <model> [--cwd <repo>]",
		"Exit 0 stored, 2 reply refused, 1 usage error or any other failure",
	}, launchFacts...) {
		if !strings.Contains(section, want) {
			t.Errorf("the README entry of review launch does not say %q", want)
		}
	}
	intro := docSection(t, readme, "overlay review <verb>\n", "    bind   --base")
	if !strings.Contains(intro, "launch is the one verb that starts a reviewer, on Codex read-only and on Claude Code with no tool") || strings.Contains(intro, "No verb launches an agent or dispatches work") {
		t.Errorf("the README intro of review <verb> does not name launch as the one verb that starts a reviewer: %q", intro)
	}
}

func TestReviewHeaderCommentNamesLaunch(t *testing.T) {
	data, err := os.ReadFile("review.go")
	if err != nil {
		t.Fatal(err)
	}
	header := normalizeDocText(strings.ReplaceAll(strings.SplitN(string(data), "\nimport (", 2)[0], "\n//", "\n"))
	for _, want := range []string{"'review launch --runtime claude|codex", "launch is the one verb that starts a reviewer, on Codex read-only and on Claude Code with no tool"} {
		if !strings.Contains(header, want) {
			t.Errorf("the header comment of review.go does not say %q", want)
		}
	}
	if strings.Contains(header, "No verb launches an agent") {
		t.Errorf("the header comment of review.go still says that no verb launches an agent")
	}
}

func TestChangelogNamesReviewLaunch(t *testing.T) {
	entry := docSection(t, readRepositoryFile(t, "CHANGELOG.md"), "- `labdrian review launch --runtime codex", "\n")

	for _, want := range []string{"own-rdd T8c", "S9", "--sandbox read-only", "The two lists of verbs of `review` now end with `launch`"} {
		if !strings.Contains(entry, strings.ReplaceAll(want, "`", "")) {
			t.Errorf("the CHANGELOG entry of review launch does not say %q", want)
		}
	}
}

// per-role-design-v3.md says what S9 (e) says on the two lines it reconciles.
func TestDesignV3ReconcilesCodexReviewer(t *testing.T) {
	v3 := normalizeDocText(readRepositoryFile(t, "docs/architecture/role-design/per-role-design-v3.md"))

	for _, want := range []string{
		"Read-only reviewer: enforced on Claude/Pi by tools; on Codex by the sandbox of the separate process labdrian review launch --runtime codex starts (D11), not by prose",
		"A codex exec-driven reviewer runner (parent shells out to Codex): rung 6; native sub-agents exist → cut; adopted by D11 (owner 2026-10-22) and built by own-rdd S9",
	} {
		if !strings.Contains(v3, want) {
			t.Errorf("per-role-design-v3.md does not say %q", want)
		}
	}
	if strings.Contains(v3, "on Codex by prose plus the parent's digest/spot-check") {
		t.Errorf("per-role-design-v3.md still says the Codex reviewer is read-only by prose")
	}
}
