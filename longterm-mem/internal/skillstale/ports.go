package skillstale

// The ports below are what the detector asks of the world. It owns them: they are shaped by what it needs to
// know, in its own words, and the packages that answer them (internal/skillstalefs on a file system and a
// repository, the observation store behind ObservationLister) are chosen by whoever composes the program. The
// detector itself touches no file, no environment variable, no process and no clock.

// SkillSource is where a project keeps the skills the detector looks at: the lock that lists them, and the
// skill file each entry points at. Both are read-only questions in the names the lock uses, which are paths
// relative to the project, never paths of the machine; the source decides how a name becomes bytes, and refuses a
// name that leaves the project.
//
// It is also the shape a project-context port takes elsewhere: ask for a file by the project-relative name the
// project's own records use, get its bytes or the error that says why not, and keep the project's location to
// the adapter. The procedural-memory presence check (C10) asks the same question of the same lock.
type SkillSource interface {
	// ReadLock returns the bytes of the project lock (ProjectLockRelPath).
	ReadLock() ([]byte, error)
	// ReadSkill returns the bytes of the skill file the lock records at target, a slash-separated path
	// relative to the project. A target that is empty, absolute, climbs out of the project, or resolves to a
	// file outside it, is an error and nothing is read.
	ReadSkill(target string) ([]byte, error)
}

// PathState is what the repository says about a path a skill names.
type PathState string

const (
	// PathPresent: the path is in the working tree.
	PathPresent PathState = "present"
	// PathDeleted: the path was in the history and a commit removed it.
	PathDeleted PathState = "deleted"
	// PathRenamed: the path's content moved to NewPath.
	PathRenamed PathState = "renamed"
	// PathUnknown: the repository cannot say what became of the path (it never held it, or the evidence
	// does not decide). The detector reports nothing for it.
	PathUnknown PathState = "unknown"
)

// PathFact is the repository's evidence for one path: what became of it, in which commit, and where it went when
// it was renamed.
type PathFact struct {
	State   PathState
	NewPath string
	Commit  string
}

// PathFactSource answers what the project's repository says about the paths a skill names. It returns a fact for
// every path asked about; a path it has nothing on is PathUnknown.
type PathFactSource interface {
	PathFacts(paths []string) (map[string]PathFact, error)
}

// CommandResolver says whether a command a skill tells its reader to run can be found where the reader would run
// it. Where that is (a PATH, a set of directories, a table in a test) is the resolver's.
type CommandResolver interface {
	ResolvesCommand(command string) bool
}
