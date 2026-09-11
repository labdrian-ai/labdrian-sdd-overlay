package pipkg_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

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

// TestCheck_RefBasis (R-006, task 2.2): a resolvable builtFrom SHA makes
// Check compare against that ref's git tree, so an uncommitted working-tree
// edit to a source file does not cause false drift, while a genuine
// committed source change since builtFrom still reports drift.
func TestCheck_RefBasis(t *testing.T) {
	overlayRoot, registryPath, headSHA := gitFixtureOverlay(t)
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")

	if err := pipkg.Build(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("Build: %v", err)
	}

	report, err := pipkg.Check(overlayRoot, registryPath, destDir)
	if err != nil {
		t.Fatalf("Check right after Build must report no drift, got: %v", err)
	}
	if report.Basis != "ref" || report.Ref != headSHA {
		t.Errorf("report = %+v, want Basis=ref Ref=%s", report, headSHA)
	}

	// Uncommitted working-tree edit: builtFrom (committed HEAD) is still
	// resolvable and unchanged, so comparing against that ref must NOT
	// report drift caused by the uncommitted edit.
	writeFile(t, filepath.Join(overlayRoot, "skills", "pi-skill", "SKILL.md"), "---\nname: pi-skill\n---\nuncommitted edit\n")
	report, err = pipkg.Check(overlayRoot, registryPath, destDir)
	if err != nil {
		t.Errorf("Check must ignore an uncommitted source edit when builtFrom is resolvable, got: %v", err)
	}
	if report.Basis != "ref" {
		t.Errorf("report.Basis = %q, want ref", report.Basis)
	}

	// Commit that edit: the built package now genuinely diverges from the
	// committed tree at the NEW HEAD, but builtFrom still names the OLD
	// commit, so Check must still compare against builtFrom (still ref,
	// still no drift) rather than the new HEAD.
	runGit(t, overlayRoot, "add", "-A")
	runGit(t, overlayRoot, "commit", "-q", "-m", "edit pi-skill")
	report, err = pipkg.Check(overlayRoot, registryPath, destDir)
	if err != nil {
		t.Errorf("Check must still compare against the recorded builtFrom ref, got: %v", err)
	}
	if report.Basis != "ref" || report.Ref != headSHA {
		t.Errorf("report = %+v, want Basis=ref Ref=%s (unchanged builtFrom)", report, headSHA)
	}

	// Real divergence from the recorded builtFrom ref is still reported:
	// tamper with the DEPLOYED file directly (destDir, not overlayRoot) so
	// it no longer matches what builtFrom's own export produces.
	deployed := filepath.Join(destDir, "skills", "pi-skill", "SKILL.md")
	if err := os.WriteFile(deployed, []byte("tampered\n"), 0644); err != nil {
		t.Fatalf("tamper with deployed file: %v", err)
	}
	_, err = pipkg.Check(overlayRoot, registryPath, destDir)
	if err == nil {
		t.Fatal("Check must report drift once the deployed package diverges from its builtFrom ref's export")
	}
	if !strings.Contains(err.Error(), "pi-skill") {
		t.Errorf("drift error should name the drifted entry, got: %v", err)
	}
}

// TestCheck_MainFallback (R-007, task 2.3): an unresolvable builtFrom SHA
// falls back to comparing against main, with Basis="main" disclosed.
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
		t.Errorf("Check must still succeed (no real drift) after falling back to main, got: %v", err)
	}
	if report.Basis != "main" {
		t.Errorf("report.Basis = %q, want main", report.Basis)
	}
	if report.Ref != unresolvable {
		t.Errorf("report.Ref = %q, want the unresolvable builtFrom %q disclosed", report.Ref, unresolvable)
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
// be passed to git as an argument -- Check falls straight back to the main
// basis instead of spawning git with attacker-controlled argv.
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
			t.Errorf("Check(builtFrom=%q) must fall back to main cleanly, got: %v", malicious, err)
		}
		if report.Basis != "main" {
			t.Errorf("Check(builtFrom=%q).Basis = %q, want main (non-hex value must never reach git)", malicious, report.Basis)
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
