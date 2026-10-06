package skills

import (
	"errors"
	"io/fs"
)

// isAbsent reports whether err says that what was asked for is not there: the one answer of a
// file system that a verb treats as "nothing yet" and not as a failure.
func isAbsent(err error) bool { return errors.Is(err, fs.ErrNotExist) }
