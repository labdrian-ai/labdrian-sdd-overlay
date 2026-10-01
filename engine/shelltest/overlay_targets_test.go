package shelltest

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// targetsHermeticEnv returns an environment for running the overlay script
// without any path to the developer's real state: HOME and STATE_DIR both
// point into the test's scratch directory. STATE_DIR is set explicitly
// because the script honors it ahead of $HOME/.labdrian-overlay, so a value
// exported in the caller's shell would otherwise leak into the run.
func targetsHermeticEnv(t *testing.T) (env []string, home string) {
	t.Helper()
	home = t.TempDir()
	return append(os.Environ(), "HOME="+home, "STATE_DIR="+home+"/state"), home
}

// runOverlayTargets runs `labdrian-overlay targets <args...>` and returns
// stdout and stderr separately, because the catalog is a machine contract:
// a diagnostic on stderr must never be mistaken for a target line.
func runOverlayTargets(t *testing.T, args ...string) (stdout, stderr string, exitCode int, home string) {
	t.Helper()
	overlay := piTargetOverlayPath(t)
	env, home := targetsHermeticEnv(t)

	cmd := exec.Command(overlay, append([]string{"targets"}, args...)...)
	cmd.Env = env
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("targets %v: failed to run: %v", args, err)
	}
	return outBuf.String(), errBuf.String(), exitCode, home
}

// TestTargets_PrintsTheCatalogOneTargetPerLine pins the wire format the TUI
// parses: one "<name><TAB><kind>" line per target, in the order `--target
// all` expands to, kind being "copy" or "package". Adding or reordering a
// target changes this contract on purpose, so this test must be edited with
// it.
func TestTargets_PrintsTheCatalogOneTargetPerLine(t *testing.T) {
	stdout, stderr, exitCode, _ := runOverlayTargets(t)

	if exitCode != 0 {
		t.Fatalf("targets exit = %d, want 0\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	const want = "claude\tcopy\nopencode\tcopy\ncodex\tcopy\npi\tpackage\n"
	if stdout != want {
		t.Fatalf("targets stdout = %q, want %q", stdout, want)
	}
	if stderr != "" {
		t.Fatalf("targets must write nothing to stderr on success, got %q", stderr)
	}
}

// TestTargets_NamesAreExactlyWhatTargetAllExpandsTo is the single-source
// guarantee: the catalog and the expansion of `--target all` are two views of
// one list, so the TUI can never show a set that `all` then exceeds. Both
// sides are read from the real script, so a second hardcoded list added to
// either would fail here.
func TestTargets_NamesAreExactlyWhatTargetAllExpandsTo(t *testing.T) {
	overlay := piTargetOverlayPath(t)

	stdout, _, exitCode, _ := runOverlayTargets(t)
	if exitCode != 0 {
		t.Fatalf("targets exit = %d, want 0", exitCode)
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSuffix(stdout, "\n"), "\n") {
		name, _, _ := strings.Cut(line, "\t")
		names = append(names, name)
	}

	out, err := sourceAndRun(t, overlay, `resolve_targets all`)
	if err != nil {
		t.Fatalf("resolve_targets all failed: %v\n%s", err, out)
	}
	if got, want := strings.Join(names, " "), strings.TrimSpace(out); got != want {
		t.Fatalf("targets names = %q, but `--target all` expands to %q", got, want)
	}
}

// TestTargets_KindsAgreeWithIsCopyTarget keeps the kind column honest: it is
// what tells the TUI that capture and restore refuse a package target, so it
// must say "copy" exactly where the backend's own is_copy_target does.
func TestTargets_KindsAgreeWithIsCopyTarget(t *testing.T) {
	overlay := piTargetOverlayPath(t)

	stdout, _, exitCode, _ := runOverlayTargets(t)
	if exitCode != 0 {
		t.Fatalf("targets exit = %d, want 0", exitCode)
	}
	for _, line := range strings.Split(strings.TrimSuffix(stdout, "\n"), "\n") {
		name, kind, ok := strings.Cut(line, "\t")
		if !ok {
			t.Fatalf("line %q has no tab-separated kind", line)
		}
		out, err := sourceAndRun(t, overlay, `is_copy_target `+name+` && echo copy || echo package`)
		if err != nil {
			t.Fatalf("is_copy_target %s failed: %v\n%s", name, err, out)
		}
		if got := strings.TrimSpace(out); got != kind {
			t.Errorf("target %s: targets says kind %q, is_copy_target says %q", name, kind, got)
		}
	}
}

// TestTargets_IsReadOnly proves the subcommand writes nothing: not under
// HOME and not under STATE_DIR (both scratch directories here).
func TestTargets_IsReadOnly(t *testing.T) {
	_, _, exitCode, home := runOverlayTargets(t)
	if exitCode != 0 {
		t.Fatalf("targets exit = %d, want 0", exitCode)
	}
	entries, err := os.ReadDir(home)
	if err != nil {
		t.Fatalf("read scratch HOME: %v", err)
	}
	if len(entries) != 0 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("targets must not write anything; scratch HOME now holds %v", names)
	}
}

// TestTargets_RefusesArguments: the catalog has no options, so an argument is
// a caller mistake and must fail loudly instead of being ignored.
func TestTargets_RefusesArguments(t *testing.T) {
	stdout, stderr, exitCode, _ := runOverlayTargets(t, "--target", "claude")

	if exitCode == 0 {
		t.Fatalf("targets with an argument must exit non-zero, got 0\nstdout: %s", stdout)
	}
	if stdout != "" {
		t.Fatalf("a refused targets call must print no catalog lines on stdout, got %q", stdout)
	}
	// "Unknown command: targets" also names the subcommand, so the message
	// must say why the call was refused, not merely that it was.
	if !strings.Contains(stderr, "takes no arguments") {
		t.Fatalf("the refusal should say targets takes no arguments, got %q", stderr)
	}
}

// TestUsage_DocumentsTheTargetsSubcommand: the format is a contract, so the
// usage text states it.
func TestUsage_DocumentsTheTargetsSubcommand(t *testing.T) {
	overlay := piTargetOverlayPath(t)
	env, _ := targetsHermeticEnv(t)

	cmd := exec.Command(overlay, "--help")
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("--help failed: %v\n%s", err, out)
	}
	usage := string(out)
	for _, fragment := range []string{"targets", "<name><TAB><kind>", "--target all"} {
		if !strings.Contains(usage, fragment) {
			t.Errorf("usage must mention %q for the targets subcommand", fragment)
		}
	}
}
