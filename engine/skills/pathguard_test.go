package skills

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWithinRoot covers the containment helper extracted from PlanInstall's
// inline R-055 guard (3b-i.1/3b-i.2). The cases mirror the fixtures in
// TestPlanInstallTraversalGuard (install_test.go, the regression net, which
// stays unmodified) at the level of the helper itself, plus the
// path-equals-root case that the inline prefix test ADMITTED and the shared
// strictly-below helper REFUSES. That difference is the one deliberate
// tightening the extraction carries; it is recorded here rather than left
// invisible.
func TestWithinRoot(t *testing.T) {
	const skillsRoot = "/target-repo/.claude/skills"

	cases := []struct {
		name string
		root string
		p    string
		want bool
	}{
		{"plain_child", skillsRoot, skillsRoot + "/safe-skill", true},
		{"nested_child", skillsRoot, skillsRoot + "/safe-skill/SKILL.md", true},
		{"id_dotdot_escapes_skills_root", skillsRoot, "/target-repo/outside", false},
		{"path_dotdot_escapes_source_root", "/overlay/skills", "/etc", false},
		{"equals_root_refused", skillsRoot, skillsRoot, false},
		{"parent_of_root_refused", skillsRoot, "/target-repo/.claude", false},
		{"sibling_prefix_not_a_child", skillsRoot, skillsRoot + "-evil/x", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := withinRoot(tc.root, tc.p); got != tc.want {
				t.Errorf("withinRoot(%q, %q) = %v, want %v", tc.root, tc.p, got, tc.want)
			}
		})
	}
}

// TestResolvePathKeepingMissing pins the resolver contract tasks.md 3b-i.5b
// states: resolve every symlink in the path's EXISTING ancestry, keep
// non-existent components literal, and error only on a genuine resolution
// failure. Bare filepath.EvalSymlinks does NOT satisfy this — it fails on a
// missing final component — which is exactly why an absent target must still
// reach readFile and report "missing <path>" from EvaluateOwnership rather
// than "unresolved-target <target>". Real symlinks under t.TempDir only.
func TestResolvePathKeepingMissing(t *testing.T) {
	t.Run("existing_symlinked_ancestor_is_resolved", func(t *testing.T) {
		tmp := t.TempDir()
		real := filepath.Join(tmp, "real")
		if err := os.MkdirAll(filepath.Join(real, "inner"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		link := filepath.Join(tmp, "link")
		if err := os.Symlink(real, link); err != nil {
			t.Fatalf("symlink: %v", err)
		}

		got, err := resolvePathKeepingMissing(filepath.Join(link, "inner"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want, err := filepath.EvalSymlinks(filepath.Join(real, "inner"))
		if err != nil {
			t.Fatalf("EvalSymlinks: %v", err)
		}
		if got != want {
			t.Errorf("resolved = %q, want %q", got, want)
		}
	})

	t.Run("missing_final_component_stays_literal", func(t *testing.T) {
		tmp := t.TempDir()
		real := filepath.Join(tmp, "real")
		if err := os.MkdirAll(real, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		link := filepath.Join(tmp, "link")
		if err := os.Symlink(real, link); err != nil {
			t.Fatalf("symlink: %v", err)
		}

		// Bare EvalSymlinks fails here; the wrapper must not.
		if _, err := filepath.EvalSymlinks(filepath.Join(link, "absent", "SKILL.md")); err == nil {
			t.Fatal("precondition: expected bare EvalSymlinks to fail on a missing component")
		}

		got, err := resolvePathKeepingMissing(filepath.Join(link, "absent", "SKILL.md"))
		if err != nil {
			t.Fatalf("unexpected error for an absent tail: %v", err)
		}
		resolvedReal, err := filepath.EvalSymlinks(real)
		if err != nil {
			t.Fatalf("EvalSymlinks: %v", err)
		}
		want := filepath.Join(resolvedReal, "absent", "SKILL.md")
		if got != want {
			t.Errorf("resolved = %q, want %q", got, want)
		}
	})

	t.Run("wholly_absent_path_under_existing_root", func(t *testing.T) {
		tmp := t.TempDir()
		got, err := resolvePathKeepingMissing(filepath.Join(tmp, "a", "b", "c"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		resolvedTmp, err := filepath.EvalSymlinks(tmp)
		if err != nil {
			t.Fatalf("EvalSymlinks: %v", err)
		}
		if want := filepath.Join(resolvedTmp, "a", "b", "c"); got != want {
			t.Errorf("resolved = %q, want %q", got, want)
		}
	})

	t.Run("symlink_loop_is_a_genuine_failure", func(t *testing.T) {
		tmp := t.TempDir()
		a := filepath.Join(tmp, "a")
		b := filepath.Join(tmp, "b")
		if err := os.Symlink(b, a); err != nil {
			t.Fatalf("symlink a: %v", err)
		}
		if err := os.Symlink(a, b); err != nil {
			t.Fatalf("symlink b: %v", err)
		}

		if _, err := resolvePathKeepingMissing(filepath.Join(a, "SKILL.md")); err == nil {
			t.Fatal("expected an error for a symlink loop, got nil")
		} else if errors.Is(err, fs.ErrNotExist) {
			t.Errorf("a symlink loop must not be reported as ErrNotExist, got %v", err)
		}
	})
}

// TestResolvedWithinRoot proves the second, non-lexical containment step:
// a symlinked `.claude` (or `.agents`) pointing out of the project root
// passes every lexical guard and must still be refused
// (review-slice-3a-ii-round-2, SEC-2).
func TestResolvedWithinRoot(t *testing.T) {
	t.Run("plain_destination_inside_root", func(t *testing.T) {
		root := t.TempDir()
		ok, err := resolvedWithinRoot(root, filepath.Join(root, ".claude", "skills", "x", "SKILL.md"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Error("a plain destination under root must be within it")
		}
	})

	t.Run("symlinked_claude_escaping_root_refused", func(t *testing.T) {
		base := t.TempDir()
		root := filepath.Join(base, "project")
		outside := filepath.Join(base, "outside")
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatalf("mkdir root: %v", err)
		}
		if err := os.MkdirAll(outside, 0o755); err != nil {
			t.Fatalf("mkdir outside: %v", err)
		}
		if err := os.Symlink(outside, filepath.Join(root, ".claude")); err != nil {
			t.Fatalf("symlink .claude: %v", err)
		}

		dest := filepath.Join(root, ".claude", "skills", "x", "SKILL.md")

		// Precondition: the lexical guard alone admits this destination.
		if !withinRoot(filepath.Clean(root), filepath.Clean(dest)) {
			t.Fatal("precondition: the lexical guard was expected to admit the symlinked destination")
		}

		ok, err := resolvedWithinRoot(root, dest)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Error("a destination reached through a symlinked .claude escaping root must be refused")
		}
	})

	t.Run("root_itself_refused", func(t *testing.T) {
		root := t.TempDir()
		ok, err := resolvedWithinRoot(root, root)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Error("root itself is not strictly below root")
		}
	})
}

// TestResolvePathKeepingMissing_DanglingSymlinkIsAFailure pins the F1 fix.
// A component that is itself a symlink whose target does not exist yet is NOT
// a "merely missing" component: keeping it literal silently un-follows the
// link, and every containment proof built on the result is decided on a path
// that only looks contained. It must be reported as a genuine resolution
// failure, while an ORDINARY absent component stays literal — EvaluateOwnership
// depends on that second half to report "missing <path>".
func TestResolvePathKeepingMissing_DanglingSymlinkIsAFailure(t *testing.T) {
	t.Run("dangling_symlink_component_fails", func(t *testing.T) {
		tmp := t.TempDir()
		link := filepath.Join(tmp, "link")
		if err := os.Symlink(filepath.Join(tmp, "never-created"), link); err != nil {
			t.Fatalf("symlink: %v", err)
		}

		if _, err := resolvePathKeepingMissing(filepath.Join(link, "skills", "x", "SKILL.md")); err == nil {
			t.Fatal("expected an error for a dangling symlink component, got nil")
		}
	})

	t.Run("dangling_symlink_as_the_final_component_fails", func(t *testing.T) {
		tmp := t.TempDir()
		link := filepath.Join(tmp, "link")
		if err := os.Symlink(filepath.Join(tmp, "never-created"), link); err != nil {
			t.Fatalf("symlink: %v", err)
		}

		if _, err := resolvePathKeepingMissing(link); err == nil {
			t.Fatal("expected an error for a dangling symlink, got nil")
		}
	})

	t.Run("ordinary_absent_component_still_stays_literal", func(t *testing.T) {
		tmp := t.TempDir()
		got, err := resolvePathKeepingMissing(filepath.Join(tmp, "absent", "SKILL.md"))
		if err != nil {
			t.Fatalf("an ordinary absent component must stay literal, got %v", err)
		}
		resolvedTmp, err := filepath.EvalSymlinks(tmp)
		if err != nil {
			t.Fatalf("EvalSymlinks: %v", err)
		}
		if want := filepath.Join(resolvedTmp, "absent", "SKILL.md"); got != want {
			t.Errorf("resolved = %q, want %q", got, want)
		}
	})
}

// TestResolvedWithinRootRefusesDanglingSymlink is the guard-level consequence
// of the F1 fix: a destination reached through a DANGLING symlinked `.claude`
// used to be admitted, because the unfollowed literal path compared as
// contained.
func TestResolvedWithinRootRefusesDanglingSymlink(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "project")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}
	// The link target is outside the root and does not exist yet.
	if err := os.Symlink(filepath.Join(base, "outside"), filepath.Join(root, ".claude")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	dest := filepath.Join(root, ".claude", "skills", "x", "SKILL.md")
	ok, err := resolvedWithinRoot(root, dest)
	if err == nil && ok {
		t.Fatal("a destination reached through a dangling symlinked .claude must not be admitted")
	}
}

// TestPlanInstallRefusesDestinationEqualToSkillsRoot is TQ-3: the deliberate
// tightening the withinRoot extraction carried (a dst EQUAL to the skills root
// was admitted by the old inline prefix check and is refused by the shared
// strictly-below helper) had no test at the PlanInstall level, only at the
// helper level. install_test.go is the untouched regression net, so this lives
// here.
func TestPlanInstallRefusesDestinationEqualToSkillsRoot(t *testing.T) {
	reg := Registry{Version: "1", Skills: []Entry{{
		ID:   ".",
		Path: "skills/whatever",
		Install: Install{
			DefaultScope:    "project",
			AllowedProjects: []string{"proj"},
		},
	}}}

	ops, err := PlanInstall(reg, "proj", "/overlay", "/target-repo")
	if err == nil {
		t.Fatalf("expected a refusal for a destination equal to the skills root, got %d ops", len(ops))
	}
	if !strings.Contains(err.Error(), "escapes target skills root") {
		t.Errorf("refusal %q does not name the target-root branch", err.Error())
	}
	if ops != nil {
		t.Errorf("a refusal must return no ops, got %v", ops)
	}
}
