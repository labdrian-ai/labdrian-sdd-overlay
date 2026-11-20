package runtime

import (
	_ "embed"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sha256hex"
)

// openCodePluginID is the v2 plugin id and the name of the directory the plugin is installed into
// (<config>/plugins/labdrian-runtime-parity/index.js, which OpenCode v2 loads without a build step).
const openCodePluginID = "labdrian-runtime-parity"
const openCodePluginFile = "index.js"
const openCodeConfigFile = "labdrian-runtime-parity.json"
const openCodeActiveFile = "labdrian-runtime-parity.active.json"

// openCodeLegacyPluginExport is the default export of the 1.x plugin file; a file at the legacy
// path that holds it beside the active marker's name was written by this adapter.
const openCodeLegacyPluginExport = "export default async function labdrianRuntimeParityPlugin("

// OpenCodePluginVersion is the version of the plugin the adapter installs; it changes with the
// plugin source and is what the active marker is compared against.
const OpenCodePluginVersion = "2026-11-20-runtime-parity-18"

//go:embed labdrian-runtime-parity-plugin.mjs
var openCodePluginSource string

// OpenCodePluginHash is the hash of the plugin source the program carries.
func OpenCodePluginHash() string {
	return sha256hex.Sum([]byte(openCodePluginSource))
}
