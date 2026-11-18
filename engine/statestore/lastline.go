package statestore

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
)

// AppendLine adds line and a newline to the file name under the directory parts (a state home followed by
// the components below it, as EnsureDirs takes them), making the directories and the file that are not there
// (0700 and 0600) and following no link. It is one write, so a line is never split between two. The file is
// only ever appended to: Publish never replaces a record, so a value that changes is kept as one line for each
// change and read with LastLine.
func AppendLine(parts []string, name, line string) error {
	if err := EnsureDirs(parts); err != nil {
		return err
	}
	f, err := OpenAppendNoFollow(filepath.Join(filepath.Join(parts...), name), 0o600)
	if err != nil {
		return err
	}
	_, werr := f.WriteString(line + "\n")
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	return werr
}

// LastLine is the last line of the file at path that a newline ends, without the newline, or "" when there is
// none. It reads only the last window bytes of the file, so its cost does not grow with the number of lines; a
// line longer than the window is returned as its last bytes, so a caller that expects short lines checks the
// shape of what it gets. A line no newline ends yet (one being written) is not the last line. The file must be
// a regular file and not a link (ErrSymlink, ErrNotRegular); a missing file satisfies errors.Is(err,
// fs.ErrNotExist).
func LastLine(path string, window int64) (string, error) {
	if window < 2 {
		return "", fmt.Errorf("statestore: window %d is too small to hold a line", window)
	}
	f, err := OpenNoFollow(path)
	if err != nil {
		if IsSymlinkRefusal(err) {
			return "", refuse(ErrSymlink, "refusing symlinked file %q", path)
		}
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", refuse(ErrNotRegular, "%q is not a regular file", path)
	}
	start := int64(0)
	if info.Size() > window {
		start = info.Size() - window
	}
	buf := make([]byte, info.Size()-start)
	n, err := f.ReadAt(buf, start)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	buf = buf[:n]
	end := len(buf)
	for end > 0 && buf[end-1] != '\n' {
		end--
	}
	if end == 0 {
		return "", nil
	}
	buf = buf[:end-1]
	begin := len(buf)
	for begin > 0 && buf[begin-1] != '\n' {
		begin--
	}
	return string(buf[begin:]), nil
}
