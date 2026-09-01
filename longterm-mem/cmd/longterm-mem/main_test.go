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

// TestRegisterExpandTarget (11b.7, R-016/R-017) pins --target's accepted
// domain and, in the "all" row, a scope boundary rather than a behaviour:
// codex is an accepted single target but is deliberately NOT part of
// "all", because its writer does not exist until 12a.6. Adding it to the
// expansion early would make `register --target all` start failing on a
// runtime nobody asked for, so the boundary is asserted rather than left
// to a reader's memory.
func TestRegisterExpandTarget(t *testing.T) {
	for _, tc := range []struct {
		target  string
		want    []string
		wantErr bool
	}{
		{target: "claude", want: []string{"claude"}},
		{target: "opencode", want: []string{"opencode"}},
		{target: "codex", want: []string{"codex"}},
		{target: "all", want: []string{"claude", "opencode"}},
		{target: "", wantErr: true},
		{target: "bogus", wantErr: true},
		{target: "Claude", wantErr: true},
	} {
		got, err := registerExpandTarget(tc.target)
		if tc.wantErr {
			if err == nil {
				t.Errorf("registerExpandTarget(%q) = %v, nil; want an error", tc.target, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("registerExpandTarget(%q) returned error: %v", tc.target, err)
			continue
		}
		if strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("registerExpandTarget(%q) = %v, want %v", tc.target, got, tc.want)
		}
	}
}

// TestDefaultRegisterConfigRoot (11b.7) pins where each runtime's config
// is looked for when --config-root is not given. The two rows that matter
// most are the ones an end-to-end test cannot see: a relative
// $XDG_CONFIG_HOME or $CODEX_HOME is ignored rather than joined against
// the current directory, because a config root that depends on where the
// command was invoked from would write into whatever directory the user
// happened to be standing in.
func TestDefaultRegisterConfigRoot(t *testing.T) {
	home := t.TempDir()
	xdg := t.TempDir()
	codexHome := t.TempDir()

	for _, tc := range []struct {
		name      string
		target    string
		xdgConfig string
		codexHome string
		want      string
	}{
		{name: "claude is $HOME itself", target: "claude", want: home},
		{name: "opencode honours an absolute XDG_CONFIG_HOME", target: "opencode", xdgConfig: xdg, want: filepath.Join(xdg, "opencode")},
		{name: "opencode ignores a relative XDG_CONFIG_HOME", target: "opencode", xdgConfig: "relative/config", want: filepath.Join(home, ".config", "opencode")},
		{name: "opencode falls back to ~/.config", target: "opencode", want: filepath.Join(home, ".config", "opencode")},
		{name: "codex honours an absolute CODEX_HOME", target: "codex", codexHome: codexHome, want: codexHome},
		{name: "codex ignores a relative CODEX_HOME", target: "codex", codexHome: "relative/codex", want: filepath.Join(home, ".codex")},
		{name: "codex falls back to ~/.codex", target: "codex", want: filepath.Join(home, ".codex")},
		{name: "an unknown target resolves to nothing", target: "bogus", want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", tc.xdgConfig)
			t.Setenv("CODEX_HOME", tc.codexHome)
			if got := defaultRegisterConfigRoot(tc.target); got != tc.want {
				t.Errorf("defaultRegisterConfigRoot(%q) = %q, want %q", tc.target, got, tc.want)
			}
		})
	}
}

// TestDefaultRegisterPathsAreEmptyWhenUnresolvable (11b.7) pins the
// fail-closed half of the contract cmd_register.go depends on: when the
// home directory cannot be resolved these return the empty string, which
// the command refuses on. The alternative -- returning a relative path --
// would write install-state.json into the process working directory and
// install a binary path that starts nothing, both reported as success.
func TestDefaultRegisterPathsAreEmptyWhenUnresolvable(t *testing.T) {
	t.Setenv("HOME", "")
	if got := defaultRegisterStateDir(); got != "" {
		t.Errorf("defaultRegisterStateDir() with no resolvable home = %q, want \"\"", got)
	}
	if got := defaultRegisterBinaryPath(); got != "" {
		t.Errorf("defaultRegisterBinaryPath() with no resolvable home = %q, want \"\"", got)
	}
	if got := defaultRegisterConfigRoot("claude"); got != "" {
		t.Errorf("defaultRegisterConfigRoot(\"claude\") with no resolvable home = %q, want \"\"", got)
	}
}

// TestRun_DispatchesRegisterSubcommand proves "register" is registered in
// run's switch, not falling through to the unknown-subcommand default (2):
// pointing HOME at a fresh temp dir with no ~/.claude.json yet makes
// cmdRegister's own RegisterClaude call fail (nothing to splice into), a
// distinct exit code (1) only reachable from inside cmdRegister, never from
// the top-level fallback.
func TestRun_DispatchesRegisterSubcommand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got := run([]string{"register", "--target", "claude", "--state-dir", t.TempDir(), "--binary", "/bin/longterm-mem"})
	if got != 1 {
		t.Fatalf("run([register --target claude ...]) = %d, want 1, proving register dispatches into cmdRegister rather than the unknown-subcommand fallback", got)
	}
}

// TestCmdRegister_UnknownTargetExitsTwo (usage error, R-016/R-017): an
// unrecognized --target value is rejected before any file I/O, exit 2.
func TestCmdRegister_UnknownTargetExitsTwo(t *testing.T) {
	got := run([]string{"register", "--target", "bogus"})
	if got != 2 {
		t.Fatalf("run([register --target bogus]) = %d, want 2", got)
	}
}

// TestCmdRegister_InstallsSuccessfully (11b.7, R-016): a real end-to-end
// install through the built command -- config-root/state-dir both point at
// fresh temp dirs, an existing ~/.claude.json fixture already declares an
// empty mcpServers object (the same "every real config already declares
// its container object" assumption jsonsplice.go documents) -- succeeds
// with exit 0 and leaves the expected entry spliced in.
func TestCmdRegister_InstallsSuccessfully(t *testing.T) {
	configRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(configRoot, ".claude.json"), []byte(`{"mcpServers":{}}`), 0o600); err != nil {
		t.Fatalf("seed .claude.json: %v", err)
	}
	stateDir := t.TempDir()

	var stdout string
	var exit int
	stdout = captureStdout(t, func() {
		exit = run([]string{"register", "--target", "claude", "--config-root", configRoot, "--state-dir", stateDir, "--binary", "/opt/bin/longterm-mem"})
	})
	if exit != 0 {
		t.Fatalf("run([register --target claude ...]) = %d, want 0; stdout:\n%s", exit, stdout)
	}
	if !strings.Contains(stdout, "claude: ok") {
		t.Fatalf("register stdout = %q, want it to report claude: ok", stdout)
	}

	got, err := os.ReadFile(filepath.Join(configRoot, ".claude.json"))
	if err != nil {
		t.Fatalf("read result config: %v", err)
	}
	if !strings.Contains(string(got), `"longterm-mem": {"type":"stdio","command":"/opt/bin/longterm-mem","args":["mcp"]}`) {
		t.Fatalf("result config does not contain the expected entry:\n%s", got)
	}
}

// TestCmdRegister_ConflictExitsSix (11b.7, R-016 "Untagged same-named entry
// is refused, not overwritten"): install-state has no record for claude,
// but ~/.claude.json already has an untagged longterm-mem entry -- register
// must refuse (exit 6) and leave the config file byte-identical.
func TestCmdRegister_ConflictExitsSix(t *testing.T) {
	configRoot := t.TempDir()
	original := []byte(`{"mcpServers":{"longterm-mem":{"type":"stdio","command":"/someone/elses/binary"}}}`)
	configPath := filepath.Join(configRoot, ".claude.json")
	if err := os.WriteFile(configPath, original, 0o600); err != nil {
		t.Fatalf("seed .claude.json: %v", err)
	}
	stateDir := t.TempDir()

	stderrPath := filepath.Join(t.TempDir(), "stderr.txt")
	captured, err := os.Create(stderrPath)
	if err != nil {
		t.Fatalf("create stderr capture: %v", err)
	}
	realStderr := os.Stderr
	os.Stderr = captured
	exit := run([]string{"register", "--target", "claude", "--config-root", configRoot, "--state-dir", stateDir, "--binary", "/opt/bin/longterm-mem"})
	os.Stderr = realStderr
	if err := captured.Close(); err != nil {
		t.Fatalf("close stderr capture: %v", err)
	}

	if exit != 6 {
		t.Fatalf("run([register --target claude ...]) with an untagged conflict = %d, want 6", exit)
	}
	stderr, err := os.ReadFile(stderrPath)
	if err != nil {
		t.Fatalf("read stderr capture: %v", err)
	}
	if !strings.Contains(string(stderr), "claude") {
		t.Fatalf("stderr = %q, want it to name the conflicting target", stderr)
	}

	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read result config: %v", err)
	}
	if string(got) != string(original) {
		t.Fatalf("config file was modified despite the refusal:\nbefore = %s\nafter = %s", original, got)
	}
}

// TestCmdRegister_AllExpandsToClaudeAndOpencode (11b.7): --target all
// registers every currently-wired runtime, each resolving its own default
// config root from HOME/XDG_CONFIG_HOME rather than a single shared
// --config-root (which --target all refuses to accept, per the usage
// guard above).
func TestCmdRegister_AllExpandsToClaudeAndOpencode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	xdgConfig := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgConfig)

	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{"mcpServers":{}}`), 0o600); err != nil {
		t.Fatalf("seed .claude.json: %v", err)
	}
	opencodeDir := filepath.Join(xdgConfig, "opencode")
	if err := os.MkdirAll(opencodeDir, 0o755); err != nil {
		t.Fatalf("mkdir opencode config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(opencodeDir, "opencode.json"), []byte(`{"mcp":{}}`), 0o600); err != nil {
		t.Fatalf("seed opencode.json: %v", err)
	}

	exit := run([]string{"register", "--target", "all", "--state-dir", t.TempDir(), "--binary", "/opt/bin/longterm-mem"})
	if exit != 0 {
		t.Fatalf("run([register --target all]) = %d, want 0", exit)
	}

	claudeGot, err := os.ReadFile(filepath.Join(home, ".claude.json"))
	if err != nil {
		t.Fatalf("read claude config: %v", err)
	}
	if !strings.Contains(string(claudeGot), `"longterm-mem":`) {
		t.Fatalf("claude config missing longterm-mem entry:\n%s", claudeGot)
	}
	opencodeGot, err := os.ReadFile(filepath.Join(opencodeDir, "opencode.json"))
	if err != nil {
		t.Fatalf("read opencode config: %v", err)
	}
	if !strings.Contains(string(opencodeGot), `"longterm-mem":`) {
		t.Fatalf("opencode config missing longterm-mem entry:\n%s", opencodeGot)
	}
}

// TestCmdRegister_UnresolvableBinaryPathIsRefused (11b.7, R-016/R-017):
// when --binary is omitted and the default path cannot be resolved
// (HOME unset), register must refuse rather than write an entry whose
// command is the empty string. XDG_CONFIG_HOME still resolves opencode's
// config root, so the config root guard does not cover this case: without
// its own check, the writer would happily install a structurally valid
// entry that can never start a server, and register would report success.
func TestCmdRegister_UnresolvableBinaryPathIsRefused(t *testing.T) {
	t.Setenv("HOME", "")
	xdgConfig := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgConfig)

	opencodeDir := filepath.Join(xdgConfig, "opencode")
	if err := os.MkdirAll(opencodeDir, 0o755); err != nil {
		t.Fatalf("mkdir opencode config dir: %v", err)
	}
	configPath := filepath.Join(opencodeDir, "opencode.json")
	original := []byte(`{"mcp":{}}`)
	if err := os.WriteFile(configPath, original, 0o600); err != nil {
		t.Fatalf("seed opencode.json: %v", err)
	}

	exit := run([]string{"register", "--target", "opencode", "--state-dir", t.TempDir()})
	if exit == 0 {
		t.Fatalf("run([register --target opencode]) with an unresolvable binary path = 0, want non-zero")
	}

	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read opencode config: %v", err)
	}
	if string(got) != string(original) {
		t.Fatalf("config was modified despite an unresolvable binary path:\nbefore = %s\nafter = %s", original, got)
	}
}

// TestCmdRegister_UnresolvableStateDirIsRefused (11b.7, R-017): the same
// refusal in the other direction. --binary is supplied, so only the
// install-state directory is unresolvable; without its own check the
// writer would create install-state.json in the process working
// directory, and the next run -- resolving a different state dir --
// would find no record and refuse its own previous write as a conflict.
func TestCmdRegister_UnresolvableStateDirIsRefused(t *testing.T) {
	t.Setenv("HOME", "")
	xdgConfig := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgConfig)

	opencodeDir := filepath.Join(xdgConfig, "opencode")
	if err := os.MkdirAll(opencodeDir, 0o755); err != nil {
		t.Fatalf("mkdir opencode config dir: %v", err)
	}
	configPath := filepath.Join(opencodeDir, "opencode.json")
	original := []byte(`{"mcp":{}}`)
	if err := os.WriteFile(configPath, original, 0o600); err != nil {
		t.Fatalf("seed opencode.json: %v", err)
	}

	exit := run([]string{"register", "--target", "opencode", "--binary", "/opt/bin/longterm-mem"})
	if exit == 0 {
		t.Fatalf("run([register --target opencode --binary ...]) with an unresolvable state dir = 0, want non-zero")
	}

	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read opencode config: %v", err)
	}
	if string(got) != string(original) {
		t.Fatalf("config was modified despite an unresolvable state dir:\nbefore = %s\nafter = %s", original, got)
	}
}

// TestCmdRegister_HardFailureOutranksConflict (11b.7): with --target all,
// one runtime refusing an untagged entry (exit 6) and another failing
// outright must not resolve to 6. A conflict is a recoverable, expected
// outcome a caller is meant to resolve by hand; a hard failure means a
// target was not registered at all. Reporting the softer of the two
// silently hides the harder one, which is exactly what cmdRegister's own
// doc comment promises never happens.
func TestCmdRegister_HardFailureOutranksConflict(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	xdgConfig := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgConfig)

	// claude: an untagged same-named entry install-state does not own, so
	// this target conflicts.
	claudeConfig := filepath.Join(home, ".claude.json")
	if err := os.WriteFile(claudeConfig, []byte(`{"mcpServers":{"longterm-mem":{"type":"stdio","command":"/someone/elses/binary"}}}`), 0o600); err != nil {
		t.Fatalf("seed .claude.json: %v", err)
	}
	// opencode: no config file at all, so this target fails outright.

	exit := run([]string{"register", "--target", "all", "--state-dir", t.TempDir(), "--binary", "/opt/bin/longterm-mem"})
	if exit != 1 {
		t.Fatalf("run([register --target all]) with one conflict and one hard failure = %d, want 1 — the hard failure must not be hidden behind the conflict's exit 6", exit)
	}
}

// buildLongtermMemBinary compiles the real longterm-mem binary to a
// scratch path, mirroring TestMain_BuildsIndependentModule's own build
// invocation, so TestCLI_NoResidualProcessAfterAnySubcommand exercises the
// real subprocess dispatch each subcommand goes through, not run() called
// in-process.
func buildLongtermMemBinary(t *testing.T) string {
	t.Helper()
	moduleRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve module root: %v", err)
	}
	binPath := filepath.Join(t.TempDir(), "longterm-mem")
	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/longterm-mem")
	cmd.Dir = moduleRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/longterm-mem failed: %v\n%s", err, out)
	}
	return binPath
}

// writeResidualFixtureVault installs every vault script Runner might
// invoke across index/query/sync/promote (allocate-address.sh,
// setup-retrieve.sh, contextual-prefix.py, bm25-index.py, retrieve.py),
// all fast-exiting fixtures mirroring internal/vault's own fixture
// conventions (index_test.go/retrieve_test.go), and pre-provisions the
// vault so Rebuild's setupScript step is skipped -- only the two refresh
// scripts and retrieve.py are exercised by the subcommands under test.
func writeResidualFixtureVault(t *testing.T, vaultRoot string) {
	t.Helper()

	binDir := filepath.Join(vaultRoot, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", binDir, err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "setup-retrieve.sh"), []byte("#!/bin/sh\nmkdir -p .vault-meta/bm25 .vault-meta/chunks\nprintf '{}' > .vault-meta/bm25/index.json\n: > .vault-meta/chunks/chunk-0.json\n"), 0o755); err != nil {
		t.Fatalf("write setup-retrieve.sh fixture: %v", err)
	}

	scriptsDir := filepath.Join(vaultRoot, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", scriptsDir, err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "contextual-prefix.py"), []byte("import sys\nsys.exit(0)\n"), 0o644); err != nil {
		t.Fatalf("write contextual-prefix.py fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "bm25-index.py"), []byte("import sys\nsys.exit(0)\n"), 0o644); err != nil {
		t.Fatalf("write bm25-index.py fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "retrieve.py"), []byte("import json, sys\nprint(json.dumps({\"candidates\": []}))\nsys.exit(0)\n"), 0o644); err != nil {
		t.Fatalf("write retrieve.py fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "allocate-address.sh"), []byte("#!/bin/sh\nprintf 'c-000902\\n'\n"), 0o755); err != nil {
		t.Fatalf("write allocate-address.sh fixture: %v", err)
	}

	metaBM25 := filepath.Join(vaultRoot, ".vault-meta", "bm25")
	metaChunks := filepath.Join(vaultRoot, ".vault-meta", "chunks")
	if err := os.MkdirAll(metaBM25, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", metaBM25, err)
	}
	if err := os.MkdirAll(metaChunks, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", metaChunks, err)
	}
	if err := os.WriteFile(filepath.Join(metaBM25, "index.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write pre-provisioned bm25 index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(metaChunks, "chunk-0.json"), nil, 0o644); err != nil {
		t.Fatalf("write pre-provisioned chunk marker: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vaultRoot, ".vault-meta", ".longterm-mem-provisioned"), []byte("2026-08-01T00:00:00Z"), 0o644); err != nil {
		t.Fatalf("write provisioned sentinel: %v", err)
	}
}

// TestCLI_NoResidualProcessAfterAnySubcommand (task 8b.9, R-034
// "No CLI subcommand leaves a residual process", -short-skippable
// integration): every subcommand that can invoke a vault subprocess is
// run, end to end, as the real built binary against a fixture project,
// and after each one completes no process referencing the fixture vault's
// own scratch path remains anywhere in the system process list --
// checked via `pgrep -f <vaultRoot>` (a full-cmdline, system-wide match,
// not a parent-scoped one) so a subprocess orphaned by a broken Runner
// call would still be caught even after the kernel reparents it once its
// immediate parent exits.
func TestCLI_NoResidualProcessAfterAnySubcommand(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the real binary and runs it against a full fixture vault; skipped under -short")
	}
	if _, err := exec.LookPath("pgrep"); err != nil {
		t.Skip("pgrep not on PATH; cannot assert no residual process remains")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not on PATH; the fixture vault's Python entrypoints cannot run")
	}

	binPath := buildLongtermMemBinary(t)

	vaultRoot := t.TempDir()
	writeResidualFixtureVault(t, vaultRoot)
	dbPath, id := promoteFixtureDB(t, "Residual Check Target", "cli-residual-project")

	home := t.TempDir()
	env := append(os.Environ(),
		"HOME="+home,
		"LONGTERM_MEM_VAULT="+vaultRoot,
		"LONGTERM_MEM_ENGRAM_DB="+dbPath,
	)

	subcommands := [][]string{
		{"status", "--project", "cli-residual-project"},
		{"doctor", "--project", "cli-residual-project"},
		{"index", "--project", "cli-residual-project"},
		{"query", "--project", "cli-residual-project", "hello"},
		{"promote", "--project", "cli-residual-project", "--id", strconv.FormatInt(id, 10)},
		{"sync", "--project", "cli-residual-project"},
	}

	for _, args := range subcommands {
		cmd := exec.Command(binPath, args...)
		cmd.Env = env
		out, _ := cmd.CombinedOutput() // some subcommands may legitimately exit non-zero; residual-process safety is checked regardless of exit code

		if residual, pgErr := exec.Command("pgrep", "-f", vaultRoot).CombinedOutput(); pgErr == nil {
			t.Fatalf("%v left a residual process referencing the fixture vault: %s\ncommand output:\n%s", args, residual, out)
		}
	}
}
