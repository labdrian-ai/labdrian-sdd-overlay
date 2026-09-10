package runtime

import (
	"os"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pipkg"
)

// PiAdapter is the runtime adapter for the Pi CLI (via gentle-pi).
//
// pi-target-plumbing wired only Target(). pi-package-build (this slice)
// wires Apply/Install/SyncCheck to engine/pipkg's Build/Check, resolving
// overlayRoot/registryPath/destDir from OVERLAY_DIR/STATE_DIR when
// constructed via NewPiAdapter() (the zero-arg path NewFoundationAdapter
// uses). Status/Update/Rollback/Uninstall remain honest
// CapabilityUnsupported stubs — those land in pi-lifecycle.
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

func (a PiAdapter) Target() Target { return a.target }

func (a PiAdapter) Apply() LifecycleResult   { return a.build(ActionApply) }
func (a PiAdapter) Install() LifecycleResult { return a.build(ActionInstall) }

func (a PiAdapter) SyncCheck() LifecycleResult {
	if a.overlayRoot == "" {
		return a.stub(ActionSyncCheck)
	}
	if err := pipkg.Check(a.overlayRoot, a.registryPath, a.destDir); err != nil {
		return NewLifecycleResult(a.target, ActionSyncCheck, CapabilityPartial, err.Error(), nil)
	}
	return NewLifecycleResult(a.target, ActionSyncCheck, CapabilitySupported, "labdrian-pi package matches the current manifest", nil)
}

func (a PiAdapter) Status() LifecycleResult    { return a.stub(ActionStatus) }
func (a PiAdapter) Update() LifecycleResult    { return a.stub(ActionUpdate) }
func (a PiAdapter) Rollback() LifecycleResult  { return a.stub(ActionRollback) }
func (a PiAdapter) Uninstall() LifecycleResult { return a.stub(ActionUninstall) }

// build runs pipkg.Build for Apply/Install. Honestly unsupported without an
// overlayRoot (e.g. OVERLAY_DIR unset); partial (never fabricated supported)
// on a build error, naming it; partial with the install hint on success —
// this slice builds the package but never runs `pi install` itself.
func (a PiAdapter) build(action Action) LifecycleResult {
	if a.overlayRoot == "" {
		return a.stub(action)
	}
	if err := pipkg.Build(a.overlayRoot, a.registryPath, a.destDir); err != nil {
		return NewLifecycleResult(a.target, action, CapabilityPartial, err.Error(), nil)
	}
	return NewLifecycleResult(a.target, action, CapabilityPartial,
		"labdrian-pi package built at "+a.destDir+"; run: pi install "+a.destDir, nil)
}

// stub reports an honest CapabilityUnsupported for the given action, with a
// message naming the SLICE THAT ACTUALLY OWNS IT (R2-misleading-stub-
// schedule) — a single shared "handled by pi-package-build" message for
// every action was wrong for Uninstall/Rollback: there is no lifecycle
// (undo/removal) logic to schedule into pi-package-build, that is
// pi-lifecycle's job. Apply/Install/Status/SyncCheck/Update are all package
// DELIVERY concerns (build, deploy, drift-check, refresh) and do land in
// pi-package-build.
func (a PiAdapter) stub(action Action) LifecycleResult {
	return NewLifecycleResult(a.target, action, CapabilityUnsupported, a.stubMessage(action), nil)
}

func (a PiAdapter) stubMessage(action Action) string {
	switch action {
	case ActionUninstall, ActionRollback:
		return "pi lifecycle (uninstall/rollback) is not implemented yet; scheduled for the pi-lifecycle PR slice"
	default:
		return "pi package delivery (pipkg build/install) is not implemented yet; scheduled for the pi-package-build PR slice"
	}
}
