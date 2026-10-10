package main

import (
	"strings"
	"testing"
)

// Every subcommand is reached through run with the process and the deps run was handed, and with
// the arguments after its name. Each is given nothing to do, so it says what it needs; what it says
// is on the stream of the process the test built (a command that used the process's own would
// write there and end the test binary with the exit of the program), and its exit is the one the
// test recorded.

func TestRunRoutesEachSubcommandThroughTheProcessItWasHanded(t *testing.T) {
	for _, tc := range []struct {
		name       string
		stream     string // "stdout" or "stderr"
		wantPrefix string
		wantExits  []int
	}{
		{"propagate", "stderr", "error: --registry is required\n", []int{1}},
		{"gate-task", "stderr", "gate-task: warning: --contract-file not provided; all Agent hooks will pass through\n", nil},
		{"merge-settings", "stderr", "error: --settings is required\n", []int{1}},
		{"uninstall-hooks", "stderr", "error: --settings is required\n", []int{1}},
		{"status", "stdout", "[FAIL] binary: ", []int{1}},
		{"prespec", "stderr", "error: prespec requires a verb: rank, lint, readiness, brief\n", []int{1}},
		{"runtime", "stderr", "error: runtime requires an action\nUsage:\n", []int{1}},
		{"gadu-generate", "stderr", "gadu-generate: OVERLAY_DIR is not set\n", []int{1}},
		{"pipkg", "stderr", "error: pipkg requires a verb: build, check\n", []int{1}},
		{"skills", "stderr", "error: skills requires a verb: list, status, ", []int{1}},
		{"sync-trigger", "stderr", "sync-trigger: error:usage event=\"\" cwd=\"\"\n", []int{0}},
		{"review-receipt", "stderr", "error: review-receipt requires a verb: capture, hook\n", []int{1}},
		{"shaper", "stderr", "error: shaper requires a verb: assess, clearance, guard-hook\n", []int{1}},
		{"roles", "stderr", "error: roles requires a verb: validate, next, resume, append, match-shaper\n", []int{1}},
		{"memory", "stderr", "error: memory requires a verb: plan\n", []int{1}},
		{"workflow", "stderr", "error: workflow requires a verb: create, start, ", []int{1}},
		{"projection", "stderr", "error: projection requires an action: hook\n", []int{1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newCapturedProcess()
			d := testDeps()
			d.getenv = environmentOf(map[string]string{"HOME": t.TempDir()})
			d.getwd = func() (string, error) { return t.TempDir(), nil }

			run(p.process, d, []string{tc.name})

			got := p.err.String()
			other := p.out.String()
			if tc.stream == "stdout" {
				got, other = other, got
			}
			if !strings.HasPrefix(got, tc.wantPrefix) {
				t.Errorf("%s began %q, want %q", tc.stream, firstLines(got, 2), tc.wantPrefix)
			}
			if tc.name != "gate-task" && tc.name != "status" && other != "" {
				t.Errorf("the other stream holds %q, want nothing", other)
			}
			if !equalInts(p.exits, tc.wantExits) {
				t.Errorf("exit calls = %v, want %v", p.exits, tc.wantExits)
			}
		})
	}
}

// What the process reads for a subcommand it reads from the deps run was handed, not from the
// program's: the overlay directory of gadu-generate, and the home and working directory of status.
func TestRunHandsEachSubcommandTheDepsItWasHanded(t *testing.T) {
	t.Run("gadu-generate reads OVERLAY_DIR from the deps", func(t *testing.T) {
		p := newCapturedProcess()
		d := testDeps()
		d.getenv = environmentOf(map[string]string{"OVERLAY_DIR": t.TempDir()})

		run(p.process, d, []string{"gadu-generate", "--check"})

		if !strings.HasPrefix(p.err.String(), "gadu-generate --check: GADU artifacts are stale or missing:") {
			t.Errorf("stderr = %q, want the check of the directory the deps named", firstLines(p.err.String(), 2))
		}
	})
	t.Run("status reads HOME and the working directory from the deps", func(t *testing.T) {
		home, project := t.TempDir(), t.TempDir()
		p := newCapturedProcess()
		d := testDeps()
		d.getenv = environmentOf(map[string]string{"HOME": home})
		d.getwd = func() (string, error) { return project, nil }

		run(p.process, d, []string{"status"})

		if out := p.out.String(); !strings.Contains(out, home) || !strings.Contains(out, project) {
			t.Errorf("the report does not name the home %s and the directory %s of the deps:\n%s", home, project, out)
		}
	})
}

// The arguments after the subcommand's name are the ones it is given, in order.
func TestRunHandsEachSubcommandTheArgumentsAfterItsName(t *testing.T) {
	p := newCapturedProcess()

	run(p.process, testDeps(), []string{"roles", "bogus", "x"})

	if want := "error: roles: unknown verb \"bogus\" (expected validate, next, resume, append, or match-shaper)\n"; p.err.String() != want {
		t.Errorf("stderr = %q, want %q", p.err.String(), want)
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
