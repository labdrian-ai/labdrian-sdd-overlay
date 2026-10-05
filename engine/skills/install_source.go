package skills

import "io/fs"

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
