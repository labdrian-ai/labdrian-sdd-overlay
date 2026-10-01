// Command tui is a Bubbletea front-end for the bin/labdrian-overlay bash CLI.
//
// It does NOT reimplement deploy/sync logic — the bash backend remains the
// single source of truth. The TUI shells out to bin/labdrian-overlay and renders the
// results, with a colored gentle-ai sync dashboard for `sync-check`.
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// main is the composition root: it locates the backend, builds the adapter
// that answers the TUI's ports from it, and hands both to the model. Nothing
// else chooses an adapter.
func main() {
	root, rootErr := RepoRoot()
	backend := newOverlayCLI(root)

	p := tea.NewProgram(newModel(deps{
		repoRoot: root,
		rootErr:  rootErr,
		catalog:  backend,
	}), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
