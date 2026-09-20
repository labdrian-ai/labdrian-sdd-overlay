package skills

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
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
