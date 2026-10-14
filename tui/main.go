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
	"github.com/labdrian-ai/labdrian-sdd-overlay/tui/internal/adapters/overlaycli"
	"github.com/labdrian-ai/labdrian-sdd-overlay/tui/internal/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/tui/internal/domain"
)

// main is the composition root: it locates the backend and hands the program
// to bubbletea. Nothing else chooses an adapter.
func main() {
	root, rootErr := overlaycli.RepoRoot()

	p := tea.NewProgram(compose(root, rootErr), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// compose builds the program's model: the adapter that answers the TUI's
// ports from the backend under root, the use cases over it, and the action
// menu, all handed to the model, which builds none of them. rootErr is why the
// backend could not be located, when it could not; the model shows it.
func compose(root string, rootErr error) tea.Model {
	backend := overlaycli.New(root)

	return newModel(deps{
		rootErr: rootErr,
		actions: domain.Actions(),
		catalog: backend,
		runner:  app.NewActionRunner(backend, backend),
		restore: app.NewRestorePlanner(backend),
		probe:   app.NewRepoProber(backend),
	})
}
