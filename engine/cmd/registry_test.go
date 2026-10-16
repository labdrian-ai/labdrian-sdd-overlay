package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/filelock"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sddregistry"
	sddfs "github.com/labdrian-ai/labdrian-sdd-overlay/engine/sddregistry/fsstore"
)

// Tests of the command line of 'registry repair-sdd', of the lock it takes and of what it says.
// What the repair writes is tested where it is decided (engine/sddregistry); here is what the
// command takes, how it takes turns with `propagate`, and its words.

// registryRun is one run of the command: what it wrote on each stream and the exits it asked for.
type registryRun struct {
	stdout, stderr string
	exits          []int
}

func runRegistryWith(args ...string) registryRun {
	var stdout, stderr bytes.Buffer
	var exits []int
	runRegistryCore(sddfs.Registry{}, sddfs.Skills{}, args, &stdout, &stderr, func(code int) { exits = append(exits, code) })
	return registryRun{stdout.String(), stderr.String(), exits}
}

// repairWorld is a project with a registry at registry (or none), and the skills a target keeps.
type repairWorld struct {
	registry   string
	skillsRoot string
}

func newRepairWorld(t *testing.T) repairWorld {
	t.Helper()
	root := t.TempDir()
	return repairWorld{registry: filepath.Join(root, "proj", ".atl", "skill-registry.md"), skillsRoot: filepath.Join(root, "skills")}
}

func (w repairWorld) args() []string {
	return []string{"repair-sdd", "--registry", w.registry, "--target", "codex", "--skills-root", w.skillsRoot}
}

func (w repairWorld) install(t *testing.T, name, text string) {
	t.Helper()
	dir := filepath.Join(w.skillsRoot, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (w repairWorld) writeRegistry(t *testing.T, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(w.registry), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(w.registry, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (w repairWorld) readRegistry(t *testing.T) string {
	t.Helper()
	got, err := os.ReadFile(w.registry)
	if err != nil {
		t.Fatal(err)
	}
	return string(got)
}

func TestRegistryRepairSDDWritesTheRowsOfTheInstalledExecutorsAndSumsUp(t *testing.T) {
	w := newRepairWorld(t)
	w.install(t, "sdd-apply", "description: \"Apply | the change\"\n")
	w.install(t, "sdd-verify", "# no description\n")
	w.writeRegistry(t, "# r\n## Skills\n| `go` |\n## Loading protocol\n")

	run := runRegistryWith(w.args()...)

	wantRegistry := "# r\n" + sddregistry.Note + "\n\n## Skills\n| `go` |\n" +
		"| `sdd-apply` | Apply \\| the change | executor | `" + w.skillsRoot + "/sdd-apply/SKILL.md` |\n" +
		"| `sdd-verify` | SDD phase executor: sdd-verify. | executor | `" + w.skillsRoot + "/sdd-verify/SKILL.md` |\n" +
		"\n## Loading protocol\n"
	if got := w.readRegistry(t); got != wantRegistry {
		t.Errorf("the registry =\n%q\nwant\n%q", got, wantRegistry)
	}
	if want := "repair-sdd-registry: registry=" + w.registry + " target=codex sdd_executors=2 missing=9\n"; run.stdout != want {
		t.Errorf("stdout = %q, want %q", run.stdout, want)
	}
	if len(run.exits) != 0 {
		t.Errorf("the command asked for exits %v on success, want none (the process ends with 0)", run.exits)
	}
}

func TestRegistryRepairSDDSaysEachExecutorThatIsNotInstalledOnStderr(t *testing.T) {
	w := newRepairWorld(t)
	w.install(t, "sdd-apply", "description: Apply\n")

	run := runRegistryWith(w.args()...)

	var want strings.Builder
	for _, name := range sddregistry.Executors()[1:] {
		want.WriteString("WARN: missing codex SDD executor skill: " + w.skillsRoot + "/" + name + "/SKILL.md\n")
	}
	if run.stderr != want.String() {
		t.Errorf("stderr =\n%s\nwant\n%s", run.stderr, want.String())
	}
	if !strings.HasSuffix(run.stdout, " sdd_executors=1 missing=10\n") {
		t.Errorf("stdout = %q, want the sum of 1 row and 10 missing", run.stdout)
	}
}

func TestRegistryRepairSDDMakesTheRegistryAndItsDirectoryForAProjectThatHasNone(t *testing.T) {
	w := newRepairWorld(t)

	run := runRegistryWith(w.args()...)

	if len(run.exits) != 0 {
		t.Fatalf("the command asked for exits %v (stderr %q)", run.exits, run.stderr)
	}
	got, err := sddregistry.Repair(sddregistry.NewRegistry("proj"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if w.readRegistry(t) != got {
		t.Errorf("the registry =\n%q\nwant\n%q", w.readRegistry(t), got)
	}
	info, err := os.Stat(w.registry)
	if err != nil || info.Mode().Perm() != 0o644 {
		t.Errorf("the registry is %v, %v, want a file of mode 644", info, err)
	}
	if _, err := os.Stat(w.registry + ".lock"); err != nil {
		t.Errorf("the lock file is not beside the registry: %v", err)
	}
}

func TestRegistryRepairSDDRefusesARegistryWithoutAHeadingAndLeavesItAsItWas(t *testing.T) {
	for _, tc := range []struct{ name, registry, heading string }{
		{"no skills heading", "# r\n## Loading protocol\n", "## Skills"},
		{"no loading protocol heading", "# r\n## Skills\n", "## Loading protocol"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newRepairWorld(t)
			w.install(t, "sdd-apply", "description: Apply\n")
			w.writeRegistry(t, tc.registry)

			run := runRegistryWith(w.args()...)

			wantErr := "ERROR: registry has no '" + tc.heading + "' section: " + w.registry + "\n"
			if run.stderr != wantErr || run.stdout != "" || !equalInts(run.exits, []int{1}) {
				t.Errorf("run = %+v, want %q on stderr, nothing on stdout and exit 1", run, wantErr)
			}
			if got := w.readRegistry(t); got != tc.registry {
				t.Errorf("the registry became %q, want it as it was", got)
			}
		})
	}
}

func TestRegistryRepairSDDRefusesWhatItIsNotAskedRight(t *testing.T) {
	good := []string{"--registry", "/r", "--target", "codex", "--skills-root", "/s"}
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"no verb", nil, "error: registry requires a verb: repair-sdd\n"},
		{"another verb", []string{"fix"}, "error: unknown registry verb \"fix\"; expected repair-sdd\n"},
		{"an unknown flag", append([]string{"repair-sdd"}, append(good, "--frobnicate")...), "error: registry repair-sdd: unknown argument \"--frobnicate\"\n"},
		{"a flag with no value", []string{"repair-sdd", "--registry"}, "error: registry repair-sdd: flag \"--registry\" requires a value\n"},
		{"no registry", []string{"repair-sdd", "--target", "codex", "--skills-root", "/s"}, "error: registry repair-sdd requires --registry, --target, and --skills-root\n"},
		{"no target", []string{"repair-sdd", "--registry", "/r", "--skills-root", "/s"}, "error: registry repair-sdd requires --registry, --target, and --skills-root\n"},
		{"no skills root", []string{"repair-sdd", "--registry", "/r", "--target", "codex"}, "error: registry repair-sdd requires --registry, --target, and --skills-root\n"},
		{"an empty value", []string{"repair-sdd", "--registry", "", "--target", "codex", "--skills-root", "/s"}, "error: registry repair-sdd requires --registry, --target, and --skills-root\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := runRegistryWith(tc.args...)
			if run.stderr != tc.want || run.stdout != "" || !equalInts(run.exits, []int{1}) {
				t.Errorf("run = %+v, want %q on stderr, nothing on stdout and exit 1", run, tc.want)
			}
		})
	}
}

func TestRegistryRepairSDDFailsWhenTheDirectoryOfTheRegistryCannotBeMade(t *testing.T) {
	w := newRepairWorld(t)
	blocker := filepath.Join(filepath.Dir(filepath.Dir(w.registry)))
	if err := os.MkdirAll(filepath.Dir(blocker), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blocker, nil, 0o644); err != nil { // "proj" is a file
		t.Fatal(err)
	}

	run := runRegistryWith(w.args()...)

	if !strings.HasPrefix(run.stderr, "ERROR: cannot make the directory of the registry "+w.registry+": ") || !equalInts(run.exits, []int{1}) {
		t.Errorf("run = %+v, want an error that names the registry and exit 1", run)
	}
}

func TestRegistryLockPathIsTheLockPropagateTakes(t *testing.T) {
	for registry, want := range map[string]string{
		"/p/.atl/skill-registry.md":      "/p/.atl/skill-registry.md.lock",
		"/p/x/../.atl/skill-registry.md": "/p/.atl/skill-registry.md.lock",
		"p//.atl/./skill-registry.md":    "p/.atl/skill-registry.md.lock",
	} {
		if got := registryLockPath(registry); got != want {
			t.Errorf("registryLockPath(%q) = %q, want %q", registry, got, want)
		}
		// propagate cleans the path it is given before it names the lock, so both name one file.
		if got := registryPathFromArgs([]string{"--registry", registry}) + ".lock"; got != want {
			t.Errorf("propagate's lock for %q = %q, want %q", registry, got, want)
		}
	}
}

// holdLock takes the lock of the registry the way a writer does and returns what lets it go.
func holdLock(t *testing.T, registry string) (release func()) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(registry), 0o755); err != nil {
		t.Fatal(err)
	}
	release, err := filelock.Acquire(registryLockPath(registry), filelock.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return release
}

func TestRegistryRepairSDDWaitsForTheLockAndThenRepairs(t *testing.T) {
	w := newRepairWorld(t)
	w.install(t, "sdd-apply", "description: Apply\n")
	w.writeRegistry(t, "## Skills\n## Loading protocol\n")
	release := holdLock(t, w.registry)

	var wg sync.WaitGroup
	var run registryRun
	wg.Add(1)
	go func() {
		defer wg.Done()
		run = runRegistryWith(w.args()...)
	}()

	time.Sleep(200 * time.Millisecond)
	if got := w.readRegistry(t); got != "## Skills\n## Loading protocol\n" {
		release()
		wg.Wait()
		t.Fatalf("the registry was written while another writer held its lock: %q", got)
	}
	release()
	wg.Wait()

	if len(run.exits) != 0 || !strings.Contains(w.readRegistry(t), "| `sdd-apply` |") {
		t.Errorf("after the lock was let go: run %+v, registry %q, want it repaired", run, w.readRegistry(t))
	}
}

func TestRegistryRepairSDDRefusesALockThatStaysTakenAndLeavesTheRegistryAlone(t *testing.T) {
	if testing.Short() {
		t.Skip("waits out the 2 s bound of the registry lock")
	}
	w := newRepairWorld(t)
	w.writeRegistry(t, "## Skills\n## Loading protocol\n")
	release := holdLock(t, w.registry)
	defer release()

	start := time.Now()
	run := runRegistryWith(w.args()...)
	elapsed := time.Since(start)

	if !strings.HasPrefix(run.stderr, "ERROR: acquiring registry lock: ") || !strings.Contains(run.stderr, registryLockPath(w.registry)) || !equalInts(run.exits, []int{1}) {
		t.Errorf("run = %+v, want a refusal that names the lock and exit 1", run)
	}
	if elapsed < filelock.DefaultWait-100*time.Millisecond || elapsed > 5*time.Second {
		t.Errorf("gave up after %v, want about the %v bound", elapsed, filelock.DefaultWait)
	}
	if got := w.readRegistry(t); got != "## Skills\n## Loading protocol\n" {
		t.Errorf("the registry became %q, want it as it was", got)
	}
}

// A repair and a propagation of the same registry take turns: the lock one holds is the lock the
// other waits for.
func TestPropagateWaitsForTheLockARepairHolds(t *testing.T) {
	if testing.Short() {
		t.Skip("waits out the 2 s bound of the registry lock")
	}
	w := newRepairWorld(t)
	w.writeRegistry(t, "## Skills\n## Loading protocol\n")
	release := holdLock(t, w.registry)
	defer release()
	p := newCapturedProcess()

	runPropagate(p.process, []string{"--registry", w.registry, "--embedded-contract", embeddedAntiGenericDesign})

	if !strings.Contains(p.err.String(), "acquiring registry lock") || !equalInts(p.exits, []int{1}) {
		t.Errorf("propagate = stderr %q, exits %v, want it to find the lock taken and refuse", p.err.String(), p.exits)
	}
}

// The usage text and the README both say how to call the verb, what it takes the lock of, what it
// prints and how it ends: a reader of either finds the flags the script passes.
func TestRegistryRepairSDDIsDocumentedInTheUsageAndTheREADME(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	for name, text := range map[string]string{
		"README":  docSection(t, string(readme), "gentle-ai-overlay registry repair-sdd", "gentle-ai-overlay runtime capabilities"),
		"usage()": docSection(t, captureUsage(t), "  engine registry repair-sdd", "  engine skills <verb>"),
	} {
		for _, want := range []string{"--registry <path>", "--target <name>", "--skills-root <dir>", "<registry>.lock", "propagate", "0644", "repair-sdd-registry: registry=", "sdd_executors=", "missing=", "WARN:", "ERROR: registry has no"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s of registry repair-sdd does not say %q", name, want)
			}
		}
	}
}

// The lock is let go when the repair ends, whatever it ended with: the next writer is not made to wait.
func TestRegistryRepairSDDLetsGoOfTheLockWhenItEnds(t *testing.T) {
	for _, tc := range []struct{ name, registry string }{
		{"after a repair", "## Skills\n## Loading protocol\n"},
		{"after a refusal", "# no headings\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newRepairWorld(t)
			w.writeRegistry(t, tc.registry)
			runRegistryWith(w.args()...)

			release, err := filelock.Acquire(registryLockPath(w.registry), filelock.Options{NoWait: true})
			if err != nil {
				t.Fatalf("the lock is still taken when the command has ended: %v", err)
			}
			release()
		})
	}
}

// scriptCallOfRepairSDD is the body of repair_sdd_registry_file, the one function of bin/labdrian-overlay
// that builds the command line of the verb.
func scriptCallOfRepairSDD(t *testing.T) string {
	t.Helper()
	text := overlaytest.ScriptText(t)
	start := strings.Index(text, "\nrepair_sdd_registry_file() {\n")
	if start < 0 {
		t.Fatal("the overlay script has no repair_sdd_registry_file")
	}
	body := text[start:]
	if end := strings.Index(body, "\n}\n"); end >= 0 {
		body = body[:end]
	}
	return body
}

// The script asks the engine to repair the registry, with the flags the command takes and nothing
// else, and edits no markdown itself.
func TestTheScriptAsksTheEngineToRepairTheRegistryWithTheFlagsItTakes(t *testing.T) {
	body := scriptCallOfRepairSDD(t)
	if !strings.Contains(body, `"$ENGINE_BINARY" registry repair-sdd`) {
		t.Fatalf("repair_sdd_registry_file does not call the engine binary it is told of (ENGINE_BINARY) with registry repair-sdd:\n%s", body)
	}
	var flags []string
	for _, word := range strings.Fields(body) {
		if strings.HasPrefix(word, "--") {
			flags = append(flags, word)
		}
	}
	sort.Strings(flags)
	if want := []string{flagRegistry, flagSkillsRoot, flagTarget}; !reflect.DeepEqual(flags, want) {
		t.Fatalf("flags of repair_sdd_registry_file = %v, want %v", flags, want)
	}
	args := []string{}
	for _, flag := range flags {
		args = append(args, flag, "value-of"+flag)
	}
	parsed, err := parseRepairSDDArgs(args)
	if err != nil {
		t.Fatalf("the command refuses the flags the script passes: %v", err)
	}
	if parsed.registry != "value-of--registry" || parsed.target != "value-of--target" || parsed.skillsRoot != "value-of--skills-root" {
		t.Errorf("the command took the flags of the script as %+v: each flag must reach its own field", parsed)
	}
	for _, gone := range []string{"awk", "grep", "mktemp", "mv "} {
		if strings.Contains(body, gone) {
			t.Errorf("repair_sdd_registry_file still uses %q: the markdown is the engine's", gone)
		}
	}
}
