package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
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

func (r proceduralLockReader) Load(dir string) projection.ProceduralSkills {
	root := r.worktreeRoot(dir)
	if root == "" {
		return projection.ProceduralSkills{}
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(skills.ProjectLockRelPath)))
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
	out := projection.ProceduralSkills{Skills: make([]projection.ProceduralSkill, 0, len(lock.Skills))}
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
