// Package projectidentity is the adapter of the skills domain to the shared resolver of a project's
// identity (identity/resolve, C23 M2). It implements the port engine/skills owns,
// skills.ProjectIdentity, by asking the shared rules, so that the skills verbs name a project as the
// workflow and longterm-mem do: the id the person gave with --project-id, the file
// .longterm-mem-project at the root of the main checkout, the origin remote, the name of the main
// checkout's root, and outside a repository the name of the directory (the owner's decisions 1 and
// 4 of M2, 2026-10-18).
//
// The rules and the reading of the repository are not here: they are the identity module's, shared
// with longterm-mem, and the composition root (engine/cmd) chooses the adapter the rules read the
// disk through (identity/gitfs). This package only translates the domain's question and answer.
//
// It imports the domain and the identity module and nothing else of the program; the dependency
// points from the adapter to the domain, never back.
package projectidentity

import (
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/identity/resolve"
)

// Shared answers the skills domain's question with the shared rules.
type Shared struct {
	Rules resolve.Resolver
}

// Identify is skills.ProjectIdentity. The shared chain always has an answer, the name of a
// directory at worst, so ok is false only with an error: a repository or a declaration that could
// not be read, or a declaration that is broken.
func (s Shared) Identify(q skills.ProjectQuery) (skills.ProjectID, bool, error) {
	id, err := s.Rules.Resolve(q.Dir, string(q.Explicit))
	if err != nil {
		return "", false, err
	}
	return skills.ProjectID(id.Project), true, nil
}
