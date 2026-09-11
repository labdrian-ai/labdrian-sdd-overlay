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
// detected fact, since there is no API to detect either flag, and no "-ns"
// alias exists for either.
const piNoDiscoveryFlagsDisclosure = "'pi --no-extensions' disables the before_agent_start contract-gate extension for that session, and 'pi --no-skills' disables skill discovery, for that session only; neither flag's use is detected at runtime, and there is no short alias for either flag"

func (a PiAdapter) Target() Target { return a.target }

// Apply mirrors OpenCodeAdapter's Apply/Install identity (a package target
// has no separate "apply without installing" concept).
func (a PiAdapter) Apply() LifecycleResult { return a.Install() }

// Install builds the package then, when `pi` is resolvable, runs
// `pi install <destDir>` with a FIXED argv (never a shell string). Without
// `pi` on PATH it keeps the build-succeeded-with-hint result.
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
	return NewLifecycleResult(a.target, ActionInstall, CapabilityRestartRequired,
		"ran `pi install "+a.destDir+"`; start a new Pi session to load it", nil)
}

func (a PiAdapter) SyncCheck() LifecycleResult {
	if a.overlayRoot == "" {
		return a.stub(ActionSyncCheck)
	}
	if err := pipkg.Check(a.overlayRoot, a.registryPath, a.destDir); err != nil {
		return NewLifecycleResult(a.target, ActionSyncCheck, CapabilityPartial, err.Error(), nil)
	}
	return NewLifecycleResult(a.target, ActionSyncCheck, CapabilitySupported, "labdrian-pi package matches the current manifest", nil)
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
	} else if err := pipkg.Check(a.overlayRoot, a.registryPath, a.destDir); err != nil {
		problems = append(problems, "in sync ("+err.Error()+")")
	}
	if !isPiPackageListed(a.destDir) {
		problems = append(problems, "listed in ~/.pi/agent/settings.json packages (not listed; run: pi install "+a.destDir+")")
	}
	if !isPiMcpRegistered(a.destDir) {
		problems = append(problems, "longterm-mem registered in mcp.json (not registered; run: longterm-mem register --target pi)")
	}

	if len(problems) == 0 {
		return NewLifecycleResult(a.target, ActionStatus, CapabilitySupported,
			"labdrian-pi package is built, in sync, and listed in ~/.pi/agent/settings.json. "+piNoDiscoveryFlagsDisclosure, nil)
	}
	return NewLifecycleResult(a.target, ActionStatus, CapabilityPartial,
		"labdrian-pi status is unproven: "+strings.Join(problems, "; ")+". "+piNoDiscoveryFlagsDisclosure, problems)
}

func (a PiAdapter) Update() LifecycleResult   { return a.build(ActionUpdate) }
func (a PiAdapter) Rollback() LifecycleResult { return a.build(ActionRollback) }

// Uninstall runs `pi remove <destDir>` (A2 — NOT `pi uninstall`), then
// removes the built package directory. `pi remove` owns settings.json
// cleanup; this adapter never opens it or mcp.json directly.
func (a PiAdapter) Uninstall() LifecycleResult {
	if !a.piPackageBuilt() {
		return NewLifecycleResult(a.target, ActionUninstall, CapabilityUnsupported,
			"labdrian-pi package is not built at "+a.destDir+"; nothing to uninstall", nil)
	}
	bin, err := resolvePiBinary()
	if err != nil {
		return NewLifecycleResult(a.target, ActionUninstall, CapabilityPartial,
			"pi CLI not found on PATH; cannot run `pi remove "+a.destDir+"` ("+err.Error()+")", nil)
	}
	if err := runPiCommand(bin, "remove", a.destDir); err != nil {
		return NewLifecycleResult(a.target, ActionUninstall, CapabilityPartial,
			"`pi remove "+a.destDir+"` failed: "+err.Error(), nil)
	}
	if err := os.RemoveAll(a.destDir); err != nil {
		return NewLifecycleResult(a.target, ActionUninstall, CapabilityPartial,
			"ran `pi remove "+a.destDir+"` but could not remove the package directory: "+err.Error(), nil)
	}
	return NewLifecycleResult(a.target, ActionUninstall, CapabilitySupported,
		"removed via `pi remove "+a.destDir+"`; package directory deleted", nil)
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
	for _, p := range settings.Packages {
		if p == destDir {
			return true
		}
	}
	return false
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
