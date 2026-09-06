package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/embed"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vecindex"
)

// stateDirEnvVar overrides the default directory this module's own
// derived state (today: the embedding index) lives under
// (LONGTERM_MEM_STATE_DIR). It is a separate contract from
// defaultRegisterStateDir's install-state directory (register_paths.go):
// that one is where `register` proves ownership of a runtime's config
// entry, this one is where `index --embeddings` keeps files nothing else
// in the module reads or writes.
const stateDirEnvVar = "LONGTERM_MEM_STATE_DIR"

// defaultStateDir resolves LONGTERM_MEM_STATE_DIR when set, else
// ~/.labdrian-overlay/longterm-mem (D5/D9's shared per-user state root).
// Empty means unresolvable -- cmdIndexEmbeddings refuses on it rather than
// writing the index wherever the process happens to be running, the same
// fail-closed contract defaultRegisterStateDir already uses.
func defaultStateDir() string {
	if p := os.Getenv(stateDirEnvVar); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".labdrian-overlay", "longterm-mem")
}

// embedConfig collects `index --embeddings`'s own flags (cmd_index.go),
// kept as one struct so cmdIndexEmbeddings's signature does not grow a
// parameter per flag.
type embedConfig struct {
	Endpoint    string
	Model       string
	Dimension   int
	InputLimit  int
	AllowRemote bool
}

// cmdIndexEmbeddings implements `longterm-mem index --project P
// --embeddings [--embed-endpoint URL] [--embed-model NAME]
// [--allow-remote-embedder]` (R-069): read P's live Engram observations,
// incrementally (re-)embed them through the configured backend, and
// persist the resulting index under <state-dir>/index/<project>/
// (vecindex.Dir). It never builds lazily from a query path (R-069) --
// this is the only production caller of vecindex.Build.
func cmdIndexEmbeddings(project string, cfg embedConfig) int {
	stateDir := defaultStateDir()
	if stateDir == "" {
		fmt.Fprintln(os.Stderr, "longterm-mem: index: could not resolve the state directory; set HOME or LONGTERM_MEM_STATE_DIR")
		return exitPathUnresolvable
	}

	store, err := engram.Open(os.Getenv(engramDBEnvVar))
	if err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: index: %v\n", err)
		return exitEngramUnavailable
	}
	defer store.Close()

	// R-020: ListObservations already excludes soft-deleted rows, so a row
	// deleted since the last build is simply absent here and Build's own
	// removal pass (R-069) drops its index entry.
	observations, err := store.ListObservations(project)
	if err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: index: %v\n", err)
		return exitEngramUnavailable
	}

	client, err := embed.NewClient(embed.Config{
		Endpoint:    cfg.Endpoint,
		Model:       cfg.Model,
		AllowRemote: cfg.AllowRemote,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: index: %v\n", err)
		return exitInternal
	}

	rows := make([]vecindex.Row, 0, len(observations))
	for _, o := range observations {
		rows = append(rows, vecindex.Row{EngramID: o.ID, Title: o.Title, Content: o.Content})
	}

	dir := vecindex.Dir(stateDir, project)
	_, result, err := vecindex.Build(context.Background(), dir, cfg.Model, cfg.Dimension, cfg.InputLimit, rows, client, buildNowRFC3339)
	if err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: index: %v\n", err)
		return exitInternal
	}

	fmt.Printf("longterm-mem: embedding index built for %s: %d embedded, %d reused, %d removed\n", project, result.Embedded, result.Reused, result.Removed)
	return exitOK
}

// buildNowRFC3339 is vecindex.Build's `now` seam, wired to the real clock.
func buildNowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}
