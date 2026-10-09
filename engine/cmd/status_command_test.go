package main

import (
	"bytes"
	"errors"
	"io/fs"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/status"
)

// canned is a status service whose ports answer what a test says, so the command is tested for
// what it adds to the use case: the lines it prints and the code it exits with.
type cannedFiles struct{ mode fs.FileMode }

func (f cannedFiles) Stat(string) (fs.FileMode, error) { return f.mode, nil }
func (f cannedFiles) ReadFile(string) ([]byte, error) {
	return nil, &fs.PathError{Op: "open", Err: fs.ErrNotExist}
}

type cannedSettings struct{ err error }

func (s cannedSettings) Settings(string) (settings.Document, error) {
	return settings.Document{}, s.err
}

func TestRenderReportWritesOneLinePerCheckInTheWordsOfTheStatusVerb(t *testing.T) {
	report := status.Report{Checks: []status.Check{
		{Label: "binary: /b", Level: status.OK},
		{Label: "registry: /r", Level: status.OK, Note: "scoped block present"},
		{Label: "hook: SessionEnd (sync-trigger)", Level: status.Warn, Note: "no entry"},
		{Label: "contract: /c", Level: status.Fail, Note: "not found"},
		{Label: "hook: UserPromptSubmit (propagate)", Level: status.Fail},
	}}
	var out bytes.Buffer
	renderReport(&out, report)
	want := "[OK  ] binary: /b\n" +
		"[OK  ] registry: /r — scoped block present\n" +
		"[WARN] hook: SessionEnd (sync-trigger) — no entry\n" +
		"[FAIL] contract: /c — not found\n" +
		"[FAIL] hook: UserPromptSubmit (propagate)\n"
	if out.String() != want {
		t.Errorf("renderReport wrote\n%s\nwant\n%s", out.String(), want)
	}
}

func TestRenderReportWritesNothingForNoChecks(t *testing.T) {
	var out bytes.Buffer
	renderReport(&out, status.Report{})
	if out.Len() != 0 {
		t.Errorf("renderReport wrote %q for a report with no checks", out.String())
	}
}

func TestTheExitCodeOfStatusSeparatesBrokenFromBehind(t *testing.T) {
	for outcome, want := range map[status.Outcome]int{status.Healthy: 0, status.Failed: 1, status.Degraded: 2} {
		if got := statusExitCode(outcome); got != want {
			t.Errorf("statusExitCode(%v) = %d, want %d", outcome, got, want)
		}
	}
}

func TestStatusCoreWritesTheReportOfTheServiceAndAnswersItsOutcome(t *testing.T) {
	service := status.Service{Files: cannedFiles{mode: 0o755}, Settings: cannedSettings{err: errors.New("boom")}}
	var out bytes.Buffer
	outcome := statusCore(&out, service, status.Request{Home: "/h"})
	if outcome != status.Failed {
		t.Errorf("outcome = %v, want Failed: the settings could not be read", outcome)
	}
	want := "[OK  ] binary: /h/.claude/bin/gentle-ai-overlay\n" +
		"[FAIL] hook: UserPromptSubmit (propagate) — cannot read /h/.claude/settings.json: boom\n"
	if got := out.String(); len(got) < len(want) || got[:len(want)] != want {
		t.Errorf("the report begins\n%s\nwant it to begin\n%s", got, want)
	}
}
