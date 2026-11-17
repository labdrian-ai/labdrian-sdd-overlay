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
const OpenCodeMemorycardPluginVersion = "2026-11-17-memorycard-2"

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
	PluginConfigRoot  string `json:"plugin_config_root"`
	MemorycardCommand string `json:"memorycard_command"`
}

// openCodeMemorycardMarker is what the plugin writes when it loads.
type openCodeMemorycardMarker struct {
	ActiveVersion string `json:"active_version"`
	ActiveHash    string `json:"active_hash"`
	PluginPath    string `json:"plugin_path"`
	ConfigRoot    string `json:"config_root"`
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

// readMemorycardPlugin reads the file at the plugin path; exists is false when there is none.
func (a OpenCodeAdapter) readMemorycardPlugin() (content []byte, exists bool, err error) {
	content, err = os.ReadFile(a.memorycardPluginPath())
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	return content, err == nil, err
}

// isMemorycardPluginOurs says whether this adapter wrote the file: the mark is in its first line, as the
// plugin header has it, so a file that quotes the mark further down is not ours.
func isMemorycardPluginOurs(content []byte) bool {
	first, _, _ := strings.Cut(string(content), "\n")
	return strings.Contains(first, openCodeMemorycardPluginMark)
}

// writeMemorycardPlugin installs the plugin and its record, and says what it did. A file at the plugin
// path that this adapter did not write is left as it is, and the result is partial.
func (a OpenCodeAdapter) writeMemorycardPlugin() (message string, partial bool) {
	path := a.memorycardPluginPath()
	current, exists, err := a.readMemorycardPlugin()
	if err != nil {
		return "memorycard plugin not written: " + err.Error(), true
	}
	if exists && !isMemorycardPluginOurs(current) {
		return "memorycard plugin not written: " + path + " exists and was not written by this adapter; move it away and run update", true
	}
	record := openCodeMemorycardRecord{
		PluginPath:        path,
		InstalledHash:     OpenCodeMemorycardPluginHash(),
		InstalledVersion:  OpenCodeMemorycardPluginVersion,
		PluginConfigRoot:  a.root,
		MemorycardCommand: a.memorycardProgram(),
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err == nil && exists && string(current) == openCodeMemorycardPluginSource {
		if have, readErr := os.ReadFile(a.memorycardConfigPath()); readErr == nil && string(have) == string(data) {
			return fmt.Sprintf("memorycard plugin already current in %s (version %s)", path, OpenCodeMemorycardPluginVersion), false
		}
	}
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
// anything else in the directory. It says whether it removed the plugin, and whether it left a file that
// is not ours.
func (a OpenCodeAdapter) removeMemorycardPlugin() (removed, foreignLeft bool, err error) {
	content, exists, err := a.readMemorycardPlugin()
	if err != nil {
		return false, false, err
	}
	if exists && isMemorycardPluginOurs(content) {
		if err := os.Remove(a.memorycardPluginPath()); err != nil && !os.IsNotExist(err) {
			return false, false, err
		}
		removed = true
	}
	// os.Remove refuses a directory that is not empty: that one is not ours to remove.
	_ = os.Remove(filepath.Dir(a.memorycardPluginPath()))
	if err := os.Remove(a.memorycardConfigPath()); err != nil && !os.IsNotExist(err) {
		return removed, false, err
	}
	return removed, exists && !removed, nil
}

// memorycardUninstallNote is the sentence the uninstall message gains: the active marker that stays when
// the plugin was removed, and the file left in place when it was not ours.
func (a OpenCodeAdapter) memorycardUninstallNote(removed, foreignLeft bool) string {
	var note string
	if removed {
		note += "; the memorycard plugin's active marker remains at " + a.memorycardActiveMarkerPath()
	}
	if foreignLeft {
		note += "; " + a.memorycardPluginPath() + " was not written by this adapter and was left in place"
	}
	return note
}

// memorycardPluginProblem checks the installed plugin against its record. It says what is wrong, and
// whether the result is partial (the install is missing, foreign or stale) or a restart owed (the file
// is not the one the record names).
func (a OpenCodeAdapter) memorycardPluginProblem() (restartOnly bool, message string, bad bool) {
	content, exists, err := a.readMemorycardPlugin()
	path := a.memorycardPluginPath()
	switch {
	case err != nil:
		return false, "OpenCode memorycard plugin unreadable: " + err.Error(), true
	case !exists:
		return false, "OpenCode memorycard plugin not installed at " + path + "; run update", true
	case !isMemorycardPluginOurs(content):
		return false, "OpenCode memorycard plugin at " + path + " was not written by this adapter", true
	}
	record, err := a.readMemorycardRecord()
	if err != nil {
		return false, "OpenCode memorycard plugin record " + a.memorycardConfigPath() + " is missing or stale: " + err.Error() + "; run update", true
	}
	if record.InstalledHash != sha256hex.Sum(content) {
		return true, "OpenCode memorycard plugin artifact changed; restart OpenCode after reinstall", true
	}
	return false, "", false
}

// readMemorycardRecord reads the record and proves it current: the version and hash of the source the
// program carries, and the paths.
func (a OpenCodeAdapter) readMemorycardRecord() (openCodeMemorycardRecord, error) {
	var record openCodeMemorycardRecord
	data, err := os.ReadFile(a.memorycardConfigPath())
	if err != nil {
		return record, err
	}
	if err := json.Unmarshal(data, &record); err != nil {
		return record, err
	}
	switch {
	case record.InstalledVersion != OpenCodeMemorycardPluginVersion:
		return record, fmt.Errorf("installed_version %q is not current %q", record.InstalledVersion, OpenCodeMemorycardPluginVersion)
	case record.InstalledHash == "":
		return record, fmt.Errorf("installed_hash is empty")
	case record.PluginPath != a.memorycardPluginPath():
		return record, fmt.Errorf("plugin_path does not match configured OpenCode root")
	case record.PluginConfigRoot != a.root:
		return record, fmt.Errorf("plugin_config_root does not match configured OpenCode root")
	}
	return record, nil
}

// memorycardActiveProblem says whether OpenCode has loaded the installed memorycard plugin: its active
// marker must exist, be newer than the plugin and its record, and name the installed version, file and root.
func (a OpenCodeAdapter) memorycardActiveProblem() (message string, bad bool) {
	restart := "OpenCode restart required to load the memorycard plugin version " + OpenCodeMemorycardPluginVersion
	data, err := os.ReadFile(a.memorycardActiveMarkerPath())
	if err != nil {
		return restart, true
	}
	var marker openCodeMemorycardMarker
	if err := json.Unmarshal(data, &marker); err != nil {
		return restart, true
	}
	if marker.ActiveVersion != OpenCodeMemorycardPluginVersion {
		return "OpenCode active memorycard plugin version mismatch; restart OpenCode to load " + OpenCodeMemorycardPluginVersion, true
	}
	if info, err := os.Stat(a.memorycardActiveMarkerPath()); err == nil {
		for _, path := range []string{a.memorycardPluginPath(), a.memorycardConfigPath()} {
			if changed, err := os.Stat(path); err == nil && info.ModTime().Before(changed.ModTime()) {
				return "OpenCode active memorycard plugin marker predates the installed plugin; restart OpenCode to load it", true
			}
		}
	}
	if marker.ActiveHash != OpenCodeMemorycardPluginHash() || marker.PluginPath != a.memorycardPluginPath() || marker.ConfigRoot != a.root {
		return "OpenCode active memorycard plugin does not match the installed one; restart OpenCode to load it", true
	}
	return "", false
}
