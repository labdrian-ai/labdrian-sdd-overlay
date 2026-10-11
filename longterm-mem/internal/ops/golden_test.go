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
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultfs"
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
	// goldenOtherAddress is the address of the second page the scenarios put beside the healthy one.
	goldenOtherAddress = "c-000043"
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

// symlinkOrSkip makes newname a symbolic link to oldname, and skips the test on a file system that cannot make
// one: a scenario about links is about a state the file system must be able to hold.
func symlinkOrSkip(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("this file system cannot make a symbolic link: %v", err)
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
			symlinkOrSkip(t, filepath.Join(root, "nowhere"), filepath.Join(root, "wiki", "memory", "c-000043.md"))
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
		// The address map, .raw/.manifest.json, in each state the file can be found in.
		{name: "19-an-address-map-that-is-not-json", build: func(t *testing.T, root string) {
			writeGoldenFile(t, root, ".raw/.manifest.json", "{not json")
		}},
		{name: "20-an-empty-address-map-file", build: func(t *testing.T, root string) {
			writeGoldenFile(t, root, ".raw/.manifest.json", "")
		}},
		{name: "21-an-address-map-file-that-is-a-json-array", build: func(t *testing.T, root string) {
			writeGoldenFile(t, root, ".raw/.manifest.json", "[]")
		}},
		{name: "22-an-address-map-that-is-a-string", build: func(t *testing.T, root string) {
			writeGoldenFile(t, root, ".raw/.manifest.json", `{"address_map":"nope"}`)
		}},
		{name: "23-an-address-map-with-a-value-that-is-not-a-string", build: func(t *testing.T, root string) {
			writeGoldenFile(t, root, ".raw/.manifest.json", `{"address_map":{"wiki/memory/c-000042.md":42}}`)
		}},
		{name: "24-a-manifest-without-an-address-map", build: func(t *testing.T, root string) {
			writeGoldenFile(t, root, ".raw/.manifest.json", `{"version":1}`)
		}},
		// Re-recorded after the owner's decision of 2026-10-10: a manifest that is the JSON null is not an
		// object, so the doctor reads it as it reads any file that is not one (the output of scenario 19), where
		// it used to read it as an empty map. It is the one golden of the address map that changed.
		{name: "25-a-manifest-that-is-null", build: func(t *testing.T, root string) {
			writeGoldenFile(t, root, ".raw/.manifest.json", "null")
		}},
		{name: "26-the-address-mapped-to-another-page", build: func(t *testing.T, root string) {
			writeGoldenFile(t, root, ".raw/.manifest.json", `{"address_map":{"wiki/memory/elsewhere.md":"`+goldenAddress+`"}}`)
		}},
		{name: "27-an-address-map-file-that-is-a-directory", build: func(t *testing.T, root string) {
			full := filepath.Join(root, ".raw", ".manifest.json")
			if err := os.Remove(full); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(full, 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "28-an-address-map-file-that-is-a-link", build: func(t *testing.T, root string) {
			full := filepath.Join(root, ".raw", ".manifest.json")
			data, err := os.ReadFile(full)
			if err != nil {
				t.Fatal(err)
			}
			writeGoldenFile(t, root, "notes/manifest.json", string(data))
			if err := os.Remove(full); err != nil {
				t.Fatal(err)
			}
			symlinkOrSkip(t, "../notes/manifest.json", full)
		}},
		{name: "29-an-address-map-file-that-is-a-link-to-nothing", build: func(t *testing.T, root string) {
			full := filepath.Join(root, ".raw", ".manifest.json")
			if err := os.Remove(full); err != nil {
				t.Fatal(err)
			}
			symlinkOrSkip(t, "../notes/absent.json", full)
		}},
		// Scenario 28 cannot tell a link that is followed from one that is refused: the map it points to is
		// the healthy one, and a doctor that ignored the link would find the same map by reading the file
		// beside it. This one points at a map that disagrees with the page, so that the finding it
		// produces, the one of scenario 26, can only come from the file the link names.
		{name: "30-an-address-map-file-that-is-a-link-to-a-map-that-disagrees", build: func(t *testing.T, root string) {
			full := filepath.Join(root, ".raw", ".manifest.json")
			writeGoldenFile(t, root, "notes/manifest.json", `{"address_map":{"wiki/memory/elsewhere.md":"`+goldenAddress+`"}}`)
			if err := os.Remove(full); err != nil {
				t.Fatal(err)
			}
			symlinkOrSkip(t, "../notes/manifest.json", full)
		}},
		// The pages, wiki/memory/<address>.md, in each state a file of the directory can be found in; then the
		// pages directory itself. Recorded for the move of the pages behind a port (Phase 9, L3 slice 3), from
		// the program as it stood before it.
		{name: "31-a-page-that-is-a-directory", build: func(t *testing.T, root string) {
			if err := os.Mkdir(filepath.Join(root, "wiki", "memory", "c-000043.md"), 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "32-a-page-that-is-a-link-to-another-page", build: func(t *testing.T, root string) {
			symlinkOrSkip(t, goldenAddress+".md", filepath.Join(root, "wiki", "memory", "c-000043.md"))
		}},
		{name: "33-a-page-that-is-a-link-to-a-directory", build: func(t *testing.T, root string) {
			if err := os.Mkdir(filepath.Join(root, "wiki", "folder"), 0o755); err != nil {
				t.Fatal(err)
			}
			symlinkOrSkip(t, "../folder", filepath.Join(root, "wiki", "memory", "c-000043.md"))
		}},
		{name: "34-a-page-with-no-frontmatter", build: func(t *testing.T, root string) {
			writeGoldenFile(t, root, "wiki/memory/c-000043.md", "Just text, no frontmatter.\n")
		}},
		{name: "35-an-empty-page", build: func(t *testing.T, root string) {
			writeGoldenFile(t, root, "wiki/memory/c-000043.md", "")
		}},
		{name: "36-files-beside-the-pages-that-are-not-pages", build: func(t *testing.T, root string) {
			writeGoldenFile(t, root, "wiki/memory/notes.txt", "not a page")
			writeGoldenFile(t, root, "wiki/memory/C-000044.MD", "not a page either")
			if err := os.Mkdir(filepath.Join(root, "wiki", "memory", "folder.md"), 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "37-a-page-named-only-md", build: func(t *testing.T, root string) {
			writeGoldenFile(t, root, "wiki/memory/.md", "---\naddress: c-000045\n---\n")
		}},
		{name: "38-a-pages-directory-that-is-a-link-to-a-directory", build: func(t *testing.T, root string) {
			if err := os.Rename(filepath.Join(root, "wiki", "memory"), filepath.Join(root, "notes-pages")); err != nil {
				t.Fatal(err)
			}
			symlinkOrSkip(t, "../notes-pages", filepath.Join(root, "wiki", "memory"))
		}},
		{name: "39-a-pages-directory-that-is-a-link-to-nothing", build: func(t *testing.T, root string) {
			if err := os.RemoveAll(filepath.Join(root, "wiki", "memory")); err != nil {
				t.Fatal(err)
			}
			symlinkOrSkip(t, "../notes/absent", filepath.Join(root, "wiki", "memory"))
		}},
		// 32, 34 and 35 (a link to another page, a page with no frontmatter, an empty page) are one golden, and
		// that is what they pin: the doctor takes a page's address from its name, and reads its content only
		// against the hashes the precedence sidecar records for it, so where the sidecar has no entry for the
		// page the content is not consulted and the three states cannot differ. These two give the page an entry
		// that records no usable revision and the hashes of the healthy page, which is what makes the content
		// matter: followed to the healthy page the link matches the entry, a page with no frontmatter does not.
		{name: "40-a-page-that-is-a-link-to-a-page-the-sidecar-entry-matches", build: func(t *testing.T, root string) {
			recordEntryMatchingTheHealthyPage(t, root)
			symlinkOrSkip(t, goldenAddress+".md", filepath.Join(root, "wiki", "memory", goldenOtherAddress+".md"))
		}},
		{name: "41-a-page-with-no-frontmatter-where-the-sidecar-entry-expects-the-healthy-page", build: func(t *testing.T, root string) {
			recordEntryMatchingTheHealthyPage(t, root)
			writeGoldenFile(t, root, "wiki/memory/"+goldenOtherAddress+".md", "Just text, no frontmatter.\n")
		}},
		// The catalog, wiki/index.md, the log, wiki/log.md, the sync state, .vault-meta/longterm-mem-sync-state.json
		// and the vault root, in each state a file or a directory can be found in. Recorded for the move of those
		// reads behind ports (Phase 9, L3 slice 4), from the program as it stood before it. A file that cannot be
		// read because of its mode is not a scenario: a test run as the superuser reads it, so its output would
		// depend on who ran the test; a directory, a link to nothing and a file under a file cannot be read by
		// anyone, and stand for it.
		{name: "42-an-index-that-is-a-directory", build: func(t *testing.T, root string) {
			replaceWithDirectory(t, root, "wiki/index.md")
		}},
		{name: "43-an-index-that-is-a-link-to-the-catalog", build: func(t *testing.T, root string) {
			replaceWithLinkToACopy(t, root, "wiki/index.md")
		}},
		{name: "44-an-index-that-is-a-link-to-nothing", build: func(t *testing.T, root string) {
			replaceWithLinkToNothing(t, root, "wiki/index.md")
		}},
		{name: "45-an-empty-index", build: func(t *testing.T, root string) {
			writeGoldenFile(t, root, "wiki/index.md", "")
		}},
		{name: "46-a-log-that-is-a-directory", build: func(t *testing.T, root string) {
			replaceWithDirectory(t, root, "wiki/log.md")
		}},
		{name: "47-a-log-that-is-a-link-to-a-log-that-names-the-page", build: func(t *testing.T, root string) {
			replaceWithLinkToACopy(t, root, "wiki/log.md")
		}},
		{name: "48-a-log-that-is-a-link-to-nothing", build: func(t *testing.T, root string) {
			replaceWithLinkToNothing(t, root, "wiki/log.md")
		}},
		{name: "49-a-wiki-directory-that-is-a-file", build: func(t *testing.T, root string) {
			if err := os.RemoveAll(filepath.Join(root, "wiki")); err != nil {
				t.Fatal(err)
			}
			writeGoldenFile(t, root, "wiki", "not a directory")
		}},
		{name: "50-a-sync-state-that-is-a-link-to-a-finished-sync", build: func(t *testing.T, root string) {
			testdata.WriteSyncState(t, root, "2026-08-31T12:00:00Z")
			replaceWithLinkToACopy(t, root, ".vault-meta/longterm-mem-sync-state.json")
		}},
		{name: "51-a-sync-state-that-is-a-link-to-nothing", build: func(t *testing.T, root string) {
			testdata.WriteSyncState(t, root, "2026-08-31T12:00:00Z")
			replaceWithLinkToNothing(t, root, ".vault-meta/longterm-mem-sync-state.json")
		}},
		{name: "52-an-empty-sync-state", build: func(t *testing.T, root string) {
			writeGoldenFile(t, root, ".vault-meta/longterm-mem-sync-state.json", "")
		}},
		{name: "53-a-sync-state-that-is-json-null", build: func(t *testing.T, root string) {
			writeGoldenFile(t, root, ".vault-meta/longterm-mem-sync-state.json", "null")
		}},
		{name: "54-a-sync-state-whose-completion-is-not-a-string", build: func(t *testing.T, root string) {
			writeGoldenFile(t, root, ".vault-meta/longterm-mem-sync-state.json", `{"last_sync_completed_at":5}`)
		}},
		{name: "55-a-vault-meta-directory-that-is-a-file", build: func(t *testing.T, root string) {
			writeGoldenFile(t, root, ".vault-meta", "not a directory")
		}},
		{name: "56-a-vault-root-that-is-a-link-to-the-vault", root: func(dir string) string {
			link := dir + "-link"
			if err := os.Symlink(dir, link); err != nil {
				panic(err)
			}
			return link
		}},
		{name: "57-a-vault-root-that-is-a-link-to-nothing", bare: true, root: func(dir string) string {
			link := filepath.Join(dir, "link")
			if err := os.Symlink(filepath.Join(dir, "absent"), link); err != nil {
				panic(err)
			}
			return link
		}},
	}
}

// replaceWithDirectory makes the vault-relative path rel a directory, where it was a file.
func replaceWithDirectory(t *testing.T, root, rel string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.Remove(full); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(full, 0o755); err != nil {
		t.Fatal(err)
	}
}

// replaceWithLinkToACopy moves the file at the vault-relative path rel to notes/ and leaves a link to it where
// it was, so that the file is the same and is reached through a link.
func replaceWithLinkToACopy(t *testing.T, root, rel string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	data, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	moved := "notes/" + filepath.Base(rel)
	writeGoldenFile(t, root, moved, string(data))
	if err := os.Remove(full); err != nil {
		t.Fatal(err)
	}
	up, err := filepath.Rel(filepath.Dir(full), filepath.Join(root, filepath.FromSlash(moved)))
	if err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, up, full)
}

// replaceWithLinkToNothing puts, at the vault-relative path rel, a link to a file that is not there.
func replaceWithLinkToNothing(t *testing.T, root, rel string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	symlinkOrSkip(t, filepath.Join(root, "notes", "absent"), full)
}

// recordEntryMatchingTheHealthyPage gives the page promoted under goldenOtherAddress a precedence entry that
// records no promoted revision and the hashes of the healthy page's frontmatter and body, which the fixture
// renders (the same page it writes under goldenAddress), so nothing here splits a file to find them.
func recordEntryMatchingTheHealthyPage(t *testing.T, root string) {
	t.Helper()
	healthy := testdata.RenderPage(t, goldenAddress, goldenTitle)
	testdata.WritePrecedenceEntry(t, root, promote.Page{Address: goldenOtherAddress, Frontmatter: healthy.Frontmatter, Body: healthy.Body})
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
				VaultRoot: vaultRoot,
				// The doctor's command wires the adapter with the prefix its output has always carried. The
				// literal is the golden files' own; cmd's TestCmdDoctor_ReportsAnUnparseableSidecarAsTheOpsGoldenDoes
				// runs the command's wiring over the same damaged vault and holds it to the text recorded here.
				Precedence:            vaultfs.New(vaultRoot, vaultfs.WithErrorPrefix("promote")),
				Pages:                 vaultfs.New(vaultRoot, vaultfs.WithErrorPrefix("promote")),
				AddressMap:            vaultfs.New(vaultRoot, vaultfs.WithErrorPrefix("promote")),
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
