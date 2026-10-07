package skills

import "io/fs"

// FileReader reads a file by name: the port the verbs read the files of an overlay and of a
// project through (the registry's neighbours, the manifest, the approval records, a SKILL.md to
// lint). The production one is os.ReadFile, which the composition root chooses.
type FileReader func(name string) ([]byte, error)

// FileStatter says what a path is: the port a verb asks whether a file is there.
type FileStatter func(name string) (fs.FileInfo, error)

// Deps is everything the verbs of `engine skills` reach outside themselves: the one value the
// composition root (engine/cmd) builds and hands to the adapter of each verb, which hands its use
// case the part it needs. Every field is a port of this package or a function of the process; the domain
// holds no os call of its own (Phase 9 unit H17). A field left nil is a wiring that was
// forgotten: a verb that needs it refuses, or, for the lock and the clock, says so.
type Deps struct {
	// ReadFile reads a file by name: the registry's neighbours, the manifest, the approval
	// records, the files of a project.
	ReadFile FileReader
	// Registries reads and encodes the registry of an overlay.
	Registries RegistryRepository
	// Approvals reads the evidence of an approval: the SKILL.md of a global skill and the record
	// beside it. The verbs that judge approval (validate, add, approve) refuse without it.
	Approvals ApprovalRecordStore
	// Tree walks the tree of skills an overlay keeps and reads the source of one skill.
	Tree SkillTree
	// Project reads and writes the files of a project and of an overlay: what install, adopt
	// and the project verbs stage, commit and put back.
	Project ProjectFS
	// ProjectLocks reads the lock of a project: what install, adopt and the project verbs decide
	// from. Each refuses without it.
	ProjectLocks ProjectLockStore
	// Cwd names the working directory, which `skills install` and `adopt` install into.
	Cwd func() (string, error)
	// Identity says which project the working directory is, for the verbs that admit skills to a
	// project by its id (install, adopt). The chain of sources it asks, and their order, is the
	// composition root's; a nil Identity is a wiring that was forgotten, and those verbs refuse.
	Identity ProjectIdentity
	// Locker takes the advisory locks of the verbs that need them.
	Locker Locker
	// Now returns the current time as an RFC 3339 UTC timestamp, for the verbs that record one
	// (approve). A nil Now refuses the approval: the verb never invents a time.
	Now func() string
}
