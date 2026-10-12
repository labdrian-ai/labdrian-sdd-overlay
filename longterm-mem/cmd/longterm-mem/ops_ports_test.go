package main

import (
	"errors"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/embed"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/ops"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vecindex"
)

// The adapters are what the ports of the diagnostics ask for. Neither package imports the other's
// consumer, so the composition root is where the two are held together: the repository reads the embedding
// index in the terms of ops.EmbeddingIndexReader, and the embedding client's own errors say what they are in the
// terms the diagnostics find them by.
var (
	_ ops.EmbeddingIndexReader = (*vecindex.Repository)(nil)
	_ ops.EmbedModelMissing    = (*embed.ModelMissingError)(nil)
)

// An index nobody built is told, by the error the repository answers, in the terms of ops (NoEmbeddingIndex):
// the method vecindex states without importing ops is the one ops finds.
func TestTheRepositoryTellsTheDiagnosticsAnIndexNobodyBuilt(t *testing.T) {
	repository := &vecindex.Repository{StateDir: t.TempDir()}

	_, err := repository.EmbeddingIndexBuiltAt("never-built")

	var nobodyBuilt ops.NoEmbeddingIndex
	if !errors.As(err, &nobodyBuilt) {
		t.Errorf("error = %v, want one the diagnostics find as ops.NoEmbeddingIndex", err)
	}
}

// The backend's unreachable error is not a missing model, in the diagnostics' terms.
func TestTheEmbeddingClientsUnreachableErrorIsNotAMissingModel(t *testing.T) {
	var missing ops.EmbedModelMissing
	if errors.As(&embed.BackendUnreachableError{Err: errors.New("refused")}, &missing) {
		t.Error("an unreachable backend was taken for a missing model")
	}
}
