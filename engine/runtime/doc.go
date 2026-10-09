// Package runtime holds the adapters of the runtimes the overlay is applied to (Claude, Codex,
// OpenCode, Pi) and of the longterm-mem component, and the Register function of each. They
// implement the Adapter port of runtime/core and reach the machine: the settings files of Claude
// and Codex, the plugin and records of OpenCode, the `pi` CLI and its files, the MCP registries.
//
// What needs no machine lives next to it. runtime/core is the vocabulary, the port, the Registry,
// the Config and the prompt rules; runtime/opencodeprompt derives and verifies the prompt config
// of the OpenCode plugin from the text of the contracts, which the OpenCode adapter reads. The
// adapters read no environment: the composition root (cmd) reads it once into a core.Config and
// the options of each adapter, builds a core.Registry, and registers the runtimes it ships.
package runtime
