// Package agentgen is the generator of the agent files of the overlay: one source per agent, projected
// to the runtimes that host it.
//
// The role agents of per-role design v3 (roles.go) have one source each, roles/<role>/body.md under
// the root of the repository, and one table of the frontmatter of each runtime. Each becomes four
// files:
//   - agents/<role>.md        (Claude Code agent file)
//   - pi/agents/<role>.md     (Pi native subagent file, read by gentle-pi)
//   - codex/agents/<role>.toml (Codex agent file)
//   - opencode/agents/<role>.md (OpenCode agent file, opencode.go)
//
// The foreman, the parent of the roles, has one source too, roles/foreman/body.md, and two files,
// agents/foreman.md and opencode/agents/foreman.md (S2, S5 of odd/tasks/labdrian-orchestrator-opencode.md).
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

	vocabulary "github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
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
// order Generate writes it and Check reports it: for each role, in the order of its table, its
// Claude, Pi and Codex files. The sources of the roles are read from repoRoot; a role whose source
// cannot be read or projected has no files in the table and is returned as a problem, so that the
// caller can say all of them.
func build(repoRoot string) (table []Artifact, problems []*sourceError) {
	models, modelsErr := ReadModelTable(repoRoot)
	// source is the text of the source of a role; a role whose source cannot be read is a problem.
	source := func(r role) (string, bool) {
		data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(r.sourcePath())))
		if err != nil {
			problems = append(problems, &sourceError{path: r.sourcePath(), verb: "reading", err: err})
			return "", false
		}
		return string(data), true
	}
	// The OpenCode files come last, after the Claude file of the foreman: the eight roles, then the foreman.
	var openCode []Artifact
	for _, r := range roles {
		data, ok := source(r)
		if !ok || modelsErr != nil {
			continue
		}
		if err := checkSkillsNamed(repoRoot, r, data); err != nil {
			problems = append(problems, &sourceError{path: r.sourcePath(), verb: "projecting", err: err})
			continue
		}
		files, err := r.project(data, models)
		if err != nil {
			problems = append(problems, &sourceError{path: r.sourcePath(), verb: "projecting", err: err})
			continue
		}
		table = append(table, files...)
		file, err := r.projectOpenCode(data, models)
		if err != nil {
			problems = append(problems, &sourceError{path: r.sourcePath(), verb: "projecting", err: err})
			continue
		}
		openCode = append(openCode, file)
	}
	// The foreman comes after the eight roles, with its Claude file and its OpenCode file only (S5).
	if data, ok := source(foreman); ok && modelsErr == nil {
		file, err := foreman.projectClaude(data, models)
		if err == nil {
			err = checkSkillsNamed(repoRoot, foreman, data)
		}
		if err != nil {
			problems = append(problems, &sourceError{path: foreman.sourcePath(), verb: "projecting", err: err})
		} else {
			table = append(table, file)
			if file, err := foreman.projectOpenCode(data, models); err != nil {
				problems = append(problems, &sourceError{path: foreman.sourcePath(), verb: "projecting", err: err})
			} else {
				openCode = append(openCode, file)
			}
		}
	}
	table = append(table, openCode...)
	// The table is reported after the sources: without it no role can be projected, so nothing is returned
	// for the roles whose sources were read.
	if modelsErr != nil {
		verb := "reading"
		var malformed *tableError
		if errors.As(modelsErr, &malformed) {
			verb = "parsing"
		}
		problems = append(problems, &sourceError{path: modelsPath, verb: verb, err: modelsErr})
	}
	return table, problems
}

// checkSkillsNamed fails when the body of the role names a skill, in the form "`<id>` skill", whose
// SKILL.md is not in the repository. A role on OpenCode has no skill tool, so the adapter hands it
// that file by path (roles.SkillsNamed); the body and the file have to agree before any file is written.
func checkSkillsNamed(repoRoot string, r role, body string) error {
	for _, id := range vocabulary.SkillsNamed(body) {
		path := "skills/" + id + "/SKILL.md"
		if info, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(path))); err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("%s names the skill `%s` but %s is not in the repository: name a skill that has a directory under skills/, or remove the mention", r.sourcePath(), id, path)
		}
	}
	return nil
}

// Artifacts is the table of what the generator emits for the repository at repoRoot, in the order
// Generate writes it and Check reports it: for each role, agents/<role>.md, pi/agents/<role>.md and
// codex/agents/<role>.toml.
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
		case p.path == modelsPath && errors.Is(p.err, fs.ErrNotExist):
			errs = append(errs, fmt.Sprintf("%s: missing (the tier table of the models is not in the repository)", p.path))
		case p.path == modelsPath:
			errs = append(errs, fmt.Sprintf("%s: cannot use the tier table of the models: %v", p.path, p.err))
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
