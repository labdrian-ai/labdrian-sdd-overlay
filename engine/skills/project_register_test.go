package skills

import (
	"io/fs"
	"os"
	"path/filepath"
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
