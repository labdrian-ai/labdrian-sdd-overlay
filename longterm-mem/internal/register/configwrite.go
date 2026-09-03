package register

import (
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/durable"
)

// configCreatePerm is the mode a runtime config (or its .bak) gets when
// longterm-mem is the one creating it. It only ever applies to a file that
// does not exist yet: an existing file keeps whatever mode its owner gave
// it (durable.WriteFile).
const configCreatePerm = 0o600

// backupPath is where a config's pre-edit bytes are kept.
func backupPath(path string) string { return path + ".bak" }

// replaceConfig is the single write sequence all four runtime-config
// writers share (WriteMember, RemoveMember, WriteTOMLSection,
// RemoveTOMLSection): back the original bytes up next to the config, then
// durably replace the config with replacement.
//
// Ordering is the point. The backup lands on disk BEFORE the replacement is
// committed, so there is never a moment where the config has been edited
// and no copy of the previous content exists. Both writes go through
// durable.WriteFile: the backup is the file a human reaches for when an
// edit went wrong, so writing it with a plain truncating os.WriteFile — as
// this package did — meant the recovery copy was the least durable file in
// the whole flow, and a crash mid-backup could leave it truncated.
//
// The backup deliberately sits next to the config path AS GIVEN, not next
// to the symlink's resolved target. A config symlinked into a dotfiles
// repository should not have longterm-mem's backups appearing as untracked
// files inside that repository; keeping the .bak beside the link leaves the
// user's tracked directory holding exactly the one file they track.
func replaceConfig(path string, original, replacement []byte) error {
	bak := backupPath(path)
	if err := durable.WriteFile(bak, original, configCreatePerm); err != nil {
		return fmt.Errorf("register: write backup %s: %w", bak, err)
	}
	if err := durable.WriteFile(path, replacement, configCreatePerm); err != nil {
		return fmt.Errorf("register: %w", err)
	}
	return nil
}
