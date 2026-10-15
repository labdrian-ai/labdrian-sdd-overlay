package domain

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
	// two verdicts — see Classify's precedence and R-006). Tracks main, not
	// HEAD, because self-update (the source of truth this mirrors) only ever
	// converges main, never the checked-out branch.
	SyncBehindOrigin
	// SyncBehindRelease means local main is behind the newest locally-known
	// release tag (D2, R-011): the primary "you should self-update" signal
	// once at least one release tag exists. Outranks SyncBehindOrigin in
	// Classify's precedence — a target's own release standing is more
	// actionable than raw untagged origin drift.
	SyncBehindRelease
)

// RepoBehindOriginNA is the sentinel value for TargetVerdict.RepoBehindOrigin
// when the origin comparison is unavailable (no remote, no cached ref, or a
// requested fetch failed) — distinct from a confirmed 0 commits behind. A
// single named constant (instead of a magic -1 repeated at each use site)
// keeps every producer/consumer of this field agreeing by construction.
const RepoBehindOriginNA = -1

// TargetVerdict holds the sync-check result for one target, as the adapter
// read it from the backend's report.
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

// Classify maps verdict counts to a color status.
//
// Priority (matches the backend ACTION precedence, extended by R-006/D2):
//   - UPSTREAM_CHANGED > 0                         -> RED    (gentle-ai synced, needs capture+apply)
//   - OVERLAY_NOT_DEPLOYED > 0 OR digest mismatch  -> YELLOW (needs apply — a stale digest means the
//     deployed files no longer match what was recorded at the last apply, same remedy as a raw file diff)
//   - REPO_BEHIND_RELEASE > 0                      -> behind-release (D2: primary "self-update" signal
//     once a release tag exists; outranks raw origin drift)
//   - REPO_BEHIND_ORIGIN > 0                       -> behind-origin (never silently healthy)
//   - otherwise                                    -> GREEN (healthy)
//
// The adapter that reads a report calls it for each verdict: what the counts
// mean for the operator is the domain's rule, and how the backend spells them
// is not.
func Classify(upstreamChanged, overlayNotDeployed, repoBehindOrigin, repoBehindRelease int, digestMismatch bool) SyncStatus {
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

// RepoBehind is how far the repository is behind, as one sync-check report
// says it (the launch-time probe, D4 and D2). A count the report does not give
// is RepoBehindOriginNA, which is not the same as "up to date".
type RepoBehind struct {
	// Origin is how many commits local main is behind origin/main.
	Origin int
	// Release is how many commits local main is behind the newest
	// locally-known release tag.
	Release int
}

// BehindOf reads how far the repository is behind from the verdicts of one
// sync-check report. Both counts are the repository's, not a target's: the
// backend works them out once and prints them on every VERDICT line, so the
// first verdict stands for the repository and the two numbers come from the
// same one.
//
// Any failure to determine a count -- no verdict at all (e.g. every target dir
// missing skips the VERDICT line, or the report could not be read) or an
// explicit "NA" -- degrades to RepoBehindOriginNA rather than Go's zero value,
// which would collapse into "confirmed 0 behind" (the exact R-006 bug class
// the reader of a report already guards against).
func BehindOf(verdicts []TargetVerdict) RepoBehind {
	if len(verdicts) == 0 {
		return RepoBehind{Origin: RepoBehindOriginNA, Release: RepoBehindOriginNA}
	}
	return RepoBehind{Origin: verdicts[0].RepoBehindOrigin, Release: verdicts[0].RepoBehindRelease}
}
