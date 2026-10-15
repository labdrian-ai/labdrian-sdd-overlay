package main

// Temporary forwarding while the vocabulary of the TUI moves into internal/domain one group at a time: the names
// the rest of this package still uses, standing for what the domain now owns. The commit that moves the last
// group qualifies every use with domain. and deletes this file.

import "github.com/labdrian-ai/labdrian-sdd-overlay/tui/internal/domain"

type (
	Target        = domain.Target
	TargetKind    = domain.TargetKind
	TargetCatalog = domain.TargetCatalog
	Backup        = domain.Backup
	BackupQuery   = domain.BackupQuery

	AgentFileEntry = domain.AgentFileEntry
	SyncStatus     = domain.SyncStatus
	TargetVerdict  = domain.TargetVerdict

	Action = domain.Action
)

const (
	KindCopy    = domain.KindCopy
	KindPackage = domain.KindPackage

	SyncUnknown        = domain.SyncUnknown
	SyncHealthy        = domain.SyncHealthy
	SyncNeedsApply     = domain.SyncNeedsApply
	SyncNeedsCapture   = domain.SyncNeedsCapture
	SyncBehindOrigin   = domain.SyncBehindOrigin
	SyncBehindRelease  = domain.SyncBehindRelease
	RepoBehindOriginNA = domain.RepoBehindOriginNA
)

func Actions() []Action { return domain.Actions() }

func allArgSets(action Action, selected []Target, allSelected bool) [][]string {
	return domain.AllArgSets(action, selected, allSelected)
}

func usesTargetAll(args []string) bool { return domain.UsesTargetAll(args) }

func invocationSeverity(err error, exitCode int) int { return domain.InvocationSeverity(err, exitCode) }

func ParseSyncCheck(output string) []TargetVerdict { return domain.ParseSyncCheck(output) }

func probeBehind(output string) int { return domain.ProbeBehind(output) }

func probeBehindRelease(output string) int { return domain.ProbeBehindRelease(output) }
