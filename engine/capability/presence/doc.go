// Package presence is the presence prober: a workflow.DependencyProber that
// answers, for the capabilities a workflow declares and for each runtime's
// credentials, one narrow question, "is the thing there?", and nothing more.
//
// Decision 4 of the Phase 7 design is the contract. The prober looks at a path
// with stat and at the PATH directories by name. It never opens, reads, hashes,
// or prints the contents of a credentials, authentication, database, or
// registration file, it never runs a program, and it never asks the network. So
// an "available" observation only ever says a file or a binary is present, and
// its detail says what that does not prove: a credentials file that is present
// is not an authenticated session, a database file that is present is not a
// healthy one, and a binary that is on PATH has not been run. An observation is
// never "authenticated" or "healthy". The static test in presence_static_test.go
// pins the source of this package to that contract: no os.Open, no os.ReadFile, no
// io/ioutil, no os/exec, no net.
//
// One signal asks something a stat cannot answer: whether the project's procedural
// skills lock can be used (C10c). The prober does not read the lock to find out; it
// asks the project through ProjectContext, a port it owns, and the composition root
// answers it with an adapter that reads the lock with the reader the SessionStart
// probe uses. So the package still has no way to open a file, and the answer is the
// session probe's: a lock that is there and cannot be used is unavailable, with the
// session probe's words.
//
// The prober is a driven adapter: engine/workflow owns the DependencyProber port
// and engine/capability stays the pure vocabulary and declarations of what each
// runtime can do, so the stat-level checks, which are a statement about what
// runtimes and dependencies leave on disk, sit in a subpackage of capability and
// depend on both; neither depends on them. The composition root (engine/cmd) builds
// the prober and hands it to the workflow lifecycle.
//
// Where the signals come from:
//
//   - memory:engram: <home>/.engram/engram.db, the database Engram keeps by default.
//   - memory:longterm-mem: <home>/.labdrian-overlay/longterm-mem-registration.json.
//     The name and the directory are the convention of engine/runtime
//     (longtermMemRegistrationFile and DefaultLongtermMemStateDir), copied here
//     because engine/runtime must not be imported by Phase 7 code. A registration
//     recorded under a --state-dir other than the default is not seen.
//   - memory:procedural-skills: the project's own lock,
//     <project root>/.labdrian/procedural-skills.lock.json, asked of the project
//     through ProjectContext (the skills directories under Home hold every kind of
//     skill, not procedural ones). Available when the lock is there and valid;
//     unavailable when the project is unknown, when it has no lock, and when the lock
//     is there and cannot be used. The words of that last case are the session
//     probe's ("the procedural skills lock cannot be read: <reason>"), the reason
//     shortened to MaxReasonRunes. With no ProjectContext the project is unknown.
//   - gentle-ai-review: an executable file named gentle-ai in one of the
//     absolute directories of the PATH value the prober was given.
//   - credentials:claude-code, credentials:codex, credentials:pi: the files
//     <home>/.claude/.credentials.json, <home>/.codex/auth.json, and
//     <home>/.pi/agent/auth.json.
//
// Any other capability name is unavailable: absence of a check is reported, not
// guessed.
package presence
