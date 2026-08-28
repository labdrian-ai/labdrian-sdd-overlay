package main

// Integration harness for `bin/labdrian-overlay`'s release-identity, digest,
// and state helpers (overlay-release-identity R-001..R-003, overlay-release-state
// R-001..R-002). This is a Go-to-bash test, following the exact pattern
// established by selfupdate_backend_test.go: it spawns the REAL backend
// script against hermetic scratch git repos, reusing that file's shared
// helpers (gitTestEnv, runGit, writeFile, realBackendBin) since both live in
// package main.
//
// Unlike self-update (a full subcommand), the functions under test here
// (resolve_latest_release_tag, compute_target_digest, state_read_target,
// state_write_target) have no CLI subcommand wired to them yet in this
// slice. runBackendFunc below sources the real script (its CLI dispatch is
// guarded behind a `[[ "${BASH_SOURCE[0]}" == "${0}" ]]` check specifically
// so it can be sourced like this) and calls one function directly -- the
// "Go-driven bash" layer named in design.md's Testing Strategy table.

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// harness additions
// ---------------------------------------------------------------------------

// pushUpstreamTag creates an ANNOTATED tag on origin's current main HEAD via
// an ephemeral throwaway clone (mirroring pushUpstreamCommit's shape), so
// the caller's own scratch clone never fetches it until the function under
// test explicitly does so (R-003's "explicit fetch" requirement).
func pushUpstreamTag(t *testing.T, origin, tag, message string) {
	t.Helper()
	base := t.TempDir()
	pub := filepath.Join(base, "publisher-tag")
	runGit(t, base, "clone", origin, pub)
	runGit(t, pub, "tag", "-a", tag, "-m", message)
	runGit(t, pub, "push", "origin", tag)
}

// runBackendFunc sources the real bin/labdrian-overlay script and calls a
// single named function directly, rather than exercising it only indirectly
// through a full subcommand. HOME and OVERLAY_DIR are pointed at the given
// scratch directories, exactly like runBackendSubcommand.
func runBackendFunc(t *testing.T, overlayDir, home, funcName string, args ...string) (string, int) {
	t.Helper()
	bin := realBackendBin(t)
	script := `source "$1"; shift; "$@"`
	cmdArgs := append([]string{"-c", script, "bash", bin, funcName}, args...)
	cmd := exec.Command("bash", cmdArgs...)
	cmd.Dir = overlayDir
	cmd.Env = append(gitTestEnv(), "OVERLAY_DIR="+overlayDir, "HOME="+home)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), 0
	}
	ee, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("running backend func %s %s: %v\n%s", funcName, strings.Join(args, " "), err, out)
	}
	return string(out), ee.ExitCode()
}

// runBackendSubcommandWithHome is runBackendSubcommand, but lets the caller
// inspect the HOME directory afterward (runBackendSubcommand allocates and
// discards its own). Needed by the apply/state integration test below,
// which must read $HOME/.labdrian-overlay/state.json once the command exits.
func runBackendSubcommandWithHome(t *testing.T, overlayDir, home string, args ...string) (string, int) {
	t.Helper()
	bin := realBackendBin(t)
	cmd := exec.Command(bin, args...)
	cmd.Dir = overlayDir
	cmd.Env = append(gitTestEnv(), "OVERLAY_DIR="+overlayDir, "HOME="+home)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), 0
	}
	ee, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("running %s %s: %v\n%s", bin, strings.Join(args, " "), err, out)
	}
	return string(out), ee.ExitCode()
}

func sha256Hex(t *testing.T, s string) string {
	t.Helper()
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// writeDigestFixtureFiles seeds a minimal two-file overlay layout: repo
// source under $overlayDir/skills/{alpha,beta}/SKILL.md, deployed live
// copies under $home/.claude/skills/{alpha,beta}/SKILL.md. Callers still
// need to write their own overlay.manifest (row order varies per test).
func writeDigestFixtureFiles(t *testing.T, overlayDir, home string) {
	t.Helper()
	for _, name := range []string{"alpha", "beta"} {
		if err := os.MkdirAll(filepath.Join(overlayDir, "skills", name), 0o755); err != nil {
			t.Fatalf("mkdir overlay skills/%s: %v", name, err)
		}
		if err := os.MkdirAll(filepath.Join(home, ".claude", "skills", name), 0o755); err != nil {
			t.Fatalf("mkdir home skills/%s: %v", name, err)
		}
	}
	writeFile(t, filepath.Join(overlayDir, "skills", "alpha", "SKILL.md"), "alpha content\n")
	writeFile(t, filepath.Join(overlayDir, "skills", "beta", "SKILL.md"), "beta content\n")
	writeFile(t, filepath.Join(home, ".claude", "skills", "alpha", "SKILL.md"), "alpha content\n")
	writeFile(t, filepath.Join(home, ".claude", "skills", "beta", "SKILL.md"), "beta content\n")
}

// ---------------------------------------------------------------------------
// task 1.1: resolve_latest_release_tag (R-003)
// ---------------------------------------------------------------------------

func TestReleaseBackend_ResolveLatestTag_NoTagsReturnsNone(t *testing.T) {
	_, clone := newScratchRepo(t)
	fakeHome := t.TempDir()

	out, code := runBackendFunc(t, clone, fakeHome, "resolve_latest_release_tag", "main")
	if code != 0 {
		t.Fatalf("resolve_latest_release_tag exit=%d, want 0\noutput:\n%s", code, out)
	}
	if got := strings.TrimSpace(out); got != "none" {
		t.Errorf("resolve_latest_release_tag with zero tags = %q, want none", got)
	}
}

// Deliberately out of lexical order: a plain string sort would prefer
// "v1.2.0" or "v1.9.0" over "v1.10.0". The clone here never fetches tags
// itself, so a correct result also proves the explicit fetch (R-003) ran
// before resolution.
func TestReleaseBackend_ResolveLatestTag_SemverOrderNotLexicalAndFetchesExplicitly(t *testing.T) {
	origin, clone := newScratchRepo(t)
	pushUpstreamTag(t, origin, "v1.2.0", "release 1.2.0")
	pushUpstreamTag(t, origin, "v1.10.0", "release 1.10.0")
	pushUpstreamTag(t, origin, "v1.9.0", "release 1.9.0")

	fakeHome := t.TempDir()
	out, code := runBackendFunc(t, clone, fakeHome, "resolve_latest_release_tag", "main")
	if code != 0 {
		t.Fatalf("resolve_latest_release_tag exit=%d, want 0\noutput:\n%s", code, out)
	}
	if got := strings.TrimSpace(out); got != "v1.10.0" {
		t.Errorf("resolve_latest_release_tag = %q, want v1.10.0 (semver order, not lexical; this clone never fetched tags itself, so this also proves the explicit fetch ran)", got)
	}
}

// R-001 requires an ANNOTATED tag. A lightweight tag with a higher version
// must be skipped.
func TestReleaseBackend_ResolveLatestTag_SkipsLightweightTag(t *testing.T) {
	origin, clone := newScratchRepo(t)
	pushUpstreamTag(t, origin, "v1.0.0", "release 1.0.0")

	base := t.TempDir()
	pub := filepath.Join(base, "publisher-lightweight")
	runGit(t, base, "clone", origin, pub)
	runGit(t, pub, "tag", "v2.0.0") // lightweight: no -a/-m
	runGit(t, pub, "push", "origin", "v2.0.0")

	fakeHome := t.TempDir()
	out, code := runBackendFunc(t, clone, fakeHome, "resolve_latest_release_tag", "main")
	if code != 0 {
		t.Fatalf("resolve_latest_release_tag exit=%d, want 0\noutput:\n%s", code, out)
	}
	if got := strings.TrimSpace(out); got != "v1.0.0" {
		t.Errorf("resolve_latest_release_tag = %q, want v1.0.0 (lightweight v2.0.0 must be skipped)", got)
	}
}

// R-003: "the highest ... tag reachable from the given ref". A higher
// version tag on a branch never merged into main must not be selected.
func TestReleaseBackend_ResolveLatestTag_OnlyReachableFromGivenRef(t *testing.T) {
	origin, clone := newScratchRepo(t)
	pushUpstreamTag(t, origin, "v1.0.0", "release 1.0.0")

	base := t.TempDir()
	pub := filepath.Join(base, "publisher-side")
	runGit(t, base, "clone", origin, pub)
	runGit(t, pub, "checkout", "-b", "side-branch")
	writeFile(t, filepath.Join(pub, "side.txt"), "side content\n")
	runGit(t, pub, "add", "side.txt")
	runGit(t, pub, "commit", "-m", "side-only commit")
	runGit(t, pub, "tag", "-a", "v9.9.9", "-m", "unreachable release")
	runGit(t, pub, "push", "origin", "side-branch")
	runGit(t, pub, "push", "origin", "v9.9.9")

	fakeHome := t.TempDir()
	out, code := runBackendFunc(t, clone, fakeHome, "resolve_latest_release_tag", "main")
	if code != 0 {
		t.Fatalf("resolve_latest_release_tag exit=%d, want 0\noutput:\n%s", code, out)
	}
	if got := strings.TrimSpace(out); got != "v1.0.0" {
		t.Errorf("resolve_latest_release_tag(main) = %q, want v1.0.0 (v9.9.9 lives on an unmerged branch)", got)
	}
}

// ---------------------------------------------------------------------------
// task 1.2: compute_target_digest (design decision D5)
// ---------------------------------------------------------------------------

func TestReleaseBackend_ComputeTargetDigest_MatchesManualAlgorithm(t *testing.T) {
	overlayDir := t.TempDir()
	homeDir := t.TempDir()
	writeDigestFixtureFiles(t, overlayDir, homeDir)
	writeFile(t, filepath.Join(overlayDir, "overlay.manifest"), "alpha/SKILL.md managed\nbeta/SKILL.md managed\n")

	out, code := runBackendFunc(t, overlayDir, homeDir, "compute_target_digest", "claude", "live")
	if code != 0 {
		t.Fatalf("compute_target_digest exit=%d, want 0\noutput:\n%s", code, out)
	}
	got := strings.TrimSpace(out)

	lines := []string{
		"skills/alpha/SKILL.md:" + sha256Hex(t, "alpha content\n"),
		"skills/beta/SKILL.md:" + sha256Hex(t, "beta content\n"),
	}
	sort.Strings(lines)
	want := sha256Hex(t, strings.Join(lines, "\n")+"\n")

	if got != want {
		t.Errorf("compute_target_digest = %q, want %q (manually computed sorted-lines sha256)", got, want)
	}
}

func TestReleaseBackend_ComputeTargetDigest_ManifestReorderIsIdentical(t *testing.T) {
	overlayDir := t.TempDir()
	homeDir := t.TempDir()
	writeDigestFixtureFiles(t, overlayDir, homeDir)

	manifestPath := filepath.Join(overlayDir, "overlay.manifest")
	writeFile(t, manifestPath, "alpha/SKILL.md managed\nbeta/SKILL.md managed\n")
	out1, code1 := runBackendFunc(t, overlayDir, homeDir, "compute_target_digest", "claude", "live")
	if code1 != 0 {
		t.Fatalf("compute_target_digest exit=%d, want 0\noutput:\n%s", code1, out1)
	}

	writeFile(t, manifestPath, "beta/SKILL.md managed\nalpha/SKILL.md managed\n")
	out2, code2 := runBackendFunc(t, overlayDir, homeDir, "compute_target_digest", "claude", "live")
	if code2 != 0 {
		t.Fatalf("compute_target_digest exit=%d, want 0\noutput:\n%s", code2, out2)
	}

	if strings.TrimSpace(out1) != strings.TrimSpace(out2) {
		t.Errorf("digest changed after reordering manifest rows with identical content: %q vs %q", out1, out2)
	}
}

func TestReleaseBackend_ComputeTargetDigest_MutationChangesDigest(t *testing.T) {
	overlayDir := t.TempDir()
	homeDir := t.TempDir()
	writeDigestFixtureFiles(t, overlayDir, homeDir)
	writeFile(t, filepath.Join(overlayDir, "overlay.manifest"), "alpha/SKILL.md managed\nbeta/SKILL.md managed\n")

	before, codeBefore := runBackendFunc(t, overlayDir, homeDir, "compute_target_digest", "claude", "live")
	if codeBefore != 0 {
		t.Fatalf("compute_target_digest exit=%d, want 0\noutput:\n%s", codeBefore, before)
	}

	writeFile(t, filepath.Join(homeDir, ".claude", "skills", "beta", "SKILL.md"), "beta content MUTATED\n")

	after, codeAfter := runBackendFunc(t, overlayDir, homeDir, "compute_target_digest", "claude", "live")
	if codeAfter != 0 {
		t.Fatalf("compute_target_digest exit=%d, want 0\noutput:\n%s", codeAfter, after)
	}

	if strings.TrimSpace(before) == strings.TrimSpace(after) {
		t.Errorf("digest did not change after a single-file out-of-band mutation")
	}
}

// "ref" mode must hash the repo-source file, not the deployed live copy --
// diverging the live copy from the repo source must not move the "ref"
// digest.
func TestReleaseBackend_ComputeTargetDigest_RefModeReadsRepoSourceNotLiveFile(t *testing.T) {
	overlayDir := t.TempDir()
	homeDir := t.TempDir()
	writeDigestFixtureFiles(t, overlayDir, homeDir)
	writeFile(t, filepath.Join(overlayDir, "overlay.manifest"), "alpha/SKILL.md managed\nbeta/SKILL.md managed\n")

	writeFile(t, filepath.Join(homeDir, ".claude", "skills", "beta", "SKILL.md"), "beta content DRIFTED LIVE\n")

	refOut, refCode := runBackendFunc(t, overlayDir, homeDir, "compute_target_digest", "claude", "ref")
	if refCode != 0 {
		t.Fatalf("compute_target_digest exit=%d, want 0\noutput:\n%s", refCode, refOut)
	}

	lines := []string{
		"skills/alpha/SKILL.md:" + sha256Hex(t, "alpha content\n"),
		"skills/beta/SKILL.md:" + sha256Hex(t, "beta content\n"),
	}
	sort.Strings(lines)
	want := sha256Hex(t, strings.Join(lines, "\n")+"\n")

	if got := strings.TrimSpace(refOut); got != want {
		t.Errorf("ref-mode digest = %q, want %q (should reflect repo source, not the drifted live file)", got, want)
	}

	liveOut, liveCode := runBackendFunc(t, overlayDir, homeDir, "compute_target_digest", "claude", "live")
	if liveCode != 0 {
		t.Fatalf("compute_target_digest exit=%d, want 0\noutput:\n%s", liveCode, liveOut)
	}
	if strings.TrimSpace(liveOut) == strings.TrimSpace(refOut) {
		t.Errorf("live and ref digests unexpectedly match despite the live/repo-source drift")
	}
}

func TestReleaseBackend_ComputeTargetDigest_MissingLiveFileYieldsMissingToken(t *testing.T) {
	overlayDir := t.TempDir()
	homeDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(overlayDir, "skills", "alpha"), 0o755); err != nil {
		t.Fatalf("mkdir skills/alpha: %v", err)
	}
	writeFile(t, filepath.Join(overlayDir, "skills", "alpha", "SKILL.md"), "alpha content\n")
	writeFile(t, filepath.Join(overlayDir, "overlay.manifest"), "alpha/SKILL.md managed\n")
	// Deliberately NOT deployed to $HOME/.claude/skills/alpha/SKILL.md --
	// a never-applied target.

	out, code := runBackendFunc(t, overlayDir, homeDir, "compute_target_digest", "claude", "live")
	if code != 0 {
		t.Fatalf("compute_target_digest exit=%d, want 0 for a not-yet-deployed target\noutput:\n%s", code, out)
	}

	want := sha256Hex(t, "skills/alpha/SKILL.md:MISSING\n")
	if got := strings.TrimSpace(out); got != want {
		t.Errorf("digest for a missing live file = %q, want %q (MISSING token)", got, want)
	}
}

// ---------------------------------------------------------------------------
// task 1.3: state_read_target / state_write_target (design decision D8)
// ---------------------------------------------------------------------------

func TestReleaseBackend_StateReadTarget_NeverDeployedWhenMissing(t *testing.T) {
	overlayDir := t.TempDir()
	homeDir := t.TempDir()

	out, code := runBackendFunc(t, overlayDir, homeDir, "state_read_target", "claude")
	if code != 0 {
		t.Fatalf("state_read_target exit=%d, want 0\noutput:\n%s", code, out)
	}
	if got := strings.TrimSpace(out); got != "NEVER_DEPLOYED" {
		t.Errorf("state_read_target on a missing state file = %q, want NEVER_DEPLOYED", got)
	}
}

func TestReleaseBackend_StateWriteTarget_FirstApplyCreatesStateFile(t *testing.T) {
	overlayDir := t.TempDir()
	homeDir := t.TempDir()

	statePath := filepath.Join(homeDir, ".labdrian-overlay", "state.json")
	if _, err := os.Stat(statePath); err == nil {
		t.Fatalf("state.json unexpectedly pre-exists")
	}

	out, code := runBackendFunc(t, overlayDir, homeDir, "state_write_target", "claude", "v1.0.0", "abc123")
	if code != 0 {
		t.Fatalf("state_write_target exit=%d, want 0\noutput:\n%s", code, out)
	}

	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("state.json was not created: %v", err)
	}
	if !strings.Contains(string(data), `"schema": 1`) {
		t.Errorf("state.json missing schema field:\n%s", data)
	}

	readOut, readCode := runBackendFunc(t, overlayDir, homeDir, "state_read_target", "claude")
	if readCode != 0 {
		t.Fatalf("state_read_target exit=%d, want 0\noutput:\n%s", readCode, readOut)
	}
	fields := strings.Split(strings.TrimSpace(readOut), "\t")
	if len(fields) != 3 || fields[0] != "v1.0.0" || fields[1] != "abc123" {
		t.Errorf("state_read_target after write = %q, want version=v1.0.0 digest=abc123", readOut)
	}
}

func TestReleaseBackend_StateWriteTarget_PreservesOtherTargetsOnUpdate(t *testing.T) {
	overlayDir := t.TempDir()
	homeDir := t.TempDir()

	if _, code := runBackendFunc(t, overlayDir, homeDir, "state_write_target", "claude", "v1.0.0", "hash-claude"); code != 0 {
		t.Fatalf("first state_write_target failed")
	}
	if _, code := runBackendFunc(t, overlayDir, homeDir, "state_write_target", "opencode", "v1.0.0", "hash-opencode"); code != 0 {
		t.Fatalf("second state_write_target failed")
	}

	out, code := runBackendFunc(t, overlayDir, homeDir, "state_read_target", "claude")
	if code != 0 {
		t.Fatalf("state_read_target exit=%d, want 0\noutput:\n%s", code, out)
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "v1.0.0\thash-claude\t") {
		t.Errorf("writing opencode's entry clobbered claude's prior entry: %q", out)
	}
}

func TestReleaseBackend_StateReadTarget_CorruptFileReportsNeverDeployedWithWarn(t *testing.T) {
	overlayDir := t.TempDir()
	homeDir := t.TempDir()

	stateDir := filepath.Join(homeDir, ".labdrian-overlay")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	writeFile(t, filepath.Join(stateDir, "state.json"), "{ not valid json at all")

	out, code := runBackendFunc(t, overlayDir, homeDir, "state_read_target", "claude")
	if code != 0 {
		t.Fatalf("state_read_target exit=%d, want 0 (corrupt state degrades, never crashes)\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "NEVER_DEPLOYED") {
		t.Errorf("corrupt state file did not report never-deployed: %q", out)
	}
	if !strings.Contains(out, "WARN") {
		t.Errorf("corrupt state file did not emit a WARN: %q", out)
	}
}

// ---------------------------------------------------------------------------
// task 1.4: cmd_apply wired to state_write_target
// ---------------------------------------------------------------------------

// The first-ever `apply` for a target creates state.json (R-001) recording
// "untagged" (D1: no release tag exists yet in this scratch repo) and a
// digest that matches an independent recomputation.
func TestReleaseBackend_ApplyRecordsVersionAndDigestForTarget(t *testing.T) {
	_, clone := newScratchRepoWithUpstream(t)

	runGit(t, clone, "checkout", "upstream")
	if err := os.MkdirAll(filepath.Join(clone, "skills", "hello"), 0o755); err != nil {
		t.Fatalf("mkdir skills/hello: %v", err)
	}
	writeFile(t, filepath.Join(clone, "overlay.manifest"), "hello/SKILL.md managed\n")
	writeFile(t, filepath.Join(clone, "skills", "hello", "SKILL.md"), "hello content\n")
	runGit(t, clone, "add", "overlay.manifest", "skills/hello/SKILL.md")
	runGit(t, clone, "commit", "-m", "add hello skill")
	runGit(t, clone, "push", "origin", "upstream")
	runGit(t, clone, "checkout", "feature-x")

	fakeHome := t.TempDir()
	out, code := runBackendSubcommandWithHome(t, clone, fakeHome, "apply", "--target", "claude")
	if code != 0 {
		t.Fatalf("apply exit=%d, want 0\noutput:\n%s", code, out)
	}

	readOut, readCode := runBackendFunc(t, clone, fakeHome, "state_read_target", "claude")
	if readCode != 0 {
		t.Fatalf("state_read_target exit=%d, want 0\noutput:\n%s", readCode, readOut)
	}
	fields := strings.Split(strings.TrimSpace(readOut), "\t")
	if len(fields) != 3 {
		t.Fatalf("state_read_target after apply = %q, want 3 tab-separated fields", readOut)
	}
	version, digest := fields[0], fields[1]
	if version != "untagged" {
		t.Errorf("recorded version = %q, want untagged (no release tag exists yet, D1 bootstrap)", version)
	}

	// apply restores the original branch (feature-x) on exit, which never
	// received the manifest/skills merge -- checkout main again so the
	// independent recomputation below reads the same overlay.manifest apply
	// itself worked from.
	runGit(t, clone, "checkout", "main")
	digestOut, digestCode := runBackendFunc(t, clone, fakeHome, "compute_target_digest", "claude", "live")
	if digestCode != 0 {
		t.Fatalf("compute_target_digest exit=%d, want 0\noutput:\n%s", digestCode, digestOut)
	}
	if got := strings.TrimSpace(digestOut); got != digest {
		t.Errorf("recorded digest %q does not match independently recomputed live digest %q", digest, got)
	}
}
