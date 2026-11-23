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
	classifierDescription = "Classifier role, off the role chain, read-only: classifies the findings one review kept (send them, the review identifier and date, the evidence page path, the general rules path and the evidence index) by the classify rules, and returns the Kinds and Evidence rows for the parent to append to docs/wiki/evidence.md."
	classifierClaudeTools = "Read, Grep, Glob"
)

// classifierNoticeHeader is the first comment line of the notice: it names the origin and the license.
const classifierNoticeHeader = "# This agent includes material from andresfrei/playbook under the MIT License below; see roles/classifier/PROVENANCE.md in the source repository."

// classifierNotice is the notice comments of the file as the design of T3c-b writes them (L47, L39): the
// header line, a "#" line, then each line of the license as "# " plus the line, or "#" for a blank one.
func classifierNotice(t *testing.T, root string) string {
	t.Helper()
	license := strings.TrimSuffix(readText(t, filepath.Join(root, "roles", "classifier", "LICENSE")), "\n")
	out := classifierNoticeHeader + "\n#\n"
	for _, line := range strings.Split(license, "\n") {
		if line == "" {
			out += "#\n"
		} else {
			out += "# " + line + "\n"
		}
	}
	return out
}

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

	want := "---\n" + classifierNotice(t, root) + "name: classifier\ndescription: " + classifierDescription + "\nmodel: sonnet\ntools: " + classifierClaudeTools + "\n---\n\n" +
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

// The license of the source travels with the copy (R5-F2): every line of roles/classifier/LICENSE, in
// order, is a comment line of the frontmatter before name:, so the notice adds no prompt text.
func TestTheClassifierFileCarriesTheLicenseNoticeOfItsSource(t *testing.T) {
	root := seededRoot(t)
	got := artifactContents(t, root)["agents/classifier.md"]
	lines := strings.Split(got, "\n")
	nameAt := slices.Index(lines, "name: classifier")
	if lines[0] != "---" || nameAt < 0 {
		t.Fatalf("agents/classifier.md has no frontmatter that starts with --- and holds name: classifier:\n%s", got)
	}
	front := lines[1:nameAt]
	if len(front) == 0 || front[0] != classifierNoticeHeader {
		t.Fatalf("the first frontmatter line is not the notice header %q: %v", classifierNoticeHeader, front)
	}
	at := 1
	for _, line := range strings.Split(strings.TrimSuffix(readText(t, filepath.Join(root, "roles", "classifier", "LICENSE")), "\n"), "\n") {
		want := "# " + line
		if line == "" {
			want = "#"
		}
		next := slices.Index(front[at:], want)
		if next < 0 {
			t.Errorf("the license line %q is not a frontmatter comment line, in order, before name:", line)
			continue
		}
		at += next + 1
	}
	for _, line := range front {
		if !strings.HasPrefix(line, "#") {
			t.Errorf("a frontmatter line before name: is not a comment: %q", line)
		}
	}
}
