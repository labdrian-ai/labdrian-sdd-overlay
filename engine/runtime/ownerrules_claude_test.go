package runtime_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings/settingsfile"
)

// rulesSource is the owner rules a test hands the adapter.
type rulesSource struct {
	text string
	err  error
}

func (s *rulesSource) OwnerRules() (string, error) { return s.text, s.err }

const rulesV1 = "## Rules\n\n- first rule\n"
const rulesV2 = "## Rules\n\n- first rule\n- second rule\n"

func rulesBlock(body string) string {
	return "<!-- labdrian:owner-rules -->\n" + strings.TrimRight(body, "\n") + "\n<!-- /labdrian:owner-rules -->"
}

func claudeWithRules(root string, source engineRuntime.OwnerRulesSource) engineRuntime.ClaudeAdapter {
	return engineRuntime.NewClaudeAdapter(root, settingsfile.Installer{}).WithOwnerRules(source)
}

func claudeMD(root string) string { return filepath.Join(root, "CLAUDE.md") }

func TestClaudeInstallCreatesAMissingClaudeMDWithTheBlockAlone(t *testing.T) {
	root := t.TempDir()
	result := claudeWithRules(root, &rulesSource{text: rulesV1}).Install()
	if result.Status != core.CapabilityRestartRequired || !strings.Contains(result.Message, "; owner rules written to "+claudeMD(root)) {
		t.Errorf("Install = %v %q", result.Status, result.Message)
	}
	if got := readFileText(t, claudeMD(root)); got != rulesBlock(rulesV1)+"\n" {
		t.Errorf("CLAUDE.md = %q", got)
	}
}

func TestClaudeInstallAppendsTheBlockAndKeepsEveryOtherByte(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(claudeMD(root), []byte(liveShapedFile), 0o644); err != nil {
		t.Fatal(err)
	}
	if result := claudeWithRules(root, &rulesSource{text: rulesV1}).Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install = %v %q", result.Status, result.Message)
	}
	if got, want := readFileText(t, claudeMD(root)), liveShapedFile+"\n"+rulesBlock(rulesV1)+"\n"; got != want {
		t.Errorf("CLAUDE.md = %q, want %q", got, want)
	}
}

func TestClaudeUpdateReplacesAStaleBlockInPlace(t *testing.T) {
	root := t.TempDir()
	before, after := "mine\n\n"+liveShapedFile+"\n", "\n<!-- user:other -->\nz\n<!-- /user:other -->\nend"
	if err := os.WriteFile(claudeMD(root), []byte(before+rulesBlock("old\n")+after), 0o644); err != nil {
		t.Fatal(err)
	}
	result := claudeWithRules(root, &rulesSource{text: rulesV2}).Update()
	if result.Status != core.CapabilityRestartRequired || !strings.Contains(result.Message, "; owner rules updated in "+claudeMD(root)) {
		t.Errorf("Update = %v %q", result.Status, result.Message)
	}
	if got, want := readFileText(t, claudeMD(root)), before+rulesBlock(rulesV2)+after; got != want {
		t.Errorf("CLAUDE.md = %q, want %q", got, want)
	}
}

func TestClaudeSecondInstallIsANoOpOnClaudeMD(t *testing.T) {
	root := t.TempDir()
	adapter := claudeWithRules(root, &rulesSource{text: rulesV1})
	adapter.Install()
	old := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := os.Chtimes(claudeMD(root), old, old); err != nil {
		t.Fatal(err)
	}
	first := readFileText(t, claudeMD(root))
	result := adapter.Install()
	info, _ := os.Stat(claudeMD(root))
	if readFileText(t, claudeMD(root)) != first || !info.ModTime().Equal(old) {
		t.Error("the second install rewrote CLAUDE.md")
	}
	if !strings.Contains(result.Message, "; owner rules already current in "+claudeMD(root)) {
		t.Errorf("Install message = %q", result.Message)
	}
}

func TestClaudeUninstallRemovesOnlyTheBlockAndRestoresTheFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(claudeMD(root), []byte(liveShapedFile), 0o644); err != nil {
		t.Fatal(err)
	}
	adapter := claudeWithRules(root, &rulesSource{text: rulesV1})
	adapter.Install()
	result := adapter.Uninstall()
	if result.Status != core.CapabilityRestartRequired || !strings.Contains(result.Message, "; owner rules removed from "+claudeMD(root)) {
		t.Errorf("Uninstall = %v %q", result.Status, result.Message)
	}
	if got := readFileText(t, claudeMD(root)); got != liveShapedFile {
		t.Errorf("CLAUDE.md = %q, want the original %q", got, liveShapedFile)
	}
	if strings.Contains(readFileText(t, claudeMD(root)), "labdrian:") {
		t.Error("a labdrian marker remains")
	}
}

func TestClaudeUninstallNeedsNoSourceAndCreatesNoClaudeMD(t *testing.T) {
	root := t.TempDir()
	result := claudeWithRules(root, &rulesSource{err: errors.New("no source")}).Uninstall()
	if result.Status != core.CapabilityRestartRequired || result.Message != "Claude lifecycle hooks removed; restart Claude to unload stale hook processes" {
		t.Errorf("Uninstall = %v %q", result.Status, result.Message)
	}
	if _, err := os.Stat(claudeMD(root)); !os.IsNotExist(err) {
		t.Error("Uninstall created CLAUDE.md")
	}
}

func TestClaudeStatusSaysMissingStaleOrCurrentForTheBlock(t *testing.T) {
	root := t.TempDir()
	newClaudeAdapter(root).Install()
	adapter := claudeWithRules(root, &rulesSource{text: rulesV1})

	result := adapter.Status()
	want := "Claude owner rules block is missing in " + claudeMD(root) + "; run 'engine runtime install --target claude'"
	if result.Status != core.CapabilityPartial || result.Message != want {
		t.Errorf("missing: Status = %v %q, want partial %q", result.Status, result.Message, want)
	}

	if err := os.WriteFile(claudeMD(root), []byte("mine\n\n"+rulesBlock("old\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result = adapter.Status()
	want = "Claude owner rules block is stale in " + claudeMD(root) + "; run 'engine runtime update --target claude'"
	if result.Status != core.CapabilityPartial || result.Message != want {
		t.Errorf("stale: Status = %v %q, want partial %q", result.Status, result.Message, want)
	}

	adapter.Update()
	result = adapter.Status()
	want = "Claude lifecycle hooks are installed and owned; owner rules block is current in " + claudeMD(root)
	if result.Status != core.CapabilitySupported || result.Message != want {
		t.Errorf("current: Status = %v %q, want supported %q", result.Status, result.Message, want)
	}
}

func TestClaudeStatusKeepsTheHookAndMainAgentVerdictsBeforeTheBlock(t *testing.T) {
	root := t.TempDir()
	adapter := claudeWithRules(root, &rulesSource{text: rulesV1})
	adapter.Install()
	if err := os.WriteFile(claudeMD(root), []byte("none\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rootSettings := parseClaudeSettingsFile(t, settingsPathOf(root))
	delete(rootSettings, "agent")
	writeSettingsMap(t, settingsPathOf(root), rootSettings)
	if result := adapter.Status(); !strings.HasPrefix(result.Message, `Claude settings "agent" key is missing`) {
		t.Errorf("Status = %q, want the main agent verdict first", result.Message)
	}
}

func TestClaudeInstallWithAnUnreadableSourceKeepsTheHooksAndClaudeMD(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(claudeMD(root), []byte(liveShapedFile), 0o644); err != nil {
		t.Fatal(err)
	}
	result := claudeWithRules(root, &rulesSource{err: errors.New("no overlay")}).Install()
	if result.Status != core.CapabilityPartial || !strings.Contains(result.Message, "owner rules could not be read: no overlay") {
		t.Errorf("Install = %v %q", result.Status, result.Message)
	}
	if got := readFileText(t, claudeMD(root)); got != liveShapedFile {
		t.Errorf("CLAUDE.md changed to %q", got)
	}
	if found, owned, err := (settingsfile.Installer{}).Inspect(settingsPathOf(root), filepath.Join(root, "bin", "gentle-ai-overlay")); err != nil || !found || !owned {
		t.Errorf("the hooks are not installed: %v %v %v", found, owned, err)
	}
}

func TestClaudeInstallWithBrokenMarkersIsPartialAndLeavesClaudeMDAsItIs(t *testing.T) {
	root := t.TempDir()
	broken := "mine\n<!-- labdrian:owner-rules -->\nno close\n"
	if err := os.WriteFile(claudeMD(root), []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	adapter := claudeWithRules(root, &rulesSource{text: rulesV1})
	installed, updated := adapter.Install(), adapter.Update()
	status := adapter.Status()
	uninstalled := adapter.Uninstall()
	for name, result := range map[string]core.LifecycleResult{"install": installed, "update": updated, "status": status, "uninstall": uninstalled} {
		if result.Status != core.CapabilityPartial || !strings.Contains(result.Message, claudeMD(root)) {
			t.Errorf("%s = %v %q, want partial naming the file", name, result.Status, result.Message)
		}
	}
	if got := readFileText(t, claudeMD(root)); got != broken {
		t.Errorf("CLAUDE.md changed to %q", got)
	}
}

// An adapter built without a source is the one every earlier caller builds: it never reads or
// writes CLAUDE.md.
func TestClaudeAdapterWithoutOwnerRulesNeverTouchesClaudeMD(t *testing.T) {
	root := t.TempDir()
	adapter := newClaudeAdapter(root)
	for _, result := range []core.LifecycleResult{adapter.Install(), adapter.Update(), adapter.Status(), adapter.Uninstall()} {
		if strings.Contains(result.Message, "owner rules") {
			t.Errorf("%s: %q mentions the owner rules", result.Action, result.Message)
		}
	}
	if _, err := os.Stat(claudeMD(root)); !os.IsNotExist(err) {
		t.Error("CLAUDE.md was created")
	}
}

func writeOverlayRules(t *testing.T, text string) string {
	t.Helper()
	overlay := t.TempDir()
	if err := os.MkdirAll(filepath.Join(overlay, "instructions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(overlay, "instructions", "owner-rules.md"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return overlay
}

func TestOverlayOwnerRulesReadsTheFileOfTheConfiguredCheckout(t *testing.T) {
	overlay := writeOverlayRules(t, rulesV2)
	got, err := engineRuntime.OverlayOwnerRules{OverlayDir: overlay}.OwnerRules()
	if err != nil || got != rulesV2 {
		t.Errorf("OwnerRules = %q %v", got, err)
	}
}

func TestOverlayOwnerRulesFailsOnAMissingFileAndOnARelativeCheckout(t *testing.T) {
	if _, err := (engineRuntime.OverlayOwnerRules{OverlayDir: t.TempDir()}).OwnerRules(); err == nil || !strings.Contains(err.Error(), "owner rules") {
		t.Errorf("a checkout without the file: error = %v", err)
	}
	if _, err := (engineRuntime.OverlayOwnerRules{OverlayDir: "relative/dir"}).OwnerRules(); err == nil || !strings.Contains(err.Error(), core.LabdrianOverlayDirVariable) {
		t.Errorf("a relative checkout: error = %v", err)
	}
}

// The registered Claude adapter reads the owner rules of the overlay the config names.
func TestRegisteredClaudeAdapterWritesTheOwnerRulesOfTheConfiguredOverlay(t *testing.T) {
	root, overlay := t.TempDir(), writeOverlayRules(t, rulesV1)
	registry := core.NewRegistry()
	if err := engineRuntime.RegisterClaudeWithOwnerRules(registry, settingsfile.Installer{}); err != nil {
		t.Fatal(err)
	}
	adapter, err := registry.New(core.TargetClaude, core.Config{ConfigRoot: root, LabdrianOverlayDir: overlay})
	if err != nil {
		t.Fatal(err)
	}
	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install = %v %q", result.Status, result.Message)
	}
	if got := readFileText(t, claudeMD(root)); got != rulesBlock(rulesV1)+"\n" {
		t.Errorf("CLAUDE.md = %q", got)
	}
}
