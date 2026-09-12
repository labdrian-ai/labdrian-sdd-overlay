package pipkg_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pipkg"
)

// gitFixtureOverlay builds the same minimal overlay tree as fixtureOverlay,
// then git-inits it and commits everything as the initial commit. Returns
// the overlay root, the registry path, and the initial commit's SHA.
// Skipped under -short (spawns real git subprocesses).
func gitFixtureOverlay(t *testing.T) (overlayRoot, registryPath, headSHA string) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping git-fixture test in -short mode")
	}
	overlayRoot, registryPath = fixtureOverlay(t)
	runGit(t, overlayRoot, "init", "-q")
	runGit(t, overlayRoot, "add", "-A")
	runGit(t, overlayRoot, "commit", "-q", "-m", "initial")
	headSHA = strings.TrimSpace(runGit(t, overlayRoot, "rev-parse", "HEAD"))
	return overlayRoot, registryPath, headSHA
}

// runGit runs a git subcommand in dir with a deterministic committer
// identity, returning combined stdout+stderr and failing the test on error.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// TestBuiltFrom_RecordedAtBuild (R-005, task 2.1): the built package.json's
// labdrian.builtFrom must equal the overlay root's resolved HEAD commit.
func TestBuiltFrom_RecordedAtBuild(t *testing.T) {
	overlayRoot, registryPath, headSHA := gitFixtureOverlay(t)
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")

	if err := pipkg.Build(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("Build: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(destDir, "package.json"))
	if err != nil {
		t.Fatalf("read package.json: %v", err)
	}
	var manifest struct {
		Labdrian struct {
			BuiltFrom string `json:"builtFrom"`
		} `json:"labdrian"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("unmarshal package.json: %v", err)
	}
	if manifest.Labdrian.BuiltFrom != headSHA {
		t.Errorf("labdrian.builtFrom = %q, want %q", manifest.Labdrian.BuiltFrom, headSHA)
	}
}

// TestCheck_RefBasis (R-006/R3-001, task 2.2): Check always compares
// against the deploy ref (main), never the recorded builtFrom commit, so
// an uncommitted working-tree edit to a source file does not cause false
// drift, while a genuine divergence between the deployed content and the
// deploy ref's tree is still reported.
func TestCheck_RefBasis(t *testing.T) {
	overlayRoot, registryPath, headSHA := gitFixtureOverlay(t)
	runGit(t, overlayRoot, "branch", "-M", "main")
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")

	if err := pipkg.Build(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("Build: %v", err)
	}

	report, err := pipkg.Check(overlayRoot, registryPath, destDir)
	if err != nil {
		t.Fatalf("Check right after Build must report no drift, got: %v", err)
	}
	if report.Basis != "deploy" || report.DeployRef != "main" || report.BuiltFrom != headSHA || report.Stale {
		t.Errorf("report = %+v, want Basis=deploy DeployRef=main BuiltFrom=%s Stale=false", report, headSHA)
	}

	// Uncommitted working-tree edit: comparing against the committed
	// deploy ref must NOT report drift caused by an uncommitted edit.
	writeFile(t, filepath.Join(overlayRoot, "skills", "pi-skill", "SKILL.md"), "---\nname: pi-skill\n---\nuncommitted edit\n")
	report, err = pipkg.Check(overlayRoot, registryPath, destDir)
	if err != nil {
		t.Errorf("Check must ignore an uncommitted source edit, got: %v", err)
	}
	if report.Basis != "deploy" {
		t.Errorf("report.Basis = %q, want deploy", report.Basis)
	}

	// Discard the uncommitted edit and tamper with the DEPLOYED file
	// directly (destDir, not overlayRoot) so it no longer matches what
	// main's own export produces.
	runGit(t, overlayRoot, "checkout", "--", "skills/pi-skill/SKILL.md")
	deployed := filepath.Join(destDir, "skills", "pi-skill", "SKILL.md")
	if err := os.WriteFile(deployed, []byte("tampered\n"), 0644); err != nil {
		t.Fatalf("tamper with deployed file: %v", err)
	}
	_, err = pipkg.Check(overlayRoot, registryPath, destDir)
	if err == nil {
		t.Fatal("Check must report drift once the deployed package diverges from main's export")
	}
	if !strings.Contains(err.Error(), "pi-skill") {
		t.Errorf("drift error should name the drifted entry, got: %v", err)
	}
}

// TestCheck_StaleAfterCommittedSourceChange (R3-001): a committed source
// change on main after the last Build must be reported as drift, naming
// the stale build -- comparing against the recorded builtFrom commit
// instead of main would hide it entirely.
func TestCheck_StaleAfterCommittedSourceChange(t *testing.T) {
	overlayRoot, registryPath, headSHA := gitFixtureOverlay(t)
	runGit(t, overlayRoot, "branch", "-M", "main")
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")

	if err := pipkg.Build(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("Build: %v", err)
	}

	writeFile(t, filepath.Join(overlayRoot, "skills", "pi-skill", "SKILL.md"), "---\nname: pi-skill\n---\ncommitted edit\n")
	runGit(t, overlayRoot, "add", "-A")
	runGit(t, overlayRoot, "commit", "-q", "-m", "edit pi-skill")

	report, err := pipkg.Check(overlayRoot, registryPath, destDir)
	if err == nil {
		t.Fatal("Check must report drift for a stale deployed package")
	}
	if !strings.Contains(err.Error(), "rebuild with apply --target pi") || !strings.Contains(err.Error(), headSHA) {
		t.Errorf("drift error should name the stale build (builtFrom %s), got: %v", headSHA, err)
	}
	if !report.Stale || report.DeployRef != "main" || report.BuiltFrom != headSHA {
		t.Errorf("report = %+v, want Stale=true DeployRef=main BuiltFrom=%s", report, headSHA)
	}
}

// TestCheck_FeatureBranchDoesNotFalseDrift (R3-001/#315): a feature branch
// checked out with unrelated committed changes must not cause Check (which
// always compares against main) to report drift or staleness.
func TestCheck_FeatureBranchDoesNotFalseDrift(t *testing.T) {
	overlayRoot, registryPath, headSHA := gitFixtureOverlay(t)
	runGit(t, overlayRoot, "branch", "-M", "main")
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")

	if err := pipkg.Build(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("Build: %v", err)
	}

	runGit(t, overlayRoot, "checkout", "-q", "-b", "feature")
	writeFile(t, filepath.Join(overlayRoot, "skills", "pi-skill", "SKILL.md"), "---\nname: pi-skill\n---\nfeature-branch edit\n")
	runGit(t, overlayRoot, "add", "-A")
	runGit(t, overlayRoot, "commit", "-q", "-m", "unrelated feature-branch edit")

	report, err := pipkg.Check(overlayRoot, registryPath, destDir)
	if err != nil {
		t.Errorf("Check must still compare against main, not the checked-out feature branch, got: %v", err)
	}
	if report.Stale {
		t.Errorf("report.Stale = true, want false (builtFrom %s still equals main's tip)", headSHA)
	}
	if report.Basis != "deploy" || report.DeployRef != "main" {
		t.Errorf("report = %+v, want Basis=deploy DeployRef=main", report)
	}
	if !strings.Contains(report.Disclosure(), "main") {
		t.Errorf("Disclosure() = %q, want it to name main", report.Disclosure())
	}
}

// TestCheck_MainFallback (R-007, task 2.3): an unresolvable builtFrom SHA
// is still compared against the deploy ref (main), disclosed as
// provenance rather than as the comparison basis.
func TestCheck_MainFallback(t *testing.T) {
	overlayRoot, registryPath, _ := gitFixtureOverlay(t)
	runGit(t, overlayRoot, "branch", "-M", "main")
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")

	if err := pipkg.Build(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Corrupt the recorded builtFrom to a well-formed but nonexistent SHA.
	unresolvable := strings.Repeat("f", 40)
	corruptBuiltFrom(t, destDir, unresolvable)

	report, err := pipkg.Check(overlayRoot, registryPath, destDir)
	if err != nil {
		t.Errorf("Check must still succeed (no real drift) comparing against main, got: %v", err)
	}
	if report.Basis != "deploy" || report.DeployRef != "main" {
		t.Errorf("report = %+v, want Basis=deploy DeployRef=main", report)
	}
	if report.BuiltFrom != unresolvable {
		t.Errorf("report.BuiltFrom = %q, want the unresolvable builtFrom %q disclosed", report.BuiltFrom, unresolvable)
	}
	if report.Stale {
		t.Error("report.Stale = true, want false: an unresolvable builtFrom cannot be proven stale")
	}
}

// TestCheck_NonGitRoot (task 2.3): an overlay root that is not a git repo
// at all reports Basis="worktree".
func TestCheck_NonGitRoot(t *testing.T) {
	overlayRoot, registryPath := fixtureOverlay(t)
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")

	if err := pipkg.Build(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("Build: %v", err)
	}

	report, err := pipkg.Check(overlayRoot, registryPath, destDir)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if report.Basis != "worktree" {
		t.Errorf("report.Basis = %q, want worktree", report.Basis)
	}
}

// TestCheck_RejectsNonHexBuiltFrom (R-007 threat matrix, task 2.4): a
// non-hex builtFrom (or one crafted to look like a git option) must never
// be passed to git as an argument -- Check still compares against main
// cleanly instead of spawning git with attacker-controlled argv, and never
// claims staleness for a value it could not resolve.
func TestCheck_RejectsNonHexBuiltFrom(t *testing.T) {
	overlayRoot, registryPath, _ := gitFixtureOverlay(t)
	runGit(t, overlayRoot, "branch", "-M", "main")
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")

	if err := pipkg.Build(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("Build: %v", err)
	}

	for _, malicious := range []string{"--upload-pack=evil", "not-hex-at-all", ""} {
		corruptBuiltFrom(t, destDir, malicious)
		report, err := pipkg.Check(overlayRoot, registryPath, destDir)
		if err != nil {
			t.Errorf("Check(builtFrom=%q) must compare against main cleanly, got: %v", malicious, err)
		}
		if report.Basis != "deploy" || report.DeployRef != "main" {
			t.Errorf("Check(builtFrom=%q) report = %+v, want Basis=deploy DeployRef=main (non-hex value must never reach git)", malicious, report)
		}
		if report.Stale {
			t.Errorf("Check(builtFrom=%q).Stale = true, want false", malicious)
		}
	}
}

// corruptBuiltFrom overwrites destDir/package.json's labdrian.builtFrom
// field with an arbitrary string, without going through Build, so tests
// can simulate an unresolvable or malicious recorded ref. It re-marshals
// only the top-level object as raw fields (never re-parsing nested
// objects like "pi"), so it never reorders keys Check's own
// stripBuiltFrom normalization would not otherwise reorder.
func corruptBuiltFrom(t *testing.T, destDir, builtFrom string) {
	t.Helper()
	path := filepath.Join(destDir, "package.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read package.json: %v", err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal package.json: %v", err)
	}
	labdrian, err := json.Marshal(map[string]string{"builtFrom": builtFrom})
	if err != nil {
		t.Fatalf("marshal labdrian field: %v", err)
	}
	doc["labdrian"] = labdrian
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal package.json: %v", err)
	}
	if err := os.WriteFile(path, out, 0644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}
}

// TestBuiltFrom_DirtyTreeNotRecorded (R3-001): Build must not record
// builtFrom as the committed HEAD when the source tree is dirty --
// otherwise Check's ref-basis regeneration would read the stale committed
// content and misreport the uncommitted edit Build actually shipped as
// drift.
func TestBuiltFrom_DirtyTreeNotRecorded(t *testing.T) {
	overlayRoot, registryPath, _ := gitFixtureOverlay(t)
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")

	// Edit a registered skill WITHOUT committing.
	writeFile(t, filepath.Join(overlayRoot, "skills", "pi-skill", "SKILL.md"), "---\nname: pi-skill\n---\nuncommitted edit\n")

	if err := pipkg.Build(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("Build: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(destDir, "package.json"))
	if err != nil {
		t.Fatalf("read package.json: %v", err)
	}
	var manifest struct {
		Labdrian *struct {
			BuiltFrom string `json:"builtFrom"`
		} `json:"labdrian"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("unmarshal package.json: %v", err)
	}
	if manifest.Labdrian != nil && manifest.Labdrian.BuiltFrom != "" {
		t.Errorf("labdrian.builtFrom = %q, want absent for a dirty source tree", manifest.Labdrian.BuiltFrom)
	}

	report, err := pipkg.Check(overlayRoot, registryPath, destDir)
	if err != nil {
		t.Errorf("Check must report no drift right after a dirty-tree Build, got: %v", err)
	}
	if report.Basis != "dirty" {
		t.Errorf("report.Basis = %q, want dirty for an unrecorded builtFrom from a dirty source tree", report.Basis)
	}
}

// TestExportGitTree_DrainsPipeOnExtractionError (R3-002): an extraction
// error (a symlink entry) partway through a `git archive` tar stream must
// not leave the remainder of stdout unread -- git blocks writing to a full
// (~64KB) pipe, so a caller that fails to drain it before cmd.Wait() hangs
// forever. Export must return the extraction error promptly instead.
func TestExportGitTree_DrainsPipeOnExtractionError(t *testing.T) {
	overlayRoot, registryPath, _ := gitFixtureOverlay(t)
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")
	if err := pipkg.Build(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("Build: %v", err)
	}

	// A symlink (sorts before the large file, so extractTar hits the
	// refusal before the large entry is read off the stream) plus a
	// >64KB file, so the unread tar remainder exceeds a pipe's kernel
	// buffer.
	if err := os.Symlink("other-skill", filepath.Join(overlayRoot, "skills", "a-symlink")); err != nil {
		t.Fatalf("create symlink fixture: %v", err)
	}
	writeFile(t, filepath.Join(overlayRoot, "skills", "z-large.bin"), strings.Repeat("x", 256*1024))
	runGit(t, overlayRoot, "add", "-A")
	runGit(t, overlayRoot, "commit", "-q", "-m", "add symlink and large file")
	badSHA := strings.TrimSpace(runGit(t, overlayRoot, "rev-parse", "HEAD"))
	corruptBuiltFrom(t, destDir, badSHA)

	done := make(chan error, 1)
	go func() {
		_, err := pipkg.Check(overlayRoot, registryPath, destDir)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Check must report an error for a symlink found in the exported git tree")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Check hung: git archive's stdout pipe was not drained after an extraction error (R3-002)")
	}
}

// TestCheck_MainFallbackWithoutMainBranch: CI checks a pull request out as
// a detached HEAD with no local "main" branch. When builtFrom is not
// resolvable the fallback must still compare against something that
// exists (origin/main, then HEAD) and disclose which, instead of failing
// with "not a valid object name: main".
func TestCheck_MainFallbackWithoutMainBranch(t *testing.T) {
	overlayRoot, registryPath, _ := gitFixtureOverlay(t)
	runGit(t, overlayRoot, "checkout", "-q", "-b", "trunk")
	if strings.TrimSpace(runGit(t, overlayRoot, "branch", "--list", "main")) != "" {
		runGit(t, overlayRoot, "branch", "-D", "main")
	}
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")
	if err := pipkg.Build(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("Build: %v", err)
	}
	corruptBuiltFrom(t, destDir, "0000000000000000000000000000000000000000")

	report, err := pipkg.Check(overlayRoot, registryPath, destDir)
	if err != nil {
		t.Fatalf("Check without a local main branch must fall back, got: %v", err)
	}
	if report.Basis != "deploy" || report.DeployRef != "HEAD" || !strings.Contains(report.Disclosure(), "HEAD") {
		t.Fatalf("expected DeployRef=HEAD disclosing the fallback, got basis=%q deployRef=%q disclosure=%q", report.Basis, report.DeployRef, report.Disclosure())
	}
}

// TestCheck_DeployRefOverride: LABDRIAN_PI_DEPLOY_REF names the deploy ref
// for checkouts that do not deploy from main (CI on a pull request, a
// feature-branch shelltest); the disclosure still names the ref used.
func TestCheck_DeployRefOverride(t *testing.T) {
	overlayRoot, registryPath, _ := gitFixtureOverlay(t)
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")
	runGit(t, overlayRoot, "checkout", "-q", "-b", "feature")
	if err := pipkg.Build(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("Build: %v", err)
	}
	writeFile(t, filepath.Join(overlayRoot, "skills", "pi-skill", "SKILL.md"), "---\nname: pi-skill\n---\nfeature edit\n")
	runGit(t, overlayRoot, "add", "-A")
	runGit(t, overlayRoot, "commit", "-q", "-m", "feature edit")
	t.Setenv("LABDRIAN_PI_DEPLOY_REF", "feature")

	report, err := pipkg.Check(overlayRoot, registryPath, destDir)
	if err == nil || !strings.Contains(err.Error(), "SKILL.md") {
		t.Fatalf("Check with the override must compare against feature and report the edit as drift, got err=%v", err)
	}
	if report.DeployRef != "feature" || !strings.Contains(report.Disclosure(), "feature") {
		t.Fatalf("expected DeployRef=feature disclosed, got %q / %q", report.DeployRef, report.Disclosure())
	}
}
