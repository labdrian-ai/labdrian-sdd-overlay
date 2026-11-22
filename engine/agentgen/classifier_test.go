package agentgen_test

// The classifier (S5, T3 of odd/tasks/merge-playbook-into-sdd-overlay.md, L37): the agent that writes
// the findings of one review as rows of the evidence page. It is off the role chain and Claude Code
// only (OD-7): one source, roles/classifier/body.md, and one generated file, agents/classifier.md.

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/agentgen"
)

const (
	classifierDescription = "Classifier role, off the role chain: writes the findings one review kept (send them, the review identifier and the repository root) as rows of docs/wiki/evidence.md by the classify rules; never changes git state."
	classifierClaudeTools = "Read, Grep, Glob, Bash, Edit, Write"
)

// The table holds agents/classifier.md exactly once, directly before the Claude file of the foreman, and
// no Pi, Codex or OpenCode file for it (OD-7: the learning loop is for Claude Code alone).
func TestTheClassifierHasAClaudeFileAndNoOther(t *testing.T) {
	table, err := agentgen.Artifacts(seededRoot(t))
	if err != nil {
		t.Fatalf("Artifacts: %v", err)
	}
	var paths []string
	for _, a := range table {
		paths = append(paths, a.Path)
	}

	count := 0
	for _, p := range paths {
		if p == "agents/classifier.md" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("the table holds agents/classifier.md %d times, want once", count)
	}
	at := slices.Index(paths, "agents/classifier.md")
	if at < 0 || at+1 >= len(paths) || paths[at+1] != "agents/foreman.md" {
		t.Errorf("agents/classifier.md is not directly before agents/foreman.md in %v", paths)
	}
	for _, other := range []string{"pi/agents/classifier.md", "codex/agents/classifier.toml", "opencode/agents/classifier.md"} {
		if slices.Contains(paths, other) {
			t.Errorf("the table holds %s: the classifier is for Claude Code alone", other)
		}
	}
}

// The Claude file of the classifier is its frontmatter (name, description, model, tools, in that order), a
// blank line and its source without the line that sends a role to memorycard, because it holds no
// memorycard tool. It has no effort field: the Claude file takes none.
func TestTheClaudeFileOfTheClassifierIsItsFrontmatterAndItsBody(t *testing.T) {
	root := seededRoot(t)
	source := readText(t, filepath.Join(root, "roles", "classifier", "body.md"))
	if strings.Count(source, memoryLine+"\n") != 1 {
		t.Fatal("roles/classifier/body.md must carry the memory line of the core prefix exactly once")
	}

	got := artifactContents(t, root)["agents/classifier.md"]

	want := "---\nname: classifier\ndescription: " + classifierDescription + "\nmodel: sonnet\ntools: " + classifierClaudeTools + "\n---\n\n" +
		strings.Replace(source, memoryLine+"\n", "", 1)
	if got != want {
		t.Errorf("agents/classifier.md =\n%s\nwant\n%s", got, want)
	}
	if field, found := effortField(got); !found {
		t.Errorf("agents/classifier.md has no frontmatter between two --- lines:\n%s", got)
	} else if field != "" {
		t.Errorf("the frontmatter of agents/classifier.md holds an effort field: %q", field)
	}
	for i, r := range got {
		if r != '\n' && (r < 0x20 || r > 0x7e) {
			t.Errorf("agents/classifier.md holds the non-ASCII or control character %q at byte %d", r, i)
			break
		}
	}
}
