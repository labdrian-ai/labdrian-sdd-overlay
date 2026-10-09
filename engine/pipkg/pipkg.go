// Package pipkg builds the labdrian-pi package tree (package.json, skills/,
// agents/) that gets installed into a Pi (gentle-pi) session via
// `pi install <path>`. It mirrors engine/gadu's Generate/Check idiom: Build
// writes the package, Check regenerates into a temp dir and diffs against
// the deployed copy to report drift.
//
// Integrity guarantees (pipkg-integrity slice, R-001..R-004):
//   - Mode drift: Check diffs both content and permission bits, so a
//     byte-identical file whose mode changed (e.g. 0644 -> 0755) is still
//     reported as drift.
//   - Build root permissions: the build root (destDir after the atomic
//     swap) is always 0755, never MkdirTemp's default 0700.
//   - Path containment: a registry entry's path is validated relative and
//     `..`-free at parse time (engine/skills.validateEntry); buildInto
//     re-checks the destination join as defense in depth before any file
//     is written for that entry.
//   - SKILL.md name/directory match: buildInto rejects a skill whose
//     SKILL.md frontmatter `name` does not equal its registry directory
//     name, so every built skill is addressable by its own id.
//
// Build provenance (sync-check-provenance slice, R-005..R-007): Build
// records the resolved source commit into package.json's
// labdrian.builtFrom (D5), but Check's comparison target is always the
// DEPLOY ref -- what `apply` actually deploys from (main, or its
// origin/main / HEAD fallback in a detached-HEAD checkout), never
// builtFrom itself (R3-001, issue #315 follow-up): comparing against
// builtFrom let committed source changes made after the last Build go
// undetected as drift. builtFrom is provenance surfaced via
// CheckReport.Stale/Disclosure, not the comparison basis. resolveComparisonSource
// picks "deploy" (a git repository, comparing against the resolved deploy
// ref), "dirty" (the package was built from an uncommitted source tree,
// comparing against the live worktree), or "worktree" (overlayRoot is not
// a git repository at all).
package pipkg

import (
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

const piTarget = "pi"

// Options are the choices of the caller that Check cannot make for itself. The composition root
// reads whatever configuration names them, once, and hands them down: the package reads no
// environment variable.
type Options struct {
	// DeployRef names the ref Check compares the package against, tried before main,
	// origin/main and HEAD, for a checkout that does not deploy from main (CI on a pull request,
	// a feature-branch shelltest). Surrounding space is ignored and empty means no preference.
	// The disclosure always prints whichever ref won, so an override never hides.
	DeployRef string
}

// Packages is the package builder: Build and Check over the ports it needs and the Options of the
// run, all fixed when the composition root makes it. The Pi adapter holds one and asks for the
// verbs, knowing neither the reader, nor git, nor the deploy ref. The builder starts no process
// and reads no environment: git is asked through Source and the choices of the run are Options.
type Packages struct {
	// Registries is how the skills registry the package is built from is read.
	Registries skills.RegistryRepository
	// Source is the git repository the overlay is checked out from. A tree that is not under
	// version control is NoRepository{}.
	Source SourceRepo
	// Options are the choices of the caller that Check cannot make for itself.
	Options Options
}

// Check compares the package against its sources; see Compare. Its first result is the
// disclosure of what it compared against, which Check owes the caller whether or not it found
// drift.
func (p Packages) Check(overlayRoot, registryPath, destDir string) (disclosure string, err error) {
	report, err := p.Compare(overlayRoot, registryPath, destDir)
	return report.Disclosure(), err
}

// loadRegistry reads the skills registry at registryPath through the port: the file of the working
// tree, or, when Check compares against the deploy ref, the file of the directory that ref was
// exported to. The registry is the domain's to judge (skills.ReadRegistry), so a package is never
// built from one it would not accept.
func loadRegistry(registries skills.RegistryRepository, registryPath string) (skills.Registry, error) {
	reg, err := skills.ReadRegistry(registries, registryPath)
	if err == nil {
		return reg, nil
	}
	if skills.IsUnreadableRegistry(err) {
		return skills.Registry{}, fmt.Errorf("pipkg: opening registry: %w", err)
	}
	return skills.Registry{}, fmt.Errorf("pipkg: parsing registry: %w", err)
}
