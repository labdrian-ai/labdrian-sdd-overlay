package agentgen_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The generated agents are checked in CI (slice 4, S1: "Check in CI"): a step of the engine job runs
// the check of the generator on the repository, so that a role source or a generated file that was
// edited alone fails the build. The test reads the workflow as text, as the shell tests do, and names
// what the step must have: the command, the repository root it is given and the directory it runs in.
func TestTheEngineJobOfCIChecksTheGeneratedAgents(t *testing.T) {
	workflow, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}

	var step []string
	for _, line := range strings.Split(string(workflow), "\n") {
		if strings.HasPrefix(strings.TrimLeft(line, " "), "- name:") {
			if stepChecksAgents(step) {
				break
			}
			step = nil
		}
		step = append(step, line)
	}
	if !stepChecksAgents(step) {
		t.Fatal("no step of .github/workflows/ci.yml runs gadu-generate --check")
	}

	text := strings.Join(step, "\n")
	for _, want := range []string{
		"working-directory: engine",
		"OVERLAY_DIR: ${{ github.workspace }}",
		"run: go run ./cmd/ gadu-generate --check",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the step that checks the generated agents lacks %q:\n%s", want, text)
		}
	}
}

func stepChecksAgents(step []string) bool {
	return strings.Contains(strings.Join(step, "\n"), "gadu-generate --check")
}
