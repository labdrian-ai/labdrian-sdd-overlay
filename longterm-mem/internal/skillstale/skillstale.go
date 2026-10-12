// Package skillstale reports project-tier procedural skills whose instructions
// no longer agree with the repository. It is deliberately report-only: the
// detector never writes a skill, lock file, registry, or Engram record.
//
// The detector is a function of what it is handed: the project lock and the skill
// files (SkillSource), the candidate observations (ObservationLister), what the
// repository says of the paths a skill names (PathFactSource), whether a command is
// where it would be run (CommandResolver) and the time. It reads no file, no
// environment variable and no clock, and starts no process; the composition root
// chooses the adapters that answer.
package skillstale

import (
	"fmt"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/staleness"
)

// QuietAfter is how long a candidate can go unobserved before its skill is reported quiet.
const QuietAfter = 180 * 24 * time.Hour

// SignalKind names one report-only retirement signal.
type SignalKind string

const (
	SignalRemoved           SignalKind = "REMOVED"
	SignalMoved             SignalKind = "MOVED"
	SignalUnresolvedCommand SignalKind = "UNRESOLVED-COMMAND"
	SignalQuiet             SignalKind = "QUIET"
	SignalSuperseded        SignalKind = "SUPERSEDED"
)

// Signal is one concrete piece of evidence for a skill finding.
type Signal struct {
	Kind    SignalKind
	Path    string
	NewPath string
	Commit  string
	Command string
	Since   string
	By      string
}

// Finding groups the evidence for one project-tier skill. A moved path is
// retained as an informational finding, but it is never also classified as a
// removal; the distinction is the safety boundary inherited from staleness.
type Finding struct {
	SkillID   string
	Candidate string
	Status    string
	Signals   []Signal
}

// ObservationLister is the port through which the detector reads the candidate
// records of a project: its live (not soft-deleted) observations. The detector
// owns this port and the memory model it speaks; the store that serves it (the
// Engram adapter in production) is wired by the caller.
type ObservationLister interface {
	ListObservations(project string) ([]memory.Observation, error)
}

// Config supplies the detector's inputs; every one is required. Source,
// Observations, Facts and Commands are the ports it reads through, and Now is the
// instant it judges quietness by. SupersededBy is optional evidence from the
// runtime that performed the global MatchCandidate lookup; the detector
// records it but never performs or triggers the retirement action itself.
type Config struct {
	// Project names the project whose candidate observations are listed.
	Project      string
	Source       SkillSource
	Observations ObservationLister
	Facts        PathFactSource
	Commands     CommandResolver
	Now          time.Time
	SupersededBy map[string]string
}

// validate refuses a Config that lacks an input, in the detector's own words. A port that is a nil pointer in an
// interface (a store that failed to open and was assigned anyway) is missing too.
func (cfg Config) validate() error {
	switch {
	case memory.IsMissing(cfg.Source):
		return fmt.Errorf("skillstale: skill source is required")
	case memory.IsMissing(cfg.Observations):
		return fmt.Errorf("skillstale: observation lister is required")
	case memory.IsMissing(cfg.Facts):
		return fmt.Errorf("skillstale: path fact source is required")
	case memory.IsMissing(cfg.Commands):
		return fmt.Errorf("skillstale: command resolver is required")
	case cfg.Now.IsZero():
		return fmt.Errorf("skillstale: the time to judge quietness by is required")
	}
	return nil
}

// Detect reads the project lock, the first target of each entry and the
// candidate observations for the requested project, all through its ports. It
// classifies the paths a skill names through the PathFactSource, resolves the
// commands in its fenced blocks through the CommandResolver, and parses
// LastObserved/Status from the matching TopicKey record. The first failure ends
// the run; nothing in this function writes anywhere.
func Detect(cfg Config) ([]Finding, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	lock, err := cfg.readLock()
	if err != nil {
		return nil, err
	}
	observations, err := cfg.Observations.ListObservations(cfg.Project)
	if err != nil {
		return nil, fmt.Errorf("skillstale: list candidate observations: %w", err)
	}
	candidates := latestCandidates(observations)

	findings := make([]Finding, 0, len(lock.Skills))
	for _, entry := range lock.Skills {
		finding, err := cfg.inspect(entry, candidates)
		if err != nil {
			return nil, err
		}
		if len(finding.Signals) > 0 {
			findings = append(findings, finding)
		}
	}
	return findings, nil
}

func (cfg Config) readLock() (ProjectLock, error) {
	data, err := cfg.Source.ReadLock()
	if err != nil {
		return ProjectLock{}, fmt.Errorf("skillstale: read %s: %w", ProjectLockRelPath, err)
	}
	return ParseProjectLock(data)
}

// inspect gathers the evidence for one lock entry, in the order a report lists it: the quiet candidate, the
// paths that are gone or moved, the commands that cannot be found, and the skill that replaces it.
func (cfg Config) inspect(entry ProjectLockEntry, candidates map[string]candidateRecord) (Finding, error) {
	body, err := cfg.readBody(entry)
	if err != nil {
		return Finding{}, fmt.Errorf("skillstale: %s: %w", entry.ID, err)
	}
	// The facts are asked for even when the skill names no path, as they always have been: the adapter reads
	// the project to answer, and a project it cannot read fails the report whatever the skill names.
	paths := staleness.ReferencedPaths(body)
	facts, err := cfg.Facts.PathFacts(paths)
	if err != nil {
		return Finding{}, fmt.Errorf("skillstale: classify paths for %s: %w", entry.ID, err)
	}

	finding := Finding{SkillID: entry.ID, Candidate: entry.Candidate}
	if candidate, ok := candidates[entry.Candidate]; ok {
		finding.Status = candidate.status
		if signal, quiet := cfg.quietSignal(candidate); quiet {
			finding.Signals = append(finding.Signals, signal)
		}
	}
	finding.Signals = append(finding.Signals, pathSignals(paths, facts)...)
	finding.Signals = append(finding.Signals, cfg.commandSignals(body)...)
	if by := supersededBy(entry, cfg.SupersededBy); by != "" {
		finding.Signals = append(finding.Signals, Signal{Kind: SignalSuperseded, By: by})
	}
	return finding, nil
}

// readBody reads the first target of the entry, as all of its targets hold the same bytes, and returns the
// skill's text below its front matter.
func (cfg Config) readBody(entry ProjectLockEntry) (string, error) {
	if len(entry.Targets) == 0 {
		return "", fmt.Errorf("lock entry has no targets")
	}
	data, err := cfg.Source.ReadSkill(entry.Targets[0])
	if err != nil {
		return "", err
	}
	return skillBody(data), nil
}

// quietSignal says the candidate has not been observed for longer than QuietAfter before Now. A record with no
// LastObserved, or one that is not a time, says nothing.
func (cfg Config) quietSignal(candidate candidateRecord) (Signal, bool) {
	if candidate.lastObserved == "" {
		return Signal{}, false
	}
	last, ok := parseTimestamp(candidate.lastObserved)
	if !ok || cfg.Now.Sub(last) <= QuietAfter {
		return Signal{}, false
	}
	return Signal{Kind: SignalQuiet, Since: candidate.lastObserved}, true
}

// pathSignals reports, for the paths a skill names, those the repository says were deleted or moved.
func pathSignals(paths []string, facts map[string]PathFact) []Signal {
	var signals []Signal
	for _, referenced := range paths {
		fact := facts[referenced]
		switch fact.State {
		case PathDeleted:
			// Unlike memory staleness, skill instructions are not records
			// of the deletion. A deleted reference is a defect regardless
			// of which side of the deletion LastObserved falls on.
			signals = append(signals, Signal{Kind: SignalRemoved, Path: referenced, Commit: fact.Commit})
		case PathRenamed:
			signals = append(signals, Signal{Kind: SignalMoved, Path: referenced, NewPath: fact.NewPath, Commit: fact.Commit})
		}
	}
	return signals
}

// commandSignals reports each command the skill's fenced blocks run that the resolver cannot find, once however
// often it is named.
func (cfg Config) commandSignals(body string) []Signal {
	var signals []Signal
	seen := map[string]bool{}
	for _, command := range fencedCommands(body) {
		if command == "" || seen[command] {
			continue
		}
		seen[command] = true
		if !cfg.Commands.ResolvesCommand(command) {
			signals = append(signals, Signal{Kind: SignalUnresolvedCommand, Command: command})
		}
	}
	return signals
}

func supersededBy(entry ProjectLockEntry, values map[string]string) string {
	if values == nil {
		return ""
	}
	if by := values[entry.ID]; by != "" {
		return by
	}
	return values[entry.Candidate]
}
