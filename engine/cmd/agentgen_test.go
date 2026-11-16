package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func agentgenRun(t *testing.T, env map[string]string, args ...string) *capturedProcess {
	t.Helper()
	p := newCapturedProcess()
	d := testDeps()
	d.getenv = environmentOf(env)
	runAgentgenGenerate(p.process, d, args)
	return p
}

// seedRoleSources puts what the generator reads from the repository under root: the role sources, and
// the tier table of the role-chain skill that holds each role's model id.
func seedRoleSources(t *testing.T, root string) {
	t.Helper()
	const repo = "../.."
	source := filepath.Join(repo, "roles")
	err := filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(repo, path)
		if err != nil {
			return err
		}
		if info.IsDir() {
			return os.MkdirAll(filepath.Join(root, rel), 0o755)
		}
		return copyFileTo(path, filepath.Join(root, rel))
	})
	if err != nil {
		t.Fatalf("copying the role sources: %v", err)
	}
	models := filepath.Join("skills", "role-chain", "references", "models.md")
	if err := os.MkdirAll(filepath.Join(root, filepath.Dir(models)), 0o755); err != nil {
		t.Fatalf("creating the model table directory: %v", err)
	}
	if err := copyFileTo(filepath.Join(repo, models), filepath.Join(root, models)); err != nil {
		t.Fatalf("copying the model table: %v", err)
	}
}

func copyFileTo(from, to string) error {
	data, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return os.WriteFile(to, data, 0o644)
}

// foremanBodyCommands are the `<group> <verb>` pairs the foreman body names inside backticks, which
// must carry the leading `labdrian`; a span whose second word is a flag, a placeholder or absent is no verb.
func foremanBodyCommands(t *testing.T) [][2]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "roles", "foreman", "body.md"))
	if err != nil {
		t.Fatalf("reading the foreman body: %v", err)
	}
	groups := map[string]bool{"workflow": true, "review": true, "skills": true, "roles": true, "shaper": true, "projection": true}
	var commands [][2]string
	for _, span := range regexp.MustCompile("`([^`]+)`").FindAllStringSubmatch(string(data), -1) {
		fields := strings.Fields(strings.TrimPrefix(span[1], "labdrian "))
		if len(fields) < 2 || !groups[fields[0]] || !regexp.MustCompile(`^[a-z][a-z-]*$`).MatchString(fields[1]) {
			continue
		}
		if !strings.HasPrefix(span[1], "labdrian ") {
			t.Errorf("the foreman body names `%s` without the `labdrian ` prefix", span[1])
		}
		commands = append(commands, [2]string{fields[0], fields[1]})
	}
	return commands
}

// verbsOf are the verbs the program says a group takes when it is run with none (the message of the
// public interface, so the test does not read the dispatcher's tables).
func verbsOf(t *testing.T, group string) []string {
	t.Helper()
	p := newCapturedProcess()
	d := testDeps()
	d.getenv = environmentOf(map[string]string{"HOME": t.TempDir()})
	d.getwd = func() (string, error) { return t.TempDir(), nil }
	run(p.process, d, []string{group})
	_, list, ok := strings.Cut(strings.SplitN(p.err.String(), "\n", 2)[0], " requires a verb: ")
	if !ok {
		t.Fatalf("`%s` with no verb said %q, want it to list its verbs", group, p.err.String())
	}
	return strings.Split(list, ", ")
}

// Every `labdrian` verb the foreman body names is a verb the program takes: a verb renamed or removed
// fails here instead of leaving the foreman to run a command that does not exist (S2).
func TestEveryLabdrianVerbTheForemanBodyNamesExists(t *testing.T) {
	commands := foremanBodyCommands(t)
	named := map[string]bool{}
	for _, c := range commands {
		named[c[0]+" "+c[1]] = true
		found := false
		for _, verb := range verbsOf(t, c[0]) {
			found = found || verb == c[1]
		}
		if !found {
			t.Errorf("the foreman body names `%s %s`, and `%s` takes no verb %q", c[0], c[1], c[0], c[1])
		}
	}
	// The body names the verbs of its stage table: a parse that finds none proves nothing.
	for _, want := range []string{"workflow open", "workflow close", "review bind", "review record", "review verify", "review check", "review sweep", "skills approve"} {
		if !named[want] {
			t.Errorf("the foreman body no longer names `%s`; the test read %v", want, commands)
		}
	}
}

// Every skill the foreman body names is a skill of the repository (S2). A backticked name is a skill
// when it is written next to the word "skill" ("the `x` skill", "skill `x`") or equals a directory of
// skills/; each such name has its SKILL.md. The three skills S2 says the body sends work to must be
// named, so a parse that finds none proves nothing.
func TestEverySkillTheForemanBodyNamesExists(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "roles", "foreman", "body.md"))
	if err != nil {
		t.Fatalf("reading the foreman body: %v", err)
	}
	body := string(data)
	hasSkill := func(name string) bool {
		_, err := os.Stat(filepath.Join("..", "..", "skills", name, "SKILL.md"))
		return err == nil
	}
	named := map[string]bool{}
	for _, pattern := range []string{"`([a-z][a-z-]*)` skill\\b", "\\bskill `([a-z][a-z-]*)`"} {
		for _, m := range regexp.MustCompile(pattern).FindAllStringSubmatch(body, -1) {
			named[m[1]] = true
			if !hasSkill(m[1]) {
				t.Errorf("the foreman body names the skill `%s`, and skills/%s/SKILL.md is not there", m[1], m[1])
			}
		}
	}
	for _, m := range regexp.MustCompile("`([a-z][a-z-]*)`").FindAllStringSubmatch(body, -1) {
		if hasSkill(m[1]) {
			named[m[1]] = true
		}
	}
	for _, want := range []string{"inception-pipeline", "role-chain", "odd-feature-doc"} {
		if !named[want] {
			t.Errorf("the foreman body no longer names the skill `%s`", want)
		}
	}
}

func TestGaduGenerateNeedsTheOverlayDirectory(t *testing.T) {
	p := agentgenRun(t, nil)

	want := "gadu-generate: OVERLAY_DIR is not set\n  Run: OVERLAY_DIR=<overlay-repo-root> gentle-ai-overlay gadu-generate\n"
	if p.err.String() != want || len(p.exits) != 1 || p.exits[0] != 1 || p.out.Len() != 0 {
		t.Errorf("stderr %q, exits %v, stdout %q; want %q, [1], nothing", p.err.String(), p.exits, p.out.String(), want)
	}
}

func TestGaduGenerateWritesTheArtifactsTheCheckThenAcceptsAndTheCheckRefusesTheirAbsence(t *testing.T) {
	root := t.TempDir()
	seedRoleSources(t, root)
	env := map[string]string{"OVERLAY_DIR": root}

	if p := agentgenRun(t, env, "--check"); len(p.exits) != 1 || p.exits[0] != 1 || !strings.HasPrefix(p.err.String(), "gadu-generate --check: generated agent artifacts are stale or missing:\n") {
		t.Errorf("check of an empty root: stderr %q, exits %v; want the stale refusal and [1]", p.err.String(), p.exits)
	}

	generated := agentgenRun(t, env)
	if want := "gadu-generate: agents/<role>.md, pi/agents/<role>.md, and codex/agents/<role>.toml written\n"; generated.out.String() != want || len(generated.exits) != 0 || generated.err.Len() != 0 {
		t.Errorf("generate: stdout %q, stderr %q, exits %v; want %q and no exit", generated.out.String(), generated.err.String(), generated.exits, want)
	}

	checked := agentgenRun(t, env, "--check")
	if want := "gadu-generate --check: OK (committed artifacts match generator output)\n"; checked.out.String() != want || len(checked.exits) != 0 || checked.err.Len() != 0 {
		t.Errorf("check after generate: stdout %q, stderr %q, exits %v; want %q and no exit", checked.out.String(), checked.err.String(), checked.exits, want)
	}
}

// The role files are written by the same command, and the check that follows names none of them as
// missing or stale: the one verb keeps all the generated agents in step (S1: Check in CI).
func TestGaduGenerateWritesTheFilesOfTheRolesToo(t *testing.T) {
	root := t.TempDir()
	seedRoleSources(t, root)

	if p := agentgenRun(t, map[string]string{"OVERLAY_DIR": root}); len(p.exits) != 0 {
		t.Fatalf("generate: stderr %q, exits %v", p.err.String(), p.exits)
	}

	for _, path := range []string{"agents/reviewer.md", "pi/agents/reviewer.md", "codex/agents/reviewer.toml", "codex/agents/delivery.toml"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); err != nil {
			t.Errorf("the command did not write %s: %v", path, err)
		}
	}
}
