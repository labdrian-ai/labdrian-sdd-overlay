package main

import (
	"reflect"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pipkg"
	runtimecore "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings/settingsfile"
)

// optionsRegistry is the registry of the runtimes the program ships, with no adapter ever asked
// to act: parsing a command line only asks it which targets there are.
func optionsRegistry(t *testing.T) *runtimecore.Registry {
	t.Helper()
	registry, err := newRuntimeRegistry(nil, settingsfile.Installer{}, noPi(), noGit(), pipkg.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func TestParseRuntimeArgsFillsTheOptionsOfTheCommandLine(t *testing.T) {
	registry := optionsRegistry(t)
	cases := []struct {
		name string
		args []string
		want runtimeOptions
	}{
		{"an action alone gets the defaults: opencode, the parity component, no root and no state",
			[]string{"status"},
			runtimeOptions{Action: runtimecore.ActionStatus, Target: runtimecore.TargetOpenCode, Component: componentRuntimeParity}},
		{"every option", []string{"install", "--target", "claude", "--config-root", "/r", "--component", "longterm-mem", "--state-dir", "/s"},
			runtimeOptions{Action: runtimecore.ActionInstall, Target: runtimecore.TargetClaude, ConfigRoot: "/r", Component: componentLongtermMem, StateDir: "/s"}},
		{"all is a target", []string{"update", "--target", "all"},
			runtimeOptions{Action: runtimecore.ActionUpdate, Target: runtimecore.TargetAll, Component: componentRuntimeParity}},
		{"a target is trimmed", []string{"uninstall", "--target", "  pi "},
			runtimeOptions{Action: runtimecore.ActionUninstall, Target: runtimecore.TargetPi, Component: componentRuntimeParity}},
		{"the last occurrence of a flag wins", []string{"status", "--target", "claude", "--target", "codex", "--config-root", "/a", "--config-root", "/b"},
			runtimeOptions{Action: runtimecore.ActionStatus, Target: runtimecore.TargetCodex, ConfigRoot: "/b", Component: componentRuntimeParity}},
		{"the order of the flags does not matter", []string{"status", "--state-dir", "/s", "--component", "longterm-mem"},
			runtimeOptions{Action: runtimecore.ActionStatus, Target: runtimecore.TargetOpenCode, Component: componentLongtermMem, StateDir: "/s"}},
		{"a value may look like a flag", []string{"status", "--config-root", "--target"},
			runtimeOptions{Action: runtimecore.ActionStatus, Target: runtimecore.TargetOpenCode, ConfigRoot: "--target", Component: componentRuntimeParity}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseRuntimeArgs(c.args, registry)
			if err != nil {
				t.Fatalf("parseRuntimeArgs(%q) = error %v", c.args, err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("parseRuntimeArgs(%q) = %+v, want %+v", c.args, got, c.want)
			}
		})
	}
}

func TestParseRuntimeArgsRefusesWhatItCannotUseInTheWordsItAlwaysHad(t *testing.T) {
	registry := optionsRegistry(t)
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no action", nil, "error: runtime requires an action"},
		{"a flag where the action goes", []string{"--target", "claude"}, "error: runtime requires an action: status | install | update | uninstall | capabilities"},
		{"an action that does not exist", []string{"restart"}, `error: unknown runtime action "restart"`},
		{"rollback is not offered", []string{"rollback"}, `error: unknown runtime action "rollback"`},
		{"--target with no value", []string{"status", "--target"}, "error: --target requires a value"},
		{"a target nobody registered", []string{"status", "--target", "vim"}, `unknown target "vim"`},
		{"--config-root with no value", []string{"status", "--config-root"}, "error: --config-root requires a value"},
		{"--component with no value", []string{"status", "--component"}, "error: --component requires a value"},
		{"a component that does not exist", []string{"status", "--component", "x"}, `error: unknown --component "x" (expected "runtime-parity" or "longterm-mem")`},
		{"--state-dir with no value", []string{"status", "--state-dir"}, "error: --state-dir requires a value"},
		{"a flag that does not exist", []string{"status", "--unknown"}, `error: unknown flag "--unknown"`},
		{"a word that is not a flag", []string{"status", "extra"}, `error: unexpected runtime argument "extra"`},
		{"a short flag is not a flag of the command", []string{"status", "-t"}, `error: unexpected runtime argument "-t"`},
		{"the flags are read before the action is judged", []string{"restart", "--unknown"}, `error: unknown flag "--unknown"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseRuntimeArgs(c.args, registry)
			if err == nil {
				t.Fatalf("parseRuntimeArgs(%q) = %+v, want the error %q", c.args, got, c.want)
			}
			if err.Error() != c.want {
				t.Errorf("parseRuntimeArgs(%q) error = %q, want %q", c.args, err.Error(), c.want)
			}
			if !reflect.DeepEqual(got, runtimeOptions{}) {
				t.Errorf("parseRuntimeArgs(%q) returned options %+v along with an error", c.args, got)
			}
		})
	}
}
