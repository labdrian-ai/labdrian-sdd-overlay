package skills

import "path/filepath"

// ApprovalRecordStore is where the evidence of an approval is read from: the files of a global
// skill, and the record beside them that a human approved exactly those. The domain decides where
// each lives (SkillDirPath, ApprovalRecordPath), what an approval covers (SkillDigest) and what a
// missing or unreadable one means; the store only reads. The verbs that judge approval (validate,
// add, approve) read through it, so a test or another medium can stand where the files of an
// overlay stand.
type ApprovalRecordStore interface {
	// ReadSkillTree reads every entry below the directory of the skill whose directory is path
	// under sourceRoot that is not a directory: a regular file with its bytes and whether its owner
	// execute bit is set (SkillEntry.Executable, read from the file the bytes were), anything else
	// (a link, a pipe, a socket, a device) with its type and no bytes. A link is reported and not
	// followed, and nothing is left out: what an approval covers is the domain's to decide, not
	// the store's. The one exception to the bytes is the approval record at the root of the skill
	// (ApprovalRecordName), which is reported like the rest but not read, because no approval
	// covers it (SkillDigest) and ReadRecord is what reads it, so that a record that cannot be read
	// is told as that and not as a skill that cannot be read. The order is the store's. An error
	// that is fs.ErrNotExist means the directory is not there, which is an answer; any other means
	// it could not be read, and the caller refuses rather than take it for a skill with no files.
	ReadSkillTree(sourceRoot, path string) ([]SkillEntry, error)
	// ReadRecord reads the approval record of skill id under sourceRoot. An error that is
	// fs.ErrNotExist means the skill has no record, which is an answer; any other means the record
	// could not be read, and the caller refuses rather than take it for no record.
	ReadRecord(sourceRoot, id string) ([]byte, error)
}

// SkillDirPath returns where the directory of the skill whose directory is path lives under
// sourceRoot.
func SkillDirPath(sourceRoot, path string) string {
	return filepath.Join(sourceRoot, path)
}

// skillMDName is the file a skill is, by name, at the root of its directory.
const skillMDName = "SKILL.md"

// SkillMDEntry returns the entry named SKILL.md at the root of the skill, whatever it is, and
// whether there is one. A link in its place is found: it is a finding of the approval (it has no
// digest), not a skill whose file is missing. Its Data is the bytes of the file only when its
// Type says it is a regular one.
func SkillMDEntry(entries []SkillEntry) (SkillEntry, bool) {
	for _, e := range entries {
		if e.Rel == skillMDName {
			return e, true
		}
	}
	return SkillEntry{}, false
}
