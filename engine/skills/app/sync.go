package app

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// SyncInput is what `skills sync-manifest` is asked: the registry to bring the manifest into line
// with, and the manifest.
type SyncInput struct {
	RegistryPath string
	ManifestPath string
}

// SyncPorts is what sync-manifest reads and writes through. The registry is only read.
type SyncPorts struct {
	// Registries reads the registry.
	Registries skills.RegistryRepository
	// Files reads the manifest.
	Files skills.FileReader
	// Staged writes the manifest.
	Staged skills.StagedWrites
}

// SyncResult is what sync-manifest did, or how far it got: UnreadWarning says what the reader left
// out of the registry it read, and is set even when sync then refused.
type SyncResult struct {
	UnreadWarning string
	// InSync says the manifest already agreed with the registry: it was not written.
	InSync bool
	// Changes says which skill rows were added, dropped and retagged when it was written.
	Changes skills.ChangeReport
}

// SyncManifest regenerates the skill rows of the manifest from the registry, keeping every other
// line byte for byte, and writes the manifest atomically; the registry is never modified (R-109).
// A manifest that already agrees is not written (SyncResult.InSync). An error is a *RegistryError,
// a *ManifestReadError, a refusal of skills.SyncManifest (its own post-condition check failed:
// nothing reaches the disk), a *ManifestSyntaxError, a *DivergenceError or a
// *skills.StagedWriteError; a manifest that cannot be written is left as it was (R-102).
func SyncManifest(p SyncPorts, in SyncInput) (SyncResult, error) {
	var res SyncResult
	reg, warning, err := readRegistry(p.Registries, in.RegistryPath)
	if err != nil {
		return res, err
	}
	res.UnreadWarning = warning
	manifest, err := p.Files(in.ManifestPath)
	if err != nil {
		return res, &ManifestReadError{Path: in.ManifestPath, Err: err}
	}
	// The regeneration checks its own result before it returns it.
	regenerated, report, err := skills.SyncManifest(reg, manifest)
	if err != nil {
		return res, err
	}
	if bytes.Equal(regenerated, manifest) {
		res.InSync = true
		return res, nil
	}
	// A second look at the result, in case something slipped between the two calls (ADR-9 step 7).
	if err := crossCheck(reg, regenerated); err != nil {
		var syntax *ManifestSyntaxError
		if errors.As(err, &syntax) {
			return res, fmt.Errorf("parsing regenerated manifest: %w", syntax.Err)
		}
		return res, err
	}
	if err := skills.CommitStaged(p.Staged, skills.OverlayFileMode, skills.StagedFile{Name: "manifest", Path: in.ManifestPath, Data: regenerated}); err != nil {
		return res, err
	}
	res.Changes = report
	return res, nil
}
