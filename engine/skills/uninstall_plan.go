package skills

// The rules of `skills uninstall`: install's ownership rule run the other way. A skill that
// install wrote, and left as it wrote it, is the program's to remove; a file someone edited, or
// one the install record does not list, is someone's, and stays.

import (
	"fmt"
	"path/filepath"
)

// InstallUninstalled: the skill's recorded files and its install record were removed.
const InstallUninstalled InstallStatus = "uninstalled"

// PlanUninstall decides the removal of the installed skill id: every file its install record
// lists that is still on disk as install wrote it, in every runtime directory, and the record
// itself. It is the install planner given a source with no files, so every recorded file is one
// the source dropped, and the same rules decide: a recorded file that was edited, is not a regular
// file, or cannot be read refuses the whole uninstall, naming it, and nothing is removed. A file
// that is already gone has nothing to remove, and a file the record does not list is left alone.
// An id with no install record, or one that is a procedural skill (project-retire's), is refused.
// in.Skills is not read.
func PlanUninstall(in InstallInput, id string) (InstallPlan, []string) {
	in.Verb = "uninstall"
	c, problems := newPlanContext(in)
	if problems != nil {
		return InstallPlan{}, problems
	}
	refuse := func(format string, a ...any) []string {
		return []string{fmt.Sprintf("skills uninstall: "+format, a...)}
	}
	if c.procedural[id] {
		return InstallPlan{}, refuse("%s is registered in the project lock as a procedural skill, written by project-register; uninstall does not touch it (`labdrian skills project-retire` removes it)", id)
	}
	record := c.record(id)
	if record == nil {
		return InstallPlan{}, refuse("%s has no install record in %s, so nothing installed it here and nothing is removed", id, ProjectLockRelPath)
	}

	var refusals []string
	_, deletes, dirs, ok := planSkill(c, InstallSkill{ID: id}, record, &refusals)
	if !ok {
		return InstallPlan{}, refusals
	}
	installs := make([]ProjectInstallEntry, 0, len(c.lock.Installs)-1)
	for _, r := range c.lock.Installs {
		if r.ID != id {
			installs = append(installs, r)
		}
	}
	lock, problems := c.lockWrite(installs)
	if problems != nil {
		return InstallPlan{}, problems
	}
	return InstallPlan{
		Verb:    "uninstall",
		Skills:  []InstallOutcome{{ID: id, Status: InstallUninstalled}},
		Deletes: deletes,
		Lock:    lock,
		Dirs:    dirs,
	}, nil
}

// RemoveEmptySkillDirs removes, after ExecuteInstallPlan carried out an uninstall, each skill
// directory of the plan that now holds no file (its empty subdirectories, such as one whose only
// file someone had already deleted, go first), and returns, relative to root, the ones that still
// hold something: files the install record did not list, which are someone's. A directory that is
// already gone is neither. It is best effort, as the pruning inside a skill is: a directory that
// cannot be read or removed is reported as kept.
func RemoveEmptySkillDirs(fsys ProjectFS, root string, p InstallPlan) (kept []string) {
	for _, dir := range p.Dirs {
		if _, err := fsys.Stat(dir); isAbsent(err) {
			continue
		}
		if !removeEmptyTree(fsys, dir) {
			kept = append(kept, relToRoot(root, dir))
		}
	}
	return kept
}

// removeEmptyTree removes dir when it holds nothing but directories that hold nothing, deepest
// first, and reports whether dir is gone. It removes no file, and does not follow a symbolic link
// (one is an entry like a file: it keeps the directory).
func removeEmptyTree(fsys ProjectFS, dir string) bool {
	entries, err := fsys.ReadDir(dir)
	if err != nil {
		return false
	}
	empty := true
	for _, e := range entries {
		if !e.IsDir() || !removeEmptyTree(fsys, filepath.Join(dir, e.Name())) {
			empty = false
		}
	}
	return empty && fsys.Remove(dir) == nil
}

// relToRoot is dir relative to root, forward-slashed, or dir itself when it is not inside root.
func relToRoot(root, dir string) string {
	rel, err := filepath.Rel(filepath.Clean(root), dir)
	if err != nil {
		return dir
	}
	return filepath.ToSlash(rel)
}
