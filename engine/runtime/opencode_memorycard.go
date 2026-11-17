package runtime

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sha256hex"
)

// The memorycard plugin is the second plugin of the OpenCode adapter (S11, L84): it hands the
// memorycard context to a new session and each subagent's final text to `memorycard capture`. It is
// installed beside the runtime-parity plugin at <config>/plugins/memorycard/index.js, and it has a
// record and an active marker of its own, so that either plugin can change without the other.
const (
	openCodeMemorycardPluginID    = "memorycard"
	openCodeMemorycardConfigFile  = "memorycard-opencode.json"
	openCodeMemorycardActiveFile  = "memorycard-opencode.active.json"
	openCodeMemorycardProgramName = "memorycard"
	// openCodeMemorycardPluginMark is in the first line of the plugin source: a file at the plugin path
	// that does not hold it was not written by this adapter, and the adapter leaves it as it is.
	openCodeMemorycardPluginMark = "labdrian-memorycard-plugin"
)

// OpenCodeMemorycardPluginVersion is the version of the memorycard plugin the adapter installs; it
// changes with the plugin source.
const OpenCodeMemorycardPluginVersion = "2026-11-17-memorycard-1"

//go:embed memorycard-opencode-plugin.mjs
var openCodeMemorycardPluginSource string

// OpenCodeMemorycardPluginHash is the hash of the memorycard plugin source the program carries.
func OpenCodeMemorycardPluginHash() string {
	return sha256hex.Sum([]byte(openCodeMemorycardPluginSource))
}

// openCodeMemorycardRecord is what the adapter writes beside the memorycard plugin: what it installed,
// where, and the program the plugin runs. The plugin reads it at load time and Status checks it.
type openCodeMemorycardRecord struct {
	PluginPath        string `json:"plugin_path"`
	InstalledHash     string `json:"installed_hash"`
	InstalledVersion  string `json:"installed_version"`
	ActivationMarker  string `json:"activation_marker"`
	PluginConfigRoot  string `json:"plugin_config_root"`
	PluginConfigScope string `json:"plugin_config_scope"`
	MemorycardCommand string `json:"memorycard_command"`
}

func (a OpenCodeAdapter) memorycardPluginPath() string {
	return filepath.Join(a.root, "plugins", openCodeMemorycardPluginID, openCodePluginFile)
}

func (a OpenCodeAdapter) memorycardConfigPath() string {
	return filepath.Join(a.root, openCodeMemorycardConfigFile)
}

func (a OpenCodeAdapter) memorycardActiveMarkerPath() string {
	return filepath.Join(a.root, openCodeMemorycardActiveFile)
}

// memorycardProgram is the program the plugin runs: the absolute path the adapter's lookup finds, as
// the MCP entry gets it (OpenCode may be started from a launcher whose PATH is not the shell's), else
// the bare name.
func (a OpenCodeAdapter) memorycardProgram() string {
	if a.options.PathLookup != nil {
		if program, err := a.options.PathLookup(openCodeMemorycardProgramName); err == nil && filepath.IsAbs(program) {
			return program
		}
	}
	return openCodeMemorycardProgramName
}

// memorycardPluginState says whether a file is at the plugin path and whether this adapter wrote it.
func (a OpenCodeAdapter) memorycardPluginState() (content []byte, exists, ours bool, err error) {
	content, err = os.ReadFile(a.memorycardPluginPath())
	if os.IsNotExist(err) {
		return nil, false, false, nil
	}
	if err != nil {
		return nil, false, false, err
	}
	return content, true, strings.Contains(string(content), openCodeMemorycardPluginMark), nil
}

// writeMemorycardPlugin installs the plugin and its record, and says what it did. A file at the plugin
// path that this adapter did not write is left as it is, and the result is partial.
func (a OpenCodeAdapter) writeMemorycardPlugin() (message string, partial bool) {
	path := a.memorycardPluginPath()
	_, exists, ours, err := a.memorycardPluginState()
	if err != nil {
		return "memorycard plugin not written: " + err.Error(), true
	}
	if exists && !ours {
		return "memorycard plugin not written: " + path + " exists and was not written by this adapter; move it away and run update", true
	}
	record := openCodeMemorycardRecord{
		PluginPath:        path,
		InstalledHash:     OpenCodeMemorycardPluginHash(),
		InstalledVersion:  OpenCodeMemorycardPluginVersion,
		ActivationMarker:  a.memorycardActiveMarkerPath(),
		PluginConfigRoot:  a.root,
		PluginConfigScope: "global-opencode-config",
		MemorycardCommand: a.memorycardProgram(),
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err == nil {
		err = os.MkdirAll(filepath.Dir(path), 0o755)
	}
	if err == nil {
		err = os.WriteFile(path, []byte(openCodeMemorycardPluginSource), 0o644)
	}
	if err == nil {
		err = os.WriteFile(a.memorycardConfigPath(), data, 0o644)
	}
	if err != nil {
		return "memorycard plugin not written: " + err.Error(), true
	}
	return fmt.Sprintf("memorycard plugin written to %s (version %s)", path, OpenCodeMemorycardPluginVersion), false
}

// removeMemorycardPlugin removes what writeMemorycardPlugin wrote: the plugin when it is ours, its
// record and an empty plugin directory. A file at the plugin path that is not ours stays, and so does
// anything else in the directory.
func (a OpenCodeAdapter) removeMemorycardPlugin() error {
	_, exists, ours, err := a.memorycardPluginState()
	if err != nil {
		return err
	}
	if exists && ours {
		if err := os.Remove(a.memorycardPluginPath()); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	// os.Remove refuses a directory that is not empty: that one is not ours to remove.
	_ = os.Remove(filepath.Dir(a.memorycardPluginPath()))
	if err := os.Remove(a.memorycardConfigPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
