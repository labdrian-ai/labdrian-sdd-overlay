package skills

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// SkipWhenCopying is the rule every whole-directory copier of a skill source tree
// shares: it reports whether the walked entry is left out. rel is the entry's path
// relative to the skill directory, in the walker's own separators.
//
// Two things are never copied, whatever else a copier refuses or skips:
//
//   - the approval record (ApprovalRecordName at the root of the skill), which is
//     repository governance state, not skill content; and
//   - a writer's temporary file, which is half of a write another verb is doing in
//     the source tree (see isWriterTempFile).
//
// A directory the rule skips is skipped whole: the caller returns filepath.SkipDir.
// Exported so that engine/pipkg, which builds the Pi package from the same trees,
// cannot drift from install.
func SkipWhenCopying(rel string, d fs.DirEntry) bool {
	if rel == ApprovalRecordName {
		return true
	}
	return isWriterTempFile(d)
}

// SourceFile is one regular file of a skill's source tree: its path relative to the
// skill directory (slash-separated), its bytes, and its permission bits.
type SourceFile struct {
	Rel  string
	Data []byte
	Mode fs.FileMode
}

// readSkillSource reads the files install would install from one skill directory,
// sorted by path. A symlink is not followed and not installed, and neither is
// anything that is not a regular file; a directory is only the way to its files, so
// an empty one installs nothing. The approval record and a writer's temporary file
// are skipped by the rule install shares with the Pi package builder.
func readSkillSource(dir string) ([]SourceFile, error) {
	var files []SourceFile
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		if rel != "." && SkipWhenCopying(rel, d) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return fmt.Errorf("reading %s: %w", p, err)
		}
		files = append(files, SourceFile{Rel: filepath.ToSlash(rel), Data: data, Mode: info.Mode().Perm()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Rel < files[j].Rel })
	return files, nil
}
