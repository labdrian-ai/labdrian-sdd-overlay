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
// pins the source of this file to that contract: no os.Open, no os.ReadFile, no
// io/ioutil, no os/exec, no net.
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
//   - memory:procedural-skills: no check exists, so it is always unavailable. The
//     one file that would show them by stat is the project's own lock,
//     <project root>/.labdrian/procedural-skills.lock.json, and the prober knows
//     only Home and PATH: a check would need a project-root input it does not have,
//     and the skills directories under Home hold every kind of skill, not
//     procedural ones. Phase 8 leaves this as it was; giving the prober a project
//     root is a decision about its inputs, made by whoever wires it, not here.
//   - gentle-ai-review: an executable file named gentle-ai in one of the
//     absolute directories of the PATH value the prober was given.
//   - credentials:claude-code, credentials:codex, credentials:pi: the files
//     <home>/.claude/.credentials.json, <home>/.codex/auth.json, and
//     <home>/.pi/agent/auth.json.
//
// Any other capability name is unavailable: absence of a check is reported, not
// guessed.
package presence

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// The capability names the prober answers for. The memory names are
// "memory:" plus a memoryscope source, and the review name is the one the
// workflow lifecycle records; they are repeated here as literals because this
// package must not depend on more of the engine than it needs, and a test pins
// them against the names the lifecycle really requests.
//
// They are the names of what is probed, not of what a runtime can do, so they
// belong to the prober and not to engine/capability's vocabulary.
const (
	CapabilityMemoryEngram      = "memory:engram"
	CapabilityMemoryLongtermMem = "memory:longterm-mem"
	CapabilityMemoryProcedural  = "memory:procedural-skills"
	CapabilityGentleAIReview    = "gentle-ai-review"
	CapabilityCredentialsClaude = "credentials:claude-code"
	CapabilityCredentialsCodex  = "credentials:codex"
	CapabilityCredentialsPi     = "credentials:pi"
)

// CredentialsCapability returns the capability name of the credentials
// presence check for a runtime target (capability.TargetClaude, TargetCodex, or
// TargetPi), and false for a target that has none (OpenCode, or an unknown value).
func CredentialsCapability(target string) (string, bool) {
	switch target {
	case capability.TargetClaude:
		return CapabilityCredentialsClaude, true
	case capability.TargetCodex:
		return CapabilityCredentialsCodex, true
	case capability.TargetPi:
		return CapabilityCredentialsPi, true
	}
	return "", false
}

// StatFS is the only filesystem access the prober has: stat, with and without
// following a final symbolic link. It has no method that opens a file, so a
// Prober cannot read one through it. The default is the operating
// system's Lstat and Stat; a test injects a fake to produce errors that are hard
// to produce on a real disk.
type StatFS interface {
	Lstat(name string) (fs.FileInfo, error)
	Stat(name string) (fs.FileInfo, error)
}

// osStatFS is the default StatFS.
type osStatFS struct{}

func (osStatFS) Lstat(name string) (fs.FileInfo, error) { return os.Lstat(name) }
func (osStatFS) Stat(name string) (fs.FileInfo, error)  { return os.Stat(name) }

// Prober implements workflow.DependencyProber with stat-level checks
// only. The zero value has no home and no PATH, so every signal is unavailable.
type Prober struct {
	// Home is the absolute home directory the per-user files are looked for
	// under. Empty, or not absolute, means the home is unknown: every signal that
	// lives under it is unavailable, and the prober never falls back to the
	// process environment on its own.
	Home string
	// Path is the PATH value scanned for the gentle-ai binary, in the
	// operating system's list format. Empty entries and relative entries are
	// skipped: they would be resolved against a working directory the prober
	// does not know.
	Path string
	// ProbeFS is the stat access; nil means the operating system's.
	ProbeFS StatFS
}

var _ workflow.DependencyProber = Prober{}

// Probe returns one observation per capability, in order. It honors ctx: when
// the context is done it returns the context's error at once, which the
// workflow lifecycle records as every capability being unavailable. It never
// returns another error, and it never returns an observation that would fail the
// workflow's bounds: every detail is a short fixed sentence.
func (p Prober) Probe(ctx context.Context, capabilities []string) ([]workflow.Observation, error) {
	out := make([]workflow.Observation, len(capabilities))
	for i, name := range capabilities {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		out[i] = p.observe(ctx, name)
	}
	return out, nil
}

func (p Prober) statFS() StatFS {
	if p.ProbeFS != nil {
		return p.ProbeFS
	}
	return osStatFS{}
}

// presenceCheck names one file signal: where it is, what to call it when it is
// missing, and what an "available" answer does not prove.
type presenceCheck struct {
	// rel is the path under the home directory, as slash-separated elements.
	rel []string
	// subject names the file for a person, without its path.
	subject string
	// limit is the parenthesized statement of what presence does not prove.
	limit string
}

// fileChecks are the per-user file signals, keyed by capability name.
var fileChecks = map[string]presenceCheck{
	CapabilityMemoryEngram: {
		rel:     []string{".engram", "engram.db"},
		subject: "the Engram database file",
		limit:   "not opened, so its contents and health are unverified",
	},
	CapabilityMemoryLongtermMem: {
		rel:     []string{".labdrian-overlay", "longterm-mem-registration.json"},
		subject: "the longterm-mem registration record",
		limit:   "not opened; whether a runtime has the MCP server loaded is unverified",
	},
	CapabilityCredentialsClaude: {
		rel:     []string{".claude", ".credentials.json"},
		subject: "the Claude Code credentials file",
		limit:   "not opened; this does not prove the runtime is authenticated",
	},
	CapabilityCredentialsCodex: {
		rel:     []string{".codex", "auth.json"},
		subject: "the Codex credentials file",
		limit:   "not opened; this does not prove the runtime is authenticated",
	},
	CapabilityCredentialsPi: {
		rel:     []string{".pi", "agent", "auth.json"},
		subject: "the Pi credentials file",
		limit:   "not opened; this does not prove the runtime is authenticated",
	},
}

// gentleAIReviewLimit is what finding the gentle-ai binary on PATH does not
// prove.
const gentleAIReviewLimit = "not executed; review mode and consent are unverified"

// gentleAIBinaryName is the file name searched for on PATH.
func gentleAIBinaryName() string {
	if runtime.GOOS == "windows" {
		return "gentle-ai.exe"
	}
	return "gentle-ai"
}

func (p Prober) observe(ctx context.Context, name string) workflow.Observation {
	switch name {
	case CapabilityMemoryProcedural:
		return unavailable(name, "no presence check exists for procedural skills")
	case CapabilityGentleAIReview:
		return p.observeGentleAI(ctx, name)
	}
	check, ok := fileChecks[name]
	if !ok {
		return unavailable(name, "no presence check exists for this capability")
	}
	if !filepath.IsAbs(p.Home) {
		return unavailable(name, "the home directory is unknown, so "+check.subject+" was not looked for")
	}
	path := filepath.Join(append([]string{p.Home}, check.rel...)...)
	found := p.statRegular(path)
	switch {
	case found.present:
		detail := check.subject + " is present (" + check.limit + ")"
		if found.symlink {
			detail += "; the checked path is a symlink"
		}
		return workflow.Observation{Capability: name, Status: workflow.ObservationAvailable, Detail: detail}
	default:
		return unavailable(name, check.subject+" was not found: "+found.why)
	}
}

// observeGentleAI scans the absolute PATH directories, by name, for an
// executable regular file called gentle-ai. It stats <dir>/gentle-ai and never
// lists a directory, opens the file, or runs it.
func (p Prober) observeGentleAI(ctx context.Context, name string) workflow.Observation {
	binary := gentleAIBinaryName()
	unchecked := 0
	for _, dir := range filepath.SplitList(p.Path) {
		if err := ctx.Err(); err != nil {
			return unavailable(name, "the PATH scan was cut short: "+classifyContext(err))
		}
		if !filepath.IsAbs(dir) {
			continue
		}
		found := p.statRegular(filepath.Join(dir, binary))
		if !found.present {
			if found.unchecked {
				unchecked++
			}
			continue
		}
		if runtime.GOOS != "windows" && found.mode&0o111 == 0 {
			continue
		}
		detail := "the gentle-ai binary is on PATH (" + gentleAIReviewLimit + ")"
		if found.symlink {
			detail += "; the checked path is a symlink"
		}
		return workflow.Observation{Capability: name, Status: workflow.ObservationAvailable, Detail: detail}
	}
	detail := "the gentle-ai binary was not found in the absolute PATH directories"
	if unchecked > 0 {
		detail += " (" + strconv.Itoa(unchecked) + " could not be checked)"
	}
	return unavailable(name, detail)
}

// statResult is what statRegular learned about one path.
type statResult struct {
	// present is true when the path is a regular file, or a symlink whose target
	// is one.
	present bool
	// symlink is true when the path itself is a symbolic link.
	symlink bool
	// mode is the mode of the file present refers to (after following a link).
	mode fs.FileMode
	// unchecked is true when the path could not be examined for a reason other
	// than not existing (permission, an I/O error).
	unchecked bool
	// why is a short reason for a path that is not present. It names an error
	// class, never a path.
	why string
}

// statRegular looks at path with Lstat and, for a symbolic link, Stat. A link
// counts as present only when following it succeeds and lands on a regular file,
// and the result says it was a link so the detail can. Anything else, a
// directory or a device included, is not present.
func (p Prober) statRegular(path string) statResult {
	fsys := p.statFS()
	info, err := fsys.Lstat(path)
	if err != nil {
		return statResult{why: classifyStatError(err), unchecked: !errors.Is(err, fs.ErrNotExist)}
	}
	result := statResult{}
	if info.Mode()&fs.ModeSymlink != 0 {
		result.symlink = true
		info, err = fsys.Stat(path)
		if err != nil {
			result.why = "a symlink whose target cannot be followed (" + classifyStatError(err) + ")"
			result.unchecked = !errors.Is(err, fs.ErrNotExist)
			return result
		}
	}
	if !info.Mode().IsRegular() {
		result.why = "the path is not a regular file"
		return result
	}
	result.present = true
	result.mode = info.Mode()
	return result
}

// classifyStatError names the class of a stat failure without repeating the
// error text, which carries the full path.
func classifyStatError(err error) string {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "no such file"
	case errors.Is(err, fs.ErrPermission):
		return "permission denied"
	default:
		return "the path could not be examined"
	}
}

func classifyContext(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "deadline exceeded"
	}
	return "cancelled"
}

func unavailable(name, detail string) workflow.Observation {
	return workflow.Observation{Capability: name, Status: workflow.ObservationUnavailable, Detail: detail}
}
