package skills

import (
	"bytes"
	"fmt"
	"io/fs"
	"sort"
)

// skillPlanner plans one skill across the runtime directories of the project, for PlanInstallOwnership.
// It collects what the skill writes and removes, and the directories it touches; a refusal is
// added to the list the verb shares, and the skill is then dropped whole.
type skillPlanner struct {
	c        *planContext
	sk       InstallSkill
	record   *ProjectInstallEntry
	recorded map[string]string // path -> sha256 of the files the install record lists
	dropped  []string          // recorded paths the source no longer has, sorted
	refusals *[]string

	writes, deletes []ProjectWrite
	dirs            []string
}

// planSkill plans one skill across the runtime directories. ok is false when it
// added refusals; its writes and deletes must then be dropped.
func planSkill(c *planContext, sk InstallSkill, record *ProjectInstallEntry, refusals *[]string) (writes, deletes []ProjectWrite, dirs []string, ok bool) {
	before := len(*refusals)
	p := newSkillPlanner(c, sk, record, refusals)
	aliased := map[string]string{}
	for _, target := range projectTargets {
		p.planRuntimeDir(target, aliased)
	}
	return p.writes, p.deletes, p.dirs, len(*refusals) == before
}

func newSkillPlanner(c *planContext, sk InstallSkill, record *ProjectInstallEntry, refusals *[]string) *skillPlanner {
	recorded := map[string]string{}
	if record != nil {
		for _, f := range record.Files {
			recorded[f.Path] = f.SHA256
		}
	}
	wanted := make(map[string]bool, len(sk.Files))
	for _, f := range sk.Files {
		wanted[f.Rel] = true
	}
	var dropped []string
	for p := range recorded {
		if !wanted[p] {
			dropped = append(dropped, p)
		}
	}
	sort.Strings(dropped)
	return &skillPlanner{c: c, sk: sk, record: record, recorded: recorded, dropped: dropped, refusals: refusals}
}

func (p *skillPlanner) refuse(format string, a ...any) {
	*p.refusals = append(*p.refusals, fmt.Sprintf("skills %s: "+format, append([]any{p.c.in.verb()}, a...)...))
}

// planRuntimeDir plans the skill in one runtime directory: whether it may be there at all, what of
// it is on disk and still as installed, what the source wants written, and what it dropped.
func (p *skillPlanner) planRuntimeDir(target ProjectTarget, aliased map[string]string) {
	dirRel := target.Dir + "/" + p.sk.ID
	dirAbs, exists, ok := p.inspectDir(dirRel, aliased)
	if !ok {
		return
	}
	p.dirs = append(p.dirs, dirAbs)

	if p.record == nil && exists {
		p.refuse("%s already exists and was not installed by skills install (it has no install record in %s); if it is exactly the current skill, run `labdrian skills adopt --project-id %s` to take ownership of it, otherwise move it away",
			dirRel, ProjectLockRelPath, p.c.in.ProjectID)
		return
	}

	current, modes := p.readRecordedFiles(dirRel, exists)
	p.planSourceFiles(dirRel, exists, current)
	p.planDroppedFiles(dirRel, current, modes)
}

// inspectDir proves the directory may be written, that no earlier runtime directory resolved to
// the same one, and says whether it exists. ok is false when it added a refusal.
func (p *skillPlanner) inspectDir(dirRel string, aliased map[string]string) (dirAbs string, exists, ok bool) {
	dirAbs, resolvedDir, err := p.c.guard.destination(dirRel)
	if err != nil {
		p.refuse("destination %s: %v", dirRel, err)
		return "", false, false
	}
	if other, dup := aliased[resolvedDir]; dup {
		p.refuse("%s and %s resolve to the same directory, so one install would silently overwrite the other", other, dirRel)
		return "", false, false
	}
	aliased[resolvedDir] = dirRel

	info, err := p.c.in.Stat(dirAbs)
	exists = err == nil
	switch {
	case err != nil && !isAbsent(err):
		p.refuse("cannot inspect %s: %v", dirRel, err)
		return "", false, false
	case exists && !info.IsDir():
		p.refuse("%s exists and is not a directory", dirRel)
		return "", false, false
	}
	return dirAbs, exists, true
}

// readRecordedFiles reads what is on disk now, for every recorded file, checked against its
// record: the bytes and the mode of each file that is still as install wrote it. A file that is
// gone is simply absent from the result; one that is edited, not regular or unreadable is a
// refusal.
func (p *skillPlanner) readRecordedFiles(dirRel string, exists bool) (current map[string][]byte, modes map[string]fs.FileMode) {
	current = map[string][]byte{}
	modes = map[string]fs.FileMode{}
	if p.record == nil || !exists {
		return current, modes
	}
	for _, f := range p.record.Files {
		rel := dirRel + "/" + f.Path
		abs, _, err := p.c.guard.destination(rel)
		if err != nil {
			p.refuse("destination %s: %v", rel, err)
			continue
		}
		data, mode, status := readInstalled(p.c.in, abs)
		switch status {
		case fileMissing:
			continue
		case fileNotRegular:
			p.refuse("%s is not a regular file, although skills install wrote a file there; move it away and run install again", rel)
			continue
		case fileUnreadable:
			p.refuse("cannot read %s", rel)
			continue
		}
		if HashSkill(data) != f.SHA256 {
			p.refuse("%s was edited since skills install wrote it (its bytes no longer match the install record); restore it or move it away, then run install again", rel)
			continue
		}
		current[f.Path] = data
		modes[f.Path] = mode
	}
	return current, modes
}

// planSourceFiles plans a write for every file the source holds that is not already on disk as
// the source has it. A file in the directory that the record does not list is left alone, unless
// the source wants that very path: then it is refused.
func (p *skillPlanner) planSourceFiles(dirRel string, exists bool, current map[string][]byte) {
	for _, f := range p.sk.Files {
		rel := dirRel + "/" + f.Rel
		abs, _, err := p.c.guard.destination(rel)
		if err != nil {
			p.refuse("destination %s: %v", rel, err)
			continue
		}
		if _, isRecorded := p.recorded[f.Rel]; isRecorded && exists {
			cur, present := current[f.Rel]
			switch {
			case !present:
				// Recorded and gone: someone deleted it, and install writes it again.
				p.writes = append(p.writes, ProjectWrite{Rel: rel, Abs: abs, Data: f.Data, Mode: f.Mode})
			case bytes.Equal(cur, f.Data):
				// Already what the source holds: not written.
			default:
				p.writes = append(p.writes, ProjectWrite{Rel: rel, Abs: abs, Data: f.Data, Mode: f.Mode, Backup: cloneProjectBytes(cur)})
			}
			continue
		}
		if exists {
			// Not recorded. If anything is there, it is not ours to overwrite.
			if _, err := p.c.in.Stat(abs); err == nil {
				p.refuse("%s exists but is not recorded as installed by skills install, and the source now wants to write it; move it away and run install again", rel)
				continue
			} else if !isAbsent(err) {
				p.refuse("cannot inspect %s: %v", rel, err)
				continue
			}
		}
		p.writes = append(p.writes, ProjectWrite{Rel: rel, Abs: abs, Data: f.Data, Mode: f.Mode})
	}
}

// planDroppedFiles plans the removal of every file the source dropped that is still on disk as
// install wrote it.
func (p *skillPlanner) planDroppedFiles(dirRel string, current map[string][]byte, modes map[string]fs.FileMode) {
	for _, path := range p.dropped {
		cur, present := current[path]
		if !present {
			continue
		}
		rel := dirRel + "/" + path
		abs, _, err := p.c.guard.destination(rel)
		if err != nil {
			p.refuse("destination %s: %v", rel, err)
			continue
		}
		p.deletes = append(p.deletes, ProjectWrite{Rel: rel, Abs: abs, Mode: modes[path], Backup: cloneProjectBytes(cur)})
	}
}

type installedFile int

const (
	fileReadable installedFile = iota
	fileMissing
	fileNotRegular
	fileUnreadable
)

// readInstalled reads one file of an installed skill through the planner's probes.
func readInstalled(in InstallInput, abs string) ([]byte, fs.FileMode, installedFile) {
	info, err := in.Stat(abs)
	if err != nil {
		if isAbsent(err) {
			return nil, 0, fileMissing
		}
		return nil, 0, fileUnreadable
	}
	if !info.Mode().IsRegular() {
		return nil, 0, fileNotRegular
	}
	data, err := in.ReadFile(abs)
	if err != nil {
		return nil, 0, fileUnreadable
	}
	return data, info.Mode().Perm(), fileReadable
}
