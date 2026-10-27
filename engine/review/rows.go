package review

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// This file is the lens rows of the reviewer (own-rdd T5, S8). The sentences of the rows are written once, in
// roles/reviewer/rows.json, and the parent of the chain copies the ones a launch needs into the prompt of the
// reviewer. The program does not read that file (a role source is not deployed, and this package reads no file):
// it holds the ids of the rows, and a test of this package holds the two to each other. A launch declares the ids of
// the rows it was sent (ParseRows), so that the number of lenses a review ran is counted from what was recorded and
// not from the receipt of another tool.

// rowIDs are the ids of the rows of roles/reviewer/rows.json, in the order of the file: the default row every launch
// is sent, the readability row of a change that touches docs, a public API, messages or help, and the row of the
// second blind launch that a Risk line which names an item asks for.
var rowIDs = [...]string{"default", "readability", "risk-line"}

// RowIDs are the ids of the rows a launch may be sent, in the order of roles/reviewer/rows.json. The slice is a copy:
// the table is the program's.
func RowIDs() []string { return slices.Clone(rowIDs[:]) }

// ParseRows reads the rows a launch declares: ids of RowIDs separated by commas with no space and none written twice
// ("default,readability"), in the order they are written. A declaration that is anything else, an empty one included,
// is refused with an error that wraps ErrInvalidRows and says what is accepted.
func ParseRows(declared string) ([]string, error) {
	ids := strings.Split(declared, ",")
	if err := checkRows(ids); err != nil {
		return nil, fmt.Errorf("%w: %q is refused, %v; the rows are %s, separated by commas with no space and none written twice (for example %q)",
			ErrInvalidRows, declared, err, strings.Join(rowIDs[:], ", "), "default,readability")
	}
	return ids, nil
}

// checkRows says why ids are not the rows of a launch, or returns nil: at least one, each an id of RowIDs, none
// written twice. The error is the reason alone; the caller wraps it in the sentinel of what it was reading.
func checkRows(ids []string) error {
	if len(ids) == 0 {
		return errors.New("it lists no row")
	}
	for i, id := range ids {
		switch {
		case !slices.Contains(rowIDs[:], id):
			return fmt.Errorf("%q is not a row", id)
		case slices.Contains(ids[:i], id):
			return fmt.Errorf("%q is written twice", id)
		}
	}
	return nil
}
