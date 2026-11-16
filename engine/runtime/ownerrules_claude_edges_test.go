package runtime_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings/settingsfile"
)

// chdirForTest moves the test into dir (outside any overlay checkout) and back when it ends.
func chdirForTest(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

// noCheckout answers as OverlayOwnerRules does on a machine with no overlay checkout.
func noCheckout() *rulesSource {
	return &rulesSource{err: engineRuntime.ErrNoOverlayCheckout}
}

const noCheckoutNote = "owner rules not installed: no overlay checkout found; set LABDRIAN_OVERLAY_DIR"

// With no checkout to read the rules from, the Claude adapter is the hooks-only adapter it was:
// the same status, the same exit, a note in the message, no CLAUDE.md written.
func TestClaudeWithNoOverlayCheckoutInstallsAndUpdatesTheHooksAloneWithANote(t *testing.T) {
	root := t.TempDir()
	adapter := claudeWithRules(root, noCheckout())
	for name, result := range map[string]core.LifecycleResult{"install": adapter.Install(), "update": adapter.Update()} {
		if result.Status != core.CapabilityRestartRequired || !strings.Contains(result.Message, "; "+noCheckoutNote) {
			t.Errorf("%s = %v %q, want restart_required with the note", name, result.Status, result.Message)
		}
	}
	if _, err := os.Stat(claudeMD(root)); !os.IsNotExist(err) {
		t.Error("CLAUDE.md was created")
	}
}

func TestClaudeWithNoOverlayCheckoutIsSupportedWhenTheHooksAre(t *testing.T) {
	root := t.TempDir()
	adapter := claudeWithRules(root, noCheckout())
	adapter.Install()
	result := adapter.Status()
	want := "Claude lifecycle hooks are installed and owned; owner rules not checked: no overlay checkout found; set LABDRIAN_OVERLAY_DIR"
	if result.Status != core.CapabilitySupported || result.Message != want {
		t.Errorf("Status = %v %q, want supported %q", result.Status, result.Message, want)
	}
}

func TestOverlayOwnerRulesWithoutACheckoutSaysNoCheckoutOnlyWhenNoneWasConfigured(t *testing.T) {
	chdirForTest(t, t.TempDir())
	if _, err := (engineRuntime.OverlayOwnerRules{}).OwnerRules(); err == nil || err != engineRuntime.ErrNoOverlayCheckout {
		t.Errorf("none configured: error = %v, want ErrNoOverlayCheckout", err)
	}
	if _, err := (engineRuntime.OverlayOwnerRules{OverlayDir: filepath.Join(t.TempDir(), "gone")}).OwnerRules(); err == nil || err == engineRuntime.ErrNoOverlayCheckout {
		t.Errorf("a configured checkout without the file: error = %v, want another error", err)
	}
}

func TestRegisteredClaudeAdapterOutsideACheckoutKeepsTheHooksOnlyStatusAndExit(t *testing.T) {
	chdirForTest(t, t.TempDir())
	root := t.TempDir()
	registry := core.NewRegistry()
	if err := engineRuntime.RegisterClaudeWithOwnerRules(registry, settingsfile.Installer{}); err != nil {
		t.Fatal(err)
	}
	adapter, err := registry.New(core.TargetClaude, core.Config{ConfigRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	results := []core.LifecycleResult{adapter.Install(), adapter.Status(), adapter.Update()}
	if core.AggregateStatus(core.ActionInstall, results[:1]) || core.AggregateStatus(core.ActionStatus, results[1:2]) || core.AggregateStatus(core.ActionUpdate, results[2:]) {
		t.Errorf("a result fails the run: %+v", results)
	}
}

func TestClaudeUninstallGivesBackTheExactBytesOfTheFileBeforeInstall(t *testing.T) {
	for name, original := range map[string]string{
		"no final newline":      "my rules",
		"live shaped":           liveShapedFile,
		"blank line at the end": "mine\n\n",
		"crlf":                  "mine\r\nmore\r\n",
	} {
		root := t.TempDir()
		if err := os.WriteFile(claudeMD(root), []byte(original), 0o644); err != nil {
			t.Fatal(err)
		}
		adapter := claudeWithRules(root, &rulesSource{text: rulesV1})
		adapter.Install()
		adapter.Uninstall()
		if got := readFileText(t, claudeMD(root)); got != original {
			t.Errorf("%s: CLAUDE.md = %q, want %q", name, got, original)
		}
	}
}

func TestClaudeUninstallRemovesAClaudeMDThatInstallCreated(t *testing.T) {
	root := t.TempDir()
	adapter := claudeWithRules(root, &rulesSource{text: rulesV1})
	adapter.Install()
	adapter.Uninstall()
	if _, err := os.Stat(claudeMD(root)); !os.IsNotExist(err) {
		t.Error("CLAUDE.md that held only the block is still there")
	}
}

func TestClaudeInstallRefusesADanglingClaudeMDLinkAndWritesNothing(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink("missing.md", claudeMD(root)); err != nil {
		t.Fatal(err)
	}
	adapter := claudeWithRules(root, &rulesSource{text: rulesV1})
	for name, result := range map[string]core.LifecycleResult{"install": adapter.Install(), "update": adapter.Update(), "status": adapter.Status()} {
		if result.Status != core.CapabilityPartial || !strings.Contains(result.Message, "symbolic link") {
			t.Errorf("%s = %v %q", name, result.Status, result.Message)
		}
	}
	if info, err := os.Lstat(claudeMD(root)); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Error("the link was replaced")
	}
	if _, err := os.Stat(filepath.Join(root, "missing.md")); !os.IsNotExist(err) {
		t.Error("the file behind the link was created")
	}
}
