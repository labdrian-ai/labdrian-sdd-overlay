package skills

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// InstallAdopted: the skill was already in the project, exactly as the source has it,
// and is now recorded as installed.
const InstallAdopted InstallStatus = "adopted"

// PlanAdopt decides `skills adopt`: which existing skill directories become ones
// `skills install` owns. It only ever records. The plan has no file writes; its one
// write is the project lock, and it is refused, every reason reported, when any skill
// cannot be adopted, so an adopt never takes half of what it was asked to.
//
// Adopting is taking someone's word for what a directory is, so the bar is exact: a
// directory is adopted only when it holds the same files as the current source, with
// the same bytes, and nothing else. When it does not, the refusal names each file that
// differs: other bytes, missing, or not in the source.
//
// Per skill:
//
//   - No record, a runtime directory that is exactly the source: adopted. One runtime
//     is enough; a runtime whose directory is absent is left for the next install to
//     write, and the plan says so in a note.
//   - No record and neither runtime has the skill: refused; there is nothing to adopt.
//   - A record, the directory as recorded and the source unchanged: unchanged.
//   - A record and the source has moved on, or a recorded file was edited: refused.
//     Adopt does not update; `skills install` does.
func PlanAdopt(in InstallInput) (InstallPlan, []string) {
	in.Verb = "adopt"
	c, problems := newPlanContext(in)
	if problems != nil {
		return InstallPlan{}, problems
	}
	if in.ReadDir == nil {
		return InstallPlan{}, []string{"skills adopt: the planner was given no directory probe"}
	}
	var refusals []string
	refuse := func(format string, a ...any) {
		refusals = append(refusals, fmt.Sprintf("skills adopt: "+format, a...))
	}

	plan := InstallPlan{Verb: in.verb()}
	installs := append([]ProjectInstallEntry(nil), c.lock.Installs...)
	recordsChanged := false

	for _, sk := range in.Skills {
		if !c.checkSkill(sk, refuse) {
			continue
		}
		desired := desiredRecord(sk)

		if record := c.record(sk.ID); record != nil {
			writes, deletes, _, ok := planSkill(c, sk, record, &refusals)
			if !ok {
				continue
			}
			if len(writes) > 0 || len(deletes) > 0 || !sameRecord(*record, desired) {
				refuse("%s is already installed by skills install and is not current with the source; run `labdrian skills install --project-id %s` to update it (adopt only takes ownership of a directory that is exactly the source)", sk.ID, in.ProjectID)
				continue
			}
			plan.Skills = append(plan.Skills, InstallOutcome{ID: sk.ID, Status: InstallUnchanged})
			continue
		}

		present, notes, ok := checkAdoptable(c, sk, &refusals)
		if !ok {
			continue
		}
		if present == 0 {
			refuse("skill %s is not installed in this project (neither %s exists), so there is nothing to adopt; run `labdrian skills install --project-id %s`", sk.ID, projectTargetDirs(sk.ID), in.ProjectID)
			continue
		}
		installs = append(installs, desired)
		recordsChanged = true
		plan.Skills = append(plan.Skills, InstallOutcome{ID: sk.ID, Status: InstallAdopted})
		plan.Notes = append(plan.Notes, notes...)
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

// projectTargetDirs names the skill's directory in every runtime, for a message.
func projectTargetDirs(id string) string {
	var dirs []string
	for _, t := range projectTargets {
		dirs = append(dirs, t.Dir+"/"+id)
	}
	return strings.Join(dirs, " nor ")
}

// checkAdoptable looks at the skill in each runtime directory and reports how many
// hold it, exactly as the source has it. A directory that holds something else is a
// refusal that says what differs; a runtime where the skill is absent is a note.
func checkAdoptable(c *planContext, sk InstallSkill, refusals *[]string) (present int, notes []string, ok bool) {
	in := c.in
	before := len(*refusals)
	refuse := func(format string, a ...any) {
		*refusals = append(*refusals, fmt.Sprintf("skills adopt: "+format, a...))
	}

	aliased := map[string]string{}
	for _, target := range projectTargets {
		dirRel := target.Dir + "/" + sk.ID
		dirAbs, resolvedDir, err := resolveWritePath(c.resolver, c.root, dirRel)
		if err != nil {
			refuse("destination %s: %v", dirRel, err)
			continue
		}
		if other, dup := aliased[resolvedDir]; dup {
			refuse("%s and %s resolve to the same directory, so they cannot both be adopted", other, dirRel)
			continue
		}
		aliased[resolvedDir] = dirRel

		info, err := in.Stat(dirAbs)
		switch {
		case err != nil && isAbsent(err):
			notes = append(notes, fmt.Sprintf("%s is not installed; run `labdrian skills install --project-id %s` to add it", dirRel, in.ProjectID))
			continue
		case err != nil:
			refuse("cannot inspect %s: %v", dirRel, err)
			continue
		case !info.IsDir():
			refuse("%s exists and is not a directory", dirRel)
			continue
		}

		diffs, err := differencesFromSource(in, dirAbs, sk)
		if err != nil {
			refuse("cannot read %s: %v", dirRel, err)
			continue
		}
		if len(diffs) > 0 {
			refuse("%s is not exactly the current skill, so it cannot be adopted: %s", dirRel, summarize(diffs))
			continue
		}
		present++
	}
	return present, notes, len(*refusals) == before
}

// differencesFromSource lists, sorted, how the directory differs from the skill's
// source: a file with other bytes, a file the source has that is missing, a file the
// source does not have, and anything in it that is not a regular file or a directory.
func differencesFromSource(in InstallInput, dir string, sk InstallSkill) ([]string, error) {
	found := map[string]string{} // slash path -> absolute path
	var odd []string
	var walk func(abs, rel string) error
	walk = func(abs, rel string) error {
		entries, err := in.ReadDir(abs)
		if err != nil {
			return err
		}
		for _, e := range entries {
			childRel := e.Name()
			if rel != "" {
				childRel = rel + "/" + e.Name()
			}
			childAbs := abs + string(filepath.Separator) + e.Name()
			switch {
			case e.IsDir():
				if err := walk(childAbs, childRel); err != nil {
					return err
				}
			case e.Type().IsRegular():
				found[childRel] = childAbs
			default:
				odd = append(odd, childRel)
			}
		}
		return nil
	}
	if err := walk(dir, ""); err != nil {
		return nil, err
	}

	var diffs []string
	source := make(map[string]bool, len(sk.Files))
	for _, f := range sk.Files {
		source[f.Rel] = true
		abs, present := found[f.Rel]
		if !present {
			diffs = append(diffs, f.Rel+" is missing")
			continue
		}
		data, err := in.ReadFile(abs)
		if err != nil {
			return nil, err
		}
		if HashSkill(data) != HashSkill(f.Data) {
			diffs = append(diffs, f.Rel+" has other bytes")
		}
	}
	for rel := range found {
		if !source[rel] {
			diffs = append(diffs, rel+" is not in the source")
		}
	}
	for _, rel := range odd {
		diffs = append(diffs, rel+" is not a regular file")
	}
	sort.Strings(diffs)
	return diffs, nil
}

// summarize joins differences into one message line, naming the first few and
// counting the rest.
func summarize(diffs []string) string {
	const shown = 8
	if len(diffs) <= shown {
		return strings.Join(diffs, "; ")
	}
	return fmt.Sprintf("%s; and %d more", strings.Join(diffs[:shown], "; "), len(diffs)-shown)
}
