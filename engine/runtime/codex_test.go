package runtime_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
)

func TestDefaultCodexConfigRootPrefersCODEXHomeWhenAbsolute(t *testing.T) {
	home := t.TempDir()
	codeXHome := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", codeXHome)

	if got := engineRuntime.DefaultCodexConfigRoot(); got != codeXHome {
		t.Fatalf("DefaultCodexConfigRoot() = %q, want %q", got, codeXHome)
	}
}

func TestDefaultCodexConfigRootFallsBackToHomeWhenCODEXHomeUnsetOrRelative(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", "")

	want := filepath.Join(home, ".codex")
	if got := engineRuntime.DefaultCodexConfigRoot(); got != want {
		t.Fatalf("DefaultCodexConfigRoot() = %q, want %q", got, want)
	}

	t.Setenv("CODEX_HOME", filepath.Join("relative", "codex"))
	if got := engineRuntime.DefaultCodexConfigRoot(); got != want {
		t.Fatalf("DefaultCodexConfigRoot() with relative CODEX_HOME = %q, want %q", got, want)
	}
}

func TestCodexAdapterRejectsUnresolvedOrRelativeRoot(t *testing.T) {
	t.Setenv("CODEX_HOME", "")
	t.Setenv("HOME", "")

	if result := engineRuntime.NewCodexAdapter("").Status(); result.Status != engineRuntime.CapabilityUnsupported {
		t.Fatalf("NewCodexAdapter(\"\").Status() = %#v, want unsupported for unresolved root", result)
	}

	relative := filepath.Join("relative", "codex")
	if result := engineRuntime.NewCodexAdapter(relative).Status(); result.Status != engineRuntime.CapabilityUnsupported {
		t.Fatalf("NewCodexAdapter(relative).Status() = %#v, want unsupported for relative root", result)
	}
}

func TestCodexInstallWritesManifestAndPreservesUnrelatedFiles(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewCodexAdapter(root)

	unrelatedPath := filepath.Join(root, "skills", "user-skill.md")
	if err := os.MkdirAll(filepath.Dir(unrelatedPath), 0o755); err != nil {
		t.Fatalf("mkdir unrelated dir: %v", err)
	}
	if err := os.WriteFile(unrelatedPath, []byte("user file content"), 0o644); err != nil {
		t.Fatalf("write unrelated file: %v", err)
	}

	if result := adapter.Install(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}

	manifestPath := filepath.Join(root, "labdrian-runtime-lifecycle.json")
	manifest := readCodexManifest(t, manifestPath)
	if manifest["managed_by"] != "labdrian-sdd-overlay" {
		t.Fatalf("managed_by = %#v, want labdrian-sdd-overlay", manifest["managed_by"])
	}
	if manifest["installed_version"] != engineRuntime.CodexInstalledVersion {
		t.Fatalf("installed_version = %#v, want %q", manifest["installed_version"], engineRuntime.CodexInstalledVersion)
	}
	if manifest["config_root"] != root {
		t.Fatalf("config_root = %#v, want %s", manifest["config_root"], root)
	}

	unrelatedData, err := os.ReadFile(unrelatedPath)
	if err != nil {
		t.Fatalf("read unrelated file after install: %v", err)
	}
	if string(unrelatedData) != "user file content" {
		t.Fatalf("unrelated file should be preserved, got %q", string(unrelatedData))
	}
}

func TestCodexUpdateRefreshesManagedManifest(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, "labdrian-runtime-lifecycle.json")
	if err := os.WriteFile(manifestPath, []byte(`{"managed_by":"labdrian-sdd-overlay","installed_version":"legacy","config_root":"`+root+`"}`), 0o644); err != nil {
		t.Fatalf("write stale manifest: %v", err)
	}

	adapter := engineRuntime.NewCodexAdapter(root)
	if result := adapter.Update(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Update() = %#v", result)
	}

	after := readCodexManifest(t, manifestPath)
	if after["installed_version"] != engineRuntime.CodexInstalledVersion {
		t.Fatalf("Update() should set installed_version to %q, got %#v", engineRuntime.CodexInstalledVersion, after["installed_version"])
	}
	if after["managed_by"] != "labdrian-sdd-overlay" {
		t.Fatalf("managed_by after update = %#v", after["managed_by"])
	}
}

func TestCodexInstallRejectsUnownedOrMalformedExistingManifest(t *testing.T) {
	for _, tt := range []struct {
		name     string
		manifest string
	}{
		{
			name:     "unowned manifest",
			manifest: `{"managed_by":"other","installed_version":"legacy","config_root":"/tmp/other"}`,
		},
		{
			name:     "malformed manifest",
			manifest: `not-json`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			manifestPath := filepath.Join(root, "labdrian-runtime-lifecycle.json")
			if err := os.WriteFile(manifestPath, []byte(tt.manifest), 0o644); err != nil {
				t.Fatalf("write bad manifest: %v", err)
			}

			before, err := os.ReadFile(manifestPath)
			if err != nil {
				t.Fatalf("read manifest before install: %v", err)
			}

			result := engineRuntime.NewCodexAdapter(root).Install()
			if result.Status != engineRuntime.CapabilityPartial {
				t.Fatalf("Install() = %#v", result)
			}

			after, err := os.ReadFile(manifestPath)
			if err != nil {
				t.Fatalf("read manifest after failed install: %v", err)
			}
			if string(after) != string(before) {
				t.Fatalf("Install() should not modify existing manifest: before=%q after=%q", string(before), string(after))
			}
			if !strings.Contains(result.Message, "existing Codex manifest") {
				t.Fatalf("failure message should mention existing Codex manifest; got %q", result.Message)
			}
		})
	}
}

func TestCodexUpdateRejectsUnownedOrMalformedManifest(t *testing.T) {
	for _, tt := range []struct {
		name     string
		manifest string
	}{
		{
			name:     "unowned manifest",
			manifest: `{"managed_by":"other","installed_version":"legacy","config_root":"/tmp/other"}`,
		},
		{
			name:     "malformed manifest",
			manifest: `not-json`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			manifestPath := filepath.Join(root, "labdrian-runtime-lifecycle.json")
			if err := os.WriteFile(manifestPath, []byte(tt.manifest), 0o644); err != nil {
				t.Fatalf("write bad manifest: %v", err)
			}

			before, err := os.ReadFile(manifestPath)
			if err != nil {
				t.Fatalf("read manifest before update: %v", err)
			}

			result := engineRuntime.NewCodexAdapter(root).Update()
			if result.Status != engineRuntime.CapabilityPartial {
				t.Fatalf("Update() = %#v", result)
			}

			after, err := os.ReadFile(manifestPath)
			if err != nil {
				t.Fatalf("read manifest after failed update: %v", err)
			}
			if string(after) != string(before) {
				t.Fatalf("Update() should not modify existing manifest: before=%q after=%q", string(before), string(after))
			}
			if !strings.Contains(result.Message, "existing Codex manifest") {
				t.Fatalf("failure message should mention existing Codex manifest; got %q", result.Message)
			}
		})
	}
}

func TestCodexStatusReportsPartialWithActivationUncertainty(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewCodexAdapter(root)

	if result := adapter.Install(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}

	status := adapter.Status()
	if status.Status != engineRuntime.CapabilityPartial {
		t.Fatalf("Status() after install = %#v", status)
	}
	if !strings.Contains(strings.ToLower(status.Message), "activation") {
		t.Fatalf("Status() message should mention activation/reload uncertainty, got %q", status.Message)
	}
	if status.Message == "" {
		t.Fatal("Status() message should not be empty")
	}
	if len(status.Reasons) == 0 {
		t.Fatal("Status() should include reasons when activation/reload proof is unavailable")
	}
	var reason string
	for _, item := range status.Reasons {
		reason = reason + " " + item
	}
	if !strings.Contains(strings.ToLower(reason), "activation") && !strings.Contains(strings.ToLower(reason), "reload") {
		t.Fatalf("Status() reasons should include activation/reload uncertainty; got %#v", status.Reasons)
	}
}

func TestCodexUninstallRemovesManifestWithoutTouchingUnrelatedFiles(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, "labdrian-runtime-lifecycle.json")
	if err := os.WriteFile(manifestPath, []byte(`{"managed_by":"labdrian-sdd-overlay","installed_version":"legacy","config_root":"`+root+`"}`), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	userPath := filepath.Join(root, "user.json")
	if err := os.WriteFile(userPath, []byte(`{"keep":"me"}`), 0o644); err != nil {
		t.Fatalf("write unrelated user file: %v", err)
	}

	adapter := engineRuntime.NewCodexAdapter(root)
	result := adapter.Uninstall()
	if result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Uninstall() = %#v", result)
	}
	if _, err := os.Stat(manifestPath); !os.IsNotExist(err) {
		t.Fatalf("manifest should be removed, stat err: %v", err)
	}
	if _, err := os.Stat(userPath); err != nil {
		t.Fatalf("unrelated user file should remain, stat err: %v", err)
	}
}

func TestCodexUninstallRejectsUnownedOrMalformedManifest(t *testing.T) {
	for _, tt := range []struct {
		name     string
		manifest string
	}{
		{
			name:     "unowned manifest",
			manifest: `{"managed_by":"other","installed_version":"legacy","config_root":"/tmp/other"}`,
		},
		{
			name:     "malformed manifest",
			manifest: `not-json`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			manifestPath := filepath.Join(root, "labdrian-runtime-lifecycle.json")
			if err := os.WriteFile(manifestPath, []byte(tt.manifest), 0o644); err != nil {
				t.Fatalf("write bad manifest: %v", err)
			}

			before, err := os.ReadFile(manifestPath)
			if err != nil {
				t.Fatalf("read manifest before uninstall: %v", err)
			}

			result := engineRuntime.NewCodexAdapter(root).Uninstall()
			if result.Status != engineRuntime.CapabilityPartial {
				t.Fatalf("Uninstall() = %#v", result)
			}

			after, err := os.ReadFile(manifestPath)
			if err != nil {
				t.Fatalf("read manifest after failed uninstall: %v", err)
			}
			if string(after) != string(before) {
				t.Fatalf("Uninstall() should not modify existing manifest: before=%q after=%q", string(before), string(after))
			}
			if !strings.Contains(result.Message, "existing Codex manifest") {
				t.Fatalf("failure message should mention existing Codex manifest; got %q", result.Message)
			}
		})
	}
}

func TestCodexStatusClassifiesMissingOrInvalidManifestAsPartial(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewCodexAdapter(root)

	status := adapter.Status()
	if status.Status != engineRuntime.CapabilityPartial {
		t.Fatalf("Status() without manifest = %#v, want partial", status)
	}

	manifestPath := filepath.Join(root, "labdrian-runtime-lifecycle.json")
	if err := os.WriteFile(manifestPath, []byte(`not-json`), 0o644); err != nil {
		t.Fatalf("write invalid manifest: %v", err)
	}
	status = adapter.Status()
	if status.Status != engineRuntime.CapabilityPartial {
		t.Fatalf("Status() with invalid manifest = %#v, want partial", status)
	}
}

func TestCodexMutationFailureIsSafeAndDoesNotClobberRootFile(t *testing.T) {
	rootFile := filepath.Join(t.TempDir(), "codex-root-file")
	if err := os.WriteFile(rootFile, []byte("preexisting"), 0o644); err != nil {
		t.Fatalf("write root file: %v", err)
	}

	adapter := engineRuntime.NewCodexAdapter(rootFile)
	result := adapter.Install()
	if result.Status != engineRuntime.CapabilityPartial {
		t.Fatalf("Install() with non-directory root = %#v, want partial", result)
	}

	after, err := os.ReadFile(rootFile)
	if err != nil {
		t.Fatalf("read root file after failed install: %v", err)
	}
	if string(after) != "preexisting" {
		t.Fatalf("failed mutation should not clobber root file; got %q", string(after))
	}
}

func readCodexManifest(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read manifest %s: %v", path, err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode manifest %s: %v\n%s", path, err, string(data))
	}
	return manifest
}
