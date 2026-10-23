package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

// agentLinkWorld is a link location and the target a link there should have, in a scratch
// directory. agentLinkState looks at the file system and at nothing else: no home, no package.
func agentLinkWorld(t *testing.T) (linkPath, targetPath string) {
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

// TestAgentLinkState_Matrix (task 5.2): missing/current/stale/conflict, both for a plain
// conflicting file and a symlink pointing elsewhere. It is an internal test of the unexported
// state because the function is not part of the adapter's surface: the adapter's own tests drive
// it through Install, Status and Uninstall.
func TestAgentLinkState_Matrix(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		linkPath, targetPath := agentLinkWorld(t)
		if got := agentLinkState(linkPath, targetPath); got != agentLinkMissing {
			t.Fatalf("agentLinkState = %q, want missing", got)
		}
	})
	t.Run("current", func(t *testing.T) {
		linkPath, targetPath := agentLinkWorld(t)
		symlinkAt(t, linkPath, targetPath)
		if got := agentLinkState(linkPath, targetPath); got != agentLinkCurrent {
			t.Fatalf("agentLinkState = %q, want current", got)
		}
	})
	t.Run("current when the target is spelled with a redundant element", func(t *testing.T) {
		linkPath, targetPath := agentLinkWorld(t)
		// Join would clean the element away, so the target is written out by hand.
		symlinkAt(t, linkPath, filepath.Dir(targetPath)+string(filepath.Separator)+"."+string(filepath.Separator)+"GADU.md")
		if got := agentLinkState(linkPath, targetPath); got != agentLinkCurrent {
			t.Fatalf("agentLinkState = %q, want current: ownership is Readlink equality after Clean", got)
		}
	})
	t.Run("stale (broken target)", func(t *testing.T) {
		linkPath, targetPath := agentLinkWorld(t)
		symlinkAt(t, linkPath, targetPath)
		if err := os.Remove(targetPath); err != nil {
			t.Fatal(err)
		}
		if got := agentLinkState(linkPath, targetPath); got != agentLinkStale {
			t.Fatalf("agentLinkState = %q, want stale", got)
		}
	})
	t.Run("conflict (regular file)", func(t *testing.T) {
		linkPath, targetPath := agentLinkWorld(t)
		if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(linkPath, []byte("gentle-pi's own managed GADU.md\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := agentLinkState(linkPath, targetPath); got != agentLinkConflict {
			t.Fatalf("agentLinkState = %q, want conflict", got)
		}
	})
	t.Run("conflict (symlink elsewhere)", func(t *testing.T) {
		linkPath, targetPath := agentLinkWorld(t)
		elsewhere := filepath.Join(t.TempDir(), "elsewhere.md")
		if err := os.WriteFile(elsewhere, []byte("not ours\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		symlinkAt(t, linkPath, elsewhere)
		if got := agentLinkState(linkPath, targetPath); got != agentLinkConflict {
			t.Fatalf("agentLinkState = %q, want conflict", got)
		}
	})
}
