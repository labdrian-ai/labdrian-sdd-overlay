package main

// The 'merge-settings' and 'uninstall-hooks' subcommands: install and remove the hook entries of a Claude Code settings.json.

import (
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings/settingsfile"
)

// parseMergeSettingsArgs extracts --settings and --hook-command from args.
func parseMergeSettingsArgs(args []string) (settingsPath, hookCommand string) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--settings":
			i++
			if i < len(args) {
				settingsPath = args[i]
			}
		case "--hook-command":
			i++
			if i < len(args) {
				hookCommand = args[i]
			}
		}
	}
	return
}

// runMergeSettings implements the 'merge-settings' subcommand.
// Fails LOUD on any error (exits 1).
func runMergeSettings(p process, args []string) {
	settingsPath, hookCommand := parseMergeSettingsArgs(args)

	if settingsPath == "" {
		fmt.Fprintln(p.stderr, "error: --settings is required")
		p.exit(1)
		return
	}
	if hookCommand == "" {
		fmt.Fprintln(p.stderr, "error: --hook-command is required")
		p.exit(1)
		return
	}

	if err := (settingsfile.Installer{}).Install(settingsPath, hookCommand); err != nil {
		fmt.Fprintf(p.stderr, "error: merge-settings: %v\n", err)
		p.exit(1)
		return
	}
	fmt.Fprintln(p.stdout, "merge-settings: hooks installed successfully")
	noticeForeignMainAgent(p, settingsPath)
}

// noticeForeignMainAgent says on stderr when the settings hold an "agent" key with a value other than
// the foreman's: the install leaves it as it is, so the foreman main session is not enabled, and a
// silent success would hide that. The install still succeeds (exit 0). A file that cannot be read
// again says nothing: the install has just read and written it.
func noticeForeignMainAgent(p process, settingsPath string) {
	doc, _, err := (settingsfile.File{Path: settingsPath}).Read()
	if err != nil || settings.MainAgent(doc.Root()) != settings.MainAgentConflict {
		return
	}
	fmt.Fprintf(p.stderr, "notice: settings.json holds \"agent\": %s; it was left as it is, so the foreman main session was not enabled (set it to %q or remove the key, then run install-hooks again)\n",
		settings.MainAgentValueJSON(doc.Root()), settings.MainAgentName)
}

// runUninstallHooks implements the 'uninstall-hooks' subcommand.
// Fails LOUD on any error (exits 1).
func runUninstallHooks(p process, args []string) {
	settingsPath, hookCommand := parseMergeSettingsArgs(args)

	if settingsPath == "" {
		fmt.Fprintln(p.stderr, "error: --settings is required")
		p.exit(1)
		return
	}
	if hookCommand == "" {
		fmt.Fprintln(p.stderr, "error: --hook-command is required")
		p.exit(1)
		return
	}

	if err := (settingsfile.Installer{}).Uninstall(settingsPath, hookCommand); err != nil {
		fmt.Fprintf(p.stderr, "error: uninstall-hooks: %v\n", err)
		p.exit(1)
		return
	}
	fmt.Fprintln(p.stdout, "uninstall-hooks: hooks removed successfully")
}
