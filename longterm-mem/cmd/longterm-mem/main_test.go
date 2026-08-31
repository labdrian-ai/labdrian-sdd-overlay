package main

import (
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"

	_ "modernc.org/sqlite"
)

// captureStdout redirects os.Stdout for the duration of fn and returns
// everything written to it -- cmd_status.go/cmd_doctor.go print their
// plain-text report to stdout, mirroring TestCmdSync_..."s own os.Stderr
// capture convention below.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stdout.txt")
	captured, err := os.Create(path)
	if err != nil {
		t.Fatalf("create stdout capture: %v", err)
	}
	real := os.Stdout
	os.Stdout = captured
	fn()
	os.Stdout = real
	if err := captured.Close(); err != nil {
		t.Fatalf("close stdout capture: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read stdout capture: %v", err)
	}
	return string(data)
}

// TestMain_BuildsIndependentModule asserts that longterm-mem/ compiles as an
// independent Go module — capable of declaring its own third-party
// dependencies — separate from engine/'s zero-dependency module (R-001).
// The build output goes to a scratch directory (-o) so this test never
// leaves a compiled binary inside the module's own source tree.
func TestMain_BuildsIndependentModule(t *testing.T) {
	moduleRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve module root: %v", err)
	}

	cmd := exec.Command("go", "build", "-o", t.TempDir()+string(filepath.Separator), "./...")
	cmd.Dir = moduleRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build ./... failed in %s: %v\n%s", moduleRoot, err, out)
	}
}

// TestRun_DispatchesSyncSubcommand proves "sync" is registered in run's
// switch (7.7), not falling through to the "unknown subcommand" default
// branch -- both paths can return exit 2, so a distinct exit code
// (vault_not_configured, 3, only reachable from inside cmdSync's own
// vaultreg.Resolve call) disambiguates dispatch from the fallback.
func TestRun_DispatchesSyncSubcommand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LONGTERM_MEM_VAULT", "")
	t.Setenv("LONGTERM_MEM_VAULTS_FILE", "")

	got := run([]string{"sync", "--project", "definitely-unconfigured-project"})
	if got != 3 {
		t.Fatalf("run([sync --project ...]) = %d, want 3 (vault_not_configured), proving sync dispatches into cmdSync rather than the unknown-subcommand fallback", got)
	}
}

// TestCmdSync_BothPassesRunDespiteAFailingObservation: promote.Sync steps
// over a broken observation rather than letting it wedge the run, but that
// hardening is worthless if the command treats Sync's error as fatal --
// one unpromotable observation would then stop promote.Propagate from ever
// running, so R-033's soft-delete and supersession statuses would never
// again land for that project, identically on every retry (review finding
// R4-sync-error-aborts-propagate). The command must run both passes,
// report what landed, and exit non-zero.
func TestCmdSync_BothPassesRunDespiteAFailingObservation(t *testing.T) {
	vaultRoot := t.TempDir()

	scriptsDir := filepath.Join(vaultRoot, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", scriptsDir, err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "allocate-address.sh"), []byte("#!/bin/sh\nprintf 'c-000900\\n'\n"), 0o755); err != nil {
		t.Fatalf("write allocate fixture: %v", err)
	}

	schema, err := os.ReadFile(filepath.Join("..", "..", "internal", "engram", "testdata", "schema.sql"))
	if err != nil {
		t.Fatalf("read engram schema fixture: %v", err)
	}
	dbPath := filepath.Join(t.TempDir(), "engram.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open fixture db: %v", err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	res, err := db.Exec(`INSERT INTO observations (session_id, sync_id, type, title, content, project, revision_count, pinned, created_at)
		 VALUES ('sess-1', 'sync-broken', 'decision', 'Broken', 'Body.', 'cmd-sync-project', 1, 0, '2026-08-01 00:00:00')`)
	if err != nil {
		t.Fatalf("insert observation: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("last insert id: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close fixture db: %v", err)
	}

	// The observation is already promoted to a page whose engram_revision
	// cannot be parsed, so deciding anything about it fails on every run.
	memoryDir := filepath.Join(vaultRoot, "wiki", "memory")
	if err := os.MkdirAll(memoryDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", memoryDir, err)
	}
	broken := "---\ntype: concept\ntitle: \"Broken\"\naddress: c-000900\nstatus: seed\nengram_id: " +
		strconv.FormatInt(id, 10) + "\nengram_revision: not-a-number\nproject: cmd-sync-project\n---\n\nBody.\n"
	if err := os.WriteFile(filepath.Join(memoryDir, "c-000900.md"), []byte(broken), 0o644); err != nil {
		t.Fatalf("write broken page: %v", err)
	}

	t.Setenv("HOME", t.TempDir())
	t.Setenv("LONGTERM_MEM_VAULT", vaultRoot)
	t.Setenv("LONGTERM_MEM_ENGRAM_DB", dbPath)

	stderrPath := filepath.Join(t.TempDir(), "stderr.txt")
	captured, err := os.Create(stderrPath)
	if err != nil {
		t.Fatalf("create stderr capture: %v", err)
	}
	realStderr := os.Stderr
	os.Stderr = captured
	exit := run([]string{"sync", "--project", "cmd-sync-project"})
	os.Stderr = realStderr
	if err := captured.Close(); err != nil {
		t.Fatalf("close stderr capture: %v", err)
	}

	if exit == 0 {
		t.Fatal("run([sync ...]) = 0, want a non-zero exit: the failing observation must be reported")
	}
	stderr, err := os.ReadFile(stderrPath)
	if err != nil {
		t.Fatalf("read stderr capture: %v", err)
	}
	// Propagate's own error can only appear if the command ran the second
	// pass -- which is exactly what treating Sync's error as fatal would
	// have prevented.
	if !strings.Contains(string(stderr), "propagate") {
		t.Fatalf("the second pass never ran: Sync's failure aborted the command instead of being reported alongside it; stderr:\n%s", stderr)
	}
}

// TestRun_DispatchesStatusSubcommand proves "status" is registered in
// run's switch (8a.3), matching TestRun_DispatchesSyncSubcommand's own
// dispatch-proof convention.
func TestRun_DispatchesStatusSubcommand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LONGTERM_MEM_VAULT", "")
	t.Setenv("LONGTERM_MEM_VAULTS_FILE", "")

	got := run([]string{"status", "--project", "definitely-unconfigured-project"})
	if got != 3 {
		t.Fatalf("run([status --project ...]) = %d, want 3 (vault_not_configured), proving status dispatches into cmdStatus rather than the unknown-subcommand fallback", got)
	}
}

// TestRun_DispatchesDoctorSubcommand proves "doctor" is registered in
// run's switch (8a.6), matching TestRun_DispatchesSyncSubcommand's own
// dispatch-proof convention.
func TestRun_DispatchesDoctorSubcommand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LONGTERM_MEM_VAULT", "")
	t.Setenv("LONGTERM_MEM_VAULTS_FILE", "")

	got := run([]string{"doctor", "--project", "definitely-unconfigured-project"})
	if got != 3 {
		t.Fatalf("run([doctor --project ...]) = %d, want 3 (vault_not_configured), proving doctor dispatches into cmdDoctor rather than the unknown-subcommand fallback", got)
	}
}

// TestCmdDoctor_ReportsEveryCheckDespiteOneFailing: the same lesson
// TestCmdSync_BothPassesRunDespiteAFailingObservation proves for sync
// (R4-sync-error-aborts-propagate), applied to doctor (task 8a description):
// a failing check must never stop the command from running and printing
// the remaining checks. The vault here has one promoted page correctly
// address-mapped but registered nowhere (wiki/index.md and wiki/log.md are
// both absent), guaranteeing wiki-registration-consistency FAILs; the test
// asserts all four check names still appear in the report and the command
// exits non-zero, proving the command ran ops.Doctor to completion instead
// of returning on the first FAIL.
func TestCmdDoctor_ReportsEveryCheckDespiteOneFailing(t *testing.T) {
	vaultRoot := t.TempDir()
	const address = "c-000777"

	obs := engram.Observation{ID: 1, Type: "decision", Title: "Unregistered Page", Content: "Body.", Project: "cmd-doctor-project", RevisionCount: 1}
	page, err := promote.EmitPage(obs, address, nil)
	if err != nil {
		t.Fatalf("EmitPage: %v", err)
	}
	full := filepath.Join(vaultRoot, page.Path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(page.Frontmatter+page.Body), 0o644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}

	rawDir := filepath.Join(vaultRoot, ".raw")
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", rawDir, err)
	}
	manifest := `{"address_map":{"` + page.Path + `":"` + address + `"}}`
	if err := os.WriteFile(filepath.Join(rawDir, ".manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	// wiki/index.md and wiki/log.md are intentionally never written: the
	// page above is not registered in either.

	t.Setenv("HOME", t.TempDir())
	t.Setenv("LONGTERM_MEM_VAULT", vaultRoot)

	var exit int
	stdout := captureStdout(t, func() {
		exit = run([]string{"doctor", "--project", "cmd-doctor-project"})
	})

	if exit == 0 {
		t.Fatal("run([doctor ...]) = 0, want non-zero: the unregistered page must fail wiki-registration-consistency")
	}
	for _, name := range []string{"vault-config-resolvable", "address-map-integrity", "wiki-registration-consistency", "runtime-prerequisites"} {
		if !strings.Contains(stdout, name) {
			t.Errorf("doctor output missing check %q; the command must report every check even though one failed:\n%s", name, stdout)
		}
	}
	if !strings.Contains(stdout, "FAIL") {
		t.Errorf("doctor output has no FAIL entry despite the unregistered page:\n%s", stdout)
	}
}

// TestCmdStatus_ReportsEveryFieldAndExitsZeroWhenUnhealthy: cmdStatus's
// whole reporting contract sat behind the vault-resolution failure that
// the only other status test triggers, so nothing ever executed the four
// output lines or the documented "an unhealthy field is still exit 0"
// behavior. Doctor got an end-to-end output test and status did not, so a
// regression that swapped the reachable/provisioned booleans, dropped the
// last-sync line, or turned an unhealthy field into a non-zero exit would
// have passed the suite unchanged (review finding
// R3-status-success-path-unproved).
//
// The vault here is deliberately unprovisioned and the project has never
// synced -- both unhealthy, both reported, still exit 0.
func TestCmdStatus_ReportsEveryFieldAndExitsZeroWhenUnhealthy(t *testing.T) {
	vaultRoot := t.TempDir()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("LONGTERM_MEM_VAULT", vaultRoot)
	t.Setenv("LONGTERM_MEM_ENGRAM_DB", filepath.Join(t.TempDir(), "absent-engram.db"))

	var exit int
	stdout := captureStdout(t, func() {
		exit = run([]string{"status", "--project", "cmd-status-project"})
	})

	if exit != 0 {
		t.Fatalf("run([status ...]) = %d, want 0: an unhealthy field is a reported state, not a command failure", exit)
	}
	if !strings.Contains(stdout, "cmd-status-project") {
		t.Errorf("status output does not name the project:\n%s", stdout)
	}
	for _, want := range []string{"engram: reachable=", "vault: provisioned=false", "last sync: never"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("status output missing %q; every field must be reported:\n%s", want, stdout)
		}
	}
}

// TestRun_DispatchesPromoteSubcommand proves "promote" is registered in
// run's switch (8b.6), matching TestRun_DispatchesSyncSubcommand's own
// dispatch-proof convention.
func TestRun_DispatchesPromoteSubcommand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LONGTERM_MEM_VAULT", "")
	t.Setenv("LONGTERM_MEM_VAULTS_FILE", "")

	got := run([]string{"promote", "--project", "definitely-unconfigured-project", "--id", "1"})
	if got != 3 {
		t.Fatalf("run([promote --project ... --id 1]) = %d, want 3 (vault_not_configured), proving promote dispatches into cmdPromote rather than the unknown-subcommand fallback", got)
	}
}

// promoteFixtureDB writes a temp Engram DB from the shared schema fixture
// with one observation row, returning its path and id -- the same
// building block TestCmdSync_BothPassesRunDespiteAFailingObservation
// establishes for this file.
func promoteFixtureDB(t *testing.T, title, project string) (dbPath string, id int64) {
	t.Helper()
	schema, err := os.ReadFile(filepath.Join("..", "..", "internal", "engram", "testdata", "schema.sql"))
	if err != nil {
		t.Fatalf("read engram schema fixture: %v", err)
	}
	dbPath = filepath.Join(t.TempDir(), "engram.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open fixture db: %v", err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	res, err := db.Exec(`INSERT INTO observations (session_id, type, title, content, project, revision_count, pinned, created_at)
		 VALUES ('sess-1', 'discovery', ?, 'Below-threshold body.', ?, 1, 0, '2026-08-01 00:00:00')`, title, project)
	if err != nil {
		t.Fatalf("insert observation: %v", err)
	}
	id, err = res.LastInsertId()
	if err != nil {
		t.Fatalf("last insert id: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close fixture db: %v", err)
	}
	return dbPath, id
}

// TestCmdPromote_PromotesObservationAndPrintsResult (8b.6 runtime-harness
// evidence, R-032): an observation that would never qualify automatically
// (type "discovery", revision 1, unpinned -- see TestEligible) is promoted
// end to end through the built command, proving cmd_promote.go's own
// wiring: the page lands on disk, is registered in the vault's catalog and
// log (R-029, reused by the explicit path per design.md), and the command
// prints the address and outcome.
func TestCmdPromote_PromotesObservationAndPrintsResult(t *testing.T) {
	vaultRoot := t.TempDir()
	scriptsDir := filepath.Join(vaultRoot, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", scriptsDir, err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "allocate-address.sh"), []byte("#!/bin/sh\nprintf 'c-000901\\n'\n"), 0o755); err != nil {
		t.Fatalf("write allocate fixture: %v", err)
	}

	dbPath, id := promoteFixtureDB(t, "Explicitly Promoted", "cmd-promote-project")

	t.Setenv("HOME", t.TempDir())
	t.Setenv("LONGTERM_MEM_VAULT", vaultRoot)
	t.Setenv("LONGTERM_MEM_ENGRAM_DB", dbPath)

	var exit int
	stdout := captureStdout(t, func() {
		exit = run([]string{"promote", "--project", "cmd-promote-project", "--id", strconv.FormatInt(id, 10)})
	})
	if exit != 0 {
		t.Fatalf("run([promote ...]) = %d, want 0; stdout:\n%s", exit, stdout)
	}
	if !strings.Contains(stdout, "c-000901") || !strings.Contains(stdout, "created") {
		t.Fatalf("promote output = %q, want it to name the new address and the created action", stdout)
	}

	page := filepath.Join(vaultRoot, "wiki", "memory", "c-000901.md")
	if _, err := os.Stat(page); err != nil {
		t.Fatalf("promoted page %s does not exist: %v", page, err)
	}
	indexData, err := os.ReadFile(filepath.Join(vaultRoot, "wiki", "index.md"))
	if err != nil {
		t.Fatalf("read wiki/index.md: %v", err)
	}
	if !strings.Contains(string(indexData), "c-000901") {
		t.Fatalf("wiki/index.md does not register the promoted page; got:\n%s", indexData)
	}
}

// TestCmdPromote_InvalidIdExits7 (8b.6, R-032's "Invalid observation id is
// rejected" scenario): an id with no matching row must exit 7
// (not_found), never a silent success, and must never write a page.
func TestCmdPromote_InvalidIdExits7(t *testing.T) {
	vaultRoot := t.TempDir()
	dbPath, _ := promoteFixtureDB(t, "Unrelated", "cmd-promote-project")

	t.Setenv("HOME", t.TempDir())
	t.Setenv("LONGTERM_MEM_VAULT", vaultRoot)
	t.Setenv("LONGTERM_MEM_ENGRAM_DB", dbPath)

	stderrPath := filepath.Join(t.TempDir(), "stderr.txt")
	captured, err := os.Create(stderrPath)
	if err != nil {
		t.Fatalf("create stderr capture: %v", err)
	}
	realStderr := os.Stderr
	os.Stderr = captured
	exit := run([]string{"promote", "--project", "cmd-promote-project", "--id", "999999"})
	os.Stderr = realStderr
	if err := captured.Close(); err != nil {
		t.Fatalf("close stderr capture: %v", err)
	}

	if exit != 7 {
		t.Fatalf("run([promote --id 999999]) = %d, want 7 (not_found)", exit)
	}
	stderr, err := os.ReadFile(stderrPath)
	if err != nil {
		t.Fatalf("read stderr capture: %v", err)
	}
	if !strings.Contains(string(stderr), "not found") {
		t.Fatalf("stderr = %q, want it to name the observation as not found", stderr)
	}
	if _, statErr := os.Stat(filepath.Join(vaultRoot, "wiki", "index.md")); !os.IsNotExist(statErr) {
		t.Fatalf("wiki/index.md exists after a rejected promote call (stat err = %v)", statErr)
	}
}
