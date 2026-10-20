package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/execrunner"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/memoryscope"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection/app"
	runtimepkg "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
)

// sessionProbeTimeout is how long one source has to answer the session-time probe. The sources are
// asked at the same time, so the SessionStart hook waits at most this long, whatever the sources do.
const sessionProbeTimeout = 3 * time.Second

// programRunner is how the probe starts a program: execrunner.Runner in the program, a fake in a test.
type programRunner interface {
	LookPath(name string) (string, error)
	Run(ctx context.Context, bin string, args ...string) ([]byte, error)
}

// sessionAvailability is the adapter of the SessionStart hook's port app.SourceAvailabilityProber
// (C23 M8, docs/architecture/adr-c23-m8-session-availability.md). It asks each source of the plan
// whether it can be read, instead of looking for a file:
//
//   - engram: the engram program is on PATH and 'engram stats' succeeds, which opens its database
//     (the engine has no SQLite driver, so the program does the opening);
//   - longterm-mem: its binary answers 'longterm-mem status' and an MCP entry for it is registered in
//     the config of any runtime (Claude Code, OpenCode, Codex);
//   - procedural-skills: the procedural skills lock of the session's project exists and is valid.
//
// Every source is asked at the same time and has sessionProbeTimeout. It never fails: a source that
// does not answer, or answers badly, is unavailable with the reason.
type sessionAvailability struct {
	runner  programRunner
	timeout time.Duration
	// longtermBinary is the path of the longterm-mem binary; entryRegistered says whether a
	// runtime has its MCP entry.
	longtermBinary  string
	entryRegistered func() bool
	// procedural reads the lock of the project a directory belongs to.
	procedural app.ProceduralSkillsReader
}

var _ app.SourceAvailabilityProber = sessionAvailability{}

// newSessionAvailability is the probe of the program: the programs of the machine, and the runtimes'
// configs under the home of the deps.
func newSessionAvailability(d deps) sessionAvailability {
	adapter := runtimepkg.NewLongtermMemAdapter(runtimeConfigFromEnv(d.env, d.homeDir), "", "")
	return sessionAvailability{
		runner:          execrunner.New(),
		timeout:         sessionProbeTimeout,
		longtermBinary:  adapter.BinaryPath,
		entryRegistered: adapter.EntryInAnyRuntime,
		procedural:      newProceduralLockReader(),
	}
}

func (p sessionAvailability) Probe(dir string, sources []memoryscope.Source) []projection.SourceAvailability {
	out := make([]projection.SourceAvailability, len(sources))
	var wg sync.WaitGroup
	for i, source := range sources {
		wg.Add(1)
		go func(i int, source memoryscope.Source) {
			defer wg.Done()
			// A probe that panics is an unavailable source, not a failed hook.
			defer func() {
				if r := recover(); r != nil {
					out[i] = projection.SourceAvailability{Source: source, Detail: fmt.Sprint("the probe failed: ", r)}
				}
			}()
			out[i] = p.probeOne(dir, source)
		}(i, source)
	}
	wg.Wait()
	return out
}

func (p sessionAvailability) probeOne(dir string, source memoryscope.Source) projection.SourceAvailability {
	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()
	switch source {
	case memoryscope.SourceEngram:
		return p.unlessFailed(source, p.runs(ctx, "engram", "engram", "stats"))
	case memoryscope.SourceLongtermMem:
		if reason := p.runs(ctx, "longterm-mem", p.longtermBinary, "status"); reason != "" {
			return unavailableSource(source, reason)
		}
		if !p.entryRegistered() {
			return unavailableSource(source, "no longterm-mem MCP entry is registered in Claude Code, OpenCode or Codex")
		}
		return projection.SourceAvailability{Source: source, Available: true}
	case memoryscope.SourceProceduralSkills:
		lock := p.procedural.Load(dir)
		switch {
		case lock.Unreadable != "":
			return unavailableSource(source, "the procedural skills lock cannot be read: "+lock.Unreadable)
		case !lock.Locked:
			return unavailableSource(source, "no procedural skills lock: none installed")
		}
		return projection.SourceAvailability{Source: source, Available: true}
	}
	return unavailableSource(source, "no availability check exists for this source")
}

func (p sessionAvailability) unlessFailed(source memoryscope.Source, reason string) projection.SourceAvailability {
	if reason != "" {
		return unavailableSource(source, reason)
	}
	return projection.SourceAvailability{Source: source, Available: true}
}

// runs runs bin (looked up on PATH when it is a bare name) with args and returns "" when it
// succeeded, otherwise why not, naming the program as subject.
func (p sessionAvailability) runs(ctx context.Context, subject, bin string, args ...string) string {
	if bin == "" {
		return "the " + subject + " program was not found"
	}
	path, err := p.runner.LookPath(bin)
	if err != nil {
		return "the " + subject + " program was not found"
	}
	if _, err := p.runner.Run(ctx, path, args...); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Sprintf("the %s program did not answer within %s", subject, p.timeout)
		}
		return fmt.Sprintf("the %s program failed: %v", subject, err)
	}
	return ""
}

func unavailableSource(source memoryscope.Source, detail string) projection.SourceAvailability {
	return projection.SourceAvailability{Source: source, Detail: detail}
}
