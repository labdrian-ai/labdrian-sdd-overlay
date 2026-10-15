package app

import (
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/tui/internal/domain"
)

// RestorePlanner decides what a restore may be offered for and what the
// operator is told it will do.
type RestorePlanner struct {
	backups domain.BackupQuery
}

// NewRestorePlanner returns the planner over the query that says what each
// target can be rolled back to. A planner without one (nil) offers nothing: a
// destructive rollback is only offered for a backup the backend vouched for.
func NewRestorePlanner(backups domain.BackupQuery) RestorePlanner {
	return RestorePlanner{backups: backups}
}

// Confirmation builds the restore action's per-invocation confirm copy (D4):
// the base overwrite-warning text from the action, followed by each candidate
// target's most recent backup timestamp + version -- asked of the BackupQuery
// port, never read from the state directory here and never a TUI-side
// timestamp picker (D4: the TUI always targets the most recent backup only). A
// target whose backups the query cannot confirm (an error, not merely "none")
// is left out like one with none: a destructive rollback is only offered for a
// backup the backend vouched for.
//
// The returned targets are the SAME backup-bearing subset the confirm text
// names -- the caller must reuse it for the actual invocation too, never
// recompute it from the current selection (that recomputation is exactly the
// bug an adversarial review caught: a target with zero backups would be
// invoked, fail, and make the whole action misreport as failed even though the
// backup-bearing target's destructive restore had already succeeded). ok is
// false when NONE of the candidates have an available backup, the signal the
// caller uses to refuse entering the confirm screen at all (R-003). candidates
// are the selected targets restore can apply to at all (copy targets only).
//
// The query spawns the backend, so a caller runs this off its UI goroutine.
// The candidates are asked one after another: a listing takes milliseconds and
// the port need not be concurrency-safe.
func (p RestorePlanner) Confirmation(action domain.Action, candidates []domain.Target) (message string, targets []domain.Target, ok bool) {
	if p.backups == nil {
		return "", nil, false
	}
	var lines []string
	for _, t := range candidates {
		backup, hasBackup, err := p.backups.LatestBackup(t.Name)
		if err != nil || !hasBackup {
			continue
		}
		// "desconocida" (unknown): the backup exists and is restorable, only
		// its version label could not be read.
		version := backup.Version
		if version == "" {
			version = "desconocida"
		}
		lines = append(lines, fmt.Sprintf("%s: %s (%s)", t.Name, backup.Timestamp, version))
		targets = append(targets, t)
	}
	if len(lines) == 0 {
		return "", nil, false
	}
	return action.ConfirmMessage + "\n\nRespaldo a restaurar:\n  " + strings.Join(lines, "\n  "), targets, true
}
