package runtime

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
)

// longtermMemObservation is what LongtermMemAdapter can determine about one
// runtime purely by read-only inspection: is the shared binary executable,
// and does that runtime's own MCP config already carry a longterm-mem
// entry (and if so, its fingerprint). Gathering these here keeps every
// os.ReadFile/os.Stat call in one place, isolated from the pure status
// matrix in longtermmem.go.
type longtermMemObservation struct {
	rootResolvable   bool
	binaryPresent    bool
	entryPresent     bool
	entryFingerprint string
}

func (a LongtermMemAdapter) observeAllTargets() map[string]longtermMemObservation {
	binaryPresent := a.binaryExecutable()
	return map[string]longtermMemObservation{
		string(TargetClaude):   a.observeClaude(binaryPresent),
		string(TargetOpenCode): a.observeOpenCode(binaryPresent),
		string(TargetCodex):    a.observeCodex(binaryPresent),
	}
}

func (a LongtermMemAdapter) binaryExecutable() bool {
	if a.BinaryPath == "" {
		return false
	}
	info, err := os.Stat(a.BinaryPath)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0o111 != 0
}

func (a LongtermMemAdapter) observeClaude(binaryPresent bool) longtermMemObservation {
	obs := longtermMemObservation{binaryPresent: binaryPresent}
	if a.ClaudeConfigPath == "" {
		return obs
	}
	obs.rootResolvable = true
	present, fingerprint, err := jsonMemberFingerprint(a.ClaudeConfigPath, "mcpServers", "longterm-mem")
	if err != nil {
		return obs
	}
	obs.entryPresent = present
	obs.entryFingerprint = fingerprint
	return obs
}

func (a LongtermMemAdapter) observeOpenCode(binaryPresent bool) longtermMemObservation {
	obs := longtermMemObservation{binaryPresent: binaryPresent}
	if a.OpenCodeConfigPath == "" {
		return obs
	}
	obs.rootResolvable = true
	present, fingerprint, err := jsonMemberFingerprint(a.OpenCodeConfigPath, "mcp", "longterm-mem")
	if err != nil {
		return obs
	}
	obs.entryPresent = present
	obs.entryFingerprint = fingerprint
	return obs
}

func (a LongtermMemAdapter) observeCodex(binaryPresent bool) longtermMemObservation {
	obs := longtermMemObservation{binaryPresent: binaryPresent}
	if a.CodexConfigPath == "" {
		return obs
	}
	obs.rootResolvable = true
	present, fingerprint, err := tomlSectionFingerprint(a.CodexConfigPath, "longterm-mem")
	if err != nil {
		return obs
	}
	obs.entryPresent = present
	obs.entryFingerprint = fingerprint
	return obs
}

// jsonMemberFingerprint reads a JSON config file (claude/opencode's own MCP
// registry) and looks for object[section][key]. Re-marshaling the decoded
// value is deterministic — encoding/json always sorts map keys — so the
// fingerprint is stable regardless of the original file's key order.
// A missing file is not an error: the register step may simply not have
// run yet for this target.
func jsonMemberFingerprint(path, section, key string) (present bool, fingerprint string, err error) {
	obj, err := readJSONObjectOrNil(path)
	if err != nil {
		return false, "", err
	}
	if obj == nil {
		return false, "", nil
	}
	sectionMap, ok := obj[section].(map[string]interface{})
	if !ok {
		return false, "", nil
	}
	entry, ok := sectionMap[key]
	if !ok {
		return false, "", nil
	}
	encoded, err := json.Marshal(entry)
	if err != nil {
		return false, "", err
	}
	return true, hashString(string(encoded)), nil
}

// readJSONObjectOrNil reads a JSON file and decodes it as a generic object.
// A not-exist file returns (nil, nil) rather than an error — shared by
// ClaudeAdapter.status() (10a.9: the exact same "read file, tolerate
// absence, decode to map" shape) and LongtermMemAdapter's claude/opencode
// inspection.
func readJSONObjectOrNil(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, err
	}
	return obj, nil
}

// codexSectionHeader matches a codex config.toml table header for the
// longterm-mem MCP server section, e.g. [mcp_servers.longterm-mem] or
// [mcp_servers."longterm-mem"] (12a.1's header regex, restated here
// read-only: engine never writes TOML, it only scans for its own status
// reporting, per the hard "stdlib only, no TOML library" constraint).
var codexSectionHeader = regexp.MustCompile(`^\s*\[mcp_servers\.(?:"longterm-mem"|longterm-mem)\]\s*$`)

// codexHeaderPattern matches ANY table header, used to find where the
// longterm-mem section ends.
var codexHeaderPattern = regexp.MustCompile(`^\s*\[`)

// codexCommandLine matches a `command = ` line inside the section, which
// must be present for the section to count as a real entry rather than a
// header with no body.
var codexCommandLine = regexp.MustCompile(`^\s*command\s*=`)

// tomlSectionFingerprint scans a codex config.toml for the longterm-mem
// table via a header/`command =` line scan (10a.5) — no TOML library, per
// the zero-dependency constraint. A missing file is not an error.
func tomlSectionFingerprint(path, name string) (present bool, fingerprint string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, "", nil
		}
		return false, "", err
	}
	lines := strings.Split(string(data), "\n")

	headerIdx := -1
	for i, line := range lines {
		if codexSectionHeader.MatchString(line) {
			headerIdx = i
			break
		}
	}
	if headerIdx == -1 {
		return false, "", nil
	}

	end := len(lines)
	hasCommand := false
	for i := headerIdx + 1; i < len(lines); i++ {
		if codexHeaderPattern.MatchString(lines[i]) {
			end = i
			break
		}
		if codexCommandLine.MatchString(lines[i]) {
			hasCommand = true
		}
	}
	if !hasCommand {
		// A header with no command line is not a real entry — nothing a
		// register step would have written.
		return false, "", nil
	}

	section := strings.TrimRight(strings.Join(lines[headerIdx:end], "\n"), "\n")
	return true, hashString(section), nil
}
