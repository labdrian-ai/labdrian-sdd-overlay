package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

// PiAdapter is the runtime adapter for the Pi CLI (via gentle-pi). It NEVER
// writes ~/.pi/agent/settings.json or ~/.pi/agent/mcp.json itself — only
// the `pi` CLI does, via the install/remove subprocess calls below.
type PiAdapter struct {
	target       core.Target
	commands     CommandRunner
	packages     PackageBuilder
	options      PiOptions
	home         string
	overlayRoot  string
	registryPath string
	destDir      string
}

// PiPaths are the places the Pi adapter works in. The composition root resolves them (see
// Config); the adapter reads no environment.
type PiPaths struct {
	// Home is the home directory whose ~/.pi/agent the `pi` CLI keeps its settings in. When it
	// is empty the adapter reports what it needs it for as unproven, never a guessed location.
	Home string
	// OverlayRoot is the overlay checkout the package is built from. When it is empty every
	// method that builds honestly reports CapabilityUnsupported rather than fabricating success.
	OverlayRoot string
	// RegistryPath is the skills registry the package is built from; empty means
	// "<OverlayRoot>/skills.registry.yaml" when OverlayRoot is set.
	RegistryPath string
	// DestDir is where the package is built and installed from.
	DestDir string
}

// NewPiAdapter constructs the Pi adapter over its ports, paths and options. It starts no process
// and builds no package but through the ports, and reads no environment.
func NewPiAdapter(ports PiPorts, paths PiPaths, options PiOptions) PiAdapter {
	registryPath := paths.RegistryPath
	if registryPath == "" && paths.OverlayRoot != "" {
		registryPath = filepath.Join(paths.OverlayRoot, "skills.registry.yaml")
	}
	return PiAdapter{target: core.TargetPi, commands: ports.Commands, packages: ports.Packages, options: options, home: paths.Home, overlayRoot: paths.OverlayRoot, registryPath: registryPath, destDir: paths.DestDir}
}

// piNoDiscoveryFlagsDisclosure is a STATIC note (R-007) — never a runtime-
// detected fact, since there is no API to detect either flag. Pi 0.85.1
// documents the short aliases -ne and -ns.
const piNoDiscoveryFlagsDisclosure = "'pi --no-extensions' disables the before_agent_start contract-gate extension for that session, and 'pi --no-skills' disables skill discovery, for that session only; neither flag's use is detected at runtime (short aliases: -ne and -ns)"

func (a PiAdapter) Target() core.Target { return a.target }

// Apply mirrors OpenCodeAdapter's Apply/Install identity (a package target
// has no separate "apply without installing" concept).
func (a PiAdapter) Apply() core.LifecycleResult { return a.Install() }

// Install builds the package then, when `pi` is resolvable, runs
// `pi install <destDir>` with a FIXED argv (never a shell string). Without
// `pi` on PATH it keeps the build-succeeded-with-hint result. Once the
// package install succeeds, it also ensures the Pi Subagents extension
// (R-012) and links the overlay-owned GADU.md agent (R-013/R-014) so GADU
// is dispatchable as a real Pi subagent, not merely a relayed persona.
func (a PiAdapter) Install() core.LifecycleResult {
	if a.overlayRoot == "" {
		return a.stub(core.ActionInstall)
	}
	if err := a.packages.Build(a.overlayRoot, a.registryPath, a.destDir); err != nil {
		return core.NewLifecycleResult(a.target, core.ActionInstall, core.CapabilityUnsupported, err.Error(), nil)
	}
	bin, err := a.commands.LookPath("pi")
	if err != nil {
		return core.NewLifecycleResult(a.target, core.ActionInstall, core.CapabilityPartial,
			"labdrian-pi package built at "+a.destDir+"; run: pi install "+a.destDir, nil)
	}
	if err := a.runPi(bin, "install", a.destDir); err != nil {
		return core.NewLifecycleResult(a.target, core.ActionInstall, core.CapabilityPartial,
			"labdrian-pi package built at "+a.destDir+" but `pi install` failed: "+err.Error(), nil)
	}

	notes := []string{a.installGaduSubagent(bin)}
	return core.NewLifecycleResult(a.target, core.ActionInstall, core.CapabilityRestartRequired,
		"ran `pi install "+a.destDir+"`; start a new Pi session to load it. "+strings.Join(notes, " "), nil)
}

// installGaduSubagent ensures the Pi Subagents extension and the
// overlay-owned GADU.md link, returning a disclosure/status note for
// Install's message. It never fails Install as a whole -- an extension or
// link problem is reported inline, honestly, but the package install
// itself already succeeded by the time this runs.
func (a PiAdapter) installGaduSubagent(bin string) string {
	home := a.home
	if home == "" {
		return "GADU Pi subagent wiring skipped: cannot resolve home directory."
	}

	var parts []string
	parts = append(parts, a.ensureSubagentRunner(bin, home))

	if err := linkGaduAgent(home, a.destDir); err != nil {
		parts = append(parts, err.Error()+".")
	} else {
		parts = append(parts, "GADU.md linked at ~/.pi/agent/agents/GADU.md.")
	}
	return strings.Join(parts, " ")
}

func (a PiAdapter) SyncCheck() core.LifecycleResult {
	if a.overlayRoot == "" {
		return a.stub(core.ActionSyncCheck)
	}
	disclosure, err := a.packages.Check(a.overlayRoot, a.registryPath, a.destDir)
	if err != nil {
		return core.NewLifecycleResult(a.target, core.ActionSyncCheck, core.CapabilityPartial, err.Error()+" ("+disclosure+")", nil)
	}
	return core.NewLifecycleResult(a.target, core.ActionSyncCheck, core.CapabilitySupported,
		"labdrian-pi package matches the current manifest ("+disclosure+")", nil)
}

// Status reports per-entry proof: built, in sync, listed in
// ~/.pi/agent/settings.json, and longterm-mem MCP-registered in mcp.json
// (read-only). All proven -> supported; built but unproven -> partial,
// naming each entry; never built -> unsupported.
func (a PiAdapter) Status() core.LifecycleResult {
	if !a.piPackageBuilt() {
		return core.NewLifecycleResult(a.target, core.ActionStatus, core.CapabilityUnsupported,
			"labdrian-pi package is not built at "+a.destDir+" (run: labdrian-overlay apply --target pi). "+piNoDiscoveryFlagsDisclosure, nil)
	}

	var problems []string
	if a.overlayRoot == "" {
		problems = append(problems, "in sync (OVERLAY_DIR unset; cannot verify the build matches the current manifest)")
	} else if disclosure, err := a.packages.Check(a.overlayRoot, a.registryPath, a.destDir); err != nil {
		problems = append(problems, "in sync ("+err.Error()+"; "+disclosure+")")
	}
	settings := readPiSettings(a.home)
	if !settings.listsPackage(a.destDir) {
		problems = append(problems, "listed in ~/.pi/agent/settings.json packages (not listed; run: pi install "+a.destDir+")")
	}
	if !isPiMcpRegistered(a.destDir) {
		problems = append(problems, "longterm-mem registered in mcp.json (not registered; run: longterm-mem register --target pi)")
	}
	if home := a.home; home != "" {
		switch settings.subagentRunner() {
		case subagentRunnerNative, subagentRunnerLegacy:
			// proven
		case subagentRunnerConflict:
			problems = append(problems, "subagent runner (gentle-pi native subagents (>= 2.6.0) AND the pi-subagents extension are both installed; the obsolete extension conflicts with gentle-pi's native subagent tools and must be removed: pi remove npm:pi-subagents-j0k3r)")
		case subagentRunnerAbsent:
			problems = append(problems, "subagent runner not proven (neither gentle-pi native subagents (>= 2.6.0) nor the legacy Pi Subagents extension is installed; run: labdrian-overlay apply --target pi)")
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
		problems = append(problems, "subagent runner not proven (cannot resolve home directory)")
		problems = append(problems, "GADU.md linked at ~/.pi/agent/agents/GADU.md (cannot resolve home directory)")
	}

	if len(problems) == 0 {
		return core.NewLifecycleResult(a.target, core.ActionStatus, core.CapabilitySupported,
			"labdrian-pi package is built, in sync, listed in ~/.pi/agent/settings.json, longterm-mem is registered in its mcp.json, a subagent runner (gentle-pi native subagents or the legacy Pi Subagents extension) is available, and GADU.md is linked. "+piNoDiscoveryFlagsDisclosure, nil)
	}
	// The message names each unproven entry. They are not also passed as reasons: the line that
	// prints a result would tell them a second time after the message.
	return core.NewLifecycleResult(a.target, core.ActionStatus, core.CapabilityPartial,
		"labdrian-pi status is unproven: "+strings.Join(problems, "; ")+". "+piNoDiscoveryFlagsDisclosure, nil)
}

func (a PiAdapter) Update() core.LifecycleResult   { return a.build(core.ActionUpdate) }
func (a PiAdapter) Rollback() core.LifecycleResult { return a.build(core.ActionRollback) }

// Uninstall removes the overlay-owned GADU.md link first (R-016 — ownership
// proven by Readlink equality; a foreign entry at the same path is left
// untouched and reported, never removed), then runs `pi remove <destDir>`
// (A2 — NOT `pi uninstall`), then removes the built package directory.
// `pi remove` owns settings.json cleanup; this adapter never opens it or
// mcp.json directly, and never uninstalls the Subagents extension package.
func (a PiAdapter) Uninstall() core.LifecycleResult {
	if !a.piPackageBuilt() {
		return core.NewLifecycleResult(a.target, core.ActionUninstall, core.CapabilityUnsupported,
			"labdrian-pi package is not built at "+a.destDir+"; nothing to uninstall", nil)
	}

	var linkNote string
	if home := a.home; home != "" {
		if err := unlinkGaduAgent(home, a.destDir); err != nil {
			linkNote = " " + err.Error() + "."
		} else {
			linkNote = " GADU.md link removed."
		}
	}

	bin, err := a.commands.LookPath("pi")
	if err != nil {
		return core.NewLifecycleResult(a.target, core.ActionUninstall, core.CapabilityPartial,
			"pi CLI not found on PATH; cannot run `pi remove "+a.destDir+"` ("+err.Error()+")."+linkNote, nil)
	}
	if err := a.runPi(bin, "remove", a.destDir); err != nil {
		return core.NewLifecycleResult(a.target, core.ActionUninstall, core.CapabilityPartial,
			"`pi remove "+a.destDir+"` failed: "+err.Error()+"."+linkNote, nil)
	}
	if err := os.RemoveAll(a.destDir); err != nil {
		return core.NewLifecycleResult(a.target, core.ActionUninstall, core.CapabilityPartial,
			"ran `pi remove "+a.destDir+"` but could not remove the package directory: "+err.Error()+"."+linkNote, nil)
	}
	return core.NewLifecycleResult(a.target, core.ActionUninstall, core.CapabilitySupported,
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
func (a PiAdapter) build(action core.Action) core.LifecycleResult {
	if a.overlayRoot == "" {
		return a.stub(action)
	}
	if err := a.packages.Build(a.overlayRoot, a.registryPath, a.destDir); err != nil {
		return core.NewLifecycleResult(a.target, action, core.CapabilityPartial, err.Error(), nil)
	}
	return core.NewLifecycleResult(a.target, action, core.CapabilityPartial,
		"labdrian-pi package rebuilt at "+a.destDir+"; run: pi install "+a.destDir, nil)
}

// stub reports an honest CapabilityUnsupported when the action cannot run
// without an overlayRoot (OVERLAY_DIR unset).
func (a PiAdapter) stub(action core.Action) core.LifecycleResult {
	msg := "pi package delivery (pipkg build/install) cannot run without OVERLAY_DIR set"
	if action == core.ActionRollback {
		msg = "pi lifecycle rollback cannot rebuild without OVERLAY_DIR set"
	}
	return core.NewLifecycleResult(a.target, action, core.CapabilityUnsupported, msg, nil)
}

// runPi runs the pi CLI at bin with a FIXED argv (verb, path), never a shell string, so no path
// content is ever shell-interpreted, under the deadline of the options. A failure carries what
// the command printed.
func (a PiAdapter) runPi(bin, verb, path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), a.options.commandTimeout())
	defer cancel()
	out, err := a.commands.Run(ctx, bin, verb, path)
	if err != nil {
		return fmt.Errorf("%w (output: %s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}
