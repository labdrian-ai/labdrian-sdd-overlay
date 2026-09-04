package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// TargetLongtermMem is the pseudo-target used for the longterm-mem
// component's aggregate LifecycleResult. It is intentionally NOT part of
// ParseTarget/ExpandTarget's domain: --target selects one of
// claude/opencode/codex/all for the runtime-parity adapters, while
// --component longterm-mem is an orthogonal CLI axis (D4) whose single
// adapter call always spans all three runtimes internally.
const TargetLongtermMem Target = "longterm-mem"

// The status matrix this adapter feeds -- LongtermMemComponentState,
// EvaluateLongtermMemComponentStatus and the named partial reasons -- lives
// in longtermmem_status.go: it is pure decision logic with no I/O, kept
// apart from the observation and persistence this file performs.

const longtermMemRegistrationFile = "longterm-mem-registration.json"
const longtermMemManagedBy = "labdrian-sdd-overlay"

// longtermMemRegistration is the engine-owned install-state record for the
// longterm-mem component (D4: "engine record"). It is distinct from, and
// unaware of, longterm-mem's own module-owned install-state.json (D9) — the
// two are independent records of independent writers.
type longtermMemRegistration struct {
	ManagedBy string                             `json:"managed_by"`
	Targets   map[string]longtermMemTargetRecord `json:"targets"`
}

type longtermMemTargetRecord struct {
	Fingerprint  string `json:"fingerprint"`
	EntryPresent bool   `json:"entry_present"`
}

// LongtermMemAdapter is the runtime lifecycle adapter for the longterm-mem
// component (R-014). Unlike the per-runtime parity adapters, one Install/
// Status/Uninstall call spans all three runtimes (claude, opencode, codex)
// and reports one aggregate LifecycleResult whose Reasons carry the
// per-runtime breakdown. Update/Rollback are refused outright (10a.4).
//
// Fields are exported so tests can point every path at a temp fixture
// without a wide constructor; production callers use
// NewLongtermMemAdapter, which fills in the real defaults.
type LongtermMemAdapter struct {
	StateDir           string
	BinaryPath         string
	ClaudeConfigPath   string
	OpenCodeConfigPath string
	CodexConfigPath    string
}

// NewLongtermMemAdapter builds a LongtermMemAdapter with real OS defaults
// for any argument left empty.
func NewLongtermMemAdapter(stateDir, binaryPath string) LongtermMemAdapter {
	if stateDir == "" {
		stateDir = DefaultLongtermMemStateDir()
	}
	if binaryPath == "" {
		binaryPath = DefaultLongtermMemBinaryPath()
	}
	return LongtermMemAdapter{
		StateDir:           stateDir,
		BinaryPath:         binaryPath,
		ClaudeConfigPath:   DefaultClaudeMCPConfigPath(),
		OpenCodeConfigPath: defaultOpenCodeMCPConfigPath(),
		CodexConfigPath:    defaultCodexMCPConfigPath(),
	}
}

// DefaultLongtermMemStateDir returns ~/.labdrian-overlay, the same root that
// hosts D5's vaults.json, so every overlay-owned cross-runtime record lives
// in one place.
func DefaultLongtermMemStateDir() string {
	home := resolveHome()
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".labdrian-overlay")
}

// LongtermMemBinaryPathForStateDir returns the binary path a given state
// dir implies: <stateDir>/bin/longterm-mem.
//
// The coupling is not a convenience — it is the contract. The overlay
// entrypoint derives LONGTERM_MEM_BINARY from its own $STATE_DIR
// ("$STATE_DIR/bin/longterm-mem"), deploys the binary there, and passes the
// same path to `longterm-mem register --binary`, so the MCP entry written
// into each runtime's config names it too. This adapter re-derives
// ownership by rebuilding that entry from its own BinaryPath, so a
// BinaryPath resolved from HOME while the state dir was overridden made
// three separate things disagree at once: the binary was reported missing,
// the entry naming the real binary stopped looking owned, and no record was
// written for a runtime that was correctly installed.
//
// An empty stateDir returns empty rather than "bin/longterm-mem" relative
// to whatever directory the process happens to be in; NewLongtermMemAdapter
// substitutes the real default for it.
func LongtermMemBinaryPathForStateDir(stateDir string) string {
	if stateDir == "" {
		return ""
	}
	return filepath.Join(stateDir, "bin", "longterm-mem")
}

// DefaultLongtermMemBinaryPath returns the fixed, documented persistent
// install path from the longterm-mem-install spec — the state-dir-derived
// path above for the DEFAULT state dir, so the default and the overridden
// case cannot drift apart.
func DefaultLongtermMemBinaryPath() string {
	return LongtermMemBinaryPathForStateDir(DefaultLongtermMemStateDir())
}

// DefaultClaudeMCPConfigPath returns ~/.claude.json — the Claude Code MCP
// server registry. This is a DIFFERENT file than ClaudeAdapter's
// settings.json (hooks); ~/.claude.json is a sibling of ~/.claude/, not a
// file inside it.
func DefaultClaudeMCPConfigPath() string {
	home := resolveHome()
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".claude.json")
}

// defaultOpenCodeMCPConfigPath reuses the same root resolution the
// runtime-parity OpenCodeAdapter already uses (10a.9: genuinely shared,
// since both need "where does opencode's global config live").
func defaultOpenCodeMCPConfigPath() string {
	root := DefaultOpenCodeConfigRoot()
	if root == "" {
		return ""
	}
	return filepath.Join(root, "opencode.json")
}

// defaultCodexMCPConfigPath reuses CodexAdapter's root resolution (10a.9).
func defaultCodexMCPConfigPath() string {
	root := DefaultCodexConfigRoot()
	if root == "" {
		return ""
	}
	return filepath.Join(root, "config.toml")
}

func resolveHome() string {
	if home := strings.TrimSpace(os.Getenv("HOME")); home != "" {
		return home
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return home
	}
	return ""
}

func (a LongtermMemAdapter) Target() Target         { return TargetLongtermMem }
func (a LongtermMemAdapter) Apply() LifecycleResult { return a.Install() }

// Install records the component's registration and reports a per-runtime
// status (R-014 scenario: "Install records registration and reports
// per-runtime status"). It performs exactly one write: registration.json
// under StateDir. It never writes to any runtime's own config file — that
// belongs to the module-owned "register" step that runs before this
// (D4 data flow: build → register → engine record).
func (a LongtermMemAdapter) Install() LifecycleResult {
	targets := a.observeAllTargets()

	reg := longtermMemRegistration{
		ManagedBy: longtermMemManagedBy,
		Targets:   map[string]longtermMemTargetRecord{},
	}
	for name, obs := range targets {
		// A record is written ONLY for a target whose entry was actually
		// observed AND is provably one this overlay wrote. Two defects live
		// in the alternative:
		//
		//   - Recording every target from observed state gave an absent
		//     runtime {Fingerprint:"", EntryPresent:false}, which the very
		//     next status read back as "record without entry" — a defect
		//     manufactured out of a runtime that simply is not installed.
		//
		//   - Recording an entry we did not write claimed a third party's
		//     MCP server as this overlay's own: the component reported
		//     `supported`, and reported "fingerprint drift" when that third
		//     party edited their own entry (A2). `longterm-mem register`
		//     refuses such an entry outright; the engine record must not
		//     quietly disagree with it.
		//
		// Skipping it here is what lets the matrix tell the truth: an
		// unowned entry has no record, so it reports as the unmanaged entry
		// it is, and an absent runtime has nothing to report at all.
		if !obs.entryPresent || !obs.entryOwned {
			continue
		}
		reg.Targets[name] = longtermMemTargetRecord{
			Fingerprint:  obs.entryFingerprint,
			EntryPresent: true,
		}
	}
	if err := a.writeRegistration(reg); err != nil {
		return a.aggregateResult(ActionInstall, nil, fmt.Sprintf("registration could not be recorded: %v", err))
	}

	results := a.evaluateAll(targets, &reg)
	return a.aggregateResult(ActionInstall, results, "")
}

// Status reports the per-runtime status directly from the existing
// registration record and read-only config inspection — no build step, no
// write (R-014 scenario: "Status and uninstall report without requiring a
// build").
func (a LongtermMemAdapter) Status() LifecycleResult {
	targets := a.observeAllTargets()
	reg, err := a.readRegistration()
	if err != nil {
		return a.aggregateResult(ActionStatus, nil, fmt.Sprintf("registration could not be read: %v", err))
	}
	results := a.evaluateAll(targets, reg)
	return a.aggregateResult(ActionStatus, results, "")
}

func (a LongtermMemAdapter) SyncCheck() LifecycleResult {
	result := a.Status()
	result.Action = ActionSyncCheck
	return result
}

// Uninstall removes only the registration record this component owns. It
// never touches any runtime's own config file (that selective removal is
// the module-owned "unregister" step, R-019) — matching the hard
// constraint that nothing in this slice writes to a user's runtime config
// outside the registration record it owns.
//
// It returns its OWN verdict rather than running the install-health matrix
// over a post-uninstall observation, which is a category error: uninstall's
// success question is "did I remove what I own?", while the matrix's
// `supported` row describes a LIVE installation (record present, entry
// present, fingerprints agreeing). Running it here made a flawless
// uninstall structurally incapable of reporting success — every path went
// through recordPresent=false and reported `partial`. Update() and
// Rollback() already return explicit verdicts for the same reason.
//
// Success is the requested END STATE holding: the record was removed, or
// was already absent. Only an unresolvable state dir or a removal that
// genuinely failed is unsupported.
//
// This says nothing about what a SUBSEQUENT status reports; that is the
// separate constraint runtime-lifecycle's "Claude uninstall removes owned
// lifecycle state" scenario places on status, not on uninstall's own result.
func (a LongtermMemAdapter) Uninstall() LifecycleResult {
	path := a.registrationPath()
	if path == "" {
		return NewLifecycleResult(TargetLongtermMem, ActionUninstall, CapabilityUnsupported,
			LongtermMemReasonConfigRootUnresolvable+": state dir could not be resolved; set HOME", nil)
	}

	removed := true
	if err := os.Remove(path); err != nil {
		if !os.IsNotExist(err) {
			return NewLifecycleResult(TargetLongtermMem, ActionUninstall, CapabilityUnsupported,
				fmt.Sprintf("registration could not be removed: %v", err), nil)
		}
		removed = false
	}

	message := "longterm-mem registration removed"
	if !removed {
		message = "longterm-mem registration was already absent"
	}
	return NewLifecycleResult(TargetLongtermMem, ActionUninstall, CapabilitySupported,
		message, a.remainingObservationLines())
}

// remainingObservationLines describes what each runtime's own config still
// holds after the engine record is gone. These are INFORMATION, not a
// verdict: removing an MCP entry from a runtime's own config is the
// module-owned `longterm-mem unregister` step's job, never this adapter's,
// so an entry that is still there is a fact to report — not a complaint
// about an uninstall that did exactly what it owns.
func (a LongtermMemAdapter) remainingObservationLines() []string {
	targets := a.observeAllTargets()
	lines := make([]string, 0, 3)
	for _, name := range []Target{TargetClaude, TargetOpenCode, TargetCodex} {
		obs := targets[string(name)]
		lines = append(lines, string(name)+": "+remainingObservation(obs))
	}
	return lines
}

func remainingObservation(obs longtermMemObservation) string {
	switch {
	case !obs.runtimePresent:
		return "runtime not installed on this machine; nothing to report"
	case obs.entryPresent:
		return "MCP entry still present in the runtime's own config; removing it is `longterm-mem unregister`'s job"
	default:
		return "no longterm-mem MCP entry in the runtime's own config"
	}
}

// Update refuses explicitly rather than performing a silent no-op: the
// longterm-mem component offers no update surface at all (D4, R-014
// scenario "No update or rollback surface is offered"). No file is read or
// written — the refusal happens before any adapter I/O, matching the
// CLI-layer parse-time refusal in cmd/main.go.
func (a LongtermMemAdapter) Update() LifecycleResult {
	return NewLifecycleResult(TargetLongtermMem, ActionUpdate, CapabilityUnsupported,
		"longterm-mem does not support update; reinstall to pick up a new binary or registration",
		[]string{"capability not offered: use install to re-record the current state"})
}

// Rollback refuses explicitly for the same reason as Update — see above.
func (a LongtermMemAdapter) Rollback() LifecycleResult {
	return NewLifecycleResult(TargetLongtermMem, ActionRollback, CapabilityUnsupported,
		"longterm-mem does not support rollback; uninstall then reinstall instead",
		[]string{"capability not offered: no versioned rollback surface exists"})
}

// --- observation to status (10a.1) ---

func (a LongtermMemAdapter) evaluateAll(targets map[string]longtermMemObservation, reg *longtermMemRegistration) map[string]LifecycleResult {
	results := map[string]LifecycleResult{}
	for _, name := range []Target{TargetClaude, TargetOpenCode, TargetCodex} {
		obs := targets[string(name)]
		var record longtermMemTargetRecord
		recordPresent := false
		if reg != nil {
			record, recordPresent = reg.Targets[string(name)]
		}
		fingerprintMatch := recordPresent && obs.entryPresent && record.Fingerprint == obs.entryFingerprint
		status, reason := EvaluateLongtermMemComponentStatus(LongtermMemComponentState{
			RootResolvable:   obs.rootResolvable,
			BinaryPresent:    obs.binaryPresent,
			RuntimePresent:   obs.runtimePresent,
			RecordPresent:    recordPresent,
			EntryPresent:     obs.entryPresent,
			FingerprintMatch: fingerprintMatch,
		})
		message := string(name) + ": " + string(status)
		if reason != "" {
			message += " — " + reason
		}
		results[string(name)] = NewLifecycleResult(name, ActionStatus, status, message, nil)
	}
	return results
}

// aggregateResult folds three per-runtime LifecycleResults into one, per
// the Adapter interface's single-result contract: the overall status is the
// worst of the three (unsupported > partial > supported), and every
// per-runtime line is preserved verbatim in Reasons so nothing is lost.
func (a LongtermMemAdapter) aggregateResult(action Action, perTarget map[string]LifecycleResult, overrideMessage string) LifecycleResult {
	if overrideMessage != "" {
		return NewLifecycleResult(TargetLongtermMem, action, CapabilityUnsupported, overrideMessage, nil)
	}

	overall := CapabilitySupported
	var reasons []string
	for _, name := range []Target{TargetClaude, TargetOpenCode, TargetCodex} {
		result, ok := perTarget[string(name)]
		if !ok {
			continue
		}
		reasons = append(reasons, result.Message)
		overall = worseLongtermMemStatus(overall, result.Status)
	}
	return NewLifecycleResult(TargetLongtermMem, action, overall, "longterm-mem per-runtime status", reasons)
}

func worseLongtermMemStatus(a, b CapabilityStatus) CapabilityStatus {
	rank := map[CapabilityStatus]int{
		CapabilitySupported:   0,
		CapabilityPartial:     1,
		CapabilityUnsupported: 2,
	}
	if rank[b] > rank[a] {
		return b
	}
	return a
}

// --- registration.json persistence ---

func (a LongtermMemAdapter) registrationPath() string {
	if a.StateDir == "" {
		return ""
	}
	return filepath.Join(a.StateDir, longtermMemRegistrationFile)
}

func (a LongtermMemAdapter) readRegistration() (*longtermMemRegistration, error) {
	path := a.registrationPath()
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var reg longtermMemRegistration
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, err
	}
	return &reg, nil
}

func (a LongtermMemAdapter) writeRegistration(reg longtermMemRegistration) error {
	path := a.registrationPath()
	if path == "" {
		return fmt.Errorf("%s: state dir could not be resolved; set HOME", LongtermMemReasonConfigRootUnresolvable)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := writeFileAtomic(path, encoded)
	if err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("registration finalization failed: %w", err)
	}
	return nil
}
