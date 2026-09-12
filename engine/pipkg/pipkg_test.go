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
	writeFile(t, filepath.Join(root, "skills", "_shared", "minimalism-contract.md"),
		"---\napplies_to_phases: [sdd-tasks, sdd-apply]\nexcluded_phases: [sdd-verify]\ninjection_point: \"## Skills to load before work\"\n---\nbody\n")
	writeFile(t, filepath.Join(root, "skills", "_shared", "anti-generic-design.md"),
		"---\napplies_to_phases: [sdd-tasks, sdd-apply]\nexcluded_phases: [sdd-verify]\ninjection_point: \"## Skills to load before work\"\n---\nbody\n")

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
	if manifest.Pi.MCP != "./mcp.json" {
		t.Errorf("package.json pi.mcp = %q, want ./mcp.json", manifest.Pi.MCP)
	}
	mcpRaw, err := os.ReadFile(filepath.Join(destDir, "mcp.json"))
	if err != nil {
		t.Fatalf("expected mcp.json to be built: %v", err)
	}
	var mcpDoc struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(mcpRaw, &mcpDoc); err != nil {
		t.Fatalf("unmarshal mcp.json: %v", err)
	}
	if len(mcpDoc.MCPServers) != 0 {
		t.Errorf("fresh mcp.json mcpServers = %v, want empty", mcpDoc.MCPServers)
	}

	gateBytes, err := os.ReadFile(filepath.Join(destDir, "extensions", "labdrian-gate.ts"))
	if err != nil {
		t.Fatalf("expected extensions/labdrian-gate.ts to be built: %v", err)
	}
	if string(gateBytes) != pipkg.GateExtensionSource() {
		t.Error("built extensions/labdrian-gate.ts must match the embedded source exactly")
	}
	for _, name := range []string{"minimalism-contract.md", "anti-generic-design.md"} {
		if _, err := os.Stat(filepath.Join(destDir, "skills", "_shared", name)); err != nil {
			t.Errorf("expected skills/_shared/%s to be built: %v", name, err)
		}
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

	if _, err := pipkg.Check(overlayRoot, registryPath, destDir); err == nil {
		t.Fatal("Check must fail when the package has never been built")
	}

	if err := pipkg.Build(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("Build: %v", err)
	}
	_, checkErr := pipkg.Check(overlayRoot, registryPath, destDir)
	if checkErr != nil {
		t.Errorf("Check must report no drift right after Build, got: %v", checkErr)
	}

	// A source edit changes what the build would produce → drift.
	writeFile(t, filepath.Join(overlayRoot, "skills", "pi-skill", "SKILL.md"), "---\nname: pi-skill\n---\nchanged body\n")
	_, err := pipkg.Check(overlayRoot, registryPath, destDir)
	if err == nil {
		t.Fatal("Check must detect drift after a source edit")
	}
	if !strings.Contains(err.Error(), "pi-skill") {
		t.Errorf("drift error should name the drifted entry, got: %v", err)
	}
}

// TestPipkgCheck_ModeDrift (R-001): a byte-identical file whose mode
// changed after Build (e.g. chmod 0644 -> 0755) must be reported as
// drifted even though its content is untouched.
func TestPipkgCheck_ModeDrift(t *testing.T) {
	overlayRoot, registryPath := fixtureOverlay(t)
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")

	if err := pipkg.Build(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, err := pipkg.Check(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("Check must report no drift right after Build, got: %v", err)
	}

	target := filepath.Join(destDir, "skills", "pi-skill", "SKILL.md")
	if err := os.Chmod(target, 0755); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	_, err := pipkg.Check(overlayRoot, registryPath, destDir)
	if err == nil {
		t.Fatal("Check must detect mode-only drift (0644 -> 0755)")
	}
	if !strings.Contains(err.Error(), "skills/pi-skill/SKILL.md") {
		t.Errorf("drift error should name the drifted entry, got: %v", err)
	}
	if !strings.Contains(err.Error(), "0644") || !strings.Contains(err.Error(), "0755") {
		t.Errorf("drift error should name both modes (0644 -> 0755), got: %v", err)
	}
}

// TestPipkgCheck_SymlinkedDestRootRefused (R-001, defense in depth): a
// destDir whose root itself is a symlink must be refused by Check, not
// silently followed into whatever it points at. listFiles already refuses
// this via WalkDir's Lstat-based root entry; this test pins that behavior.
func TestPipkgCheck_SymlinkedDestRootRefused(t *testing.T) {
	overlayRoot, registryPath := fixtureOverlay(t)
	realDest := filepath.Join(t.TempDir(), "labdrian-pi")
	if err := pipkg.Build(overlayRoot, registryPath, realDest); err != nil {
		t.Fatalf("Build: %v", err)
	}

	linkedDest := filepath.Join(t.TempDir(), "labdrian-pi-link")
	if err := os.Symlink(realDest, linkedDest); err != nil {
		t.Skipf("symlink unsupported in this environment: %v", err)
	}

	_, err := pipkg.Check(overlayRoot, registryPath, linkedDest)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Errorf("Check(destDir=symlink) = %v, want symlink refusal", err)
	}
}

// TestPipkgBuild_RootPermissions (R-002): the built package root itself
// (destDir after the atomic swap) must be 0755, not the 0700 MkdirTemp
// default.
func TestPipkgBuild_RootPermissions(t *testing.T) {
	overlayRoot, registryPath := fixtureOverlay(t)
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")

	if err := pipkg.Build(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("Build: %v", err)
	}

	info, err := os.Stat(destDir)
	if err != nil {
		t.Fatalf("stat destDir: %v", err)
	}
	if info.Mode().Perm() != 0755 {
		t.Errorf("destDir mode = %o, want 0755", info.Mode().Perm())
	}
}

// TestPipkgBuild_RejectsNameMismatch (R-004): a skill whose SKILL.md
// frontmatter `name` does not match its registry directory must be
// rejected before any file is written for it.
func TestPipkgBuild_RejectsNameMismatch(t *testing.T) {
	overlayRoot, registryPath := fixtureOverlay(t)
	writeFile(t, filepath.Join(overlayRoot, "skills", "mismatch-skill", "SKILL.md"), "---\nname: something-else\n---\nbody\n")

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
  - id: mismatch-skill
    path: mismatch-skill
    source:
      type: custom
    install:
      defaultScope: global
      targets:
        - pi
    lifecycle:
      updateStrategy: overlay-only
`
	writeFile(t, registryPath, registry)

	destDir := filepath.Join(t.TempDir(), "labdrian-pi")
	err := pipkg.Build(overlayRoot, registryPath, destDir)
	if err == nil {
		t.Fatal("Build must reject a skill whose SKILL.md name != directory name")
	}
	if !strings.Contains(err.Error(), "mismatch-skill") || !strings.Contains(err.Error(), "something-else") {
		t.Errorf("error should name the entry and the declared name, got: %v", err)
	}
	if _, statErr := os.Stat(destDir); statErr == nil {
		t.Error("destDir must not be created when Build rejects a name mismatch")
	}
}

// TestPipkgBuild_LiveRegistryNamesMatch (R-004) pins that the real overlay
// registry's skill directories all match their SKILL.md frontmatter names,
// so the new check never blocks a legitimate rebuild of the shipped
// package.
func TestPipkgBuild_LiveRegistryNamesMatch(t *testing.T) {
	overlayRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolving overlay root: %v", err)
	}
	registryPath := filepath.Join(overlayRoot, "skills.registry.yaml")
	if _, err := os.Stat(registryPath); err != nil {
		t.Skipf("live registry not found at %s: %v", registryPath, err)
	}
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")
	if err := pipkg.Build(overlayRoot, registryPath, destDir); err != nil {
		t.Errorf("Build against the live registry must pass the name/directory check, got: %v", err)
	}
}

func TestPipkgBuild_OverlapAndStaleDirSafety(t *testing.T) {
	overlayRoot, registryPath := fixtureOverlay(t)
	for _, dest := range []string{overlayRoot, filepath.Join(overlayRoot, "skills"), filepath.Dir(overlayRoot)} {
		if err := pipkg.Build(overlayRoot, registryPath, dest); err == nil || !strings.Contains(err.Error(), "overlaps overlay root") {
			t.Errorf("Build(dest=%s) = %v, want overlap error", dest, err)
		}
	}
	link := filepath.Join(t.TempDir(), "link-to-overlay")
	if err := os.Symlink(overlayRoot, link); err != nil {
		t.Fatal(err)
	}
	viaLink := filepath.Join(link, "not-yet", "labdrian-pi")
	if err := pipkg.Build(overlayRoot, registryPath, viaLink); err == nil || !strings.Contains(err.Error(), "overlaps overlay root") {
		t.Errorf("Build(dest via symlinked ancestor, nonexistent) = %v, want overlap error", err)
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
	runWithTimeout(t, func() error { _, err := pipkg.Check(overlayRoot, registryPath, destDir); return err }, "non-regular")
	os.Remove(dstFifo)
	link := filepath.Join(destDir, "link.json")
	if err := os.Symlink(filepath.Join(destDir, "package.json"), link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if _, err := pipkg.Check(overlayRoot, registryPath, destDir); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Errorf("Check = %v, want symlink error", err)
	}
}

// TestPipkgBuild_PreservesRegisteredMcpJSON pins the rule Build's own doc
// comment states: mcp.json is registration state `longterm-mem register
// --target pi` owns, so a rebuild must carry an already-registered
// mcp.json's bytes forward rather than clobbering them with a fresh empty
// skeleton (R-005).
func TestPipkgBuild_PreservesRegisteredMcpJSON(t *testing.T) {
	overlayRoot, registryPath := fixtureOverlay(t)
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")

	if err := pipkg.Build(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("first Build: %v", err)
	}

	registered := `{"mcpServers":{"longterm-mem":{"type":"stdio","command":"/opt/labdrian-overlay/bin/longterm-mem","args":["mcp"]}}}`
	if err := os.WriteFile(filepath.Join(destDir, "mcp.json"), []byte(registered), 0644); err != nil {
		t.Fatalf("simulating a prior registration: %v", err)
	}

	if err := pipkg.Build(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("second Build: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(destDir, "mcp.json"))
	if err != nil {
		t.Fatalf("read mcp.json after rebuild: %v", err)
	}
	if string(got) != registered {
		t.Errorf("mcp.json after rebuild = %s, want the registered bytes preserved unchanged:\n%s", got, registered)
	}

	if _, err := pipkg.Check(overlayRoot, registryPath, destDir); err != nil {
		t.Errorf("Check must not report drift for a registered mcp.json, got: %v", err)
	}
}

// TestPipkgCheck_IgnoresMcpJSONBak (C-02 remediation): jsonInstall (the
// writer `longterm-mem register --target pi` uses) leaves an mcp.json.bak
// sibling beside mcp.json on any content-changing register/unregister call.
// Check must exclude it from its content diff exactly as it excludes
// mcp.json itself, or every documented register call permanently drifts.
func TestPipkgCheck_IgnoresMcpJSONBak(t *testing.T) {
	overlayRoot, registryPath := fixtureOverlay(t)
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")

	if err := pipkg.Build(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Simulate the .bak jsonInstall writes on a content-changing register.
	original := `{"mcpServers": {}}` + "\n"
	if err := os.WriteFile(filepath.Join(destDir, "mcp.json.bak"), []byte(original), 0644); err != nil {
		t.Fatalf("simulating jsonInstall's .bak: %v", err)
	}

	if _, err := pipkg.Check(overlayRoot, registryPath, destDir); err != nil {
		t.Errorf("Check must not report drift for a registration-owned mcp.json.bak, got: %v", err)
	}
}

// TestPipkgBuild_PreservesRegisteredMcpJSONBak (C-02 remediation): a
// rebuild must carry an already-registered mcp.json.bak forward the same
// way it already carries mcp.json forward, so the backup sibling survives
// an `engine pipkg build` the same as its primary file does.
func TestPipkgBuild_PreservesRegisteredMcpJSONBak(t *testing.T) {
	overlayRoot, registryPath := fixtureOverlay(t)
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")

	if err := pipkg.Build(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("first Build: %v", err)
	}

	registeredBak := `{"mcpServers": {}}` + "\n"
	if err := os.WriteFile(filepath.Join(destDir, "mcp.json.bak"), []byte(registeredBak), 0644); err != nil {
		t.Fatalf("simulating a prior registration's .bak: %v", err)
	}

	if err := pipkg.Build(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("second Build: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(destDir, "mcp.json.bak"))
	if err != nil {
		t.Fatalf("mcp.json.bak must survive a rebuild: %v", err)
	}
	if string(got) != registeredBak {
		t.Errorf("mcp.json.bak after rebuild = %s, want preserved unchanged:\n%s", got, registeredBak)
	}
}
