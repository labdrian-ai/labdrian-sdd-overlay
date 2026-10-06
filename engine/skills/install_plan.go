package skills

// The ownership rules of `skills install`, as a planner that decides everything
// before anything is written. A skill that install wrote, and left as it wrote it, is
// the program's to replace; anything else in a skill directory is someone's. The
// proof is the install record in the project lock (ProjectInstallEntry): the files
// install wrote and the SHA-256 of each, checked against what is on disk now, which
// is EvaluateOwnership's rule, extended from one SKILL.md to a tree of files. The
// containment rules are the shared ones: resolveWritePath for every destination, so a
// symlinked .claude cannot aim a write out of the project or into its own skills/.

import (
	"bytes"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
)

// InstallStatus says what an install did, or would do, to one skill.
type InstallStatus string

const (
	// InstallCreated: neither runtime had the skill; both now do.
	InstallCreated InstallStatus = "installed"
	// InstallUpdated: the skill was ours and the source moved on; files were
	// replaced, added, or removed, or written again after someone deleted them.
	InstallUpdated InstallStatus = "updated"
	// InstallUnchanged: the skill is ours and already holds the source's bytes.
	InstallUnchanged InstallStatus = "unchanged"
)

// InstallSkill is one skill to install: its registry id and the files of its source
// directory (readSkillSource).
type InstallSkill struct {
	ID    string
	Files []SourceFile
}

// InstallInput carries the pre-read state PlanInstallOwnership decides from. It
// reads nothing itself: the filesystem facts come through the three probes, so a
// test controls every path the planner sees and a refusal provably writes nothing.
type InstallInput struct {
	ProjectRoot string
	// ProjectID is the --project-id install was given; it is only put in the
	// refusal that points at adopt.
	ProjectID  string
	Skills     []InstallSkill
	LockData   []byte
	LockExists bool
	// Verb names the verb in the refusals: "install", which is the default, or
	// "adopt".
	Verb string

	ReadFile    func(string) ([]byte, error)
	Stat        func(string) (fs.FileInfo, error)
	ResolvePath func(string) (string, error)
	// ReadDir lists a directory. Only adopt needs it, to prove that a directory holds
	// the source and nothing else.
	ReadDir func(string) ([]fs.DirEntry, error)
}

func (in InstallInput) verb() string {
	if in.Verb == "" {
		return "install"
	}
	return in.Verb
}

// InstallOutcome is what the plan does to one skill.
type InstallOutcome struct {
	ID     string
	Status InstallStatus
}

// InstallPlan is the complete description of one install, produced before any
// write. Writes create or replace a file, Deletes remove an owned file the source
// dropped, and Lock is the rewrite of the project lock, the commit marker, which is
// the zero ProjectWrite (Rel == "") when no record changed. A refused install has the
// zero plan.
type InstallPlan struct {
	// Verb is the command that built the plan, "install" or "adopt", which the
	// executor names in its failures. The zero value means install.
	Verb    string
	Skills  []InstallOutcome
	Writes  []ProjectWrite
	Deletes []ProjectWrite
	Lock    ProjectWrite
	// Dirs are the skill directories the plan touches, absolute: after a removal,
	// the directories it leaves empty inside them are pruned, and only those.
	Dirs []string
	// Notes are things worth telling the person that are not refusals.
	Notes []string
}

func (p InstallPlan) verb() string {
	if p.Verb == "" {
		return "install"
	}
	return p.Verb
}

// planContext is what install and adopt both establish before they look at a skill:
// the project root, the shared containment rules, and the lock as it is.
type planContext struct {
	in          InstallInput
	root        string
	resolver    RegisterInput
	lock        ProjectLock
	procedural  map[string]bool
	recordIndex map[string]int
}

// newPlanContext checks the input and parses the lock. A problem here is a refusal
// of the whole verb.
func newPlanContext(in InstallInput) (*planContext, []string) {
	verb := in.verb()
	if in.ReadFile == nil || in.Stat == nil || in.ResolvePath == nil {
		return nil, []string{fmt.Sprintf("skills %s: the planner was given no filesystem probes", verb)}
	}
	if !filepath.IsAbs(in.ProjectRoot) {
		return nil, []string{fmt.Sprintf("skills %s: the project directory %q is not an absolute path", verb, in.ProjectRoot)}
	}
	root := filepath.Clean(in.ProjectRoot)
	c := &planContext{in: in, root: root, resolver: RegisterInput{ProjectRoot: root, ResolvePath: in.ResolvePath}, lock: ProjectLock{Version: 1}}
	if in.LockExists {
		parsed, err := ParseProjectLock(in.LockData)
		if err != nil {
			return nil, []string{fmt.Sprintf("skills %s: the project lock %s cannot be read, so nothing was changed and the lock was left as it is: %v", verb, ProjectLockRelPath, err)}
		}
		c.lock = parsed
	}
	c.procedural = make(map[string]bool, len(c.lock.Skills))
	for _, e := range c.lock.Skills {
		c.procedural[e.ID] = true
	}
	c.recordIndex = make(map[string]int, len(c.lock.Installs))
	for i, r := range c.lock.Installs {
		c.recordIndex[r.ID] = i
	}
	return c, nil
}

// record is the install record of id, or nil.
func (c *planContext) record(id string) *ProjectInstallEntry {
	if i, ok := c.recordIndex[id]; ok {
		return &c.lock.Installs[i]
	}
	return nil
}

// checkSkill refuses, on the verb's behalf, a skill neither verb may touch: one the
// procedural verbs own, or one with nothing to install.
func (c *planContext) checkSkill(sk InstallSkill, refuse func(string, ...any)) bool {
	if c.procedural[sk.ID] {
		refuse("%s is registered in the project lock as a procedural skill, written by project-register; %s does not touch it", sk.ID, c.in.verb())
		return false
	}
	if len(sk.Files) == 0 {
		// Shared by install and adopt, so the verb is the one that was asked. Both read
		// the source through the same rule (readSkillSource), so "what install copies" is
		// what adopt compares too.
		refuse("skill %s has no files to %s (its source directory holds nothing that skills install copies)", sk.ID, c.in.verb())
		return false
	}
	return true
}

// desiredRecord is the install record the source implies: every file, with the digest
// of the bytes the source holds.
func desiredRecord(sk InstallSkill) ProjectInstallEntry {
	desired := ProjectInstallEntry{ID: sk.ID}
	for _, f := range sk.Files {
		desired.Files = append(desired.Files, ProjectInstallFile{Path: f.Rel, SHA256: HashSkill(f.Data)})
	}
	sort.Slice(desired.Files, func(i, j int) bool { return desired.Files[i].Path < desired.Files[j].Path })
	return desired
}

// lockWrite is the write of the project lock with the given install records, the
// commit marker of the plan.
func (c *planContext) lockWrite(installs []ProjectInstallEntry) (ProjectWrite, []string) {
	verb := c.in.verb()
	lock := c.lock
	lock.Installs = installs
	lockData, err := SerializeProjectLock(lock)
	if err != nil {
		return ProjectWrite{}, []string{fmt.Sprintf("skills %s: %v", verb, err)}
	}
	lockAbs, _, err := resolveWritePath(c.resolver, c.root, ProjectLockRelPath)
	if err != nil {
		return ProjectWrite{}, []string{fmt.Sprintf("skills %s: destination %s: %v", verb, ProjectLockRelPath, err)}
	}
	w := ProjectWrite{Rel: ProjectLockRelPath, Abs: lockAbs, Data: lockData, Mode: ProjectFileMode}
	if c.in.LockExists {
		w.Backup = cloneProjectBytes(c.in.LockData)
	}
	return w, nil
}

// PlanInstallOwnership decides an install. It returns either a plan, or the reasons
// the install is refused, every one of them and not only the first, in which case
// nothing is to be written: one foreign directory, one edited file, or one unreadable
// path stops the whole invocation, so an install never leaves half a skill or half a
// project.
//
// The rules, per skill and per runtime directory (projectTargets):
//
//   - No install record, directory absent: every file is created, and recorded.
//   - No install record, directory present: refused. It is not ours; `skills adopt`
//     is the explicit way to take ownership of one that is exactly the source.
//   - A record, every recorded file on disk still as recorded: the files the source
//     changed are replaced, the ones it dropped are removed, the ones a person
//     deleted are written again, and a file the source did not change is not
//     written at all. A file in the directory that the record does not list is left
//     alone, unless the new source wants that very path: then it is refused.
//   - A record, a recorded file edited: refused, naming the file.
//   - An id that is a procedural skill in the same lock: refused; project-register
//     owns it.
func PlanInstallOwnership(in InstallInput) (InstallPlan, []string) {
	c, problems := newPlanContext(in)
	if problems != nil {
		return InstallPlan{}, problems
	}
	var refusals []string
	refuse := func(format string, a ...any) {
		refusals = append(refusals, fmt.Sprintf("skills %s: "+format, append([]any{in.verb()}, a...)...))
	}

	plan := InstallPlan{Verb: in.verb()}
	installs := append([]ProjectInstallEntry(nil), c.lock.Installs...)
	recordsChanged := false

	for _, sk := range in.Skills {
		if !c.checkSkill(sk, refuse) {
			continue
		}
		desired := desiredRecord(sk)
		record := c.record(sk.ID)

		writes, deletes, dirs, ok := planSkill(c, sk, record, &refusals)
		if !ok {
			continue
		}

		status := InstallUnchanged
		switch {
		case record == nil:
			status = InstallCreated
		case len(writes) > 0 || len(deletes) > 0 || !sameRecord(*record, desired):
			status = InstallUpdated
		}
		if record == nil || !sameRecord(*record, desired) {
			recordsChanged = true
			if record == nil {
				installs = append(installs, desired)
			} else {
				installs[c.recordIndex[sk.ID]] = desired
			}
		}
		plan.Skills = append(plan.Skills, InstallOutcome{ID: sk.ID, Status: status})
		plan.Writes = append(plan.Writes, writes...)
		plan.Deletes = append(plan.Deletes, deletes...)
		plan.Dirs = append(plan.Dirs, dirs...)
	}

	if len(refusals) > 0 {
		return InstallPlan{}, refusals
	}
	if recordsChanged {
		lock, problems := c.lockWrite(installs)
		if problems != nil {
			return InstallPlan{}, problems
		}
		plan.Lock = lock
	}
	return plan, nil
}

// sameRecord reports whether two install records list the same files with the same
// digests, in any order.
func sameRecord(a, b ProjectInstallEntry) bool {
	if a.ID != b.ID || len(a.Files) != len(b.Files) {
		return false
	}
	want := make(map[string]string, len(a.Files))
	for _, f := range a.Files {
		want[f.Path] = f.SHA256
	}
	for _, f := range b.Files {
		if want[f.Path] != f.SHA256 {
			return false
		}
	}
	return true
}

// planSkill plans one skill across the runtime directories. ok is false when it
// added refusals; its writes and deletes must then be dropped.
func planSkill(c *planContext, sk InstallSkill, record *ProjectInstallEntry, refusals *[]string) (writes, deletes []ProjectWrite, dirs []string, ok bool) {
	in, resolver, root := c.in, c.resolver, c.root
	before := len(*refusals)
	refuse := func(format string, a ...any) {
		*refusals = append(*refusals, fmt.Sprintf("skills %s: "+format, append([]any{in.verb()}, a...)...))
	}

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

	aliased := map[string]string{}
	for _, target := range projectTargets {
		dirRel := target.Dir + "/" + sk.ID
		dirAbs, resolvedDir, err := resolveWritePath(resolver, root, dirRel)
		if err != nil {
			refuse("destination %s: %v", dirRel, err)
			continue
		}
		if other, dup := aliased[resolvedDir]; dup {
			refuse("%s and %s resolve to the same directory, so one install would silently overwrite the other", other, dirRel)
			continue
		}
		aliased[resolvedDir] = dirRel

		info, err := in.Stat(dirAbs)
		exists := err == nil
		switch {
		case err != nil && !isAbsent(err):
			refuse("cannot inspect %s: %v", dirRel, err)
			continue
		case exists && !info.IsDir():
			refuse("%s exists and is not a directory", dirRel)
			continue
		}
		dirs = append(dirs, dirAbs)

		if record == nil && exists {
			refuse("%s already exists and was not installed by skills install (it has no install record in %s); if it is exactly the current skill, run `labdrian skills adopt --project-id %s` to take ownership of it, otherwise move it away",
				dirRel, ProjectLockRelPath, in.ProjectID)
			continue
		}

		// What is on disk now, for every recorded file, checked against its record.
		current := map[string][]byte{}
		var modes = map[string]fs.FileMode{}
		if record != nil && exists {
			for _, f := range record.Files {
				rel := dirRel + "/" + f.Path
				abs, _, err := resolveWritePath(resolver, root, rel)
				if err != nil {
					refuse("destination %s: %v", rel, err)
					continue
				}
				data, mode, status := readInstalled(in, abs)
				switch status {
				case fileMissing:
					continue
				case fileNotRegular:
					refuse("%s is not a regular file, although skills install wrote a file there; move it away and run install again", rel)
					continue
				case fileUnreadable:
					refuse("cannot read %s", rel)
					continue
				}
				if HashSkill(data) != f.SHA256 {
					refuse("%s was edited since skills install wrote it (its bytes no longer match the install record); restore it or move it away, then run install again", rel)
					continue
				}
				current[f.Path] = data
				modes[f.Path] = mode
			}
		}

		for _, f := range sk.Files {
			rel := dirRel + "/" + f.Rel
			abs, _, err := resolveWritePath(resolver, root, rel)
			if err != nil {
				refuse("destination %s: %v", rel, err)
				continue
			}
			if _, isRecorded := recorded[f.Rel]; isRecorded && exists {
				cur, present := current[f.Rel]
				switch {
				case !present:
					// Recorded and gone: someone deleted it, and install writes it again.
					writes = append(writes, ProjectWrite{Rel: rel, Abs: abs, Data: f.Data, Mode: f.Mode})
				case bytes.Equal(cur, f.Data):
					// Already what the source holds: not written.
				default:
					writes = append(writes, ProjectWrite{Rel: rel, Abs: abs, Data: f.Data, Mode: f.Mode, Backup: cloneProjectBytes(cur)})
				}
				continue
			}
			if exists {
				// Not recorded. If anything is there, it is not ours to overwrite.
				if _, err := in.Stat(abs); err == nil {
					refuse("%s exists but is not recorded as installed by skills install, and the source now wants to write it; move it away and run install again", rel)
					continue
				} else if !isAbsent(err) {
					refuse("cannot inspect %s: %v", rel, err)
					continue
				}
			}
			writes = append(writes, ProjectWrite{Rel: rel, Abs: abs, Data: f.Data, Mode: f.Mode})
		}

		for _, p := range dropped {
			cur, present := current[p]
			if !present {
				continue
			}
			rel := dirRel + "/" + p
			abs, _, err := resolveWritePath(resolver, root, rel)
			if err != nil {
				refuse("destination %s: %v", rel, err)
				continue
			}
			deletes = append(deletes, ProjectWrite{Rel: rel, Abs: abs, Mode: modes[p], Backup: cloneProjectBytes(cur)})
		}
	}
	return writes, deletes, dirs, len(*refusals) == before
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
