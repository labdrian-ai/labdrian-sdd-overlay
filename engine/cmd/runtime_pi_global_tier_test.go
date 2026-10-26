package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/piguard"
)

// The Pi global tier is the part of the overlay that lands in a person's own Pi: the labdrian-pi
// package built under the state directory, `pi install` of it and of the Subagents extension, and
// the links of its agents in ~/.pi/agent/agents, all undone by `pi remove`. Every other test of it
// hands the Pi adapter a fake CommandRunner, so none proves that the program, wired with the
// process adapter, asks the CLI what the adapter says it asks. The tests below do, in a sandbox,
// and they start the real CLI only when the person running them asks for it:
//
//   - by default the `pi` the program finds is a script of the test that records how it was
//     started and succeeds. It stands first on a PATH that goes on with the guard of TestMain, so a
//     lookup that missed the script would reach the guard and fail the run, never the real CLI;
//   - with LABDRIAN_PI_LIVE_TEST=1 (piguard.LiveVariable) the program runs the real `pi` of the
//     machine against a HOME that belongs to the test and that piguard accepted.
//
// The program is run as a process, the engine binary the golden tests build, because no test of
// this package may import the process adapter (TestNoTestOfThisPackageUsesTheProcessAdapter): the
// process adapter is wired in the program, and the program is what is under test. The environment
// it gets is listed in full. Nothing of the test process is inherited: its HOME is the one TestMain
// pinned and its other variables are the person's own.

const (
	// globalTierProbeAgent is a second agent of the fixture overlay, named so that no real Pi has
	// it. A link or a package file of that name found in the person's own Pi or state afterwards is
	// proof that the run reached the machine and not the sandbox.
	globalTierProbeAgent = "zz-global-tier-probe.md"

	// globalTierSubagentsPackage is what `pi install` is asked for the Subagents extension. It is
	// written out here and not read from the adapter because it is the contract with the CLI.
	globalTierSubagentsPackage = "npm:pi-subagents-j0k3r"

	// globalTierRunTimeout stops a run of the program that does not end, so that a stand-in or a
	// real pi that waits for an answer fails the test instead of holding the run.
	globalTierRunTimeout = 2 * time.Minute
)

// recordingPiScript is the `pi` of the default mode: it records how it was started, in a log next
// to itself, and succeeds. A start is a block of lines: its arguments one by one, the HOME and the
// working directory it found. The log is built with printf, whose output has no escapes to read.
const recordingPiScript = `#!/bin/sh
{
	printf 'start\n'
	for arg in "$@"; do printf 'arg=%s\n' "$arg"; done
	printf 'home=%s\n' "$HOME"
	printf 'cwd=%s\n' "$(pwd)"
	printf 'end\n'
} >> "${0%/*}/calls.log"
exit 0
`

// globalTierWorld is the sandbox of one test: every directory the program may write, the overlay
// it reads, and what the overlay looked like before it ran. Everything is under one temporary
// directory, root, which holds nothing else.
type globalTierWorld struct {
	t             *testing.T
	root          string
	home          string // HOME of the program, and of the pi it starts
	state         string // STATE_DIR: where the package is built
	cwd           string // where the program is started, and must leave empty
	tmp           string // TMPDIR
	bin           string // holds the stand-in pi and its log, in the default mode
	overlay       string // OVERLAY_DIR: the checkout the package is built from, which the program only reads
	overlayBefore map[string]string
	pkg           string // where the package Pi loads is built: <state>/pi/labdrian-pi
}

// newGlobalTierWorld makes the sandbox. It refuses, through piguard, a HOME that is the person's
// own, so no run in it can be handed the real Pi: the refusal is made here, once, for the default
// mode and for the live one alike.
func newGlobalTierWorld(t *testing.T) *globalTierWorld {
	t.Helper()
	if goruntime.GOOS == "windows" {
		t.Skip("the stand-in pi is a shell script, and so is the guard")
	}
	root := t.TempDir()
	w := &globalTierWorld{
		t:     t,
		root:  root,
		home:  filepath.Join(root, "home"),
		state: filepath.Join(root, "state"),
		cwd:   filepath.Join(root, "cwd"),
		tmp:   filepath.Join(root, "tmp"),
		bin:   filepath.Join(root, "bin"),
	}
	for _, dir := range []string{w.home, w.state, w.cwd, w.tmp, w.bin} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("make %s: %v", dir, err)
		}
	}
	if err := piguard.RefuseRealHome(w.home); err != nil {
		t.Fatalf("the sandbox may not be the home of a Pi: %v", err)
	}
	w.overlay, _ = pipkgFixtureOverlay(t)
	writeTestFile(t, filepath.Join(w.overlay, "pi", "agents", globalTierProbeAgent),
		"---\nname: global-tier-probe\ndescription: an agent no real Pi has\n---\nbody\n")
	w.overlayBefore = treeDigest(t, w.overlay)
	w.pkg = filepath.Join(w.state, "pi", "labdrian-pi")
	return w
}

// environment is the whole environment of a run: the places the program writes are the sandbox's,
// path is the PATH it looks `pi` up on, and extra are the variables of the one mode that needs
// more. Git is told to read no configuration of the machine, as the golden tests tell it.
func (w *globalTierWorld) environment(path string, extra ...string) []string {
	env := []string{
		"HOME=" + w.home,
		"STATE_DIR=" + w.state,
		"OVERLAY_DIR=" + w.overlay,
		"XDG_CONFIG_HOME=" + filepath.Join(w.home, ".config"),
		"XDG_STATE_HOME=" + filepath.Join(w.home, ".local", "state"),
		"XDG_DATA_HOME=" + filepath.Join(w.home, ".local", "share"),
		"XDG_CACHE_HOME=" + filepath.Join(w.home, ".cache"),
		"TMPDIR=" + w.tmp,
		"PATH=" + path,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
	}
	return append(env, extra...)
}

// globalTierRun is what one run of the program did: its exit code and what it printed.
type globalTierRun struct {
	code           int
	stdout, stderr string
}

// run starts the program with args in the working directory of the sandbox and env, and waits for it.
func (w *globalTierWorld) run(env []string, args ...string) globalTierRun {
	w.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), globalTierRunTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, engineBinary(w.t), args...)
	cmd.Dir = w.cwd
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	run := globalTierRun{}
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			w.t.Fatalf("run the engine %v: %v\nstdout=%q\nstderr=%q", args, err, stdout.String(), stderr.String())
		}
		run.code = exit.ExitCode()
	}
	run.stdout, run.stderr = stdout.String(), stderr.String()
	return run
}

// standInPath is the PATH of the default mode: the directory of the stand-in pi first, then the
// PATH of this run, which has the guard first. The program finds git on it, as it does in the
// golden tests.
func (w *globalTierWorld) standInPath() string {
	return w.bin + string(os.PathListSeparator) + os.Getenv("PATH")
}

// putStandInPi writes the recording pi into the sandbox.
func (w *globalTierWorld) putStandInPi() {
	w.t.Helper()
	if err := os.WriteFile(filepath.Join(w.bin, "pi"), []byte(recordingPiScript), 0o755); err != nil {
		w.t.Fatalf("write the stand-in pi: %v", err)
	}
}

// piStart is one start of the stand-in pi, as the script recorded it.
type piStart struct {
	args []string
	home string
	cwd  string
}

// starts reads what the stand-in pi recorded, in order. A pi that was never started left no log.
func (w *globalTierWorld) starts() []piStart {
	w.t.Helper()
	raw, err := os.ReadFile(filepath.Join(w.bin, "calls.log"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		w.t.Fatalf("read the log of the stand-in pi: %v", err)
	}
	var starts []piStart
	open := false
	for _, line := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		switch {
		case line == "start":
			starts = append(starts, piStart{})
			open = true
		case line == "end":
			open = false
		case !open:
			w.t.Fatalf("the log of the stand-in pi has a line outside a start: %q\n%s", line, raw)
		case strings.HasPrefix(line, "arg="):
			last := &starts[len(starts)-1]
			last.args = append(last.args, strings.TrimPrefix(line, "arg="))
		case strings.HasPrefix(line, "home="):
			starts[len(starts)-1].home = strings.TrimPrefix(line, "home=")
		case strings.HasPrefix(line, "cwd="):
			starts[len(starts)-1].cwd = strings.TrimPrefix(line, "cwd=")
		default:
			w.t.Fatalf("the log of the stand-in pi has a line it cannot read: %q\n%s", line, raw)
		}
	}
	return starts
}

// agentLink is where the program links an agent of the package for Pi to find.
func (w *globalTierWorld) agentLink(file string) string {
	return filepath.Join(w.home, ".pi", "agent", "agents", file)
}

// assertInstalled checks what an install leaves in the sandbox, whichever pi it ran: the package
// Pi loads is built where `pi install` was told, in full, and the agents of the package are linked
// from the home Pi reads them in.
func (w *globalTierWorld) assertInstalled() {
	t := w.t
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(w.pkg, "package.json"))
	if err != nil {
		t.Fatalf("the package Pi loads was not built at %s: %v", w.pkg, err)
	}
	var manifest struct {
		Name string `json:"name"`
		Pi   struct {
			Skills     []string `json:"skills"`
			Agents     []string `json:"agents"`
			Extensions []string `json:"extensions"`
			MCP        string   `json:"mcp"`
		} `json:"pi"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("package.json is not JSON: %v\n%s", err, raw)
	}
	if manifest.Name != "labdrian-pi" {
		t.Errorf("package.json names %q, want labdrian-pi", manifest.Name)
	}
	// What the manifest tells Pi to load must be in the package, and what the overlay holds must
	// have been built into it.
	named := []string{manifest.Pi.MCP}
	named = append(named, manifest.Pi.Skills...)
	named = append(named, manifest.Pi.Agents...)
	named = append(named, manifest.Pi.Extensions...)
	for _, entry := range named {
		if entry == "" {
			t.Errorf("package.json leaves a part of the pi manifest empty: %s", raw)
			continue
		}
		if _, err := os.Stat(filepath.Join(w.pkg, entry)); err != nil {
			t.Errorf("package.json names %s for Pi to load, and it is not in the package: %v", entry, err)
		}
	}
	for _, file := range []string{
		filepath.Join("skills", "pi-skill", "SKILL.md"),
		filepath.Join("agents", "builder.md"),
		filepath.Join("agents", globalTierProbeAgent),
		filepath.Join("extensions", "labdrian-gate.ts"),
	} {
		if _, err := os.Stat(filepath.Join(w.pkg, file)); err != nil {
			t.Errorf("the built package lacks %s: %v", file, err)
		}
	}
	// The build goes through a directory beside the package and swaps it in: none may be left.
	if got := entryNames(t, filepath.Dir(w.pkg)); !slices.Equal(got, []string{"labdrian-pi"}) {
		t.Errorf("%s holds %v, want only the package", filepath.Dir(w.pkg), got)
	}
	for _, file := range []string{"builder.md", globalTierProbeAgent} {
		target, err := os.Readlink(w.agentLink(file))
		if err != nil {
			t.Errorf("the agent %s is not linked for Pi: %v", file, err)
			continue
		}
		if want := filepath.Join(w.pkg, "agents", file); target != want {
			t.Errorf("the link of %s leads to %s, want %s", file, target, want)
		}
	}
}

// assertRemoved checks what a removal leaves: the package gone from where it was built and no link
// of its agents left in the home.
func (w *globalTierWorld) assertRemoved() {
	t := w.t
	t.Helper()
	if _, err := os.Lstat(w.pkg); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the package is still at %s after the removal (%v)", w.pkg, err)
	}
	for _, file := range []string{"builder.md", globalTierProbeAgent} {
		if _, err := os.Lstat(w.agentLink(file)); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("the link of %s is still in the home after the removal (%v)", file, err)
		}
	}
}

// assertNothingEscaped checks that the run wrote nowhere the test did not make for it: the overlay
// it only reads is what it was, it left nothing in the directory it ran in, the sandbox holds only
// the directories the test made, and the machine's own Pi and state hold nothing of the probe.
func (w *globalTierWorld) assertNothingEscaped() {
	t := w.t
	t.Helper()
	if changes := treeChanges(w.overlayBefore, treeDigest(t, w.overlay)); len(changes) > 0 {
		t.Errorf("the run changed the overlay, which it only reads: %s", strings.Join(changes, "; "))
	}
	if left := entryNames(t, w.cwd); len(left) > 0 {
		t.Errorf("the run left %v in the directory it was started in", left)
	}
	if got, want := entryNames(t, w.root), []string{"bin", "cwd", "home", "state", "tmp"}; !slices.Equal(got, want) {
		t.Errorf("the sandbox holds %v, want only the directories the test made, %v", got, want)
	}
	w.assertTheMachineIsUntouched()
}

// assertTheMachineIsUntouched looks for the probe agent where the person's own Pi would link it and
// where their own state would hold the package. Nobody else makes a file of that name, so finding
// one means this run wrote it, and the sandbox did not hold. What the person's Pi does in the same
// time (its settings, its sessions) is not read: it changes without any test.
func (w *globalTierWorld) assertTheMachineIsUntouched() {
	t := w.t
	t.Helper()
	real, err := piguard.RealHome()
	if err != nil {
		t.Logf("the home of the account is unknown, so the machine's own Pi is not checked: %v", err)
		return
	}
	for _, path := range []string{
		filepath.Join(real, ".pi", "agent", "agents", globalTierProbeAgent),
		filepath.Join(real, ".labdrian-overlay", "pi", "labdrian-pi", "agents", globalTierProbeAgent),
	} {
		if _, err := os.Lstat(path); err == nil {
			t.Errorf("the run reached the machine: %s exists", path)
		}
	}
}

// treeDigest describes every entry under dir by what it is: its mode, and the digest of a file or
// the target of a link. A change anywhere in the tree shows as a different map.
func treeDigest(t *testing.T, dir string) map[string]string {
	t.Helper()
	digest := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			digest[rel] = fmt.Sprintf("%s -> %s", info.Mode(), target)
		case info.IsDir():
			digest[rel] = info.Mode().String()
		default:
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			digest[rel] = fmt.Sprintf("%s %x", info.Mode(), sha256.Sum256(data))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read the tree %s: %v", dir, err)
	}
	return digest
}

// treeChanges lists, in path order, what differs between two digests of one tree.
func treeChanges(before, after map[string]string) []string {
	var changes []string
	for path, was := range before {
		switch now, ok := after[path]; {
		case !ok:
			changes = append(changes, "removed "+path)
		case now != was:
			changes = append(changes, "changed "+path)
		}
	}
	for path := range after {
		if _, ok := before[path]; !ok {
			changes = append(changes, "added "+path)
		}
	}
	sort.Strings(changes)
	return changes
}

// entryNames lists the names in dir, sorted.
func entryNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("list %s: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

// canonicalPath is path with its links followed when it can be, and cleaned otherwise: the form in
// which two spellings of one directory compare equal.
func canonicalPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return filepath.Clean(path)
}

// assertStartedWith checks one start of the stand-in pi: the arguments it got, in the HOME and the
// directory of the sandbox, which is how the program passes its own environment on.
func (w *globalTierWorld) assertStartedWith(start piStart, args ...string) {
	t := w.t
	t.Helper()
	if !slices.Equal(start.args, args) {
		t.Errorf("pi was started with %q, want %q", start.args, args)
	}
	if start.home != w.home {
		t.Errorf("pi found HOME %q, want the home of the sandbox %q", start.home, w.home)
	}
	if canonicalPath(start.cwd) != canonicalPath(w.cwd) {
		t.Errorf("pi was started in %q, want the directory the program ran in, %q", start.cwd, w.cwd)
	}
}

// installRun is `runtime install --target pi` as the program, in the default mode.
func (w *globalTierWorld) installRun() globalTierRun {
	w.t.Helper()
	return w.run(w.environment(w.standInPath()), "runtime", "install", "--target", "pi")
}

func TestPiGlobalTierInstallRunsPiOnThePackageItBuilt(t *testing.T) {
	w := newGlobalTierWorld(t)
	w.putStandInPi()

	run := w.installRun()

	if run.code != 0 {
		t.Fatalf("install --target pi exited %d\nstdout=%q\nstderr=%q", run.code, run.stdout, run.stderr)
	}
	if want := "[pi] install: restart_required — ran `pi install " + w.pkg + "`"; !strings.Contains(run.stdout, want) {
		t.Errorf("stdout = %q, want it to say %q", run.stdout, want)
	}
	if want := "Ran `pi install " + globalTierSubagentsPackage + "`"; !strings.Contains(run.stdout, want) {
		t.Errorf("stdout = %q, want it to say %q", run.stdout, want)
	}
	if run.stderr != "" {
		t.Errorf("stderr = %q, want nothing", run.stderr)
	}

	starts := w.starts()
	if len(starts) != 2 {
		t.Fatalf("pi was started %d times (%v), want twice: the package, then the Subagents extension", len(starts), starts)
	}
	w.assertStartedWith(starts[0], "install", w.pkg)
	w.assertStartedWith(starts[1], "install", globalTierSubagentsPackage)

	w.assertInstalled()
	w.assertNothingEscaped()
}

func TestPiGlobalTierRemoveRunsPiRemoveAndDeletesThePackage(t *testing.T) {
	w := newGlobalTierWorld(t)
	w.putStandInPi()
	if run := w.installRun(); run.code != 0 {
		t.Fatalf("install --target pi exited %d\nstdout=%q\nstderr=%q", run.code, run.stdout, run.stderr)
	}
	installed := len(w.starts())
	w.assertInstalled()

	run := w.run(w.environment(w.standInPath()), "runtime", "uninstall", "--target", "pi")

	if run.code != 0 {
		t.Fatalf("uninstall --target pi exited %d\nstdout=%q\nstderr=%q", run.code, run.stdout, run.stderr)
	}
	if want := "[pi] uninstall: supported — removed via `pi remove " + w.pkg + "`; package directory deleted."; !strings.Contains(run.stdout, want) {
		t.Errorf("stdout = %q, want it to say %q", run.stdout, want)
	}
	if run.stderr != "" {
		t.Errorf("stderr = %q, want nothing", run.stderr)
	}

	all := w.starts()
	removal := all[min(installed, len(all)):]
	if len(removal) != 1 {
		t.Fatalf("the removal started pi %d times (%v), want once", len(removal), removal)
	}
	w.assertStartedWith(removal[0], "remove", w.pkg)

	w.assertRemoved()
	w.assertNothingEscaped()
}

// The sandbox is built on piguard's refusal of the home of the person running the tests. That
// refusal is proved in piguard's own tests; here it is proved in the process of this package,
// whose HOME TestMain pinned: the account's home and HOME are two different directories, and the
// refusal must still know the first.
func TestPiGlobalTierSandboxRefusesTheRealHome(t *testing.T) {
	real, err := piguard.RealHome()
	if err != nil {
		t.Skipf("the home of the account is unknown here: %v", err)
	}
	if canonicalPath(real) == canonicalPath(os.Getenv("HOME")) {
		t.Skipf("HOME is the home of the account here (%s), so a refusal of it could be HOME's", real)
	}
	if err := piguard.RefuseRealHome(real); err == nil {
		t.Fatalf("piguard takes %s, the home of the account running the tests, for a throwaway HOME", real)
	}
}

// listedPiPackages reads the packages Pi lists in the settings file under agentDir, as the paths
// they name. Pi records a local package relative to the directory of the file (packages.md), and
// the program reads it that way too, so a path that is not absolute is read from agentDir.
func listedPiPackages(t *testing.T, agentDir string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(agentDir, "settings.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatalf("read the settings of the real pi: %v", err)
	}
	var settings struct {
		Packages []json.RawMessage `json:"packages"`
	}
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatalf("settings.json is not JSON: %v\n%s", err, raw)
	}
	var listed []string
	for _, entry := range settings.Packages {
		var source string
		if json.Unmarshal(entry, &source) != nil {
			var filtered struct {
				Source string `json:"source"`
			}
			if json.Unmarshal(entry, &filtered) != nil {
				continue
			}
			source = filtered.Source
		}
		if !filepath.IsAbs(source) {
			source = filepath.Join(agentDir, source)
		}
		listed = append(listed, canonicalPath(source))
	}
	return listed
}

// The live test runs the real pi of the machine, started by the program, against a HOME that belongs
// to the test. It is skipped unless the person running the tests exports LABDRIAN_PI_LIVE_TEST=1.
// What it expects of Pi is what Pi documents (packages.md, cli.md): `pi install <local directory>`
// records the package in <agent dir>/settings.json, relative to that directory and without the
// network, and `pi remove` takes it out again. The Subagents extension is a network install of a
// third-party package, so the run turns it off; Pi is told to stay offline, to skip its version
// check and to send no telemetry; and the agent directory is named instead of left to HOME.
func TestPiGlobalTierLiveInAThrowawayHome(t *testing.T) {
	live, err := piguard.RequestLive()
	if errors.Is(err, piguard.ErrLiveNotRequested) {
		t.Skipf("%v (skipped by default, so that no run of the tests starts the real pi)", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	w := newGlobalTierWorld(t) // refuses a HOME that is the person's own
	agentDir := filepath.Join(w.home, ".pi", "agent")
	t.Logf("running the real pi at %s against the throwaway HOME %s", live.Pi, w.home)
	env := w.environment(live.Path,
		"LABDRIAN_PI_SKIP_SUBAGENTS=1",
		"PI_CODING_AGENT_DIR="+agentDir,
		"PI_OFFLINE=1",
		"PI_SKIP_VERSION_CHECK=1",
		"PI_TELEMETRY=0",
	)

	install := w.run(env, "runtime", "install", "--target", "pi")

	if install.code != 0 {
		t.Fatalf("install --target pi exited %d\nstdout=%q\nstderr=%q", install.code, install.stdout, install.stderr)
	}
	if want := "ran `pi install " + w.pkg + "`"; !strings.Contains(install.stdout, want) {
		t.Errorf("stdout = %q, want it to say %q", install.stdout, want)
	}
	if listed := listedPiPackages(t, agentDir); !slices.Contains(listed, canonicalPath(w.pkg)) {
		t.Errorf("the settings of the real pi list %q, want the package %s among them", listed, w.pkg)
	}
	w.assertInstalled()
	w.assertNothingEscaped()

	remove := w.run(env, "runtime", "uninstall", "--target", "pi")

	if remove.code != 0 {
		t.Fatalf("uninstall --target pi exited %d\nstdout=%q\nstderr=%q", remove.code, remove.stdout, remove.stderr)
	}
	if want := "removed via `pi remove " + w.pkg + "`"; !strings.Contains(remove.stdout, want) {
		t.Errorf("stdout = %q, want it to say %q", remove.stdout, want)
	}
	if listed := listedPiPackages(t, agentDir); slices.Contains(listed, canonicalPath(w.pkg)) {
		t.Errorf("the settings of the real pi still list the package after `pi remove`: %q", listed)
	}
	w.assertRemoved()
	w.assertNothingEscaped()
}
