// Package agentgen is the generator of the agent files of the overlay: one source per agent, projected
// to the runtimes that host it. Two kinds of agent are generated.
//
// The role agents of per-role design v3 (roles.go) have one source each, roles/<role>/body.md under
// the root of the repository, and one table of the frontmatter of each runtime. Each becomes three
// files:
//   - agents/<role>.md        (Claude Code agent file)
//   - pi/agents/<role>.md     (Pi native subagent file, read by gentle-pi)
//   - codex/agents/<role>.toml (Codex agent file)
//
// The GADU persona (gadu.go) has its canonical body in engine/agentgen/persona/body.md and becomes
// four files: agents/GADU.md, opencode/agents/GADU.md, pi/agents/GADU.md and
// skills/gadu-operator/SKILL.md. PR 5 of the design deletes it.
//
// None of these files is ever hand-edited. Use Generate to write them; use Check to assert they are
// not stale.
package agentgen

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Artifact is one file the generator emits: where it goes under the root of the repository, as a
// slash-separated path, and what it holds.
type Artifact struct {
	Path    string
	Content string
}

// sourceError is a role source that could not be turned into files: the path of the source, under
// the root of the repository, what was being done to it and why that failed.
type sourceError struct {
	path string
	verb string
	err  error
}

func (e *sourceError) Error() string { return e.verb + " " + e.path + ": " + e.err.Error() }
func (e *sourceError) Unwrap() error { return e.err }

// build makes the whole table of what the generator emits for the repository at repoRoot, in the
// order Generate writes it and Check reports it: the four files of GADU, then for each role, in
// the order of its table, its Claude, Pi and Codex files. The sources of the roles are read from
// repoRoot; a role whose source cannot be read or projected has no files in the table and is
// returned as a problem, so that the caller can say all of them.
func build(repoRoot string) (table []Artifact, problems []*sourceError) {
	table = gaduArtifacts()
	for _, r := range roles {
		data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(r.sourcePath())))
		if err != nil {
			problems = append(problems, &sourceError{path: r.sourcePath(), verb: "reading", err: err})
			continue
		}
		files, err := r.project(string(data))
		if err != nil {
			problems = append(problems, &sourceError{path: r.sourcePath(), verb: "projecting", err: err})
			continue
		}
		table = append(table, files...)
	}
	return table, problems
}

// Artifacts is the table of what the generator emits for the repository at repoRoot, in the order
// Generate writes it and Check reports it:
//   - agents/GADU.md, opencode/agents/GADU.md, pi/agents/GADU.md, skills/gadu-operator/SKILL.md
//   - for each role: agents/<role>.md, pi/agents/<role>.md, codex/agents/<role>.toml
//
// Each call builds the table afresh, so no caller can change what the next one sees. It fails when
// the source of a role cannot be read or projected.
func Artifacts(repoRoot string) ([]Artifact, error) {
	table, problems := build(repoRoot)
	if err := joinProblems(problems); err != nil {
		return nil, err
	}
	return table, nil
}

func joinProblems(problems []*sourceError) error {
	errs := make([]error, len(problems))
	for i, p := range problems {
		errs[i] = p
	}
	return errors.Join(errs...)
}

// Generate writes every artifact of the table under repoRoot. Parent directories are created as
// needed. Generate is idempotent and overwrites any existing files. It builds the whole table
// before it writes the first file, so a source that cannot be read leaves the files as they were.
func Generate(repoRoot string) error {
	table, problems := build(repoRoot)
	if err := joinProblems(problems); err != nil {
		return err
	}
	for _, a := range table {
		if err := writeFile(filepath.Join(repoRoot, filepath.FromSlash(a.Path)), a.Content); err != nil {
			return fmt.Errorf("writing %s: %w", a.Path, err)
		}
	}
	return nil
}

// Check compares the committed copies under repoRoot with the table, in memory: it writes nothing,
// not even a scratch directory. It returns a non-nil error with a descriptive message if any file
// is missing, unreadable, stale or divergent, or if the source of a role cannot be read or
// projected (its three files are not compared then: there is no body to compare them with). (R-003)
func Check(repoRoot string) error {
	table, problems := build(repoRoot)
	var errs []string
	for _, a := range table {
		committed, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(a.Path)))
		if err != nil {
			if os.IsNotExist(err) {
				errs = append(errs, fmt.Sprintf("%s: missing (not yet generated — run: gentle-ai-overlay gadu-generate)", a.Path))
				continue
			}
			errs = append(errs, fmt.Sprintf("%s: cannot read committed file: %v", a.Path, err))
			continue
		}
		if string(committed) != a.Content {
			errs = append(errs, fmt.Sprintf("%s: stale — committed content diverges from generator output (run: gentle-ai-overlay gadu-generate)", a.Path))
		}
	}
	for _, p := range problems {
		switch {
		case errors.Is(p.err, fs.ErrNotExist):
			errs = append(errs, fmt.Sprintf("%s: missing (the source of the role is not in the repository)", p.path))
		case p.verb == "reading":
			errs = append(errs, fmt.Sprintf("%s: cannot read the source of the role: %v", p.path, p.err))
		default:
			errs = append(errs, fmt.Sprintf("%s: cannot be projected: %v", p.path, p.err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("generated agent artifacts are stale or missing:\n  %s", strings.Join(errs, "\n  "))
	}
	return nil
}

// writeFile writes content to path, creating parent directories as needed.
func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating parent dir for %s: %w", path, err)
	}
	return os.WriteFile(path, []byte(content), 0644)
}
