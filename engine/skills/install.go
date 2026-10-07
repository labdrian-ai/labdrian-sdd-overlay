package skills

import (
	"fmt"
	"io/fs"
	"path/filepath"
)

// CopyOp is one skill to install: where its source tree is and where the first
// runtime's copy of it goes. The second runtime's copy (.agents/skills/<id>) is the
// same tree in the projectTargets table; PlanInstallOwnership derives both.
type CopyOp struct {
	SkillID string // entry id, used for output messages
	Src     string // <sourceRoot>/<entry.Path>
	Dst     string // <targetRoot>/.claude/skills/<entry.ID>
}

// PlanInstall filters reg for project-scoped skills allowed for projectID,
// building one CopyOp per admitted entry. Pure: no filesystem access.
// Declaration order from reg.Skills is preserved in the returned slice.
// Returns a non-nil error if any entry's id or path contains a traversal
// sequence that would place Dst outside <targetRoot>/.claude/skills/ or
// Src outside sourceRoot (R-055).
func PlanInstall(reg Registry, projectID ProjectID, sourceRoot, targetRoot string) ([]CopyOp, error) {
	// Pre-compute clean containment roots for traversal checks. withinRoot
	// (pathguard.go) requires already-cleaned arguments.
	srcRoot := filepath.Clean(sourceRoot)
	dstRoot := filepath.Clean(filepath.Join(targetRoot, ".claude", "skills"))

	var ops []CopyOp
	for _, e := range AdmittedToProject(reg, projectID) {
		src := filepath.Clean(filepath.Join(sourceRoot, e.Path))
		dst := filepath.Clean(filepath.Join(targetRoot, ".claude", "skills", e.ID))

		// R-055: reject traversal in both src and dst (fail-loud, pure).
		// The containment test itself is withinRoot (pathguard.go), shared
		// with resolveTarget and EvaluateOwnership; the two branches keep one
		// message each so a test can prove which one it reached
		// (review-d89971d41a526146 precedent).
		if !withinRoot(srcRoot, src) {
			return nil, fmt.Errorf("skill %q: path %q escapes source root — possible traversal", e.ID, e.Path)
		}
		if !withinRoot(dstRoot, dst) {
			return nil, fmt.Errorf("skill %q: id %q escapes target skills root — possible traversal", e.ID, e.ID)
		}

		ops = append(ops, CopyOp{
			SkillID: e.ID,
			Src:     src,
			Dst:     dst,
		})
	}
	return ops, nil
}

// AdmittedToProject is the entries of reg that install to a project of that id: the ones whose
// default scope is the project and whose allowed projects name it, in the order of the registry.
func AdmittedToProject(reg Registry, projectID ProjectID) []Entry {
	var admitted []Entry
	for _, e := range reg.Skills {
		if e.Install.DefaultScope == "project" && containsString(e.Install.AllowedProjects, projectID.String()) {
			admitted = append(admitted, e)
		}
	}
	return admitted
}

// containsString reports whether slice contains s (case-sensitive).
func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// isWriterTempFile reports whether d is a regular file that writeFileAtomic made
// and has not yet renamed into place: the temporary-file prefix followed by its
// unique suffix. It is half of a write another verb is doing in the source tree,
// not skill content. install also holds the overlay lock, which keeps those verbs
// out while it reads the tree; this is the second defence, for a writer that did not
// take it. The prefix alone, with no suffix, is not a name writeFileAtomic makes, and
// a directory with such a name is walked like any other.
func isWriterTempFile(d fs.DirEntry) bool {
	name := d.Name()
	return !d.IsDir() && len(name) > len(atomicTempPrefix) && name[:len(atomicTempPrefix)] == atomicTempPrefix
}
