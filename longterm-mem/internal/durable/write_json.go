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
// encoding and of the directory name the file and carry no caller's name, so that each caller puts its own in front
// of them; the error of the replacement is WriteFile's.
func WriteJSON(path string, v any, createPerm os.FileMode) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(path), stateDirPerm); err != nil {
		return fmt.Errorf("create directory for %s: %w", path, err)
	}
	return WriteFile(path, data, createPerm)
}
