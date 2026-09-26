package pathguard

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// TestWithinRoot covers the containment helper's core semantics: p equal to
// root is not within it, a plain child is, and a sibling that merely shares a
// prefix is refused. It also pins the fail-closed precondition: an empty or
// uncleaned argument is refused rather than trusted, and a filesystem root
// ("/") is still never a containing root.
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
		{"uncleaned_dotdot_refused", "/a", "/a/../etc", false},
		{"empty_root_refused", "", "/anywhere", false},
		{"empty_path_refused", "/a", "", false},
		{"uncleaned_root_refused", "/a/./b", "/a/./b/c", false},
		{"filesystem_root_stays_fail_closed", "/", "/etc", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := WithinRoot(tc.root, tc.p); got != tc.want {
				t.Errorf("WithinRoot(%q, %q) = %v, want %v", tc.root, tc.p, got, tc.want)
			}
		})
	}
}

// TestResolvePathKeepingMissing pins the resolver contract: resolve every
// symlink in the path's EXISTING ancestry, keep non-existent components
// literal, and error only on a genuine resolution failure.
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

		got, err := ResolvePathKeepingMissing(filepath.Join(link, "inner"))
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

		if _, err := filepath.EvalSymlinks(filepath.Join(link, "absent", "SKILL.md")); err == nil {
			t.Fatal("precondition: expected bare EvalSymlinks to fail on a missing component")
		}

		got, err := ResolvePathKeepingMissing(filepath.Join(link, "absent", "SKILL.md"))
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
		got, err := ResolvePathKeepingMissing(filepath.Join(tmp, "a", "b", "c"))
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

		if _, err := ResolvePathKeepingMissing(filepath.Join(a, "SKILL.md")); err == nil {
			t.Fatal("expected an error for a symlink loop, got nil")
		} else if errors.Is(err, fs.ErrNotExist) {
			t.Errorf("a symlink loop must not be reported as ErrNotExist, got %v", err)
		}
	})
}

// TestResolvedWithinRoot proves the second, non-lexical containment step: a
// symlinked directory pointing out of the root passes every lexical guard and
// must still be refused.
func TestResolvedWithinRoot(t *testing.T) {
	t.Run("plain_destination_inside_root", func(t *testing.T) {
		root := t.TempDir()
		ok, err := ResolvedWithinRoot(root, filepath.Join(root, ".claude", "skills", "x", "SKILL.md"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Error("a plain destination under root must be within it")
		}
	})

	t.Run("symlinked_dir_escaping_root_refused", func(t *testing.T) {
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
		if !WithinRoot(filepath.Clean(root), filepath.Clean(dest)) {
			t.Fatal("precondition: the lexical guard was expected to admit the symlinked destination")
		}

		ok, err := ResolvedWithinRoot(root, dest)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Error("a destination reached through a symlinked directory escaping root must be refused")
		}
	})

	t.Run("root_itself_refused", func(t *testing.T) {
		root := t.TempDir()
		ok, err := ResolvedWithinRoot(root, root)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Error("root itself is not strictly below root")
		}
	})
}

// TestResolvePathKeepingMissingDanglingSymlinkIsAFailure pins the dangling
// symlink case: a component that is itself a symlink whose target does not
// exist yet must be reported as a genuine resolution failure, not kept
// literal like an ordinary absent component.
func TestResolvePathKeepingMissingDanglingSymlinkIsAFailure(t *testing.T) {
	t.Run("dangling_symlink_component_fails", func(t *testing.T) {
		tmp := t.TempDir()
		link := filepath.Join(tmp, "link")
		if err := os.Symlink(filepath.Join(tmp, "never-created"), link); err != nil {
			t.Fatalf("symlink: %v", err)
		}

		if _, err := ResolvePathKeepingMissing(filepath.Join(link, "skills", "x", "SKILL.md")); err == nil {
			t.Fatal("expected an error for a dangling symlink component, got nil")
		}
	})

	t.Run("dangling_symlink_as_the_final_component_fails", func(t *testing.T) {
		tmp := t.TempDir()
		link := filepath.Join(tmp, "link")
		if err := os.Symlink(filepath.Join(tmp, "never-created"), link); err != nil {
			t.Fatalf("symlink: %v", err)
		}

		if _, err := ResolvePathKeepingMissing(link); err == nil {
			t.Fatal("expected an error for a dangling symlink, got nil")
		}
	})

	t.Run("ordinary_absent_component_still_stays_literal", func(t *testing.T) {
		tmp := t.TempDir()
		got, err := ResolvePathKeepingMissing(filepath.Join(tmp, "absent", "SKILL.md"))
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
// of the dangling-symlink fix: a destination reached through a DANGLING
// symlinked directory must never be admitted as contained.
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
	ok, err := ResolvedWithinRoot(root, dest)
	if err == nil && ok {
		t.Fatal("a destination reached through a dangling symlinked directory must not be admitted")
	}
}

// TestResolvePathKeepingMissingPermissionDenialIsAGenuineFailure proves an
// unreadable parent directory is a genuine failure, not a literal tail that
// would let a containment proof silently say "inside" for an escape hidden
// behind it.
func TestResolvePathKeepingMissingPermissionDenialIsAGenuineFailure(t *testing.T) {
	t.Run("unreadable_parent_is_an_error_not_a_literal_tail", func(t *testing.T) {
		tmp := t.TempDir()
		blocked := filepath.Join(tmp, "blocked")
		if err := os.MkdirAll(filepath.Join(blocked, "child"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if !chmodDeniesReading(t, blocked) {
			t.Skip("mode 0000 does not deny reading here; the premise of this case does not hold")
		}

		got, err := ResolvePathKeepingMissing(filepath.Join(blocked, "child"))
		if err == nil {
			t.Fatalf("a permission denial must be a genuine failure, got %q and no error", got)
		}
		if errors.Is(err, fs.ErrNotExist) {
			t.Errorf("a permission denial must not be reported as ErrNotExist, got %v", err)
		}
	})

	t.Run("containment_is_refused_not_silently_granted", func(t *testing.T) {
		base := t.TempDir()
		root := filepath.Join(base, "project")
		outside := filepath.Join(base, "outside")
		blocked := filepath.Join(root, "blocked")
		if err := os.MkdirAll(blocked, 0o755); err != nil {
			t.Fatalf("mkdir blocked: %v", err)
		}
		if err := os.MkdirAll(outside, 0o755); err != nil {
			t.Fatalf("mkdir outside: %v", err)
		}
		if err := os.Symlink(outside, filepath.Join(blocked, "esc")); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		if !chmodDeniesReading(t, blocked) {
			t.Skip("mode 0000 does not deny reading here; the premise of this case does not hold")
		}

		inside, err := ResolvedWithinRoot(root, filepath.Join(blocked, "esc", "x", "SKILL.md"))
		if err == nil && inside {
			t.Fatal("a symlink escaping the root behind an unreadable directory must never be reported as contained")
		}
	})
}

// TestResolvedWithinRootUsingRootResolutionFailureFailsClosed proves the
// root-resolution error branch. WithinRoot now refuses an empty root on its
// own, so this branch is defence in depth: it turns a failed resolution into
// the resolver's own error instead of a bare false verdict. The resolver's
// error must reach the caller unchanged.
func TestResolvedWithinRootUsingRootResolutionFailureFailsClosed(t *testing.T) {
	boom := errors.New("resolver refused the root")
	const root = "/project"

	resolve := func(p string) (string, error) {
		if p == root {
			return "", boom
		}
		return p, nil
	}

	// Precondition: WithinRoot refuses an empty root by itself, so the
	// explicit branch below is defence in depth, not the only guard.
	if WithinRoot("", "/anywhere/at/all") {
		t.Fatal("WithinRoot(\"\", p) must refuse an empty root")
	}

	inside, err := ResolvedWithinRootUsing(resolve, root, "/anywhere/at/all")
	if err == nil {
		t.Fatal("a root that cannot be resolved must be an error, not a containment verdict")
	}
	if !errors.Is(err, boom) {
		t.Errorf("error = %v, want the resolver's own failure propagated", err)
	}
	if inside {
		t.Error("a failed root resolution must never report the path as contained")
	}
}

// TestResolvedWithinRootUsingEmptyResolutionIsRefused pins the
// belt-and-braces half: even if a resolver returns an empty path with no
// error, the guard must not degrade into WithinRoot("", p).
func TestResolvedWithinRootUsingEmptyResolutionIsRefused(t *testing.T) {
	cases := map[string]func(string) (string, error){
		"empty_root": func(p string) (string, error) {
			if p == "/project" {
				return "", nil
			}
			return p, nil
		},
		"empty_path": func(p string) (string, error) {
			if p == "/project" {
				return p, nil
			}
			return "", nil
		},
	}

	for name, resolve := range cases {
		t.Run(name, func(t *testing.T) {
			inside, err := ResolvedWithinRootUsing(resolve, "/project", "/anywhere/at/all")
			if err == nil {
				t.Fatal("an empty resolution must be refused, not treated as a path")
			}
			if inside {
				t.Error("an empty resolution must never report the path as contained")
			}
		})
	}
}

// TestResolvedWithinRootUsingPathResolutionFailureFailsClosed proves the
// destination-resolution branch also propagates rather than falling through
// to a containment verdict computed from a zero value.
func TestResolvedWithinRootUsingPathResolutionFailureFailsClosed(t *testing.T) {
	sentinel := errors.New("the destination could not be resolved")
	const root = "/project"
	resolve := func(p string) (string, error) {
		if p == root {
			return root, nil
		}
		return "", sentinel
	}

	inside, err := ResolvedWithinRootUsing(resolve, root, "/project/.claude/skills/x/SKILL.md")
	if err == nil {
		t.Fatalf("a destination that cannot be resolved must fail closed, got inside=%v and no error", inside)
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error %v does not carry the resolver's own failure", err)
	}
	if inside {
		t.Error("a failed resolution must never report containment")
	}
}

// chmodDeniesReading chmods dir to 0000 (restoring it at cleanup) and reports
// whether that actually denies reading it here.
func chmodDeniesReading(t *testing.T, dir string) bool {
	t.Helper()
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if _, err := os.ReadDir(dir); err == nil {
		return false
	}
	return true
}
