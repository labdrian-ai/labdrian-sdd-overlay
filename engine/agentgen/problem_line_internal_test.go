package agentgen

// Check words a problem as missing or unreadable, with its subject, only for an error of reading; an error of
// projecting is always a projection problem (T4s of odd/tasks/merge-playbook-into-sdd-overlay.md, L55, review
// 490a46c0b44b-5 F2). The test reads the unexported sourceError, so it lives in package agentgen.

import (
	"fmt"
	"io/fs"
	"testing"
)

func TestAProjectingErrorIsNeverCalledMissing(t *testing.T) {
	for _, c := range []struct {
		name string
		p    *sourceError
		want string
	}{
		{
			"a projecting error that wraps ErrNotExist",
			&sourceError{path: "roles/x/body.md", verb: "projecting", err: fmt.Errorf("w: %w", fs.ErrNotExist)},
			"roles/x/body.md: cannot be projected: w: file does not exist",
		},
		{
			"a reading error that wraps ErrNotExist",
			&sourceError{path: "roles/x/body.md", subject: subjectSource, verb: "reading", err: fs.ErrNotExist},
			"roles/x/body.md: missing (the source of the role is not in the repository)",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := problemLine(c.p); got != c.want {
				t.Errorf("problemLine = %q, want %q", got, c.want)
			}
		})
	}
}
