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

// TestResolvePathKeepingMissing_PermissionDenialIsAGenuineFailure is COV-1:
// the `if !os.IsNotExist(err)` branch of resolvePathKeepingMissing had no
// witness. The symlink-loop case does NOT reach it — with that branch deleted,
// the Lstat probe below still catches the loop — so only a non-ENOENT failure
// that Lstat ALSO cannot see proves it. An unreadable (mode 0000) parent
// directory is exactly that: EvalSymlinks and Lstat both fail with EACCES, the
// walk then climbs to the readable parent and answers with a literal tail, and
// the containment proof built on that answer says "inside" for a symlink that
// escapes the root.
func TestResolvePathKeepingMissing_PermissionDenialIsAGenuineFailure(t *testing.T) {
	t.Run("unreadable_parent_is_an_error_not_a_literal_tail", func(t *testing.T) {
		tmp := t.TempDir()
		blocked := filepath.Join(tmp, "blocked")
		if err := os.MkdirAll(filepath.Join(blocked, "child"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if !chmodDeniesReading(t, blocked) {
			t.Skip("mode 0000 does not deny reading here; the premise of this case does not hold")
		}

		got, err := resolvePathKeepingMissing(filepath.Join(blocked, "child"))
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
		// The escape lives BEHIND the unreadable directory, so nothing can see
		// it once the walk is allowed to degrade into a literal tail.
		if err := os.Symlink(outside, filepath.Join(blocked, "esc")); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		if !chmodDeniesReading(t, blocked) {
			t.Skip("mode 0000 does not deny reading here; the premise of this case does not hold")
		}

		inside, err := resolvedWithinRoot(root, filepath.Join(blocked, "esc", "x", "SKILL.md"))
		if err == nil && inside {
			t.Fatal("a symlink escaping the root behind an unreadable directory must never be reported as contained")
		}
	})
}

// TestResolvedWithinRootUsing_RootResolutionFailureFailsClosed is COV-4. The
// root-resolution error branch had no witness. It was once the only thing
// keeping a failed resolution from reaching withinRoot("", p), which used to
// be true for every absolute path. withinRoot now refuses an empty root on its
// own, so the branch is defence in depth: it turns the failure into the
// resolver's own error instead of a bare false verdict. That error must reach
// the caller unchanged (errors.Is), which the belt-and-braces empty-path guard
// alone would not satisfy.
func TestResolvedWithinRootUsing_RootResolutionFailureFailsClosed(t *testing.T) {
	boom := errors.New("resolver refused the root")
	const root = "/project"

	resolve := func(p string) (string, error) {
		if p == root {
			return "", boom
		}
		return p, nil
	}

	// Precondition: the lexical helper refuses an empty root by itself, so
	// the explicit branch below is defence in depth, not the only guard.
	if withinRoot("", "/anywhere/at/all") {
		t.Fatal("withinRoot(\"\", p) must refuse an empty root")
	}

	inside, err := resolvedWithinRootUsing(resolve, root, "/anywhere/at/all")
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

// TestResolvedWithinRootUsing_EmptyResolutionIsRefused pins the belt-and-braces
// half of the same COV-4 finding: even if a resolver returns an empty path with
// no error, the guard must not degrade into withinRoot("", p), which is true for
// every absolute path.
func TestResolvedWithinRootUsing_EmptyResolutionIsRefused(t *testing.T) {
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
			inside, err := resolvedWithinRootUsing(resolve, "/project", "/anywhere/at/all")
			if err == nil {
				t.Fatal("an empty resolution must be refused, not treated as a path")
			}
			if inside {
				t.Error("an empty resolution must never report the path as contained")
			}
		})
	}
}

// TestResolvedWithinRootUsing_PathResolutionFailureFailsClosed is COV-B
// (review round 2, carried to slice 3b-ii). COV-4 witnessed the ROOT
// resolution branch with a resolver that fails on the root only; the
// DESTINATION branch had no witness at all. Both must propagate rather than
// fall through to a containment verdict computed from a zero value.
func TestResolvedWithinRootUsing_PathResolutionFailureFailsClosed(t *testing.T) {
	sentinel := errors.New("the destination could not be resolved")
	const root = "/project"
	resolve := func(p string) (string, error) {
		if p == root {
			return root, nil
		}
		return "", sentinel
	}

	inside, err := resolvedWithinRootUsing(resolve, root, "/project/.claude/skills/x/SKILL.md")
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
//
// It replaces the `os.Geteuid() == 0` probe the two cases above used to share
// (portability advisory, slice 3b-ii). A non-root euid does NOT imply mode
// 0000 denies: CAP_DAC_OVERRIDE on an ordinary user, several container and CI
// user setups, and filesystems that ignore mode bits all read the directory
// anyway. In those environments the first case failed spuriously and the
// second — whose assertion is the lenient `err == nil && inside` — passed
// vacuously. Only the behaviour itself can tell the two apart.
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
