package pipkg_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pipkg"
)

// fixtureOverlay builds a minimal overlay tree: two skills (one targeted at
// pi, one not), an agents/GADU.md, and a matching skills.registry.yaml.
// Returns the overlay root and the registry path.
func fixtureOverlay(t *testing.T) (overlayRoot, registryPath string) {
	t.Helper()
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "skills", "pi-skill", "SKILL.md"), "---\nname: pi-skill\n---\nbody\n")
	writeFile(t, filepath.Join(root, "skills", "pi-skill", "references", "notes.md"), "notes\n")
	writeFile(t, filepath.Join(root, "skills", "other-skill", "SKILL.md"), "---\nname: other-skill\n---\nbody\n")
	writeFile(t, filepath.Join(root, "agents", "GADU.md"), "---\nname: GADU\n---\nbody\n")

	registry := `version: "1"
skills:
  - id: pi-skill
    path: pi-skill
    source:
      type: custom
    install:
      defaultScope: global
      targets:
        - claude
        - pi
    lifecycle:
      updateStrategy: overlay-only
  - id: other-skill
    path: other-skill
    source:
      type: custom
    install:
      defaultScope: global
      targets:
        - claude
    lifecycle:
      updateStrategy: overlay-only
`
	regPath := filepath.Join(root, "skills.registry.yaml")
	writeFile(t, regPath, registry)

	return root, regPath
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestPipkgBuild_SelectsPiTargetedSkills(t *testing.T) {
	overlayRoot, registryPath := fixtureOverlay(t)
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")

	if err := pipkg.Build(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("Build: %v", err)
	}

	// pi-skill (and its references subdir) is copied; other-skill is not.
	if _, err := os.Stat(filepath.Join(destDir, "skills", "pi-skill", "SKILL.md")); err != nil {
		t.Errorf("expected pi-skill/SKILL.md to be built: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, "skills", "pi-skill", "references", "notes.md")); err != nil {
		t.Errorf("expected pi-skill/references/notes.md to be built: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, "skills", "other-skill")); err == nil {
		t.Error("other-skill must NOT be built (install.targets excludes pi)")
	}

	if _, err := os.Stat(filepath.Join(destDir, "agents", "GADU.md")); err != nil {
		t.Errorf("expected agents/GADU.md to be built: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(destDir, "package.json"))
	if err != nil {
		t.Fatalf("read package.json: %v", err)
	}
	var manifest struct {
		Name string `json:"name"`
		Pi   struct {
			Skills     []string `json:"skills"`
			Agents     []string `json:"agents"`
			Extensions []string `json:"extensions"`
			MCP        string   `json:"mcp"`
		} `json:"pi"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("unmarshal package.json: %v", err)
	}
	if manifest.Name != "labdrian-pi" {
		t.Errorf("package.json name = %q, want labdrian-pi", manifest.Name)
	}
	// No git tags reachable in this bare fixture repo → 0.0.0-dev fallback.
	if manifest.Version != "0.0.0-dev" {
		t.Errorf("package.json version = %q, want 0.0.0-dev", manifest.Version)
	}
	if len(manifest.Pi.Skills) != 1 || manifest.Pi.Skills[0] != "./skills" {
		t.Errorf("package.json pi.skills = %v, want [./skills]", manifest.Pi.Skills)
	}
	if len(manifest.Pi.Agents) != 1 || manifest.Pi.Agents[0] != "./agents" {
		t.Errorf("package.json pi.agents = %v, want [./agents]", manifest.Pi.Agents)
	}
	if len(manifest.Pi.Extensions) != 1 || manifest.Pi.Extensions[0] != "./extensions" {
		t.Errorf("package.json pi.extensions = %v, want [./extensions]", manifest.Pi.Extensions)
	}
	if manifest.Pi.MCP != "" {
		t.Errorf("package.json pi.mcp = %q, want omitted (slice 4 not landed)", manifest.Pi.MCP)
	}
}

func TestPipkgBuild_RejectsSymlinks(t *testing.T) {
	overlayRoot, registryPath := fixtureOverlay(t)
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")

	realFile := filepath.Join(overlayRoot, "skills", "pi-skill", "escape.txt")
	writeFile(t, realFile, "outside\n")
	linkPath := filepath.Join(overlayRoot, "skills", "pi-skill", "link.txt")
	if err := os.Symlink(realFile, linkPath); err != nil {
		t.Skipf("symlink unsupported in this environment: %v", err)
	}

	err := pipkg.Build(overlayRoot, registryPath, destDir)
	if err == nil {
		t.Fatal("Build must refuse a symlink under a copied skill directory")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("error should mention symlink, got: %v", err)
	}
	if _, statErr := os.Stat(destDir); statErr == nil {
		t.Error("destDir must not be created when Build refuses a symlink")
	}
}

func TestPipkgBuild_AtomicSwap(t *testing.T) {
	overlayRoot, registryPath := fixtureOverlay(t)
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")

	// Simulate stale prior content that must be cleared on swap.
	writeFile(t, filepath.Join(destDir, "stale.txt"), "leftover\n")
	writeFile(t, filepath.Join(destDir, "skills", "other-skill", "SKILL.md"), "stale\n")

	if err := pipkg.Build(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("Build: %v", err)
	}

	if _, err := os.Stat(filepath.Join(destDir, "stale.txt")); err == nil {
		t.Error("stale top-level file must be cleared by the atomic swap")
	}
	if _, err := os.Stat(filepath.Join(destDir, "skills", "other-skill")); err == nil {
		t.Error("stale other-skill dir must be cleared by the atomic swap")
	}
	info, err := os.Stat(filepath.Join(destDir, "skills"))
	if err != nil {
		t.Fatalf("stat destDir/skills: %v", err)
	}
	if info.Mode().Perm() != 0755 {
		t.Errorf("destDir/skills mode = %o, want 0755", info.Mode().Perm())
	}
	fileInfo, err := os.Stat(filepath.Join(destDir, "package.json"))
	if err != nil {
		t.Fatalf("stat package.json: %v", err)
	}
	if fileInfo.Mode().Perm() != 0644 {
		t.Errorf("package.json mode = %o, want 0644", fileInfo.Mode().Perm())
	}
}

func TestPipkgCheck_DetectsDrift(t *testing.T) {
	overlayRoot, registryPath := fixtureOverlay(t)
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")

	if err := pipkg.Check(overlayRoot, registryPath, destDir); err == nil {
		t.Fatal("Check must fail when the package has never been built")
	}

	if err := pipkg.Build(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := pipkg.Check(overlayRoot, registryPath, destDir); err != nil {
		t.Errorf("Check must report no drift right after Build, got: %v", err)
	}

	// A source edit changes what the build would produce → drift.
	writeFile(t, filepath.Join(overlayRoot, "skills", "pi-skill", "SKILL.md"), "---\nname: pi-skill\n---\nchanged body\n")
	err := pipkg.Check(overlayRoot, registryPath, destDir)
	if err == nil {
		t.Fatal("Check must detect drift after a source edit")
	}
	if !strings.Contains(err.Error(), "pi-skill") {
		t.Errorf("drift error should name the drifted entry, got: %v", err)
	}
}

func TestPipkgBuild_OverlapAndStaleDirSafety(t *testing.T) {
	overlayRoot, registryPath := fixtureOverlay(t)
	for _, dest := range []string{overlayRoot, filepath.Join(overlayRoot, "skills"), filepath.Dir(overlayRoot)} {
		if err := pipkg.Build(overlayRoot, registryPath, dest); err == nil || !strings.Contains(err.Error(), "overlaps overlay root") {
			t.Errorf("Build(dest=%s) = %v, want overlap error", dest, err)
		}
	}
	if _, err := os.Stat(filepath.Join(overlayRoot, "skills", "pi-skill", "SKILL.md")); err != nil {
		t.Errorf("overlayRoot must remain intact: %v", err)
	}
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")
	keep := filepath.Join(destDir+".stale", "keep.txt")
	writeFile(t, keep, "user content\n")
	for i := 0; i < 2; i++ {
		if err := pipkg.Build(overlayRoot, registryPath, destDir); err != nil {
			t.Fatalf("Build #%d: %v", i, err)
		}
	}
	if data, err := os.ReadFile(keep); err != nil || string(data) != "user content\n" {
		t.Errorf("pre-existing stale dir must survive, got %q, err=%v", data, err)
	}
}

// runWithTimeout fails unless fn returns within 5s and its error mentions want.
func runWithTimeout(t *testing.T, fn func() error, want string) {
	t.Helper()
	errCh := make(chan error, 1)
	go func() { errCh <- fn() }()
	select {
	case err := <-errCh:
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("got %v, want error containing %q", err, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("hung instead of refusing the non-regular file")
	}
}

func TestPipkgRefusesSpecialFiles(t *testing.T) {
	overlayRoot, registryPath := fixtureOverlay(t)
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")
	srcFifo := filepath.Join(overlayRoot, "skills", "pi-skill", "pipe")
	if err := syscall.Mkfifo(srcFifo, 0644); err != nil {
		t.Skipf("mkfifo unsupported: %v", err)
	}
	runWithTimeout(t, func() error { return pipkg.Build(overlayRoot, registryPath, destDir) }, "non-regular")
	os.Remove(srcFifo)
	if err := pipkg.Build(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("Build: %v", err)
	}
	dstFifo := filepath.Join(destDir, "pipe")
	syscall.Mkfifo(dstFifo, 0644)
	runWithTimeout(t, func() error { return pipkg.Check(overlayRoot, registryPath, destDir) }, "non-regular")
	os.Remove(dstFifo)
	link := filepath.Join(destDir, "link.json")
	if err := os.Symlink(filepath.Join(destDir, "package.json"), link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if err := pipkg.Check(overlayRoot, registryPath, destDir); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Errorf("Check = %v, want symlink error", err)
	}
}
