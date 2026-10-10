package main

import (
	"bytes"
	"errors"
	"io/fs"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings/settingsfile"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/status"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/status/fsfiles"
)

// The command is tested for what it adds to the use case: the lines it prints, the code it exits
// with and the adapters it wires. The checks themselves are the status package's tests, so the
// ports here answer a fixed world.

// cannedFiles is the Files port of a world in which every path is an executable file that can be
// statted and no file can be read: the binary is there, the contract is not.
type cannedFiles struct{ mode fs.FileMode }

func (f cannedFiles) Stat(string) (fs.FileMode, error) { return f.mode, nil }
func (f cannedFiles) ReadFile(string) ([]byte, error) {
	return nil, &fs.PathError{Op: "open", Err: fs.ErrNotExist}
}

// cannedSettings is the SettingsSource port of a world whose settings.json is read with err, and
// holds nothing otherwise.
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

func TestStatusCoreWritesTheWholeReportOfTheServiceAndAnswersItsOutcome(t *testing.T) {
	service := status.Service{Files: cannedFiles{mode: 0o755}, Settings: cannedSettings{err: errors.New("boom")}}
	var out bytes.Buffer
	outcome := statusCore(&out, service, status.Request{Home: "/h"})
	if outcome != status.Failed {
		t.Errorf("outcome = %v, want Failed: the settings could not be read", outcome)
	}
	const unreadable = " — cannot read /h/.claude/settings.json: boom\n"
	want := "[OK  ] binary: /h/.claude/bin/gentle-ai-overlay\n"
	for _, label := range []string{
		"hook: UserPromptSubmit (propagate)",
		`hook: PreToolUse matcher="Agent" (gate-task)`,
		"hook: SessionEnd (sync-trigger)",
		`hook: PreToolUse matcher="Bash" (review-receipt)`,
		"guard: shaper clearance record (PreToolUse + permissions.deny)",
		"hooks: projection (UserPromptSubmit + PreToolUse gates)",
		"guard: skills approve (PreToolUse Bash + file tools)",
	} {
		want += "[FAIL] " + label + unreadable
	}
	want += "[FAIL] contract: /h/.claude/skills/_shared/minimalism-contract.md — not found\n"
	if out.String() != want {
		t.Errorf("the report is\n%s\nwant\n%s", out.String(), want)
	}
}

func TestTheStatusCommandIsWiredToTheFileSystemAndTheSettingsFile(t *testing.T) {
	service := newStatusService()
	if _, ok := service.Files.(fsfiles.Files); !ok {
		t.Errorf("Files is %T, want the file system adapter", service.Files)
	}
	if _, ok := service.Settings.(settingsfile.Reader); !ok {
		t.Errorf("Settings is %T, want the settings.json reader", service.Settings)
	}
}
