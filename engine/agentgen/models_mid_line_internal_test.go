package agentgen

// The Mid line of models.md names every role of the mid tier (T4c of odd/tasks/merge-playbook-into-sdd-overlay.md, L90,
// `7704d28b9678-2` F2, `-5` F4): the classifier is a Claude-only mid role, and the line a parent reads to pick a model must
// list it. The roles come from the generator's own tables, so this test reads unexported rows and lives in package agentgen.

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestTheMidLineOfModelsNamesEveryMidRole(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "skills", "role-chain", "references", "models.md"))
	if err != nil {
		t.Fatal(err)
	}
	var line string
	for _, l := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(l, "- Mid, ") {
			line = l
		}
	}
	if line == "" {
		t.Fatal(`models.md has no line that starts with "- Mid, "`)
	}
	_, after, ok := strings.Cut(line, ": ")
	if !ok {
		t.Fatalf("the Mid line %q has no colon", line)
	}
	// The line is "- Mid, medium: <what the tier does>. <roles>. <model ids>.": the roles are its second sentence.
	sentences := strings.Split(after, ". ")
	if len(sentences) < 3 {
		t.Fatalf("the Mid line %q has no second sentence that lists the roles", line)
	}
	// A role the generator deploys to Claude Code alone carries the mark " (Claude Code only)" after its name (`8ee5ac95b287-5`
	// F3, L98 choice 7), so a parent on another runtime does not look for it; no other role carries it.
	const claudeOnlyMark = " (Claude Code only)"
	var got, marked []string
	for _, name := range strings.Split(sentences[1], ",") {
		name = strings.TrimSpace(name)
		if trimmed, ok := strings.CutSuffix(name, claudeOnlyMark); ok {
			name = trimmed
			marked = append(marked, name)
		}
		got = append(got, name)
	}

	var want, wantMarked []string
	for _, r := range roles {
		if r.tier == tierMid {
			want = append(want, r.name)
		}
	}
	for _, r := range claudeOnlyRoles {
		if r.tier == tierMid {
			want = append(want, r.name)
			wantMarked = append(wantMarked, r.name)
		}
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("the Mid line of models.md names %q, want the mid roles of the generator %q", got, want)
	}
	slices.Sort(marked)
	slices.Sort(wantMarked)
	if len(wantMarked) == 0 {
		t.Fatal("the generator has no Claude-only mid role, so the mark is checked against nothing")
	}
	if !slices.Equal(marked, wantMarked) {
		t.Errorf("the Mid line of models.md marks %q %q, want exactly the Claude-only mid roles %q", claudeOnlyMark, marked, wantMarked)
	}
}
