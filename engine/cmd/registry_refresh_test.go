package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Tests of the command line of 'registry refresh', of the lock it takes, of its words and of what
// it leaves in the registry. What the table says is tested where it is decided
// (engine/skillregistry); the answers of gentle-ai 4.2.0 it follows are in
// registry_refresh_parity_test.go.

// refreshWorld is a project, a home with skills of its own, and the clock of the run.
type refreshWorld struct {
	root    string
	project string
	home    string
	now     time.Time
}

func newRefreshWorld(t *testing.T) *refreshWorld {
	t.Helper()
	root := t.TempDir()
	w := &refreshWorld{
		root:    root,
		project: filepath.Join(root, "proj"),
		home:    filepath.Join(root, "home"),
		now:     time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC),
	}
	for _, dir := range []string{w.project, w.home} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return w
}

func (w *refreshWorld) markProject(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(w.project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func (w *refreshWorld) skill(t *testing.T, base, name, desc string) {
	t.Helper()
	path := filepath.Join(base, name, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	text := "---\nname: " + name + "\ndescription: " + desc + "\n---\n"
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (w *refreshWorld) registry() string {
	return filepath.Join(w.project, ".atl", "skill-registry.md")
}

func (w *refreshWorld) read(t *testing.T) string {
	t.Helper()
	got, err := os.ReadFile(w.registry())
	if err != nil {
		t.Fatal(err)
	}
	return string(got)
}

func (w *refreshWorld) deps() deps {
	return deps{
		userHomeDir: func() (string, error) { return w.home, nil },
		getwd:       func() (string, error) { return w.root, nil },
		now:         func() time.Time { return w.now },
	}
}

func (w *refreshWorld) run(args ...string) *capturedProcess {
	p := newCapturedProcess()
	runRegistry(p.process, w.deps(), append([]string{"refresh"}, args...))
	return p
}

func TestRegistryRefreshWritesTheTableAndSaysHowManySkills(t *testing.T) {
	w := newRefreshWorld(t)
	w.markProject(t)
	w.skill(t, filepath.Join(w.project, "skills"), "alpha", "first")
	w.skill(t, filepath.Join(w.home, ".claude", "skills"), "beta", "second")

	p := w.run("--cwd", w.project)

	want := "Skill registry refreshed (2 skills): " + w.registry() + "\n"
	if p.out.String() != want || p.err.String() != "" || len(p.exits) != 0 {
		t.Fatalf("run = stdout %q stderr %q exits %v, want %q on stdout", p.out.String(), p.err.String(), p.exits, want)
	}
	text := w.read(t)
	for _, line := range []string{
		"# Skill Registry — proj\n",
		"Last updated: 2026-11-18\n",
		"| `alpha` | first | project | `" + filepath.Join(w.project, "skills", "alpha", "SKILL.md") + "` |\n",
		"| `beta` | second | user | `" + filepath.Join(w.home, ".claude", "skills", "beta", "SKILL.md") + "` |\n",
	} {
		if !strings.Contains(text, line) {
			t.Errorf("registry lacks %q:\n%s", line, text)
		}
	}
	info, err := os.Stat(w.registry())
	if err != nil || info.Mode().Perm() != 0o644 {
		t.Errorf("registry mode = %v, %v, want 0644", info, err)
	}
}

func TestRegistryRefreshSkipsADirectoryThatIsNotAProjectAsGentleDoes(t *testing.T) {
	w := newRefreshWorld(t)
	w.skill(t, filepath.Join(w.project, "skills"), "alpha", "first")

	p := w.run("--cwd", w.project)

	want := "Skill registry refresh skipped (no-project-marker): " + w.project + " is not a project root; run it from a project directory (one containing .git or .atl), or create the project first.\n"
	if p.out.String() != want || p.err.String() != "" || len(p.exits) != 0 {
		t.Fatalf("run = stdout %q stderr %q exits %v, want %q", p.out.String(), p.err.String(), p.exits, want)
	}
	if _, err := os.Stat(filepath.Join(w.project, ".atl")); !os.IsNotExist(err) {
		t.Fatalf("a skipped refresh made .atl: %v", err)
	}
}

func TestRegistryRefreshSkipsADirectoryThatIsNotThere(t *testing.T) {
	w := newRefreshWorld(t)
	missing := filepath.Join(w.root, "nope")

	p := w.run("--cwd", missing)

	if !strings.HasPrefix(p.out.String(), "Skill registry refresh skipped (no-project-marker): "+missing+" is not a project root") || len(p.exits) != 0 {
		t.Fatalf("run = stdout %q exits %v", p.out.String(), p.exits)
	}
}

func TestRegistryRefreshSaysUpToDateAndWritesNothingWhenOnlyTheDateWouldChange(t *testing.T) {
	w := newRefreshWorld(t)
	w.markProject(t)
	w.skill(t, filepath.Join(w.project, "skills"), "alpha", "first")
	w.run("--cwd", w.project)
	before := w.read(t)
	w.now = w.now.Add(72 * time.Hour)

	p := w.run("--cwd", w.project)

	want := "Skill registry up to date (cache-hit): " + w.registry() + "\n"
	if p.out.String() != want || len(p.exits) != 0 {
		t.Fatalf("run = stdout %q exits %v, want %q", p.out.String(), p.exits, want)
	}
	if w.read(t) != before {
		t.Fatalf("the registry changed:\n%s", w.read(t))
	}
}

func TestRegistryRefreshWithForceWritesTheNewDate(t *testing.T) {
	w := newRefreshWorld(t)
	w.markProject(t)
	w.run("--cwd", w.project)
	w.now = w.now.Add(72 * time.Hour)

	p := w.run("--cwd", w.project, "--force")

	if want := "Skill registry refreshed (0 skills): " + w.registry() + "\n"; p.out.String() != want {
		t.Fatalf("stdout = %q, want %q", p.out.String(), want)
	}
	if !strings.Contains(w.read(t), "Last updated: 2026-11-21\n") {
		t.Fatalf("force did not write the date:\n%s", w.read(t))
	}
}

func TestRegistryRefreshWithQuietSaysNothingOfAnyOutcome(t *testing.T) {
	w := newRefreshWorld(t)
	other := newRefreshWorld(t)
	w.markProject(t)

	for name, p := range map[string]*capturedProcess{
		"skipped":    other.run("--cwd", other.project, "--quiet"),
		"refreshed":  w.run("--cwd", w.project, "--quiet"),
		"up to date": w.run("--cwd", w.project, "--quiet"),
	} {
		if p.out.String() != "" || p.err.String() != "" || len(p.exits) != 0 {
			t.Errorf("%s: stdout %q stderr %q exits %v, want silence", name, p.out.String(), p.err.String(), p.exits)
		}
	}
}

func TestRegistryRefreshKeepsEveryLineTheTableDoesNotOwn(t *testing.T) {
	w := newRefreshWorld(t)
	w.markProject(t)
	w.skill(t, filepath.Join(w.project, "skills"), "alpha", "first")
	w.run("--cwd", w.project)
	const extra = "<!-- BEGIN: minimalism-contract-scope (auto-generated) -->\n| minimalism-contract | skills/_shared/minimalism-contract.md | Inject ONLY into shaper |\n<!-- END: minimalism-contract-scope -->\n"
	const hermes = "<!-- labdrian-hermes sha256=01 root=02 -->\n| `hermes-skill` | Hermes | user | `/h/x/SKILL.md` |\n<!-- /labdrian-hermes -->\n"
	text := strings.Replace(w.read(t), "\n## Loading protocol", "\n| `sdd-apply` | Apply | executor | `/e` |\n\n"+hermes+"## Loading protocol", 1) + "\n" + extra
	if err := os.WriteFile(w.registry(), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	w.skill(t, filepath.Join(w.project, "skills"), "gamma", "third")
	w.now = w.now.Add(24 * time.Hour)

	p := w.run("--cwd", w.project)

	if !strings.Contains(p.out.String(), "refreshed (2 skills)") {
		t.Fatalf("stdout = %q", p.out.String())
	}
	got := w.read(t)
	for _, kept := range []string{"| `sdd-apply` | Apply | executor | `/e` |\n\n" + hermes + "## Loading protocol", "\n" + extra} {
		if !strings.Contains(got, kept) {
			t.Errorf("registry lost %q:\n%s", kept, got)
		}
	}
	if !strings.Contains(got, "| `gamma` | third | project |") || !strings.Contains(got, "Last updated: 2026-11-19\n") {
		t.Errorf("registry has no new row or date:\n%s", got)
	}
}

func TestRegistryRefreshResolvesARelativeCwdAgainstTheWorkingDirectory(t *testing.T) {
	w := newRefreshWorld(t)
	w.markProject(t)

	p := w.run("--cwd", "proj")

	if want := "Skill registry refreshed (0 skills): " + w.registry() + "\n"; p.out.String() != want {
		t.Fatalf("stdout = %q, want %q", p.out.String(), want)
	}
}

func TestRegistryRefreshListsTheProjectOnlyWhenThereIsNoHome(t *testing.T) {
	w := newRefreshWorld(t)
	w.markProject(t)
	w.skill(t, filepath.Join(w.home, ".claude", "skills"), "beta", "second")
	d := w.deps()
	d.userHomeDir = nil
	p := newCapturedProcess()

	runRegistry(p.process, d, []string{"refresh", "--cwd", w.project})

	if !strings.Contains(p.out.String(), "refreshed (0 skills)") {
		t.Fatalf("stdout = %q, want the home's skills left out", p.out.String())
	}
}

func TestRegistryRefreshRefusesWhatItIsNotAskedRight(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"no cwd", []string{"--force"}, "error: registry refresh requires --cwd\n"},
		{"an empty cwd", []string{"--cwd", ""}, "error: registry refresh requires --cwd\n"},
		{"a cwd with no value", []string{"--cwd"}, "error: registry refresh: flag \"--cwd\" requires a value\n"},
		{"an unknown flag", []string{"--cwd", "/p", "--frobnicate"}, "error: registry refresh: unknown argument \"--frobnicate\"\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newRefreshWorld(t)
			p := w.run(tc.args...)
			if p.err.String() != tc.want || p.out.String() != "" || !equalInts(p.exits, []int{1}) {
				t.Errorf("run = stdout %q stderr %q exits %v, want %q and exit 1", p.out.String(), p.err.String(), p.exits, tc.want)
			}
		})
	}
}

func TestRegistryVerbErrorsNameTheOneVerb(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"no verb", nil, "error: registry requires a verb: refresh\n"},
		{"another verb", []string{"fix"}, "error: unknown registry verb \"fix\"; expected refresh\n"},
		{"the verb that is gone", []string{"repair-sdd", "--registry", "r", "--target", "codex", "--skills-root", "s"}, "error: unknown registry verb \"repair-sdd\"; expected refresh\n"},
	} {
		p := newCapturedProcess()
		runRegistry(p.process, deps{}, tc.args)
		if p.err.String() != tc.want || p.out.String() != "" || !equalInts(p.exits, []int{1}) {
			t.Errorf("%s: stdout %q stderr %q exits %v, want %q", tc.name, p.out.String(), p.err.String(), p.exits, tc.want)
		}
	}
}

func TestRegistryRefreshTakesTheLockPropagateTakesAndRefusesWhenItStaysTaken(t *testing.T) {
	if testing.Short() {
		t.Skip("waits out the 2 s bound of the registry lock")
	}
	w := newRefreshWorld(t)
	w.markProject(t)
	release := holdLock(t, w.registry())
	defer release()

	p := w.run("--cwd", w.project)

	if !strings.HasPrefix(p.err.String(), "error: acquiring registry lock: ") || !equalInts(p.exits, []int{1}) || p.out.String() != "" {
		t.Fatalf("run = stdout %q stderr %q exits %v, want the lock refusal and exit 1", p.out.String(), p.err.String(), p.exits)
	}
	if _, err := os.Stat(w.registry()); !os.IsNotExist(err) {
		t.Fatalf("a refresh that found the lock taken wrote the registry: %v", err)
	}
}

// The prompt hook runs with --skip-if-busy: a registry whose lock is taken is left for the holder, at
// once and without a word, so a prompt never waits the 2 s bound for a refresh that is only a
// convenience. The registry is not written, and the exit is 0.
func TestRegistryRefreshWithSkipIfBusyLeavesATakenRegistryAloneAtOnce(t *testing.T) {
	w := newRefreshWorld(t)
	w.markProject(t)
	release := holdLock(t, w.registry())
	defer release()

	start := time.Now()
	p := w.run("--cwd", w.project, "--quiet", "--skip-if-busy")

	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("a refresh with --skip-if-busy waited %v for a taken lock", elapsed)
	}
	if p.out.String() != "" || p.err.String() != "" || len(p.exits) != 0 {
		t.Fatalf("run = stdout %q stderr %q exits %v, want silence and exit 0", p.out.String(), p.err.String(), p.exits)
	}
	if _, err := os.Stat(w.registry()); !os.IsNotExist(err) {
		t.Fatalf("a refresh that found the lock taken wrote the registry: %v", err)
	}
}

// --skip-if-busy changes nothing else: a registry that is free is refreshed as always.
func TestRegistryRefreshWithSkipIfBusyRefreshesAFreeRegistry(t *testing.T) {
	w := newRefreshWorld(t)
	w.markProject(t)
	w.skill(t, filepath.Join(w.home, ".claude", "skills"), "alpha", "Alpha. Trigger: a.")

	p := w.run("--cwd", w.project, "--quiet", "--skip-if-busy")

	if p.out.String() != "" || len(p.exits) != 0 || !strings.Contains(w.read(t), "`alpha`") {
		t.Fatalf("run = stdout %q exits %v registry %q, want a silent refresh that lists alpha", p.out.String(), p.exits, w.read(t))
	}
}

// The executor rows lost their writer. A registry that holds the old executor row of sdd-time-estimation,
// a skill the table now lists, ends with one row for it, and a second refresh changes nothing.
func TestRegistryRefreshReplacesTheOldExecutorRowOfASkillItNowLists(t *testing.T) {
	w := newRefreshWorld(t)
	w.markProject(t)
	w.skill(t, filepath.Join(w.project, "skills"), "sdd-time-estimation", "Estimate")
	w.run("--cwd", w.project, "--quiet")
	old := "| `sdd-time-estimation` | Estimate | executor | `/h/.codex/skills/sdd-time-estimation/SKILL.md` |\n"
	other := "| `sdd-apply` | Apply | executor | `/h/.codex/skills/sdd-apply/SKILL.md` |\n"
	text := strings.Replace(w.read(t), "\n## Loading protocol", "\n"+other+old+"\n## Loading protocol", 1)
	if err := os.WriteFile(w.registry(), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}

	p := w.run("--cwd", w.project, "--force")
	got := w.read(t)

	if len(p.exits) != 0 || strings.Count(got, "`sdd-time-estimation`") != 1 || !strings.Contains(got, "| `sdd-time-estimation` | Estimate | project | ") || !strings.Contains(got, other) {
		t.Fatalf("run = stderr %q exits %v registry:\n%s\nwant one project row for sdd-time-estimation and the other executor row kept", p.err.String(), p.exits, got)
	}
	if p2 := w.run("--cwd", w.project); !strings.HasPrefix(p2.out.String(), "Skill registry up to date (cache-hit): ") || w.read(t) != got {
		t.Fatalf("a second refresh = stdout %q, registry changed: %v", p2.out.String(), w.read(t) != got)
	}
}

func TestRegistryRefreshLetsGoOfTheLockWhenItEnds(t *testing.T) {
	w := newRefreshWorld(t)
	w.markProject(t)
	w.run("--cwd", w.project)

	release := holdLock(t, w.registry())
	release()
}

func TestRegistryRefreshFailsWhenTheRegistryCannotBeRead(t *testing.T) {
	w := newRefreshWorld(t)
	w.markProject(t)
	// A directory where the registry should be is a registry that is there and cannot be read.
	if err := os.MkdirAll(w.registry(), 0o755); err != nil {
		t.Fatal(err)
	}

	p := w.run("--cwd", w.project)

	if !strings.HasPrefix(p.err.String(), "error: cannot read the registry "+w.registry()) || !equalInts(p.exits, []int{1}) {
		t.Fatalf("run = stderr %q exits %v", p.err.String(), p.exits)
	}
}

func TestRegistryRefreshRefusesARegistryItDoesNotRecogniseAndLeavesItAsItWas(t *testing.T) {
	w := newRefreshWorld(t)
	w.markProject(t)
	if err := os.MkdirAll(filepath.Dir(w.registry()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(w.registry(), []byte("just the owner's words\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := w.run("--cwd", w.project)

	if !strings.HasPrefix(p.err.String(), "error: registry not recognized: ") || !equalInts(p.exits, []int{1}) || w.read(t) != "just the owner's words\n" {
		t.Fatalf("run = stderr %q exits %v registry %q", p.err.String(), p.exits, w.read(t))
	}
}

// The usage text and the README both say how to call the verb and what it does and prints.
func TestRegistryRefreshIsDocumentedInTheUsageAndTheREADME(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	for name, text := range map[string]string{
		"README":  docSection(t, string(readme), "gentle-ai-overlay registry refresh", "gentle-ai-overlay installed-targets record"),
		"usage()": docSection(t, captureUsage(t), "  engine registry refresh", "  engine skills <verb>"),
	} {
		for _, want := range []string{"--cwd <dir>", "[--force]", "[--quiet]", "<registry>.lock", "propagate", "0644", "Last updated", "cache-hit", "no-project-marker", "refreshed (<n> skills)", "gentle-ai skill-registry refresh", "whole"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s of registry refresh does not say %q", name, want)
			}
		}
	}
}
