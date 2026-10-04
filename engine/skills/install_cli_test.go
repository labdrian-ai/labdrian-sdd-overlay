package skills

// `skills install` end to end through RenderInstallCore, over a real overlay and a
// real project in temporary directories: what it prints, what it exits with, and what
// it leaves in the project. The ownership rules themselves are in install_plan_test.go.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// cliInstall is an overlay with one project-scoped skill and an empty project.
type cliInstall struct {
	overlay, project string
	registry         string
}

func newCLIInstall(t *testing.T, skillFiles map[string]string) *cliInstall {
	t.Helper()
	c := &cliInstall{overlay: t.TempDir(), project: t.TempDir(), registry: makeInstallRegistryYAML("my-skill", []string{"target-repo"})}
	c.setSource(t, skillFiles)
	return c
}

func (c *cliInstall) setSource(t *testing.T, files map[string]string) {
	t.Helper()
	if err := os.RemoveAll(filepath.Join(c.overlay, "my-skill")); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		writeTestFile(t, filepath.Join(c.overlay, "my-skill", filepath.FromSlash(name)), content)
	}
}

func (c *cliInstall) run(extra ...string) (stdout, stderr string, code int) {
	var out, errBuf bytes.Buffer
	code = -1
	args := append([]string{"--registry", "reg.yaml", "--source-root", c.overlay, "--project-id", "target-repo"}, extra...)
	RenderInstallCore(args, testRegistries(func(string) ([]byte, error) { return []byte(c.registry), nil }),
		func() (string, error) { return c.project, nil }, &out, &errBuf, func(n int) { code = n })
	return out.String(), errBuf.String(), code
}

func (c *cliInstall) snapshot(t *testing.T) map[string]string { return snapshotTree(t, c.project) }

func TestInstallCLI_CopiesTheSkillIntoBothRuntimesAndSaysSo(t *testing.T) {
	c := newCLIInstall(t, map[string]string{"SKILL.md": "# My Skill"})

	out, errOut, code := c.run()

	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if !strings.Contains(out, "installed: my-skill") {
		t.Errorf("stdout %q does not say 'installed: my-skill'", out)
	}
	for _, runtime := range []string{".claude", ".agents"} {
		data, err := os.ReadFile(filepath.Join(c.project, runtime, "skills", "my-skill", "SKILL.md"))
		if err != nil || string(data) != "# My Skill" {
			t.Errorf("%s copy = %q (err %v)", runtime, data, err)
		}
	}
}

func TestInstallCLI_ASecondRunChangesNothingAndSaysSo(t *testing.T) {
	c := newCLIInstall(t, map[string]string{"SKILL.md": "# Canonical Content"})
	if _, errOut, code := c.run(); code != 0 {
		t.Fatalf("first run: exit %d, stderr %q", code, errOut)
	}
	before := c.snapshot(t)

	out, errOut, code := c.run()

	if code != 0 || !strings.Contains(out, "unchanged: my-skill") || strings.Contains(out, "installed:") {
		t.Errorf("second run: exit %d, stdout %q, stderr %q, want exit 0 and 'unchanged: my-skill'", code, out, errOut)
	}
	assertSameTree(t, before, c.snapshot(t))
}

func TestInstallCLI_ASourceThatMovedOnIsUpdated(t *testing.T) {
	c := newCLIInstall(t, map[string]string{"SKILL.md": "v1"})
	c.run()
	c.setSource(t, map[string]string{"SKILL.md": "v2"})

	out, errOut, code := c.run()

	if code != 0 || !strings.Contains(out, "updated: my-skill") {
		t.Fatalf("exit %d, stdout %q, stderr %q, want 'updated: my-skill'", code, out, errOut)
	}
	if data, _ := os.ReadFile(filepath.Join(c.project, ".agents", "skills", "my-skill", "SKILL.md")); string(data) != "v2" {
		t.Errorf("the .agents copy = %q, want v2", data)
	}
}

// The old install removed the destination before copying, which destroyed anything
// a person had put there. The same situation now stops, with the path.
func TestInstallCLI_ARunOverAHandEditedFileIsRefusedAndNothingIsWritten(t *testing.T) {
	c := newCLIInstall(t, map[string]string{"SKILL.md": "# Canonical Content"})
	c.run()
	edited := filepath.Join(c.project, ".claude", "skills", "my-skill", "SKILL.md")
	if err := os.WriteFile(edited, []byte("# Stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	c.setSource(t, map[string]string{"SKILL.md": "# A newer canonical content"})
	before := c.snapshot(t)

	out, errOut, code := c.run()

	if code != 1 || out != "" {
		t.Fatalf("exit %d, stdout %q, want exit 1 and nothing on stdout", code, out)
	}
	for _, want := range []string{".claude/skills/my-skill/SKILL.md", "edited", "nothing was installed"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr %q does not contain %q", errOut, want)
		}
	}
	assertSameTree(t, before, c.snapshot(t))
}

func TestInstallCLI_ADirectoryThatWasNotInstalledIsRefusedWithAWayForward(t *testing.T) {
	c := newCLIInstall(t, map[string]string{"SKILL.md": "# My Skill"})
	writeTestFile(t, filepath.Join(c.project, ".claude", "skills", "my-skill", "SKILL.md"), "someone else's\n")
	before := c.snapshot(t)

	out, errOut, code := c.run()

	if code != 1 || out != "" {
		t.Fatalf("exit %d, stdout %q, want exit 1 and nothing on stdout", code, out)
	}
	for _, want := range []string{".claude/skills/my-skill", "skills adopt", "--project-id target-repo"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr %q does not contain %q", errOut, want)
		}
	}
	assertSameTree(t, before, c.snapshot(t))
}

func TestInstallCLI_AnUnreadableProjectLockStopsTheInstallAndIsNotReplaced(t *testing.T) {
	c := newCLIInstall(t, map[string]string{"SKILL.md": "# My Skill"})
	writeTestFile(t, filepath.Join(c.project, filepath.FromSlash(ProjectLockRelPath)), "{ not json")
	before := c.snapshot(t)

	_, errOut, code := c.run()

	if code != 1 || !strings.Contains(errOut, ProjectLockRelPath) {
		t.Errorf("exit %d, stderr %q, want exit 1 naming the lock", code, errOut)
	}
	assertSameTree(t, before, c.snapshot(t))
}

func TestInstallCLI_AMissingSourceDirectoryIsReportedForEverySkill(t *testing.T) {
	c := newCLIInstall(t, map[string]string{"SKILL.md": "x"})
	c.registry = `version: "1"
skills:
  - id: ghost-a
    path: ghost-a
    source:
      type: custom
    install:
      defaultScope: project
      targets:
        - claude
      allowedProjects:
        - target-repo
    lifecycle:
      updateStrategy: overlay-only
  - id: ghost-b
    path: ghost-b
    source:
      type: custom
    install:
      defaultScope: project
      targets:
        - claude
      allowedProjects:
        - target-repo
    lifecycle:
      updateStrategy: overlay-only
`

	out, errOut, code := c.run()

	if code != 1 || out != "" {
		t.Fatalf("exit %d, stdout %q, want exit 1", code, out)
	}
	for _, want := range []string{"ghost-a", "ghost-b", filepath.Join(c.overlay, "ghost-a"), "2 source director"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr %q does not contain %q", errOut, want)
		}
	}
	if entries, _ := os.ReadDir(c.project); len(entries) != 0 {
		t.Errorf("a failed install wrote %d entries into the project", len(entries))
	}
}

// A write that fails part way leaves the project as it was and says so.
func TestInstallCLI_AWriteThatFailsLeavesTheProjectAsItWas(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: directory permissions are not enforced")
	}
	c := newCLIInstall(t, map[string]string{"SKILL.md": "content"})
	skillsDir := filepath.Join(c.project, ".claude", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(skillsDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(skillsDir, 0o755) })
	before := c.snapshot(t)

	out, errOut, code := c.run()

	if code != 1 || out != "" || errOut == "" {
		t.Fatalf("exit %d, stdout %q, stderr %q, want exit 1 with a message", code, out, errOut)
	}
	assertSameTree(t, before, c.snapshot(t))
}

// install writes the skill into the two runtime directories and records it in the
// project lock, and into nothing else.
func TestInstallCLI_WritesOnlyTheRuntimeDirectoriesAndTheLock(t *testing.T) {
	c := newCLIInstall(t, map[string]string{"SKILL.md": "# My Skill", "references/guide.md": "g"})
	if _, errOut, code := c.run(); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}

	allowed := []string{
		filepath.Join(c.project, ".claude", "skills", "my-skill") + string(os.PathSeparator),
		filepath.Join(c.project, ".agents", "skills", "my-skill") + string(os.PathSeparator),
		filepath.Join(c.project, filepath.FromSlash(ProjectLockRelPath)),
	}
	for path := range c.snapshot(t) {
		abs := filepath.Join(c.project, filepath.FromSlash(strings.TrimSuffix(path, "/")))
		if path == "" || strings.HasSuffix(path, "/") { // a directory: its files are what count
			continue
		}
		ok := false
		for _, prefix := range allowed {
			if abs == prefix || strings.HasPrefix(abs, prefix) {
				ok = true
			}
		}
		if !ok {
			t.Errorf("install wrote %s, which is neither a runtime copy of the skill nor the lock", path)
		}
	}
}

func TestInstallCLI_ACurrentDirectoryThatCannotBeResolvedWritesNothing(t *testing.T) {
	c := newCLIInstall(t, map[string]string{"SKILL.md": "x"})
	var out, errBuf bytes.Buffer
	code := -1
	RenderInstallCore([]string{"--registry", "reg.yaml", "--source-root", c.overlay},
		testRegistries(func(string) ([]byte, error) { return []byte(c.registry), nil }),
		failCwdFn(), &out, &errBuf, func(n int) { code = n })
	if code != 1 || errBuf.Len() == 0 {
		t.Errorf("exit %d, stderr %q, want exit 1 with a reason", code, errBuf.String())
	}
}

// The procedural verbs share the lock with install and must carry its records along.
func TestInstallCLI_ProjectRegisterKeepsTheInstallRecords(t *testing.T) {
	c := newCLIInstall(t, map[string]string{"SKILL.md": "# My Skill"})
	if _, errOut, code := c.run(); code != 0 {
		t.Fatalf("install: exit %d, stderr %q", code, errOut)
	}
	regPath := filepath.Join(t.TempDir(), "skills.registry.yaml")
	writeTestFile(t, regPath, projectCLIRegistry)
	draft := filepath.Join(t.TempDir(), "SKILL.md")
	writeTestFile(t, draft, string(validDraft("tidy-worktree")))

	_, errOut, code := runProjectRegister(t, []string{"--project-root", c.project, "--candidate", testCandidateKey, "--registry", regPath, draft})
	if code != 0 {
		t.Fatalf("project-register: exit %d, stderr %q", code, errOut)
	}

	data, err := os.ReadFile(filepath.Join(c.project, filepath.FromSlash(ProjectLockRelPath)))
	if err != nil {
		t.Fatal(err)
	}
	lock, err := ParseProjectLock(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(lock.Skills) != 1 || lock.Skills[0].ID != "tidy-worktree" || len(lock.Installs) != 1 || lock.Installs[0].ID != "my-skill" {
		t.Errorf("lock = %+v, want the procedural entry and the install record", lock)
	}
	if out, errOut, code := c.run(); code != 0 || !strings.Contains(out, "unchanged: my-skill") {
		t.Errorf("install after project-register: exit %d, stdout %q, stderr %q, want unchanged", code, out, errOut)
	}
}
