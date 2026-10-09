package runtime_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

// fakeHooks is the HookInstaller the tests of the Claude adapter's own logic hand it: it records
// the calls and answers what the test set, and reaches no file.
type fakeHooks struct {
	calls []string

	installErr, uninstallErr error
	found, owned             bool
	inspectErr               error
}

func (f *fakeHooks) Install(path, hookCommand string) error {
	f.calls = append(f.calls, "install "+path+" "+hookCommand)
	return f.installErr
}

func (f *fakeHooks) Uninstall(path, hookCommand string) error {
	f.calls = append(f.calls, "uninstall "+path+" "+hookCommand)
	return f.uninstallErr
}

func (f *fakeHooks) Inspect(path, hookCommand string) (bool, bool, error) {
	f.calls = append(f.calls, "inspect "+path+" "+hookCommand)
	return f.found, f.owned, f.inspectErr
}

// The adapter names the settings file and the binary from its root, and nothing else: the port is
// handed <root>/settings.json and <root>/bin/gentle-ai-overlay for every step.
func TestClaudeAdapterHandsThePortTheFileAndTheBinaryOfItsRoot(t *testing.T) {
	root := t.TempDir()
	hooks := &fakeHooks{found: true, owned: true}
	adapter := engineRuntime.NewClaudeAdapter(root, hooks)
	file := filepath.Join(root, "settings.json")
	binary := filepath.Join(root, "bin", "gentle-ai-overlay")

	adapter.Install()
	adapter.Update()
	adapter.Status()
	adapter.Uninstall()

	want := []string{"install " + file + " " + binary, "install " + file + " " + binary, "inspect " + file + " " + binary, "uninstall " + file + " " + binary}
	if got := strings.Join(hooks.calls, "\n"); got != strings.Join(want, "\n") {
		t.Errorf("the port was called\n%s\nwant\n%s", got, strings.Join(want, "\n"))
	}
}

func TestClaudeAdapterReportsWhatThePortSaysAboutTheHooks(t *testing.T) {
	cases := []struct {
		name   string
		hooks  fakeHooks
		status core.CapabilityStatus
		in     string
	}{
		{"owned", fakeHooks{found: true, owned: true}, core.CapabilitySupported, "installed and owned"},
		{"found but not owned", fakeHooks{found: true}, core.CapabilityPartial, "not fully owned/installed"},
		{"no settings file", fakeHooks{}, core.CapabilityUnsupported, "settings file not found at"},
		{"unreadable", fakeHooks{inspectErr: errors.New("boom")}, core.CapabilityUnsupported, "read settings file"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hooks := c.hooks
			result := engineRuntime.NewClaudeAdapter(t.TempDir(), &hooks).Status()
			if result.Status != c.status || !strings.Contains(result.Message, c.in) {
				t.Errorf("Status() = %q %q, want %q containing %q", result.Status, result.Message, c.status, c.in)
			}
		})
	}
}

func TestClaudeAdapterNamesAnInspectErrorAfterTheFile(t *testing.T) {
	root := t.TempDir()
	hooks := &fakeHooks{inspectErr: errors.New("unexpected end of JSON input")}

	result := engineRuntime.NewClaudeAdapter(root, hooks).Status()

	want := "read settings file " + filepath.Join(root, "settings.json") + ": unexpected end of JSON input"
	if result.Message != want {
		t.Errorf("Status().Message = %q, want %q", result.Message, want)
	}
}

func TestClaudeAdapterReportsAFailedInstallOrUninstallAsPartial(t *testing.T) {
	hooks := &fakeHooks{installErr: errors.New("no space"), uninstallErr: errors.New("busy")}
	adapter := engineRuntime.NewClaudeAdapter(t.TempDir(), hooks)

	for _, c := range []struct {
		result core.LifecycleResult
		want   string
	}{{adapter.Install(), "no space"}, {adapter.Update(), "no space"}, {adapter.Uninstall(), "busy"}} {
		if c.result.Status != core.CapabilityPartial || c.result.Message != c.want {
			t.Errorf("%s = %q %q, want partial %q", c.result.Action, c.result.Status, c.result.Message, c.want)
		}
	}
}

// A root that cannot name the files stops every step before the port is reached.
func TestClaudeAdapterWithoutAUsableRootNeverReachesThePort(t *testing.T) {
	for root, want := range map[string]string{
		"":              "Claude config root could not be resolved; set HOME",
		"   ":           "Claude config root could not be resolved; set HOME",
		"relative/root": `Claude config root must be absolute, got "relative/root"`,
	} {
		hooks := &fakeHooks{}
		adapter := engineRuntime.NewClaudeAdapter(root, hooks)
		for _, result := range []core.LifecycleResult{adapter.Install(), adapter.Update(), adapter.Status(), adapter.Uninstall()} {
			if result.Status != core.CapabilityUnsupported || result.Message != want {
				t.Errorf("root %q, %s = %q %q, want unsupported %q", root, result.Action, result.Status, result.Message, want)
			}
		}
		if len(hooks.calls) != 0 {
			t.Errorf("root %q reached the port: %v", root, hooks.calls)
		}
	}
}

// An adapter built with no port, by a caller that did not go through RegisterClaude, answers every
// step with a result that says so, where it used to panic on the first call. A typed nil (a nil
// pointer that implements the port) is no port either.
func TestClaudeAdapterWithoutAHookInstallerSaysSoInsteadOfPanicking(t *testing.T) {
	var typedNil *nilInstaller
	for name, hooks := range map[string]engineRuntime.HookInstaller{"nil": nil, "typed nil": typedNil} {
		adapter := engineRuntime.NewClaudeAdapter(t.TempDir(), hooks)
		for _, result := range []core.LifecycleResult{adapter.Install(), adapter.Update(), adapter.Status(), adapter.Uninstall()} {
			if result.Status != core.CapabilityUnsupported || !strings.Contains(result.Message, "hook installer") {
				t.Errorf("%s port, %s = %q %q, want unsupported saying there is no hook installer", name, result.Action, result.Status, result.Message)
			}
		}
	}
}

// nilInstaller is a HookInstaller whose methods would dereference their receiver.
type nilInstaller struct{ calls int }

func (n *nilInstaller) Install(string, string) error   { n.calls++; return nil }
func (n *nilInstaller) Uninstall(string, string) error { n.calls++; return nil }
func (n *nilInstaller) Inspect(string, string) (bool, bool, error) {
	n.calls++
	return false, false, nil
}

// RegisterClaude takes the Claude runtime only with its port, so an adapter never fails at its
// first lifecycle step.
func TestRegisterClaudeRefusesAMissingHookInstaller(t *testing.T) {
	var typedNil *nilInstaller
	for name, hooks := range map[string]engineRuntime.HookInstaller{"nil": nil, "typed nil": typedNil} {
		if err := engineRuntime.RegisterClaude(core.NewRegistry(), hooks); err == nil {
			t.Errorf("RegisterClaude accepted a %s HookInstaller", name)
		}
	}
}
