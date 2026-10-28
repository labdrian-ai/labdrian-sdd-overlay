package main

// Phase 8 (skill lifecycle and projection) acceptance fixtures. Each criterion of
// the Phase 8 ledger's "Acceptance criteria" (odd/tasks/skill-lifecycle.md, which
// joins the repository when the phase closes) is proved end to end against the
// BUILT engine binary: separate OS processes, a temporary overlay (registry,
// manifest, skills/), a temporary project directory, realistic Claude Code hook JSON
// on stdin, and an isolated HOME and XDG_STATE_HOME under t.TempDir(). The engine
// never runs git, and nothing here does either. No test in this file can reach
// ~/.claude, ~/.pi, ~/.codex, ~/.local/state/labdrian, Engram, or the repository's
// own skills, registry, or manifest for writing: every child process gets an
// explicit, minimal environment (HOME, XDG_STATE_HOME, and a PATH that is an empty
// directory of the test's own), and the overlay and the project are directories the
// test made. The one read of the repository's tree is named under Baseline below.
//
// Criterion -> test. The subtests share one build of the binary, so they live under
// one top-level test, TestPhase8Acceptance.
//
//	walk: draft outside skills/, lint, approve,        /Lifecycle_DraftToProjectWithDriftAndReportOnlyRetirement
//	  add, manifest sync, install, project register,
//	  revise, retire, drift, report-only retirement
//	skills add needs a record for the exact bytes      /Approval_AddNeedsARecordForTheExactBytes
//	no global skill is excused from approval           /Approval_NoSkillIsGrandfathered
//	approve warns, not refuses, for lint-baseline      /Baseline_ApproveWarnsForSkillsThatPredateTheLintBudget
//	  skills that fail the hard lint; others still refused
//	approve guard: denial, other commands, family      /ApproveGuard_DeniesTheAgentAndManagesItsSettingsFamily
//	concurrent add and install are serialized          not repeated here: the multi-process tests in engine/skills
//	                                                   prove it with 15 rounds of 19 real processes, which a
//	                                                   fixture here would only shrink:
//	                                                   TestRegistryLockE2E_ConcurrentVerbsNeverLoseAnUpdate (add, remove,
//	                                                   sync-manifest, approve, validate),
//	                                                   TestRegistryLockE2E_ConcurrentApprovalsKeepTheFirstApprover,
//	                                                   TestRegistryLockE2E_ABusyLockIsReportedAsExit2AfterTheBound,
//	                                                   TestProjectLockE2E_ConcurrentRegistrationsAllLandInTheProjectLock,
//	                                                   TestProjectLockE2E_InstallsAndRegistrationsShareOneProjectWithoutLosingAnything
//	                                                   (install beside an overlay writer and readers),
//	                                                   TestProjectLockE2E_ABusyProjectIsReportedAsExit2AndNothingIsWritten;
//	                                                   cmd: TestSkillsLock_TheProductionEntryPointCreatesAndHonoursTheRegistryLock
//	                                                   (the real file locks, a busy lock is exit 2 and changes nothing)
//	install ownership, both targets, idempotent,       /Install_OwnsByHashKeepsForeignFilesAndAdoptIsExplicit
//	  adopt explicit
//	runtime capabilities: skills per runtime, the      /Capabilities_DeclareSkillsPerRuntimeWithTheApplyLimit
//	  labdrian apply limit, and the projected context
//	no os/exec or network in the new code              /NoExecOrNetwork_InTheFilesPhase8AddedOutsideTheGuardedPackages
//	                                                   for the files of cmd, settings, capability, and projection;
//	                                                   skills: TestZeroFetchImportAllowlist,
//	                                                   TestZeroFetchAllowlistExcludesExecAndNet;
//	                                                   filelock: TestProductionFilesImportOnlyTheLockingStdlib
//	no test touches real user state                    the environment of every child process above, and
//	                                                   TestMain (live_guard_test.go) for this package;
//	                                                   skills: TestMain (live_guard_test.go)
//
// Stated gaps, not faked steps.
//
//   - Report-only retirement is proved for its two halves, not as one run. The detector
//     is `longterm-mem skills-stale`, in a separate module whose package is internal; it
//     reads an Engram SQLite database and git history, so driving it here would mean
//     importing that module or starting its binary against a database and a repository
//     this test does not own. The Lifecycle walk therefore proves what this module owns:
//     the project lock the BUILT binary wrote (procedural skills and install records
//     together, after install, register, and revise, and again after retire) carries
//     exactly the fields the detector's strict parser accepts, read from the detector's
//     own source so that either side changing turns this RED; `project-retire --dry-run`
//     reports a plan and writes nothing; and retirement is a separate, explicit verb that
//     nothing in the engine calls. The detector's own behavior is proved in longterm-mem:
//     internal/skillstale TestParseProjectLock_ReadsSharedFixture,
//     TestParseProjectLock_ToleratesTheInstallRecordsOfSkillsInstall,
//     TestDetect_ReportsStaleSignalsAndDoesNotMutate, and the engine side of the shared
//     fixture, skills: TestSerializeProjectLock_MatchesSharedSkillstaleFixture. The global
//     tier has no detector (decision 4): a global skill is retired by hand.
//   - `skills install` admits project-scoped registry entries only, and `skills add`
//     registers global ones only, so no verb declares a project-scoped overlay entry: a
//     person edits the registry. The walk does exactly that, and `sync-manifest` is what
//     gives the new entry its manifest row.
//   - Project-retire runs at the end of the walk, after drift, because the retirement
//     decision comes after the report.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capabilitytest"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// --- harness ----------------------------------------------------------------

// phase8World is a phase7World (an isolated home, state home, PATH, scratch
// directory, and the process runner) with a temporary overlay and a temporary
// project directory.
type phase8World struct {
	phase7World
	overlay string // registry, manifest, and skills/
	project string // the directory skills are installed into
}

func newPhase8World(t *testing.T, binary string) phase8World {
	t.Helper()
	return phase8World{phase7World: newPhase7World(t, binary), overlay: t.TempDir(), project: t.TempDir()}
}

func (w phase8World) registryPath() string { return filepath.Join(w.overlay, "skills.registry.yaml") }
func (w phase8World) manifestPath() string { return filepath.Join(w.overlay, "overlay.manifest") }
func (w phase8World) sourceRoot() string   { return filepath.Join(w.overlay, "skills") }

func (w phase8World) skillPath(id string) string {
	return filepath.Join(w.sourceRoot(), id, "SKILL.md")
}

func (w phase8World) recordPath(id string) string {
	return filepath.Join(w.sourceRoot(), id, skills.ApprovalRecordName)
}

func (w phase8World) overlayFlags() []string {
	return []string{"--registry", w.registryPath(), "--manifest", w.manifestPath(), "--source-root", w.sourceRoot()}
}

func (w phase8World) installFlags(projectID string) []string {
	return []string{"--registry", w.registryPath(), "--source-root", w.sourceRoot(), "--project-id", projectID}
}

// skillsVerb runs 'skills <args>' from cwd.
func (w phase8World) skillsVerb(cwd string, args ...string) phase7Run {
	w.t.Helper()
	return w.engine(cwd, "", append([]string{"skills"}, args...)...)
}

// overlayVerb runs an overlay verb (add, approve, validate, sync-manifest) on the
// world's overlay.
func (w phase8World) overlayVerb(verb string, args ...string) phase7Run {
	w.t.Helper()
	return w.skillsVerb(w.dir, append(append([]string{verb}, args...), w.overlayFlags()...)...)
}

func (w phase8World) approve(id, approver string) phase7Run {
	w.t.Helper()
	return w.overlayVerb("approve", "--id", id, "--approver", approver)
}

// install and adopt run from the directory project, the way a person runs them in a
// repository.
func (w phase8World) install(project, projectID string) phase7Run {
	w.t.Helper()
	return w.skillsVerb(project, append([]string{"install"}, w.installFlags(projectID)...)...)
}

func (w phase8World) adopt(project, projectID string) phase7Run {
	w.t.Helper()
	return w.skillsVerb(project, append([]string{"adopt"}, w.installFlags(projectID)...)...)
}

func (w phase8World) writeOverlay(registry, manifest string) {
	w.t.Helper()
	writeFixtureFile(w.t, w.registryPath(), registry)
	writeFixtureFile(w.t, w.manifestPath(), manifest)
}

func (w phase8World) writeSkill(id, content string) {
	w.t.Helper()
	writeFixtureFile(w.t, w.skillPath(id), content)
}

// expect requires the exit code, and that the output (both streams) carries each text.
func (r phase7Run) expect(t *testing.T, label string, code int, wantIn ...string) phase7Run {
	t.Helper()
	if r.code != code {
		t.Fatalf("%s: exit %d, stdout %q, stderr %q, want exit %d", label, r.code, r.stdout, r.stderr, code)
	}
	for _, want := range wantIn {
		if !strings.Contains(r.stdout+r.stderr, want) {
			t.Errorf("%s: output does not contain %q\nstdout: %s\nstderr: %s", label, want, r.stdout, r.stderr)
		}
	}
	return r
}

// phase8RegistryEntry is one entry of a registry the test writes.
type phase8RegistryEntry struct {
	id       string
	scope    string // "global" or "project"
	projects []string
}

func phase8EntryYAML(e phase8RegistryEntry) string {
	var sb strings.Builder
	sb.WriteString("  - id: " + e.id + "\n    path: " + e.id + "\n    source:\n      type: custom\n")
	sb.WriteString("    install:\n      defaultScope: " + e.scope + "\n      targets:\n        - claude\n")
	if len(e.projects) > 0 {
		sb.WriteString("      allowedProjects:\n")
		for _, p := range e.projects {
			sb.WriteString("        - " + p + "\n")
		}
	}
	sb.WriteString("    lifecycle:\n      updateStrategy: overlay-only\n")
	return sb.String()
}

func phase8Registry(entries ...phase8RegistryEntry) string {
	var sb strings.Builder
	sb.WriteString("version: \"1\"\nskills:\n")
	for _, e := range entries {
		sb.WriteString(phase8EntryYAML(e))
	}
	return sb.String()
}

func phase8Manifest(ids ...string) string {
	var sb strings.Builder
	for _, id := range ids {
		sb.WriteString(id + "/SKILL.md custom\n")
	}
	return sb.String()
}

// phase8SkillMD is a SKILL.md that passes every hard lint rule; step makes two skills
// differ in more than their name.
func phase8SkillMD(name, step string) string {
	return "---\n" +
		"name: " + name + "\n" +
		"description: A concise procedural skill for " + name + ".\n" +
		"license: MIT\n" +
		"metadata:\n" +
		"  author: tester\n" +
		"  version: \"1.0\"\n" +
		"---\n" +
		"## Activation Contract\n" +
		"Load this skill for its documented procedure.\n\n" +
		"## Hard Rules\n" +
		"- Keep the procedure explicit.\n\n" +
		"## Execution Steps\n" +
		"1. " + step + "\n"
}

func phase8Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// phase8ApprovalDigest is the digest an approval binds for a skill whose directory holds a
// SKILL.md and nothing else: the SHA-256 of the one line that `sha256sum` writes for it. It is made
// here from the rule, not by the code under test.
func phase8ApprovalDigest(skillMD string) string {
	return phase8Digest([]byte(phase8Digest([]byte(skillMD)) + " -  SKILL.md\n"))
}

func phase8Read(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

// phase8SameEntries reports whether two settings files hold the same entries under every event: the order of a hook list
// is left out, because a merge that repairs a drifted entry removes it and appends the repaired one at the end of its
// list, after the entries of the families installed after it (own-rdd T11d: the builder guard comes last).
func phase8SameEntries(t *testing.T, a, b []byte) bool {
	t.Helper()
	canonical := func(data []byte) string {
		var doc map[string]any
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatalf("settings are not JSON: %v", err)
		}
		hooks, _ := doc["hooks"].(map[string]any)
		for event, list := range hooks {
			entries, _ := list.([]any)
			keys := make([]string, 0, len(entries))
			for _, e := range entries {
				k, err := json.Marshal(e)
				if err != nil {
					t.Fatal(err)
				}
				keys = append(keys, string(k))
			}
			sort.Strings(keys)
			hooks[event] = keys
		}
		out, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}
	return canonical(a) == canonical(b)
}

// phase8Overlay is the overlay as bytes, leaving out the lock files: a writer's
// exclusive lock creates the lock file beside the registry even when the verb is then
// refused, and "writes nothing" is about the registry, the manifest, and the skills.
func phase8Overlay(t *testing.T, w phase8World) map[string]string {
	t.Helper()
	snap := snapshotContents(t, w.overlay)
	for name := range snap {
		if strings.HasSuffix(name, ".lock") {
			delete(snap, name)
		}
	}
	return snap
}

// phase8Same fails when two snapshots differ, naming what differs.
func phase8Same(t *testing.T, label string, before, after map[string]string) {
	t.Helper()
	if reflect.DeepEqual(before, after) {
		return
	}
	var diffs []string
	for name, content := range after {
		if old, ok := before[name]; !ok {
			diffs = append(diffs, "new: "+name)
		} else if old != content {
			diffs = append(diffs, "changed: "+name)
		}
	}
	for name := range before {
		if _, ok := after[name]; !ok {
			diffs = append(diffs, "gone: "+name)
		}
	}
	sort.Strings(diffs)
	t.Errorf("%s changed files: %v", label, diffs)
}

// phase8ProjectLock is the project lock file as the detector would find it.
func phase8ProjectLock(t *testing.T, project string) []byte {
	t.Helper()
	return phase8Read(t, filepath.Join(project, filepath.FromSlash(skills.ProjectLockRelPath)))
}

type phase8InstallRecord struct {
	ID    string `json:"id"`
	Files []struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
	} `json:"files"`
}

func phase8Installs(t *testing.T, project string) []phase8InstallRecord {
	t.Helper()
	var lock struct {
		Installs []phase8InstallRecord `json:"installs"`
	}
	if err := json.Unmarshal(phase8ProjectLock(t, project), &lock); err != nil {
		t.Fatalf("project lock is not JSON: %v", err)
	}
	return lock.Installs
}

// --- the acceptance test ----------------------------------------------------

// TestPhase8Acceptance builds the engine binary once and drives it through every
// acceptance criterion of Phase 8. Safety: every process runs with HOME and
// XDG_STATE_HOME set to this test's own temporary directories.
func TestPhase8Acceptance(t *testing.T) {
	binary := phase6BuildEngineBinary(t)
	for _, c := range []struct {
		name string
		run  func(*testing.T, string)
	}{
		{"Lifecycle_DraftToProjectWithDriftAndReportOnlyRetirement", phase8Lifecycle},
		{"Approval_AddNeedsARecordForTheExactBytes", phase8Approval},
		{"Approval_NoSkillIsGrandfathered", phase8NoGrandfathering},
		{"Baseline_ApproveWarnsForSkillsThatPredateTheLintBudget", phase8BaselineLintBudget},
		{"ApproveGuard_DeniesTheAgentAndManagesItsSettingsFamily", phase8ApproveGuard},
		{"Install_OwnsByHashKeepsForeignFilesAndAdoptIsExplicit", phase8Install},
		{"Capabilities_DeclareSkillsPerRuntimeWithTheApplyLimit", phase8Capabilities},
		{"NoExecOrNetwork_InTheFilesPhase8AddedOutsideTheGuardedPackages", phase8NoExecOrNetwork},
	} {
		t.Run(c.name, func(t *testing.T) { c.run(t, binary) })
	}
}

// --- the walk ---------------------------------------------------------------

func phase8Lifecycle(t *testing.T, binary string) {
	w := newPhase8World(t, binary)
	const (
		id        = "tidy-notes" // the global skill that goes through add
		projSkill = "proj-skill" // the project-scoped skill that goes through install
		projectID = "proj-1"     // what install is told the project is
		candidate = "procedural/candidates/repeated-success/tidy-worktree"
	)
	reg := w.registryPath()

	// An overlay that already has one approved global skill.
	w.writeOverlay(phase8Registry(phase8RegistryEntry{id: "base", scope: "global"}), phase8Manifest("base"))
	w.writeSkill("base", phase8SkillMD("base", "Follow the base procedure."))
	w.approve("base", "alice").must(t, "approve the starting skill")
	w.overlayVerb("validate").must(t, "validate the starting overlay")

	// 1. A draft is written OUTSIDE skills/, and lint runs on it there. It is not a
	// skill yet: add cannot find it, and validate does not know it.
	draftBytes := phase8SkillMD(id, "List each decision with its owner.")
	draft := filepath.Join(w.dir, "drafts", id, "SKILL.md")
	writeFixtureFile(t, draft, draftBytes)
	if strings.HasPrefix(draft, w.overlay) {
		t.Fatalf("the draft %s is inside the overlay %s", draft, w.overlay)
	}
	w.skillsVerb(w.dir, "lint", draft).must(t, "lint the draft")
	bad := filepath.Join(w.dir, "drafts", "bad", "SKILL.md")
	writeFixtureFile(t, bad, "no front matter, no sections\n")
	if r := w.skillsVerb(w.dir, "lint", bad); r.code != 1 || r.stderr == "" {
		t.Errorf("lint of a defective draft: exit %d, stderr %q, want exit 1 and a finding", r.code, r.stderr)
	}
	w.overlayVerb("add", id).expect(t, "add a draft that is still outside skills/", 1, "SKILL.md not found")
	w.overlayVerb("validate").must(t, "validate with only a draft outside skills/")

	// 2. A person copies it to skills/<id>/SKILL.md. Copying is not approving: add is
	// refused, and nothing in the overlay changes.
	w.writeSkill(id, draftBytes)
	before := phase8Overlay(t, w)
	w.overlayVerb("add", id).expect(t, "add before approval", 1, "[APPROVAL_MISSING]", `skill "`+id+`"`)
	phase8Same(t, "a refused add", before, phase8Overlay(t, w))

	// 3. Approval: the record names the skill and the digest of its files, which are the one
	// SKILL.md.
	approved := w.approve(id, "alice").must(t, "approve the copied skill")
	digest := phase8ApprovalDigest(draftBytes)
	if !strings.Contains(approved.stdout, "sha256: "+digest) {
		t.Errorf("approve printed %q, want the digest %s of the files", approved.stdout, digest)
	}
	var record struct {
		Version    int    `json:"version"`
		Skill      string `json:"skill"`
		SHA256     string `json:"sha256"`
		ApprovedAt string `json:"approved_at"`
		Approver   string `json:"approver"`
	}
	if err := json.Unmarshal(phase8Read(t, w.recordPath(id)), &record); err != nil {
		t.Fatalf("the approval record is not JSON: %v", err)
	}
	if record.Version != 2 || record.Skill != id || record.SHA256 != digest || record.Approver != "alice" || record.ApprovedAt == "" {
		t.Errorf("approval record = %+v, want version 2, skill %s, digest %s, approver alice, and a time", record, id, digest)
	}

	// 4. add registers it in the registry and the manifest together.
	w.overlayVerb("add", id).must(t, "add the approved skill").expect(t, "add output", 0, "added: "+id)
	listed := w.skillsVerb(w.dir, "list", "--registry", reg).must(t, "list").stdout
	if !strings.Contains(listed, id+"\t") {
		t.Errorf("list does not show %s:\n%s", id, listed)
	}
	if manifest := string(phase8Read(t, w.manifestPath())); !strings.Contains(manifest, id+"/SKILL.md custom") {
		t.Errorf("the manifest has no row for %s:\n%s", id, manifest)
	}

	// 5. Manifest sync. No verb declares a project-scoped overlay entry, so a person
	// adds one to the registry; the registry and the manifest then disagree, and
	// sync-manifest is what makes them agree again.
	writeFixtureFile(t, reg, string(phase8Read(t, reg))+phase8EntryYAML(phase8RegistryEntry{id: projSkill, scope: "project", projects: []string{projectID}}))
	projBytes := []byte(phase8SkillMD(projSkill, "Follow the project procedure."))
	w.writeSkill(projSkill, string(projBytes))
	w.overlayVerb("validate").expect(t, "validate before the manifest sync", 1, "MISSING_IN_MANIFEST", projSkill)
	w.overlayVerb("sync-manifest").expect(t, "sync-manifest", 0, "1 added")
	w.overlayVerb("validate").expect(t, "validate after the manifest sync", 0,
		"registry and manifest aligned (3 skills)", "global skill approvals verified (2 skills: 2 approved)")

	// 6. install puts the project-scoped skill in BOTH runtime directories, byte for
	// byte, records it by hash, and a second run changes nothing.
	w.install(w.project, projectID).expect(t, "install", 0, "installed: "+projSkill)
	for _, runtime := range []string{".claude", ".agents"} {
		if got := phase8Read(t, filepath.Join(w.project, runtime, "skills", projSkill, "SKILL.md")); !bytes.Equal(got, projBytes) {
			t.Errorf("%s/skills/%s/SKILL.md = %q, want the source bytes", runtime, projSkill, got)
		}
	}
	installs := phase8Installs(t, w.project)
	if len(installs) != 1 || installs[0].ID != projSkill || len(installs[0].Files) != 1 ||
		installs[0].Files[0].Path != "SKILL.md" || installs[0].Files[0].SHA256 != phase8Digest(projBytes) {
		t.Errorf("install records = %+v, want one record of %s/SKILL.md with its digest", installs, projSkill)
	}
	afterInstall := snapshotContents(t, w.project)
	w.install(w.project, projectID).expect(t, "second install", 0, "unchanged: "+projSkill)
	phase8Same(t, "a second install", afterInstall, snapshotContents(t, w.project))

	// 7. The procedural tier: register, then revise. Both write both runtime
	// directories and the same lock file install keeps its records in.
	v1 := filepath.Join(w.dir, "drafts", "tidy-worktree.md")
	writeFixtureFile(t, v1, phase8ProceduralDraft("Use when a worktree must be handed over clean."))
	w.skillsVerb(w.dir, "project-register", "--project-root", w.project, "--candidate", candidate, "--registry", reg, v1).
		expect(t, "project-register", 0, "revision: 1")
	for _, runtime := range []string{".claude", ".agents"} {
		if _, err := os.Stat(filepath.Join(w.project, runtime, "skills", "tidy-worktree", "SKILL.md")); err != nil {
			t.Errorf("project-register did not write %s: %v", runtime, err)
		}
	}
	status := func() string {
		t.Helper()
		return w.skillsVerb(w.dir, "project-status", "--project-root", w.project, "--registry", reg).must(t, "project-status").stdout
	}
	if out := status(); !strings.Contains(out, "tidy-worktree rev:1 owner:agent") {
		t.Errorf("project-status after register: %q", out)
	}
	v2 := filepath.Join(w.dir, "drafts", "tidy-worktree-v2.md")
	writeFixtureFile(t, v2, phase8ProceduralDraft("Use when a worktree must be handed over clean, with no stray files."))
	w.skillsVerb(w.dir, "project-revise", "--project-root", w.project, "--candidate", candidate, "--registry", reg, v2).
		expect(t, "project-revise", 0, "revision: 2")
	for _, runtime := range []string{".claude", ".agents"} {
		if got := phase8Read(t, filepath.Join(w.project, runtime, "skills", "tidy-worktree", "SKILL.md")); !bytes.Contains(got, []byte("no stray files")) {
			t.Errorf("project-revise did not update %s: %q", runtime, got)
		}
	}
	if out := status(); !strings.Contains(out, "tidy-worktree rev:2 owner:agent") {
		t.Errorf("project-status after revise: %q", out)
	}
	w.install(w.project, projectID).expect(t, "install after register and revise", 0, "unchanged: "+projSkill)

	// 8. Drift, installed side: a hand-edited installed file is named and protected,
	// and nothing is written; restoring the bytes makes install quiet again.
	agents := filepath.Join(w.project, ".agents", "skills", projSkill, "SKILL.md")
	writeFixtureFile(t, agents, string(projBytes)+"\nEdited by hand.\n")
	frozen := snapshotContents(t, w.project)
	w.install(w.project, projectID).expect(t, "install over a hand edit", 1,
		".agents/skills/"+projSkill+"/SKILL.md", "edited", "nothing was installed")
	phase8Same(t, "a refused install", frozen, snapshotContents(t, w.project))
	writeFixtureFile(t, agents, string(projBytes))
	w.install(w.project, projectID).expect(t, "install after restoring the bytes", 0, "unchanged: "+projSkill)

	// 9. Drift, approved side: a change to the approved SKILL.md makes the record
	// stale, in validate; the approval is bound to the exact bytes, so restoring them
	// makes it valid again.
	w.writeSkill(id, draftBytes+"\nAn unreviewed line.\n")
	w.overlayVerb("validate").expect(t, "validate after editing an approved skill", 1, "[APPROVAL_STALE]", id, "stale")
	w.writeSkill(id, draftBytes)
	w.overlayVerb("validate").must(t, "validate after restoring the approved bytes")

	// 10. Report-only retirement, project tier. The lock the binary wrote is one the
	// detector's strict parser accepts, with procedural skills and install records in
	// it; retirement is a separate, explicit verb; its dry run writes nothing; and
	// retiring one skill leaves the install records alone.
	phase8AssertDetectorAcceptsLock(t, "after install, register, and revise", phase8ProjectLock(t, w.project))
	frozen = snapshotContents(t, w.project)
	w.skillsVerb(w.dir, "project-retire", "--project-root", w.project, "--dry-run", "--reason", "unused for a quarter", "--registry", reg, "tidy-worktree").
		expect(t, "project-retire --dry-run", 0, "plan: .claude/skills/tidy-worktree/SKILL.md", "plan: .agents/skills/tidy-worktree/SKILL.md")
	phase8Same(t, "a dry run of project-retire", frozen, snapshotContents(t, w.project))
	w.skillsVerb(w.dir, "project-retire", "--project-root", w.project, "--reason", "unused for a quarter", "--registry", reg, "tidy-worktree").
		expect(t, "project-retire", 0, "removed: .claude/skills/tidy-worktree/SKILL.md", "removed: .agents/skills/tidy-worktree/SKILL.md")
	for _, runtime := range []string{".claude", ".agents"} {
		if _, err := os.Stat(filepath.Join(w.project, runtime, "skills", "tidy-worktree", "SKILL.md")); !os.IsNotExist(err) {
			t.Errorf("%s/skills/tidy-worktree/SKILL.md survives project-retire (stat: %v)", runtime, err)
		}
	}
	if strings.Contains(string(phase8ProjectLock(t, w.project)), "tidy-worktree") {
		t.Error("the project lock still names tidy-worktree after project-retire")
	}
	phase8AssertDetectorAcceptsLock(t, "after retire", phase8ProjectLock(t, w.project))
	if got := phase8Installs(t, w.project); len(got) != 1 || got[0].ID != projSkill {
		t.Errorf("install records after retire = %+v, want the record of %s untouched", got, projSkill)
	}
	w.install(w.project, projectID).expect(t, "install after retire", 0, "unchanged: "+projSkill)
}

// phase8ProceduralDraft is a procedural skill draft for project-register and
// project-revise; the activation text is what the two revisions differ in.
func phase8ProceduralDraft(activation string) string {
	return "---\n" +
		"name: tidy-worktree\n" +
		"description: Tidy a git worktree before handing it to a reviewer.\n" +
		"license: Apache-2.0\n" +
		"metadata:\n" +
		"  author: someone\n" +
		"  version: 1.0.0\n" +
		"---\n" +
		"\n" +
		"## Activation Contract\n" +
		"\n" +
		activation + "\n"
}

// phase8DetectorLockKeys reads the JSON field names the retirement detector's strict
// lock parser accepts, from the detector's own source (longterm-mem, a separate module
// with an internal package, so it cannot be imported): the tags of ProjectLock and of
// ProjectLockEntry, wherever in the internal/skillstale package they are declared, so a
// type moving between the package's files does not lose them.
func phase8DetectorLockKeys(t *testing.T) (top, entry map[string]bool) {
	t.Helper()
	topKeys, entryKeys, err := phase8LockKeysIn(filepath.Join("..", "..", "longterm-mem", "internal", "skillstale"))
	if err != nil {
		t.Fatal(err)
	}
	return topKeys, entryKeys
}

// phase8LockKeysIn is the reading behind phase8DetectorLockKeys, over the package in src. It refuses a
// directory with no non-test Go source (a path that moved or a glob that matched nothing would otherwise
// show only as keys that are missing), and a package that declares ProjectLock or ProjectLockEntry twice
// (under a build tag, say), because the later declaration would silently replace the earlier one.
func phase8LockKeysIn(src string) (top, entry map[string]bool, err error) {
	sources, err := filepath.Glob(filepath.Join(src, "*.go"))
	if err != nil {
		return nil, nil, fmt.Errorf("list the detector's sources in %s: %w", src, err)
	}
	var decls []ast.Decl
	parsed := 0
	for _, path := range sources {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return nil, nil, fmt.Errorf("parse the detector's source %s: %w", path, err)
		}
		parsed++
		decls = append(decls, file.Decls...)
	}
	if parsed == 0 {
		return nil, nil, fmt.Errorf("no non-test Go source in %s; did the detector move?", src)
	}
	for _, decl := range decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			typeSpec := spec.(*ast.TypeSpec)
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}
			keys := map[string]bool{}
			for _, field := range structType.Fields.List {
				if field.Tag == nil {
					continue
				}
				tag, err := strconv.Unquote(field.Tag.Value)
				if err != nil {
					return nil, nil, fmt.Errorf("tag %s: %w", field.Tag.Value, err)
				}
				if name := strings.Split(reflect.StructTag(tag).Get("json"), ",")[0]; name != "" && name != "-" {
					keys[name] = true
				}
			}
			switch typeSpec.Name.Name {
			case "ProjectLock":
				if top != nil {
					return nil, nil, fmt.Errorf("ProjectLock is declared more than once in %s", src)
				}
				top = keys
			case "ProjectLockEntry":
				if entry != nil {
					return nil, nil, fmt.Errorf("ProjectLockEntry is declared more than once in %s", src)
				}
				entry = keys
			}
		}
	}
	if len(top) == 0 || len(entry) == 0 {
		return nil, nil, fmt.Errorf("found no json fields for ProjectLock (%v) or ProjectLockEntry (%v) in %s; did the detector move?", top, entry, src)
	}
	return top, entry, nil
}

// phase8AssertDetectorAcceptsLock restates the detector's strict parse for a lock file
// the engine wrote: only fields it declares (the installs array included), version 1,
// unique skill ids, and a first target for every procedural skill.
func phase8AssertDetectorAcceptsLock(t *testing.T, label string, data []byte) {
	t.Helper()
	top, entry := phase8DetectorLockKeys(t)
	for _, problem := range phase8DetectorLockProblems(data, top, entry) {
		t.Errorf("%s: %s", label, problem)
	}
}

// phase8DetectorLockProblems returns one line for each way the detector would refuse or
// misread the lock in data, given the field names it declares (top for ProjectLock, entry
// for ProjectLockEntry). It is a function of its inputs, with no *testing.T, so that a
// test can hand it a lock the detector would reject and see that it says so: a value of
// the wrong type for "id" or "targets" makes the detector's typed decode fail, so it is a
// problem here too, never a value silently read as empty.
func phase8DetectorLockProblems(data []byte, top, entry map[string]bool) []string {
	var problems []string
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		return []string{"the project lock is not JSON: " + err.Error()}
	}
	for key := range doc {
		if !top[key] {
			problems = append(problems, "the lock has the field "+strconv.Quote(key)+", which the detector's ProjectLock does not declare, so its strict parser would refuse the file")
		}
	}
	var version int
	if err := json.Unmarshal(doc["version"], &version); err != nil || version != 1 {
		problems = append(problems, "lock version = "+string(doc["version"])+", want 1")
	}
	var procedural []map[string]json.RawMessage
	if err := json.Unmarshal(doc["skills"], &procedural); err != nil {
		return append(problems, "skills: "+err.Error())
	}
	seen := map[string]bool{}
	for i, item := range procedural {
		for key := range item {
			if !entry[key] {
				problems = append(problems, "skill entry "+strconv.Itoa(i)+" has the field "+strconv.Quote(key)+", which the detector's ProjectLockEntry does not declare")
			}
		}
		var skillID string
		if err := json.Unmarshal(item["id"], &skillID); err != nil {
			problems = append(problems, "skill entry "+strconv.Itoa(i)+": id does not decode as the detector's string: "+err.Error())
		}
		var targets []string
		if err := json.Unmarshal(item["targets"], &targets); err != nil {
			problems = append(problems, "skill entry "+strconv.Itoa(i)+" ("+strconv.Quote(skillID)+"): targets do not decode as the detector's list of strings: "+err.Error())
		}
		if seen[skillID] {
			problems = append(problems, "skill id "+strconv.Quote(skillID)+" appears twice; the detector refuses duplicates")
		}
		seen[skillID] = true
		if len(targets) == 0 {
			problems = append(problems, "skill "+strconv.Quote(skillID)+" has no targets; the detector reads the first one")
		}
	}
	return problems
}

func TestPhase8DetectorLockProblems_SaysWhatTheDetectorWouldRefuse(t *testing.T) {
	top := map[string]bool{"version": true, "skills": true, "installs": true}
	entry := map[string]bool{"id": true, "provenance": true, "candidate": true, "sha256": true, "revision": true, "targets": true}
	const good = `{"version":1,"skills":[{"id":"a","provenance":"p","candidate":"c","sha256":"s","revision":1,"targets":[".claude/skills/a/SKILL.md"]}],"installs":[]}`

	if problems := phase8DetectorLockProblems([]byte(good), top, entry); len(problems) != 0 {
		t.Fatalf("a lock the detector accepts got problems: %v", problems)
	}
	for _, tc := range []struct {
		name string
		lock string
		want string // a substring of one problem
	}{
		{"not JSON", `{`, "not JSON"},
		{"a field the detector does not declare", `{"version":1,"skills":[],"extra":1}`, `"extra"`},
		{"an unsupported version", `{"version":2,"skills":[]}`, "want 1"},
		{"a skill field the detector does not declare", `{"version":1,"skills":[{"id":"a","targets":["t"],"extra":1}]}`, `"extra"`},
		{"an id that is not a string", `{"version":1,"skills":[{"id":5,"targets":["t"]}]}`, "id does not decode"},
		{"a skill with no id", `{"version":1,"skills":[{"targets":["t"]}]}`, "id does not decode"},
		{"targets that are not a list", `{"version":1,"skills":[{"id":"a","targets":"t"}]}`, "targets do not decode"},
		{"a skill with no targets key", `{"version":1,"skills":[{"id":"a"}]}`, "targets do not decode"},
		{"a skill with an empty targets list", `{"version":1,"skills":[{"id":"a","targets":[]}]}`, "has no targets"},
		{"a duplicate id", `{"version":1,"skills":[{"id":"a","targets":["t"]},{"id":"a","targets":["t"]}]}`, "appears twice"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			problems := phase8DetectorLockProblems([]byte(tc.lock), top, entry)
			if len(problems) == 0 {
				t.Fatalf("a lock the detector would reject got no problem: %s", tc.lock)
			}
			if !strings.Contains(strings.Join(problems, "\n"), tc.want) {
				t.Errorf("problems %q do not contain %q", problems, tc.want)
			}
		})
	}
}

// --- approval ----------------------------------------------------------------

func phase8Approval(t *testing.T, binary string) {
	w := newPhase8World(t, binary)
	w.writeOverlay(phase8Registry(phase8RegistryEntry{id: "base", scope: "global"}), phase8Manifest("base"))
	w.writeSkill("base", phase8SkillMD("base", "Follow the base procedure."))
	w.approve("base", "alice").must(t, "approve the starting skill")
	listed := func() string {
		t.Helper()
		return w.skillsVerb(w.dir, "list", "--registry", w.registryPath()).must(t, "list").stdout
	}

	// No record: refused, and nothing is written.
	w.writeSkill("gated", phase8SkillMD("gated", "Follow the gated procedure."))
	before := phase8Overlay(t, w)
	w.overlayVerb("add", "gated").expect(t, "add without a record", 1, "[APPROVAL_MISSING]", `skill "gated"`, "skills approve --id gated")
	phase8Same(t, "add without a record", before, phase8Overlay(t, w))
	if strings.Contains(listed(), "gated") {
		t.Error("a refused add registered the skill")
	}

	// A record that does not parse, and one that names another skill, are refused too:
	// a governance file that is present must be valid.
	writeFixtureFile(t, w.recordPath("gated"), "{ not json")
	before = phase8Overlay(t, w)
	w.overlayVerb("add", "gated").expect(t, "add with a malformed record", 1, "[APPROVAL_MALFORMED]")
	phase8Same(t, "add with a malformed record", before, phase8Overlay(t, w))
	writeFixtureFile(t, w.recordPath("gated"), string(phase8Read(t, w.recordPath("base"))))
	before = phase8Overlay(t, w)
	w.overlayVerb("add", "gated").expect(t, "add with another skill's record", 1, "[APPROVAL_MALFORMED]")
	phase8Same(t, "add with another skill's record", before, phase8Overlay(t, w))
	if err := os.Remove(w.recordPath("gated")); err != nil {
		t.Fatal(err)
	}

	// approve needs a label for the human: no default, no record without one.
	w.overlayVerb("approve", "--id", "gated").expect(t, "approve without --approver", 1, "--approver")
	if _, err := os.Stat(w.recordPath("gated")); !os.IsNotExist(err) {
		t.Errorf("approve without --approver wrote a record (stat: %v)", err)
	}

	// A record for the exact bytes: add succeeds.
	w.approve("gated", "alice").must(t, "approve gated")
	w.overlayVerb("add", "gated").expect(t, "add with a matching record", 0, "added: gated")
	if !strings.Contains(listed(), "gated\t") {
		t.Errorf("add with a record did not register the skill:\n%s", listed())
	}
	w.overlayVerb("validate").must(t, "validate after the add")

	// Changing the file after approval invalidates the record. A change of one byte,
	// a newline, is enough: the record is bound to the bytes, not to the meaning.
	edited := phase8SkillMD("edited", "Follow the edited procedure.")
	w.writeSkill("edited", edited)
	w.approve("edited", "alice").must(t, "approve edited")
	w.writeSkill("edited", edited+"\n")
	before = phase8Overlay(t, w)
	w.overlayVerb("add", "edited").expect(t, "add after editing an approved skill", 1, "[APPROVAL_STALE]", `skill "edited"`)
	phase8Same(t, "add after editing an approved skill", before, phase8Overlay(t, w))
	w.approve("edited", "alice").must(t, "approve the edited bytes")
	w.overlayVerb("add", "edited").expect(t, "add after approving the new bytes", 0, "added: edited")
}

// --- no skill is grandfathered ---------------------------------------------------

// phase8NoGrandfathering holds that a global skill is excused from approval for nothing it is:
// the id of a skill that predates the approval record, and the bytes it had then, need a record
// like any other skill's. (Until Phase 9 the skills that existed at the Phase 8 base were
// exempt while their SKILL.md was unchanged.)
func phase8NoGrandfathering(t *testing.T, binary string) {
	const formerlyGrandfathered = baselineSkillID
	w := newPhase8World(t, binary)
	w.writeOverlay(
		phase8Registry(
			phase8RegistryEntry{id: formerlyGrandfathered, scope: "global"},
			phase8RegistryEntry{id: "outsider", scope: "global"},
		),
		phase8Manifest(formerlyGrandfathered, "outsider"))
	w.writeSkill(formerlyGrandfathered, phase8SkillMD(formerlyGrandfathered, "Follow the old procedure."))
	w.writeSkill("outsider", phase8SkillMD("outsider", "Follow the outsider procedure."))

	// Without any record: neither skill validates, and neither is told it is excused.
	r := w.overlayVerb("validate").expect(t, "validate", 1,
		"[APPROVAL_MISSING] "+formerlyGrandfathered, "[APPROVAL_MISSING] outsider")
	if strings.Contains(r.stderr+r.stdout, "grandfathered") {
		t.Errorf("validate speaks of a grandfathered skill:\n%s%s", r.stdout, r.stderr)
	}

	w.approve(formerlyGrandfathered, "alice").must(t, "approve the skill that used to be excused")
	w.approve("outsider", "alice").must(t, "approve outsider")
	r = w.overlayVerb("validate").expect(t, "validate with both approved", 0, "(2 skills: 2 approved)")
	if strings.Contains(r.stdout, "grandfathered") {
		t.Errorf("the summary of validate still counts grandfathered skills: %q", r.stdout)
	}
}

// --- skills of the lint baseline, which predate the lint budget ------------------

// phase8LintBaselineSkill finds a skill of this repository that is on the lint baseline and whose
// SKILL.md fails the hard lint rules, as most of them do today, and returns its id and bytes. It
// is the one place this file reads the repository's own skills/ tree, read-only: the real bytes
// are copied into an overlay of the test's own, and nothing of what is read is required to be
// approved. The lint baseline is asked as approve asks it (skills.BaselineLintDecision). ok is
// false when no skill qualifies, and the caller decides whether that skips the test.
func phase8LintBaselineSkill(t *testing.T) (id string, data []byte, ok bool) {
	t.Helper()
	tree := filepath.Join("..", "..", "skills")
	dirs, err := os.ReadDir(tree)
	if err != nil {
		return "", nil, false
	}
	for _, dir := range dirs {
		data, err := os.ReadFile(filepath.Join(tree, dir.Name(), "SKILL.md"))
		if err != nil {
			continue
		}
		if hard, _ := skills.LintSkillFile(data); len(hard) > 0 {
			if _, refused := skills.BaselineLintDecision(dir.Name(), hard); !refused {
				return dir.Name(), data, true
			}
		}
	}
	return "", nil, false
}

// --- baseline skills that predate the lint budget ------------------------------

// phase8BaselineWarningSuffix is the tail of every warning line approve prints for a
// baseline skill; the pin is in this file on purpose, apart from the code that prints it.
const phase8BaselineWarningSuffix = " (baseline skill: approved with lint findings)"

// phase8OverBudgetSkillMD is a SKILL.md that fails the hard lint only because its body
// is past the token budget, the way most of the baseline skills do.
func phase8OverBudgetSkillMD(name string) string {
	return phase8SkillMD(name, "Follow the procedure.") + strings.Repeat("Another line of the procedure, long enough to count.\n", 100)
}

// phase8BaselineWarnings is what approve must print for md: one line per hard finding.
func phase8BaselineWarnings(t *testing.T, md string) string {
	t.Helper()
	hard, _ := skills.LintSkillFile([]byte(md))
	if len(hard) == 0 {
		t.Fatalf("the fixture must fail the hard lint:\n%s", md)
	}
	var sb strings.Builder
	for _, finding := range hard {
		sb.WriteString("warning: " + finding.Error() + phase8BaselineWarningSuffix + "\n")
	}
	return sb.String()
}

// phase8BaselineLintBudget is the scenario of the skills of the lint baseline that fail the
// hard lint (most of them): nothing excuses them from approval, validate asks for it, and
// approve must be able to give it, again after an upstream merge changes their files. A
// skill outside the lint baseline gets no such allowance.
func phase8BaselineLintBudget(t *testing.T, binary string) {
	t.Run("a skill of the lint baseline that fails the lint is approved with warnings", func(t *testing.T) {
		w := newPhase8World(t, binary)
		baseID := baselineSkillID
		md := phase8OverBudgetSkillMD(baseID)
		wantWarnings := phase8BaselineWarnings(t, md)
		w.writeOverlay(phase8Registry(phase8RegistryEntry{id: baseID, scope: "global"}), phase8Manifest(baseID))
		w.writeSkill(baseID, md)

		// validate asks for the approval, as it does for any global skill...
		w.overlayVerb("validate").expect(t, "validate a baseline skill", 1, "[APPROVAL_MISSING] "+baseID)

		// ...a skill outside the lint baseline with the very same finding is refused, and no
		// record is written for it...
		strayRoot := filepath.Join(w.dir, "outside", "skills")
		writeFixtureFile(t, filepath.Join(strayRoot, "outsider", "SKILL.md"), phase8OverBudgetSkillMD("outsider"))
		refused := w.skillsVerb(w.dir, "approve", "--id", "outsider", "--approver", "alice", "--source-root", strayRoot, "--registry", w.registryPath()).
			expect(t, "approve outside the lint baseline", 1, "[lint:body-hard-budget]")
		if strings.Contains(refused.stderr, "warning:") {
			t.Errorf("a skill outside the lint baseline was warned about instead of refused: %q", refused.stderr)
		}
		if _, err := os.Stat(filepath.Join(strayRoot, "outsider", skills.ApprovalRecordName)); !os.IsNotExist(err) {
			t.Errorf("a refused approve wrote a record (stat: %v)", err)
		}

		// ...and the baseline skill is approved: exit 0, one warning per finding on stderr,
		// a record of the version that covers every file of the skill.
		approved := w.approve(baseID, "alice").expect(t, "approve a baseline skill with lint findings", 0, "approved: "+baseID)
		if approved.stderr != wantWarnings {
			t.Errorf("approve printed %q on stderr, want %q", approved.stderr, wantWarnings)
		}
		var record struct {
			Version  int    `json:"version"`
			Skill    string `json:"skill"`
			SHA256   string `json:"sha256"`
			Approver string `json:"approver"`
		}
		if err := json.Unmarshal(phase8Read(t, w.recordPath(baseID)), &record); err != nil {
			t.Fatalf("the approval record is not JSON: %v", err)
		}
		if record.Version != 2 || record.Skill != baseID || record.SHA256 != phase8ApprovalDigest(md) || record.Approver != "alice" {
			t.Errorf("approval record = %+v, want version 2, skill %s, the digest of its files, approver alice", record, baseID)
		}
		w.overlayVerb("validate").expect(t, "validate after approving", 0, "(1 skills: 1 approved)")
	})

	// The same path with the real bytes of a skill of the lint baseline that fails the lint,
	// then changed by one byte as a merge would change them. The bytes come from this
	// repository's own skills/ tree and are copied into an overlay of the test's own; when no
	// skill of the lint baseline fails the lint any more (they were all rewritten since), the
	// synthetic step above is the coverage.
	t.Run("a real skill of the lint baseline that fails the lint, approved and then changed", func(t *testing.T) {
		id, data, ok := phase8LintBaselineSkill(t)
		if !ok {
			t.Skip("no skill of the lint baseline fails the hard lint rules any more")
		}
		w := newPhase8World(t, binary)
		w.writeOverlay(phase8Registry(phase8RegistryEntry{id: id, scope: "global"}), phase8Manifest(id))
		w.writeSkill(id, string(data))
		wantWarnings := phase8BaselineWarnings(t, string(data))

		w.overlayVerb("validate").expect(t, "validate the skill with no record", 1, "[APPROVAL_MISSING] "+id)
		r := w.approve(id, "alice").expect(t, "approve the skill", 0, "approved: "+id)
		if r.stderr != wantWarnings {
			t.Errorf("approve of the skill printed %q on stderr, want %q", r.stderr, wantWarnings)
		}
		w.overlayVerb("validate").expect(t, "validate the approved skill", 0, "(1 skills: 1 approved)")

		changed := string(data) + "\n"
		w.writeSkill(id, changed)
		w.overlayVerb("validate").expect(t, "validate after the bytes changed", 1, "[APPROVAL_STALE] "+id)
		r = w.approve(id, "alice").expect(t, "approve the changed skill", 0, "approved: "+id)
		if r.stderr != phase8BaselineWarnings(t, changed) {
			t.Errorf("approve of the changed skill printed %q on stderr, want its findings as warnings", r.stderr)
		}
		w.overlayVerb("validate").expect(t, "validate after approving the change", 0, "(1 skills: 1 approved)")
	})
}

// --- the approve guard ---------------------------------------------------------

// phase8HookEntry is one entry of a Claude Code settings file.
func phase8HookEntry(matcher, command string) map[string]any {
	e := map[string]any{"hooks": []any{map[string]any{"type": "command", "command": command}}}
	if matcher != "" {
		e["matcher"] = matcher
	}
	return e
}

// phase8GuardEntries lists the PreToolUse entries of doc that run 'skills guard-hook'
// of the binary at hookCommand, in file order.
func phase8GuardEntries(t *testing.T, doc map[string]any, hookCommand string) []phase7InstalledHook {
	t.Helper()
	var out []phase7InstalledHook
	hooks, _ := doc["hooks"].(map[string]any)
	entries, _ := hooks["PreToolUse"].([]any)
	for _, e := range entries {
		entry, _ := e.(map[string]any)
		inner, _ := entry["hooks"].([]any)
		for _, h := range inner {
			command, _ := h.(map[string]any)["command"].(string)
			if strings.Contains(command, hookCommand+" skills guard-hook") {
				matcher, _ := entry["matcher"].(string)
				out = append(out, phase7InstalledHook{"PreToolUse", matcher, command})
			}
		}
	}
	return out
}

func phase8ApproveGuard(t *testing.T, binary string) {
	w := newPhase8World(t, binary)
	hook := func(tool string, input map[string]any) phase7Run {
		t.Helper()
		return w.engine(w.dir, gateInput(t, w.project, "session-1", tool, input), "skills", "guard-hook")
	}

	// The agent running 'skills approve' is denied, with the reason and the command the
	// person should run, in the JSON Claude Code reads, and exit 0.
	for _, tc := range []struct{ label, command string }{
		{"plain", "labdrian skills approve --id tidy-notes --approver alice"},
		{"after a cd", "cd /work/overlay && labdrian skills approve --id tidy-notes --approver alice --source-root skills"},
		{"the installed entry point", "/home/dev/.claude/bin/gentle-ai-overlay skills approve --id tidy-notes --approver alice"},
		{"the overlay wrapper", "labdrian-overlay skills approve --id tidy-notes --approver alice"},
	} {
		r := hook("Bash", map[string]any{"command": tc.command})
		assertDenied(t, "Bash, "+tc.label, r.asHook(), "a human reviewed the exact SKILL.md bytes", "labdrian skills approve --id <id> --approver <name>")
	}
	// Writing the record by hand is denied for every tool that can write a file.
	for _, tc := range []struct{ tool, field string }{
		{"Write", "file_path"}, {"Edit", "file_path"}, {"MultiEdit", "file_path"}, {"NotebookEdit", "notebook_path"},
	} {
		r := hook(tc.tool, map[string]any{tc.field: "/work/overlay/skills/tidy-notes/" + skills.ApprovalRecordName})
		assertDenied(t, tc.tool+" of the record", r.asHook(), skills.ApprovalRecordName, "labdrian skills approve --id <id> --approver <name>")
	}
	// Everything else is allowed, without a word: no output at all, never an explicit
	// allow, which would bypass Claude Code's own permission flow.
	for _, tc := range []struct {
		label, tool string
		input       map[string]any
	}{
		{"go test", "Bash", map[string]any{"command": "go test ./..."}},
		{"another skills verb", "Bash", map[string]any{"command": "labdrian skills validate --source-root skills"}},
		{"lint of a draft", "Bash", map[string]any{"command": "labdrian skills lint drafts/tidy-notes/SKILL.md"}},
		{"a search for the words", "Bash", map[string]any{"command": "rg 'skills approve' README.md"}},
		{"a Write of the skill", "Write", map[string]any{"file_path": "/work/overlay/skills/tidy-notes/SKILL.md", "content": "x"}},
		{"a Write of a doc that spells the verb", "Write", map[string]any{"file_path": "/work/README.md", "content": "run labdrian skills approve"}},
		{"a Read of the record", "Read", map[string]any{"file_path": "/work/overlay/skills/tidy-notes/" + skills.ApprovalRecordName}},
	} {
		assertSilent(t, tc.label, hook(tc.tool, tc.input).asHook())
	}
	// Unusable input is an allow too: the guard never blocks on its own failure.
	for label, stdin := range map[string]string{"not JSON": "not json at all", "empty": ""} {
		assertSilent(t, "stdin "+label, w.engine(w.dir, stdin, "skills", "guard-hook").asHook())
	}

	// The settings family: installs, reports status, repairs drift, and uninstalls,
	// leaving every foreign entry as it found it.
	claudeDir := filepath.Join(w.home, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.json")
	hookCommand := filepath.Join(claudeDir, "bin", "gentle-ai-overlay")
	if err := os.MkdirAll(filepath.Dir(hookCommand), 0o755); err != nil {
		t.Fatal(err)
	}
	// The hook entry point is a link to the built binary. Links are the premise of this
	// part: it skips only where links cannot be made (makeSymlink) and fails on any other
	// error, so Linux, where CI runs, never skips it silently.
	makeSymlink(t, binary, hookCommand)
	// Foreign entries share our event and our matchers; one runs our binary with
	// another verb, and one runs another program with our verb.
	foreign := map[string]any{
		"model":       "opus",
		"permissions": map[string]any{"allow": []any{"Bash(ls:*)"}},
		"hooks": map[string]any{
			"Notification": []any{phase8HookEntry("", "/opt/other/notify --loud && true")},
			"PreToolUse": []any{
				phase8HookEntry("Bash", "/opt/other/bash-guard"),
				phase8HookEntry("Bash", hookCommand+" some other verb"),
				phase8HookEntry("Bash", "/opt/other/tool skills guard-hook"),
				phase8HookEntry("Write|Edit|MultiEdit|NotebookEdit", "/opt/other/edit-guard"),
			},
		},
	}
	original, err := json.MarshalIndent(foreign, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	merge := func() {
		t.Helper()
		w.engine(w.dir, "", "merge-settings", "--settings", settingsPath, "--hook-command", hookCommand).must(t, "merge-settings")
	}
	statusLine := func() string {
		t.Helper()
		for _, line := range strings.Split(w.engine(w.dir, "", "status").stdout, "\n") {
			if strings.Contains(line, "guard: skills approve") {
				return line
			}
		}
		t.Fatal("status prints no line for the skills approve guard")
		return ""
	}
	assertForeignKept := func(stage string) {
		t.Helper()
		doc := phase7ReadSettings(t, settingsPath)
		hooks, _ := doc["hooks"].(map[string]any)
		for event, entries := range foreign["hooks"].(map[string]any) {
			have := map[string]bool{}
			list, _ := hooks[event].([]any)
			for _, e := range list {
				have[phase7Canonical(t, e)] = true
			}
			for _, e := range entries.([]any) {
				if !have[phase7Canonical(t, e)] {
					t.Errorf("%s: foreign %s entry lost or changed: %s", stage, event, phase7Canonical(t, e))
				}
			}
		}
		permissions, _ := doc["permissions"].(map[string]any)
		if doc["model"] != "opus" || phase7Canonical(t, permissions["allow"]) != `["Bash(ls:*)"]` {
			t.Errorf("%s: foreign top-level keys changed: %s", stage, phase7Canonical(t, doc))
		}
	}

	if line := statusLine(); !strings.HasPrefix(line, "[WARN]") || !strings.Contains(line, "missing or drifted") || !strings.Contains(line, "install-hooks") {
		t.Errorf("status before the merge: %q", line)
	}
	merge()
	assertForeignKept("after the merge")
	installed := phase8GuardEntries(t, phase7ReadSettings(t, settingsPath), hookCommand)
	wantMatchers := []string{"Bash", "Write|Edit|MultiEdit|NotebookEdit"}
	if len(installed) != len(wantMatchers) {
		t.Fatalf("guard entries = %+v, want %d", installed, len(wantMatchers))
	}
	for i, want := range wantMatchers {
		got := installed[i]
		wantCommand := "command -v " + hookCommand + " >/dev/null 2>&1 && " + hookCommand + " skills guard-hook || true"
		if got.matcher != want || got.command != wantCommand {
			t.Errorf("guard entry %d = %+v, want matcher %q and the never-blocking command %q", i, got, want, wantCommand)
		}
	}
	if line := statusLine(); !strings.HasPrefix(line, "[OK  ]") || !strings.Contains(line, "installed") || !strings.Contains(line, "speed bump") {
		t.Errorf("status after the merge: %q", line)
	}

	// The installed commands work as Claude Code runs them: through sh, hook JSON on
	// stdin. A missing binary is a no-op, never a failure.
	runInstalled := func(command, tool string, input map[string]any) hookRun {
		t.Helper()
		return w.run(w.dir, gateInput(t, w.project, "session-1", tool, input), "sh", "-c", command).asHook()
	}
	assertDenied(t, "installed Bash guard", runInstalled(installed[0].command, "Bash", map[string]any{"command": "labdrian skills approve --id x --approver me"}), "human")
	assertSilent(t, "installed Bash guard, go test", runInstalled(installed[0].command, "Bash", map[string]any{"command": "go test ./..."}))
	assertDenied(t, "installed file guard", runInstalled(installed[1].command, "Write", map[string]any{"file_path": "/w/skills/x/" + skills.ApprovalRecordName}), skills.ApprovalRecordName)
	gone := strings.ReplaceAll(installed[0].command, hookCommand, filepath.Join(claudeDir, "bin", "missing"))
	assertSilent(t, "installed Bash guard, binary missing", runInstalled(gone, "Bash", map[string]any{"command": "labdrian skills approve --id x --approver me"}))

	// Idempotent: a second merge changes nothing at all.
	first := phase8Read(t, settingsPath)
	merge()
	if second := phase8Read(t, settingsPath); !bytes.Equal(first, second) {
		t.Errorf("a second merge changed the settings file:\n%s\nvs\n%s", second, first)
	}

	// Drift: an edited entry is reported, and the next merge repairs it to the same
	// bytes without touching a foreign entry.
	doc := phase7ReadSettings(t, settingsPath)
	for _, e := range doc["hooks"].(map[string]any)["PreToolUse"].([]any) {
		for _, h := range e.(map[string]any)["hooks"].([]any) {
			inner := h.(map[string]any)
			if strings.Contains(inner["command"].(string), hookCommand+" skills guard-hook") {
				inner["command"] = inner["command"].(string) + " --extra"
			}
		}
	}
	drifted, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath, drifted, 0o600); err != nil {
		t.Fatal(err)
	}
	if line := statusLine(); !strings.HasPrefix(line, "[WARN]") || !strings.Contains(line, "drifted") {
		t.Errorf("status with a drifted entry: %q", line)
	}
	merge()
	if repaired := phase8Read(t, settingsPath); !phase8SameEntries(t, repaired, first) {
		t.Errorf("the merge did not repair the drift to the installed entries:\n%s\nvs\n%s", repaired, first)
	}
	assertForeignKept("after the repair")

	// Uninstall removes ours and only ours: the document is the foreign one again, a
	// second uninstall changes nothing, and status reports the guard missing.
	w.engine(w.dir, "", "uninstall-hooks", "--settings", settingsPath, "--hook-command", hookCommand).must(t, "uninstall-hooks")
	after := phase7ReadSettings(t, settingsPath)
	if left := phase8GuardEntries(t, after, hookCommand); len(left) != 0 {
		t.Errorf("guard entries survive uninstall: %+v", left)
	}
	if got, want := phase7Canonical(t, after), phase7Canonical(t, foreign); got != want {
		t.Errorf("after uninstall the settings are\n%s\nwant the foreign document\n%s", got, want)
	}
	uninstalled := phase8Read(t, settingsPath)
	w.engine(w.dir, "", "uninstall-hooks", "--settings", settingsPath, "--hook-command", hookCommand).must(t, "second uninstall-hooks")
	if again := phase8Read(t, settingsPath); !bytes.Equal(again, uninstalled) {
		t.Error("a second uninstall changed the settings file")
	}
	if line := statusLine(); !strings.HasPrefix(line, "[WARN]") || !strings.Contains(line, "unguarded") {
		t.Errorf("status after uninstall: %q, want the guard reported missing", line)
	}
}

// --- install ownership ---------------------------------------------------------

func phase8Install(t *testing.T, binary string) {
	w := newPhase8World(t, binary)
	const pid = "p1"
	w.writeOverlay(
		phase8Registry(
			phase8RegistryEntry{id: "other-skill", scope: "project", projects: []string{pid}},
			phase8RegistryEntry{id: "proj-skill", scope: "project", projects: []string{pid}},
		),
		phase8Manifest("other-skill", "proj-skill"))
	source := func(id, rel, content string) {
		t.Helper()
		writeFixtureFile(t, filepath.Join(w.sourceRoot(), id, filepath.FromSlash(rel)), content)
	}
	source("other-skill", "SKILL.md", phase8SkillMD("other-skill", "Follow the other procedure."))
	source("proj-skill", "SKILL.md", phase8SkillMD("proj-skill", "Follow the project procedure."))
	source("proj-skill", "refs/extra.md", "Extra reference.\n")
	runtimes := []string{".claude", ".agents"}
	inProject := func(project, runtime, id, rel string) string {
		return filepath.Join(project, runtime, "skills", id, filepath.FromSlash(rel))
	}
	requireBytes := func(label, path, want string) {
		t.Helper()
		if got, err := os.ReadFile(path); err != nil || string(got) != want {
			t.Errorf("%s: %s = %q (%v), want %q", label, path, got, err, want)
		}
	}

	// Created in both runtime directories, nested files included, each recorded by hash.
	w.install(w.project, pid).expect(t, "first install", 0, "installed: other-skill", "installed: proj-skill")
	for _, runtime := range runtimes {
		requireBytes("first install", inProject(w.project, runtime, "proj-skill", "refs/extra.md"), "Extra reference.\n")
		requireBytes("first install", inProject(w.project, runtime, "other-skill", "SKILL.md"), phase8SkillMD("other-skill", "Follow the other procedure."))
	}
	records := phase8Installs(t, w.project)
	if len(records) != 2 {
		t.Fatalf("install records = %+v, want one per skill", records)
	}
	for _, rec := range records {
		for _, f := range rec.Files {
			if want := phase8Digest(phase8Read(t, filepath.Join(w.sourceRoot(), rec.ID, filepath.FromSlash(f.Path)))); f.SHA256 != want {
				t.Errorf("record of %s/%s has digest %s, want %s", rec.ID, f.Path, f.SHA256, want)
			}
		}
	}

	// Idempotent: nothing changes, and it says so.
	settled := snapshotContents(t, w.project)
	w.install(w.project, pid).expect(t, "second install", 0, "unchanged: other-skill", "unchanged: proj-skill")
	phase8Same(t, "a second install", settled, snapshotContents(t, w.project))

	// A file the install does not own is left alone while it replaces what it owns: the
	// source changes one file, drops one, and adds one.
	note := "A note of mine, kept beside the skill.\n"
	writeFixtureFile(t, inProject(w.project, ".claude", "proj-skill", "notes.txt"), note)
	source("proj-skill", "SKILL.md", phase8SkillMD("proj-skill", "Follow the revised procedure."))
	if err := os.Remove(filepath.Join(w.sourceRoot(), "proj-skill", "refs", "extra.md")); err != nil {
		t.Fatal(err)
	}
	source("proj-skill", "refs/new.md", "New reference.\n")
	w.install(w.project, pid).expect(t, "install after the source changed", 0, "updated: proj-skill", "unchanged: other-skill")
	for _, runtime := range runtimes {
		requireBytes("updated file", inProject(w.project, runtime, "proj-skill", "SKILL.md"), phase8SkillMD("proj-skill", "Follow the revised procedure."))
		requireBytes("added file", inProject(w.project, runtime, "proj-skill", "refs/new.md"), "New reference.\n")
		if _, err := os.Stat(inProject(w.project, runtime, "proj-skill", "refs/extra.md")); !os.IsNotExist(err) {
			t.Errorf("%s: the dropped file survives (stat: %v)", runtime, err)
		}
	}
	requireBytes("the foreign file", inProject(w.project, ".claude", "proj-skill", "notes.txt"), note)

	// A file the source now wants to write where a file of mine already is: refused,
	// naming the path, and the file is left as it is; moved away, the install goes on.
	mine := inProject(w.project, ".claude", "proj-skill", "refs/wanted.md")
	writeFixtureFile(t, mine, "Mine, not installed by skills install.\n")
	source("proj-skill", "refs/wanted.md", "Wanted by the source.\n")
	frozen := snapshotContents(t, w.project)
	w.install(w.project, pid).expect(t, "install over an unrecorded file the source wants", 1,
		".claude/skills/proj-skill/refs/wanted.md", "not recorded as installed", "nothing was installed")
	phase8Same(t, "an install refused over an unrecorded file", frozen, snapshotContents(t, w.project))
	if err := os.Remove(mine); err != nil {
		t.Fatal(err)
	}
	w.install(w.project, pid).expect(t, "install after moving the file away", 0, "updated: proj-skill")
	for _, runtime := range runtimes {
		requireBytes("the wanted file", inProject(w.project, runtime, "proj-skill", "refs/wanted.md"), "Wanted by the source.\n")
	}

	// A hand-edited file it owns is refused, naming the path, and NOTHING is written for
	// any skill, even one that could have been updated: all or nothing per invocation.
	revised := phase8SkillMD("proj-skill", "Follow the revised procedure.")
	writeFixtureFile(t, inProject(w.project, ".agents", "proj-skill", "SKILL.md"), revised+"Edited by hand.\n")
	source("other-skill", "SKILL.md", phase8SkillMD("other-skill", "Follow the changed other procedure."))
	frozen = snapshotContents(t, w.project)
	w.install(w.project, pid).expect(t, "install over a hand edit", 1,
		".agents/skills/proj-skill/SKILL.md", "edited", "nothing was installed")
	phase8Same(t, "a refused install", frozen, snapshotContents(t, w.project))
	writeFixtureFile(t, inProject(w.project, ".agents", "proj-skill", "SKILL.md"), revised)
	w.install(w.project, pid).expect(t, "install after restoring the file", 0, "updated: other-skill", "unchanged: proj-skill")

	// A skill directory it did not install is foreign: refused, naming it and the way to
	// take it over, and nothing is written, not even for the other skill.
	foreignProject := t.TempDir()
	writeFixtureFile(t, inProject(foreignProject, ".claude", "proj-skill", "SKILL.md"), "somebody else's skill\n")
	frozen = snapshotContents(t, foreignProject)
	w.install(foreignProject, pid).expect(t, "install over a foreign directory", 1,
		".claude/skills/proj-skill", "was not installed by skills install", "skills adopt")
	phase8Same(t, "an install refused over a foreign directory", frozen, snapshotContents(t, foreignProject))

	// adopt is the explicit step, and it takes every skill the project is admitted for:
	// it refuses bytes that are not exactly the current skill, naming the file; it takes
	// ownership of directories that are, writing only the lock; and only then does
	// install treat them as its own (and add the other runtime).
	otherBytes := phase8SkillMD("other-skill", "Follow the changed other procedure.")
	writeFixtureFile(t, inProject(foreignProject, ".claude", "other-skill", "SKILL.md"), otherBytes)
	frozen = snapshotContents(t, foreignProject)
	w.adopt(foreignProject, pid).expect(t, "adopt a directory with other bytes", 1,
		".claude/skills/proj-skill is not exactly the current skill", "SKILL.md has other bytes", "nothing was adopted")
	phase8Same(t, "a refused adopt", frozen, snapshotContents(t, foreignProject))
	adoptable := t.TempDir()
	writeFixtureFile(t, inProject(adoptable, ".claude", "proj-skill", "SKILL.md"), revised)
	writeFixtureFile(t, inProject(adoptable, ".claude", "proj-skill", "refs/new.md"), "New reference.\n")
	w.adopt(adoptable, pid).expect(t, "adopt a copy that lacks a file of the source", 1, ".claude/skills/proj-skill is not exactly the current skill", "refs/wanted.md is missing")
	writeFixtureFile(t, inProject(adoptable, ".claude", "proj-skill", "refs/wanted.md"), "Wanted by the source.\n")
	writeFixtureFile(t, inProject(adoptable, ".claude", "other-skill", "SKILL.md"), otherBytes)
	w.install(adoptable, pid).expect(t, "install before adopt", 1, "skills adopt")
	beforeAdopt := snapshotContents(t, adoptable)
	w.adopt(adoptable, pid).expect(t, "adopt the exact copies", 0, "adopted: proj-skill", "adopted: other-skill")
	afterAdopt := snapshotContents(t, adoptable)
	for name := range beforeAdopt {
		if beforeAdopt[name] != afterAdopt[name] {
			t.Errorf("adopt changed %s", name)
		}
	}
	var added []string
	for name := range afterAdopt {
		if _, ok := beforeAdopt[name]; !ok {
			added = append(added, name)
		}
	}
	sort.Strings(added)
	if want := []string{".labdrian", filepath.Join(".labdrian", "procedural-skills.lock.json")}; !reflect.DeepEqual(added, want) {
		t.Errorf("adopt added %v, want only the project lock %v", added, want)
	}
	w.install(adoptable, pid).expect(t, "install after adopt", 0, "updated: proj-skill", "updated: other-skill")
	requireBytes("the other runtime", inProject(adoptable, ".agents", "proj-skill", "SKILL.md"), revised)
	requireBytes("the other runtime", inProject(adoptable, ".agents", "other-skill", "SKILL.md"), otherBytes)
	w.install(adoptable, pid).expect(t, "second install after adopt", 0, "unchanged: proj-skill", "unchanged: other-skill")
}

// --- capabilities ---------------------------------------------------------------

func phase8Capabilities(t *testing.T, binary string) {
	w := newPhase8World(t, binary)
	report := phase7DecodeReport(t, w.engine(w.dir, "", "runtime", "capabilities").must(t, "runtime capabilities").stdout)
	if len(report.Declarations) != 4 {
		t.Fatalf("%d declarations, want one per runtime", len(report.Declarations))
	}
	// Every runtime states a skills claim. All four are partial: no test observes a
	// session loading a skill, and each claim says so and names the tests behind it.
	for _, d := range report.Declarations {
		var claim *capability.Claim
		for i := range d.Claims {
			if d.Claims[i].Capability == capability.Skills {
				claim = &d.Claims[i]
			}
		}
		if claim == nil {
			t.Errorf("%s has no %s claim", d.Target, capability.Skills)
			continue
		}
		if claim.Status != capability.Partial {
			t.Errorf("%s skills is %s, want partial", d.Target, claim.Status)
		}
		if len(claim.Tests) == 0 || claim.Detail == "" {
			t.Errorf("%s skills names %d tests and detail %q, want evidence and a written limit", d.Target, len(claim.Tests), claim.Detail)
		}
		if !strings.Contains(claim.Detail, "observes") {
			t.Errorf("%s skills detail does not say that loading a skill is unobserved: %q", d.Target, claim.Detail)
		}
		if err := capability.CheckEvidence(capabilitytest.NewCatalog(os.DirFS("..")), d); err != nil {
			t.Errorf("%s cites a test that does not exist: %v", d.Target, err)
		}
		// The global tier depends on labdrian apply, not on the engine, for the runtimes
		// apply writes; Pi receives its skills as a package instead, and says so.
		switch d.Target {
		case capability.TargetClaude, capability.TargetCodex, capability.TargetOpenCode:
			if !strings.Contains(claim.Detail, "labdrian apply") {
				t.Errorf("%s skills detail does not state the dependency on labdrian apply: %q", d.Target, claim.Detail)
			}
		case capability.TargetPi:
			if !strings.Contains(claim.Detail, "labdrian-pi package") {
				t.Errorf("pi skills detail does not name the package it receives skills as: %q", claim.Detail)
			}
		}
	}
	// --target selects exactly the declaration `all` prints, skills claim included.
	for _, d := range report.Declarations {
		single := phase7DecodeReport(t, w.engine(w.dir, "", "runtime", "capabilities", "--target", d.Target).must(t, "--target "+d.Target).stdout)
		if len(single.Declarations) != 1 || !reflect.DeepEqual(single.Declarations[0], d) {
			t.Errorf("--target %s printed %+v, want the declaration `all` prints", d.Target, single.Declarations)
		}
	}

	// A bound Claude Code session is told about the limit: its projected context lists
	// skills=partial with the other capabilities that are not fully supported.
	w.bindRunning("proj-1", "wf-1", "odd", "authorize")
	ctx := decodeHookOutput(t, w.promptHook(w.repo).asHook()).Context
	line := contextLineWithPrefix(ctx, "capability limits (claude): ")
	limits := strings.Split(strings.TrimPrefix(line, "capability limits (claude): "), ", ")
	var found bool
	for _, limit := range limits {
		found = found || limit == "skills=partial"
	}
	if !found {
		t.Errorf("the projected context says %q, want it to list skills=partial", line)
	}
}

// --- no exec, no network -----------------------------------------------------------

// phase8NoExecOrNetwork is the import check for the files Phase 8 added or changed in
// the packages that have no allowlist of their own. engine/skills is held by
// TestZeroFetchImportAllowlist and engine/filelock by
// TestProductionFilesImportOnlyTheLockingStdlib, so neither is repeated. engine/pipkg,
// also changed, runs the pi CLI as its adapter always did: its one process evidence
// predates Phase 8 and is not new code.
func phase8NoExecOrNetwork(t *testing.T, _ string) {
	files := []string{
		"../status/status.go", "skills_guard.go", "skills_lock.go",
		"../settings/approve_guard.go", "../settings/family.go", "../settings/projection.go", "../settings/document.go",
		"../capability/capability.go", "../capability/declarations.go",
		"../projection/context.go",
	}
	fset := token.NewFileSet()
	for _, name := range files {
		file, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Errorf("parse %s: %v", name, err)
			continue
		}
		if len(file.Imports) == 0 {
			t.Errorf("%s has no imports: did the file move?", name)
		}
		for _, imp := range file.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if path == "os/exec" || path == "net" || strings.HasPrefix(path, "net/") {
				t.Errorf("%s imports %q: Phase 8 adds no process execution and no network", name, path)
			}
		}
	}
}
