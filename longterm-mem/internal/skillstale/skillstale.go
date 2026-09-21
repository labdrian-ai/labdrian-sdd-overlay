// Package skillstale reports project-tier procedural skills whose instructions
// no longer agree with the repository. It is deliberately report-only: the
// detector never writes a skill, lock file, registry, or Engram record.
package skillstale

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/repohistory"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/staleness"
)

const (
	ProjectLockRelPath = ".labdrian/procedural-skills.lock.json"
	QuietAfter         = 180 * 24 * time.Hour
)

// ProjectLock is the read-only detector view of the engine's project lock.
// The fields intentionally mirror engine/skills.ProjectLock without creating a
// module dependency in either direction.
type ProjectLock struct {
	Version int                `json:"version"`
	Skills  []ProjectLockEntry `json:"skills"`
}

// ProjectLockEntry identifies one registered project skill and its target
// files. The detector reads only the first target, as all targets receive the
// same stamped bytes during registration.
type ProjectLockEntry struct {
	ID         string   `json:"id"`
	Provenance string   `json:"provenance"`
	Candidate  string   `json:"candidate"`
	SHA256     string   `json:"sha256"`
	Revision   int      `json:"revision"`
	Targets    []string `json:"targets"`
}

// ParseProjectLock strictly parses the shared project-lock format. Missing
// files are handled by Detect as an error; this function only parses bytes it
// was given and refuses unknown fields, trailing values, unsupported versions,
// and duplicate ids.
func ParseProjectLock(data []byte) (ProjectLock, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var lock ProjectLock
	if err := decoder.Decode(&lock); err != nil {
		return ProjectLock{}, fmt.Errorf("parse project lock: %w", err)
	}
	if err := decoder.Decode(new(json.RawMessage)); err != io.EOF {
		return ProjectLock{}, fmt.Errorf("parse project lock: trailing data after the lock value")
	}
	if lock.Version != 1 {
		return ProjectLock{}, fmt.Errorf("parse project lock: unsupported version %d, want 1", lock.Version)
	}
	seen := make(map[string]bool, len(lock.Skills))
	for _, skill := range lock.Skills {
		if seen[skill.ID] {
			return ProjectLock{}, fmt.Errorf("parse project lock: duplicate skill id %q", skill.ID)
		}
		seen[skill.ID] = true
	}
	return lock, nil
}

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

// Config supplies the detector's read-only inputs. Now and PathEnv are
// injectable for deterministic tests. SupersededBy is optional evidence from
// the runtime that performed the global MatchCandidate lookup; the detector
// records it but never performs or triggers the retirement action itself.
type Config struct {
	ProjectRoot  string
	Project      string
	Store        *engram.Store
	Now          time.Time
	PathEnv      string
	SupersededBy map[string]string
}

// Detect reads the project lock, the first SKILL.md target for each entry and
// the candidate observations for the requested project. It classifies named
// paths through staleness.ClassifyPaths, scans fenced commands with os.Stat on
// PATH, and parses LastObserved/Status from the matching TopicKey record.
// Nothing in this function writes to disk or to Engram.
func Detect(cfg Config) ([]Finding, error) {
	if !filepath.IsAbs(cfg.ProjectRoot) {
		return nil, fmt.Errorf("skillstale: project root must be absolute: %q", cfg.ProjectRoot)
	}
	if cfg.Store == nil {
		return nil, fmt.Errorf("skillstale: Engram store is required")
	}

	lockData, err := os.ReadFile(filepath.Join(cfg.ProjectRoot, ProjectLockRelPath))
	if err != nil {
		return nil, fmt.Errorf("skillstale: read %s: %w", ProjectLockRelPath, err)
	}
	lock, err := ParseProjectLock(lockData)
	if err != nil {
		return nil, err
	}
	observations, err := cfg.Store.ListObservations(cfg.Project)
	if err != nil {
		return nil, fmt.Errorf("skillstale: list candidate observations: %w", err)
	}
	candidates := latestCandidates(observations)
	now := cfg.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	pathEnv := cfg.PathEnv
	if pathEnv == "" {
		pathEnv = os.Getenv("PATH")
	}

	findings := make([]Finding, 0, len(lock.Skills))
	for _, entry := range lock.Skills {
		data, err := readFirstTarget(cfg.ProjectRoot, entry)
		if err != nil {
			return nil, fmt.Errorf("skillstale: %s: %w", entry.ID, err)
		}
		body := skillBody(data)
		paths := staleness.ReferencedPaths(body)
		facts, err := staleness.ClassifyPaths(cfg.ProjectRoot, paths)
		if err != nil {
			return nil, fmt.Errorf("skillstale: classify paths for %s: %w", entry.ID, err)
		}

		finding := Finding{
			SkillID:   entry.ID,
			Candidate: entry.Candidate,
		}
		if candidate, ok := candidates[entry.Candidate]; ok {
			finding.Status = candidate.status
			if candidate.lastObserved != "" {
				if last, ok := parseTimestamp(candidate.lastObserved); ok && now.Sub(last) > QuietAfter {
					finding.Signals = append(finding.Signals, Signal{Kind: SignalQuiet, Since: candidate.lastObserved})
				}
			}
		}

		for _, referenced := range paths {
			fact := facts[referenced]
			switch fact.State {
			case repohistory.StateDeleted:
				// Unlike memory staleness, skill instructions are not records
				// of the deletion. A deleted reference is a defect regardless
				// of which side of the deletion LastObserved falls on.
				finding.Signals = append(finding.Signals, Signal{
					Kind: SignalRemoved, Path: referenced, Commit: fact.Commit,
				})
			case repohistory.StateRenamed:
				finding.Signals = append(finding.Signals, Signal{
					Kind: SignalMoved, Path: referenced, NewPath: fact.NewPath, Commit: fact.Commit,
				})
			}
		}

		seenCommands := map[string]bool{}
		for _, command := range fencedCommands(body) {
			if seenCommands[command] {
				continue
			}
			seenCommands[command] = true
			if !commandOnPath(command, pathEnv) {
				finding.Signals = append(finding.Signals, Signal{Kind: SignalUnresolvedCommand, Command: command})
			}
		}
		if by := supersededBy(entry, cfg.SupersededBy); by != "" {
			finding.Signals = append(finding.Signals, Signal{Kind: SignalSuperseded, By: by})
		}
		if len(finding.Signals) > 0 {
			findings = append(findings, finding)
		}
	}
	return findings, nil
}

type candidateRecord struct {
	status       string
	lastObserved string
	updatedAt    string
	id           int64
}

var (
	statusField       = regexp.MustCompile(`(?m)^\s*\*\*Status\*\*:\s*(\S+)\s*$`)
	lastObservedField = regexp.MustCompile(`(?m)^\s*\*\*LastObserved\*\*:\s*(\S+)\s*$`)
)

func latestCandidates(observations []engram.Observation) map[string]candidateRecord {
	out := make(map[string]candidateRecord)
	for _, observation := range observations {
		if observation.TopicKey == "" {
			continue
		}
		candidate := candidateRecord{
			status:       firstCapture(statusField, observation.Content),
			lastObserved: firstCapture(lastObservedField, observation.Content),
			updatedAt:    observation.UpdatedAt,
			id:           observation.ID,
		}
		if previous, ok := out[observation.TopicKey]; ok && !newerCandidate(candidate, previous) {
			continue
		}
		out[observation.TopicKey] = candidate
	}
	return out
}

func newerCandidate(candidate, previous candidateRecord) bool {
	if candidate.updatedAt != previous.updatedAt {
		return candidate.updatedAt > previous.updatedAt
	}
	return candidate.id > previous.id
}

func firstCapture(pattern *regexp.Regexp, content string) string {
	match := pattern.FindStringSubmatch(content)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func parseTimestamp(raw string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		if parsed, err := time.Parse(layout, strings.TrimSpace(raw)); err == nil {
			return parsed.UTC(), true
		}
	}
	return time.Time{}, false
}

func readFirstTarget(root string, entry ProjectLockEntry) ([]byte, error) {
	if len(entry.Targets) == 0 {
		return nil, fmt.Errorf("lock entry has no targets")
	}
	abs, err := safeTarget(root, entry.Targets[0])
	if err != nil {
		return nil, err
	}
	return os.ReadFile(abs)
}

func safeTarget(root, recorded string) (string, error) {
	if recorded == "" || path.IsAbs(recorded) || filepath.IsAbs(filepath.FromSlash(recorded)) {
		return "", fmt.Errorf("invalid target %q", recorded)
	}
	for _, segment := range strings.Split(filepath.ToSlash(recorded), "/") {
		if segment == ".." {
			return "", fmt.Errorf("invalid target %q", recorded)
		}
	}
	cleanRoot := filepath.Clean(root)
	candidate := filepath.Clean(filepath.Join(cleanRoot, filepath.FromSlash(recorded)))
	inside, err := strictlyWithin(cleanRoot, candidate)
	if err != nil {
		return "", err
	}
	if !inside {
		return "", fmt.Errorf("target %q escapes project root", recorded)
	}
	resolvedRoot, err := filepath.EvalSymlinks(cleanRoot)
	if err != nil {
		return "", fmt.Errorf("resolve project root: %w", err)
	}
	resolvedTarget, err := resolveExistingTarget(candidate)
	if err != nil {
		return "", err
	}
	inside, err = strictlyWithin(filepath.Clean(resolvedRoot), filepath.Clean(resolvedTarget))
	if err != nil {
		return "", err
	}
	if !inside {
		return "", fmt.Errorf("target %q resolves outside project root", recorded)
	}
	return candidate, nil
}

func resolveExistingTarget(target string) (string, error) {
	resolved, err := filepath.EvalSymlinks(target)
	if err == nil {
		return resolved, nil
	}
	return "", fmt.Errorf("resolve target %q: %w", target, err)
}

func strictlyWithin(root, candidate string) (bool, error) {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false, fmt.Errorf("compare target containment: %w", err)
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel), nil
}

func skillBody(data []byte) string {
	text := strings.TrimPrefix(string(data), "\ufeff")
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return text
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return strings.Join(lines[i+1:], "\n")
		}
	}
	return text
}

func fencedCommands(text string) []string {
	var commands []string
	var fence string
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if fence == "" {
			if run := openingFence(trimmed); run != "" {
				fence = run
			}
			continue
		}
		if closesFence(trimmed, fence) {
			fence = ""
			continue
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "$"))
		fields := strings.Fields(trimmed)
		if len(fields) > 0 {
			commands = append(commands, strings.Trim(fields[0], "`;,"))
		}
	}
	return commands
}

func openingFence(line string) string {
	if !strings.HasPrefix(line, "```") {
		return ""
	}
	count := 0
	for count < len(line) && line[count] == '`' {
		count++
	}
	if count < 3 {
		return ""
	}
	return strings.Repeat("`", count)
}

func closesFence(line, fence string) bool {
	return strings.HasPrefix(line, fence) && strings.TrimSpace(strings.TrimPrefix(line, fence)) == ""
}

func commandOnPath(command, pathEnv string) bool {
	if command == "" {
		return true
	}
	for _, dir := range strings.Split(pathEnv, string(os.PathListSeparator)) {
		if dir == "" {
			dir = "."
		}
		info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(command)))
		if err == nil && !info.IsDir() {
			return true
		}
	}
	return false
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

// Render converts findings into the stable report lines used by the CLI.
// The report carries evidence only; it does not imply consent to retire.
func Render(findings []Finding) string {
	var out strings.Builder
	for _, finding := range findings {
		fmt.Fprintf(&out, "%s candidate:%s status:%s\n", finding.SkillID, finding.Candidate, finding.Status)
		for _, signal := range finding.Signals {
			fmt.Fprintf(&out, "  %s\n", renderSignal(signal))
		}
	}
	return out.String()
}

func renderSignal(signal Signal) string {
	switch signal.Kind {
	case SignalRemoved:
		return fmt.Sprintf("REMOVED %s (by %s)", signal.Path, signal.Commit)
	case SignalMoved:
		return fmt.Sprintf("MOVED %s -> %s", signal.Path, signal.NewPath)
	case SignalUnresolvedCommand:
		return fmt.Sprintf("UNRESOLVED-COMMAND %s", signal.Command)
	case SignalQuiet:
		return fmt.Sprintf("QUIET since %s", signal.Since)
	case SignalSuperseded:
		return fmt.Sprintf("SUPERSEDED by %s", signal.By)
	default:
		return string(signal.Kind)
	}
}
