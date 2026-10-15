package overlaycli

import (
	"bufio"
	"strconv"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/tui/internal/domain"
)

// parseSyncCheck extracts per-target verdicts from the stdout of one sync-check:
// the lines readSyncLines reads, added up by domain.SyncReport. It is what the
// launch probe makes of its one report.
func parseSyncCheck(output string) []domain.TargetVerdict {
	var report domain.SyncReport
	report.Add(readSyncLines(output))
	return report.Verdicts()
}

// readSyncLines reads the lines of a sync-check report that say something, in
// order, and nothing of what they add up to: that is domain.SyncReport's, which
// an action reads across all of its invocations.
//
// It reads the machine-friendly lines emitted by the backend:
//
//	VERDICT:<target>:UPSTREAM_CHANGED=N OVERLAY_NOT_DEPLOYED=M REPO_BEHIND_ORIGIN=<n|NA>
//	ACTION:<target>: <text>
//
// It also reads the `=== sync-check: <target> ===` section headers, which set
// the section the per-file `agents/...` status lines (format: `  <path>:
// <STATUS>`) that follow belong to. A VERDICT line gives only the fields it
// names, and one it cannot read is left out rather than guessed.
func readSyncLines(output string) []domain.SyncLine {
	var lines []domain.SyncLine

	scanner := bufio.NewScanner(strings.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Section header: "=== sync-check: <target> (<path>) ==="
		if strings.HasPrefix(line, "=== sync-check: ") {
			rest := strings.TrimPrefix(line, "=== sync-check: ")
			target, _, _ := strings.Cut(rest, " ")
			lines = append(lines, domain.SyncLine{Kind: domain.SyncSectionLine, Target: target})
			continue
		}

		if rest, ok := strings.CutPrefix(line, "VERDICT:"); ok {
			// rest = "<target>:UPSTREAM_CHANGED=N OVERLAY_NOT_DEPLOYED=M"
			target, counts, ok := strings.Cut(rest, ":")
			if !ok {
				continue
			}
			lines = append(lines, domain.SyncLine{Kind: domain.SyncVerdictLine, Target: target, Fields: readVerdictFields(counts)})
			continue
		}

		if rest, ok := strings.CutPrefix(line, "ACTION:"); ok {
			// rest = "<target>: <text>"
			target, text, ok := strings.Cut(rest, ":")
			if !ok {
				continue
			}
			lines = append(lines, domain.SyncLine{Kind: domain.SyncActionLine, Target: target, Remedy: strings.TrimSpace(text)})
			continue
		}

		// Per-file lines: the agents/ file statuses of the section last opened.
		for _, s := range []string{"IN_SYNC", "OVERLAY_NOT_DEPLOYED", "UPSTREAM_CHANGED"} {
			if rest, ok := strings.CutPrefix(line, s+": "); ok {
				if strings.HasPrefix(rest, "agents/") {
					path, _, _ := strings.Cut(rest, " ") // strip "(detail)" suffix
					lines = append(lines, domain.SyncLine{Kind: domain.SyncAgentFileLine, File: domain.AgentFileEntry{Path: path, Status: s}})
				}
				break
			}
		}
	}
	return lines
}

// readVerdictFields reads the "KEY=value" fields of a VERDICT line.
func readVerdictFields(counts string) domain.VerdictFields {
	var fields domain.VerdictFields
	for _, field := range strings.Fields(counts) {
		key, val, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}
		switch key {
		case "UPSTREAM_CHANGED":
			n, _ := strconv.Atoi(val)
			fields.UpstreamChanged = &n
		case "OVERLAY_NOT_DEPLOYED":
			n, _ := strconv.Atoi(val)
			fields.OverlayNotDeployed = &n
		case "REPO_BEHIND_ORIGIN":
			// Dedicated NA pre-check: the field's literal "NA" sentinel
			// MUST be handled before strconv.Atoi. Falling through to a
			// generic `n, _ := strconv.Atoi(val)` (as UPSTREAM_CHANGED/
			// OVERLAY_NOT_DEPLOYED do) would discard the parse error and
			// silently collapse "NA" (or an omitted field) to Go's
			// zero-value 0 — indistinguishable from "confirmed 0 commits
			// behind" and reproducing the exact silent-healthy bug class
			// R-006 exists to eliminate.
			fields.RepoBehindOrigin = readBehindCount(val, fields.RepoBehindOrigin)
		case "REPO_BEHIND_RELEASE":
			// Same dedicated NA pre-check as REPO_BEHIND_ORIGIN above,
			// for the same reason (D2 — a new field, same
			// silent-zero-collapse risk).
			fields.RepoBehindRelease = readBehindCount(val, fields.RepoBehindRelease)
		case "RECORDED_VERSION":
			fields.RecordedVersion = &val
		case "DIGEST_MATCH":
			fields.DigestMatch = &val
		}
	}
	return fields
}

// readBehindCount reads a REPO_BEHIND_* value: the NA sentinel, or a count. A
// value that is neither is not read, and what the field was is kept.
func readBehindCount(val string, kept *int) *int {
	if val == "NA" {
		na := domain.RepoBehindOriginNA
		return &na
	}
	if n, err := strconv.Atoi(val); err == nil {
		return &n
	}
	return kept
}
