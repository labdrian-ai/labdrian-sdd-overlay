package main

import (
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings/settingsfile"
)

func runCodexHooks(p process, args []string, remove bool) {
	verb := "merge-codex-hooks"
	success := "hooks installed successfully"
	if remove {
		verb = "uninstall-codex-hooks"
		success = "hooks removed successfully"
	}
	path, binary, err := parseCodexHooksArgs(args)
	if err != nil {
		fmt.Fprintf(p.stderr, "error: %s: %v\n", verb, err)
		p.exit(1)
		return
	}
	adapter := settingsfile.CodexHooks{}
	if remove {
		err = adapter.Uninstall(path, binary)
	} else {
		err = adapter.Install(path, binary)
	}
	if err != nil {
		fmt.Fprintf(p.stderr, "error: %s: %v\n", verb, err)
		p.exit(2)
		return
	}
	fmt.Fprintf(p.stdout, "%s: %s\n", verb, success)
}

func parseCodexHooksArgs(args []string) (path, binary string, err error) {
	for i := 0; i < len(args); i++ {
		flag, value, equals := strings.Cut(args[i], "=")
		if flag != "--hooks" && flag != "--hook-command" {
			return "", "", fmt.Errorf("unknown argument %q", args[i])
		}
		if !equals {
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return "", "", fmt.Errorf("%s requires a value", flag)
			}
			i++
			value = args[i]
		}
		if value == "" {
			return "", "", fmt.Errorf("%s requires a value", flag)
		}
		if flag == "--hooks" {
			path = value
		} else {
			binary = value
		}
	}
	if path == "" || binary == "" {
		return "", "", fmt.Errorf("--hooks and --hook-command are required")
	}
	return path, binary, nil
}
