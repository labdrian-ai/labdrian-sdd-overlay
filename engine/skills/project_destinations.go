package skills

import (
	"fmt"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pathguard"
)

// checkProjectDestinations re-proves, at execution time, what
// PlanProjectRegister proved when the plan was built: every destination is the
// root joined with its own repo-relative path, lies strictly below the root
// lexically AND after symlink resolution, no destination lies or LANDS under
// the project's own skills/ tree (decision (f)), no two destinations resolve to
// the SAME physical path, and each destination still exists exactly as the plan
// found it.
//
// Every one of those is point-in-time in the plan, which is the whole reason
// this function exists; re-proving only some of them was the gap D4 and D3
// named (review round 4).
//
// verb is how every refusal is worded: the name of the command that is executing
// the plan, "project-register" for the project verbs and "skills install" or
// "skills adopt" for the two that share this executor.
func checkProjectDestinations(verb string, fsys ProjectFS, root string, order []ProjectWrite) error {
	fail := func(format string, a ...any) error { return fmt.Errorf("%s: "+format, append([]any{verb}, a...)...) }
	resolvedRoot, err := fsys.ResolvePath(root)
	if err != nil {
		return fail("resolving the project root %q: %w", root, err)
	}
	if resolvedRoot == "" {
		return fail("the resolver returned an empty path for the project root %q", root)
	}
	resolvedRoot = filepath.Clean(resolvedRoot)

	// Decision (f), resolved once for the whole set (review round 4, D4). The
	// planner re-applies it to the RESOLVED destination precisely because the
	// repo-relative string cannot see a `.claude/skills` symlinked at the
	// project's own source tree; the executor's re-proof dropped that half, so a
	// symlink created after planning was written straight through.
	resolvedSkills, err := fsys.ResolvePath(filepath.Join(root, projectSourceSkillsDir))
	if err != nil {
		return fail("resolving the project's own %s/ directory: %w", projectSourceSkillsDir, err)
	}
	if resolvedSkills == "" {
		return fail("the resolver returned an empty path for the project's own %s/ directory", projectSourceSkillsDir)
	}
	resolvedSkills = filepath.Clean(resolvedSkills)

	seen := make(map[string]string, len(order))
	for _, w := range order {
		if w.Rel == "" || w.Abs == "" {
			return fail("the plan carries a write with no path")
		}
		abs := filepath.Clean(w.Abs)
		if want := filepath.Join(root, filepath.FromSlash(w.Rel)); abs != want {
			return fail("write %q resolves to %q, want %q", w.Rel, abs, want)
		}
		if !pathguard.WithinRoot(root, abs) {
			return fail("destination %q escapes the project root", w.Rel)
		}
		resolved, err := fsys.ResolvePath(abs)
		if err != nil {
			return fail("destination %q could not be resolved: %w", w.Rel, err)
		}
		if resolved == "" {
			return fail("the resolver returned an empty path for destination %q", w.Rel)
		}
		resolved = filepath.Clean(resolved)
		if !pathguard.WithinRoot(resolvedRoot, resolved) {
			return fail("destination %q escapes the project root through a symlink", w.Rel)
		}
		if underSkillsDir(w.Rel) {
			return fail("destination %q %v", w.Rel, errDestUnderSkillsDir)
		}
		if pathguard.WithinRoot(resolvedSkills, resolved) || resolved == resolvedSkills {
			return fail("destination %q %v", w.Rel, errDestResolvesUnderSkillsDir)
		}
		if other, ok := seen[resolved]; ok {
			return fail("destinations %q and %q resolve to the same file, so one would silently overwrite the other", other, w.Rel)
		}
		seen[resolved] = w.Rel

		// The plan recorded, per write, whether the destination existed: Backup
		// holds its bytes when it did and is nil when it did not. That fact
		// decides rollback's restore-vs-remove, and it is as point-in-time as
		// the containment proof, so it is re-established here too.
		//
		// A destination the planner found ABSENT that is now present is the
		// planner's "foreign skill" arriving late: writing over it would destroy
		// a file this run did not create, and rolling back would DELETE it
		// (review round 4, D3). It is refused rather than backed up, because
		// answering a race more permissively than the look-first path would mean
		// the tool overwrites on a race exactly what it refuses to overwrite
		// when it checks in time. Refusing costs nothing: nothing has been
		// written yet, so there is nothing to undo.
		//
		// A destination the planner found PRESENT that is now absent is the
		// mirror image: its backup bytes would be laid down as a brand-new file
		// nobody asked this run to create.
		_, statErr := fsys.Stat(abs)
		switch {
		case statErr == nil && w.Backup == nil:
			return fail("destination %q already exists although the plan found it absent; it was created after the plan was built and this run will not overwrite it", w.Rel)
		case statErr != nil && isAbsent(statErr) && w.Backup != nil:
			return fail("destination %q no longer exists although the plan captured its contents; it was removed after the plan was built", w.Rel)
		case statErr != nil && !isAbsent(statErr):
			return fail("inspecting destination %q: %w", w.Rel, statErr)
		}
	}
	return nil
}
