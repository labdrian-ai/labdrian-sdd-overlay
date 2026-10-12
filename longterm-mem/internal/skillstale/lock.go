package skillstale

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// ProjectLockRelPath is where a project keeps its procedural skill lock, relative to the project root: the name
// the lock is asked for by (SkillSource.ReadLock), whatever the project's location.
const ProjectLockRelPath = ".labdrian/procedural-skills.lock.json"

// ProjectLock is the read-only detector view of the engine's project lock.
// The fields intentionally mirror engine/skills.ProjectLock without creating a
// module dependency in either direction; the files under contracts/project-lock
// are what hold the two to each other (both modules read them in their tests).
type ProjectLock struct {
	Version int                `json:"version"`
	Skills  []ProjectLockEntry `json:"skills"`
	// Installs holds the records `skills install` keeps in the same file, for the
	// registry skills it wrote into the project. The detector is about procedural
	// skills and never reads them; the field is here so that the strict parser
	// accepts a lock that has them instead of reporting an unknown field. It is
	// kept raw on purpose: what is in it is engine/skills' to validate.
	Installs json.RawMessage `json:"installs,omitempty"`
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

// ParseProjectLock strictly parses the shared project-lock format. A missing
// file is the caller's concern (Detect reports it); this function only parses
// bytes it was given and refuses unknown fields, trailing values, unsupported
// versions, and duplicate ids.
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
