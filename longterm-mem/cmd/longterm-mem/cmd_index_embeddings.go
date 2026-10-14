package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/embed"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/embedindex"
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
// [--allow-remote-embedder]` (R-069): the build use case
// (embedindex.UseCase) reads P's live Engram observations, incrementally
// (re-)embeds them through the configured backend, and persists the
// resulting index under <state-dir>/index/<project>/ (vecindex.Dir). It
// never builds lazily from a query path (R-069) -- this is the only
// command that builds an index from scratch.
func (c commands) cmdIndexEmbeddings(project string, cfg embedConfig) int {
	stateDir := defaultStateDir()
	if stateDir == "" {
		fmt.Fprintf(os.Stderr, "longterm-mem: index: %v\n", stateDirUnresolved{})
		return exitPathUnresolvable
	}

	store, err := engram.Open(os.Getenv(engramDBEnvVar))
	if err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: index: %v\n", err)
		return exitEngramUnavailable
	}
	defer store.Close()

	result, err := c.embeddingIndexing(store, stateDir, cfg).Build(context.Background(), project, embedindex.Contract{
		Model:      cfg.Model,
		Dimension:  cfg.Dimension,
		InputLimit: cfg.InputLimit,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: index: %v\n", err)
		return embedIndexExitCode(err)
	}

	fmt.Printf("longterm-mem: embedding index built for %s: %d embedded, %d reused, %d removed\n", project, result.Embedded, result.Reused, result.Removed)
	return exitOK
}

// embeddingIndexing is the build of the embedding index wired to what lives outside the process: the live
// observations of the Engram store (R-020: it leaves out what was soft-deleted, so a row deleted since the last
// build is absent and the build's removal pass drops its entry), the embedding client the commands were composed
// with, made from the flags of this invocation, and the index repository over the state directory, with the wall
// clock to stamp a build with. The CLI serves one build and exits, so the repository has no cache.
func (c commands) embeddingIndexing(store *engram.Store, stateDir string, cfg embedConfig) embedindex.UseCase {
	return embedindex.UseCase{
		Observations: store,
		NewEmbedder: func() (embedindex.Embedder, error) {
			client, err := c.newEmbedClient(embed.Config{
				Endpoint:    cfg.Endpoint,
				Model:       cfg.Model,
				AllowRemote: cfg.AllowRemote,
			})
			if err != nil {
				return nil, err
			}
			return client, nil
		},
		Builder: &vecindex.Repository{StateDir: stateDir, Now: buildNowRFC3339},
	}
}

// embedIndexExitCode is the exit code of a build that stopped: Engram that cannot list its observations is
// engram_unavailable; the rest (an embedding client that is refused, an index that cannot be built) is
// longterm-mem's own side.
func embedIndexExitCode(err error) int {
	var failure *embedindex.Error
	if errors.As(err, &failure) && failure.Stage == embedindex.StageObservations {
		return exitEngramUnavailable
	}
	return exitInternal
}

// buildNowRFC3339 is vecindex.Build's `now` seam, wired to the real clock.
func buildNowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}
