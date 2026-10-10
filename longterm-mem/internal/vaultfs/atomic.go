package vaultfs

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/durable"
)

// createPerm is the mode a file gets when this package is the one creating it. What it writes carries memory
// content, or fingerprints of it, so a file this module brings into existence starts owner-only. It applies
// to creation only: a file that already exists keeps whatever mode its owner gave it.
const createPerm = 0o600

// writeFileAtomic durably replaces path with data, MkdirAll'ing the parent directory first. Everything it
// touches lives inside the user's vault, which is routinely tracked by git and routinely reached through a
// symlink, so it replaces the file the way durable.WriteFile does: through the link, keeping the mode of the
// file that is there, and by rename, so a reader never sees half a file.
//
// The errors carry the prefix promotion has always given them, "promote: ": they reach the user through
// commands whose output is part of what the refactoring that put this package here must not change.
func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("promote: create directory for %s: %w", path, err)
	}
	if err := durable.WriteFile(path, data, createPerm); err != nil {
		return fmt.Errorf("promote: %w", err)
	}
	return nil
}
