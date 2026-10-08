package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

// The settings file of Pi is read once and parsed from bytes: what the adapter asks of it
// afterwards (is my package listed, is the Subagents extension there, are the native subagents
// of gentle-pi there) is answered from the same parsed value, never from a second read.

func TestParsePiSettingsListsAPackageByAbsoluteOrRelativePath(t *testing.T) {
	home := "/home/p"
	settings := parsePiSettings(home, []byte(`{"packages":["/abs/pkg","../../rel/pkg","npm:other"]}`))
	for path, want := range map[string]bool{
		"/abs/pkg":          true,
		"/abs/pkg/":         true,
		"/home/p/rel/pkg":   true, // ../../rel/pkg resolved against /home/p/.pi/agent
		"/home/p/other/pkg": false,
		"npm:other":         false,
	} {
		if got := settings.listsPackage(path); got != want {
			t.Errorf("listsPackage(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestParsePiSettingsFindsTheSubagentsExtensionUnderEitherName(t *testing.T) {
	for raw, want := range map[string]bool{
		`{"packages":["npm:pi-subagents-j0k3r"]}`:        true,
		`{"packages":["npm:pi-subagents-j0k3r@1.5.15"]}`: true,
		`{"packages":["npm:pi-subagents"]}`:              true,
		`{"packages":["npm:pi-subagents@2.0.0"]}`:        true,
		`{"packages":["npm:pi-subagents-other"]}`:        false,
		`{"packages":[]}`:                                false,
		`{}`:                                             false,
	} {
		if got := parsePiSettings("/h", []byte(raw)).listsSubagentsExtension(); got != want {
			t.Errorf("%s: listsSubagentsExtension() = %v, want %v", raw, got, want)
		}
	}
}

func TestParsePiSettingsMeansNothingByInvalidContent(t *testing.T) {
	for _, raw := range []string{``, `not json`, `{"packages":"npm:pi-subagents"}`, `{"packages":[1,2]}`, `[]`} {
		settings := parsePiSettings("/h", []byte(raw))
		if settings.listsPackage("/p") || settings.listsSubagentsExtension() || settings.nativeSubagents() {
			t.Errorf("%q: settings that do not parse list something: %+v", raw, settings)
		}
	}
}

func TestPiSettingsNativeSubagentsFollowTheVersionOfGentlePi(t *testing.T) {
	home := t.TempDir()
	writeGentlePiVersion := func(version string) {
		dir := filepath.Join(home, ".pi", "agent", "npm", "node_modules", "gentle-pi")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"version":"`+version+`"}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for raw, want := range map[string]bool{
		`{"packages":["npm:gentle-pi@2.6.0"]}`:       true,
		`{"packages":["npm:gentle-pi@2.6.1-rc.1"]}`:  true,
		`{"packages":["npm:gentle-pi@3.0.0"]}`:       true,
		`{"packages":["npm:gentle-pi@2.5.9"]}`:       false,
		`{"packages":["npm:gentle-pi@garbage"]}`:     false,
		`{"packages":["npm:gentle-pi-other@9.0.0"]}`: false,
		`{"packages":[]}`:                            false,
	} {
		if got := parsePiSettings(home, []byte(raw)).nativeSubagents(); got != want {
			t.Errorf("%s: nativeSubagents() = %v, want %v", raw, got, want)
		}
	}

	// An unversioned entry asks the installed package for its version.
	unversioned := parsePiSettings(home, []byte(`{"packages":["npm:gentle-pi"]}`))
	if unversioned.nativeSubagents() {
		t.Error("an unversioned gentle-pi with no installed package.json counts as native")
	}
	writeGentlePiVersion("2.5.0")
	if unversioned.nativeSubagents() {
		t.Error("an unversioned gentle-pi installed at 2.5.0 counts as native")
	}
	writeGentlePiVersion("2.6.0")
	if !unversioned.nativeSubagents() {
		t.Error("an unversioned gentle-pi installed at 2.6.0 does not count as native")
	}
}

func TestPiSettingsRunnerStateNamesTheFourCases(t *testing.T) {
	cases := map[string]subagentRunner{
		`{"packages":["npm:gentle-pi@2.6.0","npm:pi-subagents-j0k3r"]}`: subagentRunnerConflict,
		`{"packages":["npm:gentle-pi@2.6.0"]}`:                          subagentRunnerNative,
		`{"packages":["npm:pi-subagents"]}`:                             subagentRunnerLegacy,
		`{"packages":["npm:gentle-pi@2.5.0"]}`:                          subagentRunnerAbsent,
	}
	for raw, want := range cases {
		if got := parsePiSettings("/h", []byte(raw)).subagentRunner(); got != want {
			t.Errorf("%s: subagentRunner() = %q, want %q", raw, got, want)
		}
	}
}

func TestReadPiSettingsWithoutAHomeReadsNothing(t *testing.T) {
	if got := readPiSettings(""); got.listsPackage("/p") || got.listsSubagentsExtension() || got.nativeSubagents() {
		t.Errorf("settings without a home list something: %+v", got)
	}
}

func TestReadPiSettingsReadsTheFileUnderTheHome(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".pi", "agent", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"packages":["/abs/pkg","npm:pi-subagents"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got := readPiSettings(home)
	if !got.listsPackage("/abs/pkg") || !got.listsSubagentsExtension() {
		t.Errorf("readPiSettings(%q) = %+v, want the packages of the file", home, got)
	}
	if missing := readPiSettings(t.TempDir()); missing.listsPackage("/abs/pkg") {
		t.Errorf("a home with no settings file lists a package: %+v", missing)
	}
}
