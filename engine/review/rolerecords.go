package review

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gitobject"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/jsonstrict"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewresult"
)

// This file is the three records of the roles that come after the builder (own-rdd T6b2, S6 c): the sweep that
// `review sweep` stores, the report of the sweeper and the findings of a polisher judge. Each is a JSON object with its
// kind and its binding (base, head, diff_sha256) at the top, written like a result (encodeFile, decodeFile), named
// like one (SweepName, SweeperName, PolisherName) and held to git again by every reader (CheckRecordBinding). The
// engine checks their shape and what they bind and never judges what a sweeper or a polisher found (S6 h).

const (
	kindSweep    = "sweep"
	kindSweeper  = "sweeper"
	kindPolisher = "polisher"

	// OutcomeClean and OutcomeFired are the outcomes of a sweep: fired exactly when one of its lists is not empty.
	OutcomeClean = "clean"
	OutcomeFired = "fired"

	// sweepCommitSubject is what the subject of the commit of a sweeper starts with.
	sweepCommitSubject = "chore(sweep)"
)

// The dispositions a sweeper gives a candidate of a fired sweep.
var dispositions = []string{"removed", "corrected", "live", "finding"}

var (
	// ErrInvalidSweepFindings is findings that a sweep record cannot hold: a path, a name, a tool, a module or a
	// line with no text, a name with no place it was found, a check with no new line.
	ErrInvalidSweepFindings = errors.New("review: invalid sweep findings")
	// ErrInvalidSweeperReport is a report of the sweeper that is not the one S6 (c) describes, or that does not
	// answer for the sweep it names.
	ErrInvalidSweeperReport = errors.New("review: invalid sweeper report")
	// ErrInvalidPolisherFindings is findings of a polisher judge that are not the object S6 (c) describes.
	ErrInvalidPolisherFindings = errors.New("review: invalid polisher findings")
)

// RecordBinding is the binding a role record carries: the two commits and the digest of the diff between them. The
// paths of the diff follow from them and are not stored.
type RecordBinding struct {
	Base       string
	Head       string
	DiffSHA256 string
}

// CheckRecordBinding holds the binding a role record was stored with to c, the candidate git gives now, as a result is
// held (CheckBinding): the base, the head and the digest of the diff must be equal. The error wraps ErrBindingDiffers.
func CheckRecordBinding(b RecordBinding, c Candidate) error {
	return CheckBinding(reviewresult.Binding{Base: b.Base, Head: b.Head, DiffSHA256: b.DiffSHA256, Paths: c.Paths}, c)
}

// checkRecordBinding is what a stored record must hold before anything compares it with git.
func checkRecordBinding(kind, want string, b RecordBinding) error {
	switch {
	case kind != want:
		return fmt.Errorf("kind is %q, want %q", kind, want)
	case !gitobject.IsName(b.Base):
		return fmt.Errorf("base %q is not a full object name", b.Base)
	case !gitobject.IsName(b.Head):
		return fmt.Errorf("head %q is not a full object name", b.Head)
	case b.DiffSHA256 == "":
		return errors.New("it holds no diff_sha256")
	}
	return nil
}

// SweepT1 is what the first check found: the paths the diff deleted or renamed, and the names it removed that the
// tree of the head still mentions (RemovedNames, then a search of that tree outside the paths of the diff).
type SweepT1 struct {
	DeletedOrRenamed []string      `json:"deleted_or_renamed"`
	RemovedNames     []RemovedName `json:"removed_names"`
}

// RemovedName is a removed name and the paths of the head that still mention it.
type RemovedName struct {
	Name string   `json:"name"`
	Hits []string `json:"hits"`
}

// SweepT2 is the new lines one analyzer printed for one Go module or for the shell files (NewFindings), normalized.
type SweepT2 struct {
	Tool   string   `json:"tool"`
	Module string   `json:"module"`
	New    []string `json:"new"`
}

// SweepFindings are the findings of the three checks: T1 the removals the head still mentions, T2 the new lines of the
// analyzers, T3 the lines of `git status --porcelain` of the working tree when the sweep ran.
type SweepFindings struct {
	T1 SweepT1   `json:"t1"`
	T2 []SweepT2 `json:"t2"`
	T3 []string  `json:"t3"`
}

// Fired is whether any check found something.
func (f SweepFindings) Fired() bool {
	return len(f.T1.DeletedOrRenamed) > 0 || len(f.T1.RemovedNames) > 0 || len(f.T2) > 0 || len(f.T3) > 0
}

// check refuses findings that name nothing.
func (f SweepFindings) check() error {
	for _, p := range f.T1.DeletedOrRenamed {
		if p == "" {
			return fmt.Errorf("%w: t1 lists an empty path", ErrInvalidSweepFindings)
		}
	}
	for _, n := range f.T1.RemovedNames {
		if n.Name == "" || len(n.Hits) == 0 || slices.Contains(n.Hits, "") {
			return fmt.Errorf("%w: t1 lists the removed name %q with no text, or with no path that mentions it", ErrInvalidSweepFindings, n.Name)
		}
	}
	for _, c := range f.T2 {
		if c.Tool == "" || c.Module == "" || len(c.New) == 0 || slices.Contains(c.New, "") {
			return fmt.Errorf("%w: t2 lists the check of %q on %q with no tool, no module, no new line or an empty one", ErrInvalidSweepFindings, c.Tool, c.Module)
		}
	}
	if slices.Contains(f.T3, "") {
		return fmt.Errorf("%w: t3 lists an empty line", ErrInvalidSweepFindings)
	}
	return nil
}

// present is the findings with every list an array, never null, as they are stored.
func (f SweepFindings) present() SweepFindings {
	f.T1.DeletedOrRenamed = nonNil(f.T1.DeletedOrRenamed)
	f.T1.RemovedNames = nonNil(f.T1.RemovedNames)
	f.T2 = nonNil(f.T2)
	f.T3 = nonNil(f.T3)
	return f
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// The stored file of a sweep, as EncodeSweep writes it:
//
//	{"kind": "sweep", "base": "...", "head": "...", "diff_sha256": "...", "outcome": "clean"|"fired",
//	 "t1": {"deleted_or_renamed": ["..."], "removed_names": [{"name": "...", "hits": ["..."]}]},
//	 "t2": [{"tool": "...", "module": "...", "new": ["..."]}], "t3": ["..."]}
type sweepFile struct {
	Kind       string `json:"kind"`
	Base       string `json:"base"`
	Head       string `json:"head"`
	DiffSHA256 string `json:"diff_sha256"`
	Outcome    string `json:"outcome"`
	SweepFindings
}

// EncodeSweep is the content of the file that stores a sweep: the findings with the binding of c, and the outcome
// they make (fired when any list holds something). It refuses findings that name nothing (ErrInvalidSweepFindings).
func EncodeSweep(c Candidate, f SweepFindings) ([]byte, error) {
	if err := f.check(); err != nil {
		return nil, err
	}
	outcome := OutcomeClean
	if f.Fired() {
		outcome = OutcomeFired
	}
	return encodeFile(kindSweep, sweepFile{Kind: kindSweep, Base: c.Base, Head: c.Head, DiffSHA256: c.DiffSHA256, Outcome: outcome, SweepFindings: f.present()})
}

// StoredSweep is a stored sweep as DecodeSweep reads it back.
type StoredSweep struct {
	Binding  RecordBinding
	Outcome  string
	Findings SweepFindings
}

// Fired is whether the sweep fired.
func (s StoredSweep) Fired() bool { return s.Outcome == OutcomeFired }

// Candidates are what the sweeper of a fired sweep answers for, one for each finding and once each, in the order
// T1 paths, T1 names, T2, T3: "T1 path: <path>", "T1 name: <name>", "T2 <tool> <module>: <line>" and "T3: <line>".
func (s StoredSweep) Candidates() []string {
	var all []string
	for _, p := range s.Findings.T1.DeletedOrRenamed {
		all = append(all, "T1 path: "+p)
	}
	for _, n := range s.Findings.T1.RemovedNames {
		all = append(all, "T1 name: "+n.Name)
	}
	for _, c := range s.Findings.T2 {
		for _, line := range c.New {
			all = append(all, "T2 "+c.Tool+" "+c.Module+": "+line)
		}
	}
	for _, line := range s.Findings.T3 {
		all = append(all, "T3: "+line)
	}
	seen := map[string]bool{}
	unique := all[:0]
	for _, id := range all {
		if !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}
	return unique
}

// DecodeSweep reads a file that EncodeSweep wrote. It reads the file as it is: the binding is not recomputed from
// git (CheckRecordBinding compares it). A file that is not one EncodeSweep writes (not one JSON object, a key written
// twice, a member it does not have, another kind, a binding that names no commits, an outcome that is not the one
// the findings make, a list that is null or missing, findings that name nothing) is refused with an error that wraps
// ErrInvalidStored.
func DecodeSweep(data []byte) (StoredSweep, error) {
	var file sweepFile
	if err := decodeFile(data, "stored sweep", &file); err != nil {
		return StoredSweep{}, err
	}
	b := RecordBinding{Base: file.Base, Head: file.Head, DiffSHA256: file.DiffSHA256}
	err := checkRecordBinding(file.Kind, kindSweep, b)
	if err == nil {
		err = file.check()
	}
	if err == nil && (file.T1.DeletedOrRenamed == nil || file.T1.RemovedNames == nil || file.T2 == nil || file.T3 == nil) {
		err = errors.New("a list of the findings is missing or null")
	}
	if err == nil && file.Outcome != map[bool]string{true: OutcomeFired, false: OutcomeClean}[file.Fired()] {
		err = fmt.Errorf("outcome %q is not the one its findings make", file.Outcome)
	}
	if err != nil {
		return StoredSweep{}, fmt.Errorf("%w: stored sweep: %v", ErrInvalidStored, err)
	}
	return StoredSweep{Binding: b, Outcome: file.Outcome, Findings: file.SweepFindings}, nil
}

// SweeperCandidate is the answer of the sweeper about one candidate of the sweep.
type SweeperCandidate struct {
	Candidate   string `json:"candidate"`
	Disposition string `json:"disposition"`
	Evidence    string `json:"evidence"`
}

// SweeperReport is what the sweeper returns: the commit of its `chore(sweep)` change, or nil when it changed nothing,
// and its answer for each candidate.
type SweeperReport struct {
	Commit     *string            `json:"commit"`
	Candidates []SweeperCandidate `json:"candidates"`
}

// checkShape holds a report to the rules that need no sweep: a candidates array, a disposition of the enum, an
// evidence with text, and a commit that is a full object name or null.
func (r SweeperReport) checkShape() error {
	refuse := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalidSweeperReport, fmt.Sprintf(format, args...))
	}
	if r.Candidates == nil {
		return refuse("it has no candidates array")
	}
	if r.Commit != nil && !gitobject.IsName(*r.Commit) {
		return refuse("commit %q is neither null nor a full object name", *r.Commit)
	}
	for _, c := range r.Candidates {
		switch {
		case c.Candidate == "":
			return refuse("a candidate has no name")
		case !slices.Contains(dispositions, c.Disposition):
			return refuse("candidate %q has the disposition %q, want one of %s", c.Candidate, c.Disposition, strings.Join(dispositions, ", "))
		case strings.TrimSpace(c.Evidence) == "":
			return refuse("candidate %q has no evidence", c.Candidate)
		}
	}
	return nil
}

// ParseSweeperReport reads the report of the sweeper from the bytes it returned: one JSON object with a commit member
// (a full object name or null) and a candidates array of objects with a candidate, a disposition of removed,
// corrected, live or finding, and an evidence with text, and nothing else. A reply over MaxReplyBytes is refused with
// an error that wraps ErrReplyTooLarge, any other that is not the report with one that wraps ErrInvalidSweeperReport.
func ParseSweeperReport(data []byte) (SweeperReport, error) {
	if len(data) > MaxReplyBytes {
		return SweeperReport{}, fmt.Errorf("%w: %d bytes, at most %d", ErrReplyTooLarge, len(data), MaxReplyBytes)
	}
	var r SweeperReport
	err := jsonstrict.CheckNoDuplicateKeys(data)
	if err == nil {
		err = jsonstrict.DecodeStrict(data, "sweeper report", &r)
	}
	if err == nil {
		var members map[string]json.RawMessage
		if json.Unmarshal(data, &members) == nil {
			if _, ok := members["commit"]; !ok {
				err = errors.New(`it has no "commit" member`)
			}
		}
	}
	if err != nil {
		return SweeperReport{}, fmt.Errorf("%w: %v", ErrInvalidSweeperReport, err)
	}
	if err := r.checkShape(); err != nil {
		return SweeperReport{}, err
	}
	return r, nil
}

// CheckSweeperReport holds the report to the sweep it answers for: the sweep fired, the report answers for every
// candidate of it once and for no other, and a report with no commit removed and corrected nothing. The error wraps
// ErrInvalidSweeperReport and names the candidate.
func CheckSweeperReport(r SweeperReport, s StoredSweep) error {
	refuse := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalidSweeperReport, fmt.Sprintf(format, args...))
	}
	if !s.Fired() {
		return refuse("the sweep it answers for did not fire, so there is nothing for a sweeper to answer")
	}
	want := s.Candidates()
	var answered []string
	for _, c := range r.Candidates {
		switch {
		case !slices.Contains(want, c.Candidate):
			return refuse("candidate %q is not one of the sweep", c.Candidate)
		case slices.Contains(answered, c.Candidate):
			return refuse("candidate %q is answered more than once", c.Candidate)
		case r.Commit == nil && (c.Disposition == "removed" || c.Disposition == "corrected"):
			return refuse("candidate %q is %s but the report names no commit", c.Candidate, c.Disposition)
		}
		answered = append(answered, c.Candidate)
	}
	for _, id := range want {
		if !slices.Contains(answered, id) {
			return refuse("candidate %q of the sweep is not answered", id)
		}
	}
	return nil
}

// CheckSweepCommit holds the commit a report names to what S6 (c) asks of it, from the facts git gives about it: its
// first parent is head, the head the sweep bound, and its subject starts with "chore(sweep)". The error wraps
// ErrInvalidSweeperReport and names the commit.
func CheckSweepCommit(head, commit, parent, subject string) error {
	switch {
	case parent != head:
		return fmt.Errorf("%w: commit %s does not have %s as its first parent", ErrInvalidSweeperReport, commit, head)
	case !strings.HasPrefix(subject, sweepCommitSubject):
		return fmt.Errorf("%w: commit %s has the subject %q, which does not start %q", ErrInvalidSweeperReport, commit, subject, sweepCommitSubject)
	}
	return nil
}

// The stored file of the sweeper, as EncodeSweeper writes it: the binding, the name of the sweep it answers for,
// and its report.
//
//	{"kind": "sweeper", "base": "...", "head": "...", "diff_sha256": "...", "sweep": "<head12>-<n>.sweep.json",
//	 "commit": "<sha>"|null, "candidates": [{"candidate": "...", "disposition": "...", "evidence": "..."}]}
type sweeperFile struct {
	Kind       string `json:"kind"`
	Base       string `json:"base"`
	Head       string `json:"head"`
	DiffSHA256 string `json:"diff_sha256"`
	Sweep      string `json:"sweep"`
	SweeperReport
}

// EncodeSweeper is the content of the file that stores the report of the sweeper: the report with the binding of c and
// the name of the sweep it answers for (a name SweepName makes). It checks the report's shape, not its answer for the
// sweep (CheckSweeperReport) nor its commit (CheckSweepCommit), which the command has done.
func EncodeSweeper(c Candidate, sweep string, r SweeperReport) ([]byte, error) {
	if err := CheckSweepName(sweep); err != nil {
		return nil, err
	}
	r.Candidates = nonNil(r.Candidates)
	if err := r.checkShape(); err != nil {
		return nil, err
	}
	return encodeFile(kindSweeper, sweeperFile{Kind: kindSweeper, Base: c.Base, Head: c.Head, DiffSHA256: c.DiffSHA256, Sweep: sweep, SweeperReport: r})
}

// StoredSweeper is a stored report of the sweeper as DecodeSweeper reads it back.
type StoredSweeper struct {
	Binding RecordBinding
	Sweep   string
	Report  SweeperReport
}

// DecodeSweeper reads a file that EncodeSweeper wrote, as DecodeSweep reads a sweep: a file that is not one
// (not one JSON object, a key written twice, a member it does not have, another kind, a binding that names no
// commits, a sweep that is not a sweep name, a report that fails its shape) is refused with an error that wraps
// ErrInvalidStored.
func DecodeSweeper(data []byte) (StoredSweeper, error) {
	var file sweeperFile
	if err := decodeFile(data, "stored sweeper report", &file); err != nil {
		return StoredSweeper{}, err
	}
	b := RecordBinding{Base: file.Base, Head: file.Head, DiffSHA256: file.DiffSHA256}
	err := checkRecordBinding(file.Kind, kindSweeper, b)
	if err == nil {
		err = CheckSweepName(file.Sweep)
	}
	if err == nil {
		err = file.SweeperReport.checkShape()
	}
	if err != nil {
		return StoredSweeper{}, fmt.Errorf("%w: stored sweeper report: %v", ErrInvalidStored, err)
	}
	return StoredSweeper{Binding: b, Sweep: file.Sweep, Report: file.SweeperReport}, nil
}

// ParsePolisherFindings reads the findings of one polisher judge from the bytes it returned (roles/polisher/body.md):
// one JSON object with a findings array of objects, each with an evidence that is a non-empty array of non-empty
// strings. It checks nothing else: the severities are not pinned, so no finding blocks (S6 h). It returns the object
// as it was given. A reply over MaxReplyBytes is refused with an error that wraps ErrReplyTooLarge, any other that is
// not the object with one that wraps ErrInvalidPolisherFindings.
func ParsePolisherFindings(data []byte) (json.RawMessage, error) {
	if len(data) > MaxReplyBytes {
		return nil, fmt.Errorf("%w: %d bytes, at most %d", ErrReplyTooLarge, len(data), MaxReplyBytes)
	}
	refuse := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalidPolisherFindings, fmt.Sprintf(format, args...))
	}
	if err := jsonstrict.CheckNoDuplicateKeys(data); err != nil {
		return nil, refuse("%v", err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil || object == nil {
		return nil, refuse("it is not a JSON object")
	}
	var findings []map[string]json.RawMessage
	if raw, ok := object["findings"]; !ok || json.Unmarshal(raw, &findings) != nil || findings == nil {
		return nil, refuse(`its "findings" is missing or is not an array of objects`)
	}
	for i, finding := range findings {
		var evidence []string
		raw, ok := finding["evidence"]
		if finding == nil || !ok || json.Unmarshal(raw, &evidence) != nil || len(evidence) == 0 || slices.Contains(evidence, "") {
			return nil, refuse(`finding %d has no "evidence" that is a non-empty array of non-empty strings`, i+1)
		}
	}
	return json.RawMessage(bytes.TrimSpace(data)), nil
}

// The stored file of the polisher, as EncodePolisher writes it:
//
//	{"kind": "polisher", "base": "...", "head": "...", "diff_sha256": "...", "findings": {...the object of the judge...}}
type polisherFile struct {
	Kind       string          `json:"kind"`
	Base       string          `json:"base"`
	Head       string          `json:"head"`
	DiffSHA256 string          `json:"diff_sha256"`
	Findings   json.RawMessage `json:"findings"`
}

// EncodePolisher is the content of the file that stores the findings of one polisher judge: the object as it was
// given, which ParsePolisherFindings must accept, with the binding of c.
func EncodePolisher(c Candidate, findings []byte) ([]byte, error) {
	object, err := ParsePolisherFindings(findings)
	if err != nil {
		return nil, err
	}
	return encodeFile(kindPolisher, polisherFile{Kind: kindPolisher, Base: c.Base, Head: c.Head, DiffSHA256: c.DiffSHA256, Findings: object})
}

// StoredPolisher is stored polisher findings as DecodePolisher reads them back.
type StoredPolisher struct {
	Binding  RecordBinding
	Findings []byte
}

// DecodePolisher reads a file that EncodePolisher wrote, as DecodeSweep reads a sweep: a file that is not one, or
// whose findings ParsePolisherFindings refuses, is refused with an error that wraps ErrInvalidStored.
func DecodePolisher(data []byte) (StoredPolisher, error) {
	var file polisherFile
	if err := decodeFile(data, "stored polisher findings", &file); err != nil {
		return StoredPolisher{}, err
	}
	b := RecordBinding{Base: file.Base, Head: file.Head, DiffSHA256: file.DiffSHA256}
	err := checkRecordBinding(file.Kind, kindPolisher, b)
	if err == nil {
		_, err = ParsePolisherFindings(file.Findings)
	}
	if err != nil {
		return StoredPolisher{}, fmt.Errorf("%w: stored polisher findings: %v", ErrInvalidStored, err)
	}
	return StoredPolisher{Binding: b, Findings: file.Findings}, nil
}
