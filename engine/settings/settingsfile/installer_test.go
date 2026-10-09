package settingsfile_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings/settingsfile"
)

// binary is the hook command the tests install for.
const binary = "/opt/h/bin"

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestInstallCreatesTheFileAndUninstallRemovesOnlyOurEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	installer := settingsfile.Installer{}

	if err := installer.Install(path, binary); err != nil {
		t.Fatalf("Install() = %v", err)
	}
	found, owned, err := installer.Inspect(path, binary)
	if err != nil || !found || !owned {
		t.Fatalf("Inspect() = %v, %v, %v after Install, want found and owned", found, owned, err)
	}
	if err := installer.Uninstall(path, binary); err != nil {
		t.Fatalf("Uninstall() = %v", err)
	}
	found, owned, err = installer.Inspect(path, binary)
	if err != nil || !found || owned {
		t.Errorf("Inspect() = %v, %v, %v after Uninstall, want the file there and not owned", found, owned, err)
	}
}

func TestInstallTwiceWritesNothingTheSecondTime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	installer := settingsfile.Installer{}
	if err := installer.Install(path, binary); err != nil {
		t.Fatal(err)
	}
	if err := installer.Install(path, binary); err != nil {
		t.Fatalf("second Install() = %v", err)
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Errorf("a second install made a backup, so it wrote: %v", err)
	}
}

func TestEmptyHookCommandIsRefusedBeforeTheFileIsLookedAt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-dir", "settings.json")
	installer := settingsfile.Installer{}

	if err := installer.Install(path, ""); !errors.Is(err, settings.ErrEmptyHookCommand) {
		t.Errorf("Install() = %v, want ErrEmptyHookCommand", err)
	}
	if err := installer.Uninstall(path, ""); !errors.Is(err, settings.ErrEmptyHookCommand) {
		t.Errorf("Uninstall() = %v, want ErrEmptyHookCommand", err)
	}
}

func TestUninstallOfAFileThatIsNotThereIsNotAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := (settingsfile.Installer{}).Uninstall(path, binary); err != nil {
		t.Fatalf("Uninstall() = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("Uninstall created the file: %v", err)
	}
}

func TestInvalidJSONIsNamedAndTheFileIsNotModified(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	write(t, path, "not json")
	installer := settingsfile.Installer{}

	for name, act := range map[string]func(string, string) error{"Install": installer.Install, "Uninstall": installer.Uninstall} {
		err := act(path, binary)
		if err == nil || !strings.Contains(err.Error(), path+" contains invalid JSON (not modified)") {
			t.Errorf("%s() = %v, want it to name the file and say it was not modified", name, err)
		}
	}
	if data, _ := os.ReadFile(path); string(data) != "not json" {
		t.Errorf("the file was modified: %q", data)
	}
}

// Install into a settings.json that holds null used to crash the process with a write to a nil
// map; it is refused like any value that is not an object, and uninstall has nothing to do.
func TestJSONNullIsRefusedByInstallAndIgnoredByUninstall(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	write(t, path, "null")
	installer := settingsfile.Installer{}

	err := installer.Install(path, binary)
	if !errors.Is(err, settings.ErrNotAnObject) || !strings.Contains(err.Error(), "contains invalid JSON (not modified)") {
		t.Errorf("Install() = %v, want the invalid JSON error wrapping ErrNotAnObject", err)
	}
	if err := installer.Uninstall(path, binary); err != nil {
		t.Errorf("Uninstall() = %v, want nothing to do", err)
	}
	if data, _ := os.ReadFile(path); string(data) != "null" {
		t.Errorf("the file was modified: %q", data)
	}
}

func TestInspectSaysWhatTheFileHolds(t *testing.T) {
	dir := t.TempDir()
	installer := settingsfile.Installer{}
	installed := filepath.Join(dir, "installed.json")
	if err := installer.Install(installed, binary); err != nil {
		t.Fatal(err)
	}
	partial := filepath.Join(dir, "partial.json")
	write(t, partial, `{"hooks":{}}`)
	null := filepath.Join(dir, "null.json")
	write(t, null, "null")
	broken := filepath.Join(dir, "broken.json")
	write(t, broken, "{")

	cases := []struct {
		name         string
		path, binary string
		found, owned bool
		wantErr      bool
	}{
		{"installed and owned", installed, binary, true, true, false},
		{"installed, another binary", installed, "/opt/other/bin", true, false, false},
		{"present but empty of hooks", partial, binary, true, false, false},
		{"null holds nothing", null, binary, false, false, false},
		{"absent", filepath.Join(dir, "none.json"), binary, false, false, false},
		{"not JSON", broken, binary, false, false, true},
		{"a directory", dir, binary, false, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			found, owned, err := installer.Inspect(c.path, c.binary)
			if (err != nil) != c.wantErr || found != c.found || owned != c.owned {
				t.Errorf("Inspect() = %v, %v, %v, want found=%v owned=%v error=%v", found, owned, err, c.found, c.owned, c.wantErr)
			}
		})
	}
}

// The error of a file that cannot be parsed is the one encoding/json reports, untouched, so the
// runtime adapter can word it as it always has.
func TestInspectReturnsTheParseErrorUnwrapped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	write(t, path, "")

	_, _, err := settingsfile.Installer{}.Inspect(path, binary)

	if err == nil || err.Error() != "unexpected end of JSON input" {
		t.Errorf("Inspect() error = %v, want the encoding/json error as it is", err)
	}
}
