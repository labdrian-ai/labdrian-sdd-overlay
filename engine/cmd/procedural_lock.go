package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability/presence"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// proceduralLockReader is the adapter of the projection hook's port app.ProceduralSkillsReader (C23
// M4 decision 5): it reads the procedural skills lock (skills.ProjectLockRelPath) at the root of the
// worktree that holds the session's directory, and turns each procedural skill's targets into the
// absolute paths of its SKILL.md files. The skills the lock records as registry installs are not
// procedural and are left out.
//
// It never fails: no worktree or no lock is no procedural skill, and a lock that cannot be read or
// is not a valid lock is reported as unreadable, with the reason.
type proceduralLockReader struct {
	// worktreeRoot is the root of the worktree that holds a directory, or "" for none.
	worktreeRoot func(dir string) string
}

func newProceduralLockReader() proceduralLockReader {
	return proceduralLockReader{worktreeRoot: func(dir string) string { return newRepoLocator().Provenance(dir).WorktreeRoot }}
}

// proceduralLockFile is where the procedural skills lock is in the worktree rooted at root.
func proceduralLockFile(root string) string {
	return filepath.Join(root, filepath.FromSlash(skills.ProjectLockRelPath))
}

func (r proceduralLockReader) Load(dir string) projection.ProceduralSkills {
	root := r.worktreeRoot(dir)
	if root == "" {
		return projection.ProceduralSkills{}
	}
	data, err := os.ReadFile(proceduralLockFile(root))
	if errors.Is(err, fs.ErrNotExist) {
		return projection.ProceduralSkills{}
	}
	if err != nil {
		return projection.ProceduralSkills{Unreadable: err.Error()}
	}
	lock, err := skills.ParseProjectLock(data)
	if err != nil {
		return projection.ProceduralSkills{Unreadable: err.Error()}
	}
	out := projection.ProceduralSkills{Locked: true, Skills: make([]projection.ProceduralSkill, 0, len(lock.Skills))}
	for _, entry := range lock.Skills {
		skill := projection.ProceduralSkill{ID: entry.ID}
		for _, target := range entry.Targets {
			if file, ok := skillFileIn(root, target); ok {
				skill.SkillFiles = append(skill.SkillFiles, file)
				// A session that entered through a link names the root by the link; the gate
				// compares the real path of a Read (realSkillFile), so the real path is listed too.
				if real, err := filepath.EvalSymlinks(file); err == nil && real != file {
					skill.SkillFiles = append(skill.SkillFiles, real)
				}
			}
		}
		out.Skills = append(out.Skills, skill)
	}
	return out
}

// skillFileIn is the absolute, cleaned path of a repo-relative target of the lock, and whether it
// is one: a target that is absolute or climbs out of the root is not one, and names no file.
func skillFileIn(root, target string) (string, bool) {
	rel := filepath.FromSlash(target)
	if target == "" || filepath.IsAbs(rel) || !filepath.IsLocal(rel) {
		return "", false
	}
	return filepath.Join(root, rel), true
}

// processProject is the adapter of the presence prober's port presence.ProjectContext (C10c): the
// project is the worktree that holds the working directory of the process, and its procedural skills
// lock is read by the reader the SessionStart probe reads it with, so the record made when a workflow
// is created and a session cannot disagree on whether a lock can be used. The reader's own words are
// the reason of a lock that cannot be used, which is the same reason the session probe gives, except
// that the path of the lock, which the file system's errors carry, becomes its place in the project:
// the record is kept in the log of a workflow, and the prober names no path there.
//
// It never fails: a working directory that cannot be determined, or that is in no worktree, is a
// project it does not know.
type processProject struct {
	// workingDir is the working directory of the process (deps.workingDir).
	workingDir func() (string, error)
	// worktreeRoot is the root of the worktree that holds a directory, or "" for none: the reader's
	// own, so that this adapter and the reader cannot disagree on which project a directory is in.
	worktreeRoot func(dir string) string
	// locks reads the lock of the project a directory belongs to.
	locks app.ProceduralSkillsReader
}

var _ presence.ProjectContext = processProject{}

func (p processProject) ReadLock() presence.LockReading {
	dir, err := p.workingDir()
	if err != nil {
		return presence.LockReading{}
	}
	root := p.worktreeRoot(dir)
	if root == "" {
		return presence.LockReading{}
	}
	lock := p.locks.Load(dir)
	switch {
	case lock.Unreadable != "":
		return presence.LockReading{State: presence.LockInvalid, Reason: withoutLockPath(lock.Unreadable, root)}
	case !lock.Locked:
		return presence.LockReading{State: presence.LockAbsent}
	}
	return presence.LockReading{State: presence.LockValid}
}

// withoutLockPath is a reason the reader gave with the path of the lock of the worktree rooted at root
// written as the lock's place in the project (skills.ProjectLockRelPath), which says the same without
// naming where the project is on this machine.
func withoutLockPath(reason, root string) string {
	return strings.ReplaceAll(reason, proceduralLockFile(root), skills.ProjectLockRelPath)
}
