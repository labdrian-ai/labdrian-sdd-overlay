package ui

import (
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/tui/internal/domain"
)

// backendName is how the transcript names the backend in the "$" line of each
// invocation: the script under the repository, as an operator would type it.
const backendName = "bin/labdrian-overlay"

// transcript is the text the result screen shows for an action: each
// invocation after its "$" line, what it printed, and how it failed (or why it
// was not made), one blank line between invocations. The use case that ran
// them records the invocations as values; this is the one place they become
// text.
func transcript(invocations []domain.Invocation) string {
	var sb strings.Builder
	for i, inv := range invocations {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf("$ %s %s\n", backendName, strings.Join(inv.Args, " ")))

		if inv.Refusal != nil {
			sb.WriteString(fmt.Sprintf("[omitido: %v]\n", inv.Refusal))
			continue
		}
		sb.WriteString(inv.Run.Output)
		if inv.Run.Err != nil {
			sb.WriteString(fmt.Sprintf("\n[exit error: %v]\n", inv.Run.Err))
		}
	}
	return sb.String()
}
