// Package app is the use case of the builder guard (own-rdd S14): given the prompt of a builder launch and
// the working directory of the session, it joins the citation of the prompt, the feature document it names
// and the decision of engine/oddtask.
package app

import (
	"errors"
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/oddtask"
)

// Documents is the port to the feature documents of a repository.
type Documents interface {
	// Read returns the feature document at path, resolved against cwd when it is relative. It fails with an
	// *oddtask.ReadError when the document cannot be read.
	Read(path, cwd string) (oddtask.File, error)
}

// Guard is the builder guard over a Documents port.
type Guard struct{ docs Documents }

// New is the Guard that reads feature documents from docs.
func New(docs Documents) Guard { return Guard{docs: docs} }

// Check says whether a builder may be launched with prompt in a session at cwd. The Reason is empty when it
// may; otherwise it is why not. A prompt that cites no task is refused before any document is read.
func (g Guard) Check(prompt, cwd string) oddtask.Reason {
	citation, reason := oddtask.ParseCitation(prompt)
	if reason != "" {
		return reason
	}
	file, err := g.docs.Read(citation.Path, cwd)
	if err != nil {
		var unreadable *oddtask.ReadError
		if errors.As(err, &unreadable) {
			return oddtask.UnreadableReason(unreadable.Path, unreadable.Cause)
		}
		return oddtask.UnreadableReason(citation.Path, fmt.Sprint(err))
	}
	return oddtask.Decide(file.Path, oddtask.ParseDocument(file.Text), citation.Tasks)
}
