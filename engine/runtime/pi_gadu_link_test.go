package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

// gaduLinkWorld is a link location and the target a link there should have, in a scratch
// directory. gaduLinkState looks at the file system and at nothing else: no home, no package.
func gaduLinkWorld(t *testing.T) (linkPath, targetPath string) {
	t.Helper()
	dir := t.TempDir()
	targetPath = filepath.Join(dir, "package", "agents", "GADU.md")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetPath, []byte("the package's GADU.md\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkPath = filepath.Join(dir, "home", ".pi", "agent", "agents", "GADU.md")
	return linkPath, targetPath
}

func symlinkAt(t *testing.T, linkPath, target string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, linkPath); err != nil {
		t.Fatal(err)
	}
}

// TestGaduLinkState_Matrix (task 5.2): missing/current/stale/conflict, both for a plain
// conflicting file and a symlink pointing elsewhere. It is an internal test of the unexported
// state because the function is not part of the adapter's surface: the adapter's own tests drive
// it through Install, Status and Uninstall.
func TestGaduLinkState_Matrix(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		linkPath, targetPath := gaduLinkWorld(t)
		if got := gaduLinkState(linkPath, targetPath); got != gaduLinkMissing {
			t.Fatalf("gaduLinkState = %q, want missing", got)
		}
	})
	t.Run("current", func(t *testing.T) {
		linkPath, targetPath := gaduLinkWorld(t)
		symlinkAt(t, linkPath, targetPath)
		if got := gaduLinkState(linkPath, targetPath); got != gaduLinkCurrent {
			t.Fatalf("gaduLinkState = %q, want current", got)
		}
	})
	t.Run("current when the target is spelled with a redundant element", func(t *testing.T) {
		linkPath, targetPath := gaduLinkWorld(t)
		// Join would clean the element away, so the target is written out by hand.
		symlinkAt(t, linkPath, filepath.Dir(targetPath)+string(filepath.Separator)+"."+string(filepath.Separator)+"GADU.md")
		if got := gaduLinkState(linkPath, targetPath); got != gaduLinkCurrent {
			t.Fatalf("gaduLinkState = %q, want current: ownership is Readlink equality after Clean", got)
		}
	})
	t.Run("stale (broken target)", func(t *testing.T) {
		linkPath, targetPath := gaduLinkWorld(t)
		symlinkAt(t, linkPath, targetPath)
		if err := os.Remove(targetPath); err != nil {
			t.Fatal(err)
		}
		if got := gaduLinkState(linkPath, targetPath); got != gaduLinkStale {
			t.Fatalf("gaduLinkState = %q, want stale", got)
		}
	})
	t.Run("conflict (regular file)", func(t *testing.T) {
		linkPath, targetPath := gaduLinkWorld(t)
		if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(linkPath, []byte("gentle-pi's own managed GADU.md\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := gaduLinkState(linkPath, targetPath); got != gaduLinkConflict {
			t.Fatalf("gaduLinkState = %q, want conflict", got)
		}
	})
	t.Run("conflict (symlink elsewhere)", func(t *testing.T) {
		linkPath, targetPath := gaduLinkWorld(t)
		elsewhere := filepath.Join(t.TempDir(), "elsewhere.md")
		if err := os.WriteFile(elsewhere, []byte("not ours\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		symlinkAt(t, linkPath, elsewhere)
		if got := gaduLinkState(linkPath, targetPath); got != gaduLinkConflict {
			t.Fatalf("gaduLinkState = %q, want conflict", got)
		}
	})
}
