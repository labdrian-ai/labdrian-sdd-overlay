package skillsfs

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/statestore"
)

// Approvals is the skills.ApprovalRecordStore of the real file system: the files of a skill and
// the approval record beside it are read from the paths the domain names (SkillDirPath and
// ApprovalRecordPath). It holds nothing: the zero value is ready, and it returns the failure of
// the system as it is, so a missing directory or record is an error that is fs.ErrNotExist and the
// words of every other refusal are the system's.
type Approvals struct{}

var _ skills.ApprovalRecordStore = Approvals{}

// ReadSkillTree reads every entry below the directory of the skill whose directory is path under
// sourceRoot that is not a directory. A regular file comes with its bytes; anything else (a link, a
// pipe, a socket, a device) comes with its type and no bytes, and is neither followed nor opened,
// so a pipe cannot block the read. Nothing is left out, dot-prefixed names included: what an
// approval covers is the domain's to decide (skills.SkillDigest). The approval record at the root
// is reported but not read: no approval covers its bytes, and ReadRecord is what reads it.
//
// It is not what install reads (Tree.ReadSkillSource), which leaves out what it does not install.
// The directory of the skill is not followed either when it is a link: that is an error, not a
// skill with the files of another directory.
func (Approvals) ReadSkillTree(sourceRoot, path string) ([]skills.SkillEntry, error) {
	dir := skills.SkillDirPath(sourceRoot, path)
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory (a symbolic link is not followed)", filepath.ToSlash(dir))
	}

	var entries []skills.SkillEntry
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		entry := skills.SkillEntry{Rel: filepath.ToSlash(rel), Type: d.Type()}
		if entry.Type.IsRegular() && entry.Rel != skills.ApprovalRecordName {
			if entry.Type, entry.Data, err = readRegular(p); err != nil {
				return err
			}
		}
		entries = append(entries, entry)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// readRegular reads the file the listing said was regular, through one descriptor opened without
// following a link and without blocking on a pipe. An entry swapped for a link or a pipe after the
// listing is reported with the type it has now, with no bytes, so the domain refuses it rather
// than reading what it points at.
func readRegular(p string) (fs.FileMode, []byte, error) {
	f, err := statestore.OpenNoFollow(p)
	if err != nil {
		if statestore.IsSymlinkRefusal(err) {
			return fs.ModeSymlink, nil, nil
		}
		return 0, nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return 0, nil, err
	}
	if !info.Mode().IsRegular() {
		return info.Mode().Type(), nil, nil
	}
	data, err := io.ReadAll(f)
	return 0, data, err
}

// ReadRecord reads the approval record of skill id under sourceRoot.
func (Approvals) ReadRecord(sourceRoot, id string) ([]byte, error) {
	return os.ReadFile(skills.ApprovalRecordPath(sourceRoot, id))
}
