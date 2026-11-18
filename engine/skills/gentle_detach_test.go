package skills_test

// S17 (gentle-ai detach), T28: the skills the overlay adopted from gentle-ai are labdrian's own. No
// adopted skill names a gentle author, none tells an agent to run the gentle-ai binary, and each one
// is a custom skill with no upstream in the registry and a custom row in the manifest. The tag a
// registry entry implies is manifestTagFor's (engine/skills/sync.go), reached here through
// SyncManifest: a manifest whose tags differ from what the registry implies is regenerated, and
// that is the difference the test reports.
//
// branch-pr and issue-creation (rewritten in T32 and T33) and systemic-issue-triage (deferred to a
// later feature) are not in the set.

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// adoptedSkillDirs are the directories under skills/ that T28 makes labdrian-owned. The ones also
// named in the registry are listed in adoptedRegistryIDs.
var adoptedSkillDirs = []string{
	"chained-pr", "work-unit-commits", "comment-writer", "cognitive-doc-design", "go-testing",
	"judgment-day", "skill-creator", "skill-improver", "_shared", "requirements-from-transcripts",
	"project-inception", "inception-pipeline", "project-manifest", "project-architect", "roadmap-maker",
}

// _shared is a directory of contracts, not an entry of the registry.
func adoptedRegistryIDs() []string {
	var ids []string
	for _, dir := range adoptedSkillDirs {
		if dir != "_shared" {
			ids = append(ids, dir)
		}
	}
	return ids
}

var (
	authorLine = regexp.MustCompile(`(?m)^\s*author:\s*(.*)$`)
	// The pattern S17 uses to find a call of the binary: the word gentle-ai that is not part of a
	// longer name (gentle-ai-overlay) or a path.
	gentleBinary = regexp.MustCompile(`(^|[^\w.-])gentle-ai([^\w/-]|$)`)
)

func adoptedSkillFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	root := filepath.Join(repoRoot(t), "skills", dir)
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		rel, _ := filepath.Rel(repoRoot(t), path)
		files[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("reading skills/%s: %v", dir, err)
	}
	return files
}

func TestAdoptedSkillsNameNoGentleAuthor(t *testing.T) {
	for _, dir := range adoptedSkillDirs {
		text, err := os.ReadFile(filepath.Join(repoRoot(t), "skills", dir, "SKILL.md"))
		if err != nil {
			t.Fatalf("reading skills/%s/SKILL.md: %v", dir, err)
		}
		match := authorLine.FindStringSubmatch(string(text))
		if match == nil {
			t.Errorf("skills/%s/SKILL.md has no author line", dir)
			continue
		}
		author := strings.ToLower(strings.Trim(strings.TrimSpace(match[1]), `"'`))
		if author != "labdrian" {
			t.Errorf("skills/%s/SKILL.md author = %q, want labdrian", dir, author)
		}
	}
}

func TestAdoptedSkillsNeverTellAnAgentToRunTheGentleAIBinary(t *testing.T) {
	for _, dir := range adoptedSkillDirs {
		for path, text := range adoptedSkillFiles(t, dir) {
			// _shared also holds the sdd-* contracts, which T29 deletes; only its SKILL.md is adopted.
			if dir == "_shared" && path != "skills/_shared/SKILL.md" {
				continue
			}
			for i, line := range strings.Split(text, "\n") {
				if gentleBinary.MatchString(line) {
					t.Errorf("%s:%d names the gentle-ai binary: %s", path, i+1, strings.TrimSpace(line))
				}
			}
		}
	}
}

// judgment-day carried gentle's negotiated review: a link to its ledger contract and a paragraph on
// capturing a lens. Neither is part of the labdrian flow.
func TestJudgmentDayCarriesNoGentleReviewLifecycle(t *testing.T) {
	for path, text := range adoptedSkillFiles(t, "judgment-day") {
		for _, banned := range []string{"review-ledger-contract", "capture-result", "negotiated"} {
			if strings.Contains(text, banned) {
				t.Errorf("%s still names %q", path, banned)
			}
		}
	}
}

func TestSkillRegistryRefreshIsLabdriansInTheAuthoringSkills(t *testing.T) {
	for _, rel := range []string{
		"skills/skill-creator/references/skill-style-guide.md",
		"skills/skill-improver/references/skill-style-guide.md",
		"skills/skill-improver/SKILL.md",
	} {
		data, err := os.ReadFile(filepath.Join(repoRoot(t), rel))
		if err != nil {
			t.Fatalf("reading %s: %v", rel, err)
		}
		if !strings.Contains(string(data), "`labdrian skill-registry refresh`") {
			t.Errorf("%s does not name `labdrian skill-registry refresh`", rel)
		}
	}
}

func TestAdoptedSkillsAreCustomWithNoUpstreamAndTheirManifestRowsAgree(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "skills.registry.yaml"))
	if err != nil {
		t.Fatalf("reading the registry: %v", err)
	}
	reg, err := decode(data)
	if err != nil {
		t.Fatalf("parsing the registry: %v", err)
	}
	for _, id := range adoptedRegistryIDs() {
		var found []skills.Entry
		for _, entry := range reg.Skills {
			if entry.ID == id {
				found = append(found, entry)
			}
		}
		if len(found) != 1 {
			t.Errorf("registry entries named %s = %d, want 1", id, len(found))
			continue
		}
		if found[0].Source.Type != "custom" || found[0].Source.Upstream != nil {
			t.Errorf("%s source = %+v, want type custom with no upstream", id, found[0].Source)
		}
	}

	manifest, err := os.ReadFile(filepath.Join(repoRoot(t), "overlay.manifest"))
	if err != nil {
		t.Fatalf("reading the manifest: %v", err)
	}
	tags := map[string]string{}
	for _, row := range skills.ReadManifestRows(manifest) {
		tags[row.Path] = row.Tag
	}
	for _, dir := range adoptedSkillDirs {
		if tag := tags[dir+"/SKILL.md"]; tag != "custom" {
			t.Errorf("manifest row %s/SKILL.md tag = %q, want custom", dir, tag)
		}
	}
	// Every other file of an adopted skill is ours too: a managed reference would be copied back
	// from upstream by capture over the edits this change makes. Of _shared only SKILL.md is adopted:
	// T29 removes its other files.
	for path, tag := range tags {
		for _, dir := range adoptedSkillDirs {
			if dir != "_shared" && strings.HasPrefix(path, dir+"/") && tag != "custom" {
				t.Errorf("manifest row %s tag = %q, want custom", path, tag)
			}
		}
	}
	// The registry and the manifest stay in step: SyncManifest, which applies manifestTagFor,
	// retags nothing.
	_, report, err := skills.SyncManifest(reg, manifest)
	if err != nil {
		t.Fatalf("SyncManifest: %v", err)
	}
	if len(report.Retagged) != 0 {
		t.Errorf("the manifest tags differ from what the registry implies: %+v", report)
	}
}
