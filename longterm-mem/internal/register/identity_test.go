package register

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The register suite seeds every fixture at 0o600 — which is exactly the
// mode os.CreateTemp produces — so a writer that silently resets a config's
// permissions to 0o600 on every install looks identical to one that
// preserves them. That structural blind spot is why the mode-preservation
// defect survived: the tests below deliberately seed a DISTINCTIVE mode
// (0o644, 0o444) that no temp file can produce by accident, so the identity
// the user's own config carries is the thing under test, not a coincidence.
const (
	// distinctiveMode is deliberately group/other-readable, so it can never
	// be reproduced by os.CreateTemp's 0o600 default.
	distinctiveMode os.FileMode = 0o644
	// readOnlyMode is a mode the OWNER cannot open for writing. It is the
	// discriminator between "replaced atomically by rename" (works: only
	// the DIRECTORY needs to be writable) and "rewritten in place by
	// truncate" (fails with EACCES). See
	// TestInstallRollback_RestoresAReadOnlyConfig.
	readOnlyMode os.FileMode = 0o444
)

// allGoldenCases is every runtime writer this package ships, so an identity
// assertion added once covers claude, opencode and codex — the JSON and the
// TOML paths — rather than only the one the author happened to think of.
func allGoldenCases() []goldenWriterCase {
	return []goldenWriterCase{claudeGoldenCase(), opencodeGoldenCase(), codexGoldenCase()}
}

// permOf returns path's permission bits, failing the test if path cannot be
// stat'd. It deliberately uses Lstat so a symlink is never silently
// followed: a test asking "what mode is this file" must not be answered
// about a different file.
func permOf(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.Mode().Perm()
}

// seedConfigWithMode seeds the case's before fixture at mode, returning the
// config path. os.WriteFile applies its perm argument only when it CREATES
// the file, and t.TempDir is empty, so the explicit Chmod is what actually
// pins the mode regardless of umask.
func seedConfigWithMode(t *testing.T, c goldenWriterCase, dir string, mode os.FileMode) string {
	t.Helper()
	path := c.seedConfig(t, dir, c.fixtureName("before"))
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("%s: chmod seeded config to %o: %v", c.target, mode, err)
	}
	if got := permOf(t, path); got != mode {
		t.Fatalf("%s: seeded config mode = %o, want %o — the fixture itself is wrong", c.target, got, mode)
	}
	return path
}

// TestRegister_InstallPreservesConfigFileMode: the config file belongs to
// the user, not to longterm-mem. Installing an MCP entry must not silently
// re-permission it — a 0o644 config that comes back 0o600 is a change to
// the user's system nobody asked for, and a 0o600 config that came back
// 0o644 would be a disclosure.
func TestRegister_InstallPreservesConfigFileMode(t *testing.T) {
	for _, c := range allGoldenCases() {
		t.Run(c.target, func(t *testing.T) {
			dir := t.TempDir()
			stateDir := t.TempDir()
			configPath := seedConfigWithMode(t, c, dir, distinctiveMode)

			if err := c.register(dir, stateDir, c.binary1); err != nil {
				t.Fatalf("%s: install returned error: %v", c.target, err)
			}

			if got := permOf(t, configPath); got != distinctiveMode {
				t.Fatalf("%s: config mode after install = %o, want %o — the install re-permissioned the user's own config", c.target, got, distinctiveMode)
			}
		})
	}
}

// TestRegister_ReinstallPreservesConfigFileMode: the replace-in-place path
// (Decide's ActionReplace) is a second, separate write, so it needs its own
// assertion — an install that preserved the mode and a reinstall that did
// not would still lose it on the very next run.
func TestRegister_ReinstallPreservesConfigFileMode(t *testing.T) {
	for _, c := range allGoldenCases() {
		t.Run(c.target, func(t *testing.T) {
			dir := t.TempDir()
			stateDir := t.TempDir()
			configPath := seedConfigWithMode(t, c, dir, distinctiveMode)

			if err := c.register(dir, stateDir, c.binary1); err != nil {
				t.Fatalf("%s: first install returned error: %v", c.target, err)
			}
			if err := c.register(dir, stateDir, c.binary2); err != nil {
				t.Fatalf("%s: reinstall returned error: %v", c.target, err)
			}

			if got := permOf(t, configPath); got != distinctiveMode {
				t.Fatalf("%s: config mode after reinstall = %o, want %o", c.target, got, distinctiveMode)
			}
		})
	}
}

// TestUnregister_PreservesConfigFileMode: uninstalling must leave the
// user's config exactly as it found it, mode included. RemoveMember and
// RemoveTOMLSection are separate writers from their install counterparts,
// with their own copy of the temp+rename sequence, so they can regress
// independently.
func TestUnregister_PreservesConfigFileMode(t *testing.T) {
	for _, c := range allGoldenCases() {
		t.Run(c.target, func(t *testing.T) {
			dir := t.TempDir()
			stateDir := t.TempDir()
			configPath := seedConfigWithMode(t, c, dir, distinctiveMode)

			if err := c.register(dir, stateDir, c.binary1); err != nil {
				t.Fatalf("%s: install returned error: %v", c.target, err)
			}
			outcome, err := c.unregister(dir, stateDir)
			if err != nil {
				t.Fatalf("%s: unregister returned error: %v", c.target, err)
			}
			if outcome != UnregisterRemoved {
				t.Fatalf("%s: unregister outcome = %v, want UnregisterRemoved", c.target, outcome)
			}

			if got := permOf(t, configPath); got != distinctiveMode {
				t.Fatalf("%s: config mode after unregister = %o, want %o", c.target, got, distinctiveMode)
			}
		})
	}
}

// TestInstallRollback_PreservesConfigFileMode covers the fourth write path,
// the one no test reached before: installWithRollback's restore. Its mode
// argument was captured from the config but never applied, because
// os.WriteFile honours a perm argument only when it CREATES the file — and
// by the time the rollback runs, the config always exists (writeConfig's
// own rename just created it). The capture was dead code; this asserts the
// behaviour it only pretended to implement.
func TestInstallRollback_PreservesConfigFileMode(t *testing.T) {
	for _, c := range allGoldenCases() {
		t.Run(c.target, func(t *testing.T) {
			dir := t.TempDir()
			stateDir := t.TempDir()
			configPath := seedConfigWithMode(t, c, dir, distinctiveMode)
			before, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatalf("%s: read seeded config: %v", c.target, err)
			}

			failInstallStateSave(t)

			if err := c.register(dir, stateDir, c.binary1); err == nil {
				t.Fatalf("%s: install with an unsavable install-state returned nil error, want a failure", c.target)
			}

			got, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatalf("%s: read config after rollback: %v", c.target, err)
			}
			if string(got) != string(before) {
				t.Fatalf("%s: rollback did not restore the original bytes:\nbefore =\n%s\nafter =\n%s", c.target, before, got)
			}
			if mode := permOf(t, configPath); mode != distinctiveMode {
				t.Fatalf("%s: config mode after rollback = %o, want %o — the rollback re-permissioned the user's own config while claiming to restore it", c.target, mode, distinctiveMode)
			}
		})
	}
}

// TestInstallRollback_RestoresAReadOnlyConfig is the atomicity assertion
// for the rollback, expressed as something a test can actually observe.
//
// A rename-based replace needs write permission on the DIRECTORY only; a
// truncate-and-write needs write permission on the FILE. So a config the
// owner has deliberately made read-only (0o444 — a real thing people do to
// dotfiles) can be replaced atomically but cannot be rewritten in place.
//
// This test therefore fails two different ways for two different defects:
// it fails on the mode if the rollback keeps re-permissioning the file, and
// it fails on the error if someone "fixes" the mode by bolting a Chmod onto
// a truncating os.WriteFile — because that write would then hit EACCES on
// the very file it was supposed to rescue. Only an atomic replace that also
// carries the mode across satisfies both halves.
func TestInstallRollback_RestoresAReadOnlyConfig(t *testing.T) {
	for _, c := range allGoldenCases() {
		t.Run(c.target, func(t *testing.T) {
			dir := t.TempDir()
			stateDir := t.TempDir()
			configPath := seedConfigWithMode(t, c, dir, readOnlyMode)
			before, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatalf("%s: read seeded config: %v", c.target, err)
			}

			failInstallStateSave(t)

			err = c.register(dir, stateDir, c.binary1)
			if err == nil {
				t.Fatalf("%s: install with an unsavable install-state returned nil error, want a failure", c.target)
			}
			if strings.Contains(err.Error(), "remove the") {
				t.Fatalf("%s: the rollback itself failed on a read-only config — it cannot rewrite the file in place, it has to replace it: %v", c.target, err)
			}

			got, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatalf("%s: read config after rollback: %v", c.target, err)
			}
			if string(got) != string(before) {
				t.Fatalf("%s: rollback did not restore the original bytes of a read-only config:\nbefore =\n%s\nafter =\n%s", c.target, before, got)
			}
			if mode := permOf(t, configPath); mode != readOnlyMode {
				t.Fatalf("%s: config mode after rollback = %o, want %o", c.target, mode, readOnlyMode)
			}

			assertNoTempLeftovers(t, dir)
		})
	}
}

// TestRegister_ConfigSymlinkIsWrittenThrough: a dotfiles layout points
// ~/.claude.json at a file inside a tracked repository. An atomic replace
// that renames onto the LINK deletes the link and leaves a regular file in
// its place: the user's real dotfile never receives the entry, the
// repository never sees a change, and register still reports success. The
// write has to be resolved to the link's target and land there.
func TestRegister_ConfigSymlinkIsWrittenThrough(t *testing.T) {
	for _, c := range allGoldenCases() {
		t.Run(c.target, func(t *testing.T) {
			dir := t.TempDir()
			stateDir := t.TempDir()
			dotfiles := t.TempDir()

			realPath := filepath.Join(dotfiles, "tracked"+c.fixtureExt)
			if err := os.WriteFile(realPath, c.readFixture(t, c.fixtureName("before")), 0o600); err != nil {
				t.Fatalf("%s: seed dotfiles target: %v", c.target, err)
			}
			if err := os.Chmod(realPath, distinctiveMode); err != nil {
				t.Fatalf("%s: chmod dotfiles target: %v", c.target, err)
			}
			configPath := filepath.Join(dir, c.configFileName)
			if err := os.Symlink(realPath, configPath); err != nil {
				t.Fatalf("%s: symlink %s -> %s: %v", c.target, configPath, realPath, err)
			}

			if err := c.register(dir, stateDir, c.binary1); err != nil {
				t.Fatalf("%s: install returned error: %v", c.target, err)
			}

			info, err := os.Lstat(configPath)
			if err != nil {
				t.Fatalf("%s: lstat config after install: %v", c.target, err)
			}
			if info.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("%s: %s is no longer a symlink after install (mode %v) — the write replaced the user's link instead of following it", c.target, configPath, info.Mode())
			}

			landed, err := os.ReadFile(realPath)
			if err != nil {
				t.Fatalf("%s: read the symlink's target after install: %v", c.target, err)
			}
			want := c.readFixture(t, c.fixtureName("after-install"))
			if string(landed) != string(want) {
				t.Fatalf("%s: the symlink's real target did not receive the install:\ngot =\n%s\nwant =\n%s", c.target, landed, want)
			}
			if mode := permOf(t, realPath); mode != distinctiveMode {
				t.Fatalf("%s: the symlink's real target mode = %o, want %o", c.target, mode, distinctiveMode)
			}
		})
	}
}

// TestRegister_BackupSurvivesAReadOnlyPredecessor: the .bak files are the
// entire recovery story for a config edit gone wrong, and they were the one
// write in this package still done with a plain truncating os.WriteFile —
// exactly the discipline the atomic writers exist to avoid. A .bak left
// read-only by a previous run (or by a user who hardened their backups)
// makes that visible: a truncating write fails outright, an atomic replace
// succeeds and keeps the mode.
func TestRegister_BackupSurvivesAReadOnlyPredecessor(t *testing.T) {
	for _, c := range allGoldenCases() {
		t.Run(c.target, func(t *testing.T) {
			dir := t.TempDir()
			stateDir := t.TempDir()
			configPath := c.seedConfig(t, dir, c.fixtureName("before"))
			bakPath := configPath + ".bak"
			if err := os.WriteFile(bakPath, []byte("stale backup from an earlier run\n"), 0o600); err != nil {
				t.Fatalf("%s: seed stale .bak: %v", c.target, err)
			}
			if err := os.Chmod(bakPath, readOnlyMode); err != nil {
				t.Fatalf("%s: chmod stale .bak: %v", c.target, err)
			}

			if err := c.register(dir, stateDir, c.binary1); err != nil {
				t.Fatalf("%s: install returned error with a read-only stale .bak present: %v", c.target, err)
			}

			bak, err := os.ReadFile(bakPath)
			if err != nil {
				t.Fatalf("%s: read .bak after install: %v", c.target, err)
			}
			want := c.readFixture(t, c.fixtureName("before"))
			if string(bak) != string(want) {
				t.Fatalf("%s: .bak = %s, want the pre-install bytes %s", c.target, bak, want)
			}
			if mode := permOf(t, bakPath); mode != readOnlyMode {
				t.Fatalf("%s: .bak mode after install = %o, want %o", c.target, mode, readOnlyMode)
			}
		})
	}
}

// TestRegister_AtomicReplaceBreaksHardlinksOnPurpose pins the one identity
// property this package deliberately does NOT preserve. Replacing a file by
// rename always detaches it from any hardlink pointing at the old inode;
// preserving the link would mean writing through the existing inode, which
// is precisely the truncate-and-write behaviour that can leave a config
// half-written after a crash.
//
// The trade-off is atomicity over link identity, and it is recorded here as
// an executable statement rather than only a comment, so a future change
// cannot quietly "fix" hardlinks by giving up the guarantee that matters.
func TestRegister_AtomicReplaceBreaksHardlinksOnPurpose(t *testing.T) {
	c := claudeGoldenCase()
	dir := t.TempDir()
	stateDir := t.TempDir()
	configPath := c.seedConfig(t, dir, c.fixtureName("before"))
	before, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read seeded config: %v", err)
	}

	linkPath := filepath.Join(dir, "hardlink.json")
	if err := os.Link(configPath, linkPath); err != nil {
		t.Fatalf("hardlink %s -> %s: %v", linkPath, configPath, err)
	}

	if err := c.register(dir, stateDir, c.binary1); err != nil {
		t.Fatalf("install returned error: %v", err)
	}

	linked, err := os.ReadFile(linkPath)
	if err != nil {
		t.Fatalf("read hardlink after install: %v", err)
	}
	if string(linked) != string(before) {
		t.Fatalf("the hardlink followed the install — the config was rewritten through its existing inode instead of being replaced atomically:\nlink =\n%s\nwant (pre-install bytes) =\n%s", linked, before)
	}
}

// failInstallStateSave forces the one failure the filesystem cannot provoke
// on its own: install-state's save failing AFTER the config write already
// landed. See installWithRollback for why that window is the failure a
// re-run cannot recover from.
func failInstallStateSave(t *testing.T) {
	t.Helper()
	restore := saveInstallState
	saveInstallState = func(*InstallState, string) error { return errors.New("install-state is unwritable") }
	t.Cleanup(func() { saveInstallState = restore })
}

// assertNoTempLeftovers fails if dir still holds any of this package's
// temp files. Every durable write removes its temp file on both the success
// and the failure path; a leftover means a write aborted somewhere it did
// not clean up after itself.
func assertNoTempLeftovers(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Fatalf("temp file %s survived in %s — a partially written replacement is still observable", e.Name(), dir)
		}
	}
}

// TestJSONInstall_RollbackIsReachableWithoutStubbingTheSave is the
// counter-example to writer.go's own claim that the rollback can never run
// outside a test: it asserts a purely filesystem-level way to reach it.
//
// LoadInstallState treats a missing install-state.json as an empty state
// and no error, so a state directory that exists but is not writable loads
// fine and only fails at Save — the exact window installWithRollback is
// for. A comment asserting an error path is unreachable is how that path
// stayed untested; this test makes the claim falsifiable.
func TestJSONInstall_RollbackIsReachableWithoutStubbingTheSave(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	original := []byte("{\n  \"mcpServers\": {\n    \"other-project-server\": {\"type\": \"stdio\"}\n  }\n}\n")
	if err := os.WriteFile(configPath, original, 0o600); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	if err := os.Chmod(configPath, distinctiveMode); err != nil {
		t.Fatalf("chmod seeded config: %v", err)
	}

	// A real, un-stubbed unwritable state directory: install-state.json
	// does not exist inside it, so the load succeeds and only the save
	// fails.
	stateDir := filepath.Join(t.TempDir(), "state")
	if err := os.Mkdir(stateDir, 0o500); err != nil {
		t.Fatalf("mkdir read-only state dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(stateDir, 0o700) })

	err := jsonInstall("claude", configPath, stateDir, "mcpServers", "longterm-mem", json.RawMessage(`{"type":"stdio"}`))
	if err == nil {
		t.Fatal("jsonInstall against an unwritable state directory returned nil error — the rollback window was never entered")
	}

	got, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("read config after the failed install: %v", readErr)
	}
	if string(got) != string(original) {
		t.Fatalf("the config kept a longterm-mem entry install-state has no record of:\nbefore =\n%s\nafter =\n%s", original, got)
	}
	if mode := permOf(t, configPath); mode != distinctiveMode {
		t.Fatalf("config mode after rollback = %o, want %o", mode, distinctiveMode)
	}
}
