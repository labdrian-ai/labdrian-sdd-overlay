package skills

import (
	"io/fs"
)

// ProjectTarget is one runtime-visible directory a project-tier procedural
// skill is written to. Name is the runtime label used in output; Dir is the
// repo-relative, slash-separated directory that holds <id>/SKILL.md.
type ProjectTarget struct{ Name, Dir string }

// projectTargets is the FIXED, ordered target table (design.md, "Fixed
// table"; contract section 10). It is never `.pi/skills`, and no code
// branches on the target name or on the Codex smoke-test verdict — the Codex
// status cell in the contract is prose only.
// TestProjectTargetsMatchContractTable pins these rows against the contract
// document.
var projectTargets = []ProjectTarget{
	{"claude", ".claude/skills"},
	{"agents", ".agents/skills"},
}

// ProjectFileMode is the mode of every file a project registration writes:
// 0644, never executable (design.md, "Execution"; threat matrix).
const ProjectFileMode fs.FileMode = 0o644

// PiTrustNote is the disclosure every SUCCESSFUL project-register run prints
// (design.md, "Pi trust note"; contract section 10). Writing `.agents/skills`
// in a project Pi has not yet trusted makes Pi prompt once for project trust
// on its next start, and a non-interactive Pi run ignores the skill until
// then. It is printed by the CLI on success only — never on a refusal and
// never under --dry-run.
const PiTrustNote = "note: Pi loads .agents/skills only after the project is trusted; Pi may prompt once for project trust."

// ProjectWrite is one planned file write. Rel is the repo-relative,
// slash-separated path (ready to use as a git pathspec); Abs is the resolved
// absolute path; Data is the exact bytes to write. Backup holds the
// destination's current bytes when it already existed, and is nil for a
// genuinely new file — the distinction rollback needs to decide between
// restoring and removing (3b-ii).
//
// Mode is the mode the executor creates the file with. It is always
// ProjectFileMode: carrying it on the write is what BINDS the constant the
// design mandates to the bytes that will actually be created, instead of
// leaving it declared but unused (review round 3, PLAN-2).
type ProjectWrite struct {
	Rel    string
	Abs    string
	Data   []byte
	Mode   fs.FileMode
	Backup []byte
}

// ProjectPlan is the complete, validated description of one project-tier
// operation, produced before any filesystem mutation. Registration and
// revision use Writes; retirement uses DeleteWrites. Lock is kept apart from
// both because it is the commit marker and is always updated last.
type ProjectPlan struct {
	ID           string
	SHA256       string
	Revision     int
	AbsorbedInto string
	Writes       []ProjectWrite
	DeleteWrites []ProjectWrite
	Deletes      []string
	Lock         ProjectWrite
}

// RegisterInput carries the pre-read state PlanProjectRegister needs, so the
// planner itself performs no filesystem access: the draft bytes, the lock
// bytes, the overlay registry, and two injected probes.
//
// design.md names PlanProjectRegister(in RegisterInput) but never enumerates
// RegisterInput's fields, so these are derived from the validate-before-write
// steps and the CLI surface rather than quoted. Stat and ResolvePath are
// injected for the same reason EvaluateOwnership injects its readers: the
// planner stays pure and a test controls every path it sees.
//
// LockExists distinguishes "no lock file in this project yet" (an empty lock,
// and a lock write with no backup) from "a lock file exists and holds these
// bytes". LockData is ignored when LockExists is false.
//
// A nil ResolvePath is a fail-closed refusal, never a silent degradation to
// the lexical guard alone — that guard is exactly what a symlinked component
// defeats (tasks.md 3b-i.5b).
type RegisterInput struct {
	ProjectRoot  string
	DraftPath    string
	DraftData    []byte
	CandidateKey string
	Registry     Registry
	LockData     []byte
	LockExists   bool

	Stat        func(string) (fs.FileInfo, error)
	ResolvePath func(string) (string, error)
}

// ReviseInput carries the pre-read state PlanProjectRevise needs. Revision is
// deliberately a separate planner from registration: it reads an existing
// lock entry, proves ownership through EvaluateOwnership, and captures the
// current target bytes as backups before producing a write plan. The injected
// readers keep the planner deterministic and let tests prove that a refusal
// performs no mutation.
type ReviseInput struct {
	ProjectRoot  string
	DraftPath    string
	DraftData    []byte
	CandidateKey string
	LockData     []byte
	LockExists   bool

	ReadFile    func(string) ([]byte, error)
	ReadDir     func(string) ([]fs.DirEntry, error)
	Stat        func(string) (fs.FileInfo, error)
	ResolvePath func(string) (string, error)
}

// RetireInput carries the pre-read state PlanProjectRetire needs. The
// planner verifies ownership against the project lock before it plans any
// deletion. Registry is used only when AbsorbedInto names a global target;
// project-lock existence is checked from the same parsed lock for a project
// target. All filesystem probes are injected so the planner stays pure.
type RetireInput struct {
	ProjectRoot  string
	ID           string
	Reason       string
	AbsorbedInto string
	Registry     Registry
	LockData     []byte
	LockExists   bool

	ReadFile    func(string) ([]byte, error)
	ReadDir     func(string) ([]fs.DirEntry, error)
	Stat        func(string) (fs.FileInfo, error)
	ResolvePath func(string) (string, error)
}

// cloneProjectBytes keeps an existing empty file distinguishable from a new
// file: ProjectWrite.Backup uses nil as the "new destination" marker.
func cloneProjectBytes(data []byte) []byte {
	out := make([]byte, len(data))
	copy(out, data)
	return out
}

// projectCommitOrder is the order the executor stages and commits in: the
// Writes in projectTargets order, then the lock LAST, because the lock is the
// commit marker (design.md, "Commit phase"; ADR-9's registry-last).
func projectCommitOrder(p ProjectPlan) []ProjectWrite {
	return append(append(make([]ProjectWrite, 0, len(p.Writes)+1), p.Writes...), p.Lock)
}

// ProjectCommitOrder lists the writes of a registration or a revision in the order they are
// committed: the skills in projectTargets order, then the lock last. These are the paths a person
// is told ("plan:" before the write, "wrote:" after it) and feeds to git.
func ProjectCommitOrder(p ProjectPlan) []ProjectWrite { return projectCommitOrder(p) }

// ProjectRetireCommitOrder lists the writes of a retirement in the order they are committed: the
// deletions, then the lock last.
func ProjectRetireCommitOrder(p ProjectPlan) []ProjectWrite {
	return append(append([]ProjectWrite(nil), p.DeleteWrites...), p.Lock)
}
