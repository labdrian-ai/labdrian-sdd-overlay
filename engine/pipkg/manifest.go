package pipkg

import (
	_ "embed"
)

// gateExtensionSource is the exact bytes shipped as
// extensions/labdrian-gate.ts inside the built package (slice 3,
// pi-contract-gate, R-004). Embedding rather than a string literal keeps a
// single source of truth: the file this constant embeds is the same one
// the node-driven Go tests and Pi's own jiti loader execute.
//
//go:embed labdrian-gate.ts
var gateExtensionSource string

// gateContractFiles are the managed contracts labdrian-gate.ts reads at
// runtime (its CONTRACT_RELATIVE_PATHS), copied from overlayRoot's
// skills/_shared/ into the built package's skills/_shared/ (R-004/R-007):
// these two files are not registry-driven, since they are gate
// infrastructure rather than an installable skill.
var gateContractFiles = []string{"minimalism-contract.md", "anti-generic-design.md"}

// GateExtensionSource returns the exact embedded labdrian-gate.ts bytes
// this build ships, for tests that need to run the real source under Node
// without duplicating it.
func GateExtensionSource() string { return gateExtensionSource }

// packageManifest is the subset of package.json fields this package writes.
type packageManifest struct {
	Name     string         `json:"name"`
	Version  string         `json:"version"`
	Pi       piField        `json:"pi"`
	Labdrian *labdrianField `json:"labdrian,omitempty"`
}

// labdrianField is the "labdrian" key inside package.json (R-005): the
// resolved source commit this package was built from, so a later
// sync-check can compare against that exact ref instead of whatever branch
// happens to be checked out (#315).
type labdrianField struct {
	BuiltFrom string `json:"builtFrom,omitempty"`
}

// piField is the "pi" key inside package.json. MCP is a package-relative
// path to mcp.json (A1, R-005) — pi-mcp-adapter reads that file's own
// top-level mcpServers object, not an inline value here.
type piField struct {
	Skills     []string `json:"skills"`
	Agents     []string `json:"agents"`
	Extensions []string `json:"extensions"`
	MCP        string   `json:"mcp"`
}

// mcpConfigFileName is the file package.json's "pi":{"mcp":...} points at
// (A1). registerPi (longterm-mem/internal/register) writes
// mcpServers.longterm-mem into this exact file; Build never overwrites an
// existing one's content (see Build's own doc comment) because that
// registration is state longterm-mem owns, not build output.
const mcpConfigFileName = "mcp.json"

// mcpSkeleton is the empty mcp.json Build ships for a package that has
// never been registered against yet.
const mcpSkeleton = "{\"mcpServers\": {}}\n"

// mcpConfigBakFileName is the backup sibling jsonInstall (the writer
// `longterm-mem register --target pi` uses) writes beside mcp.json on any
// content-changing register/unregister call. It is registration state the
// same as mcp.json itself (Build's own doc comment), so Build preserves it
// across a rebuild and Check excludes it from its content diff, exactly as
// both already do for mcp.json -- otherwise every documented register call
// would show as permanent drift.
const mcpConfigBakFileName = mcpConfigFileName + ".bak"
