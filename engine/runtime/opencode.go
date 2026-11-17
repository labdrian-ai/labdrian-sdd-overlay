package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/opencodeprompt"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sha256hex"
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

	// AgentsSkillsDir is ~/.agents/skills, the second place OpenCode reads skills from. Empty means
	// none is checked.
	AgentsSkillsDir string

	// PathLookup finds an executable by name, as exec.LookPath does on the PATH the caller chose. The
	// adapter writes the path it finds as the command of an MCP server, because OpenCode may be started
	// from a launcher whose PATH is not the shell's. Nil means no lookup: the bare name is written.
	PathLookup func(name string) (string, error)
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
	_, jsonErr := a.removeFromOpenCodeJSON()
	agentsErr := a.removeAgents()
	pluginErr := os.Remove(a.pluginPath())
	// An empty plugin directory, left by this removal or an earlier one, would be loaded by OpenCode
	// as a plugin with no index.js. os.Remove refuses a directory that is not empty: that one is not
	// ours to remove.
	_ = os.Remove(filepath.Dir(a.pluginPath()))
	legacyErr := a.removeLegacyPlugin()
	memorycardErr := a.removeMemorycardPlugin()
	configErr := os.Remove(a.configPath())
	if pluginErr != nil && !os.IsNotExist(pluginErr) {
		return a.result(action, core.CapabilityPartial, pluginErr.Error())
	}
	if configErr != nil && !os.IsNotExist(configErr) {
		return a.result(action, core.CapabilityPartial, configErr.Error())
	}
	if legacyErr != nil {
		return a.result(action, core.CapabilityPartial, legacyErr.Error())
	}
	if memorycardErr != nil {
		return a.result(action, core.CapabilityPartial, memorycardErr.Error())
	}
	if jsonErr != nil || agentsErr != nil {
		var failed []string
		if jsonErr != nil {
			failed = append(failed, "OpenCode config not cleaned: "+jsonErr.Error())
		}
		if agentsErr != nil {
			failed = append(failed, "OpenCode AGENTS.md not cleaned: "+agentsErr.Error())
		}
		return a.result(action, core.CapabilityPartial, strings.Join(failed, "; "))
	}
	return a.result(action, core.CapabilityRestartRequired, "OpenCode plugin bridge removed; restart OpenCode to unload any already loaded plugin. Active marker remains at "+a.activeMarkerPath()+"; remove it only after OpenCode is fully stopped/restarted; the memorycard plugin's active marker remains at "+a.memorycardActiveMarkerPath())
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
	message := "OpenCode plugin changed; restart OpenCode to load version " + cfg.InstalledVersion
	// The v2 plugin is written, so a restart is owed whatever the cleanup of the 1.x file says.
	if err := a.removeLegacyPlugin(); err != nil {
		message += "; " + err.Error()
	}
	memorycardMessage, memorycardPartial := a.writeMemorycardPlugin()
	message += "; " + memorycardMessage
	// A refused opencode.json does not stop AGENTS.md or the skills report: each says its own result.
	partial := false
	jsonMessage, err := a.writeOpenCodeJSON()
	if err != nil {
		partial = true
		jsonMessage = "OpenCode config not written: " + err.Error()
	}
	message += "; " + jsonMessage
	agentsMessage, agentsPartial := a.writeAgents()
	message += "; " + agentsMessage + a.shadowingSkillsNote()
	if partial || agentsPartial || memorycardPartial {
		return a.result(action, core.CapabilityPartial, message)
	}
	return a.result(action, core.CapabilityRestartRequired, message)
}

// status is the lifecycle status with the note about the labdrian skills that shadow the role agents,
// which is a fact about the machine and so on every outcome.
func (a OpenCodeAdapter) status(action core.Action) core.LifecycleResult {
	result := a.statusOf(action)
	return a.result(action, result.Status, result.Message+a.shadowingSkillsNote())
}

func (a OpenCodeAdapter) statusOf(action core.Action) core.LifecycleResult {
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
	// A file left at the 1.x path fails to load beside the v2 plugin, whoever wrote it.
	if _, err := os.Lstat(a.legacyPluginPath()); err == nil {
		return a.result(action, core.CapabilityPartial, "OpenCode 1.x plugin file remains at "+a.legacyPluginPath()+"; OpenCode v2 cannot load it: run update to remove the one labdrian wrote, or remove it by hand")
	}
	cfg, err := a.readConfig()
	if err != nil {
		if opencodeprompt.IsMismatch(err) {
			return a.result(action, core.CapabilityRestartRequired, "OpenCode prompt_config is stale or tampered; reinstall/update and restart OpenCode: "+err.Error())
		}
		return a.result(action, core.CapabilityPartial, "OpenCode config missing or invalid: "+err.Error())
	}
	currentHash := sha256hex.Sum(plugin)
	if cfg.InstalledHash != currentHash {
		return a.result(action, core.CapabilityRestartRequired, "OpenCode plugin artifact changed; restart OpenCode after reinstall")
	}
	if message, bad := a.openCodeJSONStatus(); bad {
		return a.result(action, core.CapabilityPartial, message)
	}
	if message, bad := a.agentsStatus(); bad {
		return a.result(action, core.CapabilityPartial, message)
	}
	if restartOnly, message, bad := a.memorycardPluginProblem(); bad {
		if restartOnly {
			return a.result(action, core.CapabilityRestartRequired, message)
		}
		return a.result(action, core.CapabilityPartial, message)
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
	if message, bad := a.memorycardActiveProblem(); bad {
		return a.result(action, core.CapabilityRestartRequired, message)
	}
	return a.result(action, core.CapabilitySupported, "OpenCode plugin active with version "+cfg.InstalledVersion+" and hash "+currentHash)
}

func (a OpenCodeAdapter) pluginPath() string {
	return filepath.Join(a.root, "plugins", openCodePluginID, openCodePluginFile)
}

// legacyPluginPath is where the 1.x layout installed the plugin, as a single file.
func (a OpenCodeAdapter) legacyPluginPath() string {
	return filepath.Join(a.root, "plugins", openCodePluginID+".js")
}

// removeLegacyPlugin removes the 1.x plugin file this adapter wrote, which OpenCode v2 would fail to
// load beside the v2 plugin. The file is ours only when it holds the 1.x plugin's default export and names its active marker;
// any other file at that path is left as it is. A missing file is not an error.
func (a OpenCodeAdapter) removeLegacyPlugin() error {
	data, err := os.ReadFile(a.legacyPluginPath())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read legacy OpenCode plugin %s: %w", a.legacyPluginPath(), err)
	}
	if !strings.Contains(string(data), openCodeLegacyPluginExport) || !strings.Contains(string(data), openCodeActiveFile) {
		return nil
	}
	if err := os.Remove(a.legacyPluginPath()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove legacy OpenCode plugin %s: %w", a.legacyPluginPath(), err)
	}
	return nil
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
