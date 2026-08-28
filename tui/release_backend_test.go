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

// ---------------------------------------------------------------------------
// task 2.1: cmd_self_update rewritten onto resolve_latest_release_tag (R-004)
// ---------------------------------------------------------------------------

// Converges main to the resolved release tag's commit, not to origin/main's
// raw HEAD, when origin carries untagged commits beyond the latest tag.
func TestReleaseBackend_SelfUpdate_ConvergesToTagNotRawOriginHead(t *testing.T) {
	origin, clone := newScratchRepo(t)
	pushUpstreamCommit(t, origin, "tagged.txt", "v2\n", "advance to be tagged")
	pushUpstreamTag(t, origin, "v1.5.0", "release 1.5.0")
	pushUpstreamCommit(t, origin, "untagged.txt", "v3\n", "untagged advance beyond the tag")

	featureHeadBefore := headRev(t, clone, "feature-x")

	out, code := runSelfUpdate(t, clone)
	if code != 0 {
		t.Fatalf("self-update exit=%d, want 0\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "v1.5.0") {
		t.Errorf("output does not name the resolved release v1.5.0:\n%s", out)
	}

	if got := currentBranch(t, clone); got != "feature-x" {
		t.Errorf("current branch = %q, want restored to feature-x", got)
	}
	featureHeadAfter := headRev(t, clone, "feature-x")
	if featureHeadAfter != featureHeadBefore {
		t.Errorf("feature-x HEAD changed (%s -> %s); only main should move", featureHeadBefore, featureHeadAfter)
	}

	mainHead := headRev(t, clone, "main")
	tagHead := headRev(t, clone, "v1.5.0^{commit}")
	originMainHead := headRev(t, clone, "origin/main")
	if mainHead != tagHead {
		t.Errorf("main HEAD = %s, want it to equal the resolved tag's commit = %s", mainHead, tagHead)
	}
	if mainHead == originMainHead {
		t.Errorf("main HEAD unexpectedly equals origin/main's raw HEAD (%s); origin carries untagged commits beyond the tag", originMainHead)
	}
}

// Local main already at the latest known release tag: exit 0, no checkout at
// all (same no-checkout discipline as the pre-existing zero-tag
// TestSelfUpdateBackend_UpToDateNoCheckout), reporting the version by name.
func TestReleaseBackend_SelfUpdate_AlreadyAtLatestTagNoOp(t *testing.T) {
	origin, clone := newScratchRepoOnBranch(t, "main")
	pushUpstreamTag(t, origin, "v1.0.0", "release 1.0.0")

	reflogBefore := runGit(t, clone, "reflog", "show", "--no-abbrev", "HEAD")

	out, code := runSelfUpdate(t, clone)
	if code != 0 {
		t.Fatalf("self-update exit=%d, want 0\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "already up to date with v1.0.0") {
		t.Errorf("output does not report already-up-to-date-with-v1.0.0:\n%s", out)
	}

	reflogAfter := runGit(t, clone, "reflog", "show", "--no-abbrev", "HEAD")
	if reflogBefore != reflogAfter {
		t.Errorf("HEAD reflog changed -- a checkout occurred when none was expected\nbefore:\n%s\nafter:\n%s", reflogBefore, reflogAfter)
	}
}

// A dirty tracked tree still blocks self-update BEFORE any tag resolution or
// checkout, even when a release tag exists -- regression guard proving the
// rewrite did not reorder tag resolution ahead of the pre-existing refusal.
func TestReleaseBackend_SelfUpdate_DirtyTreeBlocksUnderTagMode(t *testing.T) {
	origin, clone := newScratchRepo(t)
	pushUpstreamTag(t, origin, "v1.0.0", "release 1.0.0")

	readmePath := filepath.Join(clone, "README.md")
	writeFile(t, readmePath, "dirty tracked change\n")

	mainHeadBefore := headRev(t, clone, "main")

	out, code := runSelfUpdate(t, clone)
	if code == 0 {
		t.Fatalf("self-update exit=0, want nonzero for a dirty tracked tree under tag mode\noutput:\n%s", out)
	}
	if !strings.Contains(out, "uncommitted tracked changes") {
		t.Errorf("output does not name the dirty-tree refusal:\n%s", out)
	}
	if got := currentBranch(t, clone); got != "feature-x" {
		t.Errorf("current branch = %q, want unchanged feature-x", got)
	}
	if got := headRev(t, clone, "main"); got != mainHeadBefore {
		t.Errorf("main HEAD changed (%s -> %s), want untouched", mainHeadBefore, got)
	}
}

// D1 pre-first-tag bootstrap: zero tags anywhere falls back verbatim to the
// legacy origin/main convergence, printing the documented notice.
func TestReleaseBackend_SelfUpdate_ZeroTagFallbackConvergesLegacyWithNotice(t *testing.T) {
	origin, clone := newScratchRepo(t)
	pushUpstreamCommit(t, origin, "upstream.txt", "v2\n", "upstream advance")

	out, code := runSelfUpdate(t, clone)
	if code != 0 {
		t.Fatalf("self-update exit=%d, want 0\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "no release tags yet") {
		t.Errorf("output does not carry the D1 legacy-convergence notice:\n%s", out)
	}
	if !strings.Contains(out, "fast-forward") {
		t.Errorf("output does not mention a fast-forward:\n%s", out)
	}

	mainHead := headRev(t, clone, "main")
	originMainHead := headRev(t, clone, "origin/main")
	if mainHead != originMainHead {
		t.Errorf("main HEAD = %s, want it to equal origin/main HEAD = %s (legacy convergence)", mainHead, originMainHead)
	}
}

// ---------------------------------------------------------------------------
// task 2.2: compute_repo_behind_release (design decision D2)
// ---------------------------------------------------------------------------

func TestReleaseBackend_ComputeRepoBehindRelease_NAPreTag(t *testing.T) {
	_, clone := newScratchRepo(t)
	fakeHome := t.TempDir()

	out, code := runBackendFunc(t, clone, fakeHome, "compute_repo_behind_release", "false")
	if code != 0 {
		t.Fatalf("compute_repo_behind_release exit=%d, want 0\noutput:\n%s", code, out)
	}
	// runBackendFunc's CombinedOutput merges the stderr diagnostic
	// (SYNC_CHECK:(repo): ...) that precedes the stdout "NA" value, exactly
	// like compute_repo_behind_origin's own sibling pattern -- assert only
	// the final line, which is the actual machine-parseable value.
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if got := lines[len(lines)-1]; got != "NA" {
		t.Errorf("compute_repo_behind_release with zero tags = %q, want NA (full output: %q)", got, out)
	}
}

// After self-update has converged main to a known release tag, and origin
// gains further UNTAGGED commits afterward, the cached-only comparison must
// still report 0 -- REPO_BEHIND_RELEASE cares only about the newest KNOWN
// tag, never raw origin/main drift (that's REPO_BEHIND_ORIGIN's job).
func TestReleaseBackend_ComputeRepoBehindRelease_ZeroAfterSelfUpdateConvergesWithUntaggedOriginAhead(t *testing.T) {
	origin, clone := newScratchRepoOnBranch(t, "main")
	pushUpstreamCommit(t, origin, "advance.txt", "v2\n", "advance before tag")
	pushUpstreamTag(t, origin, "v1.0.0", "release 1.0.0")

	fakeHome := t.TempDir()
	out, code := runBackendSubcommandWithHome(t, clone, fakeHome, "self-update")
	if code != 0 {
		t.Fatalf("self-update exit=%d, want 0\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "v1.0.0") {
		t.Fatalf("self-update output does not report v1.0.0:\n%s", out)
	}

	// origin/main advances further WITHOUT a new tag.
	pushUpstreamCommit(t, origin, "untagged.txt", "v3\n", "untagged advance")

	behindOut, behindCode := runBackendFunc(t, clone, fakeHome, "compute_repo_behind_release", "false")
	if behindCode != 0 {
		t.Fatalf("compute_repo_behind_release exit=%d, want 0\noutput:\n%s", behindCode, behindOut)
	}
	if got := strings.TrimSpace(behindOut); got != "0" {
		t.Errorf("compute_repo_behind_release after self-update convergence (untagged origin ahead) = %q, want 0", got)
	}
}

// A tag known to be reachable from origin/main, but not yet merged into
// local main, reports the real commit count -- proves the comparison is a
// real rev-list, not a hardcoded 0.
func TestReleaseBackend_ComputeRepoBehindRelease_NonZeroBehindKnownTag(t *testing.T) {
	origin, clone := newScratchRepo(t)
	pushUpstreamCommit(t, origin, "advance.txt", "v2\n", "advance before tag")
	pushUpstreamTag(t, origin, "v1.0.0", "release 1.0.0")

	// Cache origin/main and the new tag locally WITHOUT moving local main.
	runGit(t, clone, "fetch", "origin", "main")
	fakeHome := t.TempDir()
	if _, code := runBackendFunc(t, clone, fakeHome, "resolve_latest_release_tag", "origin/main"); code != 0 {
		t.Fatalf("resolve_latest_release_tag setup call failed")
	}

	out, code := runBackendFunc(t, clone, fakeHome, "compute_repo_behind_release", "false")
	if code != 0 {
		t.Fatalf("compute_repo_behind_release exit=%d, want 0\noutput:\n%s", code, out)
	}
	if got := strings.TrimSpace(out); got != "1" {
		t.Errorf("compute_repo_behind_release = %q, want 1 (local main is 1 commit behind the known tag)", got)
	}
}

// ---------------------------------------------------------------------------
// task 2.2: cmd_sync_check VERDICT/ACTION extensions (sync-check-verdicts
// R-007/R-008)
// ---------------------------------------------------------------------------

func TestReleaseBackend_SyncCheck_NeverDeployedTargetNoFabricatedVersion(t *testing.T) {
	origin, clone := newScratchRepo(t)
	// A REAL release tag exists (available to be named) so this test proves
	// the never-deployed guard actively suppresses it -- not merely that
	// there was nothing real to name yet (D1 pre-first-tag bootstrap).
	pushUpstreamTag(t, origin, "v1.5.0", "release 1.5.0")
	writeFile(t, filepath.Join(clone, "overlay.manifest"), "hello/SKILL.md managed\n")

	fakeHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(fakeHome, ".claude", "skills"), 0o755); err != nil {
		t.Fatalf("mkdir scratch HOME/.claude/skills: %v", err)
	}

	out, code := runBackendSubcommandWithHome(t, clone, fakeHome, "sync-check", "--target", "claude")
	if code != 0 {
		t.Fatalf("sync-check exit=%d, want 0\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "REPO_BEHIND_RELEASE=") {
		t.Errorf("VERDICT does not carry a REPO_BEHIND_RELEASE field:\n%s", out)
	}
	if !strings.Contains(out, "RECORDED_VERSION=NA") {
		t.Errorf("VERDICT does not report RECORDED_VERSION=NA for a never-deployed target:\n%s", out)
	}
	if !strings.Contains(out, "DIGEST_MATCH=NA") {
		t.Errorf("VERDICT does not report DIGEST_MATCH=NA for a never-deployed target:\n%s", out)
	}
	if !strings.Contains(out, "ACTION:claude: run 'overlay apply --target claude'") {
		t.Errorf("ACTION does not recommend apply for a never-deployed target:\n%s", out)
	}
	if strings.Contains(out, "release v") {
		t.Errorf("ACTION fabricated a version for a never-deployed target:\n%s", out)
	}
}

// The incident-regression scenario: self-update fast-forwarded main to a new
// release tag, but apply hasn't re-deployed the target yet, so its recorded
// digest (from an OLDER version) has gone stale -- the ACTION line must name
// the new release version, not merely a raw commit-behind count.
//
// This exercises the "release version IS already known" half of R-007 --
// e.g. right after a self-update, or with sync-check's own --check-origin --
// so it explicitly passes --fetch (post CRITICAL-1 fix, sync-check's default
// path is cached-only and would otherwise never learn about v1.5.0; the
// "not known yet" half is TestReleaseBackend_SyncCheck_DefaultDoesNotFetchTags
// below).
func TestReleaseBackend_SyncCheck_DigestMismatchActionNamesReleaseVersion(t *testing.T) {
	origin, clone := newScratchRepo(t)
	pushUpstreamTag(t, origin, "v1.5.0", "release 1.5.0")
	writeFile(t, filepath.Join(clone, "overlay.manifest"), "hello/SKILL.md managed\n")

	fakeHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(fakeHome, ".claude", "skills"), 0o755); err != nil {
		t.Fatalf("mkdir scratch HOME/.claude/skills: %v", err)
	}

	// Seed a recorded state at an OLDER version so the target IS deployed
	// (not never-deployed) but its digest is stale relative to the new tag.
	if _, code := runBackendFunc(t, clone, fakeHome, "state_write_target", "claude", "v1.4.0", "stale-digest-does-not-match"); code != 0 {
		t.Fatalf("state_write_target setup call failed")
	}

	out, code := runBackendSubcommandWithHome(t, clone, fakeHome, "sync-check", "--target", "claude", "--fetch")
	if code != 0 {
		t.Fatalf("sync-check exit=%d, want 0\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "RECORDED_VERSION=v1.4.0") {
		t.Errorf("VERDICT does not report RECORDED_VERSION=v1.4.0:\n%s", out)
	}
	if !strings.Contains(out, "DIGEST_MATCH=no") {
		t.Errorf("VERDICT does not report DIGEST_MATCH=no:\n%s", out)
	}
	wantAction := "ACTION:claude: run 'overlay apply --target claude' (release v1.5.0 available)"
	if !strings.Contains(out, wantAction) {
		t.Errorf("ACTION does not name the available release version:\nwant substring: %s\ngot:\n%s", wantAction, out)
	}
}

// Regression (sdd-verify CRITICAL-1): sync-check's default path (no
// --check-origin/--fetch) must invoke NO `git fetch`, exactly like its
// siblings compute_repo_behind_origin/compute_repo_behind_release --
// sync-check-verdicts R-001 and tui-self-update R-001 (every TUI launch
// runs a flagless sync-check via probeBehindOriginCmd) both require this.
// resolve_latest_release_tag's unconditional fetch, called unconditionally
// from cmd_sync_check's release_version line, violated it: this reproduces
// the exact scenario the verify report proved by execution (a clone with
// zero local tags, origin carrying a real annotated tag, plain sync-check)
// and additionally asserts sync-check completes successfully with an
// "untagged" fallback rather than silently depending on network access.
func TestReleaseBackend_SyncCheck_DefaultDoesNotFetchTags(t *testing.T) {
	origin, clone := newScratchRepo(t)
	pushUpstreamTag(t, origin, "v1.2.3", "release 1.2.3")
	writeFile(t, filepath.Join(clone, "overlay.manifest"), "hello/SKILL.md managed\n")

	if tags := runGit(t, clone, "tag", "-l"); strings.TrimSpace(tags) != "" {
		t.Fatalf("scratch clone already has local tags before sync-check runs: %q", tags)
	}

	fakeHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(fakeHome, ".claude", "skills"), 0o755); err != nil {
		t.Fatalf("mkdir scratch HOME/.claude/skills: %v", err)
	}

	out, code := runBackendSubcommandWithHome(t, clone, fakeHome, "sync-check", "--target", "claude")
	if code != 0 {
		t.Fatalf("sync-check exit=%d, want 0\noutput:\n%s", code, out)
	}

	if tags := runGit(t, clone, "tag", "-l"); strings.TrimSpace(tags) != "" {
		t.Errorf("default sync-check (no --check-origin/--fetch) invoked git fetch and populated local tags: %q", tags)
	}
	if !strings.Contains(out, "ACTION:claude: run 'overlay apply --target claude'") {
		t.Errorf("ACTION did not fall back to the untagged case (no release version should be knowable without a fetch):\n%s", out)
	}
	if strings.Contains(out, "release v1.2.3") {
		t.Errorf("ACTION named the release version despite never fetching it:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// task 2.3: cmd_status in-sync-at-version (sync-check-verdicts R-008)
// ---------------------------------------------------------------------------

func TestReleaseBackend_Status_InSyncTargetNamesVersion(t *testing.T) {
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
	applyOut, applyCode := runBackendSubcommandWithHome(t, clone, fakeHome, "apply", "--target", "claude")
	if applyCode != 0 {
		t.Fatalf("apply exit=%d, want 0\noutput:\n%s", applyCode, applyOut)
	}

	// apply restores 'feature-x' on exit, which never received the manifest
	// merge -- checkout 'main' so status reads the same overlay.manifest
	// apply itself deployed from (same discipline as the task 1.4 test).
	runGit(t, clone, "checkout", "main")

	out, code := runBackendSubcommandWithHome(t, clone, fakeHome, "status", "--target", "claude")
	if code != 0 {
		t.Fatalf("status exit=%d, want 0\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "version: in sync at untagged") {
		t.Errorf("status does not report claude in sync at its recorded version:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// task 2.3: cmd_update (design decision D6, overlay-update-check R-001)
// ---------------------------------------------------------------------------

func TestReleaseBackend_Update_PreFirstTagReportsNoReleasesPublishedYet(t *testing.T) {
	_, clone := newScratchRepo(t)
	fakeHome := t.TempDir()

	out, code := runBackendSubcommandWithHome(t, clone, fakeHome, "update")
	if code != 0 {
		t.Fatalf("update exit=%d, want 0\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "Latest release: (no releases published yet)") {
		t.Errorf("update does not report the D1 pre-first-tag notice:\n%s", out)
	}
}

func TestReleaseBackend_Update_ReportsLatestVersionAndNeverDeployedTargets(t *testing.T) {
	origin, clone := newScratchRepo(t)
	pushUpstreamTag(t, origin, "v1.5.0", "release 1.5.0")

	fakeHome := t.TempDir()
	out, code := runBackendSubcommandWithHome(t, clone, fakeHome, "update")
	if code != 0 {
		t.Fatalf("update exit=%d, want 0\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "Latest release: v1.5.0") {
		t.Errorf("update does not report the latest release version:\n%s", out)
	}
	if !strings.Contains(out, "claude: never deployed") {
		t.Errorf("update does not report a never-deployed target honestly:\n%s", out)
	}
}

func TestReleaseBackend_Update_BehindTargetReportedByName(t *testing.T) {
	origin, clone := newScratchRepo(t)
	pushUpstreamTag(t, origin, "v1.5.0", "release 1.5.0")

	fakeHome := t.TempDir()
	if _, code := runBackendFunc(t, clone, fakeHome, "state_write_target", "claude", "v1.4.0", "some-digest"); code != 0 {
		t.Fatalf("state_write_target setup call failed")
	}

	out, code := runBackendSubcommandWithHome(t, clone, fakeHome, "update")
	if code != 0 {
		t.Fatalf("update exit=%d, want 0\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "claude: behind (v1.5.0 available, have v1.4.0)") {
		t.Errorf("update does not report claude as behind by name:\n%s", out)
	}
}

func TestReleaseBackend_Update_UpToDateTargetReportedByName(t *testing.T) {
	origin, clone := newScratchRepo(t)
	pushUpstreamTag(t, origin, "v1.5.0", "release 1.5.0")

	fakeHome := t.TempDir()
	if _, code := runBackendFunc(t, clone, fakeHome, "state_write_target", "claude", "v1.5.0", "some-digest"); code != 0 {
		t.Fatalf("state_write_target setup call failed")
	}

	out, code := runBackendSubcommandWithHome(t, clone, fakeHome, "update")
	if code != 0 {
		t.Fatalf("update exit=%d, want 0\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "claude: up-to-date (v1.5.0)") {
		t.Errorf("update does not report claude as up-to-date:\n%s", out)
	}
}

// Running `update` twice in a row with no intervening change must leave
// every git ref, target file, and state-file byte identical (R-001).
func TestReleaseBackend_Update_ZeroMutationAcrossRepeatedRuns(t *testing.T) {
	origin, clone := newScratchRepo(t)
	pushUpstreamTag(t, origin, "v1.5.0", "release 1.5.0")

	fakeHome := t.TempDir()
	if _, code := runBackendFunc(t, clone, fakeHome, "state_write_target", "claude", "v1.4.0", "some-digest"); code != 0 {
		t.Fatalf("state_write_target setup call failed")
	}

	mainHeadBefore := headRev(t, clone, "main")
	featureHeadBefore := headRev(t, clone, "feature-x")
	statePath := filepath.Join(fakeHome, ".labdrian-overlay", "state.json")
	stateBefore, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read state.json before: %v", err)
	}

	if _, code := runBackendSubcommandWithHome(t, clone, fakeHome, "update"); code != 0 {
		t.Fatalf("first update failed")
	}
	if _, code := runBackendSubcommandWithHome(t, clone, fakeHome, "update"); code != 0 {
		t.Fatalf("second update failed")
	}

	if got := currentBranch(t, clone); got != "feature-x" {
		t.Errorf("current branch = %q, want unchanged feature-x (update must never checkout)", got)
	}
	if got := headRev(t, clone, "main"); got != mainHeadBefore {
		t.Errorf("main HEAD changed (%s -> %s), want untouched", mainHeadBefore, got)
	}
	if got := headRev(t, clone, "feature-x"); got != featureHeadBefore {
		t.Errorf("feature-x HEAD changed (%s -> %s), want untouched", featureHeadBefore, got)
	}

	stateAfter, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read state.json after: %v", err)
	}
	if string(stateBefore) != string(stateAfter) {
		t.Errorf("state.json changed across two 'update' runs:\nbefore:\n%s\nafter:\n%s", stateBefore, stateAfter)
	}
}
