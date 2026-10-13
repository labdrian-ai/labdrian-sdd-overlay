package main

import (
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/projectid"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/projectidfs"
)

// identityRules are the project identity rules reading the repositories on this machine's file system: the
// rules own the port they read through (projectid.RepositoryInspector) and projectidfs is what answers it. This
// is where the program meets the disk for them, and the only place that names the adapter.
func identityRules() projectid.Resolver {
	return projectid.NewResolver(projectidfs.Inspector{})
}
