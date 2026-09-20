package skills

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- fixtures -------------------------------------------------------------

// projectCLIRegistry is a minimal, parseable overlay registry that matches
// nothing. Registration reads it through --registry for the MatchCandidate
// identity check (design.md, "Identity"), so every CLI test needs a real,
// readable one: an unreadable registry is a fail-closed refusal.
const projectCLIRegistry = `version: "1"
skills:
  - id: unrelated-skill
    path: unrelated-skill
    source:
      type: custom
    install:
      defaultScope: global
      targets:
        - claude
    lifecycle:
      updateStrategy: overlay-only
`

// projectCLIRegistryMatching is the same registry with one entry whose id is
// the draft's own id, so MatchCandidate matches and registration refuses. It
// is the witness that --registry's VALUE is actually read rather than merely
// consumed as a wrapper flag.
const projectCLIRegistryMatching = `version: "1"
skills:
  - id: tidy-worktree
    path: tidy-worktree
    source:
      type: custom
    install:
      defaultScope: global
      targets:
        - claude
    lifecycle:
      updateStrategy: overlay-only
`

// projectCLIEnv builds one isolated registration environment: an empty
// project root, a draft OUTSIDE it (step 2 of the validate-before-write
// order), a readable registry, and an overlay-style `skills-lock.json` decoy
// inside the root whose bytes every test asserts unchanged. Everything roots
// at t.TempDir(): no test here reads or writes $HOME, the live
// .claude/skills, .agents/skills, .pi or the repository's own
// skills-lock.json.
type projectCLIEnv struct {
	root         string
	draftPath    string
	registryPath string
	decoyPath    string
	decoyBytes   []byte
}

func newProjectCLIEnv(t *testing.T, id, registryYAML string) projectCLIEnv {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, "project")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir project root: %v", err)
	}
	draftDir := filepath.Join(base, "drafts")
	if err := os.MkdirAll(draftDir, 0o755); err != nil {
		t.Fatalf("mkdir draft dir: %v", err)
	}
	draftPath := filepath.Join(draftDir, "SKILL.md")
	if err := os.WriteFile(draftPath, validDraft(id), 0o644); err != nil {
		t.Fatalf("write draft: %v", err)
	}
	registryPath := filepath.Join(base, "skills.registry.yaml")
	if err := os.WriteFile(registryPath, []byte(registryYAML), 0o644); err != nil {
		t.Fatalf("write registry: %v", err)
	}
	decoy := []byte("{\"decoy\":\"the overlay's own lock file is never touched\"}\n")
	decoyPath := filepath.Join(root, "skills-lock.json")
	if err := os.WriteFile(decoyPath, decoy, 0o644); err != nil {
		t.Fatalf("write skills-lock.json decoy: %v", err)
	}
	return projectCLIEnv{root: root, draftPath: draftPath, registryPath: registryPath, decoyPath: decoyPath, decoyBytes: decoy}
}

// assertDecoyUntouched proves the run never wrote the overlay-tier
// skills-lock.json, which this capability must never touch at any tier.
func (e projectCLIEnv) assertDecoyUntouched(t *testing.T) {
	t.Helper()
	got, err := os.ReadFile(e.decoyPath)
	if err != nil {
		t.Fatalf("reading skills-lock.json decoy: %v", err)
	}
	if !bytes.Equal(got, e.decoyBytes) {
		t.Errorf("skills-lock.json bytes changed: got %q, want %q", got, e.decoyBytes)
	}
}

// assertNothingWritten proves a refusal left the project root exactly as the
// fixture built it: no target directory, no lock file.
func (e projectCLIEnv) assertNothingWritten(t *testing.T) {
	t.Helper()
	for _, rel := range []string{".claude", ".agents", ".labdrian"} {
		if _, err := os.Stat(filepath.Join(e.root, rel)); !os.IsNotExist(err) {
			t.Errorf("%s must not exist after a refusal (stat err = %v)", rel, err)
		}
	}
	e.assertDecoyUntouched(t)
}

// runProjectRegister drives RenderProjectRegisterCore with real temp-dir I/O
// and a captured exit code, the way lint_cli_test.go drives RenderLintCore.
func runProjectRegister(t *testing.T, args []string) (stdout, stderr string, exitCode int) {
	t.Helper()
	var out, errBuf bytes.Buffer
	exitCode = -1
	RenderProjectRegisterCore(args, os.ReadFile, os.Stat, resolvePathKeepingMissing, osProjectFS{}, &out, &errBuf, func(c int) { exitCode = c })
	return out.String(), errBuf.String(), exitCode
}

// refusingReadFile fails the test if it is ever called: an argument-level
// refusal must happen before any file is read, so nothing is read and nothing
// can be written.
func refusingReadFile(t *testing.T) readFileFn {
	t.Helper()
	return func(p string) ([]byte, error) {
		t.Errorf("readFile(%q) must not be called: the argument refusal happens before any I/O", p)
		return nil, errors.New("must not be called")
	}
}

func runProjectRegisterNoIO(t *testing.T, args []string) (stdout, stderr string, exitCode int) {
	t.Helper()
	var out, errBuf bytes.Buffer
	exitCode = -1
	RenderProjectRegisterCore(args, refusingReadFile(t), func(string) (fs.FileInfo, error) {
		t.Error("stat must not be called before the argument refusal")
		return nil, errors.New("must not be called")
	}, resolvePathKeepingMissing, osProjectFS{}, &out, &errBuf, func(c int) { exitCode = c })
	return out.String(), errBuf.String(), exitCode
}

func registerArgs(e projectCLIEnv, extra ...string) []string {
	args := []string{"--project-root", e.root, "--candidate", testCandidateKey, "--registry", e.registryPath}
	args = append(args, extra...)
	return append(args, e.draftPath)
}

// --- the happy path -------------------------------------------------------

// TestRenderProjectRegisterCore_WritesEveryTargetAndPrintsTrustNote is the
// positive case: a real run writes one SKILL.md per fixed target plus the
// lock, and prints `wrote:` per path, then `sha256:`, `revision:` and the Pi
// trust `note:` line LAST (design.md decision (c) step 6, plus the Pi trust
// consequence). The note prints on success only.
func TestRenderProjectRegisterCore_WritesEveryTargetAndPrintsTrustNote(t *testing.T) {
	const id = "tidy-worktree"
	e := newProjectCLIEnv(t, id, projectCLIRegistry)

	out, errOut, code := runProjectRegister(t, registerArgs(e))
	if code != 0 {
		t.Fatalf("expected exit 0 for a valid registration, got %d, stderr: %q", code, errOut)
	}

	wantRels := []string{
		".claude/skills/" + id + "/SKILL.md",
		".agents/skills/" + id + "/SKILL.md",
		ProjectLockRelPath,
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != len(wantRels)+3 {
		t.Fatalf("expected %d output lines (one wrote: per path, sha256, revision, note), got %d: %q", len(wantRels)+3, len(lines), out)
	}
	for i, rel := range wantRels {
		if lines[i] != "wrote: "+rel {
			t.Errorf("line %d = %q, want %q", i, lines[i], "wrote: "+rel)
		}
	}

	skillBytes, err := os.ReadFile(filepath.Join(e.root, filepath.FromSlash(wantRels[0])))
	if err != nil {
		t.Fatalf("reading the written SKILL.md: %v", err)
	}
	wantSum := HashSkill(skillBytes)
	if lines[len(wantRels)] != "sha256: "+wantSum {
		t.Errorf("sha256 line = %q, want %q (the hash of the bytes actually written)", lines[len(wantRels)], "sha256: "+wantSum)
	}
	if lines[len(wantRels)+1] != "revision: 1" {
		t.Errorf("revision line = %q, want %q", lines[len(wantRels)+1], "revision: 1")
	}
	if lines[len(wantRels)+2] != PiTrustNote {
		t.Errorf("last line = %q, want the Pi trust note %q", lines[len(wantRels)+2], PiTrustNote)
	}

	for _, rel := range wantRels {
		if _, err := os.Stat(filepath.Join(e.root, filepath.FromSlash(rel))); err != nil {
			t.Errorf("%s must exist after a successful run: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(e.root, ".pi")); !os.IsNotExist(err) {
		t.Errorf(".pi/ must never be created (stat err = %v)", err)
	}
	e.assertDecoyUntouched(t)
}

// TestRenderProjectRegisterCore_DryRunPrintsPlanAndWritesNothing pins the
// step-4 contract: --dry-run prints the `plan: <rel>` lines the agent feeds
// to `git check-ignore` and writes nothing at all. It prints no `wrote:`
// line, no `sha256:`/`revision:` line, and — because nothing was written —
// no Pi trust note.
func TestRenderProjectRegisterCore_DryRunPrintsPlanAndWritesNothing(t *testing.T) {
	const id = "tidy-worktree"
	e := newProjectCLIEnv(t, id, projectCLIRegistry)

	out, errOut, code := runProjectRegister(t, registerArgs(e, "--dry-run"))
	if code != 0 {
		t.Fatalf("expected exit 0 for a valid --dry-run, got %d, stderr: %q", code, errOut)
	}

	want := "plan: .claude/skills/" + id + "/SKILL.md\n" +
		"plan: .agents/skills/" + id + "/SKILL.md\n" +
		"plan: " + ProjectLockRelPath + "\n"
	if out != want {
		t.Errorf("--dry-run stdout = %q, want exactly the plan lines %q", out, want)
	}
	if strings.Contains(out, PiTrustNote) {
		t.Errorf("--dry-run must not print the Pi trust note, got %q", out)
	}
	e.assertNothingWritten(t)
}

// --- argument refusals ----------------------------------------------------

// TestRenderProjectRegisterCore_MissingProjectRootExitsOneWithoutReading
// pins the no-cwd-fallback rule (R-002 precedent, design.md validate step 1):
// a missing --project-root is a usage error, never a silent registration
// against the working directory. The injected readFile and stat both fail the
// test if called, so "nothing written" is proved by "nothing even read".
func TestRenderProjectRegisterCore_MissingProjectRootExitsOneWithoutReading(t *testing.T) {
	out, errOut, code := runProjectRegisterNoIO(t, []string{"--candidate", testCandidateKey, "/tmp/nowhere/SKILL.md"})
	if code != 1 {
		t.Fatalf("expected exit 1 without --project-root, got %d, stderr: %q", code, errOut)
	}
	if !strings.Contains(errOut, "--project-root") {
		t.Errorf("stderr must name the missing flag --project-root, got %q", errOut)
	}
	if out != "" {
		t.Errorf("a refusal must print nothing to stdout, got %q", out)
	}
	if strings.Contains(errOut, PiTrustNote) {
		t.Errorf("a refusal must never print the Pi trust note, got %q", errOut)
	}
}

// TestRenderProjectRegisterCore_RelativeProjectRootRefusedWithoutReading
// pins the same rule's second half: a RELATIVE root is refused before any
// path is joined against it, so no read and no write can be aimed at a
// cwd-derived location.
func TestRenderProjectRegisterCore_RelativeProjectRootRefusedWithoutReading(t *testing.T) {
	out, errOut, code := runProjectRegisterNoIO(t, []string{"--project-root", "relative/project", "--candidate", testCandidateKey, "/tmp/nowhere/SKILL.md"})
	if code != 1 {
		t.Fatalf("expected exit 1 for a relative --project-root, got %d, stderr: %q", code, errOut)
	}
	if !strings.Contains(errOut, "relative/project") {
		t.Errorf("stderr must name the offending relative root, got %q", errOut)
	}
	if out != "" {
		t.Errorf("a refusal must print nothing to stdout, got %q", out)
	}
}

// TestRenderProjectRegisterCore_MissingCandidateRefusedWithoutReading: the
// candidate key is what binds the registered skill back to its Engram record,
// so it is required, not defaulted.
func TestRenderProjectRegisterCore_MissingCandidateRefusedWithoutReading(t *testing.T) {
	_, errOut, code := runProjectRegisterNoIO(t, []string{"--project-root", "/abs/project", "/tmp/nowhere/SKILL.md"})
	if code != 1 {
		t.Fatalf("expected exit 1 without --candidate, got %d, stderr: %q", code, errOut)
	}
	if !strings.Contains(errOut, "--candidate") {
		t.Errorf("stderr must name the missing flag --candidate, got %q", errOut)
	}
}

// TestRenderProjectRegisterCore_MissingDraftArgumentRefusedWithoutReading:
// with both flags present but no positional, there is nothing to register.
func TestRenderProjectRegisterCore_MissingDraftArgumentRefusedWithoutReading(t *testing.T) {
	_, errOut, code := runProjectRegisterNoIO(t, []string{"--project-root", "/abs/project", "--candidate", testCandidateKey})
	if code != 1 {
		t.Fatalf("expected exit 1 without a draft argument, got %d, stderr: %q", code, errOut)
	}
	if !strings.Contains(errOut, "<draft-file>") {
		t.Errorf("stderr must name the missing <draft-file> argument, got %q", errOut)
	}
}

// TestRenderProjectRegisterCore_UnknownFlagAfterPathRefused and its
// before-path sibling protect the same property RenderLintCore's
// unknown-flag rejection protects (review-b75e4a27b9494ff8 R4-001): an
// unrecognized dash-argument is NEVER silently dropped, because dropping it
// would let a mistyped `--dry-run` register for real while the operator
// believes they asked for a plan.
func TestRenderProjectRegisterCore_UnknownFlagAfterPathRefused(t *testing.T) {
	_, errOut, code := runProjectRegisterNoIO(t, []string{"--project-root", "/abs/project", "--candidate", testCandidateKey, "/tmp/nowhere/SKILL.md", "--dryrun"})
	if code != 1 {
		t.Fatalf("expected exit 1 for an unknown flag, got %d, stderr: %q", code, errOut)
	}
	if !strings.Contains(errOut, "--dryrun") {
		t.Errorf("stderr must name the offending unknown flag %q, got %q", "--dryrun", errOut)
	}
}

// TestRenderProjectRegisterCore_UnknownFlagBeforePathRefused is the
// position-independence half: the parser is a single pass over args, so an
// unknown flag must be rejected wherever it sits, not only after the
// positional has been bound (task 4.0 item 4).
func TestRenderProjectRegisterCore_UnknownFlagBeforePathRefused(t *testing.T) {
	_, errOut, code := runProjectRegisterNoIO(t, []string{"--dryrun", "--project-root", "/abs/project", "--candidate", testCandidateKey, "/tmp/nowhere/SKILL.md"})
	if code != 1 {
		t.Fatalf("expected exit 1 for an unknown flag before the path, got %d, stderr: %q", code, errOut)
	}
	if !strings.Contains(errOut, "--dryrun") {
		t.Errorf("stderr must name the offending unknown flag %q, got %q", "--dryrun", errOut)
	}
}

// TestRenderProjectRegisterCore_ExtraPositionalRefused: project-register
// takes exactly one draft. A second positional is a usage error, not a
// silently ignored argument — the same false-clean hazard as lint's.
func TestRenderProjectRegisterCore_ExtraPositionalRefused(t *testing.T) {
	_, errOut, code := runProjectRegisterNoIO(t, []string{"--project-root", "/abs/project", "--candidate", testCandidateKey, "/tmp/a.md", "/tmp/b.md"})
	if code != 1 {
		t.Fatalf("expected exit 1 for a second positional, got %d, stderr: %q", code, errOut)
	}
	if !strings.Contains(errOut, "/tmp/b.md") {
		t.Errorf("stderr must name the offending extra argument, got %q", errOut)
	}
}

// TestRenderProjectRegisterCore_EndOfOptionsEscapeBindsDashPrefixedDraft
// pins the `--` escape (task 4.0 item 3, widened to this parser): a draft
// path that begins with a dash is a legitimate filename, and without `--` it
// would be misparsed as an unknown flag and refused. After `--`, every
// remaining argument is a positional.
func TestRenderProjectRegisterCore_EndOfOptionsEscapeBindsDashPrefixedDraft(t *testing.T) {
	var got string
	var out, errBuf bytes.Buffer
	exitCode := -1
	readFile := func(p string) ([]byte, error) {
		if strings.HasSuffix(p, "skills.registry.yaml") {
			return []byte(projectCLIRegistry), nil
		}
		got = p
		return nil, errors.New("draft read stops the run here, on purpose")
	}
	RenderProjectRegisterCore(
		[]string{"--project-root", "/abs/project", "--candidate", testCandidateKey, "--registry", "skills.registry.yaml", "--", "-weird-draft.md"},
		readFile, os.Stat, resolvePathKeepingMissing, osProjectFS{}, &out, &errBuf, func(c int) { exitCode = c })

	if got != "-weird-draft.md" {
		t.Errorf("after `--` the draft path must bind verbatim, readFile saw %q", got)
	}
	if exitCode != 1 {
		t.Fatalf("expected exit 1 when the draft cannot be read, got %d, stderr: %q", exitCode, errBuf.String())
	}
}

// --- wrapper flags and fail-closed reads ----------------------------------

// TestRenderProjectRegisterCore_TolerantOfWrapperInjectedTrailingFlags: the
// `labdrian` wrapper appends `--registry <path> --manifest <path>
// --source-root <path>` after every verb's own arguments. --manifest and
// --source-root are consumed and ignored; leaving them to the default branch
// would misparse their VALUE as the draft positional.
func TestRenderProjectRegisterCore_TolerantOfWrapperInjectedTrailingFlags(t *testing.T) {
	const id = "tidy-worktree"
	e := newProjectCLIEnv(t, id, projectCLIRegistry)

	args := []string{"--project-root", e.root, "--candidate", testCandidateKey, "--dry-run", e.draftPath,
		"--registry", e.registryPath, "--manifest", "overlay.manifest", "--source-root", "skills"}
	out, errOut, code := runProjectRegister(t, args)
	if code != 0 {
		t.Fatalf("expected exit 0 with wrapper-injected trailing flags, got %d, stderr: %q", code, errOut)
	}
	if !strings.Contains(out, "plan: .claude/skills/"+id+"/SKILL.md") {
		t.Errorf("wrapper flags must not disturb the plan output, got %q", out)
	}
	e.assertNothingWritten(t)
}

// TestRenderProjectRegisterCore_RegistryValueIsRead proves --registry is not
// merely consumed like --manifest: its VALUE feeds MatchCandidate, so a
// registry already carrying the draft's id refuses the registration (the
// global skill owns that identity; the project must not shadow it).
func TestRenderProjectRegisterCore_RegistryValueIsRead(t *testing.T) {
	const id = "tidy-worktree"
	e := newProjectCLIEnv(t, id, projectCLIRegistryMatching)

	out, errOut, code := runProjectRegister(t, registerArgs(e))
	if code != 1 {
		t.Fatalf("expected exit 1 when the overlay registry already owns the id, got %d, stdout: %q", code, out)
	}
	if !strings.Contains(errOut, id) {
		t.Errorf("stderr must name the matched id, got %q", errOut)
	}
	e.assertNothingWritten(t)
}

// TestRenderProjectRegisterCore_UnreadableRegistryRefuses: an unreadable
// registry is fail-closed (design.md, "Identity"). Registering while the
// identity check cannot run would be exactly the shadowing the check exists
// to prevent.
func TestRenderProjectRegisterCore_UnreadableRegistryRefuses(t *testing.T) {
	const id = "tidy-worktree"
	e := newProjectCLIEnv(t, id, projectCLIRegistry)
	missing := filepath.Join(e.root, "..", "no-such-registry.yaml")

	args := []string{"--project-root", e.root, "--candidate", testCandidateKey, "--registry", missing, e.draftPath}
	out, errOut, code := runProjectRegister(t, args)
	if code != 1 {
		t.Fatalf("expected exit 1 for an unreadable registry, got %d, stdout: %q", code, out)
	}
	if !strings.Contains(errOut, "no-such-registry.yaml") {
		t.Errorf("stderr must name the unreadable registry, got %q", errOut)
	}
	e.assertNothingWritten(t)
}

// --- refusals print no note and write nothing -----------------------------

// TestRenderProjectRegisterCore_PlannerRefusalPrintsNoNoteAndWritesNothing:
// a refusal raised INSIDE the planner (here: a draft that lies inside the
// project root) must reach the operator as exit 1 on stderr, with an empty
// stdout — no `wrote:` line the agent could feed to `git add`, and no Pi
// trust note claiming a write that never happened.
func TestRenderProjectRegisterCore_PlannerRefusalPrintsNoNoteAndWritesNothing(t *testing.T) {
	const id = "tidy-worktree"
	e := newProjectCLIEnv(t, id, projectCLIRegistry)
	inside := filepath.Join(e.root, "draft-SKILL.md")
	if err := os.WriteFile(inside, validDraft(id), 0o644); err != nil {
		t.Fatalf("write inside-root draft: %v", err)
	}

	args := []string{"--project-root", e.root, "--candidate", testCandidateKey, "--registry", e.registryPath, inside}
	out, errOut, code := runProjectRegister(t, args)
	if code != 1 {
		t.Fatalf("expected exit 1 for a draft inside the project root, got %d, stdout: %q", code, out)
	}
	if out != "" {
		t.Errorf("a refusal must print nothing to stdout, got %q", out)
	}
	if strings.Contains(errOut, PiTrustNote) {
		t.Errorf("a refusal must never print the Pi trust note, got %q", errOut)
	}
	if !strings.Contains(errOut, "outside the project root") {
		t.Errorf("stderr must explain the refusal, got %q", errOut)
	}
	e.assertNothingWritten(t)
}

// --- printed paths are git pathspecs (task 4.6) ---------------------------

// TestRenderProjectRegisterCore_PrintedPathsAreRepoRelativePathspecs is the
// REFACTOR-step proof for task 4.6. Every `plan:` and `wrote:` path must be
// repo-relative with forward slashes, because the agent procedure feeds them
// straight to `git -C R check-ignore --`, `git -C R add --` and `git -C R
// commit --`. An absolute path would leak the checkout root and a backslash
// would not be a pathspec at all.
func TestRenderProjectRegisterCore_PrintedPathsAreRepoRelativePathspecs(t *testing.T) {
	const id = "tidy-worktree"
	e := newProjectCLIEnv(t, id, projectCLIRegistry)

	planOut, _, planCode := runProjectRegister(t, registerArgs(e, "--dry-run"))
	if planCode != 0 {
		t.Fatalf("dry run failed: %d", planCode)
	}
	wroteOut, _, code := runProjectRegister(t, registerArgs(e))
	if code != 0 {
		t.Fatalf("real run failed: %d", code)
	}

	planPaths := printedPaths(t, planOut, "plan: ")
	wrotePaths := printedPaths(t, wroteOut, "wrote: ")
	if len(planPaths) == 0 {
		t.Fatalf("no plan paths were printed")
	}
	if len(planPaths) != len(wrotePaths) {
		t.Fatalf("the plan set (%d) and the wrote set (%d) must be the same paths", len(planPaths), len(wrotePaths))
	}
	for i := range planPaths {
		if planPaths[i] != wrotePaths[i] {
			t.Errorf("plan path %q and wrote path %q must be identical, so `check-ignore` and `add` see one pathspec set", planPaths[i], wrotePaths[i])
		}
	}
	for _, p := range wrotePaths {
		if filepath.IsAbs(p) || strings.HasPrefix(p, "/") {
			t.Errorf("printed path %q must be repo-relative, not absolute", p)
		}
		if strings.Contains(p, "\\") {
			t.Errorf("printed path %q must use forward slashes", p)
		}
		if strings.Contains(p, e.root) {
			t.Errorf("printed path %q must never carry the absolute checkout root", p)
		}
		// The decisive property: joined onto the root it names a real file,
		// which is exactly what `git -C R add -- <p>` would stage.
		if _, err := os.Stat(filepath.Join(e.root, filepath.FromSlash(p))); err != nil {
			t.Errorf("printed path %q must resolve under the project root: %v", p, err)
		}
	}
}

func printedPaths(t *testing.T, out, prefix string) []string {
	t.Helper()
	var paths []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, prefix) {
			paths = append(paths, strings.TrimPrefix(line, prefix))
		}
	}
	return paths
}

// --- dispatch -------------------------------------------------------------

// TestSkillsCore_DispatchesProjectRegister proves the verb reaches the core
// through SkillsCore with the verb token stripped, so `project-register` is
// never re-read as the draft positional.
func TestSkillsCore_DispatchesProjectRegister(t *testing.T) {
	const id = "tidy-worktree"
	e := newProjectCLIEnv(t, id, projectCLIRegistry)

	var out, errBuf bytes.Buffer
	exitCode := -1
	args := append([]string{"project-register"}, registerArgs(e, "--dry-run")...)
	SkillsCore("project-register", args, os.ReadFile, &out, &errBuf, func(c int) { exitCode = c })

	if exitCode != 0 {
		t.Fatalf("expected exit 0 through SkillsCore, got %d, stderr: %q", exitCode, errBuf.String())
	}
	if !strings.Contains(out.String(), "plan: "+ProjectLockRelPath) {
		t.Errorf("SkillsCore must dispatch project-register to its core, got %q", out.String())
	}
	e.assertNothingWritten(t)
}

// TestSkillsCore_VerbEnumerationsNameProjectRegister keeps both usage
// messages honest: a verb that dispatches but is missing from the
// enumeration is undiscoverable.
func TestSkillsCore_VerbEnumerationsNameProjectRegister(t *testing.T) {
	for _, verb := range []string{"", "no-such-verb"} {
		var out, errBuf bytes.Buffer
		SkillsCore(verb, nil, os.ReadFile, &out, &errBuf, func(int) {})
		if !strings.Contains(errBuf.String(), "project-register") {
			t.Errorf("the verb enumeration for %q must name project-register, got %q", verb, errBuf.String())
		}
	}
}
