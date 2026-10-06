package skills

// ProjectID is the identity a registry gives a project when it admits a skill to it
// (install.allowedProjects), and the one `skills install` and `skills adopt` say in what they
// print. It is a name and nothing more: how a directory comes to have one is the business of the
// ProjectIdentity that answers for it.
type ProjectID string

// String is the id as a registry spells it.
func (id ProjectID) String() string { return string(id) }

// ProjectQuery is what a source of project identity is asked: the directory the verb works in, and
// the id the person gave for it (--project-id), which is empty when they gave none.
type ProjectQuery struct {
	Dir      string
	Explicit ProjectID
}

// ProjectIdentity says which project a directory is. The skills domain owns the question and
// holds no answer of its own: the id of a project is whatever the person said, what its repository
// says of itself, or what its directory is called, in the order the composition root puts the
// sources that know (Phase 9, decision Q8), and the domain asks once.
//
// Identify reports ok=false when this source has no answer for the query, so that a chain of
// sources can ask the next; the chain as a whole reports it when none had one, and then a verb
// refuses, naming the project it could not name. An error is a source that could not tell, which
// is not the same as having no answer: it stops the chain, because passing on to the next source
// would name a project by something less than what was asked.
type ProjectIdentity interface {
	Identify(q ProjectQuery) (id ProjectID, ok bool, err error)
}
