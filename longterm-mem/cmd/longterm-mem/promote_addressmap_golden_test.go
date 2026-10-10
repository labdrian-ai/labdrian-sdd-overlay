package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vault"
)

// The golden files under testdata/addressmap-golden pin what `longterm-mem promote` and `longterm-mem sync`
// do to a vault whose address map, .raw/.manifest.json, is in each state the file can be found in: with keys
// of other tools, a stale entry, not JSON, empty, an array, an address_map of the wrong type, a directory, a
// symbolic link, a dangling link. After the two commands of a scenario they hold the exit code, stdout and
// stderr and every file of the vault, with its mode and, for a link, where it points, character for
// character. They were recorded from the program as it stood before the address map moved behind a port
// (Phase 9, L3 slice 2), and that change must not alter one byte of them. The two scenarios added since,
// a manifest that is the JSON null and an address_map that is, were recorded after the owner's decisions
// about them.
//
// Two kinds of text in them come from outside the program: the operating system's words for an error ("is a
// directory", "no such file or directory"), which Linux and macOS spell alike (CI runs on Linux; the module
// does not build for Windows), and the wording of encoding/json's decode errors, which belongs to the Go
// toolchain. TestGoldenPinsTheJSONErrorsOfTheToolchain names the second: a toolchain that words them
// differently fails that test first, and the goldens then need re-recording for that reason alone.
//
// To rewrite them after a change that is MEANT to alter the output:
//
//	go test ./cmd/longterm-mem -run TestPromoteAddressMapGolden -update-promote-golden
//
// and read the diff before committing it.
type addressMapScenario struct {
	name string
	// seed leaves the address map in the state the scenario is about; root is the vault.
	seed func(t *testing.T, root string)
}

// writeManifest writes the vault's .raw/.manifest.json with the given content and mode.
func writeManifest(t *testing.T, root, content string, mode os.FileMode) {
	t.Helper()
	dir := filepath.Join(root, ".raw")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".manifest.json")
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

// symlinkOrSkip makes newname a symbolic link to oldname, and skips the test on a file system that cannot make
// one: a scenario about links is about a state the file system must be able to hold.
func symlinkOrSkip(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("this file system cannot make a symbolic link: %v", err)
	}
}

// temporaryFileName is the random suffix of the file an atomic write creates beside its target, which an error
// message may name.
var temporaryFileName = regexp.MustCompile(`\.tmp-\d+`)

func addressMapScenarios() []addressMapScenario {
	return []addressMapScenario{
		{name: "01-a-manifest-with-keys-of-other-tools", seed: func(t *testing.T, root string) {
			writeManifest(t, root, `{"version":2,"sources":{"notes/a.md":"hash-a"},"extra":{"keep":[1,2,3]},"address_map":{"wiki/memory/c-000100.md":"c-000100"}}`, 0o644)
		}},
		{name: "02-a-stale-entry-for-the-new-path-is-replaced", seed: func(t *testing.T, root string) {
			writeManifest(t, root, `{"version":1,"address_map":{"wiki/memory/c-000901.md":"c-000555"}}`, 0o640)
		}},
		{name: "03-a-manifest-that-is-not-json", seed: func(t *testing.T, root string) {
			writeManifest(t, root, "{not json", 0o644)
		}},
		{name: "04-an-empty-manifest", seed: func(t *testing.T, root string) {
			writeManifest(t, root, "", 0o644)
		}},
		{name: "05-a-manifest-that-is-a-json-array", seed: func(t *testing.T, root string) {
			writeManifest(t, root, "[]", 0o644)
		}},
		{name: "06-an-address-map-that-is-a-string", seed: func(t *testing.T, root string) {
			writeManifest(t, root, `{"address_map":"nope"}`, 0o644)
		}},
		{name: "07-an-address-map-with-a-value-that-is-not-a-string", seed: func(t *testing.T, root string) {
			writeManifest(t, root, `{"address_map":{"wiki/memory/c-000100.md":1}}`, 0o644)
		}},
		{name: "08-a-manifest-that-is-a-directory", seed: func(t *testing.T, root string) {
			if err := os.MkdirAll(filepath.Join(root, ".raw", ".manifest.json"), 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "09-a-manifest-that-is-a-link-to-another-file-of-the-vault", seed: func(t *testing.T, root string) {
			target := filepath.Join(root, "notes", "manifest.json")
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, []byte(`{"version":1,"address_map":{"wiki/memory/c-000100.md":"c-000100"}}`), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(root, ".raw"), 0o755); err != nil {
				t.Fatal(err)
			}
			symlinkOrSkip(t, "../notes/manifest.json", filepath.Join(root, ".raw", ".manifest.json"))
		}},
		{name: "10-a-manifest-that-is-a-link-to-nothing", seed: func(t *testing.T, root string) {
			if err := os.MkdirAll(filepath.Join(root, ".raw"), 0o755); err != nil {
				t.Fatal(err)
			}
			symlinkOrSkip(t, "../notes/absent.json", filepath.Join(root, ".raw", ".manifest.json"))
		}},
		{name: "11-no-manifest-and-no-raw-directory", seed: func(t *testing.T, root string) {}},
		// Recorded after the owner's decision of 2026-10-10: a manifest that is the JSON null is not an object, so
		// it is unparseable like any other file that is not one. Before it, promote and sync panicked here
		// (assignment to entry in nil map, exit 2), so no golden of the earlier program exists for it.
		{name: "12-a-manifest-that-is-json-null", seed: func(t *testing.T, root string) {
			writeManifest(t, root, "null", 0o644)
		}},
		// Recorded after the owner's decision of 2026-10-10: an address_map of JSON null is an empty map, as the
		// doctor already reads it. Before it, promote and sync panicked here too.
		{name: "13-an-address-map-that-is-json-null", seed: func(t *testing.T, root string) {
			writeManifest(t, root, `{"version":2,"extra":{"keep":true},"address_map":null}`, 0o644)
		}},
	}
}

// renderVaultWithModes lists every file of the vault, sorted: a regular file with its mode and content, a
// link with where it points, and a directory with its mode.
func renderVaultWithModes(t *testing.T, root string) string {
	t.Helper()
	var paths []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path != root {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk the vault: %v", err)
	}
	sort.Strings(paths)
	var b strings.Builder
	for _, path := range paths {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				t.Fatal(err)
			}
			fmt.Fprintf(&b, "=== %s (link to %s) ===\n", filepath.ToSlash(rel), target)
		case info.IsDir():
			fmt.Fprintf(&b, "=== %s/ (directory %v) ===\n", filepath.ToSlash(rel), info.Mode().Perm())
		default:
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			fmt.Fprintf(&b, "=== %s (%v) ===\n%s", filepath.ToSlash(rel), info.Mode().Perm(), data)
			if !strings.HasSuffix(string(data), "\n") {
				b.WriteString("\n")
			}
		}
	}
	return b.String()
}

// The decode errors the goldens print are the toolchain's, not the program's. If this test fails after a Go
// upgrade, the goldens that quote them (05, 06 and 07) have changed for that reason and for no other.
func TestGoldenPinsTheJSONErrorsOfTheToolchain(t *testing.T) {
	var asObject map[string]json.RawMessage
	var asStrings map[string]string
	for _, c := range []struct {
		name  string
		err   error
		quote string
	}{
		{"an array where the manifest is an object", json.Unmarshal([]byte("[]"), &asObject), "json: cannot unmarshal array into Go value of type map[string]json.RawMessage"},
		{"a string where address_map is an object", json.Unmarshal([]byte(`"nope"`), &asStrings), "json: cannot unmarshal string into Go value of type map[string]string"},
		{"a number where an address is a string", json.Unmarshal([]byte(`{"a":1}`), &asStrings), "json: cannot unmarshal number into Go value of type string"},
	} {
		if c.err == nil || c.err.Error() != c.quote {
			t.Errorf("%s: encoding/json says %v, where the goldens quote %q", c.name, c.err, c.quote)
		}
	}
}

func TestPromoteAddressMapGolden(t *testing.T) {
	if !vault.PrerequisitePresent("python3") {
		t.Skip("python3 is not on PATH: the vault's index rebuild runs its scripts under it")
	}
	goldenDir, err := filepath.Abs(filepath.Join("testdata", "addressmap-golden"))
	if err != nil {
		t.Fatalf("resolve the golden directory: %v", err)
	}
	for _, scenario := range addressMapScenarios() {
		t.Run(scenario.name, func(t *testing.T) {
			fixedUmask(t)
			w := newPromoteGoldenWorld(t)
			t.Setenv("HOME", w.home)
			t.Setenv("LONGTERM_MEM_ENGRAM_DB", w.dbPath)
			t.Setenv("LONGTERM_MEM_VAULT", w.vaultRoot)
			t.Setenv("LONGTERM_MEM_VAULTS_FILE", "")
			t.Chdir(t.TempDir())
			scenario.seed(t, w.vaultRoot)

			var out strings.Builder
			for _, args := range [][]string{
				{"promote", "--project", promoteGoldenProject, "--id", "1"},
				{"sync", "--project", promoteGoldenProject},
			} {
				code, stdout, stderr := runCaptured(t, args)
				fmt.Fprintf(&out, "args: %s\nexit: %d\n--- stdout ---\n%s--- stderr ---\n%s", strings.Join(args, " "), code, stdout, stderr)
			}
			out.WriteString("--- vault ---\n" + renderVaultWithModes(t, w.vaultRoot))
			checkPromoteGolden(t, goldenDir, scenario.name, temporaryFileName.ReplaceAllString(w.normalise(out.String()), ".tmp-<n>"))
		})
	}
}
