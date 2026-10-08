// Package skillsfs is the adapter of the skills domain to the file system. It implements the
// ports engine/skills owns and the program wires in engine/cmd: Tree, the tree of skills an
// overlay keeps (skills.SkillTree), and Project, the files of a project and the staged writes of
// an overlay (skills.ProjectFS, whose StagedWrites are how the registry, the manifest and the
// approval records are written).
//
// The domain decides what is content and what is not (skills.SkipWhenCopying and the dot-name
// rule it documents on SkillTree) and how a failure is worded for a verb; this package walks a
// real directory, reads and writes real files, and says the step that failed. It imports the
// domain and nothing else of the module but pathguard/fsresolve, whose resolution of links it calls; the
// dependency points from the adapter to the domain, never back.
//
// A staged write is the temporary file the standard library makes (named ".tmp-skills-" and a
// number), written, synced, closed and set to the mode asked for, and then renamed into place by
// the caller. engine/atomicfile is the one copy of the same idea that also refuses to replace a
// link and flushes the directory; moving these writes onto it would change what a verb does to
// a link at its destination and the words of a failure, and is a decision of the owner (Phase 9
// ledger, batch 11).
package skillsfs
