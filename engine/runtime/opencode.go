package runtime

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const openCodePluginFile = "labdrian-runtime-parity.js"
const openCodeConfigFile = "labdrian-runtime-parity.json"
const openCodeActiveFile = "labdrian-runtime-parity.active.json"

const OpenCodePluginVersion = "2026-06-22-runtime-parity-3"

//go:embed labdrian-runtime-parity-plugin.mjs
var openCodePluginSource string

type OpenCodeAdapter struct {
	root string
}

type openCodeConfig struct {
	// InstalledHash is the plugin artifact hash last written by the adapter.
	// InstalledVersion is the deterministic plugin version last installed.
	PluginPath        string               `json:"plugin_path"`
	InstalledHash     string               `json:"installed_hash"`
	InstalledVersion  string               `json:"installed_version"`
	ActivationMarker  string               `json:"activation_marker"`
	PluginConfigRoot  string               `json:"plugin_config_root"`
	PluginConfigScope string               `json:"plugin_config_scope"`
	PromptConfig      openCodePromptConfig `json:"prompt_config"`
	PromptConfigHash  string               `json:"prompt_config_hash"`
}

type openCodePromptConfig struct {
	ContractPath   string   `json:"contract_path"`
	IncludedPhases []string `json:"included_phases"`
	ExcludedPhases []string `json:"excluded_phases"`
	InjectionPoint string   `json:"injection_point"`
}

type openCodeActiveMarker struct {
	ActiveVersion          string `json:"active_version"`
	ActiveHash             string `json:"active_hash"`
	ActivePromptConfigHash string `json:"active_prompt_config_hash"`
	PluginPath             string `json:"plugin_path"`
	ConfigRoot             string `json:"config_root"`
}

func NewOpenCodeAdapter(root string) OpenCodeAdapter {
	return OpenCodeAdapter{root: root}
}

func DefaultOpenCodeConfigRoot() string {
	if dir := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); dir != "" {
		if filepath.IsAbs(dir) {
			return filepath.Join(dir, "opencode")
		}
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".config", "opencode")
	}
	return ""
}

func OpenCodePluginHash() string {
	sum := sha256.Sum256([]byte(openCodePluginSource))
	return hex.EncodeToString(sum[:])
}

func (a OpenCodeAdapter) Target() Target             { return TargetOpenCode }
func (a OpenCodeAdapter) Apply() LifecycleResult     { return a.Install() }
func (a OpenCodeAdapter) Install() LifecycleResult   { return a.install(ActionInstall) }
func (a OpenCodeAdapter) Status() LifecycleResult    { return a.status(ActionStatus) }
func (a OpenCodeAdapter) SyncCheck() LifecycleResult { return a.status(ActionSyncCheck) }
func (a OpenCodeAdapter) Update() LifecycleResult    { return a.install(ActionUpdate) }
func (a OpenCodeAdapter) Rollback() LifecycleResult  { return a.uninstall(ActionRollback) }

func (a OpenCodeAdapter) Uninstall() LifecycleResult { return a.uninstall(ActionUninstall) }

func (a OpenCodeAdapter) uninstall(action Action) LifecycleResult {
	if err := a.validateRoot(); err != nil {
		return a.result(action, CapabilityUnsupported, err.Error())
	}
	pluginErr := os.Remove(a.pluginPath())
	configErr := os.Remove(a.configPath())
	if pluginErr != nil && !os.IsNotExist(pluginErr) {
		return a.result(action, CapabilityPartial, pluginErr.Error())
	}
	if configErr != nil && !os.IsNotExist(configErr) {
		return a.result(action, CapabilityPartial, configErr.Error())
	}
	return a.result(action, CapabilityRestartRequired, "OpenCode plugin bridge removed; restart OpenCode to unload any already loaded plugin. Active marker remains at "+a.activeMarkerPath()+"; remove it only after OpenCode is fully stopped/restarted")
}

func (a OpenCodeAdapter) install(action Action) LifecycleResult {
	if err := a.validateRoot(); err != nil {
		return a.result(action, CapabilityUnsupported, err.Error())
	}
	if err := os.MkdirAll(filepath.Dir(a.pluginPath()), 0o755); err != nil {
		return a.result(action, CapabilityUnsupported, err.Error())
	}
	if err := os.WriteFile(a.pluginPath(), []byte(openCodePluginSource), 0o644); err != nil {
		return a.result(action, CapabilityUnsupported, err.Error())
	}
	promptConfig, err := loadOpenCodePromptConfig()
	if err != nil {
		return a.result(action, CapabilityPartial, "OpenCode prompt config could not be derived from minimalism-contract frontmatter: "+err.Error())
	}
	cfg := openCodeConfig{
		PluginPath:        a.pluginPath(),
		InstalledHash:     OpenCodePluginHash(),
		InstalledVersion:  OpenCodePluginVersion,
		ActivationMarker:  a.activeMarkerPath(),
		PluginConfigRoot:  a.root,
		PluginConfigScope: "global-opencode-config",
		PromptConfig:      promptConfig,
		PromptConfigHash:  promptConfigHash(promptConfig),
	}
	if err := a.writeConfig(cfg); err != nil {
		return a.result(action, CapabilityPartial, err.Error())
	}
	return a.result(action, CapabilityRestartRequired, "OpenCode plugin changed; restart OpenCode to load version "+cfg.InstalledVersion)
}

func (a OpenCodeAdapter) status(action Action) LifecycleResult {
	if err := a.validateRoot(); err != nil {
		return a.result(action, CapabilityUnsupported, err.Error())
	}
	plugin, err := os.ReadFile(a.pluginPath())
	if err != nil {
		if os.IsNotExist(err) {
			if active, activeErr := a.readActiveMarker(); activeErr == nil && active.ActiveVersion != "" {
				return a.result(action, CapabilityRestartRequired, "OpenCode plugin removed but active marker remains for "+active.ActiveVersion+"; restart OpenCode to unload the plugin")
			} else if activeErr != nil && a.activeMarkerExists() {
				return a.result(action, CapabilityRestartRequired, "OpenCode plugin removed but active marker at "+a.activeMarkerPath()+" is unreadable or invalid; restart OpenCode and perform manual cleanup only after the plugin is unloaded")
			}
		}
		return a.result(action, CapabilityUnsupported, "OpenCode plugin not installed")
	}
	cfg, err := a.readConfig()
	if err != nil {
		if isPromptConfigMismatch(err) {
			return a.result(action, CapabilityRestartRequired, "OpenCode prompt_config is stale or tampered; reinstall/update and restart OpenCode: "+err.Error())
		}
		return a.result(action, CapabilityPartial, "OpenCode config missing or invalid: "+err.Error())
	}
	currentHash := hashString(string(plugin))
	if cfg.InstalledHash != currentHash {
		return a.result(action, CapabilityRestartRequired, "OpenCode plugin artifact changed; restart OpenCode after reinstall")
	}
	active, err := a.readActiveMarker()
	if err != nil {
		return a.result(action, CapabilityRestartRequired, "OpenCode restart required to load plugin version "+cfg.InstalledVersion)
	}
	if a.activeMarkerOlderThanInstalledConfig() {
		return a.result(action, CapabilityRestartRequired, "OpenCode active marker predates installed plugin/config; restart OpenCode to load current runtime bridge")
	}
	if active.ActiveVersion != cfg.InstalledVersion {
		return a.result(action, CapabilityRestartRequired, "OpenCode active plugin version mismatch; restart OpenCode to load "+cfg.InstalledVersion)
	}
	if active.ActiveHash != currentHash {
		return a.result(action, CapabilityRestartRequired, "OpenCode active plugin hash mismatch; restart OpenCode to load current plugin")
	}
	if active.ActivePromptConfigHash != cfg.PromptConfigHash {
		return a.result(action, CapabilityRestartRequired, "OpenCode active prompt config mismatch; restart OpenCode to load current prompt config")
	}
	if active.PluginPath != cfg.PluginPath {
		return a.result(action, CapabilityRestartRequired, "OpenCode active plugin path mismatch; restart OpenCode to load "+cfg.PluginPath)
	}
	if active.ConfigRoot != "" && active.ConfigRoot != cfg.PluginConfigRoot {
		return a.result(action, CapabilityRestartRequired, "OpenCode active config root mismatch; restart OpenCode to load "+cfg.PluginConfigRoot)
	}
	return a.result(action, CapabilitySupported, "OpenCode plugin active with version "+cfg.InstalledVersion+" and hash "+currentHash)
}

func (a OpenCodeAdapter) writeConfig(cfg openCodeConfig) error {
	if err := os.MkdirAll(filepath.Dir(a.configPath()), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(a.configPath(), data, 0o644)
}

func (a OpenCodeAdapter) readConfig() (openCodeConfig, error) {
	data, err := os.ReadFile(a.configPath())
	if err != nil {
		return openCodeConfig{}, err
	}
	var cfg openCodeConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return openCodeConfig{}, err
	}
	if cfg.InstalledHash == "" {
		return openCodeConfig{}, fmt.Errorf("installed_hash is empty")
	}
	if cfg.InstalledVersion == "" {
		return openCodeConfig{}, fmt.Errorf("installed_version is empty")
	}
	if cfg.InstalledVersion != OpenCodePluginVersion {
		return openCodeConfig{}, fmt.Errorf("installed_version %q is not current %q", cfg.InstalledVersion, OpenCodePluginVersion)
	}
	if cfg.InstalledHash != OpenCodePluginHash() {
		return openCodeConfig{}, fmt.Errorf("installed_hash is not current plugin hash")
	}
	if cfg.PluginPath != a.pluginPath() {
		return openCodeConfig{}, fmt.Errorf("plugin_path does not match configured OpenCode root")
	}
	if cfg.PluginConfigRoot != "" && cfg.PluginConfigRoot != a.root {
		return openCodeConfig{}, fmt.Errorf("plugin_config_root does not match configured OpenCode root")
	}
	expectedPromptConfig, err := loadOpenCodePromptConfig()
	if err != nil {
		return openCodeConfig{}, fmt.Errorf("current prompt_config could not be derived: %w", err)
	}
	if err := validatePromptConfig(cfg.PromptConfig, expectedPromptConfig); err != nil {
		return openCodeConfig{}, promptConfigMismatchError{err: err}
	}
	expectedHash := promptConfigHash(expectedPromptConfig)
	if cfg.PromptConfigHash != expectedHash {
		return openCodeConfig{}, promptConfigMismatchError{err: fmt.Errorf("prompt_config_hash %q is not current %q", cfg.PromptConfigHash, expectedHash)}
	}
	return cfg, nil
}

func (a OpenCodeAdapter) readActiveMarker() (openCodeActiveMarker, error) {
	data, err := os.ReadFile(a.activeMarkerPath())
	if err != nil {
		return openCodeActiveMarker{}, err
	}
	var marker openCodeActiveMarker
	if err := json.Unmarshal(data, &marker); err != nil {
		return openCodeActiveMarker{}, err
	}
	if marker.ActiveVersion == "" {
		return openCodeActiveMarker{}, fmt.Errorf("active_version is empty")
	}
	if marker.ActiveHash == "" {
		return openCodeActiveMarker{}, fmt.Errorf("active_hash is empty")
	}
	if marker.ActivePromptConfigHash == "" {
		return openCodeActiveMarker{}, fmt.Errorf("active_prompt_config_hash is empty")
	}
	if marker.PluginPath == "" {
		return openCodeActiveMarker{}, fmt.Errorf("plugin_path is empty")
	}
	if marker.ConfigRoot == "" {
		return openCodeActiveMarker{}, fmt.Errorf("config_root is empty")
	}
	return marker, nil
}

func (a OpenCodeAdapter) pluginPath() string {
	return filepath.Join(a.root, "plugins", openCodePluginFile)
}

func (a OpenCodeAdapter) configPath() string {
	return filepath.Join(a.root, openCodeConfigFile)
}

func (a OpenCodeAdapter) activeMarkerPath() string {
	return filepath.Join(a.root, openCodeActiveFile)
}

func (a OpenCodeAdapter) activeMarkerExists() bool {
	_, err := os.Stat(a.activeMarkerPath())
	return err == nil
}

func (a OpenCodeAdapter) activeMarkerOlderThanInstalledConfig() bool {
	marker, err := os.Stat(a.activeMarkerPath())
	if err != nil {
		return false
	}
	for _, path := range []string{a.pluginPath(), a.configPath()} {
		info, err := os.Stat(path)
		if err == nil && marker.ModTime().Before(info.ModTime()) {
			return true
		}
	}
	return false
}

func (a OpenCodeAdapter) validateRoot() error {
	if a.root == "" {
		return fmt.Errorf("OpenCode config root could not be resolved; set HOME or XDG_CONFIG_HOME")
	}
	if !filepath.IsAbs(a.root) {
		return fmt.Errorf("OpenCode config root must be absolute, got %q", a.root)
	}
	return nil
}

func (a OpenCodeAdapter) result(action Action, status CapabilityStatus, message string) LifecycleResult {
	return NewLifecycleResult(TargetOpenCode, action, status, message, nil)
}

func loadOpenCodePromptConfig() (openCodePromptConfig, error) {
	contractPath := filepath.Join("skills", "_shared", "minimalism-contract.md")
	root, err := overlayRoot()
	if err != nil {
		return openCodePromptConfig{}, err
	}
	content, err := os.ReadFile(filepath.Join(root, contractPath))
	if err != nil {
		return openCodePromptConfig{}, err
	}
	phases, err := LoadContractPhases(string(content))
	if err != nil {
		return openCodePromptConfig{}, err
	}
	return openCodePromptConfig{
		ContractPath:   filepath.ToSlash(contractPath),
		IncludedPhases: append([]string(nil), phases.AppliesTo...),
		ExcludedPhases: append([]string(nil), phases.Excluded...),
		InjectionPoint: injectionHeader(phases),
	}, nil
}

type promptConfigMismatchError struct {
	err error
}

func (e promptConfigMismatchError) Error() string { return e.err.Error() }

func isPromptConfigMismatch(err error) bool {
	_, ok := err.(promptConfigMismatchError)
	return ok
}

func promptConfigHash(config openCodePromptConfig) string {
	data, err := json.Marshal(config)
	if err != nil {
		return ""
	}
	return hashString(string(data))
}

func validatePromptConfig(got, want openCodePromptConfig) error {
	if got.ContractPath == "" {
		return fmt.Errorf("prompt_config.contract_path is empty")
	}
	if got.InjectionPoint == "" {
		return fmt.Errorf("prompt_config.injection_point is empty")
	}
	if !equalStringSlices(got.IncludedPhases, want.IncludedPhases) {
		return fmt.Errorf("prompt_config.included_phases %v is not current %v", got.IncludedPhases, want.IncludedPhases)
	}
	if !equalStringSlices(got.ExcludedPhases, want.ExcludedPhases) {
		return fmt.Errorf("prompt_config.excluded_phases %v is not current %v", got.ExcludedPhases, want.ExcludedPhases)
	}
	if got.ContractPath != want.ContractPath {
		return fmt.Errorf("prompt_config.contract_path %q is not current %q", got.ContractPath, want.ContractPath)
	}
	if got.InjectionPoint != want.InjectionPoint {
		return fmt.Errorf("prompt_config.injection_point %q is not current %q", got.InjectionPoint, want.InjectionPoint)
	}
	return nil
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func overlayRoot() (string, error) {
	if root := strings.TrimSpace(os.Getenv("LABDRIAN_OVERLAY_DIR")); root != "" {
		if filepath.IsAbs(root) {
			return root, nil
		}
		return "", fmt.Errorf("LABDRIAN_OVERLAY_DIR must be absolute, got %q", root)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for dir := cwd; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "skills", "_shared", "minimalism-contract.md")
		if _, err := os.Stat(candidate); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return "", fmt.Errorf("could not locate skills/_shared/minimalism-contract.md; set LABDRIAN_OVERLAY_DIR")
}

func hashString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
