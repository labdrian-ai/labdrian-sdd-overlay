package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// verbRun is what one call of a skills verb left: both streams and every exit it asked for.
type verbRun struct {
	stdout, stderr string
	exits          []int
}

func (r verbRun) code() int {
	if len(r.exits) == 0 {
		return 0
	}
	return r.exits[0]
}

// runSkillsVerb calls a verb of the adapter with deps and args and records what it did.
func runSkillsVerb(verb func(skills.Deps, []string, io.Writer, io.Writer, func(int)), deps skills.Deps, args ...string) verbRun {
	var out, errOut bytes.Buffer
	var r verbRun
	verb(deps, args, &out, &errOut, func(c int) { r.exits = append(r.exits, c) })
	r.stdout, r.stderr = out.String(), errOut.String()
	return r
}

// skillsTestDeps wires the real registry reader and the real file reader, over files the test made
// in its own directory.
func skillsTestDeps() skills.Deps {
	return skills.Deps{Registries: newRegistryRepository(), ReadFile: os.ReadFile}
}

const cliRegistry = `version: "1"
skills:
  - id: zeta
    path: zeta
    source:
      type: custom
    install:
      defaultScope: global
      targets:
        - claude
    lifecycle:
      updateStrategy: overlay-only
  - id: alpha
    path: alpha
    source:
      type: core
      upstream:
        owner: someone
    install:
      defaultScope: global
      targets:
        - claude
        - pi
    lifecycle:
      updateStrategy: vendor-merge
`

func writeCLIFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSkillsListPrintsOneSortedLineForEachEntry(t *testing.T) {
	dir := t.TempDir()
	reg := writeCLIFile(t, dir, "r.yaml", cliRegistry)
	got := runSkillsVerb(skillsList, skillsTestDeps(), "list", "--registry", reg)
	want := "alpha\tcore\tvendor-merge\tclaude,pi\nzeta\tcustom\toverlay-only\tclaude\n"
	if got.stdout != want || got.stderr != "" || len(got.exits) != 0 {
		t.Errorf("list = %+v, want stdout %q and nothing else", got, want)
	}
}

func TestSkillsStatusCountsTheEntries(t *testing.T) {
	dir := t.TempDir()
	reg := writeCLIFile(t, dir, "r.yaml", cliRegistry)
	got := runSkillsVerb(skillsStatus, skillsTestDeps(), "status", "--registry", reg, "--manifest", "m", "--source-root", "s")
	want := "Total:  2\nCore:   1\nCustom: 1\nStatus: OK\n"
	if got.stdout != want || got.stderr != "" || len(got.exits) != 0 {
		t.Errorf("status = %+v, want stdout %q and nothing else", got, want)
	}
}

func TestSkillsRegistryVerbsRefuseWhatTheyCannotUse(t *testing.T) {
	dir := t.TempDir()
	absent := filepath.Join(dir, "absent.yaml")
	broken := writeCLIFile(t, dir, "broken.yaml", "version: \"1\"\nskills:\n  - id: Bad Id\n")
	for _, verb := range []struct {
		name string
		fn   func(skills.Deps, []string, io.Writer, io.Writer, func(int))
	}{{"list", skillsList}, {"status", skillsStatus}} {
		t.Run(verb.name+" of a registry that is not there", func(t *testing.T) {
			got := runSkillsVerb(verb.fn, skillsTestDeps(), verb.name, "--registry", absent)
			if got.code() != 1 || got.stdout != "" || !strings.HasPrefix(got.stderr, "error: reading registry "+`"`+absent+`": `) {
				t.Errorf("%s = %+v, want exit 1 and the words of a store that cannot be read", verb.name, got)
			}
		})
		t.Run(verb.name+" of a registry that is not usable", func(t *testing.T) {
			got := runSkillsVerb(verb.fn, skillsTestDeps(), verb.name, "--registry", broken)
			if got.code() != 1 || got.stdout != "" || !strings.HasPrefix(got.stderr, "error: parsing registry: ") {
				t.Errorf("%s = %+v, want exit 1 and error: parsing registry:", verb.name, got)
			}
		})
		t.Run(verb.name+" refuses a flag it does not know and reads nothing", func(t *testing.T) {
			got := runSkillsVerb(verb.fn, skillsTestDeps(), verb.name, "--frobnicate")
			want := "error: skills " + verb.name + ": unknown flag \"--frobnicate\"\n"
			if got.code() != 1 || got.stdout != "" || got.stderr != want {
				t.Errorf("%s = %+v, want exit 1 and %q", verb.name, got, want)
			}
		})
	}
}

func TestSkillsRegistryVerbsWarnOfWhatTheReaderLeftOut(t *testing.T) {
	dir := t.TempDir()
	reg := writeCLIFile(t, dir, "r.yaml", strings.Replace(cliRegistry, "skills:\n", "skills:\n", 1)+"futureField: 1\n")
	for name, fn := range map[string]func(skills.Deps, []string, io.Writer, io.Writer, func(int)){"list": skillsList, "status": skillsStatus} {
		got := runSkillsVerb(fn, skillsTestDeps(), name, "--registry", reg)
		if got.code() != 0 || got.stdout == "" || !strings.Contains(got.stderr, "futureField") {
			t.Errorf("%s = %+v, want the output and a warning on stderr that names the field", name, got)
		}
	}
}

func TestSkillsLintTellsWhatItFound(t *testing.T) {
	dir := t.TempDir()
	good := writeCLIFile(t, dir, "good.md", skillFile("good"))
	bad := writeCLIFile(t, dir, "bad.md", strings.TrimPrefix(skillFile("bad"), "---\n"))
	deps := skillsTestDeps()

	if got := runSkillsVerb(skillsLint, deps, good); got.code() != 0 || got.stdout != "" || got.stderr != "" || len(got.exits) != 1 {
		t.Errorf("lint of a clean skill = %+v, want exit 0 and no output", got)
	}
	if got := runSkillsVerb(skillsLint, deps, bad); got.code() != 1 || got.stdout != "" || !strings.Contains(got.stderr, "[lint:") {
		t.Errorf("lint of a bad skill = %+v, want exit 1 and the finding on stderr", got)
	}
	if got := runSkillsVerb(skillsLint, deps, "--rules", good); got.code() != 0 || got.stdout != skills.RenderLintRules() {
		t.Errorf("lint --rules = %+v, want the rule table and exit 0", got)
	}
	absent := filepath.Join(dir, "absent.md")
	if got := runSkillsVerb(skillsLint, deps, absent); got.code() != 1 || !strings.HasPrefix(got.stderr, "error: reading "+`"`+absent+`": `) {
		t.Errorf("lint of a file that is not there = %+v", got)
	}
	if got := runSkillsVerb(skillsLint, deps); got.code() != 1 || got.stderr != "error: skills lint requires a path or --rules\n" {
		t.Errorf("lint with no path = %+v", got)
	}
	if got := runSkillsVerb(skillsLint, deps, ""); got.code() != 1 || got.stderr != "error: skills lint requires a path or --rules\n" {
		t.Errorf("lint of the empty path = %+v", got)
	}
}

func TestSkillsLintRefusesWhatItDoesNotKnow(t *testing.T) {
	deps := skillsTestDeps()
	if got := runSkillsVerb(skillsLint, deps, "--frobnicate", "a.md"); got.code() != 1 || got.stderr != "error: skills lint: unknown flag \"--frobnicate\"\n" {
		t.Errorf("lint with an unknown flag = %+v", got)
	}
	if got := runSkillsVerb(skillsLint, deps, "a.md", "b.md"); got.code() != 1 || got.stderr != "error: skills lint: unexpected extra argument \"b.md\" (lint accepts exactly one path)\n" {
		t.Errorf("lint with two paths = %+v", got)
	}
}

func TestSkillsLintReportsWarningsOnStdoutAndStillPasses(t *testing.T) {
	dir := t.TempDir()
	// A description between the recommended bound (160 runes) and the hard one (250) warns and
	// does not block.
	long := strings.Replace(skillFile("warn"), "A concise procedural skill for warn.", "A procedural skill for warn."+strings.Repeat(" word", 30), 1)
	path := writeCLIFile(t, dir, "warn.md", long)
	got := runSkillsVerb(skillsLint, skillsTestDeps(), path)
	hard, warnings := skills.LintSkillFile([]byte(long))
	if len(hard) != 0 || len(warnings) == 0 {
		t.Fatalf("the fixture must produce a warning and no hard finding: hard=%v warnings=%v", hard, warnings)
	}
	if got.code() != 0 || !strings.HasPrefix(got.stdout, "[lint:"+warnings[0].Rule+"] ") || got.stderr != "" {
		t.Errorf("lint = %+v, want the warning on stdout, nothing on stderr, exit 0", got)
	}
}
