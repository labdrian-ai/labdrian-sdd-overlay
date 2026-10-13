package main

import (
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/adoption"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/identityledger"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/projectid"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/projectidfs"
)

// identityRules are the project identity rules reading the repositories on this machine's file system: the
// rules own the port they read through (projectid.RepositoryInspector) and projectidfs is what answers it. This
// is where the program meets the disk for them, and the only place that names the adapter.
//
// It builds the resolver on every call and keeps none: the inspector holds nothing (every question is answered
// from the disk as it is when asked), so construction costs nothing and there is no instance to share or to
// get stale between two calls of one command.
func identityRules() projectid.Resolver {
	return projectid.NewResolver(projectidfs.Inspector{})
}

// adoption is the use case that chooses the project a command acts on when it was given none, wired to what
// lives outside the process: the identity rules on this machine's repositories, the stores that say what memory
// already exists (machineStores), the repository's own ledger (identityledger) and the commands' clock, which
// dates what the ledger records.
func (c commands) adoption() adoption.UseCase {
	return adoption.UseCase{
		Identity: identityRules(),
		Stores:   machineStores{},
		Ledger:   identityledger.Ledger{},
		Clock:    c.clock,
	}
}
