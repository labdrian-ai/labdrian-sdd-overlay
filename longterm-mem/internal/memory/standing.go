package memory

// Neighbour is the observation on the far side of a relation.
type Neighbour struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

// Standing is what a store's relation ledger says about one observation, in the only terms a reader
// cares about: may I treat this as current?
//
// It exists because the ledger is otherwise unread. Engram records every verdict in its relation
// table and its own search never joins that table -- verified on a copy of a real database:
// inserting "B supersedes A" left the search results for A byte-identical, still ranked first,
// unmarked. So a memory that was explicitly replaced comes back looking exactly like one that was
// not, and the reader reintroduces what was abandoned. longterm-mem cannot fix Engram's search (its
// connection is read-only, R-002), but it can refuse to repeat the omission in its OWN answers.
//
// The JSON names are the wire format of the query command and the MCP query tool, which carry a
// Standing as it is.
type Standing struct {
	// SupersededBy names the observations that replaced this one. Direction is the whole of it:
	// being the TARGET of a supersedes means something replaced you, being the SOURCE means you are
	// the replacement.
	SupersededBy []Neighbour `json:"superseded_by,omitempty"`
	// ConflictsWith names observations judged to contradict this one.
	ConflictsWith []Neighbour `json:"conflicts_with,omitempty"`
	// Unjudged names observations Engram flagged against this one and nobody ever decided about. On
	// the real database this was the largest class by far, and every one of them was invisible to
	// every reader: an undecided conflict is not the same as no conflict.
	Unjudged []Neighbour `json:"unjudged,omitempty"`
}

// Empty reports whether there is nothing worth telling a reader.
func (s Standing) Empty() bool {
	return len(s.SupersededBy) == 0 && len(s.ConflictsWith) == 0 && len(s.Unjudged) == 0
}
