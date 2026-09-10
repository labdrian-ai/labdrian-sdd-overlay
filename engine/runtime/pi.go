package runtime

// PiAdapter is the runtime adapter for the Pi CLI (via gentle-pi).
//
// This is the pi-target-plumbing slice: only Target() is wired to a real
// value. Every lifecycle method returns an honest CapabilityUnsupported
// stub — package build (pipkg), install/apply dispatch, and status/
// uninstall logic land in later slices (pi-package-build, pi-lifecycle).
// Returning unsupported here (never supported or partial) keeps this slice
// from claiming proof it cannot yet produce.
type PiAdapter struct {
	target Target
}

// NewPiAdapter constructs the Pi adapter skeleton.
func NewPiAdapter() PiAdapter {
	return PiAdapter{target: TargetPi}
}

func (a PiAdapter) Target() Target             { return a.target }
func (a PiAdapter) Apply() LifecycleResult     { return a.stub(ActionApply) }
func (a PiAdapter) Install() LifecycleResult   { return a.stub(ActionInstall) }
func (a PiAdapter) Status() LifecycleResult    { return a.stub(ActionStatus) }
func (a PiAdapter) SyncCheck() LifecycleResult { return a.stub(ActionSyncCheck) }
func (a PiAdapter) Update() LifecycleResult    { return a.stub(ActionUpdate) }
func (a PiAdapter) Rollback() LifecycleResult  { return a.stub(ActionRollback) }
func (a PiAdapter) Uninstall() LifecycleResult { return a.stub(ActionUninstall) }

func (a PiAdapter) stub(action Action) LifecycleResult {
	return NewLifecycleResult(a.target, action, CapabilityUnsupported,
		"pi package delivery (pipkg build/install) is not implemented yet; scheduled for the pi-package-build PR slice", nil)
}
