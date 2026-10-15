package domain

import (
	"bufio"
	"strconv"
	"strings"
)

// AgentFileEntry records the per-file sync status of a single agent file.
type AgentFileEntry struct {
	Path   string // relative path, e.g. "agents/GADU.md"
	Status string // "IN_SYNC", "OVERLAY_NOT_DEPLOYED", "UPSTREAM_CHANGED"
}

// SyncStatus is a color-coded health classification for a single target.
type SyncStatus int

const (
	// SyncUnknown means no VERDICT line was parsed for the target.
	SyncUnknown SyncStatus = iota
	// SyncHealthy means live == main: in sync with gentle-ai.
	SyncHealthy
	// SyncNeedsApply means overlay is not deployed (apply required).
	SyncNeedsApply
	// SyncNeedsCapture means upstream (gentle-ai) changed (capture + apply required).
	SyncNeedsCapture
	// SyncBehindOrigin means local main is behind this repo's own origin/main
	// (informational drift; never overridden by, and never masks, the other
	// two verdicts — see classify's precedence and R-006). Tracks main, not
	// HEAD, because self-update (the source of truth this mirrors) only ever
	// converges main, never the checked-out branch.
	SyncBehindOrigin
	// SyncBehindRelease means local main is behind the newest locally-known
	// release tag (D2, R-011): the primary "you should self-update" signal
	// once at least one release tag exists. Outranks SyncBehindOrigin in
	// classify's precedence — a target's own release standing is more
	// actionable than raw untagged origin drift.
	SyncBehindRelease
)

// RepoBehindOriginNA is the sentinel value for TargetVerdict.RepoBehindOrigin
// when the origin comparison is unavailable (no remote, no cached ref, or a
// requested fetch failed) — distinct from a confirmed 0 commits behind. A
// single named constant (instead of a magic -1 repeated at each use site)
// keeps every producer/consumer of this field agreeing by construction.
const RepoBehindOriginNA = -1

// TargetVerdict holds the parsed sync-check result for one target.
type TargetVerdict struct {
	Target             string
	UpstreamChanged    int
	OverlayNotDeployed int
	RepoBehindOrigin   int    // count; RepoBehindOriginNA when unavailable
	RepoBehindRelease  int    // count; RepoBehindOriginNA when unavailable (D2 — NA sentinel reused, not a new constant)
	RecordedVersion    string // verbatim wire value: "vX.Y.Z", "untagged" (D1), or "NA" (never deployed)
	DigestMatch        string // verbatim wire value: "yes", "no", or "NA" (never deployed)
	Action             string
	Status             SyncStatus
	AgentFiles         []AgentFileEntry // per-file statuses for agents/ paths
}

// classify maps verdict counts to a color status.
//
// Priority (matches the backend ACTION precedence, extended by R-006/D2):
//   - UPSTREAM_CHANGED > 0                         -> RED    (gentle-ai synced, needs capture+apply)
//   - OVERLAY_NOT_DEPLOYED > 0 OR digest mismatch  -> YELLOW (needs apply — a stale digest means the
//     deployed files no longer match what was recorded at the last apply, same remedy as a raw file diff)
//   - REPO_BEHIND_RELEASE > 0                      -> behind-release (D2: primary "self-update" signal
//     once a release tag exists; outranks raw origin drift)
//   - REPO_BEHIND_ORIGIN > 0                       -> behind-origin (never silently healthy)
//   - otherwise                                    -> GREEN (healthy)
func classify(upstreamChanged, overlayNotDeployed, repoBehindOrigin, repoBehindRelease int, digestMismatch bool) SyncStatus {
	switch {
	case upstreamChanged > 0:
		return SyncNeedsCapture
	case overlayNotDeployed > 0 || digestMismatch:
		return SyncNeedsApply
	case repoBehindRelease > 0:
		return SyncBehindRelease
	case repoBehindOrigin > 0:
		return SyncBehindOrigin
	default:
		return SyncHealthy
	}
}

// ParseSyncCheck extracts per-target verdicts from sync-check stdout.
//
// It reads the machine-friendly lines emitted by the backend:
//
//	VERDICT:<target>:UPSTREAM_CHANGED=N OVERLAY_NOT_DEPLOYED=M REPO_BEHIND_ORIGIN=<n|NA>
//	ACTION:<target>: <text>
//
// It also tracks `=== sync-check: <target> ===` section headers to set the
// active target scope, and captures per-file `agents/...` status lines
// (format: `  <path>: <STATUS>`) into [TargetVerdict.AgentFiles].
//
// Verdicts are returned in first-seen order.
func ParseSyncCheck(output string) []TargetVerdict {
	order := []string{}
	byTarget := map[string]*TargetVerdict{}

	get := func(name string) *TargetVerdict {
		v, ok := byTarget[name]
		if !ok {
			// RepoBehindOrigin/RepoBehindRelease default to RepoBehindOriginNA
			// until a VERDICT line explicitly sets them — mirrors the
			// backend's NA sentinel and avoids a defensive zero-value
			// collapsing into "0 behind".
			v = &TargetVerdict{Target: name, RepoBehindOrigin: RepoBehindOriginNA, RepoBehindRelease: RepoBehindOriginNA}
			byTarget[name] = v
			order = append(order, name)
		}
		return v
	}

	scanner := bufio.NewScanner(strings.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var currentTarget string // tracks the active section for per-file lines
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Section header: "=== sync-check: <target> (<path>) ==="
		if strings.HasPrefix(line, "=== sync-check: ") {
			rest := strings.TrimPrefix(line, "=== sync-check: ")
			currentTarget, _, _ = strings.Cut(rest, " ")
			continue
		}

		if rest, ok := strings.CutPrefix(line, "VERDICT:"); ok {
			// rest = "<target>:UPSTREAM_CHANGED=N OVERLAY_NOT_DEPLOYED=M"
			target, counts, ok := strings.Cut(rest, ":")
			if !ok {
				continue
			}
			v := get(target)
			for _, field := range strings.Fields(counts) {
				key, val, ok := strings.Cut(field, "=")
				if !ok {
					continue
				}
				switch key {
				case "UPSTREAM_CHANGED":
					n, _ := strconv.Atoi(val)
					v.UpstreamChanged = n
				case "OVERLAY_NOT_DEPLOYED":
					n, _ := strconv.Atoi(val)
					v.OverlayNotDeployed = n
				case "REPO_BEHIND_ORIGIN":
					// Dedicated NA pre-check: the field's literal "NA" sentinel
					// MUST be handled before strconv.Atoi. Falling through to a
					// generic `n, _ := strconv.Atoi(val)` (as UPSTREAM_CHANGED/
					// OVERLAY_NOT_DEPLOYED do) would discard the parse error and
					// silently collapse "NA" (or an omitted field) to Go's
					// zero-value 0 — indistinguishable from "confirmed 0 commits
					// behind" and reproducing the exact silent-healthy bug class
					// R-006 exists to eliminate.
					if val == "NA" {
						v.RepoBehindOrigin = RepoBehindOriginNA
					} else if n, err := strconv.Atoi(val); err == nil {
						v.RepoBehindOrigin = n
					}
				case "REPO_BEHIND_RELEASE":
					// Same dedicated NA pre-check as REPO_BEHIND_ORIGIN above,
					// for the same reason (D2 — a new field, same
					// silent-zero-collapse risk).
					if val == "NA" {
						v.RepoBehindRelease = RepoBehindOriginNA
					} else if n, err := strconv.Atoi(val); err == nil {
						v.RepoBehindRelease = n
					}
				case "RECORDED_VERSION":
					v.RecordedVersion = val
				case "DIGEST_MATCH":
					v.DigestMatch = val
				}
			}
			v.Status = classify(v.UpstreamChanged, v.OverlayNotDeployed, v.RepoBehindOrigin, v.RepoBehindRelease, v.DigestMatch == "no")
			continue
		}

		if rest, ok := strings.CutPrefix(line, "ACTION:"); ok {
			// rest = "<target>: <text>"
			target, text, ok := strings.Cut(rest, ":")
			if !ok {
				continue
			}
			v := get(target)
			v.Action = strings.TrimSpace(text)
			continue
		}

		// Per-file lines: capture agents/ file statuses within the current section.
		for _, s := range []string{"IN_SYNC", "OVERLAY_NOT_DEPLOYED", "UPSTREAM_CHANGED"} {
			if rest, ok := strings.CutPrefix(line, s+": "); ok {
				if strings.HasPrefix(rest, "agents/") && currentTarget != "" {
					path, _, _ := strings.Cut(rest, " ") // strip "(detail)" suffix
					v := get(currentTarget)
					v.AgentFiles = append(v.AgentFiles, AgentFileEntry{Path: path, Status: s})
				}
				break
			}
		}
	}

	result := make([]TargetVerdict, 0, len(order))
	for _, name := range order {
		result = append(result, *byTarget[name])
	}
	return result
}

// ProbeBehind extracts the launch-time origin-behind count from cached-only
// sync-check output. It reuses ParseSyncCheck (D4) instead of re-deriving
// REPO_BEHIND_ORIGIN detection, which stays owned by sync-check-verdicts —
// this capability only reads that published value. Any failure to determine
// a concrete count — zero verdicts (e.g. every target dir missing skips the
// VERDICT line), an explicit "NA", or unparseable/garbage output — degrades
// to RepoBehindOriginNA rather than Go's zero value, which would collapse
// into "confirmed 0 behind" (the exact R-006 bug class ParseSyncCheck
// already guards against for its own callers).
func ProbeBehind(output string) int {
	verdicts := ParseSyncCheck(output)
	if len(verdicts) == 0 {
		return RepoBehindOriginNA
	}
	return verdicts[0].RepoBehindOrigin
}

// ProbeBehindRelease is ProbeBehind's D2 counterpart: it extracts
// REPO_BEHIND_RELEASE instead of REPO_BEHIND_ORIGIN from the same
// cached-only sync-check output, with the identical degrade-to-NA contract
// (zero verdicts, explicit "NA", or unparseable output all resolve to
// RepoBehindOriginNA — the sentinel is reused, per D2, not redefined).
func ProbeBehindRelease(output string) int {
	verdicts := ParseSyncCheck(output)
	if len(verdicts) == 0 {
		return RepoBehindOriginNA
	}
	return verdicts[0].RepoBehindRelease
}
