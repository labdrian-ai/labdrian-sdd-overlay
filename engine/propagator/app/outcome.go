package app

// Outcome says what one pass of the use case found in the registry, and what it did about it. It
// is the only thing the loop decides on: a pass is never told apart by what it would have printed.
type Outcome int

const (
	// Absent: there is no registry. The project does not use the overlay and there is nothing
	// to do, unless the registry was required, which is an error and not an Outcome.
	Absent Outcome = iota + 1
	// Empty: the registry exists and holds nothing but white space. That is never a registry:
	// it is the mark of a read torn by a writer in the middle of its work, or of a file that was
	// cut short, and writing over it would replace the whole registry with one block. Nothing
	// was written.
	Empty
	// Unchanged: the registry already holds the block as the contract asks. Nothing was written.
	Unchanged
	// Written: the registry did not hold the block, and now it does.
	Written
)

// String is the name of the outcome in the words of a log, not of a terminal.
func (o Outcome) String() string {
	switch o {
	case Absent:
		return "absent"
	case Empty:
		return "empty"
	case Unchanged:
		return "unchanged"
	case Written:
		return "written"
	}
	return "unknown"
}
