package runtime

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MarkedBlock is a region of a text file that labdrian owns, delimited by two marker lines. The
// file is the owner's too, and an installer writes it: everything outside the two lines is never
// read for meaning and never changed, so the owner's text, and the blocks other tools keep in the
// same file, survive every install, update and uninstall byte for byte.
//
// The Claude adapter writes the owner rules between OwnerRulesBlock's markers in CLAUDE.md; the
// adapters that write the same rules into an AGENTS.md file reuse this type and its functions.
type MarkedBlock struct {
	// Open and Close are the two marker lines. A line is a marker when it equals the text, a
	// trailing carriage return aside.
	Open, Close string
}

// OwnerRulesBlock is the block the owner rules (instructions/owner-rules.md) are written in.
var OwnerRulesBlock = MarkedBlock{
	Open:  "<!-- labdrian:owner-rules -->",
	Close: "<!-- /labdrian:owner-rules -->",
}

// BlockState says how a block in a file compares with the text it should hold.
type BlockState int

const (
	// BlockMissing: the file has no block, or there is no file.
	BlockMissing BlockState = iota
	// BlockStale: the file has the block and it holds other text.
	BlockStale
	// BlockCurrent: the file has the block and it holds the text.
	BlockCurrent
)

// BlockChange says what writing a block did.
type BlockChange int

const (
	// BlockAdded: there was no block and one was appended (a missing file was created).
	BlockAdded BlockChange = iota
	// BlockReplaced: the block was there with other text and was replaced in place.
	BlockReplaced
	// BlockUnchanged: the block was there with this text and the file was not written.
	BlockUnchanged
)

// errBlockMarkers is the error of a file whose markers do not form one pair. Nothing is written to
// such a file: which text the owner means to keep is not for the installer to guess.
var errBlockMarkers = errors.New("its markers are not one open line followed by one close line")

// render is the block as written for body: the open line, the body without trailing newlines, the
// close line. It has no newline after the close line.
func (b MarkedBlock) render(body string) string {
	return b.Open + "\n" + strings.TrimRight(body, "\n") + "\n" + b.Close
}

// locate finds the block in content: start is the offset of its open line and end the offset just
// after its close line, not counting that line's newline.
func (b MarkedBlock) locate(content string) (start, end int, found bool, err error) {
	opens, closes := 0, 0
	closeBeforeOpen := false
	offset := 0
	for _, line := range strings.SplitAfter(content, "\n") {
		text := strings.TrimRight(line, "\r\n")
		switch text {
		case b.Open:
			if opens == 0 {
				start = offset
			}
			opens++
		case b.Close:
			if opens == 0 {
				closeBeforeOpen = true
			}
			if closes == 0 {
				end = offset + len(text)
			}
			closes++
		}
		offset += len(line)
	}
	switch {
	case opens == 0 && closes == 0:
		return 0, 0, false, nil
	case opens != 1 || closes != 1 || closeBeforeOpen:
		return 0, 0, false, errBlockMarkers
	}
	return start, end, true, nil
}

// Apply is content with the block holding body: replaced in place when it is there, appended when it
// is not (after one blank line, and after a newline when the content has none at its end). change
// says which; the content is returned as it was for BlockUnchanged and on an error.
func (b MarkedBlock) Apply(content, body string) (updated string, change BlockChange, err error) {
	block := b.render(body)
	start, end, found, err := b.locate(content)
	if err != nil {
		return content, BlockUnchanged, err
	}
	if found {
		updated = content[:start] + block + content[end:]
		if updated == content {
			return content, BlockUnchanged, nil
		}
		return updated, BlockReplaced, nil
	}
	switch {
	case content == "":
	case strings.HasSuffix(content, "\n"):
		content += "\n"
	default:
		content += "\n\n"
	}
	return content + block + "\n", BlockAdded, nil
}

// Remove is content without the block: its two lines, the newline after them and one blank line that
// separated it from the text before, so the append of Apply is undone exactly. removed is false when
// the block is not there; the content is returned as it was on an error.
func (b MarkedBlock) Remove(content string) (updated string, removed bool, err error) {
	start, end, found, err := b.locate(content)
	if err != nil {
		return content, false, err
	}
	if !found {
		return content, false, nil
	}
	switch {
	case strings.HasPrefix(content[end:], "\r\n"):
		end += 2
	case strings.HasPrefix(content[end:], "\n"):
		end++
	}
	switch {
	case strings.HasSuffix(content[:start], "\n\n"):
		start--
	case start == 0 && strings.HasPrefix(content[end:], "\n"):
		end++
	}
	return content[:start] + content[end:], true, nil
}

// State compares the block of content with the one body would give.
func (b MarkedBlock) State(content, body string) (BlockState, error) {
	start, end, found, err := b.locate(content)
	if err != nil {
		return BlockMissing, err
	}
	switch {
	case !found:
		return BlockMissing, nil
	case content[start:end] == b.render(body):
		return BlockCurrent, nil
	}
	return BlockStale, nil
}

// WriteFile makes the block at path hold body, creating the file (and its directory) when it is
// absent. A file that is already right is not written. A write replaces the file whole through a
// rename in its directory, keeps its mode, and follows a symbolic link to the file it names.
func (b MarkedBlock) WriteFile(path, body string) (BlockChange, error) {
	content, mode, err := readBlockFile(path)
	if err != nil {
		return BlockUnchanged, err
	}
	updated, change, err := b.Apply(content, body)
	if err != nil {
		return BlockUnchanged, fmt.Errorf("%s: %w", path, err)
	}
	if change == BlockUnchanged {
		return change, nil
	}
	if err := writeBlockFile(path, updated, mode); err != nil {
		return BlockUnchanged, err
	}
	return change, nil
}

// RemoveFile removes the block from the file at path. A file that is absent, or has no block, is
// left alone and removed is false. A file left with nothing in it stays as an empty file.
func (b MarkedBlock) RemoveFile(path string) (removed bool, err error) {
	content, mode, err := readBlockFile(path)
	if err != nil {
		return false, err
	}
	updated, removed, err := b.Remove(content)
	if err != nil {
		return false, fmt.Errorf("%s: %w", path, err)
	}
	if !removed {
		return false, nil
	}
	return true, writeBlockFile(path, updated, mode)
}

// StateOfFile is State for the file at path; a file that is absent has no block.
func (b MarkedBlock) StateOfFile(path, body string) (BlockState, error) {
	content, _, err := readBlockFile(path)
	if err != nil {
		return BlockMissing, err
	}
	state, err := b.State(content, body)
	if err != nil {
		return BlockMissing, fmt.Errorf("%s: %w", path, err)
	}
	return state, nil
}

// defaultBlockFileMode is the mode of a file the installer creates: what the owner's editor would.
const defaultBlockFileMode os.FileMode = 0o644

// readBlockFile is the text and mode of the file at path, or no text and the default mode when the
// file is absent.
func readBlockFile(path string) (string, os.FileMode, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return "", defaultBlockFileMode, nil
	}
	if err != nil {
		return "", 0, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", 0, err
	}
	return string(data), info.Mode().Perm(), nil
}

// writeBlockFile writes content to the file path names, through a temporary file in its directory
// and a rename, so a reader sees the old file or the new one and a failure leaves the old one.
func writeBlockFile(path, content string, mode os.FileMode) error {
	target := path
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		target = resolved
	}
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(target)+".labdrian-")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), target)
}
