package ops

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/ops/testdata"
)

// The golden files under testdata/golden pin what Doctor and Status report about a vault, character
// for character, for vaults in each state the two commands read: healthy, empty, missing, and damaged in the
// ways their checks name. They were recorded from the program as it stood before the vault's file system
// moved behind a port (Phase 9, L3), and that change must not alter one byte of them. The only thing the
// test replaces is the temporary directory, which differs from one run to the next.
//
// To rewrite them after a change that is MEANT to alter the output:
//
//	go test ./internal/ops -run TestGolden -update-ops-golden
//
// and read the diff before committing it.
var updateOpsGolden = flag.Bool("update-ops-golden", false, "rewrite the golden files of doctor and status")

// goldenAddress and goldenTitle name the promoted page every scenario starts from.
const (
	goldenAddress = "c-000042"
	goldenTitle   = "Widget Decision"
)

// goldenScenario is a vault in one state: it starts as the healthy vault (or as an empty directory when bare
// is set) and build breaks it.
type goldenScenario struct {
	name string
	bare bool
	// build edits the vault; root is its directory.
	build func(t *testing.T, root string)
	// root, when set, is the vault root handed to Doctor and Status instead of the scenario's directory.
	root func(dir string) string
}

func writeGoldenFile(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", rel, err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func goldenScenarios() []goldenScenario {
	return []goldenScenario{
		{name: "01-a-healthy-vault"},
		{name: "02-an-empty-vault", bare: true},
		{name: "03-a-vault-that-does-not-exist", bare: true, root: func(dir string) string { return filepath.Join(dir, "absent") }},
		{name: "04-a-vault-root-that-is-a-file", bare: true, root: func(dir string) string {
			path := filepath.Join(dir, "file")
			if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
				panic(err)
			}
			return path
		}},
		{name: "05-an-address-map-without-the-page", build: func(t *testing.T, root string) {
			testdata.WriteAddressMap(t, root, map[string]string{})
		}},
		{name: "06-a-page-registered-nowhere", build: func(t *testing.T, root string) {
			for _, rel := range []string{"wiki/index.md", "wiki/log.md"} {
				if err := os.Remove(filepath.Join(root, rel)); err != nil {
					t.Fatal(err)
				}
			}
		}},
		{name: "07-a-page-the-sidecar-does-not-know", build: func(t *testing.T, root string) {
			if err := os.Remove(filepath.Join(root, ".raw", ".longterm-mem-manifest.json")); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "08-a-sidecar-that-is-not-json", build: func(t *testing.T, root string) {
			writeGoldenFile(t, root, ".raw/.longterm-mem-manifest.json", "{not json")
		}},
		{name: "09-a-sidecar-that-is-a-directory", build: func(t *testing.T, root string) {
			full := filepath.Join(root, ".raw", ".longterm-mem-manifest.json")
			if err := os.Remove(full); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(full, 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "10-a-wedged-page-with-no-recorded-revision", build: func(t *testing.T, root string) {
			editPromotedPage(t, root, goldenAddress)
		}},
		{name: "11-an-edited-page-with-a-recorded-revision", build: func(t *testing.T, root string) {
			recordPrecedenceRevision(t, root, goldenAddress, 3)
			editPromotedPage(t, root, goldenAddress)
		}},
		{name: "12-a-page-that-cannot-be-read", build: func(t *testing.T, root string) {
			if err := os.Symlink(filepath.Join(root, "nowhere"), filepath.Join(root, "wiki", "memory", "c-000043.md")); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "13-a-pages-directory-that-cannot-be-listed", build: func(t *testing.T, root string) {
			dir := filepath.Join(root, "wiki", "memory")
			if err := os.RemoveAll(dir); err != nil {
				t.Fatal(err)
			}
			writeGoldenFile(t, root, "wiki/memory", "not a directory")
		}},
		{name: "14-a-log-that-does-not-name-the-page", build: func(t *testing.T, root string) {
			writeGoldenFile(t, root, "wiki/log.md", "# Log\n")
		}},
		{name: "15-a-sync-state-of-a-finished-sync", build: func(t *testing.T, root string) {
			testdata.WriteSyncState(t, root, "2026-08-31T12:00:00Z")
		}},
		{name: "16-a-sync-state-that-is-not-json", build: func(t *testing.T, root string) {
			writeGoldenFile(t, root, ".vault-meta/longterm-mem-sync-state.json", "{not json")
		}},
		{name: "17-a-sync-state-with-no-completion", build: func(t *testing.T, root string) {
			writeGoldenFile(t, root, ".vault-meta/longterm-mem-sync-state.json", `{"schema":1}`)
		}},
		{name: "18-a-sync-state-that-is-a-directory", build: func(t *testing.T, root string) {
			if err := os.MkdirAll(filepath.Join(root, ".vault-meta", "longterm-mem-sync-state.json"), 0o755); err != nil {
				t.Fatal(err)
			}
		}},
	}
}

// buildGoldenVault fills root with the healthy vault: one promoted page, its address-map entry, its
// precedence record and its catalog and log entries.
func buildGoldenVault(t *testing.T, root string) {
	t.Helper()
	page := testdata.WritePromotedPage(t, root, goldenAddress, goldenTitle)
	testdata.WriteAddressMap(t, root, map[string]string{"wiki/memory/" + goldenAddress + ".md": goldenAddress})
	testdata.WritePrecedenceEntry(t, root, page)
	testdata.RegisterPage(t, root, goldenAddress, goldenTitle)
}

func TestGolden(t *testing.T) {
	goldenDir, err := filepath.Abs(filepath.Join("testdata", "golden"))
	if err != nil {
		t.Fatalf("resolve the golden directory: %v", err)
	}
	for _, scenario := range goldenScenarios() {
		t.Run(scenario.name, func(t *testing.T) {
			dir, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatalf("resolve the temporary directory: %v", err)
			}
			if !scenario.bare {
				buildGoldenVault(t, dir)
			}
			if scenario.build != nil {
				scenario.build(t, dir)
			}
			vaultRoot := dir
			if scenario.root != nil {
				vaultRoot = scenario.root(dir)
			}
			stateDir, liveIDs := newHealthyEmbeddingDeps(t)

			doctor, err := Doctor(context.Background(), DoctorDeps{
				VaultRoot:             vaultRoot,
				PrerequisitePresent:   func(string) bool { return true },
				StateDir:              stateDir,
				LiveObservationIDs:    func(string) ([]int64, error) { return liveIDs, nil },
				EmbeddingBackendCheck: func(context.Context) error { return nil },
			}, doctorTestProject)
			if err != nil {
				t.Fatalf("Doctor: %v", err)
			}
			doctorJSON, err := json.MarshalIndent(doctor, "", "  ")
			if err != nil {
				t.Fatalf("encode the doctor report: %v", err)
			}

			var statusText string
			status, err := Status(context.Background(), StatusDeps{
				VaultRoot:        vaultRoot,
				StateDir:         stateDir,
				VaultProvisioned: func(string) bool { return false },
				EngramReachable:  func(context.Context) (bool, string) { return true, "" },
			}, doctorTestProject)
			if err != nil {
				statusText = "error: " + err.Error()
			} else {
				statusJSON, err := json.MarshalIndent(status, "", "  ")
				if err != nil {
					t.Fatalf("encode the status report: %v", err)
				}
				statusText = string(statusJSON)
			}

			got := "--- doctor ---\n" + string(doctorJSON) + "\n--- status ---\n" + statusText + "\n"
			got = strings.ReplaceAll(got, dir, "<vault>")
			checkOpsGolden(t, goldenDir, scenario.name, got)
		})
	}
}

// checkOpsGolden compares got with the golden file of the scenario, or rewrites that file when the update
// flag is given.
func checkOpsGolden(t *testing.T, dir, name, got string) {
	t.Helper()
	path := filepath.Join(dir, name+".golden")
	if *updateOpsGolden {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("create the golden directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden file: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden file: %v (record it with -update-ops-golden)", err)
	}
	if string(want) != got {
		t.Errorf("the scenario %q changed from its golden file %s:\n--- want ---\n%s\n--- got ---\n%s", name, path, want, got)
	}
}
