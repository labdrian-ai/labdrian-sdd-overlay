package runtime

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/opencodeprompt"
)

// OpenCodeAdapter installs the runtime-parity plugin into the OpenCode config directory and
// reports whether OpenCode is running it. The plugin carries the prompt config derived from the
// overlay's contracts (opencodeprompt); the adapter writes the plugin and its records and reads
// them back.
type OpenCodeAdapter struct {
	root    string
	options OpenCodeOptions
}

// OpenCodeOptions are the choices the composition root hands the OpenCode adapter.
type OpenCodeOptions struct {
	// OverlayDir is the overlay checkout whose skills/_shared contracts the plugin carries, as the
	// environment gave it ($LABDRIAN_OVERLAY_DIR; see core.Config.LabdrianOverlayDir). Empty means
	// the nearest checkout above the working directory.
	OverlayDir string
}

// NewOpenCodeAdapter builds the OpenCode adapter over root, the directory its plugin is installed
// into (see core.Config.OpenCodeRoot for the default the caller resolves). It reads no
// environment.
func NewOpenCodeAdapter(root string, options OpenCodeOptions) OpenCodeAdapter {
	return OpenCodeAdapter{root: root, options: options}
}

func (a OpenCodeAdapter) Target() core.Target             { return core.TargetOpenCode }
func (a OpenCodeAdapter) Apply() core.LifecycleResult     { return a.Install() }
func (a OpenCodeAdapter) Install() core.LifecycleResult   { return a.install(core.ActionInstall) }
func (a OpenCodeAdapter) Status() core.LifecycleResult    { return a.status(core.ActionStatus) }
func (a OpenCodeAdapter) SyncCheck() core.LifecycleResult { return a.status(core.ActionSyncCheck) }
func (a OpenCodeAdapter) Update() core.LifecycleResult    { return a.install(core.ActionUpdate) }
func (a OpenCodeAdapter) Rollback() core.LifecycleResult  { return a.uninstall(core.ActionRollback) }
func (a OpenCodeAdapter) Uninstall() core.LifecycleResult { return a.uninstall(core.ActionUninstall) }

func (a OpenCodeAdapter) uninstall(action core.Action) core.LifecycleResult {
	if err := a.validateRoot(); err != nil {
		return a.result(action, core.CapabilityUnsupported, err.Error())
	}
	pluginErr := os.Remove(a.pluginPath())
	configErr := os.Remove(a.configPath())
	if pluginErr != nil && !os.IsNotExist(pluginErr) {
		return a.result(action, core.CapabilityPartial, pluginErr.Error())
	}
	if configErr != nil && !os.IsNotExist(configErr) {
		return a.result(action, core.CapabilityPartial, configErr.Error())
	}
	return a.result(action, core.CapabilityRestartRequired, "OpenCode plugin bridge removed; restart OpenCode to unload any already loaded plugin. Active marker remains at "+a.activeMarkerPath()+"; remove it only after OpenCode is fully stopped/restarted")
}

func (a OpenCodeAdapter) install(action core.Action) core.LifecycleResult {
	if err := a.validateRoot(); err != nil {
		return a.result(action, core.CapabilityUnsupported, err.Error())
	}
	promptConfig, err := a.promptConfig()
	if err != nil {
		return a.result(action, core.CapabilityPartial, "OpenCode prompt config could not be derived from the contracts: "+err.Error())
	}
	promptHash, err := opencodeprompt.Hash(promptConfig)
	if err != nil {
		return a.result(action, core.CapabilityPartial, "OpenCode prompt config could not be hashed: "+err.Error())
	}
	cfg := openCodeConfig{
		PluginPath:        a.pluginPath(),
		InstalledHash:     OpenCodePluginHash(),
		InstalledVersion:  OpenCodePluginVersion,
		ActivationMarker:  a.activeMarkerPath(),
		PluginConfigRoot:  a.root,
		PluginConfigScope: "global-opencode-config",
		PromptConfig:      promptConfig,
		PromptConfigHash:  promptHash,
	}
	if err := a.writeConfig(cfg); err != nil {
		return a.result(action, core.CapabilityPartial, err.Error())
	}
	if err := os.MkdirAll(filepath.Dir(a.pluginPath()), 0o755); err != nil {
		return a.result(action, core.CapabilityUnsupported, err.Error())
	}
	if err := os.WriteFile(a.pluginPath(), []byte(openCodePluginSource), 0o644); err != nil {
		return a.result(action, core.CapabilityUnsupported, err.Error())
	}
	return a.result(action, core.CapabilityRestartRequired, "OpenCode plugin changed; restart OpenCode to load version "+cfg.InstalledVersion)
}

func (a OpenCodeAdapter) status(action core.Action) core.LifecycleResult {
	if err := a.validateRoot(); err != nil {
		return a.result(action, core.CapabilityUnsupported, err.Error())
	}
	plugin, err := os.ReadFile(a.pluginPath())
	if err != nil {
		if os.IsNotExist(err) {
			if active, activeErr := a.readActiveMarker(); activeErr == nil && active.ActiveVersion != "" {
				return a.result(action, core.CapabilityRestartRequired, "OpenCode plugin removed but active marker remains for "+active.ActiveVersion+"; restart OpenCode to unload the plugin")
			} else if activeErr != nil && a.activeMarkerExists() {
				return a.result(action, core.CapabilityRestartRequired, "OpenCode plugin removed but active marker at "+a.activeMarkerPath()+" is unreadable or invalid; restart OpenCode and perform manual cleanup only after the plugin is unloaded")
			}
		}
		return a.result(action, core.CapabilityUnsupported, "OpenCode plugin not installed")
	}
	cfg, err := a.readConfig()
	if err != nil {
		if opencodeprompt.IsMismatch(err) {
			return a.result(action, core.CapabilityRestartRequired, "OpenCode prompt_config is stale or tampered; reinstall/update and restart OpenCode: "+err.Error())
		}
		return a.result(action, core.CapabilityPartial, "OpenCode config missing or invalid: "+err.Error())
	}
	currentHash := hashString(string(plugin))
	if cfg.InstalledHash != currentHash {
		return a.result(action, core.CapabilityRestartRequired, "OpenCode plugin artifact changed; restart OpenCode after reinstall")
	}
	active, err := a.readActiveMarker()
	if err != nil {
		return a.result(action, core.CapabilityRestartRequired, "OpenCode restart required to load plugin version "+cfg.InstalledVersion)
	}
	if a.activeMarkerOlderThanInstalledConfig() {
		return a.result(action, core.CapabilityRestartRequired, "OpenCode active marker predates installed plugin/config; restart OpenCode to load current runtime bridge")
	}
	if active.ActiveVersion != cfg.InstalledVersion {
		return a.result(action, core.CapabilityRestartRequired, "OpenCode active plugin version mismatch; restart OpenCode to load "+cfg.InstalledVersion)
	}
	if active.ActiveHash != currentHash {
		return a.result(action, core.CapabilityRestartRequired, "OpenCode active plugin hash mismatch; restart OpenCode to load current plugin")
	}
	if active.ActivePromptConfigHash != cfg.PromptConfigHash {
		return a.result(action, core.CapabilityRestartRequired, "OpenCode active prompt config mismatch; restart OpenCode to load current prompt config")
	}
	if active.PluginPath != cfg.PluginPath {
		return a.result(action, core.CapabilityRestartRequired, "OpenCode active plugin path mismatch; restart OpenCode to load "+cfg.PluginPath)
	}
	if active.ConfigRoot != "" && active.ConfigRoot != cfg.PluginConfigRoot {
		return a.result(action, core.CapabilityRestartRequired, "OpenCode active config root mismatch; restart OpenCode to load "+cfg.PluginConfigRoot)
	}
	return a.result(action, core.CapabilitySupported, "OpenCode plugin active with version "+cfg.InstalledVersion+" and hash "+currentHash)
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

func (a OpenCodeAdapter) validateRoot() error {
	if a.root == "" {
		return fmt.Errorf("OpenCode config root could not be resolved; set HOME or XDG_CONFIG_HOME")
	}
	if !filepath.IsAbs(a.root) {
		return fmt.Errorf("OpenCode config root must be absolute, got %q", a.root)
	}
	return nil
}

func (a OpenCodeAdapter) result(action core.Action, status core.CapabilityStatus, message string) core.LifecycleResult {
	return core.NewLifecycleResult(core.TargetOpenCode, action, status, message, nil)
}
