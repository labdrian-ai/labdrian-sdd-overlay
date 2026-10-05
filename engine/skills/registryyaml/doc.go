// Package registryyaml is the YAML file of the skills registry (skills.registry.yaml), as an
// adapter of skills.RegistryRepository: it reads the file into the registry model the skills
// domain owns, and writes the model back as the one form of the file the program writes.
//
// It knows the file and nothing of what a registry means. Whether a registry may hold what it
// holds is the domain's rule (skills.Registry.Validate), applied by the domain to what this package
// returns (skills.ReadRegistry); nothing in this package calls it, and a test fails if something
// does. The two checks it keeps that name a line (a scope outside its two words, a source that
// contradicts its type) are checks of the file, worded with the line they are on, and the domain
// holds every registry to the same rules again.
//
// The file format is a strict subset of YAML (see Decode for what is refused and why, Encode for
// the one form that is written).
//
// # The version rule
//
// The file says which format it is in: its top-level "version". The reader finds it first and
// dispatches on it. Each version has a decoder of its own (decoders, in versions.go, and the table
// of its fields, in schema.go), and a version no decoder is for is refused, naming it, before
// anything else of the file is read, so the entries of a format this reader does not know are
// never read the wrong way. Version 1 is the only one. A change to the format that a reader of the
// version before it would misread is a new version, with a new decoder beside the old ones; a
// change that only adds a field is not.
//
// # The reader policy (decision Q5 of Phase 9)
//
// The reader is tolerant. A field it does not know, in any mapping of the file, is left out and
// said (skills.Registry.Unread has one note for it, with its line), and what is under it is
// skipped. The verbs that read a registry warn of what was left out and go on with the rest. A
// registry that left something out cannot be written back whole, so the verbs that change it (add,
// remove) refuse it, and so does Encode. (pipkg reads the registry too, and does not warn: what it
// builds a package from, an entry's id, path and install.targets, is all of the must-understand
// set.)
//
// What the reader never does is read a field of the must-understand set without understanding it.
// Such a field in a shape the decoder does not read (a value where a block belongs, a block where a
// value belongs) is refused, naming the field and the line, instead of being left out. Any other
// field in a shape that is not its own has the part that is in the wrong shape left out and said,
// like a key the reader does not know: the block under a value that should be one is skipped, and
// the value on the line of a block is noted and the block under it is read.
//
// The must-understand set of version 1 is the fields whose meaning changes what install, approval or
// projection does, found by reading what consumes each field. A registry read without one of them
// would still be a registry and would say something else, with nothing said (a skill with no scope
// skips the approval check, a project skill with no projects is admitted nowhere). MustUnderstand
// returns the set, and a test holds this list to it:
//
//	version                  selects the decoder: every other field is read in the format it names
//	skills                   the entries: a value there is not an empty list, and nothing would be installed
//	id                       names the skill in the install directory and in the project lock
//	path                     the directory the skill is read from, and the containment boundary of install and pipkg
//	source                   holds source.type
//	source.type              decides the manifest tag (managed or custom), so how sync-manifest and validate treat it
//	install                  holds the scope, the projects and the targets
//	install.defaultScope     global or project: where it is installed, and a scope read as none skips the approval check
//	install.allowedProjects  the projects install admits a project skill to: read as empty, it admits it to none
//	install.targets          the runtimes it is projected to: the membership of the Pi package, and what list prints
//
// Fields the reader may leave out of a shape it cannot read, because nothing decides on them:
// source.upstream and its owner, source.repo and source.ref (provenance, which only the domain's
// rule on the source type looks at), and lifecycle and its updateStrategy (shown by list, and
// required by the domain's rule). The domain still refuses a registry that lacks one it requires.
package registryyaml
