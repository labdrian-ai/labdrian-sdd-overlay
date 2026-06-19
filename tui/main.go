// Command tui is a Bubbletea front-end for the bin/overlay bash CLI.
//
// It does NOT reimplement deploy/sync logic — the bash backend remains the
// single source of truth. The TUI shells out to bin/overlay and renders the
// results, with a colored gentle-ai sync dashboard for `sync-check`.
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	p := tea.NewProgram(newModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
