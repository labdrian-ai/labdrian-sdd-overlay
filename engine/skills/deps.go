package skills

// Deps is everything the verbs of `engine skills` reach outside themselves: the one value the
// composition root (engine/cmd) builds and hands to SkillsCoreAt, which hands each verb the part
// it needs. Every field is a port of this package or a function of the process; the domain
// holds no os call of its own (Phase 9 unit H17). A field left nil is a wiring that was
// forgotten: a verb that needs it refuses, or, for the lock and the clock, says so.
type Deps struct {
	// ReadFile reads a file by name: the registry's neighbours, the manifest, the approval
	// records, the files of a project.
	ReadFile readFileFn
	// Registries reads and encodes the registry of an overlay.
	Registries RegistryRepository
	// Tree walks the tree of skills an overlay keeps and reads the source of one skill.
	Tree SkillTree
	// Locker takes the advisory locks of the verbs that need them.
	Locker Locker
	// Now returns the current time as an RFC 3339 UTC timestamp, for the verbs that record one
	// (approve). A nil Now refuses the approval: the verb never invents a time.
	Now func() string
}
