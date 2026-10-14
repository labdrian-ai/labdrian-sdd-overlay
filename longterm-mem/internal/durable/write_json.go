package durable

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// stateDirPerm is the mode WriteJSON gives a directory it has to make: the files it writes are the module's own
// record of itself, in a directory of the module's own, so nobody else has a reason to enter it.
const stateDirPerm = 0o700

// EncodeError is the failure of WriteJSON to encode the value: the file it was to write and the encoder's own error,
// found with errors.Is and errors.As. It is a type of its own so that a caller that has always said an encoding
// failure in words of its own can say it in them (the identity ledger puts no file in them); its own words are
// "marshal <file>: <the encoder's>".
type EncodeError struct {
	Path string
	Err  error
}

func (e *EncodeError) Error() string { return "marshal " + e.Path + ": " + e.Err.Error() }

func (e *EncodeError) Unwrap() error { return e.Err }

// WriteJSON replaces path with v encoded as JSON, indented by two spaces and ended by a newline, through
// WriteFile: atomically, onto the file a link points to, keeping the mode of a file that is there and giving a file
// it creates createPerm. It makes the directory of path, owner-only, when it is not there.
//
// It is for the files this module keeps for itself (the vault registry, the install-state record, the identity
// ledger), each of which used to spell out this sequence by hand, with the same words for the same failures, and
// whose writers none of them owned. A file that holds somebody else's content (a runtime's configuration, a
// vault's page) keeps its own bytes and goes through WriteFile.
//
// The value is encoded first: one that cannot be leaves the file as it was and makes no directory. The errors of the
// encoding (an *EncodeError) and of the directory name the file and carry no caller's name, so that each caller puts
// its own in front of them, or, as the identity ledger does for the encoding, says the failure in the words it has
// always used; the error of the replacement is WriteFile's.
func WriteJSON(path string, v any, createPerm os.FileMode) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return &EncodeError{Path: path, Err: err}
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(path), stateDirPerm); err != nil {
		return fmt.Errorf("create directory for %s: %w", path, err)
	}
	return WriteFile(path, data, createPerm)
}
