package pipkg

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// resolveComparisonSource decides Check's comparison basis (R-006/R-007,
// R3-001) and returns the root directory and registry path to build the
// "want" side from, plus a cleanup func for any temp export directory
// created.
//
//   - overlayRoot is not a git repository at all -> Basis="worktree",
//     comparing directly against overlayRoot/registryPath (unchanged
//     pre-R-005 behavior).
//   - builtFrom is absent because Build ran against a dirty source tree ->
//     Basis="dirty": comparing directly against the live overlayRoot,
//     since any git export would reproduce the last commit instead of the
//     uncommitted content Build actually shipped.
//   - otherwise -> Basis="deploy": resolve the DEPLOY ref (the first of
//     "main", "origin/main", "HEAD" that exists locally -- a pull-request
//     checkout in CI is a detached HEAD with no local "main") and export
//     skills/, agents/, and skills.registry.yaml at that commit via `git
//     archive` into a temp dir to compare against. This is always the
//     deploy ref, never the recorded labdrian.builtFrom commit (R3-001):
//     cmd_apply always deploys from main, so that is what "does the
//     deployed package match what apply would deploy" must compare
//     against. builtFrom (read from destDir's package.json, validated
//     against builtFromPattern before ever reaching a git argument -- an
//     absent, non-hex, or unresolvable value is simply carried as "" into
//     the report) is surfaced as provenance via Stale/Disclosure only.
//
// buildRev is the ref buildInto's provenance resolution should use for the
// comparison build (fed to resolvePackageVersion against the ORIGINAL
// overlayRoot, which -- unlike a git-archive export -- still has full tag
// history): the resolved deploy ref for "deploy" (git describe accepts a
// branch name), or overlayRoot's current HEAD for "worktree" (matching
// Check's pre-R-005 behavior exactly).
func resolveComparisonSource(overlayRoot, registryPath, destDir string, opts Options) (report CheckReport, sourceRoot, sourceRegistry, buildRev string, cleanup func(), err error) {
	noopCleanup := func() {}
	if exec.Command("git", "-C", overlayRoot, "rev-parse", "--is-inside-work-tree").Run() != nil {
		return CheckReport{Basis: "worktree"}, overlayRoot, registryPath, resolveBuildRev(overlayRoot), noopCleanup, nil
	}

	builtFrom := readBuiltFrom(destDir)
	// R3-001: an absent builtFrom because Build ran against a dirty tree
	// can only be reproduced by comparing against that same live tree --
	// any git export would reproduce the last commit instead.
	if builtFrom == "" && isSourceDirty(overlayRoot) {
		return CheckReport{Basis: "dirty"}, overlayRoot, registryPath, "", noopCleanup, nil
	}

	// A pull-request checkout in CI is a detached HEAD with no local
	// "main"; fall back through the refs that can exist and disclose the
	// one used. This -- never builtFrom -- is the comparison target
	// (R3-001): cmd_apply always deploys from main.
	// Options.DeployRef lets a checkout that is not main (CI on a pull
	// request, a feature-branch shelltest) name the ref it deploys from; the
	// disclosure always prints whichever ref won, so an override never hides.
	candidates := []string{"main", "origin/main", "HEAD"}
	if override := strings.TrimSpace(opts.DeployRef); override != "" {
		candidates = append([]string{override}, candidates...)
	}
	deployRef := "main"
	for _, candidate := range candidates {
		if exec.Command("git", "-C", overlayRoot, "cat-file", "-e", candidate+"^{commit}").Run() == nil {
			deployRef = candidate
			break
		}
	}
	tipOut, tipErr := exec.Command("git", "-C", overlayRoot, "rev-parse", deployRef).Output()
	if tipErr != nil {
		return CheckReport{}, "", "", "", noopCleanup, fmt.Errorf("pipkg: resolving %s: %w", deployRef, tipErr)
	}
	tip := strings.TrimSpace(string(tipOut))

	// Stale: builtFrom is a resolvable commit strictly BEHIND the deploy
	// ref's tip (a proper ancestor): commits landed on the deploy ref since
	// the last Build are not deployed, even when the file-level diff below
	// still matches. A build that is ahead of or diverged from the deploy
	// ref (a feature-branch checkout, #315) is not stale; any content it
	// differs in is caught by the file-level diff, never by this flag.
	stale := builtFrom != "" && builtFromPattern.MatchString(builtFrom) && builtFrom != tip &&
		exec.Command("git", "-C", overlayRoot, "merge-base", "--is-ancestor", builtFrom, tip).Run() == nil

	root, deployCleanup, exportErr := exportGitTree(overlayRoot, deployRef)
	if exportErr != nil {
		return CheckReport{}, "", "", "", noopCleanup, fmt.Errorf("pipkg: exporting %s for comparison: %w", deployRef, exportErr)
	}
	report = CheckReport{Basis: "deploy", DeployRef: deployRef, DeployTip: shortSHA(tip), BuiltFrom: builtFrom, Stale: stale}
	return report, root, filepath.Join(root, "skills.registry.yaml"), deployRef, deployCleanup, nil
}

// shortSHA renders sha's short (12-hex-char) form, or sha unchanged when
// it is already shorter than that.
func shortSHA(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

// readBuiltFrom reads destDir/package.json's labdrian.builtFrom value,
// returning "" on any read/parse failure or when the field is absent --
// never an error, since an unrecorded/unreadable value simply means Check
// falls back to the main basis.
func readBuiltFrom(destDir string) string {
	raw, err := os.ReadFile(filepath.Join(destDir, "package.json"))
	if err != nil {
		return ""
	}
	var manifest packageManifest
	if json.Unmarshal(raw, &manifest) != nil || manifest.Labdrian == nil {
		return ""
	}
	return manifest.Labdrian.BuiltFrom
}

// exportGitTree exports skills/, agents/, and skills.registry.yaml at rev
// from the overlayRoot git repository into a fresh temp directory via `git
// archive`, returning that directory and a cleanup func. rev MUST already
// be a value this package trusts as a git ref (a validated 40-hex SHA, or
// the fixed literal "main") -- never attacker-controlled input, since it is
// passed directly as a git argument.
func exportGitTree(overlayRoot, rev string) (string, func(), error) {
	tmp, err := os.MkdirTemp("", "labdrian-pi-source-*")
	if err != nil {
		return "", func() {}, fmt.Errorf("pipkg: creating temp source dir: %w", err)
	}
	cleanup := func() { os.RemoveAll(tmp) }

	paths := []string{"skills", "agents", "skills.registry.yaml"}
	// "pi" (pi/agents/GADU.md, resolveGaduAgentSource's preferred source) is
	// a newer addition to the tree: a git-archive pathspec that matches no
	// files at rev makes the whole archive command fail, so only include it
	// when rev actually has that path -- otherwise comparing against a
	// deploy ref that predates it would break every Check call.
	if exec.Command("git", "-C", overlayRoot, "cat-file", "-e", rev+":pi").Run() == nil {
		paths = append(paths, "pi")
	}
	cmd := exec.Command("git", append([]string{"-C", overlayRoot, "archive", "--format=tar", rev, "--"}, paths...)...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("pipkg: preparing git archive: %w", err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("pipkg: starting git archive: %w", err)
	}
	extractErr := extractTar(stdout, tmp)
	// R3-002: extractTar can return before the stream is exhausted; drain
	// any remainder or git blocks forever on a full stdout pipe below.
	_, _ = io.Copy(io.Discard, stdout)
	waitErr := cmd.Wait()
	if waitErr != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("pipkg: git archive %s: %w (%s)", rev, waitErr, strings.TrimSpace(stderr.String()))
	}
	if extractErr != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("pipkg: extracting git archive %s: %w", rev, extractErr)
	}
	return tmp, cleanup, nil
}
