package skills

import (
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sha256hex"
)

// SkillEntry is one entry below the directory of a skill that is not a directory itself, as a store
// saw it without following a link: the unit an approval is made of. A directory is only the way to
// its entries, so an empty one is not reported and is not covered.
type SkillEntry struct {
	// Rel is the path of the entry relative to the skill directory, with forward slashes.
	Rel string
	// Type holds the type bits of the entry (fs.ModeType): none for a regular file, fs.ModeSymlink
	// for a link, and so on for a pipe, a socket or a device.
	Type fs.FileMode
	// Data is the content of a regular file. It is empty for any other entry: a store does not read
	// through a link. It may be empty too for the approval record at the root of the skill, which
	// a digest leaves out whatever it holds.
	Data []byte
}

// SkillDigest returns the digest an approval binds: the lowercase hex SHA-256 of the list that
// `sha256sum` writes for the files of the skill. The list has one line for each regular file below
// the skill directory, however deep, in the order of their paths as bytes: the SHA-256 of the file
// bytes, two spaces, the path relative to the skill directory in slash form, and a newline. The
// digest of a skill can be made again by hand:
//
//	(cd skills/<id> && find . -type f ! -path ./.approval.json | sed 's|^\./||' |
//	  LC_ALL=C sort | xargs -d '\n' sha256sum -- | sha256sum)
//
// The approval record at the root of the skill is left out, because a digest cannot cover the file
// that holds it; a file of that name deeper in the tree is a file like any other. Nothing of a file
// but its path and its bytes is covered, with no normalization of line endings, a BOM or trailing
// whitespace: an approval binds the files, not an interpretation of them. The digest is of the list
// and not of the bytes joined, so that where one file ends and the next begins is part of what is
// approved.
//
// A skill whose directory holds an entry that is not a regular file, a link above all, has no
// digest: the bytes behind a link are not the skill's, and an approval that skipped it would hold
// while they changed. A name with a line break in it cannot be one line of the list, and is refused
// for the same reason, and so is one with a backslash, which sha256sum would escape. The error says every entry it refused, in the order of their paths.
func SkillDigest(entries []SkillEntry) (string, error) {
	ordered := append([]SkillEntry(nil), entries...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Rel < ordered[j].Rel })

	var problems []string
	var list strings.Builder
	for _, e := range ordered {
		switch {
		case !e.Type.IsRegular():
			problems = append(problems, fmt.Sprintf("%s is %s", e.Rel, entryKind(e.Type)))
		case strings.ContainsAny(e.Rel, "\r\n"):
			problems = append(problems, fmt.Sprintf("%q has a line break in its name", e.Rel))
		case strings.Contains(e.Rel, `\`):
			// sha256sum escapes such a name in its list, so the documented recipe could not
			// reproduce the digest.
			problems = append(problems, fmt.Sprintf("%q has a backslash in its name", e.Rel))
		case e.Rel == ApprovalRecordName:
			// The record is what holds the digest, so it is not part of it.
		default:
			list.WriteString(sha256hex.Sum(e.Data))
			list.WriteString("  ")
			list.WriteString(e.Rel)
			list.WriteByte('\n')
		}
	}
	if len(problems) > 0 {
		return "", errors.New(strings.Join(problems, "; "))
	}
	return sha256hex.Sum([]byte(list.String())), nil
}

// entryKind says what a type that is not a regular file is, for the sentence "<path> is <kind>".
func entryKind(t fs.FileMode) string {
	switch {
	case t&fs.ModeSymlink != 0:
		return "a symbolic link"
	case t&fs.ModeNamedPipe != 0:
		return "a named pipe"
	case t&fs.ModeSocket != 0:
		return "a socket"
	case t&fs.ModeDevice != 0:
		return "a device"
	default:
		return "not a regular file"
	}
}
