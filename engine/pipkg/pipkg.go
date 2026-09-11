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
// labdrian.builtFrom (D5). Check uses that field to pick its comparison
// basis (CheckReport.Basis, resolveComparisonSource): the recorded commit
// when it is a resolvable 40-hex SHA ("ref"), main when it is not
// ("main", always disclosed), or the plain working tree when overlayRoot
// is not a git repository at all ("worktree"). This closes #315's false
// drift reports on a feature branch with unrelated changes.
package pipkg

import (
	"archive/tar"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

const piTarget = "pi"

// gateExtensionSource is the exact bytes shipped as
// extensions/labdrian-gate.ts inside the built package (slice 3,
// pi-contract-gate, R-004). Embedding rather than a string literal keeps a
// single source of truth: the file this constant embeds is the same one
// the node-driven Go tests and Pi's own jiti loader execute.
//
//go:embed labdrian-gate.ts
var gateExtensionSource string

// gateContractFiles are the managed contracts labdrian-gate.ts reads at
// runtime (its CONTRACT_RELATIVE_PATHS), copied from overlayRoot's
// skills/_shared/ into the built package's skills/_shared/ (R-004/R-007):
// these two files are not registry-driven, since they are gate
// infrastructure rather than an installable skill.
var gateContractFiles = []string{"minimalism-contract.md", "anti-generic-design.md"}

// GateExtensionSource returns the exact embedded labdrian-gate.ts bytes
// this build ships, for tests that need to run the real source under Node
// without duplicating it.
func GateExtensionSource() string { return gateExtensionSource }

// packageManifest is the subset of package.json fields this package writes.
type packageManifest struct {
	Name     string         `json:"name"`
	Version  string         `json:"version"`
	Pi       piField        `json:"pi"`
	Labdrian *labdrianField `json:"labdrian,omitempty"`
}

// labdrianField is the "labdrian" key inside package.json (R-005): the
// resolved source commit this package was built from, so a later
// sync-check can compare against that exact ref instead of whatever branch
// happens to be checked out (#315).
type labdrianField struct {
	BuiltFrom string `json:"builtFrom,omitempty"`
}

// piField is the "pi" key inside package.json. MCP is a package-relative
// path to mcp.json (A1, R-005) — pi-mcp-adapter reads that file's own
// top-level mcpServers object, not an inline value here.
type piField struct {
	Skills     []string `json:"skills"`
	Agents     []string `json:"agents"`
	Extensions []string `json:"extensions"`
	MCP        string   `json:"mcp"`
}

// mcpConfigFileName is the file package.json's "pi":{"mcp":...} points at
// (A1). registerPi (longterm-mem/internal/register) writes
// mcpServers.longterm-mem into this exact file; Build never overwrites an
// existing one's content (see Build's own doc comment) because that
// registration is state longterm-mem owns, not build output.
const mcpConfigFileName = "mcp.json"

// mcpSkeleton is the empty mcp.json Build ships for a package that has
// never been registered against yet.
const mcpSkeleton = "{\"mcpServers\": {}}\n"

// mcpConfigBakFileName is the backup sibling jsonInstall (the writer
// `longterm-mem register --target pi` uses) writes beside mcp.json on any
// content-changing register/unregister call. It is registration state the
// same as mcp.json itself (Build's own doc comment), so Build preserves it
// across a rebuild and Check excludes it from its content diff, exactly as
// both already do for mcp.json -- otherwise every documented register call
// would show as permanent drift.
const mcpConfigBakFileName = mcpConfigFileName + ".bak"

// Build assembles the labdrian-pi package into destDir: package.json,
// skills/ (every skills.registry.yaml entry whose install.targets includes
// "pi"), agents/GADU.md, and mcp.json (R-005). It builds into a sibling
// temp directory first and atomically swaps it into destDir (R-010),
// refusing any symlink found under a copied source tree and clearing
// whatever previously lived at destDir. File modes are 0644, directory
// modes 0755.
//
// mcp.json is the one file this rebuild does NOT unconditionally
// overwrite: it is registration state `longterm-mem register --target pi`
// owns, not generated build output, so a previously registered destDir's
// mcp.json bytes are carried forward into the freshly built tree before
// the swap — the simplest rule that survives a rebuild without ever
// touching the registered content, and the only one this atomic
// stage/rename/replace shape (swap) permits: destDir is wholesale replaced
// by tmpDir, so anything not copied into tmpDir first is lost.
func Build(overlayRoot, registryPath, destDir string) error {
	if err := checkNoOverlap(overlayRoot, destDir); err != nil {
		return err
	}

	reg, err := loadRegistry(registryPath)
	if err != nil {
		return err
	}

	parentDir := filepath.Dir(destDir)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("pipkg: creating %s: %w", parentDir, err)
	}
	tmpDir, err := os.MkdirTemp(parentDir, ".labdrian-pi-build-*")
	if err != nil {
		return fmt.Errorf("pipkg: creating temp build dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	// os.MkdirTemp creates its directory 0700; the build root (which
	// becomes destDir via swap) must be 0755 like every other directory
	// this package writes (R-002).
	if err := os.Chmod(tmpDir, 0755); err != nil {
		return fmt.Errorf("pipkg: setting build root permissions: %w", err)
	}

	if err := buildInto(overlayRoot, reg, tmpDir, overlayRoot, resolveBuildRev(overlayRoot)); err != nil {
		return err
	}
	if err := preserveIfExists(destDir, tmpDir, mcpConfigFileName); err != nil {
		return err
	}
	if err := preserveIfExists(destDir, tmpDir, mcpConfigBakFileName); err != nil {
		return err
	}
	return swap(tmpDir, destDir)
}

// preserveIfExists copies name from destDir into tmpDir unchanged when it
// exists, so a rebuild carries registration-owned state forward instead of
// losing it to the atomic swap (Build never regenerates name itself).
func preserveIfExists(destDir, tmpDir, name string) error {
	existing, err := os.ReadFile(filepath.Join(destDir, name))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("pipkg: reading existing %s: %w", name, err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, name), existing, 0644); err != nil {
		return fmt.Errorf("pipkg: preserving existing %s: %w", name, err)
	}
	return nil
}

// CheckReport discloses which git state Check actually compared the
// deployed package against (R-006/R-007):
//   - "ref": the deployed package.json's labdrian.builtFrom SHA, resolvable
//     locally. Ref holds that SHA.
//   - "main": builtFrom was absent, non-hex, or not resolvable locally, so
//     Check fell back to comparing against main. Ref holds the recorded
//     (unresolvable) builtFrom value, if any, for the disclosure message.
//   - "worktree": overlayRoot is not a git repository at all, so Check
//     compared against the plain working tree, exactly as before R-005.
type CheckReport struct {
	Basis string
	Ref   string
}

// Disclosure renders a one-line, human-readable statement of what Check
// actually compared against, for callers (pipkg CLI, PiAdapter, the
// sync-check bash helper) to surface (R-006/R-007: the comparison basis
// must always be disclosed, not just on fallback).
func (r CheckReport) Disclosure() string {
	switch r.Basis {
	case "ref":
		return "compared against builtFrom ref " + r.Ref
	case "main":
		if r.Ref != "" {
			return "compared against main; builtFrom " + r.Ref + " is not resolvable locally"
		}
		return "compared against main; builtFrom is not recorded"
	case "worktree":
		return "compared against the worktree (overlay root is not a git repository)"
	default:
		return ""
	}
}

// builtFromPattern is the full-length hex-SHA-1 shape a recorded
// labdrian.builtFrom value must match before it is ever used in a git
// argument (R-007 threat matrix: a non-hex or "--option"-shaped value must
// never reach a git subprocess argv).
var builtFromPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

// Check regenerates the package into a temp dir and diffs it, file by file,
// against destDir. Returns a CheckReport disclosing the comparison basis,
// and a non-nil, drift-naming error when destDir is missing, has extra
// files, is missing files, or has changed content.
//
// The comparison source is resolved by resolveComparisonSource (R-006):
// when destDir's package.json carries a resolvable labdrian.builtFrom SHA,
// Check compares against that commit's exported tree rather than the
// current working-tree checkout, so a feature branch with unrelated
// changes no longer reports false drift (#315). package.json's own
// labdrian.builtFrom field is normalized out of the content comparison
// (via stripBuiltFrom) so recording a different (but still correct) ref
// never counts as drift by itself.
func Check(overlayRoot, registryPath, destDir string) (CheckReport, error) {
	got, err := listFiles(destDir)
	if err != nil {
		if os.IsNotExist(err) {
			return CheckReport{}, fmt.Errorf("pipkg: package not built at %s (run: labdrian-overlay apply --target pi)", destDir)
		}
		return CheckReport{}, fmt.Errorf("pipkg: reading built package: %w", err)
	}

	report, sourceRoot, sourceRegistry, buildRev, cleanup, err := resolveComparisonSource(overlayRoot, registryPath, destDir)
	if err != nil {
		return CheckReport{}, err
	}
	defer cleanup()

	reg, err := loadRegistry(sourceRegistry)
	if err != nil {
		return report, err
	}

	tmpDir, err := os.MkdirTemp("", "labdrian-pi-check-*")
	if err != nil {
		return report, fmt.Errorf("pipkg: creating temp check dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// versionRoot is always the ORIGINAL overlayRoot, never sourceRoot: a
	// git-archive export has no .git directory, so it cannot resolve tag
	// history itself. Using overlayRoot (which has full history) plus the
	// resolved buildRev keeps the comparison's package.json "version"
	// field consistent with what a real Build at that ref would have
	// produced, exactly the same reasoning as builtFrom being normalized
	// out of the diff, but here fixing the input instead of the output.
	if err := buildInto(sourceRoot, reg, tmpDir, overlayRoot, buildRev); err != nil {
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

// resolveComparisonSource decides Check's comparison basis (R-006/R-007)
// and returns the root directory and registry path to build the "want"
// side from, plus a cleanup func for any temp export directory created.
//
//   - overlayRoot is not a git repository at all -> Basis="worktree",
//     comparing directly against overlayRoot/registryPath (unchanged
//     pre-R-005 behavior).
//   - destDir's package.json carries a labdrian.builtFrom value matching
//     builtFromPattern AND resolvable via `git cat-file -e <sha>^{commit}`
//     -> Basis="ref": export skills/, agents/, and skills.registry.yaml at
//     that commit via `git archive` into a temp dir and compare against
//     that.
//   - otherwise (absent, non-hex, or unresolvable) -> Basis="main": the
//     same export, but at "main", with the original (possibly empty,
//     possibly malicious-looking) builtFrom value carried in Ref purely
//     for the disclosure message -- it is NEVER passed to git itself.
//
// buildRev is the ref buildInto's provenance resolution should use for the
// comparison build (fed to resolvePackageVersion against the ORIGINAL
// overlayRoot, which -- unlike a git-archive export -- still has full tag
// history): the resolved builtFrom SHA for "ref", the literal "main" for
// "main" (git describe accepts a branch name), or overlayRoot's current
// HEAD for "worktree" (matching Check's pre-R-005 behavior exactly).
func resolveComparisonSource(overlayRoot, registryPath, destDir string) (report CheckReport, sourceRoot, sourceRegistry, buildRev string, cleanup func(), err error) {
	noopCleanup := func() {}
	if exec.Command("git", "-C", overlayRoot, "rev-parse", "--is-inside-work-tree").Run() != nil {
		return CheckReport{Basis: "worktree"}, overlayRoot, registryPath, resolveBuildRev(overlayRoot), noopCleanup, nil
	}

	builtFrom := readBuiltFrom(destDir)
	if builtFrom != "" && builtFromPattern.MatchString(builtFrom) {
		if exec.Command("git", "-C", overlayRoot, "cat-file", "-e", builtFrom+"^{commit}").Run() == nil {
			root, refCleanup, exportErr := exportGitTree(overlayRoot, builtFrom)
			if exportErr == nil {
				return CheckReport{Basis: "ref", Ref: builtFrom}, root, filepath.Join(root, "skills.registry.yaml"), builtFrom, refCleanup, nil
			}
		}
	}

	root, mainCleanup, exportErr := exportGitTree(overlayRoot, "main")
	if exportErr != nil {
		return CheckReport{}, "", "", "", noopCleanup, fmt.Errorf("pipkg: exporting main for comparison: %w", exportErr)
	}
	return CheckReport{Basis: "main", Ref: builtFrom}, root, filepath.Join(root, "skills.registry.yaml"), "main", mainCleanup, nil
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

	cmd := exec.Command("git", "-C", overlayRoot, "archive", "--format=tar", rev, "--", "skills", "agents", "skills.registry.yaml")
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

// extractTar writes r's tar stream into dest, refusing any entry (symlink
// or otherwise) whose name would resolve outside dest -- git archive never
// produces such entries for a normal repository, but this is defense in
// depth against a corrupted or crafted archive stream.
func extractTar(r io.Reader, dest string) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dest, filepath.FromSlash(hdr.Name))
		if rel, relErr := filepath.Rel(dest, target); relErr != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("refusing tar entry outside destination: %s", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		case tar.TypeSymlink, tar.TypeLink:
			return fmt.Errorf("refusing symlink in git archive: %s", hdr.Name)
		}
	}
}

// buildInto writes the full package tree for reg into dir (either the real
// temp build dir for Build, or a throwaway comparison dir for Check).
// overlayRoot is where skills/agents/_shared source files are read from --
// for Check's ref/main basis this is a throwaway git-archive export, never
// a full checkout. provenanceRoot is always a real git checkout (Build's
// own overlayRoot) with full tag history, used together with rev to
// compute package.json's version and labdrian.builtFrom fields (D5): an
// export has no .git and cannot resolve tags itself, so provenanceRoot
// keeps that resolution correct even when overlayRoot is a export. rev ==
// "" means provenanceRoot is not a git repo (or HEAD is unresolvable),
// yielding the same "0.0.0-dev" / omitted-labdrian fallback as before
// R-005.
func buildInto(overlayRoot string, reg skills.Registry, dir string, provenanceRoot, rev string) error {
	skillsDir := filepath.Join(dir, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		return fmt.Errorf("pipkg: creating skills dir: %w", err)
	}
	for _, e := range reg.Skills {
		if !containsTarget(e.Install.Targets, piTarget) {
			continue
		}
		// R-003 (defense in depth, D3): validateEntry already rejects an
		// unclean/absolute/".."-bearing path at parse time, since the
		// same e.Path is joined to both the source and destination roots
		// below. Re-check the destination join here too, so a future
		// caller that constructs a Registry without going through
		// ParseRegistry cannot escape skillsDir either.
		dst := filepath.Join(skillsDir, e.Path)
		if rel, err := filepath.Rel(skillsDir, dst); err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("pipkg: entry %q: path %q escapes the package skills directory", e.ID, e.Path)
		}
		// R-004: the skill's SKILL.md frontmatter `name` must match the
		// entry's directory name, or the built package would ship a
		// skill that Claude/Codex/Pi cannot address by its own id.
		src := filepath.Join(overlayRoot, "skills", e.Path)
		if err := checkSkillNameMatchesPath(src, e.Path); err != nil {
			return fmt.Errorf("pipkg: entry %q: %w", e.ID, err)
		}
		if err := copyTree(src, dst); err != nil {
			return fmt.Errorf("pipkg: copying skill %q: %w", e.ID, err)
		}
	}

	agentsDir := filepath.Join(dir, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		return fmt.Errorf("pipkg: creating agents dir: %w", err)
	}
	agentSrc := filepath.Join(overlayRoot, "agents", "GADU.md")
	if err := copyFile(agentSrc, filepath.Join(agentsDir, "GADU.md")); err != nil {
		return fmt.Errorf("pipkg: copying agents/GADU.md: %w", err)
	}

	sharedDir := filepath.Join(skillsDir, "_shared")
	if err := os.MkdirAll(sharedDir, 0755); err != nil {
		return fmt.Errorf("pipkg: creating skills/_shared dir: %w", err)
	}
	for _, name := range gateContractFiles {
		src := filepath.Join(overlayRoot, "skills", "_shared", name)
		if err := copyFile(src, filepath.Join(sharedDir, name)); err != nil {
			return fmt.Errorf("pipkg: copying skills/_shared/%s: %w", name, err)
		}
	}

	extensionsDir := filepath.Join(dir, "extensions")
	if err := os.MkdirAll(extensionsDir, 0755); err != nil {
		return fmt.Errorf("pipkg: creating extensions dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(extensionsDir, "labdrian-gate.ts"), []byte(gateExtensionSource), 0644); err != nil {
		return fmt.Errorf("pipkg: writing extensions/labdrian-gate.ts: %w", err)
	}

	if err := os.WriteFile(filepath.Join(dir, mcpConfigFileName), []byte(mcpSkeleton), 0644); err != nil {
		return fmt.Errorf("pipkg: writing %s: %w", mcpConfigFileName, err)
	}

	// D5: feed the caller-resolved rev to both the version tag lookup and
	// labdrian.builtFrom -- "Build already shells to git; single source"
	// (design D5). rev == "" (provenanceRoot not a git repo, or HEAD
	// unresolvable) leaves Labdrian nil (omitempty), exactly the
	// pre-R-005 behavior.
	manifest := packageManifest{
		Name:    "labdrian-pi",
		Version: resolvePackageVersion(provenanceRoot, rev),
		Pi: piField{
			Skills:     []string{"./skills"},
			Agents:     []string{"./agents"},
			Extensions: []string{"./extensions"},
			MCP:        "./" + mcpConfigFileName,
		},
	}
	if rev != "" {
		manifest.Labdrian = &labdrianField{BuiltFrom: rev}
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("pipkg: encoding package.json: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), append(raw, '\n'), 0644); err != nil {
		return fmt.Errorf("pipkg: writing package.json: %w", err)
	}
	return nil
}

// checkSkillNameMatchesPath reads <src>/SKILL.md and requires its
// frontmatter `name:` field to equal filepath.Base(entryPath) (R-004,
// D4). It scans only the line-oriented frontmatter block (between the
// opening and closing "---" delimiters) for a top-level "name:" key,
// deliberately not a general YAML parser -- validateEntry in
// engine/skills stays filesystem-free by design, so this filesystem-aware
// check lives here instead.
func checkSkillNameMatchesPath(src, entryPath string) error {
	data, err := os.ReadFile(filepath.Join(src, "SKILL.md"))
	if err != nil {
		return fmt.Errorf("reading SKILL.md: %w", err)
	}
	name, err := frontmatterName(string(data))
	if err != nil {
		return err
	}
	want := filepath.Base(entryPath)
	if name != want {
		return fmt.Errorf("SKILL.md name %q does not match directory %q", name, want)
	}
	return nil
}

// frontmatterName extracts the `name:` value from a SKILL.md's YAML
// frontmatter block (the text between the first two "---" delimiter
// lines). It is a minimal, line-oriented scan -- not a YAML parser -- and
// returns an error if no frontmatter block or no name key is found.
func frontmatterName(content string) (string, error) {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", fmt.Errorf("SKILL.md has no frontmatter block")
	}
	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			return "", fmt.Errorf("SKILL.md frontmatter has no %q field", "name")
		}
		rest, ok := strings.CutPrefix(trimmed, "name:")
		if !ok {
			continue
		}
		name := strings.TrimSpace(rest)
		name = strings.Trim(name, `"'`)
		return name, nil
	}
	return "", fmt.Errorf("SKILL.md frontmatter is not terminated")
}

// loadRegistry parses the skills registry at registryPath.
func loadRegistry(registryPath string) (skills.Registry, error) {
	f, err := os.Open(registryPath)
	if err != nil {
		return skills.Registry{}, fmt.Errorf("pipkg: opening registry: %w", err)
	}
	defer f.Close()
	reg, err := skills.ParseRegistry(f)
	if err != nil {
		return skills.Registry{}, fmt.Errorf("pipkg: parsing registry: %w", err)
	}
	return reg, nil
}

// resolveBuildRev resolves overlayRoot's checked-out HEAD commit SHA
// (R-005), returning "" when overlayRoot is not a git repo or HEAD cannot
// be resolved (this is a local, no-fetch lookup — never touches the
// network).
func resolveBuildRev(overlayRoot string) string {
	out, err := exec.Command("git", "-C", overlayRoot, "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// resolvePackageVersion resolves the newest reachable "v*"-tag reachable
// from rev (D5: the same resolved commit labdrian.builtFrom records,
// "single source"), stripped of its leading "v", falling back to
// "0.0.0-dev" when no tag is reachable, rev is empty, or overlayRoot is not
// a git repo (this is a local, no-fetch lookup — never touches the
// network).
func resolvePackageVersion(overlayRoot, rev string) string {
	args := []string{"-C", overlayRoot, "describe", "--tags", "--abbrev=0", "--match", "v*"}
	if rev != "" {
		args = append(args, rev)
	}
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return "0.0.0-dev"
	}
	return strings.TrimPrefix(strings.TrimSpace(string(out)), "v")
}

// checkNoOverlap refuses a destDir that equals, is inside, or contains
// overlayRoot (R3-destination-overlap).
func checkNoOverlap(overlayRoot, destDir string) error {
	absOverlay, err := filepath.Abs(overlayRoot)
	if err != nil {
		return fmt.Errorf("pipkg: resolving overlay root: %w", err)
	}
	if r, err := filepath.EvalSymlinks(absOverlay); err == nil {
		absOverlay = r
	}
	absDest, err := filepath.Abs(destDir)
	if err != nil {
		return fmt.Errorf("pipkg: resolving destination: %w", err)
	}
	absDest = resolveExistingAncestors(absDest)
	contains := func(base, target string) bool {
		rel, err := filepath.Rel(base, target)
		return err == nil && !strings.HasPrefix(rel, "..")
	}
	if contains(absOverlay, absDest) || contains(absDest, absOverlay) {
		return fmt.Errorf("pipkg: destination %s overlaps overlay root %s", destDir, overlayRoot)
	}
	return nil
}

// resolveExistingAncestors canonicalizes path by evaluating symlinks on its
// longest existing ancestor and re-appending the nonexistent tail, so a
// destination reached through a symlinked parent cannot escape the overlap
// check merely because it does not exist yet.
func resolveExistingAncestors(path string) string {
	tail := ""
	for cur := path; ; {
		if r, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(r, tail)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return path
		}
		tail = filepath.Join(filepath.Base(cur), tail)
		cur = parent
	}
}

// containsTarget reports whether targets contains want.
func containsTarget(targets []string, want string) bool {
	for _, t := range targets {
		if t == want {
			return true
		}
	}
	return false
}

// copyTree recursively copies src into dst, refusing any symlink found
// anywhere in the tree (R-010) and setting 0755 on directories / 0644 on
// files.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlink at %s", path)
		}
		if !d.IsDir() && !d.Type().IsRegular() {
			return fmt.Errorf("refusing non-regular file at %s", path)
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		return copyFile(path, target)
	})
}

// copyFile copies src to dst, refusing a symlink source (R-010).
func copyFile(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing symlink at %s", src)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

// swap atomically replaces destDir with tmpDir's content, staging any
// previous destDir aside in a freshly created sibling dir and removing only
// that fresh dir afterward — a pre-existing "<destDir>.stale" is never
// touched (R3-stale-directory-deletion). On failure it restores destDir.
func swap(tmpDir, destDir string) error {
	hadPrevious := false
	var staleDir string
	if _, err := os.Stat(destDir); err == nil {
		staleDir, err = os.MkdirTemp(filepath.Dir(destDir), ".labdrian-pi-stale-*")
		if err != nil {
			return fmt.Errorf("pipkg: creating stale staging dir: %w", err)
		}
		if err := syscall.Rename(destDir, staleDir); err != nil { // os.Rename refuses a dir newpath
			_ = os.RemoveAll(staleDir)
			return fmt.Errorf("pipkg: staging previous package aside: %w", err)
		}
		hadPrevious = true
	}

	if err := os.Rename(tmpDir, destDir); err != nil {
		if hadPrevious {
			_ = os.Rename(staleDir, destDir)
		}
		return fmt.Errorf("pipkg: swapping built package into place: %w", err)
	}
	if hadPrevious {
		_ = os.RemoveAll(staleDir)
	}
	return nil
}

// fileEntry is one file's content plus its permission bits, as recorded by
// listFiles. Check diffs both: a byte-identical file whose mode changed is
// still drift (R-001).
type fileEntry struct {
	data []byte
	perm fs.FileMode
}

// listFiles walks root and returns every regular file's content and mode,
// keyed by its slash-separated path relative to root.
func listFiles(root string) (map[string]fileEntry, error) {
	out := make(map[string]fileEntry)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlink at %s", path)
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("refusing non-regular file at %s", path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = fileEntry{data: data, perm: info.Mode().Perm()}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
