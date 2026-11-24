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
	var got []string
	for _, name := range strings.Split(sentences[1], ",") {
		got = append(got, strings.TrimSpace(name))
	}

	var want []string
	for _, r := range roles {
		if r.tier == tierMid {
			want = append(want, r.name)
		}
	}
	for _, r := range claudeOnlyRoles {
		if r.tier == tierMid {
			want = append(want, r.name)
		}
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("the Mid line of models.md names %q, want the mid roles of the generator %q", got, want)
	}
}
