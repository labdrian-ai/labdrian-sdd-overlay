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

// OpenCodePluginVersion is the version of the plugin the adapter installs; it changes with the
// plugin source and is what the active marker is compared against.
const OpenCodePluginVersion = "2026-11-16-runtime-parity-6"

//go:embed labdrian-runtime-parity-plugin.mjs
var openCodePluginSource string

// OpenCodePluginHash is the hash of the plugin source the program carries.
func OpenCodePluginHash() string {
	return sha256hex.Sum([]byte(openCodePluginSource))
}
