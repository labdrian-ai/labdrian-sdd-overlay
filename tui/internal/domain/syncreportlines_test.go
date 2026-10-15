package domain

// What the lines of a sync-check report add up to. The adapter reads the lines
// (a line is only what the backend printed, typed); this is the rule that makes
// the verdicts of them, which the program applies across all the invocations of
// an action. Its cases are the ones the reader of the whole transcript had: the
// old text parser is kept in package overlaycli as the specification they are
// checked against there, line for line.

import (
	"reflect"
	"strings"
	"testing"
)

func intp(n int) *int { return &n }

func strp(s string) *string { return &s }

func sectionOf(target string) SyncLine { return SyncLine{Kind: SyncSectionLine, Target: target} }

func verdictOf(target string, fields VerdictFields) SyncLine {
	return SyncLine{Kind: SyncVerdictLine, Target: target, Fields: fields}
}

func remedyOf(target, text string) SyncLine {
	return SyncLine{Kind: SyncActionLine, Target: target, Remedy: text}
}

func fileOf(path, status string) SyncLine {
	return SyncLine{Kind: SyncAgentFileLine, File: AgentFileEntry{Path: path, Status: status}}
}

func targetsOf(verdicts []TargetVerdict) string {
	var names []string
	for _, v := range verdicts {
		names = append(names, v.Target)
	}
	return strings.Join(names, ",")
}

func TestSyncReport_NothingReadHasNoVerdictAndASectionAloneNamesNone(t *testing.T) {
	var report SyncReport
	if got := report.Verdicts(); got == nil || len(got) != 0 {
		t.Errorf("an empty report has Verdicts() = %#v, want an empty one", got)
	}

	report.Add([]SyncLine{sectionOf("claude"), fileOf("agents/A.md", "IN_SYNC")})
	if got := report.Verdicts(); len(got) != 1 || got[0].Target != "claude" {
		// An agent file line names the section it is in, which makes a verdict of
		// it, as the backend has always had it; a section header by itself does not.
		t.Errorf("verdicts = %+v, want the one the file line named", got)
	}

	var headerOnly SyncReport
	headerOnly.Add([]SyncLine{sectionOf("claude")})
	if got := headerOnly.Verdicts(); len(got) != 0 {
		t.Errorf("a section header made the verdicts %+v, want none: it names a target but gives nothing", got)
	}
}

func TestSyncReport_AVerdictLineGivesOnlyTheFieldsItNames(t *testing.T) {
	var report SyncReport
	report.Add([]SyncLine{verdictOf("claude", VerdictFields{
		UpstreamChanged: intp(3), RepoBehindOrigin: intp(5), RecordedVersion: strp("v1.0.0"), DigestMatch: strp("yes"),
	})})

	v := report.Verdicts()[0]
	want := TargetVerdict{
		Target: "claude", UpstreamChanged: 3, RepoBehindOrigin: 5, RepoBehindRelease: RepoBehindOriginNA,
		RecordedVersion: "v1.0.0", DigestMatch: "yes", Status: SyncNeedsCapture,
	}
	if !reflect.DeepEqual(v, want) {
		t.Errorf("verdict = %+v, want %+v: a count the line did not give is unknown (NA), not zero", v, want)
	}
}

// A target named again is the same verdict: it keeps its place and the later
// line updates the fields it names, over the ones an earlier line gave.
func TestSyncReport_ATargetNamedAgainIsTheSameVerdictUpdatedByTheFieldsTheLaterLineGives(t *testing.T) {
	var report SyncReport
	report.Add([]SyncLine{
		verdictOf("claude", VerdictFields{OverlayNotDeployed: intp(1), RepoBehindRelease: intp(2), RecordedVersion: strp("v1.0.0")}),
		remedyOf("claude", "apply it"),
		verdictOf("codex", VerdictFields{}),
	})
	report.Add([]SyncLine{
		verdictOf("claude", VerdictFields{OverlayNotDeployed: intp(0), UpstreamChanged: intp(4)}),
		remedyOf("claude", "capture it"),
	})

	verdicts := report.Verdicts()
	if got := targetsOf(verdicts); got != "claude,codex" {
		t.Fatalf("verdicts of %q, want claude (once, where first seen) then codex", got)
	}
	claude := verdicts[0]
	if claude.UpstreamChanged != 4 || claude.OverlayNotDeployed != 0 || claude.Action != "capture it" {
		t.Errorf("claude = %+v, want the fields the later lines gave", claude)
	}
	if claude.RecordedVersion != "v1.0.0" || claude.RepoBehindRelease != 2 {
		t.Errorf("claude = %+v, want the fields the later line did not name to be what they were", claude)
	}
	if claude.Status != SyncNeedsCapture {
		t.Errorf("claude.Status = %d, want it ranked again from the merged fields (upstream changed outranks the release)", claude.Status)
	}
}

// Targets come in the order they were first named, by whatever line named them.
func TestSyncReport_VerdictsComeInTheOrderTargetsWereFirstNamed(t *testing.T) {
	var report SyncReport
	report.Add([]SyncLine{
		remedyOf("pi", "a remedy before any verdict"),
		verdictOf("claude", VerdictFields{}),
		fileOf("agents/A.md", "IN_SYNC"), // no section is open: it names no one
		sectionOf("codex"), fileOf("agents/B.md", "IN_SYNC"),
		verdictOf("pi", VerdictFields{}),
		verdictOf("claude", VerdictFields{}),
	})

	if got := targetsOf(report.Verdicts()); got != "pi,claude,codex" {
		t.Errorf("verdicts of %q, want pi, claude, codex: the order each was first named", got)
	}
}

// A line that gives a status-relevant field ranks the verdict again; a line
// that does not leaves its status as the last VERDICT line made it.
func TestSyncReport_TheStatusIsRankedAgainWhenAVerdictLineArrives(t *testing.T) {
	var report SyncReport
	report.Add([]SyncLine{verdictOf("claude", VerdictFields{DigestMatch: strp(DigestMismatch)})})
	if got := report.Verdicts()[0].Status; got != SyncNeedsApply {
		t.Fatalf("a digest that no longer matches ranks %d, want SyncNeedsApply", got)
	}

	report.Add([]SyncLine{remedyOf("claude", "something")})
	if got := report.Verdicts()[0].Status; got != SyncNeedsApply {
		t.Errorf("an action line changed the status to %d; only a VERDICT line ranks it", got)
	}

	report.Add([]SyncLine{verdictOf("claude", VerdictFields{DigestMatch: strp("yes")})})
	if got := report.Verdicts()[0].Status; got != SyncHealthy {
		t.Errorf("the digest matching again ranks %d, want SyncHealthy", got)
	}

	var unranked SyncReport
	unranked.Add([]SyncLine{remedyOf("pi", "a remedy and no verdict")})
	if got := unranked.Verdicts()[0].Status; got != SyncUnknown {
		t.Errorf("a target no VERDICT line named has status %d, want SyncUnknown: nothing was said about it", got)
	}
}

// An agent file belongs to the section last opened -- even if the line that
// opened it came before, in an earlier Add -- and to no one before any section.
func TestSyncReport_AnAgentFileBelongsToTheSectionLastOpened(t *testing.T) {
	var report SyncReport
	report.Add([]SyncLine{
		fileOf("agents/LOST.md", "IN_SYNC"), // no section is open yet
		sectionOf("claude"), verdictOf("claude", VerdictFields{}),
	})
	report.Add([]SyncLine{
		fileOf("agents/CARRIED.md", "OVERLAY_NOT_DEPLOYED"), // claude's section is still open
		sectionOf("codex"), verdictOf("codex", VerdictFields{}), fileOf("agents/OWN.md", "UPSTREAM_CHANGED"),
	})

	verdicts := report.Verdicts()
	if got := targetsOf(verdicts); got != "claude,codex" {
		t.Fatalf("verdicts of %q, want claude and codex", got)
	}
	if got, want := verdicts[0].AgentFiles, []AgentFileEntry{{Path: "agents/CARRIED.md", Status: "OVERLAY_NOT_DEPLOYED"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("claude.AgentFiles = %+v, want %+v", got, want)
	}
	if got, want := verdicts[1].AgentFiles, []AgentFileEntry{{Path: "agents/OWN.md", Status: "UPSTREAM_CHANGED"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("codex.AgentFiles = %+v, want %+v", got, want)
	}
}

// A section with no name closes the one before it: the files after it are no one's.
func TestSyncReport_ASectionWithNoNameLeavesTheFilesThatFollowItWithNoOne(t *testing.T) {
	var report SyncReport
	report.Add([]SyncLine{sectionOf("claude"), verdictOf("claude", VerdictFields{}), sectionOf(""), fileOf("agents/A.md", "IN_SYNC")})

	if files := report.Verdicts()[0].AgentFiles; len(files) != 0 {
		t.Errorf("claude.AgentFiles = %+v, want none: the section that followed it had no target", files)
	}
}

// The verdicts a report gives are its state at that moment, and theirs: reading
// more lines does not change a slice already handed out, and changing one does
// not change the report.
func TestSyncReport_VerdictsAreCopiesOfTheReportsState(t *testing.T) {
	var report SyncReport
	report.Add([]SyncLine{sectionOf("claude"), verdictOf("claude", VerdictFields{}), fileOf("agents/A.md", "IN_SYNC")})

	scribbled := report.Verdicts()
	scribbled[0].AgentFiles[0].Path = "agents/CHANGED.md"
	if got := report.Verdicts()[0].AgentFiles[0].Path; got != "agents/A.md" {
		t.Errorf("changing a verdict handed out changed the report: its first file is now %q", got)
	}

	given := report.Verdicts()
	report.Add([]SyncLine{fileOf("agents/B.md", "IN_SYNC"), verdictOf("claude", VerdictFields{UpstreamChanged: intp(9)})})
	if v := given[0]; v.UpstreamChanged != 0 || len(v.AgentFiles) != 1 {
		t.Errorf("a verdict already given became %+v", v)
	}
}
