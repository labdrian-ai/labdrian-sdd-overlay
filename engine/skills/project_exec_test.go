package skills

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

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
	// Deliberately NOT ProjectFileMode. The rollback guarantee is that the
	// pre-run tree comes back byte-identical "including file modes", and a
	// pre-existing lock written at 0644 cannot witness that: rollback restored
	// through the planned write's Mode, which is always 0644, so a tree that had
	// been widened from 0600 to 0644 compared equal (review round 4, D1).
	if err := os.WriteFile(lockAbs, existing, 0o600); err != nil {
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
	info, err := os.Stat(lockAbs)
	if err != nil {
		t.Fatalf("stat the restored lock: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("the restored lock has mode %04o, want the pre-run mode %04o", perm, 0o600)
	}
	assertSameTree(t, before, snapshotTree(t, root))
}

// TestExecuteProjectPlan_RollbackLeavesAnUntouchedDestinationAlone is the other
// half of D1 (review round 4): rollback walks the PLANNED writes, so a
// destination this run never renamed over was being rewritten anyway — a fresh
// inode, and (for a pre-existing lock) a fresh mode — on every failure that
// happened before its rename. Restoring identical bytes still replaces the
// file, and "byte-identical" is a claim about the tree, not only its contents.
//
// The failure is injected at rename index 0, so the lock's rename is never
// reached: the lock must come back with its own mode AND its own inode.
func TestExecuteProjectPlan_RollbackLeavesAnUntouchedDestinationAlone(t *testing.T) {
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
	if err := os.WriteFile(lockAbs, existing, 0o600); err != nil {
		t.Fatalf("write existing lock: %v", err)
	}
	in.LockData, in.LockExists = existing, true

	p, err := PlanProjectRegister(in)
	if err != nil {
		t.Fatalf("unexpected refusal: %v", err)
	}
	before := snapshotTree(t, root)
	beforeIno := inodeOf(t, lockAbs)

	order := planOrder(p)
	fsys := newFakeProjectFS(failAt("rename", order[0].Abs, errInjected))
	var stdout, stderr bytes.Buffer
	if err := ExecuteProjectPlan(p, fsys, &stdout, &stderr); err == nil {
		t.Fatal("an injected rename failure must fail the execution")
	}
	if stderr.Len() != 0 {
		t.Errorf("a successful rollback prints nothing to stderr, got %q", stderr.String())
	}
	if got := inodeOf(t, lockAbs); got != beforeIno {
		t.Errorf("the lock was rewritten (inode %d -> %d) although this run never renamed over it", beforeIno, got)
	}
	assertSameTree(t, before, snapshotTree(t, root))
}

// inodeOf reports the file's inode number, so a test can tell "the same file is
// still there" from "an identical file was written in its place". It skips on a
// platform whose Stat does not expose one.
func inodeOf(t *testing.T, p string) uint64 {
	t.Helper()
	info, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat %q: %v", p, err)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Skip("this platform does not expose an inode number")
	}
	return uint64(st.Ino)
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

// TestExecuteProjectPlan_RollbackRemovesDirectoriesAPartialMkdirCreated is D2.
// `s.created` was appended to only AFTER MkdirAll returned nil, but
// os.MkdirAll creates the ancestors it can before failing on a deeper
// component. Those ancestors stayed behind forever: rollback never learned
// about them, so a failed run left `.claude/skills/` in a project that had
// never had one.
func TestExecuteProjectPlan_RollbackRemovesDirectoriesAPartialMkdirCreated(t *testing.T) {
	p, root := executablePlan(t, "tidy-worktree")
	leaf := filepath.Dir(planOrder(p)[0].Abs)
	before := snapshotTree(t, root)

	fsys := newFakeProjectFS(failPartialMkdir(leaf, errInjected))
	var stdout, stderr bytes.Buffer
	err := ExecuteProjectPlan(p, fsys, &stdout, &stderr)
	if err == nil {
		t.Fatal("an injected mkdir failure must fail the execution")
	}
	if !errors.Is(err, errInjected) {
		t.Errorf("error %v does not carry the injected cause", err)
	}
	if errors.Is(err, ErrRollbackIncomplete) {
		t.Errorf("rollback itself must succeed here, got %v (stderr %q)", err, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("a failed run must report no `wrote:` line, got %q", stdout.String())
	}
	assertSameTree(t, before, snapshotTree(t, root))
}

// TestExecuteProjectPlan_RefusesADestinationThatAppearedAfterPlanning is D3.
// Restore-vs-remove was decided purely from the plan-time Backup, and every
// SKILL.md write has Backup == nil, so a destination that did not exist when
// the plan was built but DOES exist when it runs was deleted by rollback — a
// file this run never created. On the success path the same gap silently
// overwrote it, which is exactly the "foreign skill" the planner refuses.
//
// The decision is to REFUSE, not to back the file up and restore it. The
// planner already refuses a destination that exists and is not in the lock; a
// destination that appears between plan and write is the same fact arriving
// late, and answering it differently would mean the tool overwrites on a race
// what it refuses to overwrite when it looks first. Refusing also keeps the
// promise cheap: nothing is written, so there is nothing to roll back.
func TestExecuteProjectPlan_RefusesADestinationThatAppearedAfterPlanning(t *testing.T) {
	p, root := executablePlan(t, "tidy-worktree")
	victim := planOrder(p)[0]
	if victim.Backup != nil {
		t.Fatal("precondition: a SKILL.md write is planned with no backup")
	}
	if err := os.MkdirAll(filepath.Dir(victim.Abs), 0o755); err != nil {
		t.Fatalf("mkdir destination dir: %v", err)
	}
	if err := os.WriteFile(victim.Abs, []byte("someone else's skill\n"), 0o600); err != nil {
		t.Fatalf("write the intruding file: %v", err)
	}
	before := snapshotTree(t, root)

	var stdout, stderr bytes.Buffer
	err := ExecuteProjectPlan(p, newFakeProjectFS(nil), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected a refusal for a destination that appeared after planning")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("refusal %q does not name the reason", err.Error())
	}
	if stdout.Len() != 0 {
		t.Errorf("a refused run must report no `wrote:` line, got %q", stdout.String())
	}
	assertSameTree(t, before, snapshotTree(t, root))
}

// TestExecuteProjectPlan_RefusesADestinationResolvingIntoTheSkillsTree is D4.
// checkProjectDestinations re-proved root containment and the alias rule but
// dropped decision (f): no destination may resolve under <root>/skills/. A
// `.claude/skills` that becomes a symlink into the project's own source tree
// between plan and write was therefore written anyway, landing the registration
// physically in the tree the design forbids as a destination.
// The executor also refuses a destination that names the project's own
// skills/ tree LEXICALLY, without any symlink. PlanProjectRegister can never
// emit such a write, but ExecuteProjectPlan takes a ProjectPlan value: a plan
// built anywhere else must still meet decision (f). The symlink test above
// exercises the resolved half only, so this guard needs its own witness.
func TestExecuteProjectPlan_RefusesALexicalSkillsTreeDestination(t *testing.T) {
	p, root := executablePlan(t, "tidy-worktree")
	rel := filepath.ToSlash(filepath.Join(projectSourceSkillsDir, "tidy-worktree", projectSkillFileName))
	p.Writes[0].Rel = rel
	p.Writes[0].Abs = filepath.Join(root, filepath.FromSlash(rel))
	before := snapshotTree(t, root)

	var stdout, stderr bytes.Buffer
	err := ExecuteProjectPlan(p, newFakeProjectFS(nil), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected a refusal for a destination lexically under the project's skills/ tree")
	}
	// The lexical pre-pass owns this message. The resolved check below it also
	// refuses the same write ("resolves into the project's own skills/ tree"),
	// so decision (f) is not fail-open without the pre-pass — but the cheaper,
	// more precise wording is what this test pins.
	if !strings.Contains(err.Error(), "lies under skills/") {
		t.Errorf("refusal %q is not the lexical decision-(f) refusal", err.Error())
	}
	if stdout.Len() != 0 {
		t.Errorf("a refused run must report no `wrote:` line, got %q", stdout.String())
	}
	assertSameTree(t, before, snapshotTree(t, root))
}

func TestExecuteProjectPlan_RefusesADestinationResolvingIntoTheSkillsTree(t *testing.T) {
	p, root := executablePlan(t, "tidy-worktree")
	if err := os.MkdirAll(filepath.Join(root, projectSourceSkillsDir), 0o755); err != nil {
		t.Fatalf("mkdir the project's own skills tree: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	// The symlink appears AFTER the plan was built — the whole point.
	if err := os.Symlink(filepath.Join(root, projectSourceSkillsDir), filepath.Join(root, ".claude", "skills")); err != nil {
		t.Fatalf("symlink .claude/skills -> skills: %v", err)
	}
	before := snapshotTree(t, root)

	var stdout, stderr bytes.Buffer
	err := ExecuteProjectPlan(p, newFakeProjectFS(nil), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected a refusal for a destination resolving into the project's own skills/ tree")
	}
	if !strings.Contains(err.Error(), "skills/ tree") {
		t.Errorf("refusal %q does not name decision (f)", err.Error())
	}
	if stdout.Len() != 0 {
		t.Errorf("a refused run must report no `wrote:` line, got %q", stdout.String())
	}
	assertSameTree(t, before, snapshotTree(t, root))
}

// TestExecuteProjectPlan_RollbackIncompleteReportsRepoRelativePaths is D5.
// (*projectStager).rel sat at 0.0% coverage: the path formatting used in every
// `error: rollback incomplete: <rel-path>` line for a leftover temp or a
// created directory was never once executed by a test, so the operator-facing
// pointer — which is meant to be usable as a git pathspec — was unproven.
func TestExecuteProjectPlan_RollbackIncompleteReportsRepoRelativePaths(t *testing.T) {
	t.Run("created_directory", func(t *testing.T) {
		p, root := executablePlan(t, "tidy-worktree")
		order := planOrder(p)
		stuck := filepath.Dir(order[1].Abs)

		fsys := newFakeProjectFS(func(f *fakeProjectFS, op, path string) error {
			if op == "rename" && path == order[0].Abs {
				return errInjected
			}
			if op == "remove" && path == stuck {
				return errInjected
			}
			return nil
		})
		var stdout, stderr bytes.Buffer
		err := ExecuteProjectPlan(p, fsys, &stdout, &stderr)
		if !errors.Is(err, ErrRollbackIncomplete) {
			t.Fatalf("error %v must be recognisable as an incomplete rollback (stderr %q)", err, stderr.String())
		}
		want := "error: rollback incomplete: " + path.Dir(order[1].Rel) + "\n"
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("stderr = %q, want it to contain %q", stderr.String(), want)
		}
		if strings.Contains(stderr.String(), root) {
			t.Errorf("stderr = %q, want a repo-relative path, not one carrying the absolute root", stderr.String())
		}
		if _, statErr := os.Stat(stuck); statErr != nil {
			t.Errorf("the reported directory must be the one actually left behind: %v", statErr)
		}
	})

	t.Run("leftover_temp", func(t *testing.T) {
		p, root := executablePlan(t, "tidy-worktree")
		order := planOrder(p)

		var leftover string
		fsys := newFakeProjectFS(func(f *fakeProjectFS, op, path string) error {
			if op == "rename" && path == order[0].Abs {
				return errInjected
			}
			if op == "remove" && strings.Contains(filepath.Base(path), ".tmp-skills-") &&
				filepath.Dir(path) == filepath.Dir(order[1].Abs) {
				leftover = path
				return errInjected
			}
			return nil
		})
		var stdout, stderr bytes.Buffer
		err := ExecuteProjectPlan(p, fsys, &stdout, &stderr)
		if !errors.Is(err, ErrRollbackIncomplete) {
			t.Fatalf("error %v must be recognisable as an incomplete rollback (stderr %q)", err, stderr.String())
		}
		if leftover == "" {
			t.Fatal("no leftover temp removal was ever attempted, so this test proves nothing")
		}
		rel, rerr := filepath.Rel(root, leftover)
		if rerr != nil {
			t.Fatalf("rel %q: %v", leftover, rerr)
		}
		want := "error: rollback incomplete: " + filepath.ToSlash(rel) + "\n"
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("stderr = %q, want it to contain %q", stderr.String(), want)
		}
		if strings.Contains(stderr.String(), root) {
			t.Errorf("stderr = %q, want a repo-relative path, not one carrying the absolute root", stderr.String())
		}
		if _, statErr := os.Stat(leftover); statErr != nil {
			t.Errorf("the reported temp must be the one actually left behind: %v", statErr)
		}
	})
}
