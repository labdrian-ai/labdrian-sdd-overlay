package runtime_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

// What the Pi adapter asks of the CommandRunner port: which lifecycle steps run `pi` at all, the
// deadline each command runs under, and what the adapter reports when the CLI is missing or fails.

// Only Install and Uninstall start `pi`. Every other step answers from files, and none of them so
// much as looks the program up.
func TestPiAdapter_OnlyInstallAndUninstallReachTheCLI(t *testing.T) {
	adapter, _, commands := newBuiltPiAdapterWithStub(t)
	adapter.Status()
	adapter.SyncCheck()
	adapter.Update()
	adapter.Rollback()
	if len(commands.calls) != 0 || len(commands.lookups) != 0 {
		t.Fatalf("Status, SyncCheck, Update and Rollback reached the CLI: lookups=%v calls=%v", commands.lookups, commands.calls)
	}
}

// The adapter finds the CLI by the name pi and runs the path the port gave back.
func TestPiAdapter_RunsTheProgramTheLookupFound(t *testing.T) {
	adapter, _, commands := newBuiltPiAdapterWithStub(t)
	adapter.Install()
	if len(commands.lookups) == 0 || commands.lookups[0] != "pi" {
		t.Fatalf("lookups = %v, want pi", commands.lookups)
	}
	for _, call := range commands.calls {
		if call.Bin != fakePiPath {
			t.Errorf("a command ran %q, want the path the lookup found, %s", call.Bin, fakePiPath)
		}
	}
}

func TestPiAdapter_CommandsRunUnderTheDefaultDeadline(t *testing.T) {
	adapter, _, commands := newBuiltPiAdapterWithStub(t)
	adapter.Install()
	if len(commands.calls) == 0 {
		t.Fatal("Install ran no command")
	}
	assertDeadlineWithin(t, commands, engineRuntime.DefaultPiCommandTimeout)

	adapter, _, commands = newBuiltPiAdapterWithStub(t)
	adapter.Uninstall()
	if len(commands.calls) != 1 {
		t.Fatalf("Uninstall ran %d commands, want 1", len(commands.calls))
	}
	assertDeadlineWithin(t, commands, engineRuntime.DefaultPiCommandTimeout)
}

func TestPiAdapter_TheDeadlineIsAnOptionOfTheAdapter(t *testing.T) {
	adapter, _, commands := newBuiltPiAdapterWithOptions(t, engineRuntime.PiOptions{CommandTimeout: 45 * time.Second})
	adapter.Install()
	if len(commands.calls) == 0 {
		t.Fatal("Install ran no command")
	}
	assertDeadlineWithin(t, commands, 45*time.Second)
}

// assertDeadlineWithin fails unless every command ran under a deadline that is in the future and
// no later than limit: a deadline that was not set, or one longer than asked for, fails.
func assertDeadlineWithin(t *testing.T, commands *fakeCommands, limit time.Duration) {
	t.Helper()
	for _, call := range commands.calls {
		if !call.HasDeadline {
			t.Errorf("`pi %s` ran with no deadline", strings.Join(call.Args, " "))
			continue
		}
		if call.Remaining <= 0 || call.Remaining > limit {
			t.Errorf("`pi %s` ran with %v left, want more than 0 and at most %v", strings.Join(call.Args, " "), call.Remaining, limit)
		}
	}
}

// A command that does not finish in time fails the step the way any failing command does: the
// message names the command and the reason, the step is partial and nothing is claimed.
func TestPiAdapter_ACommandThatMissesItsDeadlineIsAPartialInstall(t *testing.T) {
	adapter, _, commands := newBuiltPiAdapterWithStub(t)
	commands.fail = func(args []string) error {
		return errors.New(context.DeadlineExceeded.Error() + " (signal: killed)")
	}

	result := adapter.Install()
	if result.Status != core.CapabilityPartial {
		t.Fatalf("Install = %s, want partial", result)
	}
	for _, want := range []string{"but `pi install` failed", "context deadline exceeded"} {
		if !strings.Contains(result.Message, want) {
			t.Errorf("message = %q, want it to contain %q", result.Message, want)
		}
	}
	if len(commands.calls) != 1 {
		t.Errorf("Install went on after the package install failed: %v", commands.invocations())
	}
}

func TestPiAdapter_AFailingCommandCarriesWhatItPrinted(t *testing.T) {
	adapter, destDir, commands := newBuiltPiAdapterWithStub(t)
	commands.output = "  no such package\n"
	commands.fail = func(args []string) error { return errors.New("exit status 1") }

	result := adapter.Uninstall()
	if result.Status != core.CapabilityPartial {
		t.Fatalf("Uninstall = %s, want partial", result)
	}
	if want := "`pi remove " + destDir + "` failed: exit status 1 (output: no such package)."; !strings.Contains(result.Message, want) {
		t.Errorf("message = %q, want it to contain %q", result.Message, want)
	}
	if _, err := os.Stat(destDir); err != nil {
		t.Errorf("the package directory was removed although `pi remove` failed: %v", err)
	}
}

// An extension install that fails is told, and does not undo the package install that worked.
func TestPiAdapter_AFailingExtensionInstallIsToldInline(t *testing.T) {
	adapter, _, commands := newBuiltPiAdapterWithStub(t)
	commands.fail = func(args []string) error {
		if len(args) == 2 && args[1] == "npm:pi-subagents-j0k3r" {
			return errors.New("npm exploded")
		}
		return nil
	}
	result := adapter.Install()
	if result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install = %s, want restart_required: the package install succeeded", result)
	}
	if !strings.Contains(result.Message, "`pi install npm:pi-subagents-j0k3r` failed: npm exploded") {
		t.Errorf("message = %q, want the extension failure told inline", result.Message)
	}
}

func TestPiAdapter_WithoutPiInstallStaysPartialWithTheHint(t *testing.T) {
	adapter, destDir, commands := newBuiltPiAdapterWithStub(t)
	commands.missing = true

	result := adapter.Install()
	if result.Status != core.CapabilityPartial {
		t.Fatalf("Install without pi = %s, want partial", result)
	}
	if want := "labdrian-pi package built at " + destDir + "; run: pi install " + destDir; result.Message != want {
		t.Errorf("message = %q, want %q", result.Message, want)
	}
	if len(commands.calls) != 0 {
		t.Errorf("a command ran although pi was not found: %v", commands.invocations())
	}
}

func TestPiAdapter_WithoutPiUninstallLeavesThePackageAndSaysWhy(t *testing.T) {
	adapter, destDir, commands := newBuiltPiAdapterWithStub(t)
	commands.missing = true

	result := adapter.Uninstall()
	if result.Status != core.CapabilityPartial {
		t.Fatalf("Uninstall without pi = %s, want partial", result)
	}
	if !strings.Contains(result.Message, "pi CLI not found on PATH; cannot run `pi remove "+destDir+"`") {
		t.Errorf("message = %q, want it to say pi was not found", result.Message)
	}
	if _, err := os.Stat(filepath.Join(destDir, "package.json")); err != nil {
		t.Errorf("the package was removed although `pi remove` never ran: %v", err)
	}
}

// RegisterPi takes the Pi runtime only with both ports, so an adapter never fails at its first
// lifecycle step for want of a process or a builder.
func TestRegisterPiRefusesMissingPorts(t *testing.T) {
	packages := newPiPackages()
	for name, ports := range map[string]engineRuntime.PiPorts{
		"no commands": {Packages: packages},
		"no packages": {Commands: &fakeCommands{}},
		"neither":     {},
	} {
		t.Run(name, func(t *testing.T) {
			if err := engineRuntime.RegisterPi(core.NewRegistry(), ports); err == nil {
				t.Fatal("RegisterPi accepted ports with a nil member")
			}
		})
	}
}

// The Config the composition root fills decides whether the Subagents extension is installed: the
// adapter the registry builds skips it when Config.PiSkipSubagents says so, and only then.
func TestThePiFactoryTakesTheSkipFromTheConfig(t *testing.T) {
	for _, skip := range []bool{false, true} {
		overlayRoot, _ := piFixtureOverlay(t)
		home := t.TempDir()
		writePiSettingsPackages(t, home, nil)
		commands := &fakeCommands{}
		adapter, err := shippedRegistryWith(t, commands).New(core.TargetPi, core.Config{
			Home: home, OverlayDir: overlayRoot, StateDir: t.TempDir(), PiSkipSubagents: skip,
		})
		if err != nil {
			t.Fatal(err)
		}
		result := adapter.Install()
		if result.Status == core.CapabilityUnsupported {
			t.Fatalf("skip=%v: Install = %s", skip, result)
		}
		installedExtension := false
		for _, token := range readAllRecordedTokens(t, commands) {
			installedExtension = installedExtension || strings.Contains(token, "pi-subagents")
		}
		if installedExtension == skip {
			t.Errorf("skip=%v: the extension install ran = %v, want %v", skip, installedExtension, !skip)
		}
	}
}
