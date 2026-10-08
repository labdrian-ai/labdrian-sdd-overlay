package pathguard

import (
	"errors"
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
