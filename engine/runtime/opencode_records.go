package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/opencodeprompt"
)

// openCodeConfig is the record the adapter writes beside the plugin: what it installed, where,
// and the prompt config the plugin was given. The plugin reads it at load time and Status checks
// it against the current plugin and contracts.
type openCodeConfig struct {
	// InstalledHash is the plugin artifact hash last written by the adapter.
	// InstalledVersion is the deterministic plugin version last installed.
	PluginPath        string                      `json:"plugin_path"`
	InstalledHash     string                      `json:"installed_hash"`
	InstalledVersion  string                      `json:"installed_version"`
	ActivationMarker  string                      `json:"activation_marker"`
	PluginConfigRoot  string                      `json:"plugin_config_root"`
	PluginConfigScope string                      `json:"plugin_config_scope"`
	PromptConfig      opencodeprompt.PromptConfig `json:"prompt_config"`
	PromptConfigHash  string                      `json:"prompt_config_hash"`
}

// openCodeActiveMarker is the record the plugin writes when it loads: which version, hash and
// prompt config it is running. Status compares it with the installed record to tell whether a
// restart is owed.
type openCodeActiveMarker struct {
	ActiveVersion          string `json:"active_version"`
	ActiveHash             string `json:"active_hash"`
	ActivePromptConfigHash string `json:"active_prompt_config_hash"`
	PluginPath             string `json:"plugin_path"`
	ConfigRoot             string `json:"config_root"`
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

// readConfig reads the installed record and proves it current: the plugin version and hash, the
// paths, and the prompt config against the one the contracts give today. A prompt config that is
// not current comes back as an opencodeprompt.MismatchError, which Status reports as a restart
// owed; anything else is a record that is missing or invalid.
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
	current, err := a.promptConfig()
	if err != nil {
		return openCodeConfig{}, fmt.Errorf("current prompt_config could not be derived: %w", err)
	}
	if err := opencodeprompt.Verify(cfg.PromptConfig, cfg.PromptConfigHash, current); err != nil {
		return openCodeConfig{}, err
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

func (a OpenCodeAdapter) activeMarkerExists() bool {
	_, err := os.Stat(a.activeMarkerPath())
	return err == nil
}

// activeMarkerOlderThanInstalledConfig says whether the plugin or its record was written after the
// plugin last loaded, which means the loaded plugin is not the installed one.
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
