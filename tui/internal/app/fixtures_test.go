package app

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/tui/internal/domain"
	"github.com/labdrian-ai/labdrian-sdd-overlay/tui/internal/testhome"
)

func TestMain(m *testing.M) { os.Exit(testhome.Run(m)) }

func TestTheTestsRunInAHomeOfTheirOwn(t *testing.T) { testhome.Check(t) }

// threeCopyTargets is the catalog most tests run against: the three per-file
// targets, as the backend lists them.
func threeCopyTargets() []domain.Target {
	return []domain.Target{
		{Name: "claude", Kind: domain.KindCopy},
		{Name: "opencode", Kind: domain.KindCopy},
		{Name: "codex", Kind: domain.KindCopy},
	}
}

// fourTargets adds the package target, as the backend's own catalog does.
func fourTargets() []domain.Target {
	return append(threeCopyTargets(), domain.Target{Name: "pi", Kind: domain.KindPackage})
}

func targetNames(targets []domain.Target) string {
	var names []string
	for _, t := range targets {
		names = append(names, t.Name)
	}
	return strings.Join(names, ", ")
}

// fakeCatalog is a TargetCatalog that answers from memory and counts how many
// times it was asked.
type fakeCatalog struct {
	targets []domain.Target
	err     error
	asked   int
}

func (f *fakeCatalog) Targets() ([]domain.Target, error) {
	f.asked++
	if f.err != nil {
		return nil, f.err
	}
	return append([]domain.Target(nil), f.targets...), nil
}

// fakeBackups is a BackupQuery that answers from memory. It records which
// targets it was asked about, so a test can prove a target was never queried.
type fakeBackups struct {
	byTarget map[string]domain.Backup
	errs     map[string]error
	asked    []string
}

func (f *fakeBackups) LatestBackup(target string) (domain.Backup, bool, error) {
	f.asked = append(f.asked, target)
	b, ok := f.byTarget[target]
	// A failing query may still hand back what it knew. The planner must go by
	// the error, not by a backup returned alongside it.
	return b, ok, f.errs[target]
}

// fakeBackend is a Backend that records every invocation and answers each
// from the script: by the whole invocation when it has an entry for it, else
// by its subcommand (the first argument); no entry is a clean run that printed
// nothing.
type fakeBackend struct {
	script map[string]domain.Execution
	ran    []string
}

func (f *fakeBackend) Run(args []string) domain.Execution {
	call := strings.Join(args, " ")
	f.ran = append(f.ran, call)
	if run, ok := f.script[call]; ok {
		return run
	}
	return f.script[args[0]]
}

// The lines of a sync-check report, as the adapter reads them: what a test
// needs to give an execution a report without any text.

func intp(n int) *int { return &n }

func strptr(s string) *string { return &s }

func section(target string) domain.SyncLine {
	return domain.SyncLine{Kind: domain.SyncSectionLine, Target: target}
}

func verdict(target string, fields domain.VerdictFields) domain.SyncLine {
	return domain.SyncLine{Kind: domain.SyncVerdictLine, Target: target, Fields: fields}
}

func remedy(target, text string) domain.SyncLine {
	return domain.SyncLine{Kind: domain.SyncActionLine, Target: target, Remedy: text}
}

func agentFile(path, status string) domain.SyncLine {
	return domain.SyncLine{Kind: domain.SyncAgentFileLine, File: domain.AgentFileEntry{Path: path, Status: status}}
}

// failing is an Execution that ended with exit status code.
func failing(output string, code int) domain.Execution {
	return domain.Execution{Output: output, ExitCode: code, Err: fmt.Errorf("exit status %d", code)}
}
