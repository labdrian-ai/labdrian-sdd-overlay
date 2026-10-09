package pipkg

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// CheckReport discloses which git state Check actually compared the
// deployed package against, and whether the deployed package is stale
// relative to the deploy ref (R3-001):
//   - "deploy": overlayRoot is a git repository. DeployRef names the ref
//     actually exported and compared against ("main", "origin/main", or
//     "HEAD" in a detached-HEAD checkout with no local main). DeployTip
//     holds that ref's resolved tip commit, short form. BuiltFrom holds
//     the deployed package's recorded labdrian.builtFrom SHA, if any
//     (provenance, never the comparison target).
//   - "worktree": overlayRoot is not a git repository at all, so Check
//     compared against the plain working tree, exactly as before R-005.
//   - "dirty": builtFrom is absent because Build ran against a dirty
//     source tree, so Check compared directly against the live
//     overlayRoot instead of any git export.
//
// Stale reports whether the deployed package is out of date: BuiltFrom is
// a full commit id that is a proper ancestor of DeployRef's tip, so commits
// landed on the deploy ref since the last Build. A package built from a
// commit that is ahead of the tip, or on another line of history (a
// feature-branch checkout), is not stale; any content it differs in is
// caught by the file-level diff. A stale package means committed source
// changes since the last Build are not deployed, even though the deployed
// content may still match DeployRef's tree -- Check treats this as drift in
// its own right.
type CheckReport struct {
	Basis     string
	DeployRef string
	DeployTip string
	BuiltFrom string
	Stale     bool
}

// Disclosure renders a one-line, human-readable statement of what Check
// actually compared against, for callers (pipkg CLI, PiAdapter, the
// sync-check bash helper) to surface (R-006/R-007: the comparison basis
// must always be disclosed, not just on fallback).
func (r CheckReport) Disclosure() string {
	switch r.Basis {
	case "deploy":
		builtFrom := r.BuiltFrom
		if builtFrom == "" {
			builtFrom = "unrecorded"
		}
		return "compared against " + r.DeployRef + " (" + r.DeployTip + "); package built from " + builtFrom
	case "worktree":
		return "compared against the worktree (overlay root is not a git repository)"
	case "dirty":
		return "compared against the worktree; the package was built from an uncommitted source tree"
	default:
		return ""
	}
}

// builtFromPattern is the full-length hex-SHA-1 shape a recorded
// labdrian.builtFrom value must match before it is ever used in a git
// argument (R-007 threat matrix: a non-hex or "--option"-shaped value must
// never reach a git subprocess argv).
var builtFromPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

// Compare regenerates the package into a temp dir and diffs it, file by file,
// against destDir. Returns a CheckReport disclosing the comparison basis,
// and a non-nil, drift-naming error when destDir is missing, has extra
// files, is missing files, or has changed content. Packages.Check is this
// operation reduced to the disclosure line, which is the shape the Pi
// adapter's port wants; the CheckReport is what the tests read.
//
// The comparison source is resolved by resolveComparisonSource (R-006):
// Compare compares the deployed package against the DEPLOY ref's exported
// tree (main, or its origin/main / HEAD fallback), never against the
// recorded labdrian.builtFrom commit -- comparing against builtFrom let
// committed source changes made after the last Build go undetected as
// drift (R3-001). A stale deployed package (builtFrom resolvable and
// behind the deploy ref's tip) is reported as drift in its own right, in
// addition to any file-level differences. package.json's own
// labdrian.builtFrom field is normalized out of the content comparison
// (via stripBuiltFrom) so recording a different (but still correct) ref
// never counts as file-level drift by itself.
func (p Packages) Compare(overlayRoot, registryPath, destDir string) (CheckReport, error) {
	got, err := listFiles(destDir)
	if err != nil {
		if os.IsNotExist(err) {
			return CheckReport{}, fmt.Errorf("pipkg: package not built at %s (run: labdrian-overlay apply --target pi)", destDir)
		}
		return CheckReport{}, fmt.Errorf("pipkg: reading built package: %w", err)
	}

	report, sourceRoot, sourceRegistry, buildRev, cleanup, err := p.resolveComparisonSource(overlayRoot, registryPath, destDir)
	if err != nil {
		return CheckReport{}, err
	}
	defer cleanup()

	reg, err := loadRegistry(p.Registries, sourceRegistry)
	if err != nil {
		return report, err
	}

	tmpDir, err := os.MkdirTemp("", "labdrian-pi-check-*")
	if err != nil {
		return report, fmt.Errorf("pipkg: creating temp check dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// The provenance root (buildInto's fourth argument) is always the ORIGINAL
	// overlayRoot, never sourceRoot: a git-archive export has no .git directory,
	// so it cannot resolve tag history itself. Using overlayRoot (which has full
	// history) plus the resolved buildRev makes the comparison's package.json
	// "version" the one a Build whose provenance is that ref would write -- the
	// newest v* tag reachable from it. That is the same reasoning as builtFrom
	// being normalized out of the diff, but here fixing the input instead of
	// the output.
	if err := p.buildInto(sourceRoot, reg, tmpDir, overlayRoot, buildRev); err != nil {
		return report, fmt.Errorf("pipkg: regenerating for check: %w", err)
	}

	want, err := listFiles(tmpDir)
	if err != nil {
		return report, fmt.Errorf("pipkg: reading regenerated package: %w", err)
	}

	// mcp.json is registration state longterm-mem owns (Build's own doc
	// comment), never regenerated build output: Check only proves the file
	// is present, and never diffs its content, or every `register --target
	// pi` call would show as permanent drift.
	_, wantHasMCP := want[mcpConfigFileName]
	_, gotHasMCP := got[mcpConfigFileName]
	delete(want, mcpConfigFileName)
	delete(got, mcpConfigFileName)
	if wantHasMCP && !gotHasMCP {
		return report, fmt.Errorf("labdrian-pi package drift:\n  %s: missing", mcpConfigFileName)
	}

	// mcp.json.bak is the backup sibling jsonInstall writes on any
	// content-changing register/unregister call -- the same registration
	// state as mcp.json itself, and never regenerated build output. Build
	// never writes it into want, so it is only ever present in got; drop it
	// unconditionally rather than flagging it as "extra".
	delete(got, mcpConfigBakFileName)

	var drift []string
	for rel, wantEntry := range want {
		gotEntry, ok := got[rel]
		if !ok {
			drift = append(drift, fmt.Sprintf("%s: missing", rel))
			continue
		}
		wantData, gotData := wantEntry.data, gotEntry.data
		if rel == "package.json" {
			// R-006/R-007: the two sides were built from different git
			// states on purpose (the deployed package vs. its recorded
			// builtFrom, or the main fallback); comparing raw bytes would
			// make every recomputed labdrian.builtFrom value read as
			// drift. Strip it from both sides before comparing content.
			wantData = stripBuiltFrom(wantData)
			gotData = stripBuiltFrom(gotData)
		}
		if string(wantData) != string(gotData) {
			drift = append(drift, fmt.Sprintf("%s: changed", rel))
			continue
		}
		// R-001: a byte-identical file whose mode changed is still
		// drift -- the registry-recorded mode is part of build output.
		if wantEntry.perm != gotEntry.perm {
			drift = append(drift, fmt.Sprintf("%s: mode %04o -> %04o", rel, wantEntry.perm, gotEntry.perm))
		}
	}
	for rel := range got {
		if _, ok := want[rel]; !ok {
			drift = append(drift, fmt.Sprintf("%s: extra", rel))
		}
	}
	if report.Stale {
		drift = append(drift, fmt.Sprintf("package built from %s but %s is at %s: rebuild with apply --target pi", report.BuiltFrom, report.DeployRef, report.DeployTip))
	}
	if len(drift) > 0 {
		sort.Strings(drift)
		return report, fmt.Errorf("labdrian-pi package drift:\n  %s", strings.Join(drift, "\n  "))
	}
	return report, nil
}

// stripBuiltFrom removes package.json's labdrian.builtFrom field before
// Check compares two package.json files that were legitimately built from
// different git states (R-006/R-007's own-basis normalization), and always
// re-marshals through encoding/json so both sides are compared in the same
// canonical (compact, key-sorted) form regardless of whether either side
// had a labdrian.builtFrom field at all -- otherwise a byte-identical pair
// that merely differs in json.MarshalIndent's whitespace would misreport
// as drift. A parse failure returns data unchanged, so a malformed
// package.json still surfaces as ordinary content drift rather than being
// silently swallowed.
func stripBuiltFrom(data []byte) []byte {
	var doc map[string]json.RawMessage
	if json.Unmarshal(data, &doc) != nil {
		return data
	}
	rawLabdrian, ok := doc["labdrian"]
	if !ok {
		out, err := json.Marshal(doc)
		if err != nil {
			return data
		}
		return out
	}
	var labdrian map[string]json.RawMessage
	if json.Unmarshal(rawLabdrian, &labdrian) != nil {
		return data
	}
	delete(labdrian, "builtFrom")
	if len(labdrian) == 0 {
		delete(doc, "labdrian")
	} else {
		merged, err := json.Marshal(labdrian)
		if err != nil {
			return data
		}
		doc["labdrian"] = merged
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return data
	}
	return out
}
