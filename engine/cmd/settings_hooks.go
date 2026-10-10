package main

// The 'merge-settings' and 'uninstall-hooks' subcommands: install and remove the hook entries of a Claude Code settings.json.

import (
	"fmt"
	"os"

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
func runMergeSettings(args []string) {
	settingsPath, hookCommand := parseMergeSettingsArgs(args)

	if settingsPath == "" {
		fmt.Fprintln(os.Stderr, "error: --settings is required")
		os.Exit(1)
	}
	if hookCommand == "" {
		fmt.Fprintln(os.Stderr, "error: --hook-command is required")
		os.Exit(1)
	}

	if err := (settingsfile.Installer{}).Install(settingsPath, hookCommand); err != nil {
		fmt.Fprintf(os.Stderr, "error: merge-settings: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stdout, "merge-settings: hooks installed successfully")
}

// runUninstallHooks implements the 'uninstall-hooks' subcommand.
// Fails LOUD on any error (exits 1).
func runUninstallHooks(args []string) {
	settingsPath, hookCommand := parseMergeSettingsArgs(args)

	if settingsPath == "" {
		fmt.Fprintln(os.Stderr, "error: --settings is required")
		os.Exit(1)
	}
	if hookCommand == "" {
		fmt.Fprintln(os.Stderr, "error: --hook-command is required")
		os.Exit(1)
	}

	if err := (settingsfile.Installer{}).Uninstall(settingsPath, hookCommand); err != nil {
		fmt.Fprintf(os.Stderr, "error: uninstall-hooks: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stdout, "uninstall-hooks: hooks removed successfully")
}
