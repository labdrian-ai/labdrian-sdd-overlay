package runtime_test

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"

	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

func ownerRulesOnly(root string, source engineRuntime.OwnerRulesSource) core.Adapter {
	return engineRuntime.NewClaudeOwnerRulesAdapter(root, source)
}

func TestClaudeOwnerRulesAdapterIsTheClaudeTarget(t *testing.T) {
	if got := ownerRulesOnly(t.TempDir(), &rulesSource{text: rulesV1}).Target(); got != core.TargetClaude {
		t.Errorf("Target = %q", got)
	}
}

func TestClaudeOwnerRulesInstallWritesTheBlockIntoAnAbsentFile(t *testing.T) {
	root := t.TempDir()
	result := ownerRulesOnly(root, &rulesSource{text: rulesV1}).Install()
	if result.Status != core.CapabilityRestartRequired || result.Message != "owner rules written to "+claudeMD(root) {
		t.Errorf("Install = %v %q", result.Status, result.Message)
	}
	if got := readFileText(t, claudeMD(root)); got != rulesBlock(rulesV1)+"\n" {
		t.Errorf("CLAUDE.md = %q", got)
	}
}

func TestClaudeOwnerRulesInstallAppendsToAForeignFileAndUninstallGivesItBack(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(claudeMD(root), []byte(liveShapedFile), 0o644); err != nil {
		t.Fatal(err)
	}
	adapter := ownerRulesOnly(root, &rulesSource{text: rulesV1})
	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install = %v %q", result.Status, result.Message)
	}
	if got, want := readFileText(t, claudeMD(root)), liveShapedFile+"\n"+rulesBlock(rulesV1)+"\n"; got != want {
		t.Errorf("CLAUDE.md = %q, want %q", got, want)
	}
	result := adapter.Uninstall()
	if result.Status != core.CapabilityRestartRequired || result.Message != "owner rules removed from "+claudeMD(root) {
		t.Errorf("Uninstall = %v %q", result.Status, result.Message)
	}
	if got := readFileText(t, claudeMD(root)); got != liveShapedFile {
		t.Errorf("CLAUDE.md after uninstall = %q, want the pre-install bytes %q", got, liveShapedFile)
	}
}

func TestClaudeOwnerRulesSecondInstallIsAlreadyCurrentAndLeavesTheBytes(t *testing.T) {
	root := t.TempDir()
	adapter := ownerRulesOnly(root, &rulesSource{text: rulesV1})
	adapter.Install()
	first := readFileText(t, claudeMD(root))
	result := adapter.Install()
	if result.Message != "owner rules already current in "+claudeMD(root) {
		t.Errorf("second Install = %q", result.Message)
	}
	if readFileText(t, claudeMD(root)) != first {
		t.Error("the second install changed CLAUDE.md")
	}
}

func TestClaudeOwnerRulesUpdateReplacesAStaleBlock(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(claudeMD(root), []byte("mine\n"+rulesBlock("old\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := ownerRulesOnly(root, &rulesSource{text: rulesV2}).Update()
	if result.Message != "owner rules updated in "+claudeMD(root) {
		t.Errorf("Update = %v %q", result.Status, result.Message)
	}
	if got, want := readFileText(t, claudeMD(root)), "mine\n"+rulesBlock(rulesV2)+"\n"; got != want {
		t.Errorf("CLAUDE.md = %q, want %q", got, want)
	}
}

func TestClaudeOwnerRulesStatus(t *testing.T) {
	root := t.TempDir()
	adapter := ownerRulesOnly(root, &rulesSource{text: rulesV1})
	if r := adapter.Status(); r.Status != core.CapabilityPartial || !strings.Contains(r.Message, "Claude owner rules block is missing in "+claudeMD(root)) {
		t.Errorf("missing: %v %q", r.Status, r.Message)
	}
	adapter.Install()
	if r := adapter.Status(); r.Status != core.CapabilitySupported || r.Message != "owner rules block is current in "+claudeMD(root) {
		t.Errorf("current: %v %q", r.Status, r.Message)
	}
	if r := ownerRulesOnly(root, &rulesSource{text: rulesV2}).Status(); r.Status != core.CapabilityPartial || !strings.Contains(r.Message, "Claude owner rules block is stale in "+claudeMD(root)) {
		t.Errorf("stale: %v %q", r.Status, r.Message)
	}
}

func TestClaudeOwnerRulesWithNoCheckoutIsUnsupportedForInstallUpdateAndStatusAndLeavesClaudeMD(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(claudeMD(root), []byte(liveShapedFile), 0o644); err != nil {
		t.Fatal(err)
	}
	adapter := ownerRulesOnly(root, &rulesSource{err: engineRuntime.ErrNoOverlayCheckout})
	for name, r := range map[string]struct {
		result core.LifecycleResult
		note   string
	}{
		"install": {adapter.Install(), "owner rules not installed: " + engineRuntime.ErrNoOverlayCheckout.Error()},
		"update":  {adapter.Update(), "owner rules not installed: " + engineRuntime.ErrNoOverlayCheckout.Error()},
		"status":  {adapter.Status(), "owner rules not checked: " + engineRuntime.ErrNoOverlayCheckout.Error()},
	} {
		if r.result.Status != core.CapabilityUnsupported || r.result.Message != r.note {
			t.Errorf("%s = %v %q, want unsupported %q", name, r.result.Status, r.result.Message, r.note)
		}
	}
	if got := readFileText(t, claudeMD(root)); got != liveShapedFile {
		t.Errorf("CLAUDE.md = %q", got)
	}
}

func TestClaudeOwnerRulesUninstallWithNoCheckoutStillRemovesTheBlock(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(claudeMD(root), []byte(liveShapedFile+"\n"+rulesBlock(rulesV1)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := ownerRulesOnly(root, &rulesSource{err: engineRuntime.ErrNoOverlayCheckout}).Uninstall()
	if r.Status != core.CapabilityRestartRequired || r.Message != "owner rules removed from "+claudeMD(root) {
		t.Errorf("Uninstall = %v %q", r.Status, r.Message)
	}
	if got := readFileText(t, claudeMD(root)); got != liveShapedFile {
		t.Errorf("CLAUDE.md after uninstall = %q, want %q", got, liveShapedFile)
	}
}

func TestClaudeOwnerRulesNeverTouchesSettingsJSON(t *testing.T) {
	root := t.TempDir()
	settingsJSON := filepath.Join(root, "settings.json")
	fixture := "{\n  \"hooks\": {},\n  \"permissions\": {\"deny\": [\"x\"]}\n}\n"
	if err := os.WriteFile(settingsJSON, []byte(fixture), 0o600); err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256([]byte(fixture))
	adapter := ownerRulesOnly(root, &rulesSource{text: rulesV1})
	for name, act := range map[string]func() core.LifecycleResult{"install": adapter.Install, "update": adapter.Update, "status": adapter.Status, "uninstall": adapter.Uninstall} {
		act()
		data, err := os.ReadFile(settingsJSON)
		if err != nil || sha256.Sum256(data) != want {
			t.Errorf("after %s settings.json = %q, err %v", name, data, err)
		}
		if _, err := os.Stat(settingsJSON + ".bak"); err == nil {
			t.Errorf("after %s a settings.json.bak appeared", name)
		}
	}
}
