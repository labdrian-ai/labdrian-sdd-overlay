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
	for _, anchor := range []string{
		"$HOME/.claude/skills/_shared/evidence/evidence.sh",
		`sh "<script>"`,
		"would the rule of this kind have prevented this finding?",
		"Never commit, push, or change git state.",
		"Only add rows.",
		"exits 3",
		"`index`",
		"`rule <kind>`",
	} {
		if !strings.Contains(body, anchor) {
			t.Errorf("roles/classifier/body.md does not hold %q", anchor)
		}
	}
	lower := strings.ToLower(body)
	for _, banned := range []string{"claude_plugin_root", "~/.claude", "engram"} {
		if strings.Contains(lower, banned) {
			t.Errorf("roles/classifier/body.md holds %q", banned)
		}
	}
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
}
