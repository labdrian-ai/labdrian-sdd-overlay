package skills

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// --- fixtures -------------------------------------------------------------

const testCandidateKey = "procedural/candidates/repeated-success/tidy-worktree"

// validDraft is a lint-clean draft SKILL.md: every hard lint rule
// (frontmatter-fence, required-fields, description-one-line, description-max,
// body-hard-budget) passes, and its frontmatter carries only allowlisted
// top-level keys. Tests mutate a copy of it to exercise one refusal each.
func validDraft(name string) []byte {
	return []byte("---\n" +
		"name: " + name + "\n" +
		"description: Tidy a git worktree before handing it to a reviewer.\n" +
		"license: Apache-2.0\n" +
		"metadata:\n" +
		"  author: someone\n" +
		"  version: 1.0.0\n" +
		"---\n" +
		"\n" +
		"## Activation Contract\n" +
		"\n" +
		"Use when a worktree must be handed over clean.\n")
}

// registerInput builds a RegisterInput whose every field is valid, over a
// real (empty) project root created with t.TempDir. The draft lives OUTSIDE
// that root, as step 2 requires. Nothing here touches $HOME, the live
// .claude/skills, .agents/skills, .pi or skills-lock.json.
func registerInput(t *testing.T, id string) RegisterInput {
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
	data := validDraft(id)
	if err := os.WriteFile(draftPath, data, 0o644); err != nil {
		t.Fatalf("write draft: %v", err)
	}

	return RegisterInput{
		ProjectRoot:  root,
		DraftPath:    draftPath,
		DraftData:    data,
		CandidateKey: testCandidateKey,
		Registry:     Registry{Version: "1"},
		Stat:         os.Stat,
		ResolvePath:  resolvePathKeepingMissing,
	}
}

func planRelPaths(p ProjectPlan) []string {
	rels := make([]string, 0, len(p.Writes)+1)
	for _, w := range p.Writes {
		rels = append(rels, w.Rel)
	}
	return append(rels, p.Lock.Rel)
}

// --- the happy path -------------------------------------------------------

// TestPlanProjectRegister_PlansEveryTarget is the positive case: a valid
// registration plans one SKILL.md per fixed target directory plus the lock,
// with the hash of the STAMPED bytes, revision 1, and never a .pi/ path.
func TestPlanProjectRegister_PlansEveryTarget(t *testing.T) {
	const id = "tidy-worktree"
	in := registerInput(t, id)

	plan, err := PlanProjectRegister(in)
	if err != nil {
		t.Fatalf("unexpected refusal: %v", err)
	}

	if plan.ID != id {
		t.Errorf("plan.ID = %q, want %q", plan.ID, id)
	}
	if plan.Revision != 1 {
		t.Errorf("plan.Revision = %d, want 1 for a first registration", plan.Revision)
	}
	if len(plan.Writes) != len(projectTargets) {
		t.Fatalf("plan.Writes = %d, want one per target (%d)", len(plan.Writes), len(projectTargets))
	}

	wantRels := []string{
		".claude/skills/" + id + "/SKILL.md",
		".agents/skills/" + id + "/SKILL.md",
	}
	for i, want := range wantRels {
		w := plan.Writes[i]
		if w.Rel != want {
			t.Errorf("Writes[%d].Rel = %q, want %q", i, w.Rel, want)
		}
		if w.Abs != filepath.Join(in.ProjectRoot, filepath.FromSlash(want)) {
			t.Errorf("Writes[%d].Abs = %q, want it under the project root", i, w.Abs)
		}
		if w.Backup != nil {
			t.Errorf("Writes[%d].Backup = %v, want nil for a new file", i, w.Backup)
		}
	}

	// The bytes written are the STAMPED bytes, and the recorded hash is their
	// hash — the design's "stamp, then lint, then hash, then write" order.
	stamped, err := StampProvenance(in.DraftData, testCandidateKey)
	if err != nil {
		t.Fatalf("stamping the fixture: %v", err)
	}
	for i, w := range plan.Writes {
		if string(w.Data) != string(stamped) {
			t.Errorf("Writes[%d].Data is not the stamped draft", i)
		}
	}
	if plan.SHA256 != HashSkill(stamped) {
		t.Errorf("plan.SHA256 = %q, want the hash of the stamped bytes %q", plan.SHA256, HashSkill(stamped))
	}

	// Lock is a planned write of its own, at the overlay-owned path, and is
	// never skills-lock.json.
	if plan.Lock.Rel != ProjectLockRelPath {
		t.Errorf("plan.Lock.Rel = %q, want %q", plan.Lock.Rel, ProjectLockRelPath)
	}
	if plan.Lock.Backup != nil {
		t.Errorf("plan.Lock.Backup = %v, want nil when no lock file existed", plan.Lock.Backup)
	}
	lock, err := ParseProjectLock(plan.Lock.Data)
	if err != nil {
		t.Fatalf("planned lock bytes do not parse: %v", err)
	}
	if len(lock.Skills) != 1 {
		t.Fatalf("planned lock holds %d entries, want 1", len(lock.Skills))
	}
	e := lock.Skills[0]
	if e.ID != id || e.SHA256 != plan.SHA256 || e.Revision != 1 {
		t.Errorf("lock entry = %+v, want id/sha256/revision to match the plan", e)
	}
	if e.Provenance != "procedural" || e.Candidate != testCandidateKey {
		t.Errorf("lock entry provenance/candidate = %q/%q", e.Provenance, e.Candidate)
	}
	if len(e.Targets) != len(wantRels) {
		t.Fatalf("lock entry records %d targets, want %d", len(e.Targets), len(wantRels))
	}
	for i, want := range wantRels {
		if e.Targets[i] != want {
			t.Errorf("lock entry target %d = %q, want %q", i, e.Targets[i], want)
		}
	}

	if len(plan.Deletes) != 0 {
		t.Errorf("plan.Deletes = %v, want none for a first registration", plan.Deletes)
	}

	// Never .pi/, in any planned path.
	for _, rel := range planRelPaths(plan) {
		if strings.Contains(rel, ".pi/") {
			t.Errorf("planned path %q names .pi/ — this capability never writes it", rel)
		}
		if strings.Contains(rel, "skills-lock.json") {
			t.Errorf("planned path %q names skills-lock.json — never opened by this capability", rel)
		}
	}
}

// TestPlanProjectRegister_FileModeIs0644 pins the mode every planned file
// carries: 0644, never executable (design.md, "Execution"; threat matrix).
func TestPlanProjectRegister_FileModeIs0644(t *testing.T) {
	if ProjectFileMode != fs.FileMode(0o644) {
		t.Errorf("ProjectFileMode = %v, want 0644", ProjectFileMode)
	}
	if ProjectFileMode&0o111 != 0 {
		t.Errorf("ProjectFileMode = %v carries an execute bit", ProjectFileMode)
	}
}

// TestPlanProjectRegister_PreservesExistingLockEntries proves the lock write
// is a merge, not an overwrite: an existing entry survives and the entries
// come back sorted by id, with the pre-existing lock bytes captured as the
// backup that rollback (3b-ii) restores.
func TestPlanProjectRegister_PreservesExistingLockEntries(t *testing.T) {
	in := registerInput(t, "tidy-worktree")
	existing, err := SerializeProjectLock(ProjectLock{Version: 1, Skills: []ProjectLockEntry{{
		ID:         "zzz-other",
		Provenance: "procedural",
		Candidate:  "procedural/candidates/repeated-success/zzz-other",
		SHA256:     "abc",
		Revision:   2,
		Targets:    []string{".claude/skills/zzz-other/SKILL.md"},
	}}})
	if err != nil {
		t.Fatalf("building the existing lock: %v", err)
	}
	in.LockData = existing
	in.LockExists = true

	plan, err := PlanProjectRegister(in)
	if err != nil {
		t.Fatalf("unexpected refusal: %v", err)
	}
	if string(plan.Lock.Backup) != string(existing) {
		t.Error("plan.Lock.Backup must hold the pre-existing lock bytes")
	}
	lock, err := ParseProjectLock(plan.Lock.Data)
	if err != nil {
		t.Fatalf("planned lock bytes do not parse: %v", err)
	}
	if len(lock.Skills) != 2 {
		t.Fatalf("planned lock holds %d entries, want 2", len(lock.Skills))
	}
	if lock.Skills[0].ID != "tidy-worktree" || lock.Skills[1].ID != "zzz-other" {
		t.Errorf("entries = %q/%q, want them sorted by id with the existing one preserved",
			lock.Skills[0].ID, lock.Skills[1].ID)
	}
}

// --- refusals -------------------------------------------------------------

// TestPlanProjectRegister_Refusals covers every refusal design.md names for
// the register path (the validate-before-write list, items 1-11, plus the
// identity rules). Each case mutates exactly one field of an otherwise valid
// input, so a green case proves that one guard and nothing else.
func TestPlanProjectRegister_Refusals(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(t *testing.T, in *RegisterInput)
		wantMsg string
	}{
		{
			name:    "project_root_not_absolute",
			mutate:  func(t *testing.T, in *RegisterInput) { in.ProjectRoot = "relative/project" },
			wantMsg: "must be absolute",
		},
		{
			name: "project_root_does_not_exist",
			mutate: func(t *testing.T, in *RegisterInput) {
				in.ProjectRoot = filepath.Join(in.ProjectRoot, "absent")
			},
			wantMsg: "must exist",
		},
		{
			name: "project_root_is_a_file",
			mutate: func(t *testing.T, in *RegisterInput) {
				f := filepath.Join(in.ProjectRoot, "a-file")
				if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
					t.Fatalf("write: %v", err)
				}
				in.ProjectRoot = f
			},
			wantMsg: "must be a directory",
		},
		{
			name: "draft_inside_the_project_root",
			mutate: func(t *testing.T, in *RegisterInput) {
				inside := filepath.Join(in.ProjectRoot, "draft-SKILL.md")
				if err := os.WriteFile(inside, in.DraftData, 0o644); err != nil {
					t.Fatalf("write: %v", err)
				}
				in.DraftPath = inside
			},
			wantMsg: "outside the project root",
		},
		{
			name: "draft_contains_carriage_returns",
			mutate: func(t *testing.T, in *RegisterInput) {
				in.DraftData = []byte(strings.Replace(string(in.DraftData), "\n", "\r\n", 1))
			},
			wantMsg: "carriage return",
		},
		{
			name: "frontmatter_allowed_tools",
			mutate: func(t *testing.T, in *RegisterInput) {
				in.DraftData = []byte(strings.Replace(string(in.DraftData),
					"license:", "allowed-tools: Bash\nlicense:", 1))
			},
			wantMsg: "allowed-tools",
		},
		{
			name: "frontmatter_disable_model_invocation",
			mutate: func(t *testing.T, in *RegisterInput) {
				in.DraftData = []byte(strings.Replace(string(in.DraftData),
					"license:", "disable-model-invocation: true\nlicense:", 1))
			},
			wantMsg: "disable-model-invocation",
		},
		{
			name: "frontmatter_model",
			mutate: func(t *testing.T, in *RegisterInput) {
				in.DraftData = []byte(strings.Replace(string(in.DraftData),
					"license:", "model: opus\nlicense:", 1))
			},
			wantMsg: "model",
		},
		{
			name: "frontmatter_hooks",
			mutate: func(t *testing.T, in *RegisterInput) {
				in.DraftData = []byte(strings.Replace(string(in.DraftData),
					"license:", "hooks: none\nlicense:", 1))
			},
			wantMsg: "hooks",
		},
		{
			name: "stamped_draft_fails_lint",
			mutate: func(t *testing.T, in *RegisterInput) {
				// Drop the required license field: required-fields is a hard rule.
				in.DraftData = []byte(strings.Replace(string(in.DraftData),
					"license: Apache-2.0\n", "", 1))
			},
			wantMsg: "lint",
		},
		{
			name:    "candidate_key_shape_invalid",
			mutate:  func(t *testing.T, in *RegisterInput) { in.CandidateKey = "procedural/candidates/nope/x" },
			wantMsg: "candidate",
		},
		{
			name:    "lock_does_not_parse",
			mutate:  func(t *testing.T, in *RegisterInput) { in.LockData, in.LockExists = []byte("{"), true },
			wantMsg: "lock",
		},
		{
			name: "lock_version_not_one",
			mutate: func(t *testing.T, in *RegisterInput) {
				in.LockData, in.LockExists = []byte(`{"version":2,"skills":[]}`), true
			},
			wantMsg: "version",
		},
		{
			name: "id_already_in_lock",
			mutate: func(t *testing.T, in *RegisterInput) {
				data, err := SerializeProjectLock(ProjectLock{Version: 1, Skills: []ProjectLockEntry{{
					ID:      "tidy-worktree",
					SHA256:  "abc",
					Targets: []string{".claude/skills/tidy-worktree/SKILL.md"},
				}}})
				if err != nil {
					t.Fatalf("serialize: %v", err)
				}
				in.LockData, in.LockExists = data, true
			},
			wantMsg: "already registered",
		},
		{
			name: "foreign_existing_target_dir",
			mutate: func(t *testing.T, in *RegisterInput) {
				// The live `archify` layout: a directory already sits at the
				// destination and no lock entry claims it.
				dir := filepath.Join(in.ProjectRoot, ".claude", "skills", "tidy-worktree")
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
			},
			wantMsg: "foreign skill",
		},
		{
			name: "traversal_id",
			mutate: func(t *testing.T, in *RegisterInput) {
				in.DraftData = []byte(strings.Replace(string(in.DraftData),
					"name: tidy-worktree", "name: ../../etc", 1))
			},
			wantMsg: "id",
		},
		{
			name: "id_not_normalized",
			mutate: func(t *testing.T, in *RegisterInput) {
				in.DraftData = []byte(strings.Replace(string(in.DraftData),
					"name: tidy-worktree", "name: Tidy--Worktree-", 1))
			},
			wantMsg: "id",
		},
		{
			name: "id_matches_overlay_registry",
			mutate: func(t *testing.T, in *RegisterInput) {
				in.Registry = Registry{Version: "1", Skills: []Entry{{
					ID:   "tidy-worktree",
					Path: "skills/tidy-worktree",
				}}}
			},
			wantMsg: "registry",
		},
		{
			name: "symlinked_claude_escaping_root",
			mutate: func(t *testing.T, in *RegisterInput) {
				outside := filepath.Join(filepath.Dir(in.ProjectRoot), "outside")
				if err := os.MkdirAll(outside, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				if err := os.Symlink(outside, filepath.Join(in.ProjectRoot, ".claude")); err != nil {
					t.Fatalf("symlink: %v", err)
				}
			},
			wantMsg: "escapes",
		},
		{
			name:    "no_resolver_injected",
			mutate:  func(t *testing.T, in *RegisterInput) { in.ResolvePath = nil },
			wantMsg: "resolver",
		},
		{
			name: "lock_target_escapes_root",
			mutate: func(t *testing.T, in *RegisterInput) {
				// A poisoned lock recording a target outside the project must
				// never be carried forward into a write plan (3b-i.5a).
				data, err := SerializeProjectLock(ProjectLock{Version: 1, Skills: []ProjectLockEntry{{
					ID:      "poisoned",
					SHA256:  "abc",
					Targets: []string{"../../etc/passwd"},
				}}})
				if err != nil {
					t.Fatalf("serialize: %v", err)
				}
				in.LockData, in.LockExists = data, true
			},
			wantMsg: "outside the project root",
		},
		{
			name: "lock_target_absolute",
			mutate: func(t *testing.T, in *RegisterInput) {
				data, err := SerializeProjectLock(ProjectLock{Version: 1, Skills: []ProjectLockEntry{{
					ID:      "poisoned",
					SHA256:  "abc",
					Targets: []string{"/etc/passwd"},
				}}})
				if err != nil {
					t.Fatalf("serialize: %v", err)
				}
				in.LockData, in.LockExists = data, true
			},
			wantMsg: "outside the project root",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := registerInput(t, "tidy-worktree")
			tc.mutate(t, &in)

			plan, err := PlanProjectRegister(in)
			if err == nil {
				t.Fatalf("expected a refusal, got a plan with %d writes", len(plan.Writes))
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("refusal %q does not mention %q", err.Error(), tc.wantMsg)
			}
			if len(plan.Writes) != 0 || plan.Lock.Rel != "" {
				t.Errorf("a refusal must return the zero plan, got %+v", plan)
			}
		})
	}
}

// TestPlanProjectRegister_RefusesDestinationUnderSkillsDir is validate step
// 11 (decision (f)): no destination may lie under <root>/skills/. The fixed
// target table cannot produce one today, so the guard is proved against the
// table itself — if a future row ever names skills/, this turns red.
func TestPlanProjectRegister_RefusesDestinationUnderSkillsDir(t *testing.T) {
	for _, target := range projectTargets {
		if underSkillsDir(target.Dir + "/x/SKILL.md") {
			t.Errorf("target %q lies under skills/, which decision (f) forbids", target.Dir)
		}
	}
	if !underSkillsDir("skills/x/SKILL.md") {
		t.Error("underSkillsDir must refuse a destination under skills/")
	}
	if !underSkillsDir("skills/SKILL.md") {
		t.Error("underSkillsDir must refuse a destination directly under skills/")
	}
	if underSkillsDir("skills-other/x/SKILL.md") {
		t.Error("underSkillsDir must not refuse a merely skills-prefixed sibling")
	}
}

// TestProjectTargetsMatchContractTable parses section 10 of the
// procedural-candidate-detection contract and fails unless its Directory rows
// equal the Go projectTargets rows exactly, in order, and neither side names
// .pi/skills (design.md, "Fixed table"). Only the Directory column is
// compared: Runtime(s), Status and Evidence are prose, and the Codex status
// cell is explicitly non-branching.
func TestProjectTargetsMatchContractTable(t *testing.T) {
	repoRoot := ooQualityContractRepoRoot(t)
	content := readRepoFile(t, repoRoot, "skills/_shared/procedural-candidate-detection.md")

	const heading = "## 10. Runtime targets"
	idx := strings.Index(content, heading)
	if idx < 0 {
		t.Fatalf("contract section %q not found", heading)
	}
	section := content[idx+len(heading):]
	if next := strings.Index(section, "\n## "); next >= 0 {
		section = section[:next]
	}

	var dirs []string
	rowIdx := 0
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") {
			continue
		}
		rowIdx++
		if rowIdx <= 2 { // header row, then the |---|---| alignment row
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		if len(cells) != 4 {
			t.Fatalf("contract row %d has %d cells, want 4: %s", rowIdx, len(cells), line)
		}
		dir := strings.TrimSuffix(strings.Trim(strings.TrimSpace(cells[0]), "`"), "/")
		dirs = append(dirs, dir)
	}

	if len(dirs) != len(projectTargets) {
		t.Fatalf("contract lists %d target directories, Go has %d", len(dirs), len(projectTargets))
	}
	for i, dir := range dirs {
		if dir != projectTargets[i].Dir {
			t.Errorf("row %d: contract %q != Go %q", i, dir, projectTargets[i].Dir)
		}
		if strings.Contains(dir, ".pi/skills") {
			t.Errorf("contract row %d names .pi/skills, which is never written", i)
		}
	}
	for i, target := range projectTargets {
		if strings.Contains(target.Dir, ".pi/skills") {
			t.Errorf("Go row %d names .pi/skills, which is never written", i)
		}
	}
}

// TestPlanProjectRegister_RefusesRelativeDraftPath is the write-path half of
// the "draft must lie outside the project root" guard (review round 3, F2 /
// PLAN-1 / SPEC-1). A RELATIVE draft path defeated that guard entirely:
// filepath.Clean does not absolutize, so the lexical withinRoot comparison
// against an absolute root was always false and the resolver returned an
// equally relative path that compared false too — a draft physically sitting
// inside the project root was ACCEPTED and planned. The planner is pure and
// must not consult the process working directory, so the only sound answer is
// to refuse a non-absolute draft outright, exactly as --project-root is
// already required to be absolute.
func TestPlanProjectRegister_RefusesRelativeDraftPath(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, in *RegisterInput) string
	}{
		{
			name: "relative_draft_physically_inside_the_root",
			setup: func(t *testing.T, in *RegisterInput) string {
				if err := os.WriteFile(filepath.Join(in.ProjectRoot, "draft.md"), in.DraftData, 0o644); err != nil {
					t.Fatalf("write: %v", err)
				}
				return "draft.md"
			},
		},
		{
			name: "relative_draft_outside_the_root",
			setup: func(t *testing.T, in *RegisterInput) string {
				return filepath.Join("..", "drafts", "SKILL.md")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := registerInput(t, "tidy-worktree")
			in.DraftPath = tc.setup(t, &in)

			plan, err := PlanProjectRegister(in)
			if err == nil {
				t.Fatalf("expected a refusal for relative draft %q, got a plan with %d writes", in.DraftPath, len(plan.Writes))
			}
			if !strings.Contains(err.Error(), "must be an absolute path") {
				t.Errorf("refusal %q does not name the absolute-path requirement", err.Error())
			}
			if len(plan.Writes) != 0 || plan.Lock.Rel != "" {
				t.Errorf("a refusal must return the zero plan, got %+v", plan)
			}
		})
	}
}

// TestPlanProjectRegister_RefusesDraftResolvingInsideRoot is the SYMLINK half
// of the same guard (review round 3, TQ-2). The only draft case in the
// refusal table is caught lexically, so the resolved check had no covering
// test: here the draft path lies outside the root lexically and reaches a
// file inside it through a symlinked directory.
func TestPlanProjectRegister_RefusesDraftResolvingInsideRoot(t *testing.T) {
	in := registerInput(t, "tidy-worktree")
	base := filepath.Dir(in.ProjectRoot)
	link := filepath.Join(base, "link-to-project")
	if err := os.Symlink(in.ProjectRoot, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	draft := filepath.Join(link, "draft-SKILL.md")
	if err := os.WriteFile(draft, in.DraftData, 0o644); err != nil {
		t.Fatalf("write draft: %v", err)
	}
	in.DraftPath = draft

	// Precondition: the lexical guard alone admits this draft.
	if withinRoot(filepath.Clean(in.ProjectRoot), filepath.Clean(draft)) {
		t.Fatal("precondition: the lexical guard was expected to admit the symlinked draft")
	}

	plan, err := PlanProjectRegister(in)
	if err == nil {
		t.Fatalf("expected a refusal, got a plan with %d writes", len(plan.Writes))
	}
	if !strings.Contains(err.Error(), "resolves inside the project root") {
		t.Errorf("refusal %q does not name the resolved-containment reason", err.Error())
	}
}

// TestPlanProjectRegister_RefusesDanglingSymlinkedTargetDir covers F1: a
// DANGLING symlink at <root>/.claude defeated the resolved containment guard.
// resolvePathKeepingMissing read the ENOENT from filepath.EvalSymlinks as
// "this component merely does not exist yet" and kept it literal, so the link
// was never followed and containment was decided on a path that only LOOKED
// contained. The live-symlink case (in the refusal table) was refused
// correctly; only the dangling one slipped through.
func TestPlanProjectRegister_RefusesDanglingSymlinkedTargetDir(t *testing.T) {
	in := registerInput(t, "tidy-worktree")
	outside := filepath.Join(filepath.Dir(in.ProjectRoot), "outside-not-created-yet")
	if err := os.Symlink(outside, filepath.Join(in.ProjectRoot, ".claude")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Fatalf("precondition: the symlink target must not exist, got %v", err)
	}

	plan, err := PlanProjectRegister(in)
	if err == nil {
		t.Fatalf("expected a refusal for a dangling symlinked .claude, got a plan with %d writes", len(plan.Writes))
	}
	if !strings.Contains(err.Error(), "could not be resolved") {
		t.Errorf("refusal %q does not name the resolution failure", err.Error())
	}
	if len(plan.Writes) != 0 || plan.Lock.Rel != "" {
		t.Errorf("a refusal must return the zero plan, got %+v", plan)
	}
}

// TestPlanProjectRegister_PlannedWritesCarryFileMode covers PLAN-2:
// ProjectFileMode was declared but no planned write carried it, so nothing
// bound the mode the design requires to what the executor will actually
// create.
func TestPlanProjectRegister_PlannedWritesCarryFileMode(t *testing.T) {
	in := registerInput(t, "tidy-worktree")
	plan, err := PlanProjectRegister(in)
	if err != nil {
		t.Fatalf("unexpected refusal: %v", err)
	}
	for i, w := range plan.Writes {
		if w.Mode != ProjectFileMode {
			t.Errorf("Writes[%d].Mode = %v, want %v", i, w.Mode, ProjectFileMode)
		}
	}
	if plan.Lock.Mode != ProjectFileMode {
		t.Errorf("plan.Lock.Mode = %v, want %v", plan.Lock.Mode, ProjectFileMode)
	}
}

// TestPlanProjectRegister_RefusesLockTargetUnderSkillsDir covers PLAN-3 and
// TQ-1 together. The underSkillsDir guard inside resolveWritePath had NO
// covering test — deleting it left the suite green — although tasks.md 3b-i.4
// claims a refusal case for "any destination under <root>/skills/". Reaching
// it through a lock-recorded target also proves PLAN-3: such a refusal used to
// be reported as "outside the project root", which is self-contradicting for a
// target that is plainly inside it.
func TestPlanProjectRegister_RefusesLockTargetUnderSkillsDir(t *testing.T) {
	in := registerInput(t, "tidy-worktree")
	data, err := SerializeProjectLock(ProjectLock{Version: 1, Skills: []ProjectLockEntry{{
		ID:      "sneaky",
		SHA256:  "abc",
		Targets: []string{"skills/sneaky/SKILL.md"},
	}}})
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	in.LockData, in.LockExists = data, true

	plan, err := PlanProjectRegister(in)
	if err == nil {
		t.Fatalf("expected a refusal, got a plan with %d writes", len(plan.Writes))
	}
	if !strings.Contains(err.Error(), "under the project's own skills/ directory") {
		t.Errorf("refusal %q is not the lexical decision-(f) refusal", err.Error())
	}
	if strings.Contains(err.Error(), "resolves into") {
		t.Errorf("refusal %q was decided by the resolved pass, not the lexical one", err.Error())
	}
	if strings.Contains(err.Error(), "outside the project root") {
		t.Errorf("refusal %q calls a target inside the root 'outside the project root'", err.Error())
	}
	if len(plan.Writes) != 0 || plan.Lock.Rel != "" {
		t.Errorf("a refusal must return the zero plan, got %+v", plan)
	}
}

// TestPlanProjectRegister_RefusesWithoutStatProbe covers TQ-4: the
// nil-ResolvePath fail-closed guard was tested while its nil-Stat twin was
// not.
// A destination whose Stat fails for a reason other than "does not exist" —
// a permission denial, say — must refuse the plan. Reading such an error as
// "absent" would fail open: the planner would decide nothing is in the way
// while it simply could not look.
func TestPlanProjectRegister_RefusesUninspectableDestination(t *testing.T) {
	in := registerInput(t, "tidy-worktree")
	realStat := in.Stat
	in.Stat = func(p string) (fs.FileInfo, error) {
		if strings.Contains(p, filepath.Join(".claude", "skills", "tidy-worktree")) {
			return nil, fmt.Errorf("stat %s: %w", p, fs.ErrPermission)
		}
		return realStat(p)
	}

	plan, err := PlanProjectRegister(in)
	if err == nil {
		t.Fatalf("an uninspectable destination must refuse the plan, got %d writes", len(plan.Writes))
	}
	if !strings.Contains(err.Error(), "inspecting destination") {
		t.Fatalf("refusal %q does not name the failed inspection", err)
	}
}

func TestPlanProjectRegister_RefusesWithoutStatProbe(t *testing.T) {
	in := registerInput(t, "tidy-worktree")
	in.Stat = nil

	plan, err := PlanProjectRegister(in)
	if err == nil {
		t.Fatalf("expected a refusal, got a plan with %d writes", len(plan.Writes))
	}
	if !strings.Contains(err.Error(), "stat probe") {
		t.Errorf("refusal %q does not name the missing stat probe", err.Error())
	}
	if len(plan.Writes) != 0 || plan.Lock.Rel != "" {
		t.Errorf("a refusal must return the zero plan, got %+v", plan)
	}
}

// TestCheckRegisterIdentity_BranchMessagesAreDistinct covers TQ-5. The
// refusal table asserted only the substring "id", which every identity
// refusal contains, so `traversal_id` and `id_not_normalized` were
// indistinguishable and the slugRe branch was never reached at all. The
// pattern check now runs FIRST, which makes it reachable, and each branch
// owns one message asserted here in full.
func TestCheckRegisterIdentity_BranchMessagesAreDistinct(t *testing.T) {
	cases := []struct {
		name    string
		id      string
		want    string
		notWant string
	}{
		{
			name:    "empty_id",
			id:      "",
			want:    "the draft frontmatter declares no name",
			notWant: "identifier pattern",
		},
		{
			name:    "traversal_id_fails_the_pattern",
			id:      "../../etc",
			want:    `id "../../etc" does not match the skill identifier pattern`,
			notWant: "is not normalized",
		},
		{
			name:    "uppercase_id_fails_the_pattern",
			id:      "Tidy--Worktree-",
			want:    `id "Tidy--Worktree-" does not match the skill identifier pattern`,
			notWant: "is not normalized",
		},
		{
			name:    "pattern_valid_but_not_normalized",
			id:      "a--b",
			want:    `id "a--b" is not normalized, want "a-b"`,
			notWant: "identifier pattern",
		},
		{
			name:    "trailing_hyphen_is_a_pattern_match_but_not_normalized",
			id:      "a-",
			want:    `id "a-" is not normalized, want "a"`,
			notWant: "identifier pattern",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkRegisterIdentity(tc.id, Registry{Version: "1"})
			if err == nil {
				t.Fatalf("expected a refusal for id %q", tc.id)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("refusal %q does not carry %q", err.Error(), tc.want)
			}
			if strings.Contains(err.Error(), tc.notWant) {
				t.Errorf("refusal %q also carries the other branch's wording %q", err.Error(), tc.notWant)
			}
		})
	}
}

// TestPlanProjectRegister_IdentityBranchesThroughThePlanner proves both
// identity branches are reachable end-to-end through PlanProjectRegister with
// the SAME distinct messages, so the planner and the helper cannot drift
// (TQ-5).
func TestPlanProjectRegister_IdentityBranchesThroughThePlanner(t *testing.T) {
	cases := []struct {
		name, id, want string
	}{
		{"traversal_id", "../../etc", "does not match the skill identifier pattern"},
		{"not_normalized_id", "a--b", `is not normalized, want "a-b"`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := registerInput(t, "tidy-worktree")
			in.DraftData = []byte(strings.Replace(string(in.DraftData),
				"name: tidy-worktree", "name: "+tc.id, 1))

			_, err := PlanProjectRegister(in)
			if err == nil {
				t.Fatalf("expected a refusal for id %q", tc.id)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("refusal %q does not carry %q", err.Error(), tc.want)
			}
		})
	}
}

// TestPlanProjectRegister_RefusesDestinationResolvingIntoSkillsTree is SEC-1.
// Decision (f) — "the overlay's source tree is never a registration
// destination" — was enforced LEXICALLY only: underSkillsDir inspects the
// repo-relative string, and the resolved containment check only proves the
// destination is inside the root, never that it is outside <root>/skills. So
// with `.claude/skills` (or `.agents/skills`) a symlink to the project's own
// `skills/` tree, the registration was ACCEPTED and its bytes would have
// physically landed in the source tree decision (f) protects.
func TestPlanProjectRegister_RefusesDestinationResolvingIntoSkillsTree(t *testing.T) {
	// linkSkills points <root>/<dir> at <root>/skills, creating both the
	// parent of the link and the real skills/ tree (a DANGLING link is a
	// different, already-covered refusal).
	linkSkills := func(t *testing.T, root, dir string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(root, "skills"), 0o755); err != nil {
			t.Fatalf("mkdir skills: %v", err)
		}
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, filepath.FromSlash(dir))), 0o755); err != nil {
			t.Fatalf("mkdir link parent: %v", err)
		}
		if err := os.Symlink(filepath.Join(root, "skills"), filepath.Join(root, filepath.FromSlash(dir))); err != nil {
			t.Fatalf("symlink %s: %v", dir, err)
		}
	}

	t.Run("symlinked_claude_skills", func(t *testing.T) {
		in := registerInput(t, "tidy-worktree")
		linkSkills(t, in.ProjectRoot, ".claude/skills")

		// Precondition: the lexical decision-(f) guard admits this destination.
		if underSkillsDir(".claude/skills/tidy-worktree/SKILL.md") {
			t.Fatal("precondition: the lexical guard was expected to admit the destination")
		}

		plan, err := PlanProjectRegister(in)
		if err == nil {
			t.Fatalf("expected a refusal, got a plan with %d writes", len(plan.Writes))
		}
		if !strings.Contains(err.Error(), "resolves into the project's own skills/ tree") {
			t.Errorf("refusal %q does not name the resolved skills/ tree reason", err.Error())
		}
		if len(plan.Writes) != 0 || plan.Lock.Rel != "" {
			t.Errorf("a refusal must return the zero plan, got %+v", plan)
		}
	})

	t.Run("symlinked_agents_skills", func(t *testing.T) {
		in := registerInput(t, "tidy-worktree")
		linkSkills(t, in.ProjectRoot, ".agents/skills")

		plan, err := PlanProjectRegister(in)
		if err == nil {
			t.Fatalf("expected a refusal, got a plan with %d writes", len(plan.Writes))
		}
		if !strings.Contains(err.Error(), "resolves into the project's own skills/ tree") {
			t.Errorf("refusal %q does not name the resolved skills/ tree reason", err.Error())
		}
		if len(plan.Writes) != 0 || plan.Lock.Rel != "" {
			t.Errorf("a refusal must return the zero plan, got %+v", plan)
		}
	})

	t.Run("lock_recorded_target_reaching_skills_through_a_symlink", func(t *testing.T) {
		in := registerInput(t, "tidy-worktree")
		linkSkills(t, in.ProjectRoot, ".claude/skills")
		data, err := SerializeProjectLock(ProjectLock{Version: 1, Skills: []ProjectLockEntry{{
			ID:      "sneaky",
			SHA256:  "abc",
			Targets: []string{".claude/skills/sneaky/SKILL.md"},
		}}})
		if err != nil {
			t.Fatalf("serialize: %v", err)
		}
		in.LockData, in.LockExists = data, true

		plan, err := PlanProjectRegister(in)
		if err == nil {
			t.Fatalf("expected a refusal, got a plan with %d writes", len(plan.Writes))
		}
		if !strings.Contains(err.Error(), "resolves into the project's own skills/ tree") {
			t.Errorf("refusal %q does not name the resolved skills/ tree reason", err.Error())
		}
		if !strings.Contains(err.Error(), `lock entry "sneaky"`) {
			t.Errorf("refusal %q does not name the offending lock entry", err.Error())
		}
		if strings.Contains(err.Error(), "outside the project root") {
			t.Errorf("refusal %q calls a target inside the root 'outside the project root'", err.Error())
		}
	})

	t.Run("lock_recorded_target_resolving_to_skills_itself", func(t *testing.T) {
		// The resolved containment helper is strictly-below, so a target that
		// resolves to <root>/skills ITSELF slips past it; decision (f) forbids
		// that path too, and a lock-recorded target is the way to reach it
		// (the fixed table always appends <id>/SKILL.md).
		in := registerInput(t, "tidy-worktree")
		if err := os.MkdirAll(filepath.Join(in.ProjectRoot, "skills"), 0o755); err != nil {
			t.Fatalf("mkdir skills: %v", err)
		}
		if err := os.Symlink(filepath.Join(in.ProjectRoot, "skills"), filepath.Join(in.ProjectRoot, "legacy-link")); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		data, err := SerializeProjectLock(ProjectLock{Version: 1, Skills: []ProjectLockEntry{{
			ID:      "sneaky",
			SHA256:  "abc",
			Targets: []string{"legacy-link"},
		}}})
		if err != nil {
			t.Fatalf("serialize: %v", err)
		}
		in.LockData, in.LockExists = data, true

		plan, err := PlanProjectRegister(in)
		if err == nil {
			t.Fatalf("expected a refusal, got a plan with %d writes", len(plan.Writes))
		}
		if !strings.Contains(err.Error(), "resolves into the project's own skills/ tree") {
			t.Errorf("refusal %q does not name the resolved skills/ tree reason", err.Error())
		}
	})

	t.Run("an_ordinary_registration_still_plans_every_target", func(t *testing.T) {
		// The resolved decision-(f) check must not refuse the normal case,
		// where <root>/skills does not exist at all.
		in := registerInput(t, "tidy-worktree")
		plan, err := PlanProjectRegister(in)
		if err != nil {
			t.Fatalf("unexpected refusal: %v", err)
		}
		if len(plan.Writes) != len(projectTargets) {
			t.Errorf("plan.Writes = %d, want %d", len(plan.Writes), len(projectTargets))
		}
	})
}

// TestPlanProjectRegister_RefusesLockTargetEqualToSkillsDir is COV-3: the
// `clean == "skills"` disjunct of underSkillsDir had no test at all — every
// existing case carried a `skills/` prefix — so a lock-recorded target naming
// exactly `skills` could not witness it.
func TestPlanProjectRegister_RefusesLockTargetEqualToSkillsDir(t *testing.T) {
	if !underSkillsDir("skills") {
		t.Error("underSkillsDir must refuse a destination equal to skills")
	}
	if !underSkillsDir("./skills") {
		t.Error("underSkillsDir must refuse a destination cleaning to skills")
	}

	in := registerInput(t, "tidy-worktree")
	data, err := SerializeProjectLock(ProjectLock{Version: 1, Skills: []ProjectLockEntry{{
		ID:      "sneaky",
		SHA256:  "abc",
		Targets: []string{"skills"},
	}}})
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	in.LockData, in.LockExists = data, true

	plan, err := PlanProjectRegister(in)
	if err == nil {
		t.Fatalf("expected a refusal, got a plan with %d writes", len(plan.Writes))
	}
	// The LEXICAL wording, specifically: the resolved decision-(f) pass added
	// for SEC-1 refuses this target too, so asserting only "never a
	// registration destination" would let the cheap first pass be deleted with
	// the suite green (review round 2, COV-5).
	if !strings.Contains(err.Error(), "under the project's own skills/ directory") {
		t.Errorf("refusal %q is not the lexical decision-(f) refusal", err.Error())
	}
	if strings.Contains(err.Error(), "resolves into") {
		t.Errorf("refusal %q was decided by the resolved pass, not the lexical one", err.Error())
	}
}

// TestPlanProjectRegister_RefusesLockTargetWithDotDotSegments is COV-2: the
// LEXICAL half of the shared write guard (resolveTarget plus the "escapes the
// project root" refusal) had no witness — deleting it left the suite green,
// because the resolved check silently substituted for it. This target is the
// case the resolved check CANNOT see: its ".." segments normalize back inside
// the project root, so containment holds after resolution and only
// resolveTarget's per-segment refusal rejects it.
func TestPlanProjectRegister_RefusesLockTargetWithDotDotSegments(t *testing.T) {
	const target = ".claude/skills/../../.claude/skills/x/SKILL.md"

	// Precondition: this target is NOT visible to either later check — it
	// cleans to a path plainly inside the root and outside skills/.
	if underSkillsDir(target) {
		t.Fatal("precondition: the decision-(f) guard was expected to admit the target")
	}
	in := registerInput(t, "tidy-worktree")
	root := filepath.Clean(in.ProjectRoot)
	joined := filepath.Clean(filepath.Join(root, filepath.FromSlash(target)))
	if !withinRoot(root, joined) {
		t.Fatalf("precondition: %q was expected to normalize back inside the root", joined)
	}

	data, err := SerializeProjectLock(ProjectLock{Version: 1, Skills: []ProjectLockEntry{{
		ID:      "sneaky",
		SHA256:  "abc",
		Targets: []string{target},
	}}})
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	in.LockData, in.LockExists = data, true

	plan, err := PlanProjectRegister(in)
	if err == nil {
		t.Fatalf("expected a refusal for a target carrying .. segments, got a plan with %d writes", len(plan.Writes))
	}
	if !strings.Contains(err.Error(), "outside the project root") {
		t.Errorf("refusal %q does not name the lexical containment reason", err.Error())
	}
	if len(plan.Writes) != 0 || plan.Lock.Rel != "" {
		t.Errorf("a refusal must return the zero plan, got %+v", plan)
	}
}

// --- ExecuteProjectPlan: fixtures -----------------------------------------

// executablePlan produces a real, validated plan over a real (empty) t.TempDir
// project root and returns it with that root. Nothing here touches $HOME, the
// live .claude/skills, .agents/skills, .pi or skills-lock.json.
func executablePlan(t *testing.T, id string) (ProjectPlan, string) {
	t.Helper()
	in := registerInput(t, id)
	p, err := PlanProjectRegister(in)
	if err != nil {
		t.Fatalf("unexpected refusal while building the fixture plan: %v", err)
	}
	return p, filepath.Clean(in.ProjectRoot)
}

// planOrder is the executor's own commit order: the Writes in projectTargets
// order, then the lock LAST.
func planOrder(p ProjectPlan) []ProjectWrite {
	return append(append([]ProjectWrite{}, p.Writes...), p.Lock)
}

// snapshotTree records every path under root with its bytes and permission
// bits, so "the pre-run tree is byte-identical" is a single comparison rather
// than a handful of spot checks. Directories are recorded too, because a
// rollback that leaves an empty `.claude/skills/<id>/` behind has not restored
// the tree.
func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return rerr
		}
		rel = filepath.ToSlash(rel)
		info, ierr := d.Info()
		if ierr != nil {
			return ierr
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			target, lerr := os.Readlink(p)
			if lerr != nil {
				return lerr
			}
			out[rel+"@"] = "symlink " + target
			return nil
		}
		if d.IsDir() {
			out[rel+"/"] = fmt.Sprintf("dir %04o", info.Mode().Perm())
			return nil
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		out[rel] = fmt.Sprintf("file %04o %s", info.Mode().Perm(), b)
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %q: %v", root, err)
	}
	return out
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// assertSameTree reports every difference rather than the first, because a
// rollback usually leaves several traces at once (a file, its directory, a
// leftover temp) and seeing one at a time hides the shape of the bug.
func assertSameTree(t *testing.T, want, got map[string]string) {
	t.Helper()
	for _, k := range sortedKeys(want) {
		if g, ok := got[k]; !ok {
			t.Errorf("rollback lost %q (was %q)", k, want[k])
		} else if g != want[k] {
			t.Errorf("rollback changed %q: got %q, want %q", k, g, want[k])
		}
	}
	for _, k := range sortedKeys(got) {
		if _, ok := want[k]; !ok {
			t.Errorf("rollback left %q behind (%q)", k, got[k])
		}
	}
}

// fakeProjectFS is the injected projectFS of tasks.md 3b-ii.1/3b-ii.3: every
// call is delegated to the real filesystem over a t.TempDir, and `fail` may
// turn any single call into an error. Delegating rather than simulating is
// deliberate — the rollback claim is about the real tree, so the fake injects
// failures and nothing else.
type fakeProjectFS struct {
	real  osProjectFS
	after bool // set by the injector: perform the call, THEN report failure
	fail  func(f *fakeProjectFS, op, path string) error
	n     map[string]int
	log   []string
}

func newFakeProjectFS(fail func(f *fakeProjectFS, op, path string) error) *fakeProjectFS {
	return &fakeProjectFS{fail: fail, n: map[string]int{}}
}

func (f *fakeProjectFS) check(op, path string) error {
	f.n[op]++
	f.log = append(f.log, op+" "+path)
	if f.fail == nil {
		return nil
	}
	return f.fail(f, op, path)
}

func (f *fakeProjectFS) Stat(name string) (fs.FileInfo, error) {
	if err := f.check("stat", name); err != nil {
		return nil, err
	}
	return f.real.Stat(name)
}

func (f *fakeProjectFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if err := f.check("readdir", name); err != nil {
		return nil, err
	}
	return f.real.ReadDir(name)
}

func (f *fakeProjectFS) MkdirAll(dir string, perm fs.FileMode) error {
	if err := f.check("mkdirall", dir); err != nil {
		return err
	}
	return f.real.MkdirAll(dir, perm)
}

func (f *fakeProjectFS) WriteTemp(dir string, data []byte, perm fs.FileMode) (string, error) {
	if err := f.check("writetemp", dir); err != nil {
		return "", err
	}
	return f.real.WriteTemp(dir, data, perm)
}

func (f *fakeProjectFS) Rename(oldPath, newPath string) error {
	f.after = false
	if err := f.check("rename", newPath); err != nil {
		if f.after {
			// The rename lands and still reports failure.
			_ = f.real.Rename(oldPath, newPath)
		}
		return err
	}
	return f.real.Rename(oldPath, newPath)
}

func (f *fakeProjectFS) Remove(name string) error {
	if err := f.check("remove", name); err != nil {
		return err
	}
	return f.real.Remove(name)
}

func (f *fakeProjectFS) ResolvePath(name string) (string, error) {
	if err := f.check("resolvepath", name); err != nil {
		return "", err
	}
	return f.real.ResolvePath(name)
}

// failAt returns a `fail` func that fails the n-th (1-based) call of op whose
// path equals want.
func failAt(op, want string, err error) func(*fakeProjectFS, string, string) error {
	return func(f *fakeProjectFS, gotOp, gotPath string) error {
		if gotOp == op && gotPath == want {
			return err
		}
		return nil
	}
}

// --- ExecuteProjectPlan: the happy path -----------------------------------

// TestExecuteProjectPlan_WritesEveryTargetAndLockLast is the positive case:
// every planned SKILL.md and the lock land with the exact planned bytes at
// mode 0644, the lock is renamed LAST (it is the commit marker), and the
// executor reports one `wrote: <rel>` line per file in that order.
func TestExecuteProjectPlan_WritesEveryTargetAndLockLast(t *testing.T) {
	p, _ := executablePlan(t, "tidy-worktree")
	fsys := newFakeProjectFS(nil)
	var stdout, stderr bytes.Buffer

	if err := ExecuteProjectPlan(p, fsys, &stdout, &stderr); err != nil {
		t.Fatalf("unexpected execution failure: %v (stderr %q)", err, stderr.String())
	}

	order := planOrder(p)
	for _, w := range order {
		info, err := os.Stat(w.Abs)
		if err != nil {
			t.Fatalf("planned write %q was not created: %v", w.Rel, err)
		}
		if got := info.Mode().Perm(); got != ProjectFileMode {
			t.Errorf("%q has mode %04o, want %04o", w.Rel, got, ProjectFileMode)
		}
		got, err := os.ReadFile(w.Abs)
		if err != nil {
			t.Fatalf("read back %q: %v", w.Rel, err)
		}
		if !bytes.Equal(got, w.Data) {
			t.Errorf("%q holds %q, want %q", w.Rel, got, w.Data)
		}
		if strings.Contains(w.Rel, ".pi/") || strings.Contains(w.Rel, "skills-lock.json") {
			t.Errorf("executed write %q names a forbidden path", w.Rel)
		}
	}

	// The lock is the commit marker: its rename is the LAST rename performed.
	var renames []string
	for _, entry := range fsys.log {
		if strings.HasPrefix(entry, "rename ") {
			renames = append(renames, strings.TrimPrefix(entry, "rename "))
		}
	}
	if len(renames) != len(order) {
		t.Fatalf("got %d renames, want %d: %v", len(renames), len(order), renames)
	}
	for i, w := range order {
		if renames[i] != w.Abs {
			t.Errorf("rename %d targets %q, want %q", i, renames[i], w.Abs)
		}
	}

	var wantOut strings.Builder
	for _, w := range order {
		fmt.Fprintf(&wantOut, "wrote: %s\n", w.Rel)
	}
	if stdout.String() != wantOut.String() {
		t.Errorf("stdout = %q, want %q", stdout.String(), wantOut.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("a successful run must print nothing to stderr, got %q", stderr.String())
	}
}

// failAfter returns a `fail` func for a call that LANDS and then reports
// failure: fakeProjectFS consults it before delegating, so the caller pairs it
// with delegation by hand. It exists because the only way the lock's Backup
// restore is ever reached is a lock rename that already happened and still
// reported an error — an NFS or wrapper reality, and the one shape that makes
// the "a pre-existing lock is RESTORED, never deleted" rule observable, since
// the lock is renamed last and nothing else follows it.
func failAfterRename(want string, err error) func(*fakeProjectFS, string, string) error {
	fired := false
	return func(f *fakeProjectFS, gotOp, gotPath string) error {
		if gotOp != "rename" || gotPath != want || fired {
			return nil
		}
		// Only the COMMIT rename fails; the rollback's restoring rename to the
		// same path must be allowed through, or the test would prove nothing
		// about the restore.
		fired = true
		// Let the rename land first, then report the failure.
		f.after = true
		return err
	}
}

var errInjected = errors.New("injected failure")

// injectionPoints enumerates every stage/commit call a failure can be injected
// at for one plan, as tasks.md 3b-ii.1 requires ("an injected failure at each
// rename index and during staging").
func injectionPoints(p ProjectPlan, root string) []struct {
	name string
	op   string
	path string
} {
	order := planOrder(p)
	points := []struct {
		name string
		op   string
		path string
	}{
		{"mkdir_first_target", "mkdirall", filepath.Dir(order[0].Abs)},
		{"mkdir_lock_dir", "mkdirall", filepath.Dir(order[2].Abs)},
		{"stage_first_target", "writetemp", filepath.Dir(order[0].Abs)},
		{"stage_second_target", "writetemp", filepath.Dir(order[1].Abs)},
		{"stage_lock", "writetemp", filepath.Dir(order[2].Abs)},
	}
	for i, w := range order {
		points = append(points, struct {
			name string
			op   string
			path string
		}{fmt.Sprintf("rename_index_%d", i), "rename", w.Abs})
	}
	_ = root
	return points
}

// TestExecuteProjectPlan_RollbackLeavesThePreRunTreeUnchanged is tasks.md
// 3b-ii.1: with a failure injected at each rename index and during staging,
// the tree is byte-identical to its pre-run snapshot — no committed file, no
// leftover temp, and no directory this run created.
func TestExecuteProjectPlan_RollbackLeavesThePreRunTreeUnchanged(t *testing.T) {
	for _, point := range injectionPoints(mustPlanFor(t, "tidy-worktree")) {
		t.Run(point.name, func(t *testing.T) {
			p, root := executablePlan(t, "tidy-worktree")
			// The fixture plan above is a fresh root, so rebuild the injection
			// point against THIS root.
			pt := matchingPoint(t, p, point.name)
			before := snapshotTree(t, root)

			fsys := newFakeProjectFS(failAt(pt.op, pt.path, errInjected))
			var stdout, stderr bytes.Buffer
			err := ExecuteProjectPlan(p, fsys, &stdout, &stderr)
			if err == nil {
				t.Fatalf("an injected %s failure must fail the execution", pt.op)
			}
			if !errors.Is(err, errInjected) {
				t.Errorf("error %v does not carry the injected cause", err)
			}
			if errors.Is(err, ErrRollbackIncomplete) {
				t.Errorf("rollback itself must succeed here, got %v (stderr %q)", err, stderr.String())
			}
			if stderr.Len() != 0 {
				t.Errorf("a successful rollback prints nothing to stderr, got %q", stderr.String())
			}
			if stdout.Len() != 0 {
				t.Errorf("a failed run must report no `wrote:` line, got %q", stdout.String())
			}
			assertSameTree(t, before, snapshotTree(t, root))
		})
	}
}

// mustPlanFor builds one throwaway plan purely to enumerate the injection
// point NAMES; each subtest then rebuilds its own plan over its own root.
func mustPlanFor(t *testing.T, id string) (ProjectPlan, string) {
	t.Helper()
	return executablePlan(t, id)
}

func matchingPoint(t *testing.T, p ProjectPlan, name string) struct {
	name string
	op   string
	path string
} {
	t.Helper()
	for _, pt := range injectionPoints(p, "") {
		if pt.name == name {
			return pt
		}
	}
	t.Fatalf("no injection point named %q", name)
	panic("unreachable")
}

// TestExecuteProjectPlan_RollbackRestoresAPreExistingLock proves the rule the
// design states in as many words: a lock file that already existed at plan
// time carries backup bytes and is RESTORED byte-for-byte, never deleted,
// precisely because it may already carry other skills' entries.
func TestExecuteProjectPlan_RollbackRestoresAPreExistingLock(t *testing.T) {
	in := registerInput(t, "tidy-worktree")
	root := filepath.Clean(in.ProjectRoot)
	existing, err := SerializeProjectLock(ProjectLock{Version: 1, Skills: []ProjectLockEntry{{
		ID:         "zzz-other",
		Provenance: "procedural",
		Candidate:  "procedural/candidates/repeated-success/zzz-other",
		SHA256:     "abc",
		Revision:   2,
		Targets:    []string{".claude/skills/zzz-other/SKILL.md"},
	}}})
	if err != nil {
		t.Fatalf("building the existing lock: %v", err)
	}
	lockAbs := filepath.Join(root, filepath.FromSlash(ProjectLockRelPath))
	if err := os.MkdirAll(filepath.Dir(lockAbs), 0o755); err != nil {
		t.Fatalf("mkdir lock dir: %v", err)
	}
	if err := os.WriteFile(lockAbs, existing, 0o644); err != nil {
		t.Fatalf("write existing lock: %v", err)
	}
	in.LockData, in.LockExists = existing, true

	p, err := PlanProjectRegister(in)
	if err != nil {
		t.Fatalf("unexpected refusal: %v", err)
	}
	if p.Lock.Backup == nil {
		t.Fatal("precondition: the plan must have captured the pre-existing lock bytes as a backup")
	}
	before := snapshotTree(t, root)

	fsys := newFakeProjectFS(failAfterRename(p.Lock.Abs, errInjected))
	var stdout, stderr bytes.Buffer
	if err := ExecuteProjectPlan(p, fsys, &stdout, &stderr); err == nil {
		t.Fatal("a rename that reports failure must fail the execution")
	}
	if stderr.Len() != 0 {
		t.Errorf("a successful rollback prints nothing to stderr, got %q", stderr.String())
	}

	got, err := os.ReadFile(lockAbs)
	if err != nil {
		t.Fatalf("the pre-existing lock must survive rollback: %v", err)
	}
	if !bytes.Equal(got, existing) {
		t.Errorf("lock after rollback = %q, want the pre-existing bytes %q", got, existing)
	}
	assertSameTree(t, before, snapshotTree(t, root))
}

// TestExecuteProjectPlan_RollbackRemovesALockItCreated is the other half of
// the same rule: a lock created for the very first registration in a project
// has no backup and is REMOVED, taking its directory with it.
func TestExecuteProjectPlan_RollbackRemovesALockItCreated(t *testing.T) {
	p, root := executablePlan(t, "tidy-worktree")
	if p.Lock.Backup != nil {
		t.Fatal("precondition: a first registration plans a lock with no backup")
	}
	before := snapshotTree(t, root)

	fsys := newFakeProjectFS(failAfterRename(p.Lock.Abs, errInjected))
	var stdout, stderr bytes.Buffer
	if err := ExecuteProjectPlan(p, fsys, &stdout, &stderr); err == nil {
		t.Fatal("a rename that reports failure must fail the execution")
	}
	if _, err := os.Stat(p.Lock.Abs); !os.IsNotExist(err) {
		t.Errorf("a lock this run created must be removed on rollback, Stat gave %v", err)
	}
	assertSameTree(t, before, snapshotTree(t, root))
}

// TestExecuteProjectPlan_RollbackOfRollback is tasks.md 3b-ii.3: when the
// rollback itself fails, the operator gets a precise pointer —
// `error: rollback incomplete: <rel-path>` — and the run fails, distinct from
// the ordinary rollback-succeeds cases above.
func TestExecuteProjectPlan_RollbackOfRollback(t *testing.T) {
	p, root := executablePlan(t, "tidy-worktree")
	order := planOrder(p)

	// Commit the first SKILL.md, then fail the second rename so rollback must
	// remove the first — and fail that removal.
	fsys := newFakeProjectFS(func(f *fakeProjectFS, op, path string) error {
		if op == "rename" && path == order[1].Abs {
			return errInjected
		}
		if op == "remove" && path == order[0].Abs {
			return errInjected
		}
		return nil
	})
	var stdout, stderr bytes.Buffer
	err := ExecuteProjectPlan(p, fsys, &stdout, &stderr)
	if err == nil {
		t.Fatal("a failed rollback must fail the execution")
	}
	if !errors.Is(err, ErrRollbackIncomplete) {
		t.Errorf("error %v must be recognisable as an incomplete rollback", err)
	}
	want := "error: rollback incomplete: " + order[0].Rel + "\n"
	if !strings.Contains(stderr.String(), want) {
		t.Errorf("stderr = %q, want it to contain %q", stderr.String(), want)
	}
	if stdout.Len() != 0 {
		t.Errorf("a failed run must report no `wrote:` line, got %q", stdout.String())
	}
	// The pointer must be honest: that path really is still there.
	if _, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(order[0].Rel))); statErr != nil {
		t.Errorf("the reported path must be the one actually left behind: %v", statErr)
	}
}

// TestPlanProjectRegister_RefusesAliasedTargets closes ALIAS-1 (review round
// 2, carried to slice 3b-ii): the two FIXED targets are distinct strings, but
// nothing stopped them resolving to the SAME physical directory — with
// `<root>/.agents` a symlink to `<root>/.claude`, both destinations name one
// file on disk.
//
// The registration is REFUSED rather than collapsed to a single write. Two
// reasons decide it: (1) the lock would record two targets for one file, so
// EvaluateOwnership would later read the same bytes twice and a human deleting
// "one" copy would appear to have deleted both; (2) rollback's headline
// guarantee — the pre-run tree comes back byte-identical — becomes
// conditionally false, because the second planned write would silently
// overwrite the first and only one of the two removals can succeed. A refusal
// keeps both properties unconditional, and an aliased `.agents` is an unusual
// project layout the human can undo, not a state the tool should guess at.
func TestPlanProjectRegister_RefusesAliasedTargets(t *testing.T) {
	in := registerInput(t, "tidy-worktree")
	root := filepath.Clean(in.ProjectRoot)
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	// Precondition: without the alias this exact input plans cleanly, so the
	// refusal below can only be about the alias.
	if _, err := PlanProjectRegister(in); err != nil {
		t.Fatalf("precondition: the un-aliased input must plan cleanly, got %v", err)
	}
	if err := os.Symlink(filepath.Join(root, ".claude"), filepath.Join(root, ".agents")); err != nil {
		t.Fatalf("symlink .agents -> .claude: %v", err)
	}

	plan, err := PlanProjectRegister(in)
	if err == nil {
		t.Fatalf("expected a refusal for two targets resolving to one file, got a plan with %d writes", len(plan.Writes))
	}
	if !strings.Contains(err.Error(), "resolve to the same file") {
		t.Errorf("refusal %q does not name the aliasing reason", err.Error())
	}
	if len(plan.Writes) != 0 || plan.Lock.Rel != "" {
		t.Errorf("a refusal must return the zero plan, got %+v", plan)
	}
}

// TestExecuteProjectPlan_RefusesAliasedDestinations is the check-then-act half
// of ALIAS-1: the planner's proof is point-in-time, so a plan whose two
// destinations were distinct when it was built must still be refused if they
// have since come to name one file. Nothing is written.
func TestExecuteProjectPlan_RefusesAliasedDestinations(t *testing.T) {
	p, root := executablePlan(t, "tidy-worktree")
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	if err := os.Symlink(filepath.Join(root, ".claude"), filepath.Join(root, ".agents")); err != nil {
		t.Fatalf("symlink .agents -> .claude: %v", err)
	}
	before := snapshotTree(t, root)

	var stdout, stderr bytes.Buffer
	err := ExecuteProjectPlan(p, newFakeProjectFS(nil), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected a refusal for two destinations resolving to one file")
	}
	if !strings.Contains(err.Error(), "resolve to the same file") {
		t.Errorf("refusal %q does not name the aliasing reason", err.Error())
	}
	assertSameTree(t, before, snapshotTree(t, root))
}
