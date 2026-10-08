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
	proof, err := newDestinationProof(verb, fsys, root)
	if err != nil {
		return err
	}
	seen := make(map[string]string, len(order))
	for _, w := range order {
		if err := proof.check(w, seen); err != nil {
			return err
		}
	}
	return nil
}

// destinationProof is what is established once for a whole set of writes: the project root and the
// project's own skills/ tree, both resolved.
type destinationProof struct {
	verb           string
	fsys           ProjectFS
	root           string
	resolvedRoot   string
	resolvedSkills string
}

// fail words a refusal for the verb that is executing the plan.
func (p destinationProof) fail(format string, a ...any) error {
	return fmt.Errorf("%s: "+format, append([]any{p.verb}, a...)...)
}

func newDestinationProof(verb string, fsys ProjectFS, root string) (destinationProof, error) {
	proof := destinationProof{verb: verb, fsys: fsys, root: root}
	resolvedRoot, err := fsys.ResolvePath(root)
	if err != nil {
		return proof, proof.fail("resolving the project root %q: %w", root, err)
	}
	if resolvedRoot == "" {
		return proof, proof.fail("the resolver returned an empty path for the project root %q", root)
	}
	proof.resolvedRoot = filepath.Clean(resolvedRoot)

	// Decision (f), resolved once for the whole set (review round 4, D4). The
	// planner re-applies it to the RESOLVED destination precisely because the
	// repo-relative string cannot see a `.claude/skills` symlinked at the
	// project's own source tree; the executor's re-proof dropped that half, so a
	// symlink created after planning was written straight through.
	resolvedSkills, err := fsys.ResolvePath(filepath.Join(root, projectSourceSkillsDir))
	if err != nil {
		return proof, proof.fail("resolving the project's own %s/ directory: %w", projectSourceSkillsDir, err)
	}
	if resolvedSkills == "" {
		return proof, proof.fail("the resolver returned an empty path for the project's own %s/ directory", projectSourceSkillsDir)
	}
	proof.resolvedSkills = filepath.Clean(resolvedSkills)
	return proof, nil
}

// check re-proves one write: its path is the root joined with its own repo-relative path, it lies
// strictly below the root lexically and after resolution, it is not under the project's own
// skills/ tree, no earlier write resolved to the same physical path (seen), and it still exists
// exactly as the plan found it.
func (p destinationProof) check(w ProjectWrite, seen map[string]string) error {
	if w.Rel == "" || w.Abs == "" {
		return p.fail("the plan carries a write with no path")
	}
	abs := filepath.Clean(w.Abs)
	if want := filepath.Join(p.root, filepath.FromSlash(w.Rel)); abs != want {
		return p.fail("write %q resolves to %q, want %q", w.Rel, abs, want)
	}
	if !pathguard.WithinRoot(p.root, abs) {
		return p.fail("destination %q escapes the project root", w.Rel)
	}
	resolved, err := p.fsys.ResolvePath(abs)
	if err != nil {
		return p.fail("destination %q could not be resolved: %w", w.Rel, err)
	}
	if resolved == "" {
		return p.fail("the resolver returned an empty path for destination %q", w.Rel)
	}
	resolved = filepath.Clean(resolved)
	if !pathguard.WithinRoot(p.resolvedRoot, resolved) {
		return p.fail("destination %q escapes the project root through a symlink", w.Rel)
	}
	if underSkillsDir(w.Rel) {
		return p.fail("destination %q %v", w.Rel, errDestUnderSkillsDir)
	}
	if pathguard.WithinRoot(p.resolvedSkills, resolved) || resolved == p.resolvedSkills {
		return p.fail("destination %q %v", w.Rel, errDestResolvesUnderSkillsDir)
	}
	if other, ok := seen[resolved]; ok {
		return p.fail("destinations %q and %q resolve to the same file, so one would silently overwrite the other", other, w.Rel)
	}
	seen[resolved] = w.Rel
	return p.checkStillAsPlanned(w, abs)
}

// checkStillAsPlanned re-establishes what the plan recorded, per write, about whether the
// destination existed: Backup holds its bytes when it did and is nil when it did not. That fact
// decides rollback's restore-vs-remove, and it is as point-in-time as the containment proof.
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
func (p destinationProof) checkStillAsPlanned(w ProjectWrite, abs string) error {
	_, statErr := p.fsys.Stat(abs)
	switch {
	case statErr == nil && w.Backup == nil:
		return p.fail("destination %q already exists although the plan found it absent; it was created after the plan was built and this run will not overwrite it", w.Rel)
	case statErr != nil && isAbsent(statErr) && w.Backup != nil:
		return p.fail("destination %q no longer exists although the plan captured its contents; it was removed after the plan was built", w.Rel)
	case statErr != nil && !isAbsent(statErr):
		return p.fail("inspecting destination %q: %w", w.Rel, statErr)
	}
	return nil
}
