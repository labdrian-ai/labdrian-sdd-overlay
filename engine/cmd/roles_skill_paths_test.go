package main

// roles skill-paths (labdrian-orchestrator-opencode T43 commit a, S8 L117 b): the verb a runtime whose roles
// cannot use the skill tool calls on a launch prompt. It reads the prompt on stdin and prints it with each
// bare skill id under '## Skills to load before work' made the absolute path of its SKILL.md under
// --skills-root. A refusal is exit 2 and one stderr line, nothing on stdout. Every test goes through the
// command; the rules of the rewrite itself are pinned in engine/gate.

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const skillPathsHeader = "## Skills to load before work"

// runSkillPaths runs 'roles skill-paths' with args and prompt on stdin.
func runSkillPaths(prompt string, args ...string) (stdout, stderr string, exits []int) {
	var out, errBuf bytes.Buffer
	runRolesCore(append([]string{"skill-paths"}, args...), strings.NewReader(prompt), &out, &errBuf, func(n int) { exits = append(exits, n) }, nil)
	return out.String(), errBuf.String(), exits
}

// skillPathsRoot is a skills root holding test-first with its SKILL.md.
func skillPathsRoot(t *testing.T) (root, skillFile string) {
	t.Helper()
	root = t.TempDir()
	skillFile = filepath.Join(root, "test-first", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillFile, []byte("# test-first\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, skillFile
}

func TestRolesSkillPathsPrintsThePromptWithTheIdsAsPaths(t *testing.T) {
	root, skillFile := skillPathsRoot(t)
	prompt := "Build T1.\n" + skillPathsHeader + "\ntest-first\n\n## Notes\ntest-first\n"

	stdout, stderr, exits := runSkillPaths(prompt, "--skills-root", root)

	want := "Build T1.\n" + skillPathsHeader + "\n" + skillFile + "\n\n## Notes\ntest-first\n"
	if stdout != want || stderr != "" || !reflect.DeepEqual(exits, []int{0}) {
		t.Fatalf("skill-paths printed %q / %q, exits %v; want %q, nothing on stderr, exit 0", stdout, stderr, exits, want)
	}
}

func TestRolesSkillPathsPrintsAPromptWithNoHeaderAsItCame(t *testing.T) {
	root, _ := skillPathsRoot(t)
	for name, prompt := range map[string]string{"text": "Build T1.\r\n\ttest-first  \nno final newline", "empty": ""} {
		t.Run(name, func(t *testing.T) {
			stdout, stderr, exits := runSkillPaths(prompt, "--skills-root", root)
			if stdout != prompt || stderr != "" || !reflect.DeepEqual(exits, []int{0}) {
				t.Fatalf("skill-paths printed %q / %q, exits %v; want the prompt byte for byte and exit 0", stdout, stderr, exits)
			}
		})
	}
}

func TestRolesSkillPathsRefusesAsExit2WithOneLineAndNothingOnStdout(t *testing.T) {
	root, _ := skillPathsRoot(t)
	for name, c := range map[string]struct{ prompt, line string }{
		"the colon form": {"x\n" + skillPathsHeader + ": test-first, go-test\n",
			`error: roles skill-paths: line 2 "## Skills to load before work: test-first, go-test" is not the header alone: write "## Skills to load before work" alone on its line, then one skill id or absolute SKILL.md path per line` + "\n"},
		"an unknown id": {skillPathsHeader + "\nno-such\n",
			`error: roles skill-paths: line 2 "no-such" names no skill: write a skill id of ` + root + ` (a directory with a SKILL.md) or the absolute path of its SKILL.md` + "\n"},
	} {
		t.Run(name, func(t *testing.T) {
			stdout, stderr, exits := runSkillPaths(c.prompt, "--skills-root", root)
			if stdout != "" || stderr != c.line || !reflect.DeepEqual(exits, []int{2}) {
				t.Fatalf("skill-paths printed %q / %q, exits %v; want nothing, %q and exit 2", stdout, stderr, exits, c.line)
			}
		})
	}
}

func TestRolesSkillPathsRefusesItsUsageAsExit1(t *testing.T) {
	root, _ := skillPathsRoot(t)
	for name, c := range map[string]struct {
		args []string
		want string
	}{
		"no root":          {nil, "error: roles skill-paths: --skills-root is required\n"},
		"relative root":    {[]string{"--skills-root", "skills"}, "error: roles skill-paths: --skills-root must be an absolute path, got \"skills\"\n"},
		"root not a dir":   {[]string{"--skills-root", filepath.Join(root, "absent")}, "error: roles skill-paths: --skills-root " + filepath.Join(root, "absent") + " is not a directory\n"},
		"an unknown flag":  {[]string{"--skills-root", root, "--cwd", "/x"}, "error: roles skill-paths: unknown flag \"--cwd\"\n"},
		"a root twice":     {[]string{"--skills-root", root, "--skills-root", root}, "error: roles skill-paths: --skills-root given twice\n"},
		"a positional arg": {[]string{"--skills-root", root, "test-first"}, "error: roles skill-paths: unexpected argument \"test-first\"\n"},
	} {
		t.Run(name, func(t *testing.T) {
			stdout, stderr, exits := runSkillPaths(skillPathsHeader+"\ntest-first\n", c.args...)
			if stdout != "" || stderr != c.want || !reflect.DeepEqual(exits, []int{1}) {
				t.Fatalf("skill-paths printed %q / %q, exits %v; want nothing, %q and exit 1", stdout, stderr, exits, c.want)
			}
		})
	}
}

// The other roles verbs refuse --skills-root as they refused any flag of another verb before.
func TestRolesOtherVerbsStillRefuseSkillsRoot(t *testing.T) {
	var out, errBuf bytes.Buffer
	var exits []int
	runRolesCore([]string{"match-shaper", "--skills-root", "/x"}, strings.NewReader(""), &out, &errBuf, func(n int) { exits = append(exits, n) }, nil)
	if want := "error: roles match-shaper: unknown flag \"--skills-root\"\n"; errBuf.String() != want || out.String() != "" || !reflect.DeepEqual(exits, []int{1}) {
		t.Fatalf("match-shaper printed %q / %q, exits %v; want %q and exit 1", out.String(), errBuf.String(), exits, want)
	}
}
