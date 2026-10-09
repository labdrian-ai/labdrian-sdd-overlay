package pipkg

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pathguard/fsresolve"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

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
func (p Packages) Build(overlayRoot, registryPath, destDir string) error {
	if err := checkNoOverlap(overlayRoot, destDir); err != nil {
		return err
	}

	// A build refuses a registry the reader did not read whole and says what was left out in the
	// refusal, so it reads without the repository's warning, which would say it before. (Check
	// goes on with such a registry and keeps the warning.)
	reg, err := loadRegistry(skills.WithoutUnreadWarning(p.Registries), registryPath)
	if err != nil {
		return err
	}
	// A package is an artifact others consume: it is not built from a registry the reader did not
	// read whole, and nothing is written when it refuses (decision 4 of the owner).
	if err := reg.CheckBuildable(); err != nil {
		return fmt.Errorf("pipkg: %w", err)
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

	rev, err := p.resolveBuildRev(overlayRoot)
	if err != nil {
		return err
	}
	if err := p.buildInto(overlayRoot, reg, tmpDir, overlayRoot, rev); err != nil {
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
func (p Packages) buildInto(overlayRoot string, reg skills.Registry, dir string, provenanceRoot, rev string) error {
	skillsDir := filepath.Join(dir, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		return fmt.Errorf("pipkg: creating skills dir: %w", err)
	}
	for _, e := range reg.Skills {
		if !containsTarget(e.Install.Targets, piTarget) {
			continue
		}
		// R-003 (defense in depth, D3): Registry.Validate already rejects an
		// unclean/absolute/".."-bearing path when the registry is read, since the
		// same e.Path is joined to both the source and destination roots
		// below. Re-check the destination join here too, so a future
		// caller that constructs a Registry without going through
		// skills.ReadRegistry cannot escape skillsDir either.
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
	agentSrc := resolveGaduAgentSource(overlayRoot)
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
	version, err := p.resolvePackageVersion(provenanceRoot, rev)
	if err != nil {
		return err
	}
	manifest := packageManifest{
		Name:    "labdrian-pi",
		Version: version,
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

// resolveGaduAgentSource returns the source path this package's
// agents/GADU.md is copied from: overlayRoot's pi-specific
// pi/agents/GADU.md (the compact, pi-claude-cli-modeled variant
// engine/gadu.Generate emits) when present, else overlayRoot's generic
// agents/GADU.md -- kept as a fallback so an older checkout that predates
// the pi/agents/GADU.md variant still builds.
func resolveGaduAgentSource(overlayRoot string) string {
	piSpecific := filepath.Join(overlayRoot, "pi", "agents", "GADU.md")
	if _, err := os.Stat(piSpecific); err == nil {
		return piSpecific
	}
	return filepath.Join(overlayRoot, "agents", "GADU.md")
}

// checkSkillNameMatchesPath reads <src>/SKILL.md and requires its
// frontmatter `name:` field to equal filepath.Base(entryPath) (R-004,
// D4). The frontmatter is read by skills.ReadFrontmatter, the same reader
// the lint uses, which is deliberately not a general YAML parser --
// validateEntry in engine/skills stays filesystem-free by design, so this
// filesystem-aware check lives here instead.
func checkSkillNameMatchesPath(src, entryPath string) error {
	data, err := os.ReadFile(filepath.Join(src, "SKILL.md"))
	if err != nil {
		return fmt.Errorf("reading SKILL.md: %w", err)
	}
	name, err := skillName(data)
	if err != nil {
		return err
	}
	want := filepath.Base(entryPath)
	if name != want {
		return fmt.Errorf("SKILL.md name %q does not match directory %q", name, want)
	}
	return nil
}

// skillName is the `name` a SKILL.md declares at the top level of its frontmatter, said in the
// words the build has always used for what it cannot find.
func skillName(data []byte) (string, error) {
	fm, err := skills.ReadFrontmatter(data)
	if err != nil {
		// ReadFrontmatter fails only for a missing fence (a *FrontmatterError); anything it
		// might return later is not read as a frontmatter either.
		var fault *skills.FrontmatterError
		if errors.As(err, &fault) && fault.Fault == skills.NoClosingFence {
			return "", fmt.Errorf("SKILL.md frontmatter is not terminated")
		}
		return "", fmt.Errorf("SKILL.md has no frontmatter block")
	}
	entry, ok := fm.Entry("name")
	if !ok {
		return "", fmt.Errorf("SKILL.md frontmatter has no %q field", "name")
	}
	return entry.Text(), nil
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
	// The destination may not exist yet, so its existing ancestry is resolved and the rest kept
	// as written. A link in that ancestry whose target does not exist yet is refused, not
	// followed as if absent: creating the destination would create the target, which may lie
	// inside the overlay root.
	absDest, err = fsresolve.KeepingMissing(absDest)
	if err != nil {
		return fmt.Errorf("pipkg: resolving destination: %w", err)
	}
	contains := func(base, target string) bool {
		rel, err := filepath.Rel(base, target)
		return err == nil && !strings.HasPrefix(rel, "..")
	}
	if contains(absOverlay, absDest) || contains(absDest, absOverlay) {
		return fmt.Errorf("pipkg: destination %s overlaps overlay root %s", destDir, overlayRoot)
	}
	return nil
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
