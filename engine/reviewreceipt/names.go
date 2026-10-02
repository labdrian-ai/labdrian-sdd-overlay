package reviewreceipt

import (
	"fmt"
	"strings"
	"unicode"
)

// UnsafeNameError reports a name that cannot be used as one path component. The receipts
// folder of a change, and the file a receipt is kept in, are joined into a path by whatever
// keeps the receipts, so each must be exactly one component: a name with a separator or a dot
// segment would put a file outside the folder, and one with white space at an end would put
// it in a sibling that is not the one the caller meant.
type UnsafeNameError struct {
	// Kind says what the name is for: "change name" or "receipt file name".
	Kind string
	// Name is the refused name, as it was given.
	Name string
	// Reason says which rule it breaks.
	Reason string
}

func (e *UnsafeNameError) Error() string {
	return fmt.Sprintf("reviewreceipt: %s %q is not a safe path component: %s", e.Kind, e.Name, e.Reason)
}

// UnusableReceiptError reports an approved receipt that cannot be kept because of the
// document it came from. It stops the capture instead of skipping the receipt, since a
// receipt skipped in silence would be lost; so it names the document and the remedy.
type UnusableReceiptError struct {
	// Origin is where the document was read, as the source named it.
	Origin string
	// Err is why the receipt cannot be kept.
	Err error
}

func (e *UnusableReceiptError) Error() string {
	return fmt.Sprintf("reviewreceipt: the approved receipt in %s cannot be kept: %v; correct or remove that lineage in the review tool's store, then run the command again", e.Origin, e.Err)
}

func (e *UnusableReceiptError) Unwrap() error { return e.Err }

// The reasons a name is refused, in the order the rules are checked.
const (
	reasonEmpty      = "it is empty"
	reasonDotSegment = "it is a dot segment"
	reasonSeparator  = "it holds a path separator"
	reasonWhiteSpace = "it has leading or trailing white space"
	reasonControl    = "it holds a control character"
)

// pathSeparators are the characters that end a path component on some platform the program
// runs on. The rule does not depend on the platform it runs on: a name with a backslash is
// refused on Linux too, so a receipt directory moved between systems stays valid.
const pathSeparators = `/\`

// CheckPathComponent reports why name cannot be one path component, or nil when it can. kind
// says what the name is for and starts the message ("change name", "receipt file name"). The
// rule is pure and the same on every platform: a name is one component when it is not empty,
// is not "." or "..", holds no path separator (either slash), has no white space at either
// end, and holds no control character.
//
// Every name the Service gives a port is checked here first, so an adapter that joins it into
// a path is handed only names that stay inside the folder they are joined to.
func CheckPathComponent(kind, name string) error {
	refuse := func(reason string) error {
		return &UnsafeNameError{Kind: kind, Name: name, Reason: reason}
	}
	switch {
	case name == "":
		return refuse(reasonEmpty)
	case name == "." || name == "..":
		return refuse(reasonDotSegment)
	case strings.ContainsAny(name, pathSeparators):
		return refuse(reasonSeparator)
	case strings.TrimSpace(name) != name:
		return refuse(reasonWhiteSpace)
	case strings.IndexFunc(name, unicode.IsControl) >= 0:
		return refuse(reasonControl)
	}
	return nil
}
