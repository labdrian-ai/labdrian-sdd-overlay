package skills

// The classifier role (S5, S9, T3 of odd/tasks/merge-playbook-into-sdd-overlay.md, L37): its source adapted
// from playbook's classify.md, its deploy to Claude Code alone (OD-7), and the license and provenance that
// travel with the source.

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestClassifierBodyIsTheClassifyAgentAdapted(t *testing.T) {
	root := skillsRepoRoot(t)
	body := readRepoFile(t, root, "roles/classifier/body.md")

	lines := strings.Split(body, "\n")
	if len(lines) < 6 || !reflect.DeepEqual(lines[:6], roleCorePrefix("classifier")) {
		t.Errorf("the first six lines of roles/classifier/body.md are not the core prefix of the role bodies")
	}

	// K1 (L47, R2-F1): the Rules, Gotchas and Output sections equal their want-lists line by line, so a
	// reworded or deleted line fails the test.
	for _, section := range []struct {
		heading string
		want    []string
	}{
		{"## Rules", classifierRules},
		{"## Gotchas", classifierGotchas},
		{"## Output", classifierOutput},
	} {
		if got := bodySectionLines(body, section.heading); !reflect.DeepEqual(got, section.want) {
			t.Errorf("%s of roles/classifier/body.md =\n%q\nwant\n%q", section.heading, got, section.want)
		}
	}

	steps := strings.Join(bodySectionLines(body, "## Steps"), "\n")
	for _, anchor := range []string{
		"Read `<page>`",
		"<index>",
		"would the rule of this kind have prevented this finding?",
		"Return one Evidence row per finding",
		"Check each row against the Format",
		"one Kinds row per kind, never for a kind `<page>` already has",
		"a rule in every Kinds row at level 2 or more",
		"for the kind of every Evidence row",
	} {
		if !strings.Contains(steps, anchor) {
			t.Errorf("the Steps of roles/classifier/body.md do not hold %q", anchor)
		}
	}
	for _, anchor := range []string{"<rules>", "Kinds rows:", "Evidence rows:", "When one of the three is not given, stop and say so.", "<failed>"} {
		if !strings.Contains(body, anchor) {
			t.Errorf("roles/classifier/body.md does not hold %q", anchor)
		}
	}
	lower := strings.ToLower(body)
	for _, banned := range []string{"claude_plugin_root", "~/.claude", "$home", `sh "`, "evidence.sh", "engram", "script counts that review"} {
		if strings.Contains(lower, banned) {
			t.Errorf("roles/classifier/body.md holds %q", banned)
		}
	}
}

// bodySectionLines are the non-blank lines of the section that starts at heading, up to the next "## ".
func bodySectionLines(body, heading string) []string {
	var out []string
	in := false
	for _, line := range strings.Split(body, "\n") {
		switch {
		case line == heading:
			in = true
		case strings.HasPrefix(line, "## "):
			in = false
		case in && line != "":
			out = append(out, line)
		}
	}
	return out
}

var classifierRules = []string{
	"- Change no file and run no command: return the rows under Output, and the parent appends them to `<page>` and runs the evidence check.",
	"- Only add rows. Never return a row that deletes, edits, reorders, or consolidates a row, and never rename a kind. Consolidation is proposed to the owner at the close of a work.",
	"- A kind names the mistake, not the place: `test-passes-when-broken`, not `auth-bug`. It is kebab-case, and its rule fits in one sentence a step can act on.",
	"- Write no secret and no customer data in a row. A finding is one general line.",
	"- A script counts. Never count in your head or write a count in a row.",
	"- A kind's level and rule come from `<page>` first, then from `<rules>`. A kind in neither has no level or rule to take.",
}

var classifierGotchas = []string{
	"- Two kinds that look close, such as `missing-test` and `test-passes-when-broken`, are settled by the central test: \"write the test\" does not prevent a test that exists and misses the failure, so they are two kinds.",
	"- A review that lists the same mistake in several places gives one row per place, all with the same review identifier. The parent's count takes that review once. Do not merge the rows.",
	"- A finding the review marks as not real, dismissed, or already fixed before the review is not evidence. Skip it.",
	"- A kind with no rule is at level 1. A kind whose rule at level k did not prevent the finding gets the row at level k, the kind's current level.",
	"- Never return the example rows of the format.",
}

var classifierOutput = []string{
	"- A line `Kinds rows:`, then one fence opened by `` ```markdown `` with the Kinds rows to add, one per line, with no header; an empty fence when there are none.",
	"- A line `Evidence rows:`, then one fence opened by `` ```markdown `` with the Evidence rows to add, the same way.",
	"- Any finding skipped, with the reason.",
}

func TestClassifierIsDeployedToClaudeAlone(t *testing.T) {
	root := skillsRepoRoot(t)
	rows := ReadManifestRows([]byte(readRepoFile(t, root, "overlay.manifest")))

	var found []ManifestRow
	for _, row := range rows {
		if row.Path == "classifier.md" {
			found = append(found, row)
		}
		if row.Path == "codex/agents/classifier.toml" || row.Path == "opencode/agents/classifier.md" {
			t.Errorf("the manifest has a row for %s: the classifier is for Claude Code alone", row.Path)
		}
	}
	if want := []ManifestRow{{Path: "classifier.md", Tag: "custom", Route: "agent"}}; !reflect.DeepEqual(found, want) {
		t.Fatalf("rows named classifier.md = %+v, want exactly %+v", found, want)
	}

	for target, want := range map[string]string{"claude": "/h/.claude/agents/classifier.md", "codex": "", "opencode": "", "pi": ""} {
		routes, err := ResolveRoutes(rows, false, "/o", RouteDestinations{Target: target, SkillsRoot: "/h/skills", AgentsRoot: "/h/.claude/agents"})
		if err != nil {
			t.Fatalf("%s: ResolveRoutes over the manifest of the repository: %v", target, err)
		}
		seen := false
		for _, r := range routes {
			if r.Path == "classifier.md" {
				seen = true
				if r.Dest != want {
					t.Errorf("%s: classifier.md goes to %q, want %q", target, r.Dest, want)
				}
			}
		}
		if !seen {
			t.Errorf("%s: no route for classifier.md", target)
		}
	}

	if _, err := os.Stat(filepath.Join(root, "agents", "classifier.md")); err != nil {
		t.Errorf("agents/classifier.md: %v", err)
	}
	// A Pi file would be shipped by the Pi package, which copies every pi/agents/*.md.
	for _, absent := range []string{"pi/agents/classifier.md", "codex/agents/classifier.toml", "opencode/agents/classifier.md"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(absent))); err == nil {
			t.Errorf("%s exists: the classifier is for Claude Code alone", absent)
		}
	}
}

func TestClassifierLicenseAndProvenanceTravelWithTheSource(t *testing.T) {
	root := skillsRepoRoot(t)
	if got, want := gitBlobID(readRepoFile(t, root, "roles/classifier/LICENSE")), evidenceSnapshotBlobs["LICENSE"]; got != want {
		t.Errorf("roles/classifier/LICENSE has blob %s, want the snapshot's %s", got, want)
	}
	provenance := readRepoFile(t, root, "roles/classifier/PROVENANCE.md")
	for _, want := range []string{
		"6de8566bf7f1150f49a75c1162c0f4cf87bf57c5",
		"v3.6.0",
		"andresfrei/playbook",
		"plugins/playbook-agents/agents/classify.md",
		"MIT",
	} {
		if !strings.Contains(provenance, want) {
			t.Errorf("roles/classifier/PROVENANCE.md does not hold %q", want)
		}
	}
	if section := changedSinceTheSnapshot(t, provenance); section == "" || section == "- Nothing." {
		t.Errorf("the Changed section of roles/classifier/PROVENANCE.md is %q, want the list of changes", section)
	}
	// The Changed line that says the body names the feed quotes the body: the quoted text is in it (T3c-f, F4).
	const feedLineEnd = " names the feed of the evidence store."
	quoted := ""
	for _, line := range strings.Split(provenance, "\n") {
		if strings.HasSuffix(line, feedLineEnd) {
			_, rest, _ := strings.Cut(line, `"`)
			quoted, _, _ = strings.Cut(rest, `"`)
		}
	}
	if quoted == "" {
		t.Errorf("roles/classifier/PROVENANCE.md has no Changed line that quotes the body and ends with %q", feedLineEnd)
	} else if body := readRepoFile(t, root, "roles/classifier/body.md"); !strings.Contains(body, quoted) {
		t.Errorf("roles/classifier/PROVENANCE.md quotes %q, which roles/classifier/body.md does not hold", quoted)
	}
}
