package skillstale

import (
	"fmt"
	"strings"
)

// Render converts findings into the stable report lines used by the CLI.
// The report carries evidence only; it does not imply consent to retire.
func Render(findings []Finding) string {
	var out strings.Builder
	for _, finding := range findings {
		fmt.Fprintf(&out, "%s candidate:%s status:%s\n", finding.SkillID, finding.Candidate, finding.Status)
		for _, signal := range finding.Signals {
			fmt.Fprintf(&out, "  %s\n", renderSignal(signal))
		}
	}
	return out.String()
}

func renderSignal(signal Signal) string {
	switch signal.Kind {
	case SignalRemoved:
		return fmt.Sprintf("REMOVED %s (by %s)", signal.Path, signal.Commit)
	case SignalMoved:
		return fmt.Sprintf("MOVED %s -> %s", signal.Path, signal.NewPath)
	case SignalUnresolvedCommand:
		return fmt.Sprintf("UNRESOLVED-COMMAND %s", signal.Command)
	case SignalQuiet:
		return fmt.Sprintf("QUIET since %s", signal.Since)
	case SignalSuperseded:
		return fmt.Sprintf("SUPERSEDED by %s", signal.By)
	default:
		return string(signal.Kind)
	}
}
