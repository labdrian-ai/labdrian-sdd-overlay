// Package fieldrule holds the rules for the shape of one field of a versioned
// document that more than one record shares, so that a Goal and a Shaper
// handoff say the same thing in the same words when they refuse the same
// shape. It checks one decoded field and knows nothing of the document it
// came from: the name of the field is the caller's, and so is what to do with
// the error.
//
// It is a domain package: pure standard library, no file, no clock, no
// entropy.
package fieldrule

import (
	"fmt"
	"strings"
)

// CheckStringArray requires values to be a string array that was present (nil
// is an absent or null array, which is not the same as an empty one), to hold
// at least minItems items, and to hold only items with something in them: an
// item that is empty or only white space (as strings.TrimSpace counts it) is
// refused. The first problem found is the one reported, in that order, and a
// blank item is named by its 1-based position. name is the field as the
// document calls it.
func CheckStringArray(name string, values []string, minItems int) error {
	if values == nil {
		return fmt.Errorf("%s must be a non-null string array", name)
	}
	if len(values) < minItems {
		return fmt.Errorf("%s must contain at least %d item(s)", name, minItems)
	}
	for i, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s item %d must be non-blank", name, i+1)
		}
	}
	return nil
}
