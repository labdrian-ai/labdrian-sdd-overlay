package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const codexManifestFile = "labdrian-runtime-lifecycle.json"
const managedByLabdrianOverlay = "labdrian-sdd-overlay"

// CodexInstalledVersion is intentionally stable so manifest contents stay
// deterministic across runs and tests.
const CodexInstalledVersion = "2026-07-05-codex-runtime-lifecycle"

type codexManifest struct {
	ManagedBy        string `json:"managed_by"`
	InstalledVersion string `json:"installed_version"`
	ConfigRoot       string `json:"config_root"`
}

// CodexAdapter is the runtime adapter for the Codex CLI.
// It owns only one manifest file so unrelated user state is preserved.
type CodexAdapter struct {
	target Target
	root   string
}

func NewCodexAdapter(root string) CodexAdapter {
	if root == "" {
		root = DefaultCodexConfigRoot()
	}
	return CodexAdapter{target: TargetCodex, root: root}
}

func DefaultCodexConfigRoot() string {
	if dir := strings.TrimSpace(os.Getenv("CODEX_HOME")); dir != "" {
		clean := filepath.Clean(dir)
		if filepath.IsAbs(clean) {
			return clean
		}
	}

	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".codex")
	}

	return ""
}

func (a CodexAdapter) Target() Target         { return a.target }
func (a CodexAdapter) Apply() LifecycleResult { return a.Install() }

func (a CodexAdapter) Install() LifecycleResult {
	if err := a.validateRootForMutation("Codex config root"); err != nil {
		return a.result(ActionInstall, statusForRootError(err), err.Error())
	}

	if err := a.ensureManifestSafeForMutation("install"); err != nil {
		if os.IsNotExist(err) {
			// missing manifest is safe for initialization
		} else {
			return a.result(ActionInstall, CapabilityPartial, err.Error())
		}
	}

	manifest := codexManifest{
		ManagedBy:        managedByLabdrianOverlay,
		InstalledVersion: CodexInstalledVersion,
		ConfigRoot:       a.root,
	}

	if err := a.writeManifest(manifest); err != nil {
		return a.result(ActionInstall, CapabilityPartial, err.Error())
	}

	return a.result(ActionInstall, CapabilityRestartRequired, "Codex lifecycle manifest installed; restart long-running Codex processes to pick up updates")
}

func (a CodexAdapter) Status() LifecycleResult { return a.status() }

func (a CodexAdapter) SyncCheck() LifecycleResult {
	result := a.Status()
	result.Action = ActionSyncCheck
	return result
}

func (a CodexAdapter) Update() LifecycleResult {
	if err := a.validateRootForMutation("Codex config root"); err != nil {
		return a.result(ActionUpdate, statusForRootError(err), err.Error())
	}

	if err := a.ensureManifestSafeForMutation("update"); err != nil {
		if !os.IsNotExist(err) {
			return a.result(ActionUpdate, CapabilityPartial, err.Error())
		}
	}

	manifest, err := a.readManifest()
	if err != nil {
		if !os.IsNotExist(err) {
			return a.result(ActionUpdate, CapabilityPartial, "failed to read existing Codex manifest: "+err.Error())
		}
		manifest = codexManifest{}
	}

	if manifest.ManagedBy == "" {
		manifest.ManagedBy = managedByLabdrianOverlay
	}
	manifest.InstalledVersion = CodexInstalledVersion
	manifest.ConfigRoot = a.root

	if err := a.writeManifest(manifest); err != nil {
		return a.result(ActionUpdate, CapabilityPartial, err.Error())
	}

	return a.result(ActionUpdate, CapabilityRestartRequired, "Codex lifecycle manifest updated; restart long-running Codex processes to pick up updates")
}

func (a CodexAdapter) Rollback() LifecycleResult { return a.Uninstall() }

func (a CodexAdapter) Uninstall() LifecycleResult {
	if err := a.validateRootForMutation("Codex config root"); err != nil {
		return a.result(ActionUninstall, statusForRootError(err), err.Error())
	}

	if err := a.ensureManifestSafeForMutation("uninstall"); err != nil {
		if !os.IsNotExist(err) {
			return a.result(ActionUninstall, CapabilityPartial, err.Error())
		}
	}

	manifestPath := filepath.Join(a.root, codexManifestFile)
	if err := os.Remove(manifestPath); err != nil && !os.IsNotExist(err) {
		return a.result(ActionUninstall, CapabilityPartial, "could not remove Codex lifecycle manifest: "+err.Error())
	}

	return a.result(ActionUninstall, CapabilityRestartRequired, "Codex lifecycle manifest removed; restart long-running Codex processes")
}

func (a CodexAdapter) status() LifecycleResult {
	if err := a.validateRoot(); err != nil {
		return a.result(ActionStatus, CapabilityUnsupported, err.Error())
	}

	manifest, err := a.readManifest()
	if err != nil {
		if os.IsNotExist(err) {
			return a.result(ActionStatus, CapabilityPartial, "Codex lifecycle manifest not found at "+filepath.Join(a.root, codexManifestFile))
		}
		return a.result(ActionStatus, CapabilityPartial, "failed to read Codex manifest: "+err.Error())
	}

	if err := validateCodexManifest(manifest, a.root); err != nil {
		return a.result(ActionStatus, CapabilityPartial, err.Error())
	}

	return a.resultWithReasons(ActionStatus, CapabilityPartial, "Codex lifecycle manifest is present and managed, but activation/reload state is unverified", []string{"activation/reload proof unavailable until Codex session lifecycle integration is implemented"})
}

func (a CodexAdapter) result(action Action, status CapabilityStatus, message string) LifecycleResult {
	return NewLifecycleResult(a.target, action, status, message, nil)
}

func (a CodexAdapter) resultWithReasons(action Action, status CapabilityStatus, message string, reasons []string) LifecycleResult {
	return NewLifecycleResult(a.target, action, status, message, reasons)
}

func (a CodexAdapter) manifestPath() string {
	return filepath.Join(a.root, codexManifestFile)
}

func (a CodexAdapter) readManifest() (codexManifest, error) {
	data, err := os.ReadFile(a.manifestPath())
	if err != nil {
		return codexManifest{}, err
	}

	var manifest codexManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return codexManifest{}, err
	}

	return manifest, nil
}

func (a CodexAdapter) writeManifest(manifest codexManifest) error {
	if err := a.validateRootForMutation("Codex config root"); err != nil {
		return err
	}

	if err := os.MkdirAll(a.root, 0o755); err != nil {
		return fmt.Errorf("Codex config root directory could not be prepared: %w", err)
	}

	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}

	tmp, err := writeFileAtomic(a.manifestPath(), encoded)
	if err != nil {
		return err
	}

	if err := os.Rename(tmp, a.manifestPath()); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("Codex manifest finalization failed: %w", err)
	}

	return nil
}

func (a CodexAdapter) ensureManifestSafeForMutation(action string) error {
	manifestPath := a.manifestPath()

	manifest, err := a.readManifest()
	if err != nil {
		if os.IsNotExist(err) {
			return err
		}
		return fmt.Errorf("cannot validate existing Codex manifest for %s at %q: %w", action, manifestPath, err)
	}

	if err := validateCodexManifest(manifest, a.root); err != nil {
		return fmt.Errorf("cannot mutate existing Codex manifest for %s at %q (must be managed by %q): %w", action, manifestPath, managedByLabdrianOverlay, err)
	}

	return nil
}

func writeFileAtomic(path string, data []byte) (string, error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-labdrian-runtime-*")
	if err != nil {
		return "", fmt.Errorf("Codex manifest temp creation failed: %w", err)
	}

	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("Codex manifest temp write failed: %w", err)
	}

	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("Codex manifest temp sync failed: %w", err)
	}

	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("Codex manifest temp close failed: %w", err)
	}

	if err := os.Chmod(tmpPath, 0o644); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("Codex manifest temp permission set failed: %w", err)
	}

	return tmpPath, nil
}

func validateCodexManifest(manifest codexManifest, root string) error {
	if manifest.ManagedBy != managedByLabdrianOverlay {
		return fmt.Errorf("manifest managed_by mismatch: %q", manifest.ManagedBy)
	}
	if strings.TrimSpace(manifest.InstalledVersion) == "" {
		return fmt.Errorf("manifest installed_version is empty")
	}
	if manifest.ConfigRoot != root {
		return fmt.Errorf("manifest config_root %q does not match resolved root %q", manifest.ConfigRoot, root)
	}
	return nil
}

func (a CodexAdapter) validateRoot() error {
	if a.root == "" {
		return fmt.Errorf("Codex config root could not be resolved; set CODEX_HOME or HOME")
	}
	if !filepath.IsAbs(a.root) {
		return fmt.Errorf("Codex config root must be absolute, got %q", a.root)
	}
	return nil
}

func (a CodexAdapter) validateRootForMutation(field string) error {
	if err := a.validateRoot(); err != nil {
		return err
	}

	info, err := os.Stat(a.root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("%s %q cannot be resolved: %w", field, a.root, err)
	}

	if !info.IsDir() {
		return fmt.Errorf("%s %q is not a directory", field, a.root)
	}

	return nil
}

func statusForRootError(err error) CapabilityStatus {
	errMsg := err.Error()
	if strings.HasPrefix(errMsg, "Codex config root could not be resolved") ||
		strings.HasPrefix(errMsg, "Codex config root must be absolute") {
		return CapabilityUnsupported
	}
	return CapabilityPartial
}
