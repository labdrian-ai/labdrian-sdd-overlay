package oddtask

import "fmt"

// guardPrefix and guardAdvice frame every reason into the one message of a denial.
const (
	guardPrefix = "labdrian builder guard: "
	guardAdvice = `. The builder never designs: launch the shaper first to design the task and mark its line "shaped: L<n>", ` +
		`then launch the builder with the line "Spec: odd/tasks/<feature>.md, T<n>" in its prompt. ` +
		`This guard reads text only, so it is a speed bump, not a security boundary.`
)

// Message is the whole text of a denial: the reason, and what to do.
func Message(r Reason) string { return guardPrefix + string(r) + guardAdvice }

// The reasons that need nothing from the document.
const (
	ReasonNoCitation    Reason = `the prompt cites no task (it has no line "Spec: odd/tasks/<feature>.md, T<n>")`
	ReasonManyCitations Reason = `the prompt has more than one "Spec:" line`
	ReasonBadCitation   Reason = `the "Spec:" line does not name a feature document odd/tasks/<feature>.md and at least one task T<n>`
	ReasonInternal      Reason = "the guard hit an internal error and could not check this launch"
)

// Why a feature document cannot be read.
const (
	CauseNotFound   = "not found"
	CauseNotRegular = "not a regular file"
	CauseLink       = "a symbolic link"
	CauseTooLarge   = "larger than 8 MiB"
	CauseNoCwd      = "a relative path and no working directory"

	CauseOutsideRepository = "a relative path that leaves the repository"
	CauseNoRepository      = "a relative path and no git repository holds the working directory"
)

// ReadError is a feature document that could not be read, with the absolute path that was tried.
type ReadError struct {
	Path  string
	Cause string
}

func (e *ReadError) Error() string { return string(UnreadableReason(e.Path, e.Cause)) }

// UnreadableReason refuses a launch whose feature document cannot be read.
func UnreadableReason(path, cause string) Reason {
	return Reason(fmt.Sprintf("the feature document %s cannot be read (%s)", path, cause))
}

// TooLargeReason refuses an Agent call over limit bytes, which is not judged.
func TooLargeReason(limit int) Reason {
	return Reason(fmt.Sprintf("the Agent call is larger than %d bytes and could not be checked", limit))
}
