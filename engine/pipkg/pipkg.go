// Package pipkg builds the labdrian-pi package tree (package.json, skills/,
// agents/) that gets installed into a Pi (gentle-pi) session via
// `pi install <path>`. It mirrors engine/gadu's Generate/Check idiom: Build
// writes the package, Check regenerates into a temp dir and diffs against
// the deployed copy to report drift (R-003).
package pipkg

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

const piTarget = "pi"

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

// swap atomically replaces destDir with tmpDir's content: the previous
// destDir (if any) is renamed aside, tmpDir is renamed into place, then the
// staged-aside previous content is removed — clearing any stale files a
// prior build left behind. On rename failure it restores the prior
// destDir so a failed swap never leaves destDir absent.
func swap(tmpDir, destDir string) error {
	staleDir := destDir + ".stale"
	_ = os.RemoveAll(staleDir)

	hadPrevious := false
	if _, err := os.Stat(destDir); err == nil {
		if err := os.Rename(destDir, staleDir); err != nil {
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
	_ = os.RemoveAll(staleDir)
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
