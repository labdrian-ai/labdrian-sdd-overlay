// Package projectidfs names, for this module, the adapter that answers the questions the identity rules
// (internal/projectid, over the shared rules of identity/resolve) ask of a repository from the metadata git keeps
// on a disk: where the repository is, the declaration file of its main checkout, and its git configuration.
//
// Since C23 M2 the adapter is the shared one, identity/gitfs: the engine and this module read a repository with
// the same code, so they cannot disagree about where it is. It never shells out, which production code in this
// module may not do (see exec_allowlist_test.go, R-021). This package keeps the one name the composition root
// wires and the guards watch (projectidfs_adapter_guard_test.go): no package of this module but the composition
// root reaches the disk adapter, and none but this one imports identity/gitfs, whose use of the file system is
// what the architecture test's pure module (the identity module) must not let into the domain.
package projectidfs

import (
	"github.com/labdrian-ai/labdrian-sdd-overlay/identity/gitfs"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/projectid"
)

// Inspector reads repositories from the file system. It holds nothing: every question is answered from the
// disk as it is when asked. Its zero value names no flag in a refusal, which is right here: longterm-mem's
// --project never reaches the rules.
type Inspector = gitfs.Repositories

var _ projectid.RepositoryInspector = Inspector{}
