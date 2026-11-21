package projection

import (
	"slices"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/memoryscope"
)

// SourceAvailability is what a session-time probe found out about one memory source (C23 M8,
// docs/architecture/adr-c23-m8-session-availability.md). Detail is the reason a source is
// unavailable, for the person or the agent to read; it is empty for an available one.
type SourceAvailability struct {
	Source    memoryscope.Source
	Available bool
	Detail    string
	// Uncreated says the source has no store yet: its store does not exist or is empty, and its first
	// open creates it. It is read only when Available is false.
	Uncreated bool
}

// SourceProber probes the sources it is given and answers for each. The domain asks only about the
// sources of the plan, and reads only the answers for those: a source the probe leaves out is not
// reported, and one the probe adds is ignored. The caller supplies it (the SessionStart hook does;
// the gate-task rewrite and 'memory plan --instructions' give none), so the domain starts no
// process and opens no file.
type SourceProber func(sources []memoryscope.Source) []SourceAvailability

// degradationLines are the lines the instructions block adds for the sources of a plan that probe
// found unavailable, in the order of the plan: one line per source, saying not to read it and why.
// They are information only: they change neither the plan nor what the gate allows. With no probe
// or no source there are none, and a probe that panics costs these lines and never the block.
//
// The line for engram names memorycard, not the source: the probe of source engram asks memorycard
// (engine/cmd/session_availability.go), whose reads are of source engram. The gate reads no probe, so
// the plan still allows those reads, and the block says writes are not limited. The three forms are the
// generic line of every other source; for engram the tool line, for a store that cannot be used (the
// plan allows its reads and does not limit its writes); and, when the answer is Uncreated, the
// no-store line, which forbids no tool, because a write is what creates the store and a ban on it
// would repeat every session. Uncreated changes nothing for any other source.
func degradationLines(sources []memoryscope.Source, probe SourceProber) (lines []string) {
	if probe == nil || len(sources) == 0 {
		return nil
	}
	defer func() {
		if recover() != nil {
			lines = nil
		}
	}()
	answers := probe(slices.Clone(sources))
	for _, source := range sources {
		i := slices.IndexFunc(answers, func(a SourceAvailability) bool { return a.Source == source })
		if i < 0 || answers[i].Available {
			continue
		}
		line, tail := "- Do not read "+sanitizeLine(string(source))+": it is unavailable in this session", "."
		if source == memoryscope.SourceEngram {
			if answers[i].Uncreated {
				line, tail = "- memorycard has no store yet", ": memorycard creates it when it first opens it, so its reads find nothing until a record is saved."
			} else {
				line, tail = "- Do not call the memorycard tools: memorycard is unavailable in this session", ", though the plan allows its reads and does not limit its writes."
			}
		}
		if detail := clip(sanitizeLine(answers[i].Detail), maxDetailRunes); detail != "" {
			line += " (" + detail + ")"
		}
		lines = append(lines, line+tail)
	}
	return lines
}
