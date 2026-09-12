package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pipkg"
)

// PiAdapter is the runtime adapter for the Pi CLI (via gentle-pi). It NEVER
// writes ~/.pi/agent/settings.json or ~/.pi/agent/mcp.json itself — only
// the `pi` CLI does, via the install/remove subprocess calls below.
type PiAdapter struct {
	target       Target
	overlayRoot  string
	registryPath string
	destDir      string
}

// NewPiAdapter constructs the Pi adapter, resolving its build paths from
// OVERLAY_DIR/STATE_DIR (empty when unset — every wired method then
// honestly reports CapabilityUnsupported rather than fabricating success).
func NewPiAdapter() PiAdapter {
	return NewPiAdapterWithPaths(os.Getenv("OVERLAY_DIR"), "", "")
}

// NewPiAdapterWithPaths constructs the Pi adapter with explicit build
// paths. An empty registryPath defaults to "<overlayRoot>/skills.registry.yaml"
// when overlayRoot is set; an empty destDir defaults to
// DefaultPiPackageDir(os.Getenv("STATE_DIR")).
func NewPiAdapterWithPaths(overlayRoot, registryPath, destDir string) PiAdapter {
	if registryPath == "" && overlayRoot != "" {
		registryPath = filepath.Join(overlayRoot, "skills.registry.yaml")
	}
	if destDir == "" {
		destDir = DefaultPiPackageDir(os.Getenv("STATE_DIR"))
	}
	return PiAdapter{target: TargetPi, overlayRoot: overlayRoot, registryPath: registryPath, destDir: destDir}
}

// DefaultPiPackageDir returns "<stateDir>/pi/labdrian-pi", defaulting
// stateDir to "$HOME/.labdrian-overlay" when empty.
func DefaultPiPackageDir(stateDir string) string {
	if stateDir == "" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			stateDir = filepath.Join(home, ".labdrian-overlay")
		}
	}
	return filepath.Join(stateDir, "pi", "labdrian-pi")
}

// piNoDiscoveryFlagsDisclosure is a STATIC note (R-007) — never a runtime-
// detected fact, since there is no API to detect either flag. Pi 0.85.1
// documents the short aliases -ne and -ns.
const piNoDiscoveryFlagsDisclosure = "'pi --no-extensions' disables the before_agent_start contract-gate extension for that session, and 'pi --no-skills' disables skill discovery, for that session only; neither flag's use is detected at runtime (short aliases: -ne and -ns)"

func (a PiAdapter) Target() Target { return a.target }

// Apply mirrors OpenCodeAdapter's Apply/Install identity (a package target
// has no separate "apply without installing" concept).
func (a PiAdapter) Apply() LifecycleResult { return a.Install() }

// Install builds the package then, when `pi` is resolvable, runs
// `pi install <destDir>` with a FIXED argv (never a shell string). Without
// `pi` on PATH it keeps the build-succeeded-with-hint result. Once the
// package install succeeds, it also ensures the Pi Subagents extension
// (R-012) and links the overlay-owned GADU.md agent (R-013/R-014) so GADU
// is dispatchable as a real Pi subagent, not merely a relayed persona.
func (a PiAdapter) Install() LifecycleResult {
	if a.overlayRoot == "" {
		return a.stub(ActionInstall)
	}
	if err := pipkg.Build(a.overlayRoot, a.registryPath, a.destDir); err != nil {
		return NewLifecycleResult(a.target, ActionInstall, CapabilityUnsupported, err.Error(), nil)
	}
	bin, err := resolvePiBinary()
	if err != nil {
		return NewLifecycleResult(a.target, ActionInstall, CapabilityPartial,
			"labdrian-pi package built at "+a.destDir+"; run: pi install "+a.destDir, nil)
	}
	if err := runPiCommand(bin, "install", a.destDir); err != nil {
		return NewLifecycleResult(a.target, ActionInstall, CapabilityPartial,
			"labdrian-pi package built at "+a.destDir+" but `pi install` failed: "+err.Error(), nil)
	}

	notes := []string{a.installGaduSubagent(bin)}
	return NewLifecycleResult(a.target, ActionInstall, CapabilityRestartRequired,
		"ran `pi install "+a.destDir+"`; start a new Pi session to load it. "+strings.Join(notes, " "), nil)
}

// installGaduSubagent ensures the Pi Subagents extension and the
// overlay-owned GADU.md link, returning a disclosure/status note for
// Install's message. It never fails Install as a whole -- an extension or
// link problem is reported inline, honestly, but the package install
// itself already succeeded by the time this runs.
func (a PiAdapter) installGaduSubagent(bin string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "GADU Pi subagent wiring skipped: cannot resolve home directory."
	}

	var parts []string
	parts = append(parts, ensureSubagentsExtension(bin, home))

	if err := linkGaduAgent(home, a.destDir); err != nil {
		parts = append(parts, err.Error()+".")
	} else {
		parts = append(parts, "GADU.md linked at ~/.pi/agent/agents/GADU.md.")
	}
	return strings.Join(parts, " ")
}

func (a PiAdapter) SyncCheck() LifecycleResult {
	if a.overlayRoot == "" {
		return a.stub(ActionSyncCheck)
	}
	report, err := pipkg.Check(a.overlayRoot, a.registryPath, a.destDir)
	if err != nil {
		return NewLifecycleResult(a.target, ActionSyncCheck, CapabilityPartial, err.Error()+" ("+report.Disclosure()+")", nil)
	}
	return NewLifecycleResult(a.target, ActionSyncCheck, CapabilitySupported,
		"labdrian-pi package matches the current manifest ("+report.Disclosure()+")", nil)
}

// Status reports per-entry proof: built, in sync, listed in
// ~/.pi/agent/settings.json, and longterm-mem MCP-registered in mcp.json
// (read-only). All proven -> supported; built but unproven -> partial,
// naming each entry; never built -> unsupported.
func (a PiAdapter) Status() LifecycleResult {
	if !a.piPackageBuilt() {
		return NewLifecycleResult(a.target, ActionStatus, CapabilityUnsupported,
			"labdrian-pi package is not built at "+a.destDir+" (run: labdrian-overlay apply --target pi). "+piNoDiscoveryFlagsDisclosure, nil)
	}

	var problems []string
	if a.overlayRoot == "" {
		problems = append(problems, "in sync (OVERLAY_DIR unset; cannot verify the build matches the current manifest)")
	} else if report, err := pipkg.Check(a.overlayRoot, a.registryPath, a.destDir); err != nil {
		problems = append(problems, "in sync ("+err.Error()+"; "+report.Disclosure()+")")
	}
	if !isPiPackageListed(a.destDir) {
		problems = append(problems, "listed in ~/.pi/agent/settings.json packages (not listed; run: pi install "+a.destDir+")")
	}
	if !isPiMcpRegistered(a.destDir) {
		problems = append(problems, "longterm-mem registered in mcp.json (not registered; run: longterm-mem register --target pi)")
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if !isSubagentsExtensionListed(home) {
			problems = append(problems, "Pi Subagents extension installed (not installed; run: labdrian-overlay apply --target pi)")
		}
		switch gaduLinkState(gaduLinkPath(home), gaduSourcePath(a.destDir)) {
		case gaduLinkCurrent:
			// proven
		case gaduLinkMissing:
			problems = append(problems, "GADU.md linked at ~/.pi/agent/agents/GADU.md (missing; run: labdrian-overlay apply --target pi)")
		case gaduLinkStale:
			problems = append(problems, "GADU.md linked at ~/.pi/agent/agents/GADU.md (stale; run: labdrian-overlay apply --target pi)")
		case gaduLinkConflict:
			problems = append(problems, "GADU.md linked at ~/.pi/agent/agents/GADU.md (conflict: a foreign file already exists there)")
		}
	} else {
		problems = append(problems, "Pi Subagents extension installed (cannot resolve home directory)")
		problems = append(problems, "GADU.md linked at ~/.pi/agent/agents/GADU.md (cannot resolve home directory)")
	}

	if len(problems) == 0 {
		return NewLifecycleResult(a.target, ActionStatus, CapabilitySupported,
			"labdrian-pi package is built, in sync, listed in ~/.pi/agent/settings.json, longterm-mem is registered in its mcp.json, the Pi Subagents extension is installed, and GADU.md is linked. "+piNoDiscoveryFlagsDisclosure, nil)
	}
	return NewLifecycleResult(a.target, ActionStatus, CapabilityPartial,
		"labdrian-pi status is unproven: "+strings.Join(problems, "; ")+". "+piNoDiscoveryFlagsDisclosure, problems)
}

func (a PiAdapter) Update() LifecycleResult   { return a.build(ActionUpdate) }
func (a PiAdapter) Rollback() LifecycleResult { return a.build(ActionRollback) }

// Uninstall removes the overlay-owned GADU.md link first (R-016 — ownership
// proven by Readlink equality; a foreign entry at the same path is left
// untouched and reported, never removed), then runs `pi remove <destDir>`
// (A2 — NOT `pi uninstall`), then removes the built package directory.
// `pi remove` owns settings.json cleanup; this adapter never opens it or
// mcp.json directly, and never uninstalls the Subagents extension package.
func (a PiAdapter) Uninstall() LifecycleResult {
	if !a.piPackageBuilt() {
		return NewLifecycleResult(a.target, ActionUninstall, CapabilityUnsupported,
			"labdrian-pi package is not built at "+a.destDir+"; nothing to uninstall", nil)
	}

	var linkNote string
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if err := unlinkGaduAgent(home, a.destDir); err != nil {
			linkNote = " " + err.Error() + "."
		} else {
			linkNote = " GADU.md link removed."
		}
	}

	bin, err := resolvePiBinary()
	if err != nil {
		return NewLifecycleResult(a.target, ActionUninstall, CapabilityPartial,
			"pi CLI not found on PATH; cannot run `pi remove "+a.destDir+"` ("+err.Error()+")."+linkNote, nil)
	}
	if err := runPiCommand(bin, "remove", a.destDir); err != nil {
		return NewLifecycleResult(a.target, ActionUninstall, CapabilityPartial,
			"`pi remove "+a.destDir+"` failed: "+err.Error()+"."+linkNote, nil)
	}
	if err := os.RemoveAll(a.destDir); err != nil {
		return NewLifecycleResult(a.target, ActionUninstall, CapabilityPartial,
			"ran `pi remove "+a.destDir+"` but could not remove the package directory: "+err.Error()+"."+linkNote, nil)
	}
	return NewLifecycleResult(a.target, ActionUninstall, CapabilitySupported,
		"removed via `pi remove "+a.destDir+"`; package directory deleted."+linkNote, nil)
}

// piPackageBuilt reports whether destDir holds a built package
// (package.json present) — the proof Status/Uninstall gate on before ever
// resolving or invoking the `pi` CLI.
func (a PiAdapter) piPackageBuilt() bool {
	_, err := os.Stat(filepath.Join(a.destDir, "package.json"))
	return err == nil
}

// build runs pipkg.Build for Update/Rollback: unsupported without an
// overlayRoot, partial (never fabricated supported) either way otherwise —
// a rebuild alone cannot prove Status's per-entry proof.
func (a PiAdapter) build(action Action) LifecycleResult {
	if a.overlayRoot == "" {
		return a.stub(action)
	}
	if err := pipkg.Build(a.overlayRoot, a.registryPath, a.destDir); err != nil {
		return NewLifecycleResult(a.target, action, CapabilityPartial, err.Error(), nil)
	}
	return NewLifecycleResult(a.target, action, CapabilityPartial,
		"labdrian-pi package rebuilt at "+a.destDir+"; run: pi install "+a.destDir, nil)
}

// stub reports an honest CapabilityUnsupported when the action cannot run
// without an overlayRoot (OVERLAY_DIR unset).
func (a PiAdapter) stub(action Action) LifecycleResult {
	msg := "pi package delivery (pipkg build/install) cannot run without OVERLAY_DIR set"
	if action == ActionRollback {
		msg = "pi lifecycle rollback cannot rebuild without OVERLAY_DIR set"
	}
	return NewLifecycleResult(a.target, action, CapabilityUnsupported, msg, nil)
}

// resolvePiBinary returns the pi CLI to invoke. LABDRIAN_PI_BIN overrides
// discovery (tests use it, so they never touch a real `pi` a developer
// machine may have on PATH); production resolves via exec.LookPath("pi").
func resolvePiBinary() (string, error) {
	if override := strings.TrimSpace(os.Getenv("LABDRIAN_PI_BIN")); override != "" {
		return override, nil
	}
	return exec.LookPath("pi")
}

// runPiCommand execs bin with a FIXED argv (verb, path), never a shell
// string, so no path content is ever shell-interpreted.
func runPiCommand(bin, verb, path string) error {
	out, err := exec.Command(bin, verb, path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w (output: %s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// isPiPackageListed reports whether destDir is present in
// ~/.pi/agent/settings.json's "packages" array (read-only probe).
func isPiPackageListed(destDir string) bool {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return false
	}
	raw, err := os.ReadFile(filepath.Join(home, ".pi", "agent", "settings.json"))
	if err != nil {
		return false
	}
	var settings struct {
		Packages []string `json:"packages"`
	}
	if json.Unmarshal(raw, &settings) != nil {
		return false
	}
	want := filepath.Clean(destDir)
	settingsDir := filepath.Join(home, ".pi", "agent")
	for _, p := range settings.Packages {
		// Pi resolves relative package entries against the settings file's
		// directory (packages.md); `pi install <abs>` records them that way.
		if !filepath.IsAbs(p) {
			p = filepath.Join(settingsDir, p)
		}
		if filepath.Clean(p) == want {
			return true
		}
	}
	return false
}

// subagentsExtensionPackage is the fixed argv target R-012 installs when
// neither accepted package is already present.
const subagentsExtensionPackage = "npm:pi-subagents-j0k3r"

// subagentsSkipEnv opts out of the extension probe/install entirely (R-012)
// -- for environments that manage the Subagents extension separately.
const subagentsSkipEnv = "LABDRIAN_PI_SKIP_SUBAGENTS"

// subagentsAcceptedPackagePrefixes are the two package names gentle-pi's
// Subagents extension ships under (D11); either satisfies R-012.
var subagentsAcceptedPackagePrefixes = []string{"npm:pi-subagents-j0k3r", "npm:pi-subagents"}

// hasAcceptedSubagentsPrefix reports whether entry is one of the accepted
// package names, with or without an "@version" suffix.
func hasAcceptedSubagentsPrefix(entry string) bool {
	for _, prefix := range subagentsAcceptedPackagePrefixes {
		if entry == prefix || strings.HasPrefix(entry, prefix+"@") {
			return true
		}
	}
	return false
}

// isSubagentsExtensionListed reports whether ~/.pi/agent/settings.json's
// "packages" array already lists either accepted Subagents extension name
// (read-only probe; mirrors isPiPackageListed's parse, without the
// destDir-relative resolution a package path needs).
func isSubagentsExtensionListed(home string) bool {
	raw, err := os.ReadFile(filepath.Join(home, ".pi", "agent", "settings.json"))
	if err != nil {
		return false
	}
	var settings struct {
		Packages []string `json:"packages"`
	}
	if json.Unmarshal(raw, &settings) != nil {
		return false
	}
	for _, p := range settings.Packages {
		if hasAcceptedSubagentsPrefix(p) {
			return true
		}
	}
	return false
}

// ensureSubagentsExtension installs pi-subagents-j0k3r via a FIXED argv
// (verb, package) when neither accepted package name is already listed
// (R-012). Always returns a human-readable disclosure/status note; never
// fails Install as a whole on an install error -- the caller folds the
// note into its own message instead.
func ensureSubagentsExtension(bin, home string) string {
	if os.Getenv(subagentsSkipEnv) == "1" {
		return "Pi Subagents extension check skipped (" + subagentsSkipEnv + "=1)."
	}
	if isSubagentsExtensionListed(home) {
		return "Pi Subagents extension already installed."
	}
	disclosure := "installing third-party Pi extension pi-subagents-j0k3r (npm) required for GADU dispatch."
	if err := runPiCommand(bin, "install", subagentsExtensionPackage); err != nil {
		return disclosure + " `pi install " + subagentsExtensionPackage + "` failed: " + err.Error() + "."
	}
	return disclosure + " Ran `pi install " + subagentsExtensionPackage + "`."
}

// gaduLinkPath returns the overlay-owned GADU agent link location.
func gaduLinkPath(home string) string {
	return filepath.Join(home, ".pi", "agent", "agents", "GADU.md")
}

// gaduSourcePath returns the package's own generated agents/GADU.md -- the
// STABLE symlink target across `swap` rebuilds (only the inode changes,
// design D10).
func gaduSourcePath(destDir string) string {
	return filepath.Join(destDir, "agents", "GADU.md")
}

// gaduLink is gaduLinkState's result: exactly one of missing/current/
// stale/conflict (D13 -- collapsing these into a single boolean was
// rejected as it hides which failure mode is present).
type gaduLink string

const (
	gaduLinkMissing  gaduLink = "missing"
	gaduLinkCurrent  gaduLink = "current"
	gaduLinkStale    gaduLink = "stale"
	gaduLinkConflict gaduLink = "conflict"
)

// gaduLinkState reports linkPath's ownership/state relative to
// expectedTarget. Ownership is proven ONLY by os.Readlink equality with
// expectedTarget, never by file contents or a side record: missing (no
// entry), current (our symlink, target resolves), stale (our symlink, but
// its target no longer exists -- e.g. destDir was rebuilt from scratch),
// or conflict (a pre-existing regular file, or a symlink pointing
// elsewhere) -- a conflict is reported and left untouched, never
// overwritten.
func gaduLinkState(linkPath, expectedTarget string) gaduLink {
	info, err := os.Lstat(linkPath)
	if err != nil {
		return gaduLinkMissing
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return gaduLinkConflict
	}
	target, err := os.Readlink(linkPath)
	if err != nil || filepath.Clean(target) != filepath.Clean(expectedTarget) {
		return gaduLinkConflict
	}
	if _, err := os.Stat(expectedTarget); err != nil {
		return gaduLinkStale
	}
	return gaduLinkCurrent
}

// GaduLinkStateForTest exposes gaduLinkState to engine/runtime's external
// test package (runtime_test), which cannot see the unexported gaduLink
// type or constants. Test-only surface.
func GaduLinkStateForTest(linkPath, expectedTarget string) string {
	return string(gaduLinkState(linkPath, expectedTarget))
}

// validateGaduFrontmatter mirrors just enough of pi-subagents-j0k3r's own
// frontmatter parser (config.ts parseFrontmatterWithIssues) to catch the
// one failure mode that silently drops the subagent at dispatch time: a
// `tools` field declared BOTH as an inline scalar and a YAML list. `name`
// and `description` are required (the extension falls back to the
// filename/a generic description otherwise, which is not what GADU wants);
// `model` is optional and unconstrained (R-014).
func validateGaduFrontmatter(content string) error {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return fmt.Errorf("gadu frontmatter: missing opening --- delimiter")
	}
	var name, description string
	var toolsInline, toolsList, closed bool
	var currentKey string
	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			closed = true
			break
		}
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "-") {
			if currentKey == "tools" {
				toolsList = true
			}
			continue
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		currentKey = key
		switch key {
		case "name":
			name = value
		case "description":
			description = value
		case "tools":
			if value != "" {
				toolsInline = true
			}
		}
	}
	if !closed {
		return fmt.Errorf("gadu frontmatter: missing closing --- delimiter")
	}
	if name == "" {
		return fmt.Errorf("gadu frontmatter: missing required 'name'")
	}
	if description == "" {
		return fmt.Errorf("gadu frontmatter: missing required 'description'")
	}
	if toolsInline && toolsList {
		return fmt.Errorf("gadu frontmatter: 'tools' declared both as an inline scalar and a YAML list; choose one")
	}
	return nil
}

// linkGaduAgent verifies the package's agents/GADU.md frontmatter (R-014),
// then symlinks it at ~/.pi/agent/agents/GADU.md (R-013). A pre-existing
// regular file or a symlink pointing elsewhere is a conflict, reported and
// left untouched -- never overwritten. A stale link (ours, but its target
// no longer exists) is recreated.
func linkGaduAgent(home, destDir string) error {
	target := gaduSourcePath(destDir)
	content, err := os.ReadFile(target)
	if err != nil {
		return fmt.Errorf("gadu link: package agents/GADU.md not found at %s: %w", target, err)
	}
	if err := validateGaduFrontmatter(string(content)); err != nil {
		return err
	}

	linkPath := gaduLinkPath(home)
	switch gaduLinkState(linkPath, target) {
	case gaduLinkCurrent:
		return nil
	case gaduLinkConflict:
		return fmt.Errorf("gadu link: %s exists and is not the overlay-owned symlink; leaving it untouched", linkPath)
	case gaduLinkStale:
		if err := os.Remove(linkPath); err != nil {
			return fmt.Errorf("gadu link: removing stale link: %w", err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(linkPath), 0755); err != nil {
		return fmt.Errorf("gadu link: creating agents dir: %w", err)
	}
	if err := os.Symlink(target, linkPath); err != nil {
		return fmt.Errorf("gadu link: creating symlink: %w", err)
	}
	return nil
}

// unlinkGaduAgent removes ~/.pi/agent/agents/GADU.md ONLY when it is the
// overlay-owned symlink (Readlink equality against destDir's agents/
// GADU.md). Never touches a conflicting entry or any other file in that
// directory, and never removes the Subagents extension package itself
// (R-016).
func unlinkGaduAgent(home, destDir string) error {
	linkPath := gaduLinkPath(home)
	target := gaduSourcePath(destDir)
	switch gaduLinkState(linkPath, target) {
	case gaduLinkMissing:
		return nil
	case gaduLinkConflict:
		return fmt.Errorf("gadu link: %s is not the overlay-owned symlink; leaving it untouched", linkPath)
	default: // current or stale -- still ours
		return os.Remove(linkPath)
	}
}

// isPiMcpRegistered reports whether destDir/mcp.json exists, parses, and
// carries an mcpServers.longterm-mem entry -- the proof `longterm-mem
// register --target pi` ran (read-only probe; never written here).
func isPiMcpRegistered(destDir string) bool {
	raw, err := os.ReadFile(filepath.Join(destDir, "mcp.json"))
	if err != nil {
		return false
	}
	var mcp struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if json.Unmarshal(raw, &mcp) != nil {
		return false
	}
	_, ok := mcp.MCPServers["longterm-mem"]
	return ok
}
