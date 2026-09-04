package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// longtermMemObservation is what LongtermMemAdapter can determine about one
// runtime purely by read-only inspection: is the shared binary executable,
// is that runtime installed on this machine at all, and does its own MCP
// config already carry a longterm-mem entry (and if so, its fingerprint and
// whether this overlay is the one that wrote it). Gathering these here keeps
// every os.ReadFile/os.Stat call in one place, isolated from the pure status
// matrix in longtermmem_status.go.
type longtermMemObservation struct {
	rootResolvable bool
	binaryPresent  bool
	// runtimePresent is whether this runtime's own config file exists on
	// disk, decided by os.Stat — never by whether the path STRING is
	// non-empty, and never by whether the file parses.
	runtimePresent bool
	entryPresent   bool
	// entryOwned is whether the observed entry is one this overlay wrote.
	// It is strictly narrower than entryPresent, and the two must never be
	// conflated: a foreign entry is PRESENT but NOT OWNED, and folding
	// ownership into presence would make it vanish from the status matrix
	// entirely (reported as a healthy "not registered") instead of being
	// reported as the unmanaged entry it is.
	entryOwned       bool
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
	obs.runtimePresent = runtimeConfigFileExists(a.ClaudeConfigPath)
	present, fingerprint, err := jsonMemberFingerprint(a.ClaudeConfigPath, "mcpServers", "longterm-mem")
	if err != nil {
		return obs
	}
	obs.entryPresent = present
	obs.entryFingerprint = fingerprint
	obs.entryOwned = present && fingerprint == a.ownedClaudeFingerprint()
	return obs
}

func (a LongtermMemAdapter) observeOpenCode(binaryPresent bool) longtermMemObservation {
	obs := longtermMemObservation{binaryPresent: binaryPresent}
	if a.OpenCodeConfigPath == "" {
		return obs
	}
	obs.rootResolvable = true
	obs.runtimePresent = runtimeConfigFileExists(a.OpenCodeConfigPath)
	present, fingerprint, err := jsonMemberFingerprint(a.OpenCodeConfigPath, "mcp", "longterm-mem")
	if err != nil {
		return obs
	}
	obs.entryPresent = present
	obs.entryFingerprint = fingerprint
	obs.entryOwned = present && fingerprint == a.ownedOpenCodeFingerprint()
	return obs
}

func (a LongtermMemAdapter) observeCodex(binaryPresent bool) longtermMemObservation {
	obs := longtermMemObservation{binaryPresent: binaryPresent}
	if a.CodexConfigPath == "" {
		return obs
	}
	obs.rootResolvable = true
	obs.runtimePresent = runtimeConfigFileExists(a.CodexConfigPath)
	present, fingerprint, err := tomlSectionFingerprint(a.CodexConfigPath, "longterm-mem")
	if err != nil {
		return obs
	}
	obs.entryPresent = present
	obs.entryFingerprint = fingerprint
	obs.entryOwned = present && fingerprint == a.ownedCodexFingerprint()
	return obs
}

// runtimeConfigFileExists answers "is this runtime installed on this
// machine" the only way that is actually evidence: by stat-ing its own
// config file.
//
// Two things it deliberately does NOT do. It does not look at whether the
// path STRING is empty — that only says HOME resolved, a machine-wide fact
// shared by all three runtimes, and reading it as a per-runtime signal is
// what made an absent runtime indistinguishable from a present one. And it
// does not care whether the file parses: an unreadable or malformed config
// is emphatically not proof the runtime is missing.
//
// Only a definite os.IsNotExist counts as absent. Any other stat failure
// (a permission error, a dangling symlink, an I/O error) means absence
// could not be proven, and reporting "runtime not installed" on that basis
// would be a guess stated as fact.
func runtimeConfigFileExists(path string) bool {
	if path == "" {
		return false
	}
	if _, err := os.Stat(path); err != nil {
		return !os.IsNotExist(err)
	}
	return true
}

// --- ownership (A2) ---
//
// `longterm-mem register` marks NO ownership tag inside a runtime's own
// config file. Its sidecar install-state.json is explicit about this:
// "This is the ONLY place longterm-mem's ownership fingerprint lives —
// never inside a runtime's own config file, so that runtime's schema never
// carries an unknown key it doesn't recognize" (R-016, R-017). There is
// therefore no marker to read, and none is invented here.
//
// What the engine has instead is the one thing that distinguishes the
// register package's own after-install fixtures from its untagged ones: the
// entry's exact shape, naming the binary THIS adapter installs. The engine
// re-derives ownership by rebuilding the entry `register` would write for
// a.BinaryPath and comparing it against what was observed — exactly the
// "re-derives the same state from its own scan of each runtime's config"
// contract the register package's own doc.go names for this adapter.
//
// This is deliberately conservative in one direction only: an entry we
// cannot prove is ours is reported as unmanaged, never adopted. The cost is
// that an entry longterm-mem wrote and a human then reformatted stops
// looking owned; the benefit is that a third party's server can never be
// recorded as this overlay's own, which is the defect this replaces.

// ownedClaudeEntry mirrors register.claudeEntry — the exact shape
// RegisterClaude writes into mcpServers.longterm-mem.
type ownedClaudeEntry struct {
	Type    string   `json:"type"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// ownedOpenCodeEntry mirrors register.opencodeEntry — opencode's own
// single-argument-list command shape, unlike claude's command/args split.
type ownedOpenCodeEntry struct {
	Type    string   `json:"type"`
	Command []string `json:"command"`
	Enabled bool     `json:"enabled"`
}

// ownedCodexSectionTemplate mirrors register.codexSectionTemplate.
const ownedCodexSectionTemplate = "[mcp_servers.%s]\ncommand = %q\nargs = [\"mcp\"]\n"

func (a LongtermMemAdapter) ownedClaudeFingerprint() string {
	return a.ownedJSONFingerprint(ownedClaudeEntry{Type: "stdio", Command: a.BinaryPath, Args: []string{"mcp"}})
}

func (a LongtermMemAdapter) ownedOpenCodeFingerprint() string {
	return a.ownedJSONFingerprint(ownedOpenCodeEntry{Type: "local", Command: []string{a.BinaryPath, "mcp"}, Enabled: true})
}

// ownedCodexFingerprint mirrors tomlSectionFingerprint's own hashed value:
// the section text with trailing newlines trimmed. It matches only the
// unquoted `[mcp_servers.longterm-mem]` header form, which is the one
// register writes; a hand-quoted header is a config we did not write.
func (a LongtermMemAdapter) ownedCodexFingerprint() string {
	if a.BinaryPath == "" {
		return ""
	}
	section := fmt.Sprintf(ownedCodexSectionTemplate, "longterm-mem", a.BinaryPath)
	return hashString(strings.TrimRight(section, "\n"))
}

// ownedJSONFingerprint routes the expected entry through the SAME
// decode-then-remarshal canonicalization jsonMemberFingerprint applies to
// what it reads, so the two fingerprints are comparable: a struct marshals
// in field order, a decoded generic value marshals in sorted-key order, and
// comparing one against the other would never match.
//
// An empty BinaryPath yields no fingerprint at all rather than one for the
// entry `{"command":""}`, so an adapter that could not resolve its binary
// path can never accidentally match anything.
func (a LongtermMemAdapter) ownedJSONFingerprint(entry interface{}) string {
	if a.BinaryPath == "" {
		return ""
	}
	encoded, err := json.Marshal(entry)
	if err != nil {
		return ""
	}
	var decoded interface{}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return ""
	}
	canonical, err := json.Marshal(decoded)
	if err != nil {
		return ""
	}
	return hashString(string(canonical))
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
