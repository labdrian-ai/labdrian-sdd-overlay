// Package pipkg builds the labdrian-pi package tree (package.json, skills/,
// agents/) that gets installed into a Pi (gentle-pi) session via
// `pi install <path>`. It mirrors engine/gadu's Generate/Check idiom: Build
// writes the package, Check regenerates into a temp dir and diffs against
// the deployed copy to report drift (R-003).
package pipkg

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
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
	Name    string  `json:"name"`
	Version string  `json:"version"`
	Pi      piField `json:"pi"`
}

// piField is the "pi" key inside package.json. MCP is omitted (empty)
// until the pi-longterm-mem-mcp slice lands (R-002 scope boundary).
type piField struct {
	Skills     []string `json:"skills"`
	Agents     []string `json:"agents"`
	Extensions []string `json:"extensions"`
	MCP        string   `json:"mcp,omitempty"`
}

// Build assembles the labdrian-pi package into destDir: package.json,
// skills/ (every skills.registry.yaml entry whose install.targets includes
// "pi"), and agents/GADU.md. It builds into a sibling temp directory first
// and atomically swaps it into destDir (R-010), refusing any symlink found
// under a copied source tree and clearing whatever previously lived at
// destDir. File modes are 0644, directory modes 0755.
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

	if err := buildInto(overlayRoot, reg, tmpDir); err != nil {
		return err
	}
	return swap(tmpDir, destDir)
}

// Check regenerates the package into a temp dir and diffs it, file by file,
// against destDir. Returns a non-nil, drift-naming error when destDir is
// missing, has extra files, is missing files, or has changed content.
func Check(overlayRoot, registryPath, destDir string) error {
	reg, err := loadRegistry(registryPath)
	if err != nil {
		return err
	}

	tmpDir, err := os.MkdirTemp("", "labdrian-pi-check-*")
	if err != nil {
		return fmt.Errorf("pipkg: creating temp check dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := buildInto(overlayRoot, reg, tmpDir); err != nil {
		return fmt.Errorf("pipkg: regenerating for check: %w", err)
	}

	want, err := listFiles(tmpDir)
	if err != nil {
		return fmt.Errorf("pipkg: reading regenerated package: %w", err)
	}
	got, err := listFiles(destDir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("pipkg: package not built at %s (run: labdrian-overlay apply --target pi)", destDir)
		}
		return fmt.Errorf("pipkg: reading built package: %w", err)
	}

	var drift []string
	for rel, wantBytes := range want {
		gotBytes, ok := got[rel]
		if !ok {
			drift = append(drift, fmt.Sprintf("%s: missing", rel))
			continue
		}
		if string(wantBytes) != string(gotBytes) {
			drift = append(drift, fmt.Sprintf("%s: changed", rel))
		}
	}
	for rel := range got {
		if _, ok := want[rel]; !ok {
			drift = append(drift, fmt.Sprintf("%s: extra", rel))
		}
	}
	if len(drift) > 0 {
		sort.Strings(drift)
		return fmt.Errorf("labdrian-pi package drift:\n  %s", strings.Join(drift, "\n  "))
	}
	return nil
}

// buildInto writes the full package tree for reg into dir (either the real
// temp build dir for Build, or a throwaway comparison dir for Check).
func buildInto(overlayRoot string, reg skills.Registry, dir string) error {
	skillsDir := filepath.Join(dir, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		return fmt.Errorf("pipkg: creating skills dir: %w", err)
	}
	for _, e := range reg.Skills {
		if !containsTarget(e.Install.Targets, piTarget) {
			continue
		}
		src := filepath.Join(overlayRoot, "skills", e.Path)
		dst := filepath.Join(skillsDir, e.Path)
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

	manifest := packageManifest{
		Name:    "labdrian-pi",
		Version: resolvePackageVersion(overlayRoot),
		Pi: piField{
			Skills:     []string{"./skills"},
			Agents:     []string{"./agents"},
			Extensions: []string{"./extensions"},
		},
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

// resolvePackageVersion resolves the newest reachable "v*"-tag from
// overlayRoot's git history, stripped of its leading "v", falling back to
// "0.0.0-dev" when no tag is reachable or overlayRoot is not a git repo
// (this is a local, no-fetch lookup — never touches the network).
func resolvePackageVersion(overlayRoot string) string {
	out, err := exec.Command("git", "-C", overlayRoot, "describe", "--tags", "--abbrev=0", "--match", "v*").Output()
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

// listFiles walks root and returns every regular file's content, keyed by
// its slash-separated path relative to root.
func listFiles(root string) (map[string][]byte, error) {
	out := make(map[string][]byte)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlink at %s", path)
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
		out[filepath.ToSlash(rel)] = data
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
