// Package app is the use case behind `engine registry repair-sdd`: it reads a project's skill
// registry (or starts one, when the project has none), reads the SDD executor skills the machine has
// installed, and writes the registry back with a row for each. The rules for the text are
// engine/sddregistry's; where the registry and the skills are is reached through the two ports this
// package owns, which engine/sddregistry/fsstore answers and the composition root wires.
//
// It takes no lock, prints nothing and exits nowhere: the lock that keeps two writers of the
// registry apart, the words a person reads and the exit code are the command's.
package app

import (
	"errors"
	"io/fs"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sddregistry"
)

// RegistryStore is how the use case reaches the registry of a project.
//
// The caller holds whatever lock the registry needs for as long as it needs it. The port takes
// none, because what a lock protects is the whole run, not one call.
type RegistryStore interface {
	// Read returns the content of the registry at path. A registry that is not there is an error
	// for which errors.Is(err, fs.ErrNotExist) holds, so that "there is none" is told from "it could
	// not be read" without looking at the words of an error.
	Read(path string) ([]byte, error)
	// Write replaces the registry at path with content, whole: a reader running beside the write
	// sees the old registry or the new one, never half of either. The directory the registry lives
	// in is made when there is none.
	Write(path string, content []byte) error
}

// SkillSource is how the use case reaches the skills a machine has installed.
type SkillSource interface {
	// IsFile says whether path is a regular file: a directory, or nothing, is not.
	IsFile(path string) bool
	// ReadFile returns the content of the file at path.
	ReadFile(path string) ([]byte, error)
}

// Request is one repair: which registry, and where the target keeps its skills.
type Request struct {
	// Registry is the path of the registry, as the store is to be asked for it.
	Registry string
	// SkillsRoot is the directory the target keeps its skills in. The file of a skill is
	// <SkillsRoot>/<name>/SKILL.md.
	SkillsRoot string
}

// Result is what a repair did.
type Result struct {
	// Missing holds the file of each executor skill that is not installed, in the order of the
	// executors; such a skill has no row.
	Missing []string
	// Rows is how many lines of the registry written begin like an executor row.
	Rows int
}

// Service runs the use case over the ports it is given.
type Service struct {
	registry RegistryStore
	skills   SkillSource
}

// New returns the use case over the registry store and the skill source.
func New(registry RegistryStore, skills SkillSource) *Service {
	return &Service{registry: registry, skills: skills}
}

// RegistryReadError is a registry that is there and cannot be read.
type RegistryReadError struct {
	Path string
	Err  error
}

func (e *RegistryReadError) Error() string {
	return "cannot read the registry " + e.Path + ": " + e.Err.Error()
}

func (e *RegistryReadError) Unwrap() error { return e.Err }

// SkillReadError is an installed executor skill that cannot be read.
type SkillReadError struct {
	Path string
	Err  error
}

func (e *SkillReadError) Error() string { return "cannot read " + e.Path + ": " + e.Err.Error() }

func (e *SkillReadError) Unwrap() error { return e.Err }

// RegistryWriteError is a registry that cannot be written.
type RegistryWriteError struct {
	Path string
	Err  error
}

func (e *RegistryWriteError) Error() string {
	return "cannot write the registry " + e.Path + ": " + e.Err.Error()
}

func (e *RegistryWriteError) Unwrap() error { return e.Err }

// Repair reads the registry, or makes one when there is none, puts a row in it for each executor
// skill the target has installed, and writes it. A registry that lacks a heading the rows go
// between is refused (a *sddregistry.MissingSectionError) and nothing is written. An executor skill
// that is not installed has no row and is told in the Result.
func (s *Service) Repair(req Request) (Result, error) {
	content, err := s.current(req.Registry)
	if err != nil {
		return Result{}, err
	}

	var rows []sddregistry.Executor
	var missing []string
	// The root is joined to the name by one slash. Only the slashes the root ends with are dropped,
	// to make that one: the ones it has inside stay, because the path is written into the registry
	// as it was joined and the script joined by concatenation.
	root := strings.TrimRight(req.SkillsRoot, "/")
	for _, name := range sddregistry.Executors() {
		skillPath := root + "/" + name + "/SKILL.md"
		if !s.skills.IsFile(skillPath) {
			missing = append(missing, skillPath)
			continue
		}
		text, err := s.skills.ReadFile(skillPath)
		if err != nil {
			return Result{}, &SkillReadError{Path: skillPath, Err: err}
		}
		rows = append(rows, sddregistry.NewExecutor(name, skillPath, string(text)))
	}

	repaired, err := sddregistry.Repair(content, rows)
	if err != nil {
		return Result{}, err
	}
	if err := s.registry.Write(req.Registry, []byte(repaired)); err != nil {
		return Result{}, &RegistryWriteError{Path: req.Registry, Err: err}
	}
	return Result{Missing: missing, Rows: sddregistry.CountRows(repaired)}, nil
}

// current is the registry as it is, or the one a project that has none starts with.
func (s *Service) current(path string) (string, error) {
	data, err := s.registry.Read(path)
	switch {
	case err == nil:
		return string(data), nil
	case errors.Is(err, fs.ErrNotExist):
		return sddregistry.NewRegistry(sddregistry.ProjectName(path)), nil
	}
	return "", &RegistryReadError{Path: path, Err: err}
}
