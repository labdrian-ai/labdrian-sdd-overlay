package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memorycard"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
)

// The stores `sync --source`, `promote --source` and the MCP promote tool's source name. This registry is the only
// place that names a store: promotion (internal/promote) is written over the promote.Source port, so a new store is
// one adapter plus one entry here, with no change in promotion. memorycard is the only entry: Engram is frozen and
// its history is imported into memorycard (Phase 9, S10).
const sourceMemorycard = "memorycard"

// openedSource is a store opened for one run: what promotion reads it through, what closes it, and whether it could
// only be opened through a fallback that serves a snapshot instead of the live database.
type openedSource struct {
	source   promote.Source
	closer   io.Closer
	degraded bool
}

// registeredSource is a store the promotion commands can read.
type registeredSource struct {
	name string
	// open opens the store read-only, at the path its environment variable names or its default.
	open func() (openedSource, error)
}

// registeredSources holds one entry, memorycard, in the order the commands list them.
var registeredSources = []registeredSource{
	{name: sourceMemorycard, open: openMemorycardSource},
}

func openMemorycardSource() (openedSource, error) {
	store, err := memorycard.Open(os.Getenv(memorycardDBEnvVar))
	if err != nil {
		return openedSource{}, err
	}
	return openedSource{source: memorycard.NewSource(store), closer: store, degraded: store.Degraded()}, nil
}

// registeredSourceNamed is the registered store called name.
func registeredSourceNamed(name string) (registeredSource, bool) {
	for _, s := range registeredSources {
		if s.name == name {
			return s, true
		}
	}
	return registeredSource{}, false
}

// registeredSourceNames lists the stores for a message: "memorycard", or "a or b".
func registeredSourceNames() string {
	names := make([]string, len(registeredSources))
	for i, s := range registeredSources {
		names[i] = s.name
	}
	return strings.Join(names, " or ")
}

// sourceRefusal is why a command cannot go on with the --source it was given, or "" when it is a registered store.
// With no name given, required says whether that is a refusal (promote asks for one, because its id means what the
// store says it means; sync has a default).
func sourceRefusal(name string, required bool) string {
	if name == "" && required {
		return fmt.Sprintf("--source is required: use --source %s", registeredSourceNames())
	}
	if _, ok := registeredSourceNamed(name); !ok {
		return fmt.Sprintf("--source %q is not a store: use %s", name, registeredSourceNames())
	}
	return ""
}

// The memorycard adapter is the port promotion reads.
var _ promote.Source = (*memorycard.Source)(nil)

// openSource opens the registered store called name for the command. When it could only be opened through the
// immutable=1 fallback and declareDegraded is set (a command that writes pages from what it reads), the command
// says so on stderr. The caller closes what it gets with closeStore. exitOK, or the exit code to return after
// saying why: a database that cannot be opened is the unavailable store.
func (a app) openSource(name string, declareDegraded bool) (promote.Source, io.Closer, int) {
	entry, ok := registeredSourceNamed(name)
	if !ok {
		return nil, nil, a.refuse(fmt.Errorf("%s", sourceRefusal(name, true)), exitUsage)
	}
	opened, err := entry.open()
	if err != nil {
		return nil, nil, a.refuse(err, exitEngramUnavailable)
	}
	if declareDegraded && opened.degraded {
		// The store keeps no cause for the fallback, so the line names none.
		fmt.Fprintf(os.Stderr, "longterm-mem: %s: %s is being read through the immutable=1 fallback, so this run works from the snapshot taken when the connection was opened, not the live database\n", a.command, name)
	}
	return opened.source, opened.closer, exitOK
}
