package register

import (
	"fmt"
	"path/filepath"
)

// codexConfigFileName is codex's own config file name, living directly
// inside its config root ($CODEX_HOME or ~/.codex — resolved by the
// caller, cmd_register.go, not by this package).
const codexConfigFileName = "config.toml"

// codexTableKey is the top-level table codex's MCP servers live under.
const codexTableKey = "mcp_servers"

// codexTarget is install-state's key for this runtime.
const codexTarget = "codex"

// codexSectionTemplate is the exact shape longterm-mem writes for
// mcp_servers.longterm-mem (R-018): a [mcp_servers.longterm-mem] table
// with command and args keys, mirroring engine/runtime's own read-only
// fixture shape (engine/runtime/longtermmem_test.go) byte for byte, so a
// human comparing what `register` writes against what `doctor`/`status`
// expect to read sees the same file.
const codexSectionTemplate = "[mcp_servers.%s]\ncommand = %q\nargs = [\"mcp\"]\n"

// RegisterCodex installs (or reinstalls) the ownership-tagged longterm-mem
// MCP server table into codex's configuration at configRoot/config.toml
// (R-018), recording the section's fingerprint under
// stateDir/install-state.json. See tomlInstall for the shared D9 decision
// flow codex's TOML writer follows, mirroring jsonInstall's own contract:
// unrelated sections and top-level keys are left byte-identical, a
// reinstall replaces the existing tagged section in place, and an untagged
// same-named section is refused with ErrConflict rather than overwritten.
func RegisterCodex(configRoot, stateDir, binary string) error {
	configPath := filepath.Join(configRoot, codexConfigFileName)
	newSection := []byte(fmt.Sprintf(codexSectionTemplate, "longterm-mem", binary))
	return tomlInstall(codexTarget, configPath, stateDir, codexTableKey, "longterm-mem", binary, newSection)
}
