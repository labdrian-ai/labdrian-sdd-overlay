package skills

// SkillTree is the port through which the skills domain reads the tree of skills an overlay
// keeps and the source of one skill. It is owned here and implemented by an adapter that walks a
// real directory (engine/skills/skillsfs): the domain knows what it wants to learn of a tree,
// not how a tree is walked.
//
// What an adapter lists and reads is decided by rules of the domain, which the adapter applies
// and does not invent: which names are content (ScanSkillFiles leaves out every name that begins
// with a dot) and which files are never copied (SkipWhenCopying).
type SkillTree interface {
	// ScanSkillFiles lists every regular file below skillsDir as a slash-separated path relative
	// to skillsDir, sorted. A name that begins with a dot is left out, a directory with it, so
	// that editor scratch files and version-control metadata are not reported as content; so is
	// anything that is not a regular file, and a link is not followed. An error says the
	// directory could not be inspected or walked.
	ScanSkillFiles(skillsDir string) ([]string, error)
	// ReadSkillSource reads the files `skills install` would install from one skill directory,
	// sorted by path, with their bytes and permission bits. A link is not followed and not
	// installed, nor is anything that is not a regular file; a directory is only the way to its
	// files, so an empty one installs nothing; the files SkipWhenCopying names are left out.
	ReadSkillSource(dir string) ([]SourceFile, error)
}
