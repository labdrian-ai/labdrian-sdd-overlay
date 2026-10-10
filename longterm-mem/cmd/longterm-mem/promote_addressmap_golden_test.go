package main

import (
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
// (Phase 9, L3 slice 2), and that change must not alter one byte of them.
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
			if err := os.Symlink("../notes/manifest.json", filepath.Join(root, ".raw", ".manifest.json")); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "10-a-manifest-that-is-a-link-to-nothing", seed: func(t *testing.T, root string) {
			if err := os.MkdirAll(filepath.Join(root, ".raw"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("../notes/absent.json", filepath.Join(root, ".raw", ".manifest.json")); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "11-no-manifest-and-no-raw-directory", seed: func(t *testing.T, root string) {}},
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
