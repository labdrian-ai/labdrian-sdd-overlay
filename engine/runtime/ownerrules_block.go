package runtime

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sha256hex"
)

// MarkedBlock is a region of a text file that labdrian owns, delimited by two marker lines. The
// file is the owner's too, and an installer writes it: everything outside the two lines is never
// read for meaning and never changed (apart from the separator Apply adds when it appends, which
// Remove takes away again), so the owner's text, and the blocks other tools keep in the same file,
// survive every install, update and uninstall byte for byte.
//
// Any adapter that keeps a block of labdrian text in a file the owner also edits uses this type:
// the Claude adapter for the owner rules in CLAUDE.md, and the adapters that write AGENTS.md files.
type MarkedBlock struct {
	// Open and Close are the two marker lines. A line is a marker when it equals the text, a
	// carriage return and trailing spaces or tabs aside.
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

// beforeBlockRename is called by the file writer after the new file is complete and before it checks
// that the old one is unchanged. A test uses it to change the file at that moment.
var beforeBlockRename = func(path string) {}

// lineEnding is the line ending of content: CRLF when its first line ends with one, LF otherwise.
func lineEnding(content string) string {
	if i := strings.Index(content, "\n"); i > 0 && content[i-1] == '\r' {
		return "\r\n"
	}
	return "\n"
}

// render is the block as written for body with the line ending eol: the open line, the body
// without trailing newlines, the close line. It has no newline after the close line.
func (b MarkedBlock) render(body, eol string) string {
	body = strings.ReplaceAll(strings.TrimRight(body, "\r\n"), "\r\n", "\n")
	return b.Open + eol + strings.ReplaceAll(body, "\n", eol) + eol + b.Close
}

// locate finds the block in content: start is the offset of its open line and end the offset just
// after its close line, not counting that line's line ending.
func (b MarkedBlock) locate(content string) (start, end int, found bool, err error) {
	var opens, closes []int
	closeFirst := false
	offset := 0
	for n, line := range strings.SplitAfter(content, "\n") {
		text := strings.TrimRight(line, "\r\n")
		switch strings.TrimRight(text, " \t") {
		case b.Open:
			if len(opens) == 0 {
				start = offset
			}
			opens = append(opens, n+1)
		case b.Close:
			if len(opens) == 0 {
				closeFirst = true
			}
			if len(closes) == 0 {
				end = offset + len(text)
			}
			closes = append(closes, n+1)
		}
		offset += len(line)
	}
	switch {
	case len(opens) == 0 && len(closes) == 0:
		return 0, 0, false, nil
	case len(opens) != 1 || len(closes) != 1 || closeFirst:
		detail := "open: " + lineNumbers(opens) + "; close: " + lineNumbers(closes)
		if closeFirst {
			detail += "; the close line comes first"
		}
		return 0, 0, false, fmt.Errorf("%w (%s)", errBlockMarkers, detail)
	}
	return start, end, true, nil
}

// lineNumbers says which lines a marker was found on.
func lineNumbers(lines []int) string {
	switch len(lines) {
	case 0:
		return "none"
	case 1:
		return "line " + strconv.Itoa(lines[0])
	}
	numbers := make([]string, len(lines))
	for i, n := range lines {
		numbers[i] = strconv.Itoa(n)
	}
	return "lines " + strings.Join(numbers, ", ")
}

// Apply is content with the block holding body, in the line ending of the content: replaced in
// place when it is there, appended when it is not. An append is separated from the text before it by
// one blank line; a content that ends without a newline is not given one at its end, so Remove can
// give the content back as it was. change says which; the content is returned as it was for
// BlockUnchanged and on an error.
func (b MarkedBlock) Apply(content, body string) (updated string, change BlockChange, err error) {
	eol := lineEnding(content)
	block := b.render(body, eol)
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
		return block + eol, BlockAdded, nil
	case strings.HasSuffix(content, "\n"):
		return content + eol + block + eol, BlockAdded, nil
	}
	return content + eol + eol + block, BlockAdded, nil
}

// Remove is content without the block: its two lines and the line ending after them. When the block
// is the last thing in the content, as Apply leaves it, the blank line Apply put before it goes
// too, so an Apply followed by a Remove gives the content back byte for byte. A block that text
// follows is removed alone: the blank lines around it are the owner's. removed is false when the
// block is not there; the content is returned as it was on an error.
func (b MarkedBlock) Remove(content string) (updated string, removed bool, err error) {
	start, end, found, err := b.locate(content)
	if err != nil {
		return content, false, err
	}
	if !found {
		return content, false, nil
	}
	eol := lineEnding(content)
	blockEnd, trailing := end, false
	switch {
	case strings.HasPrefix(content[end:], "\r\n"):
		blockEnd, trailing = end+2, true
	case strings.HasPrefix(content[end:], "\n"):
		blockEnd, trailing = end+1, true
	}
	if blockEnd == len(content) && strings.HasSuffix(content[:start], eol+eol) {
		if trailing {
			start -= len(eol)
		} else {
			start -= 2 * len(eol)
		}
	}
	return content[:start] + content[blockEnd:], true, nil
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
	case content[start:end] == b.render(body, lineEnding(content)):
		return BlockCurrent, nil
	}
	return BlockStale, nil
}

// WriteFile makes the block at path hold body, creating the file (and its directory) when it is
// absent. A file that is already right is not written. A write replaces the file whole through a
// rename in its directory, keeps its mode, follows a symbolic link to the file it names, and is
// refused when the file changed after it was read. A symbolic link to a file that does not exist is
// refused too.
func (b MarkedBlock) WriteFile(path, body string) (BlockChange, error) {
	file, err := readBlockFile(path)
	if err != nil {
		return BlockUnchanged, err
	}
	updated, change, err := b.Apply(file.content, body)
	if err != nil {
		return BlockUnchanged, fmt.Errorf("%s: %w", path, err)
	}
	if change == BlockUnchanged {
		return change, nil
	}
	if err := writeBlockFile(path, updated, file); err != nil {
		return BlockUnchanged, err
	}
	return change, nil
}

// RemoveFile removes the block from the file at path. A file that is absent, or has no block, is
// left alone and removed is false. A file left with nothing in it is removed: it held only the
// block, and it was created for it.
func (b MarkedBlock) RemoveFile(path string) (removed bool, err error) {
	file, err := readBlockFile(path)
	if err != nil {
		return false, err
	}
	updated, removed, err := b.Remove(file.content)
	if err != nil {
		return false, fmt.Errorf("%s: %w", path, err)
	}
	if !removed {
		return false, nil
	}
	if updated == "" {
		if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() {
			return true, removeBlockFile(path, file)
		}
	}
	return true, writeBlockFile(path, updated, file)
}

// StateOfFile is State for the file at path; a file that is absent has no block.
func (b MarkedBlock) StateOfFile(path, body string) (BlockState, error) {
	file, err := readBlockFile(path)
	if err != nil {
		return BlockMissing, err
	}
	state, err := b.State(file.content, body)
	if err != nil {
		return BlockMissing, fmt.Errorf("%s: %w", path, err)
	}
	return state, nil
}

// defaultBlockFileMode is the mode of a file the installer creates: what the owner's editor would.
const defaultBlockFileMode os.FileMode = 0o644

// blockFile is a file as it was read: its text and mode, and whether it was there.
type blockFile struct {
	content string
	mode    os.FileMode
	existed bool
}

// readBlockFile reads the file at path. A file that is absent has no text and the default mode.
func readBlockFile(path string) (blockFile, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		if link, lerr := os.Lstat(path); lerr == nil && link.Mode()&os.ModeSymlink != 0 {
			return blockFile{}, fmt.Errorf("%s is a symbolic link to a file that does not exist; it was left as it is", path)
		}
		return blockFile{mode: defaultBlockFileMode}, nil
	}
	if err != nil {
		return blockFile{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return blockFile{}, err
	}
	return blockFile{content: string(data), mode: info.Mode().Perm(), existed: true}, nil
}

// unchangedSince checks that the file at target is as it was read: the same bytes, or still absent.
func unchangedSince(target string, read blockFile) error {
	beforeBlockRename(target)
	now, err := os.ReadFile(target)
	switch {
	case !read.existed && os.IsNotExist(err):
		return nil
	case read.existed && err == nil && sha256hex.Sum(now) == sha256hex.Sum([]byte(read.content)):
		return nil
	}
	return fmt.Errorf("%s changed while it was being written; nothing was written", target)
}

// writeBlockFile writes content to the file path names, through a temporary file in its directory
// that is synced and renamed over the file, so a reader sees the old file or the new one and a
// failure leaves the old one. It refuses when the file is not as read (a lost edit is worse than a
// refused write).
func writeBlockFile(path, content string, read blockFile) error {
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
	if err := tmp.Chmod(read.mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := unchangedSince(target, read); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), target)
}

// removeBlockFile removes the file at path when it is as read.
func removeBlockFile(path string, read blockFile) error {
	if err := unchangedSince(path, read); err != nil {
		return err
	}
	return os.Remove(path)
}
