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

// DigestMismatch is the value of TargetVerdict.DigestMatch that says the
// deployed files no longer match what was recorded at the last apply: it counts
// for the status of a verdict (Classify) like a file that is not deployed.
const DigestMismatch = "no"

// SyncLineKind says what a line of a sync-check report was about.
type SyncLineKind int

const (
	// SyncSectionLine opens the section of a target: the agent file lines that
	// follow belong to it, until another section opens. It names a target but
	// is no verdict of it.
	SyncSectionLine SyncLineKind = iota
	// SyncVerdictLine gives the counts and versions of a target.
	SyncVerdictLine
	// SyncActionLine gives the remedy the backend offers for a target.
	SyncActionLine
	// SyncAgentFileLine gives the status of an agent file, in the section last
	// opened.
	SyncAgentFileLine
)

// VerdictFields are the fields a VERDICT line gave. A nil field is one the line
// did not give (or gave in a form the adapter could not read), and leaves what
// an earlier line gave.
type VerdictFields struct {
	UpstreamChanged    *int
	OverlayNotDeployed *int
	RepoBehindOrigin   *int
	RepoBehindRelease  *int
	RecordedVersion    *string
	DigestMatch        *string
}

// SyncLine is one line of a sync-check report that said something, as the
// adapter read it. The adapter knows how the backend spells a line; what the
// lines add up to is the domain's (SyncReport).
type SyncLine struct {
	Kind SyncLineKind
	// Target is the target a section, verdict or action line is about.
	Target string
	// Fields are what a verdict line gave.
	Fields VerdictFields
	// Remedy is what an action line offers.
	Remedy string
	// File is the agent file an agent file line gave the status of.
	File AgentFileEntry
}

// SyncReport adds up the lines of a sync-check report into the verdict of each
// target. It reads the lines of as many outputs as an action produced, in the
// order they were produced, as ONE report, which is how the operator has always
// been shown them: a target named again is the same verdict (it keeps the place
// where it was first seen, and a later line updates the fields it gives), and
// an agent file line belongs to the section last opened, even in an earlier
// output. The zero value is an empty report.
type SyncReport struct {
	order    []string
	byTarget map[string]*TargetVerdict
	section  string
}

// Add reads lines, which come after the ones already read.
func (r *SyncReport) Add(lines []SyncLine) {
	for _, line := range lines {
		switch line.Kind {
		case SyncSectionLine:
			r.section = line.Target
		case SyncVerdictLine:
			v := r.verdict(line.Target)
			line.Fields.applyTo(v)
			v.Status = Classify(v.UpstreamChanged, v.OverlayNotDeployed, v.RepoBehindOrigin, v.RepoBehindRelease, v.DigestMatch == DigestMismatch)
		case SyncActionLine:
			r.verdict(line.Target).Action = line.Remedy
		case SyncAgentFileLine:
			if r.section != "" {
				v := r.verdict(r.section)
				v.AgentFiles = append(v.AgentFiles, line.File)
			}
		}
	}
}

// Verdicts are the verdicts of the targets the lines named, in the order they
// were first named; none when no line named one. They are the report's state
// at the time of the call: later lines do not change them.
func (r *SyncReport) Verdicts() []TargetVerdict {
	out := make([]TargetVerdict, 0, len(r.order))
	for _, name := range r.order {
		v := *r.byTarget[name]
		v.AgentFiles = append([]AgentFileEntry(nil), v.AgentFiles...)
		out = append(out, v)
	}
	return out
}

// verdict is the verdict of target, which a line naming it makes if it was not
// there. RepoBehindOrigin and RepoBehindRelease are RepoBehindOriginNA until a
// line gives them -- mirroring the backend's NA sentinel and keeping a verdict
// nobody gave counts for from collapsing into "0 behind".
func (r *SyncReport) verdict(target string) *TargetVerdict {
	if v, ok := r.byTarget[target]; ok {
		return v
	}
	if r.byTarget == nil {
		r.byTarget = map[string]*TargetVerdict{}
	}
	v := &TargetVerdict{Target: target, RepoBehindOrigin: RepoBehindOriginNA, RepoBehindRelease: RepoBehindOriginNA}
	r.byTarget[target] = v
	r.order = append(r.order, target)
	return v
}

// applyTo sets on v the fields f gave.
func (f VerdictFields) applyTo(v *TargetVerdict) {
	if f.UpstreamChanged != nil {
		v.UpstreamChanged = *f.UpstreamChanged
	}
	if f.OverlayNotDeployed != nil {
		v.OverlayNotDeployed = *f.OverlayNotDeployed
	}
	if f.RepoBehindOrigin != nil {
		v.RepoBehindOrigin = *f.RepoBehindOrigin
	}
	if f.RepoBehindRelease != nil {
		v.RepoBehindRelease = *f.RepoBehindRelease
	}
	if f.RecordedVersion != nil {
		v.RecordedVersion = *f.RecordedVersion
	}
	if f.DigestMatch != nil {
		v.DigestMatch = *f.DigestMatch
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
