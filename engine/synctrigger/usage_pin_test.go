package synctrigger

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// These pin what the trigger says when it is started with an event or a working directory it
// cannot use, and where it says it. They never reach a program, so they hold for the trigger as
// it is whatever starts the programs and whatever tells it the time: the line on stderr, the
// absence of a log when Run is refused, and the line in the log when RunChild is.

// usageLine is what the trigger says on stderr for an event and a cwd, as the caller gave them.
func usageLine(event, cwd string) string {
	return fmt.Sprintf("sync-trigger: error:usage event=%q cwd=%q\n", event, cwd)
}

var usageLogLine = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z event=(\S*) cwd=(\S*) outcome=error:usage exit=0 duration=\S+$`)

func TestRunRefusesAnEventOrCwdItCannotUseSayingSoOnStderrAndLeavesNoLog(t *testing.T) {
	for _, tc := range []struct{ name, event, cwd string }{
		{"an unknown event and an absolute cwd", "bogus", "/abs/dir"},
		{"an unknown event and a relative cwd, which is said as it was given", "bogus", "rel/dir"},
		{"an empty event", "", "/abs/dir"},
		{"an empty cwd, which never inherits the caller's", "session-end", ""},
		{"an empty event and an empty cwd", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stateDir := t.TempDir()
			var stderr strings.Builder

			got := Run(Options{Event: tc.event, Cwd: tc.cwd, StateDir: stateDir, Stderr: &stderr})

			if got != 0 {
				t.Errorf("Run() = %d, want 0: the hook must never see a failure", got)
			}
			if want := usageLine(tc.event, tc.cwd); stderr.String() != want {
				t.Errorf("stderr = %q, want %q", stderr.String(), want)
			}
			if _, err := os.Stat(filepath.Join(stateDir, "logs")); !os.IsNotExist(err) {
				t.Errorf("the log directory exists after a refusal (stat error %v): nothing had started", err)
			}
		})
	}
}

func TestRunChildRefusesAnEventOrCwdItCannotUseSayingSoOnStderrAndInTheLog(t *testing.T) {
	for _, tc := range []struct{ name, event, cwd string }{
		{"an empty cwd", "session-end", ""},
		{"a relative cwd, which a child is never given and does not resolve", "session-end", "rel/dir"},
		{"an unknown event", "bogus", "/abs/dir"},
		{"an empty event", "", "/abs/dir"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stateDir := t.TempDir()
			var stderr strings.Builder

			got := RunChild(Options{Event: tc.event, Cwd: tc.cwd, StateDir: stateDir, Binary: filepath.Join(stateDir, "never-reached"), Stderr: &stderr})

			if got.Kind != "error:usage" || got.Exit != 0 {
				t.Errorf("RunChild = %+v, want Kind error:usage and Exit 0", got)
			}
			if want := usageLine(tc.event, tc.cwd); stderr.String() != want {
				t.Errorf("stderr = %q, want %q", stderr.String(), want)
			}
			data, err := os.ReadFile(filepath.Join(stateDir, "logs", "sync-trigger.log"))
			if err != nil {
				t.Fatalf("read log: %v", err)
			}
			line := strings.TrimSuffix(string(data), "\n")
			m := usageLogLine.FindStringSubmatch(line)
			if m == nil {
				t.Fatalf("log = %q, want one line of the form `<time> event=<e> cwd=<c> outcome=error:usage exit=0 duration=<d>`", data)
			}
			if tc.event != "" && m[1] != tc.event {
				t.Errorf("log event = %q, want %q", m[1], tc.event)
			}
		})
	}
}
