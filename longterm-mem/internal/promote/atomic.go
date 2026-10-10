package promote

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/durable"
)

// fileCreatePerm is the mode a file of the vault gets when promote is the one
// creating it: the catalog, the log and the sync-state record. They carry
// memory content, or the trace of it, so a file this module brings into
// existence starts owner-only. It applies to creation only: a file that
// already exists keeps whatever mode its owner gave it.
const fileCreatePerm = 0o600

// writeFileAtomic durably replaces path with data, MkdirAll'ing the parent
// directory first. The pages are no longer written here: they go through the
// PageRepository the vault file system adapter satisfies, which replaces them
// the same way. What still comes through this function is what the next L3
// slice moves behind a port, wiki/index.md, wiki/log.md and the sync-state
// record, and it goes with them.
//
// Everything it touches lives inside the USER'S Obsidian vault, not inside
// longterm-mem's own state directory, and the callers rewrite files that
// already exist. So this is not a writer of module-owned files that can pick
// its own permissions: a vault file is routinely 0o644, routinely tracked by
// git, and in a synced or dotfiles-style vault routinely reached through a
// symlink.
//
// It used to hand-roll tmp+fsync+rename, which made every file it touched
// come back as a fresh os.CreateTemp inode: 0o644 silently became 0o600,
// and a symlinked file was replaced by a regular file, leaving the user's
// real note unedited while promote reported success. durable.WriteFile
// resolves the link and carries the mode across; see its doc comment for
// the one identity property (hardlinks) deliberately traded for atomicity.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("promote: create directory for %s: %w", path, err)
	}
	if err := durable.WriteFile(path, data, fileCreatePerm); err != nil {
		return fmt.Errorf("promote: %w", err)
	}
	return nil
}
