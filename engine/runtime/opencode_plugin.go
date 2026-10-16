package runtime

import (
	_ "embed"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sha256hex"
)

const openCodePluginFile = "labdrian-runtime-parity.js"
const openCodeConfigFile = "labdrian-runtime-parity.json"
const openCodeActiveFile = "labdrian-runtime-parity.active.json"

// OpenCodePluginVersion is the version of the plugin the adapter installs; it changes with the
// plugin source and is what the active marker is compared against.
const OpenCodePluginVersion = "2026-07-08-runtime-parity-4"

//go:embed labdrian-runtime-parity-plugin.mjs
var openCodePluginSource string

// OpenCodePluginHash is the hash of the plugin source the program carries.
func OpenCodePluginHash() string {
	return sha256hex.Sum([]byte(openCodePluginSource))
}
