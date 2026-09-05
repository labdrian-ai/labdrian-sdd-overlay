package main

// Integration harness for `bin/labdrian-overlay`'s backup/restore and
// surfacing additions (overlay-backup-restore R-001..R-003,
// overlay-release-surfacing R-001..R-002). This is a Go-to-bash test,
// following the exact pattern established by selfupdate_backend_test.go and
// release_backend_test.go: it spawns the REAL backend script against
// hermetic scratch git repos / plain scratch directories, reusing shared
// helpers (gitTestEnv, runGit, writeFile, realBackendBin, runBackendFunc,
// runBackendSubcommandWithHome, newScratchRepo, pushUpstreamTag,
// writeDigestFixtureFiles) since all of these live in package main.
//
// `restore` (design.md's threat matrix: "restore performs zero git
// operations") is deliberately exercised with a plain, non-git OVERLAY_DIR
// in several tests below -- any accidental git invocation inside cmd_restore
// would fail outright against a directory with no `.git`, so a passing test
// there is itself evidence of the zero-git-ops contract.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// task 3a.1: backup_target / prune_backups (overlay-backup-restore R-001, R-002)
// ---------------------------------------------------------------------------

func TestRestoreBackend_BackupTarget_CreatesBackupBeforeOverwrite(t *testing.T) {
	overlayDir := t.TempDir()
	homeDir := t.TempDir()
	writeDigestFixtureFiles(t, overlayDir, homeDir)
	writeFile(t, filepath.Join(overlayDir, "overlay.manifest"), "alpha/SKILL.md managed\nbeta/SKILL.md managed\n")

	// Mutate the repo source (about to be deployed) so live (unmutated)
	// differs from what's about to be applied -- claude's live beta file
	// still holds its ORIGINAL content at this point.
	writeFile(t, filepath.Join(overlayDir, "skills", "beta", "SKILL.md"), "beta content NEW VERSION\n")

	out, code := runBackendFunc(t, overlayDir, homeDir, "backup_target", "claude")
	if code != 0 {
		t.Fatalf("backup_target exit=%d, want 0\noutput:\n%s", code, out)
	}
	ts := strings.TrimSpace(out)
	if ts == "" {
		t.Fatalf("backup_target produced no timestamp when a deployed file differs from what's about to be applied")
	}

	backupFile := filepath.Join(homeDir, ".labdrian-overlay", "backups", "claude", ts, "skills", "beta", "SKILL.md")
	data, err := os.ReadFile(backupFile)
	if err != nil {
		t.Fatalf("backup file not created at %s: %v", backupFile, err)
	}
	if string(data) != "beta content\n" {
		t.Errorf("backup captured %q, want the PRE-change live content %q", data, "beta content\n")
	}

	// alpha never differed -- it must not be swept into the backup.
	if _, err := os.Stat(filepath.Join(homeDir, ".labdrian-overlay", "backups", "claude", ts, "skills", "alpha", "SKILL.md")); err == nil {
		t.Errorf("backup unexpectedly captured alpha/SKILL.md, which never differed")
	}
}

func TestRestoreBackend_BackupTarget_NoOpCreatesNoBackup(t *testing.T) {
	overlayDir := t.TempDir()
	homeDir := t.TempDir()
	writeDigestFixtureFiles(t, overlayDir, homeDir)
	writeFile(t, filepath.Join(overlayDir, "overlay.manifest"), "alpha/SKILL.md managed\nbeta/SKILL.md managed\n")

	out, code := runBackendFunc(t, overlayDir, homeDir, "backup_target", "claude")
	if code != 0 {
		t.Fatalf("backup_target exit=%d, want 0\noutput:\n%s", code, out)
	}
	if got := strings.TrimSpace(out); got != "" {
		t.Errorf("backup_target produced a timestamp %q for a true no-op (live already matches what would be deployed)", got)
	}

	backupsDir := filepath.Join(homeDir, ".labdrian-overlay", "backups", "claude")
	if _, err := os.Stat(backupsDir); err == nil {
		t.Errorf("backup_target created a backups directory for a no-op apply: %s", backupsDir)
	}
}

// A target with NO prior deployed file at all (never applied before) also
// has nothing meaningful to preserve -- this must degrade like a no-op, not
// fabricate a backup of a file that was never actually deployed.
func TestRestoreBackend_BackupTarget_NeverDeployedCreatesNoBackup(t *testing.T) {
	overlayDir := t.TempDir()
	homeDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(overlayDir, "skills", "alpha"), 0o755); err != nil {
		t.Fatalf("mkdir skills/alpha: %v", err)
	}
	writeFile(t, filepath.Join(overlayDir, "skills", "alpha", "SKILL.md"), "alpha content\n")
	writeFile(t, filepath.Join(overlayDir, "overlay.manifest"), "alpha/SKILL.md managed\n")
	// Deliberately NOT deployed to $HOME/.claude/skills/alpha/SKILL.md.

	out, code := runBackendFunc(t, overlayDir, homeDir, "backup_target", "claude")
	if code != 0 {
		t.Fatalf("backup_target exit=%d, want 0\noutput:\n%s", code, out)
	}
	if got := strings.TrimSpace(out); got != "" {
		t.Errorf("backup_target produced a timestamp %q for a never-deployed target (nothing to preserve)", got)
	}
}

func TestRestoreBackend_PruneBackups_FourthPrunesOldest(t *testing.T) {
	overlayDir := t.TempDir()
	homeDir := t.TempDir()

	backupsDir := filepath.Join(homeDir, ".labdrian-overlay", "backups", "claude")
	timestamps := []string{
		"20260101T000000Z",
		"20260101T000001Z",
		"20260101T000002Z",
		"20260101T000003Z",
	}
	for _, ts := range timestamps {
		if err := os.MkdirAll(filepath.Join(backupsDir, ts), 0o755); err != nil {
			t.Fatalf("mkdir backup %s: %v", ts, err)
		}
	}

	out, code := runBackendFunc(t, overlayDir, homeDir, "prune_backups", "claude")
	if code != 0 {
		t.Fatalf("prune_backups exit=%d, want 0\noutput:\n%s", code, out)
	}

	entries, err := os.ReadDir(backupsDir)
	if err != nil {
		t.Fatalf("read backups dir: %v", err)
	}
	if len(entries) != 3 {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Fatalf("backups dir has %d entries after prune, want 3: %v", len(entries), names)
	}
	if _, err := os.Stat(filepath.Join(backupsDir, timestamps[0])); err == nil {
		t.Errorf("oldest backup %s was not pruned", timestamps[0])
	}
	for _, ts := range timestamps[1:] {
		if _, err := os.Stat(filepath.Join(backupsDir, ts)); err != nil {
			t.Errorf("newer backup %s was unexpectedly removed", ts)
		}
	}
}

// Regression: backup_target's same-second collision suffix ("<ts>-1", "<ts>-2", ...)
// must not invert prune order. Bash's glob expansion sorts paths *with* their
// trailing slash, under which "<ts>-1/" sorts before "<ts>/" (ASCII '-' < '/'),
// even though "<ts>-1" is the chronologically newer backup.
func TestRestoreBackend_PruneBackups_SameSecondCollisionPrunesTrueOldest(t *testing.T) {
	overlayDir := t.TempDir()
	homeDir := t.TempDir()

	backupsDir := filepath.Join(homeDir, ".labdrian-overlay", "backups", "claude")
	// Chronological order: 002Z (oldest) < 002Z-1 < 002Z-2 < 003Z (newest).
	timestamps := []string{
		"20260101T000002Z",
		"20260101T000002Z-1",
		"20260101T000002Z-2",
		"20260101T000003Z",
	}
	for _, ts := range timestamps {
		if err := os.MkdirAll(filepath.Join(backupsDir, ts), 0o755); err != nil {
			t.Fatalf("mkdir backup %s: %v", ts, err)
		}
	}

	out, code := runBackendFunc(t, overlayDir, homeDir, "prune_backups", "claude")
	if code != 0 {
		t.Fatalf("prune_backups exit=%d, want 0\noutput:\n%s", code, out)
	}

	if _, err := os.Stat(filepath.Join(backupsDir, timestamps[0])); err == nil {
		t.Errorf("true oldest backup %s was not pruned", timestamps[0])
	}
	for _, ts := range timestamps[1:] {
		if _, err := os.Stat(filepath.Join(backupsDir, ts)); err != nil {
			t.Errorf("newer backup %s was unexpectedly removed", ts)
		}
	}
}

// End-to-end: cmd_apply itself must invoke backup_target -- a second apply
// that lands a real content change over an already-deployed target creates
// exactly one new backup containing the PRE-change content.
func TestRestoreBackend_Apply_CreatesBackupBeforeOverwrite(t *testing.T) {
	_, clone := newScratchRepoWithUpstream(t)

	runGit(t, clone, "checkout", "upstream")
	if err := os.MkdirAll(filepath.Join(clone, "skills", "hello"), 0o755); err != nil {
		t.Fatalf("mkdir skills/hello: %v", err)
	}
	writeFile(t, filepath.Join(clone, "overlay.manifest"), "hello/SKILL.md managed\n")
	writeFile(t, filepath.Join(clone, "skills", "hello", "SKILL.md"), "hello v1\n")
	runGit(t, clone, "add", "overlay.manifest", "skills/hello/SKILL.md")
	runGit(t, clone, "commit", "-m", "add hello skill v1")
	runGit(t, clone, "push", "origin", "upstream")
	runGit(t, clone, "checkout", "feature-x")

	fakeHome := t.TempDir()
	if out, code := runBackendSubcommandWithHome(t, clone, fakeHome, "apply", "--target", "claude"); code != 0 {
		t.Fatalf("first apply exit=%d, want 0\noutput:\n%s", code, out)
	}

	// First-ever deploy: nothing was previously live, so nothing to
	// preserve -- expect zero backups so far.
	backupsDir := filepath.Join(fakeHome, ".labdrian-overlay", "backups", "claude")
	firstEntries, _ := os.ReadDir(backupsDir)
	if len(firstEntries) != 0 {
		t.Fatalf("first (never-deployed) apply created %d backup(s), want 0", len(firstEntries))
	}

	// Land a new version on upstream and merge it into main via a second apply.
	runGit(t, clone, "checkout", "upstream")
	writeFile(t, filepath.Join(clone, "skills", "hello", "SKILL.md"), "hello v2\n")
	runGit(t, clone, "add", "skills/hello/SKILL.md")
	runGit(t, clone, "commit", "-m", "hello v2")
	runGit(t, clone, "push", "origin", "upstream")
	runGit(t, clone, "checkout", "feature-x")

	if out, code := runBackendSubcommandWithHome(t, clone, fakeHome, "apply", "--target", "claude"); code != 0 {
		t.Fatalf("second apply exit=%d, want 0\noutput:\n%s", code, out)
	}

	entries, err := os.ReadDir(backupsDir)
	if err != nil {
		t.Fatalf("read backups dir after second apply: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("backups dir has %d entries after second apply, want exactly 1 new backup", len(entries))
	}

	backupFile := filepath.Join(backupsDir, entries[0].Name(), "skills", "hello", "SKILL.md")
	data, err := os.ReadFile(backupFile)
	if err != nil {
		t.Fatalf("backup file not found: %v", err)
	}
	if string(data) != "hello v1\n" {
		t.Errorf("backup content = %q, want pre-change v1 content", data)
	}
}

// ---------------------------------------------------------------------------
// task 3a.2: cmd_restore (design decision D4, overlay-backup-restore R-003)
// ---------------------------------------------------------------------------

func TestRestoreBackend_Restore_MatchesLatestBackup(t *testing.T) {
	overlayDir := t.TempDir()
	homeDir := t.TempDir()
	writeDigestFixtureFiles(t, overlayDir, homeDir)
	writeFile(t, filepath.Join(overlayDir, "overlay.manifest"), "alpha/SKILL.md managed\nbeta/SKILL.md managed\n")

	backupsDir := filepath.Join(homeDir, ".labdrian-overlay", "backups", "claude")
	older := filepath.Join(backupsDir, "20260101T000000Z")
	newer := filepath.Join(backupsDir, "20260101T000100Z")
	for _, dir := range []string{older, newer} {
		if err := os.MkdirAll(filepath.Join(dir, "skills", "alpha"), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		if err := os.MkdirAll(filepath.Join(dir, "skills", "beta"), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	writeFile(t, filepath.Join(older, "skills", "alpha", "SKILL.md"), "alpha OLDER\n")
	writeFile(t, filepath.Join(older, "skills", "beta", "SKILL.md"), "beta OLDER\n")
	writeFile(t, filepath.Join(older, ".meta"), "v1.0.0\tolder-digest\t2026-01-01T00:00:00Z")
	writeFile(t, filepath.Join(newer, "skills", "alpha", "SKILL.md"), "alpha NEWER\n")
	writeFile(t, filepath.Join(newer, "skills", "beta", "SKILL.md"), "beta NEWER\n")
	writeFile(t, filepath.Join(newer, ".meta"), "v1.1.0\tnewer-digest\t2026-01-01T00:01:00Z")

	// Mutate live files so a real change is verifiable.
	writeFile(t, filepath.Join(homeDir, ".claude", "skills", "alpha", "SKILL.md"), "alpha DRIFTED LIVE\n")

	out, code := runBackendSubcommandWithHome(t, overlayDir, homeDir, "restore", "--target", "claude")
	if code != 0 {
		t.Fatalf("restore exit=%d, want 0\noutput:\n%s", code, out)
	}

	gotAlpha, err := os.ReadFile(filepath.Join(homeDir, ".claude", "skills", "alpha", "SKILL.md"))
	if err != nil {
		t.Fatalf("read restored alpha: %v", err)
	}
	if string(gotAlpha) != "alpha NEWER\n" {
		t.Errorf("restored alpha = %q, want the MOST RECENT backup's content %q", gotAlpha, "alpha NEWER\n")
	}
	gotBeta, err := os.ReadFile(filepath.Join(homeDir, ".claude", "skills", "beta", "SKILL.md"))
	if err != nil {
		t.Fatalf("read restored beta: %v", err)
	}
	if string(gotBeta) != "beta NEWER\n" {
		t.Errorf("restored beta = %q, want the MOST RECENT backup's content %q", gotBeta, "beta NEWER\n")
	}

	// R-003 says restore rolls the target back; the digest recorded in state
	// afterward must match the just-restored live content, and the version
	// must come from the selected backup's .meta, not be fabricated.
	digestOut, digestCode := runBackendFunc(t, overlayDir, homeDir, "compute_target_digest", "claude", "live")
	if digestCode != 0 {
		t.Fatalf("compute_target_digest exit=%d, want 0\noutput:\n%s", digestCode, digestOut)
	}
	readOut, readCode := runBackendFunc(t, overlayDir, homeDir, "state_read_target", "claude")
	if readCode != 0 {
		t.Fatalf("state_read_target exit=%d, want 0\noutput:\n%s", readCode, readOut)
	}
	fields := strings.Split(strings.TrimSpace(readOut), "\t")
	if len(fields) != 3 {
		t.Fatalf("state_read_target after restore = %q, want 3 tab-separated fields", readOut)
	}
	if fields[0] != "v1.1.0" {
		t.Errorf("recorded version after restore = %q, want v1.1.0 (the restored backup's version)", fields[0])
	}
	if fields[1] != strings.TrimSpace(digestOut) {
		t.Errorf("recorded digest after restore = %q, want it to match the recomputed live digest %q", fields[1], strings.TrimSpace(digestOut))
	}
}

// Regression: same-second collision suffix must not fool restore's "most
// recent" default. "20260101T000002Z-1" only exists because backup_target
// found "20260101T000002Z" already taken in the same second, so it IS the
// true most recent backup even though it lexically looks like a suffix of
// the other.
func TestRestoreBackend_Restore_SameSecondCollisionPicksTrueMostRecent(t *testing.T) {
	overlayDir := t.TempDir()
	homeDir := t.TempDir()
	writeDigestFixtureFiles(t, overlayDir, homeDir)
	writeFile(t, filepath.Join(overlayDir, "overlay.manifest"), "alpha/SKILL.md managed\nbeta/SKILL.md managed\n")

	backupsDir := filepath.Join(homeDir, ".labdrian-overlay", "backups", "claude")
	older := filepath.Join(backupsDir, "20260101T000002Z")
	trueNewest := filepath.Join(backupsDir, "20260101T000002Z-1")
	for _, dir := range []string{older, trueNewest} {
		if err := os.MkdirAll(filepath.Join(dir, "skills", "alpha"), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		if err := os.MkdirAll(filepath.Join(dir, "skills", "beta"), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	writeFile(t, filepath.Join(older, "skills", "alpha", "SKILL.md"), "alpha OLDER\n")
	writeFile(t, filepath.Join(older, "skills", "beta", "SKILL.md"), "beta OLDER\n")
	writeFile(t, filepath.Join(older, ".meta"), "v1.0.0\tolder-digest\t2026-01-01T00:00:02Z")
	writeFile(t, filepath.Join(trueNewest, "skills", "alpha", "SKILL.md"), "alpha NEWEST\n")
	writeFile(t, filepath.Join(trueNewest, "skills", "beta", "SKILL.md"), "beta NEWEST\n")
	writeFile(t, filepath.Join(trueNewest, ".meta"), "v1.1.0\tnewest-digest\t2026-01-01T00:00:02Z")

	out, code := runBackendSubcommandWithHome(t, overlayDir, homeDir, "restore", "--target", "claude")
	if code != 0 {
		t.Fatalf("restore exit=%d, want 0\noutput:\n%s", code, out)
	}

	gotAlpha, err := os.ReadFile(filepath.Join(homeDir, ".claude", "skills", "alpha", "SKILL.md"))
	if err != nil {
		t.Fatalf("read restored alpha: %v", err)
	}
	if string(gotAlpha) != "alpha NEWEST\n" {
		t.Errorf("restored alpha = %q, want the TRUE most recent backup's content %q (same-second collision must not invert order)", gotAlpha, "alpha NEWEST\n")
	}
}

func TestRestoreBackend_Restore_NoBackupExitsNonZeroNoFileChanges(t *testing.T) {
	overlayDir := t.TempDir()
	homeDir := t.TempDir()
	writeDigestFixtureFiles(t, overlayDir, homeDir)
	writeFile(t, filepath.Join(overlayDir, "overlay.manifest"), "alpha/SKILL.md managed\nbeta/SKILL.md managed\n")

	before, err := os.ReadFile(filepath.Join(homeDir, ".claude", "skills", "alpha", "SKILL.md"))
	if err != nil {
		t.Fatalf("read alpha before: %v", err)
	}

	out, code := runBackendSubcommandWithHome(t, overlayDir, homeDir, "restore", "--target", "claude")
	if code == 0 {
		t.Fatalf("restore exit=0, want nonzero when no backups exist\noutput:\n%s", out)
	}
	if !strings.Contains(out, "No backups available") {
		t.Errorf("restore error does not name the no-backups condition:\n%s", out)
	}

	after, err := os.ReadFile(filepath.Join(homeDir, ".claude", "skills", "alpha", "SKILL.md"))
	if err != nil {
		t.Fatalf("read alpha after: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("live file changed despite the no-backup refusal")
	}
}

func TestRestoreBackend_Restore_RefusesTargetAll(t *testing.T) {
	overlayDir := t.TempDir()
	homeDir := t.TempDir()

	out, code := runBackendSubcommandWithHome(t, overlayDir, homeDir, "restore", "--target", "all")
	if code == 0 {
		t.Fatalf("restore --target all exit=0, want nonzero (restore refuses 'all')\noutput:\n%s", out)
	}
	if !strings.Contains(out, "all") {
		t.Errorf("refusal does not mention 'all':\n%s", out)
	}
}

func TestRestoreBackend_Restore_ListShowsRetainedBackups(t *testing.T) {
	overlayDir := t.TempDir()
	homeDir := t.TempDir()

	backupsDir := filepath.Join(homeDir, ".labdrian-overlay", "backups", "claude")
	ts := "20260101T000000Z"
	if err := os.MkdirAll(filepath.Join(backupsDir, ts), 0o755); err != nil {
		t.Fatalf("mkdir backup: %v", err)
	}
	writeFile(t, filepath.Join(backupsDir, ts, ".meta"), "v1.0.0\tsome-digest\t2026-01-01T00:00:00Z")

	out, code := runBackendSubcommandWithHome(t, overlayDir, homeDir, "restore", "--target", "claude", "--list")
	if code != 0 {
		t.Fatalf("restore --list exit=%d, want 0\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, ts) {
		t.Errorf("--list output does not name the retained backup %q:\n%s", ts, out)
	}
	if !strings.Contains(out, "v1.0.0") {
		t.Errorf("--list output does not show the backup's recorded version:\n%s", out)
	}
}

func TestRestoreBackend_Restore_ListNoBackupsIsNotAnError(t *testing.T) {
	overlayDir := t.TempDir()
	homeDir := t.TempDir()

	out, code := runBackendSubcommandWithHome(t, overlayDir, homeDir, "restore", "--target", "claude", "--list")
	if code != 0 {
		t.Fatalf("restore --list exit=%d, want 0 even with zero backups\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "No backups") {
		t.Errorf("--list with zero backups does not say so plainly:\n%s", out)
	}
}

func TestRestoreBackend_Restore_BackupFlagSelectsSpecificTimestamp(t *testing.T) {
	overlayDir := t.TempDir()
	homeDir := t.TempDir()
	writeDigestFixtureFiles(t, overlayDir, homeDir)
	writeFile(t, filepath.Join(overlayDir, "overlay.manifest"), "alpha/SKILL.md managed\n")

	backupsDir := filepath.Join(homeDir, ".labdrian-overlay", "backups", "claude")
	older := "20260101T000000Z"
	newer := "20260101T000100Z"
	for _, ts := range []string{older, newer} {
		if err := os.MkdirAll(filepath.Join(backupsDir, ts, "skills", "alpha"), 0o755); err != nil {
			t.Fatalf("mkdir backup %s: %v", ts, err)
		}
	}
	writeFile(t, filepath.Join(backupsDir, older, "skills", "alpha", "SKILL.md"), "alpha OLDER\n")
	writeFile(t, filepath.Join(backupsDir, newer, "skills", "alpha", "SKILL.md"), "alpha NEWER\n")

	out, code := runBackendSubcommandWithHome(t, overlayDir, homeDir, "restore", "--target", "claude", "--backup", older)
	if code != 0 {
		t.Fatalf("restore --backup exit=%d, want 0\noutput:\n%s", code, out)
	}

	got, err := os.ReadFile(filepath.Join(homeDir, ".claude", "skills", "alpha", "SKILL.md"))
	if err != nil {
		t.Fatalf("read restored alpha: %v", err)
	}
	if string(got) != "alpha OLDER\n" {
		t.Errorf("restore --backup %s restored %q, want the EXPLICITLY selected older content %q", older, got, "alpha OLDER\n")
	}
}

func TestRestoreBackend_Restore_UnknownBackupTimestampErrors(t *testing.T) {
	overlayDir := t.TempDir()
	homeDir := t.TempDir()
	writeDigestFixtureFiles(t, overlayDir, homeDir)
	writeFile(t, filepath.Join(overlayDir, "overlay.manifest"), "alpha/SKILL.md managed\n")

	backupsDir := filepath.Join(homeDir, ".labdrian-overlay", "backups", "claude", "20260101T000000Z")
	if err := os.MkdirAll(filepath.Join(backupsDir, "skills", "alpha"), 0o755); err != nil {
		t.Fatalf("mkdir backup: %v", err)
	}
	writeFile(t, filepath.Join(backupsDir, "skills", "alpha", "SKILL.md"), "alpha content\n")

	out, code := runBackendSubcommandWithHome(t, overlayDir, homeDir, "restore", "--target", "claude", "--backup", "20990101T000000Z")
	if code == 0 {
		t.Fatalf("restore --backup with an unknown timestamp exit=0, want nonzero\noutput:\n%s", out)
	}
	if !strings.Contains(out, "not found") {
		t.Errorf("restore does not report the unknown-timestamp condition clearly:\n%s", out)
	}
}

// restore must never touch git, even when OVERLAY_DIR is not a git
// repository at all -- any accidental git invocation would fail outright
// here, so a clean pass is itself evidence of zero git operations.
func TestRestoreBackend_Restore_ZeroGitOps(t *testing.T) {
	overlayDir := t.TempDir() // deliberately NOT a git repository
	homeDir := t.TempDir()
	writeDigestFixtureFiles(t, overlayDir, homeDir)
	writeFile(t, filepath.Join(overlayDir, "overlay.manifest"), "alpha/SKILL.md managed\n")

	backupsDir := filepath.Join(homeDir, ".labdrian-overlay", "backups", "claude", "20260101T000000Z")
	if err := os.MkdirAll(filepath.Join(backupsDir, "skills", "alpha"), 0o755); err != nil {
		t.Fatalf("mkdir backup: %v", err)
	}
	writeFile(t, filepath.Join(backupsDir, "skills", "alpha", "SKILL.md"), "alpha RESTORED\n")

	out, code := runBackendSubcommandWithHome(t, overlayDir, homeDir, "restore", "--target", "claude")
	if code != 0 {
		t.Fatalf("restore exit=%d, want 0 even with a non-git OVERLAY_DIR (restore performs zero git operations)\noutput:\n%s", code, out)
	}
	got, err := os.ReadFile(filepath.Join(homeDir, ".claude", "skills", "alpha", "SKILL.md"))
	if err != nil {
		t.Fatalf("read restored alpha: %v", err)
	}
	if string(got) != "alpha RESTORED\n" {
		t.Errorf("restore did not apply the backup content: %q", got)
	}
}

// ---------------------------------------------------------------------------
// task 3a.3: cmd_doctor per-target digest row (overlay-release-surfacing R-001)
// ---------------------------------------------------------------------------

func TestRestoreBackend_Doctor_InSyncTargetPasses(t *testing.T) {
	overlayDir := t.TempDir()
	homeDir := t.TempDir()
	writeDigestFixtureFiles(t, overlayDir, homeDir)
	writeFile(t, filepath.Join(overlayDir, "overlay.manifest"), "alpha/SKILL.md managed\nbeta/SKILL.md managed\n")

	digestOut, digestCode := runBackendFunc(t, overlayDir, homeDir, "compute_target_digest", "claude", "live")
	if digestCode != 0 {
		t.Fatalf("compute_target_digest exit=%d, want 0\noutput:\n%s", digestCode, digestOut)
	}
	digest := strings.TrimSpace(digestOut)
	if _, code := runBackendFunc(t, overlayDir, homeDir, "state_write_target", "claude", "v1.0.0", digest); code != 0 {
		t.Fatalf("state_write_target setup call failed")
	}

	out, code := runBackendSubcommandWithHome(t, overlayDir, homeDir, "doctor")
	if code != 0 {
		t.Fatalf("doctor exit=%d, want 0\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "PASS  claude:") {
		t.Errorf("doctor does not report an in-sync PASS row for claude:\n%s", out)
	}
}

func TestRestoreBackend_Doctor_DriftedTargetWarnsExitZero(t *testing.T) {
	overlayDir := t.TempDir()
	homeDir := t.TempDir()
	writeDigestFixtureFiles(t, overlayDir, homeDir)
	writeFile(t, filepath.Join(overlayDir, "overlay.manifest"), "alpha/SKILL.md managed\nbeta/SKILL.md managed\n")

	if _, code := runBackendFunc(t, overlayDir, homeDir, "state_write_target", "claude", "v1.0.0", "stale-digest-does-not-match"); code != 0 {
		t.Fatalf("state_write_target setup call failed")
	}

	out, code := runBackendSubcommandWithHome(t, overlayDir, homeDir, "doctor")
	if code != 0 {
		t.Fatalf("doctor exit=%d, want 0 (a digest drift WARN must not fail doctor's exit code)\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "WARN  claude:") {
		t.Errorf("doctor does not report a drift WARN row for claude:\n%s", out)
	}
	if !strings.Contains(out, "run 'labdrian apply --target claude'") {
		t.Errorf("doctor's drift WARN does not recommend apply:\n%s", out)
	}
}

func TestRestoreBackend_Doctor_ExistingChecksUnaffected(t *testing.T) {
	overlayDir := t.TempDir()
	homeDir := t.TempDir()

	out, code := runBackendSubcommandWithHome(t, overlayDir, homeDir, "doctor")
	if code != 0 {
		t.Fatalf("doctor exit=%d, want 0\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "PASS  go on PATH") {
		t.Errorf("doctor's pre-existing go-on-PATH check regressed:\n%s", out)
	}
	if !strings.Contains(out, "doctor: OK") {
		t.Errorf("doctor's pre-existing OK summary regressed:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// task 3a.4: cmd_version + --version alias (overlay-release-surfacing R-002)
// ---------------------------------------------------------------------------

func TestRestoreBackend_Version_NeverDeployedTarget(t *testing.T) {
	_, clone := newScratchRepo(t)
	fakeHome := t.TempDir()

	out, code := runBackendSubcommandWithHome(t, clone, fakeHome, "version")
	if code != 0 {
		t.Fatalf("version exit=%d, want 0\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "never deployed") {
		t.Errorf("version does not report a never-deployed target honestly:\n%s", out)
	}
}

func TestRestoreBackend_Version_BehindTargetReportedByName(t *testing.T) {
	origin, clone := newScratchRepo(t)
	pushUpstreamTag(t, origin, "v1.4.0", "release 1.4.0")

	fakeHome := t.TempDir()
	if _, code := runBackendFunc(t, clone, fakeHome, "state_write_target", "claude", "v1.3.0", "some-digest"); code != 0 {
		t.Fatalf("state_write_target setup call failed")
	}

	out, code := runBackendSubcommandWithHome(t, clone, fakeHome, "version")
	if code != 0 {
		t.Fatalf("version exit=%d, want 0\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "v1.4.0") {
		t.Errorf("version does not report the repository's current release version:\n%s", out)
	}
	if !strings.Contains(out, "claude: v1.3.0 (behind)") {
		t.Errorf("version does not report claude as behind by name:\n%s", out)
	}
}

func TestRestoreBackend_VersionFlag_AliasesVersionCommand(t *testing.T) {
	_, clone := newScratchRepo(t)
	fakeHome := t.TempDir()

	out, code := runBackendSubcommandWithHome(t, clone, fakeHome, "--version")
	if code != 0 {
		t.Fatalf("--version exit=%d, want 0\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "never deployed") {
		t.Errorf("--version alias does not behave like the version command:\n%s", out)
	}
}
