package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// runQuietly runs one subcommand with both standard streams redirected to
// a scratch file and returns its exit code. Every test below drives a
// command that is meant to fail, and this file cares only about the code
// each one answers with -- their diagnostics would otherwise scroll
// through the test output. The stderr redirect is inline rather than a
// shared helper because the exit code, not the message, is what is under
// test here.
func runQuietly(t *testing.T, args []string) int {
	t.Helper()
	sink, err := os.Create(filepath.Join(t.TempDir(), "stderr.txt"))
	if err != nil {
		t.Fatalf("create stderr sink: %v", err)
	}
	defer sink.Close()

	var exit int
	captureStdout(t, func() {
		real := os.Stderr
		os.Stderr = sink
		defer func() { os.Stderr = real }()
		exit = run(args)
	})
	return exit
}

// vaultResolvingSubcommands is every subcommand that resolves a project's
// vault through vaultreg.Resolve before doing any work. All six share one
// failure-mapping contract, so they are asserted as one table rather than
// six near-identical tests.
func vaultResolvingSubcommands(project string) []struct {
	name string
	args []string
} {
	return []struct {
		name string
		args []string
	}{
		{"query", []string{"query", "--project", project, "anything"}},
		{"index", []string{"index", "--project", project}},
		{"sync", []string{"sync", "--project", project}},
		{"status", []string{"status", "--project", project}},
		{"doctor", []string{"doctor", "--project", project}},
		{"promote", []string{"promote", "--source", "memorycard", "--project", project, "--id", "1"}},
	}
}

// TestVaultResolution_UnreadableRegistryExitsInternalNotVaultNotConfigured:
// exit 3 is named vault_not_configured, and an operator who sees it looks
// for a missing registry row. A registry file that cannot be parsed is a
// different failure entirely -- the row may well be there -- so reporting
// it as 3 sends them to add an entry that already exists while the real
// cause (a corrupt ~/.labdrian-overlay/vaults.json) stays invisible. Only
// vaultreg.ErrVaultNotConfigured may claim exit 3; every other resolution
// failure is internal (exit 1).
func TestVaultResolution_UnreadableRegistryExitsInternalNotVaultNotConfigured(t *testing.T) {
	for _, tc := range vaultResolvingSubcommands("corrupt-registry-project") {
		t.Run(tc.name, func(t *testing.T) {
			registry := filepath.Join(t.TempDir(), "vaults.json")
			if err := os.WriteFile(registry, []byte("{ this is not valid json"), 0o600); err != nil {
				t.Fatalf("write corrupt registry: %v", err)
			}
			t.Setenv("HOME", t.TempDir())
			t.Setenv("LONGTERM_MEM_VAULT", "")
			t.Setenv("LONGTERM_MEM_VAULTS_FILE", registry)

			if exit := runQuietly(t, tc.args); exit != 1 {
				t.Fatalf("run(%v) = %d, want 1 (internal): an unparseable vault registry is not a missing registry row, and must not wear exit 3's vault_not_configured meaning", tc.args, exit)
			}
		})
	}
}

// TestVaultResolution_UnconfiguredProjectExitsVaultNotConfigured is the
// other half of the test above: narrowing exit 3 must not erase it. A
// project with no registry row is exactly what exit 3 is for (R-023).
func TestVaultResolution_UnconfiguredProjectExitsVaultNotConfigured(t *testing.T) {
	for _, tc := range vaultResolvingSubcommands("definitely-unconfigured-project") {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("LONGTERM_MEM_VAULT", "")
			t.Setenv("LONGTERM_MEM_VAULTS_FILE", "")

			if exit := runQuietly(t, tc.args); exit != 3 {
				t.Fatalf("run(%v) = %d, want 3 (vault_not_configured): a project with no registry row is the one failure exit 3 names", tc.args, exit)
			}
		})
	}
}

// unreadableEngramDB writes a syntactically valid SQLite file that has
// none of Engram's tables. engram.Open pings it successfully, so the
// command gets past its exit-4 open guard, and every later read fails --
// the shape of an Engram database that has become unusable underneath a
// running command.
func unreadableEngramDB(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "engram.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open fixture db: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE placeholder (id INTEGER)`); err != nil {
		t.Fatalf("materialize fixture db: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close fixture db: %v", err)
	}
	return dbPath
}

// TestCmdQuery_EngramReadFailureExitsEngramUnavailable pins exit 4 for the
// query surface. Before this test nothing in the CLI suite asserted exit 4
// at all, so any regression that renamed engram_unavailable into some
// other non-zero code passed unnoticed.
func TestCmdQuery_EngramReadFailureExitsEngramUnavailable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LONGTERM_MEM_VAULT", t.TempDir())
	t.Setenv("LONGTERM_MEM_ENGRAM_DB", unreadableEngramDB(t))

	if exit := runQuietly(t, []string{"query", "--project", "engram-broken-project", "anything"}); exit != 4 {
		t.Fatalf("run([query ...]) = %d, want 4 (engram_unavailable)", exit)
	}
}

// TestCmdIndex_VaultRebuildFailureExitsVaultSubprocessFailed pins exit 5
// for the index surface: the vault here has no bin/setup-retrieve.sh, so
// vault.Rebuild's provision step cannot run. Before this test nothing in
// the CLI suite asserted exit 5 either.
func TestCmdIndex_VaultRebuildFailureExitsVaultSubprocessFailed(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LONGTERM_MEM_VAULT", t.TempDir())

	if exit := runQuietly(t, []string{"index", "--project", "unprovisionable-project"}); exit != 5 {
		t.Fatalf("run([index ...]) = %d, want 5 (vault_subprocess_failed)", exit)
	}
}

// TestCmdSync_EngramListingFailureExitsEngramUnavailable: sync used to
// answer 5 (vault_subprocess_failed) for every failure it could not
// complete, including one where no vault subprocess ever ran. An Engram
// database the command cannot list is engram_unavailable (exit 4), and an
// operator reading 5 would go looking at the vault's Python tooling for a
// fault that is not there.
func TestCmdSync_EngramListingFailureExitsEngramUnavailable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LONGTERM_MEM_VAULT", t.TempDir())
	t.Setenv("LONGTERM_MEM_ENGRAM_DB", unreadableEngramDB(t))

	if exit := runQuietly(t, []string{"sync", "--project", "engram-broken-project"}); exit != 4 {
		t.Fatalf("run([sync ...]) = %d, want 4 (engram_unavailable): the listing failed before any vault subprocess ran, so this is not a vault_subprocess_failed", exit)
	}
}
