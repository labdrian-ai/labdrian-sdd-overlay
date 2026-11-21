// Package app is the use case behind `engine registry refresh`: it finds the skills a project and
// its machine keep, and writes the table of them into the project's .atl/skill-registry.md,
// leaving every line of the registry that the table does not own as it was. The rules for the text
// are engine/skillregistry's; where the skills are, where the registry is, the lock that keeps two
// writers of the registry apart and what day it is are reached through the ports this package owns.
//
// It prints nothing and exits nowhere: the words a person reads and the exit code are the
// command's.
package app

import (
	"errors"
	"io/fs"
	"path"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skillregistry"
)

// RegistryStore is how the use case reaches the registry of a project. The caller of Refresh holds
// whatever lock the registry needs through the Locker; the store takes none.
type RegistryStore interface {
	// Read returns the content of the registry at path. A registry that is not there is an error
	// for which errors.Is(err, fs.ErrNotExist) holds.
	Read(path string) ([]byte, error)
	// Write replaces the registry at path with content, whole: a reader running beside the write
	// sees the old registry or the new one, never half of either. The directory the registry lives
	// in is made when there is none.
	Write(path string, content []byte) error
}

// SkillSource is how the use case reaches the skills of a machine.
type SkillSource interface {
	// IsProject says whether dir is the root of a project: a directory that holds .git or .atl.
	IsProject(dir string) bool
	// Scan returns the skills kept in the source, one for each directory of it that holds a
	// SKILL.md, in the order of the directory names, and whether the source is a directory.
	Scan(source skillregistry.Source) (candidates []skillregistry.Candidate, exists bool)
}

// Locker is the lock that keeps the writers of one registry apart.
type Locker interface {
	// Lock takes the lock of the registry at path, waiting for its holder up to a bound the
	// adapter owns, and returns the function that lets it go.
	Lock(registry string) (release func(), err error)
}

// RegistryPath is where the registry of a project is, relative to the project.
const RegistryPath = ".atl/skill-registry.md"

// dateLayout is how the registry says the day it was written.
const dateLayout = "2006-01-02"

// Outcome is what a refresh did.
type Outcome int

// The outcomes of a refresh.
const (
	// Skipped is a directory that is not a project: nothing was read or written.
	Skipped Outcome = iota
	// UpToDate is a registry that already holds the table: only its date would have changed.
	UpToDate
	// Refreshed is a registry that was written.
	Refreshed
)

// Request is one refresh.
type Request struct {
	// Cwd is the project: an absolute directory.
	Cwd string
	// Home is the home whose skills are listed after the project's; empty lists the project's only.
	Home string
	// Force writes the registry even when only its date would change.
	Force bool
}

// Result is what a refresh did.
type Result struct {
	Outcome Outcome
	// Registry is the path of the registry of the project.
	Registry string
	// Skills is how many rows the table holds.
	Skills int
}

// Service runs the use case over the ports it is given.
type Service struct {
	registry RegistryStore
	skills   SkillSource
	locker   Locker
	now      func() time.Time
}

// New returns the use case over its ports and the clock that dates the registry.
func New(registry RegistryStore, skills SkillSource, locker Locker, now func() time.Time) *Service {
	return &Service{registry: registry, skills: skills, locker: locker, now: now}
}

// LockError is a registry whose lock could not be taken.
type LockError struct {
	Path string
	Err  error
}

func (e *LockError) Error() string { return "acquiring registry lock: " + e.Err.Error() }

func (e *LockError) Unwrap() error { return e.Err }

// RegistryReadError is a registry that is there and cannot be read.
type RegistryReadError struct {
	Path string
	Err  error
}

func (e *RegistryReadError) Error() string {
	return "cannot read the registry " + e.Path + ": " + e.Err.Error()
}

func (e *RegistryReadError) Unwrap() error { return e.Err }

// RegistryWriteError is a registry that cannot be written.
type RegistryWriteError struct {
	Path string
	Err  error
}

func (e *RegistryWriteError) Error() string {
	return "cannot write the registry " + e.Path + ": " + e.Err.Error()
}

func (e *RegistryWriteError) Unwrap() error { return e.Err }

// Refresh writes the table of skills into the registry of the project at req.Cwd.
func (s *Service) Refresh(req Request) (Result, error) {
	if !s.skills.IsProject(req.Cwd) {
		return Result{Outcome: Skipped}, nil
	}
	registry := path.Join(req.Cwd, RegistryPath)

	var labels []string
	var found []skillregistry.Candidate
	for _, source := range skillregistry.Sources(req.Cwd, req.Home) {
		candidates, exists := s.skills.Scan(source)
		if !exists {
			continue
		}
		labels = append(labels, source.Label)
		found = append(found, candidates...)
	}
	rows := skillregistry.Rows(found)

	release, err := s.locker.Lock(registry)
	if err != nil {
		return Result{}, &LockError{Path: registry, Err: err}
	}
	defer release()

	existing, err := s.current(registry)
	if err != nil {
		return Result{}, err
	}
	header := skillregistry.Header{Project: path.Base(path.Clean(req.Cwd)), Sources: labels}
	result := Result{Outcome: Refreshed, Registry: registry, Skills: len(rows)}

	// The registry is as it should be when writing it with the day it already says changes nothing.
	if date := skillregistry.LastUpdated(existing); !req.Force && existing != "" && date != "" {
		header.Date = date
		same, err := skillregistry.Compose(existing, header, rows)
		if err != nil {
			return Result{}, err
		}
		if same == existing {
			result.Outcome = UpToDate
			return result, nil
		}
	}

	header.Date = s.now().UTC().Format(dateLayout)
	content, err := skillregistry.Compose(existing, header, rows)
	if err != nil {
		return Result{}, err
	}
	if err := s.registry.Write(registry, []byte(content)); err != nil {
		return Result{}, &RegistryWriteError{Path: registry, Err: err}
	}
	return result, nil
}

// current is the registry as it is, or "" for a project that has none.
func (s *Service) current(registry string) (string, error) {
	data, err := s.registry.Read(registry)
	switch {
	case err == nil:
		return string(data), nil
	case errors.Is(err, fs.ErrNotExist):
		return "", nil
	}
	return "", &RegistryReadError{Path: registry, Err: err}
}
