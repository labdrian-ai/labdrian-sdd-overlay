package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/embed"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/query"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vault"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vecindex"
)

// The golden files under testdata/vault-registry-golden pin what each command does with the vault registry
// (~/.labdrian-overlay/vaults.json) and what it prints and exits with while it does: the registry's state after every
// command (absent, or its mode, the mode of its directory and its bytes), beside the exit code, stdout and stderr.
// They were recorded from the program before Phase 9, L9 made the resolution of a vault pure, and the change that
// followed (the owner's decision of 2026-10-14, "Solo los que escriben": only a command that writes seeds the
// registry) moved one thing in them, the registry line of a command that only reads (it says "absent" where it
// said the registry was made), and not one byte of what a command prints or exits with.
//
// The world is the promote goldens' (an Engram database, a vault with its stub scripts), with the vault where the
// registry's seeded row for the default project puts it, ~/labdrian-brain, and the registry file left to the
// program: no LONGTERM_MEM_VAULT, no LONGTERM_MEM_VAULTS_FILE, a HOME of the test's own.
//
// To rewrite them after a change that is MEANT to alter the output:
//
//	go test ./cmd/longterm-mem -run TestVaultRegistryGolden -update-promote-golden
//
// and read the diff before committing it.

// registryDefaultProject is the project the seeded row of the registry belongs to.
const registryDefaultProject = "labdrian-sdd-overlay"

// registryScenario is one command, or a few in a row, run in a world with the registry in the state it names.
type registryScenario struct {
	name string
	// registry is the content of the registry file the world starts with; nil leaves it absent.
	registry *string
	// vaultEnv is the value of LONGTERM_MEM_VAULT; "{vault}" stands for the world's vault.
	vaultEnv string
	commands [][]string
}

func registryContent(content string) *string { return &content }

func registryScenarios() []registryScenario {
	project := func(name string) []string { return []string{"--project", name} }
	with := func(name string, args ...string) []string {
		return append(append([]string{args[0]}, project(name)...), args[1:]...)
	}
	const unconfigured = "no-such-project"
	const vaultFlag = "{vault}"

	return []registryScenario{
		// The default project, the registry absent: what resolving the vault leaves behind, command by command.
		{name: "01-status", commands: [][]string{with(registryDefaultProject, "status")}},
		{name: "02-status-json", commands: [][]string{with(registryDefaultProject, "status", "--json")}},
		{name: "03-doctor", commands: [][]string{with(registryDefaultProject, "doctor")}},
		{name: "04-query", commands: [][]string{with(registryDefaultProject, "query", "pinned")}},
		{name: "05-sync-dry-run", commands: [][]string{with(registryDefaultProject, "sync", "--dry-run")}},
		{name: "06-sync", commands: [][]string{with(registryDefaultProject, "sync")}},
		{name: "07-promote", commands: [][]string{{"promote", "--project", registryDefaultProject, "--id", "1"}}},
		{name: "08-promote-reconcile", commands: [][]string{{"promote", "reconcile", "--project", registryDefaultProject, "c-000901"}}},
		{name: "09-index", commands: [][]string{with(registryDefaultProject, "index")}},

		// A project the registry has no row for: exit 3 for every command that resolves a vault, and the registry
		// it leaves.
		{name: "10-status-of-an-unconfigured-project", commands: [][]string{with(unconfigured, "status")}},
		{name: "11-doctor-of-an-unconfigured-project", commands: [][]string{with(unconfigured, "doctor")}},
		{name: "12-query-of-an-unconfigured-project", commands: [][]string{with(unconfigured, "query", "pinned")}},
		{name: "13-sync-dry-run-of-an-unconfigured-project", commands: [][]string{with(unconfigured, "sync", "--dry-run")}},
		{name: "14-sync-of-an-unconfigured-project", commands: [][]string{with(unconfigured, "sync")}},
		{name: "15-promote-of-an-unconfigured-project", commands: [][]string{{"promote", "--project", unconfigured, "--id", "1"}}},
		{name: "16-index-of-an-unconfigured-project", commands: [][]string{with(unconfigured, "index")}},

		// The vault is given, by flag or by environment: the registry is not consulted, and not created.
		{name: "17-status-with-the-vault-flag", commands: [][]string{with(registryDefaultProject, "status", "--vault", vaultFlag)}},
		{name: "18-sync-with-the-vault-flag", commands: [][]string{with(registryDefaultProject, "sync", "--vault", vaultFlag)}},
		{name: "19-doctor-with-the-vault-in-the-environment", vaultEnv: vaultFlag, commands: [][]string{with(unconfigured, "doctor")}},

		// A registry that is there is never touched, whatever it says and whichever command reads it.
		{name: "20-status-with-the-default-row-deleted", registry: registryContent(`{"schema": 1, "vaults": {}}` + "\n"), commands: [][]string{with(registryDefaultProject, "status")}},
		{name: "21-sync-with-the-default-row-deleted", registry: registryContent(`{"schema": 1, "vaults": {}}` + "\n"), commands: [][]string{with(registryDefaultProject, "sync")}},
		{name: "22-status-with-a-registry-that-is-not-json", registry: registryContent("{ not json"), commands: [][]string{with(registryDefaultProject, "status")}},
		{name: "23-sync-with-a-registry-that-is-not-json", registry: registryContent("{ not json"), commands: [][]string{with(registryDefaultProject, "sync")}},
		{name: "24-status-with-the-default-row-pointing-elsewhere", registry: registryContent(`{"schema": 1, "vaults": {"labdrian-sdd-overlay": {"path": "{vault}"}}}` + "\n"), commands: [][]string{with(registryDefaultProject, "status")}},

		// A read and then a write, from a registry nobody has made: the order is the whole point.
		{name: "25-status-then-sync", commands: [][]string{with(registryDefaultProject, "status"), with(registryDefaultProject, "sync")}},
		{name: "26-sync-then-status", commands: [][]string{with(registryDefaultProject, "sync"), with(registryDefaultProject, "status")}},
	}
}

func TestVaultRegistryGolden(t *testing.T) {
	if !vault.PrerequisitePresent("python3") {
		t.Skip("python3 is not on PATH: the vault's index rebuild runs its scripts under it")
	}
	goldenDir, err := filepath.Abs(filepath.Join("testdata", "vault-registry-golden"))
	if err != nil {
		t.Fatalf("resolve the golden directory: %v", err)
	}
	for _, scenario := range registryScenarios() {
		t.Run(scenario.name, func(t *testing.T) {
			fixedUmask(t)
			w := newRegistryWorld(t)
			t.Setenv("HOME", w.home)
			t.Setenv("LONGTERM_MEM_ENGRAM_DB", w.dbPath)
			t.Setenv("LONGTERM_MEM_VAULT", strings.ReplaceAll(scenario.vaultEnv, "{vault}", w.vaultRoot))
			t.Setenv("LONGTERM_MEM_VAULTS_FILE", "")
			t.Chdir(t.TempDir())

			if scenario.registry != nil {
				w.writeRegistry(t, strings.ReplaceAll(*scenario.registry, "{vault}", w.vaultRoot))
			}

			var out strings.Builder
			for _, args := range scenario.commands {
				for i := range args {
					args[i] = strings.ReplaceAll(args[i], "{vault}", w.vaultRoot)
				}
				code, stdout, stderr := runPromoteGolden(t, args)
				fmt.Fprintf(&out, "args: %s\nexit: %d\n--- stdout ---\n%s--- stderr ---\n%s--- registry ---\n%s",
					strings.Join(args, " "), code, stdout, stderr, w.renderRegistry(t))
			}
			checkPromoteGolden(t, goldenDir, scenario.name, w.normalise(out.String()))
		})
	}
}

// The tools of the MCP session resolve the vault of the project of each call, as the commands do, and leave the
// registry as they do: each is run here over the world, with the registry absent, and the registry it leaves is
// read. The query tool only reads and leaves the machine as it found it; the promote tool writes a page, and seeds
// the registry (the owner's decision of 2026-10-14: only a command that writes does).
func TestVaultRegistry_WhatTheMCPToolsLeave(t *testing.T) {
	if !vault.PrerequisitePresent("python3") {
		t.Skip("python3 is not on PATH: the vault's index rebuild runs its scripts under it")
	}
	for name, tc := range map[string]struct {
		call func(t *testing.T, c commands, store *engram.Store)
		// seeded is whether the registry exists once the tool has answered.
		seeded bool
	}{
		"the query tool": {seeded: false, call: func(t *testing.T, c commands, store *engram.Store) {
			embedClient, err := testEmbedClient(embed.Config{Model: vecindex.DefaultModel})
			if err != nil {
				t.Fatalf("the embedding client: %v", err)
			}
			if _, err := c.mcpQuery(store, embedClient, vecindex.NewLoadCache())(context.Background(), query.Request{Project: registryDefaultProject, Query: "pinned", Top: 5}); err != nil {
				t.Fatalf("the query tool: %v", err)
			}
		}},
		"the promote tool": {seeded: true, call: func(t *testing.T, c commands, store *engram.Store) {
			if _, err := c.mcpPromote(store)(context.Background(), registryDefaultProject, 1); err != nil {
				t.Fatalf("the promote tool: %v", err)
			}
		}},
	} {
		t.Run(name, func(t *testing.T) {
			fixedUmask(t)
			w := newRegistryWorld(t)
			t.Setenv("HOME", w.home)
			t.Setenv("LONGTERM_MEM_VAULT", "")
			t.Setenv("LONGTERM_MEM_VAULTS_FILE", "")
			t.Chdir(t.TempDir())
			store, err := engram.Open(w.dbPath)
			if err != nil {
				t.Fatalf("open the Engram database: %v", err)
			}
			defer store.Close()

			if got := w.renderRegistry(t); got != "absent\n" {
				t.Fatalf("the world starts with a registry:\n%s", got)
			}
			tc.call(t, commands{newEmbedClient: testEmbedClient, clock: promoteGoldenClock}, store)

			got := w.renderRegistry(t)
			want := "absent"
			if tc.seeded {
				want = "seeded"
			}
			if exists := got != "absent\n"; exists != tc.seeded {
				t.Errorf("registry after the tool = %q, want it %s", got, want)
			}
			if tc.seeded && !strings.Contains(got, `"labdrian-sdd-overlay"`) {
				t.Errorf("registry after the tool = %q, want the row of the default project", got)
			}
		})
	}
}

// registryWorld is a promote golden world whose observations belong to the default project and whose vault is
// where the seeded row of the registry puts the default project's: ~/labdrian-brain.
type registryWorld struct {
	*promoteGoldenWorld
}

func newRegistryWorld(t *testing.T) *registryWorld {
	t.Helper()
	w := &registryWorld{newPromoteGoldenWorld(t)}
	w.execSQL(t, `UPDATE observations SET project = ?`, registryDefaultProject)
	symlinkOrSkip(t, w.vaultRoot, filepath.Join(w.home, "labdrian-brain"))
	return w
}

// registryPath is where the program keeps the registry when nothing says otherwise.
func (w *registryWorld) registryPath() string {
	return filepath.Join(w.home, ".labdrian-overlay", "vaults.json")
}

func (w *registryWorld) writeRegistry(t *testing.T, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(w.registryPath()), 0o755); err != nil {
		t.Fatalf("create the registry's directory: %v", err)
	}
	if err := os.WriteFile(w.registryPath(), []byte(content), 0o644); err != nil {
		t.Fatalf("write the registry: %v", err)
	}
}

// renderRegistry says what the world's registry is now: absent, or its mode, the mode of its directory and its
// bytes.
func (w *registryWorld) renderRegistry(t *testing.T) string {
	t.Helper()
	info, err := os.Stat(w.registryPath())
	if os.IsNotExist(err) {
		return "absent\n"
	}
	if err != nil {
		t.Fatalf("stat the registry: %v", err)
	}
	dir, err := os.Stat(filepath.Dir(w.registryPath()))
	if err != nil {
		t.Fatalf("stat the registry's directory: %v", err)
	}
	data, err := os.ReadFile(w.registryPath())
	if err != nil {
		t.Fatalf("read the registry: %v", err)
	}
	return fmt.Sprintf("file %v, directory %v\n%s", info.Mode().Perm(), dir.Mode().Perm(), data)
}
