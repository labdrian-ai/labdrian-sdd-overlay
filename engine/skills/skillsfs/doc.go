// Package skillsfs is the adapter of the skills domain to the file system: the tree of skills an
// overlay keeps, which it walks and reads for engine/skills (SkillTree), and, as the later slices
// of Phase 9 unit H17 move them here, the files of a project and the writes of an overlay.
//
// The domain decides what is content and what is not (its names and its rules are
// skills.SkipWhenCopying and the dot-name rule it documents on the port); this package walks a
// real directory and applies them. It imports the domain and nothing the domain does not allow:
// the dependency points from the adapter to the domain, never back.
package skillsfs
