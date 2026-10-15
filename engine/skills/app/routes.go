package app

import (
	"errors"
	"io/fs"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// RoutesInput is what `manifest routes` is asked: which manifest, which overlay it describes,
// where the rows are copied to for which target, and whether only the managed rows are wanted.
type RoutesInput struct {
	ManifestPath string
	OverlayRoot  string
	Destinations skills.RouteDestinations
	ManagedOnly  bool
}

// RoutesPorts is what the routing reads through.
type RoutesPorts struct {
	// Manifest reads the manifest file.
	Manifest skills.FileReader
}

// RoutesResult is what the routing found: where each row goes, and whether the manifest was there at
// all. A manifest that does not exist is a manifest with no row, as the overlay script always read it
// (its commands then act on nothing and say so, where a manifest that cannot be read is a refusal);
// ManifestMissing is how the command can say it is so.
type RoutesResult struct {
	Routes          []skills.ResolvedRoute
	ManifestMissing bool
}

// ManifestRoutes reads the manifest and says what the overlay does with each row for the target.
// An error is a manifest that exists and could not be read (a *ManifestReadError) or a row that could
// not be placed (a *skills.UnroutedRowError, a *skills.MissingRootError or a *skills.UnknownTargetError);
// there is no partial answer, so that a caller never acts on part of a manifest.
func ManifestRoutes(p RoutesPorts, in RoutesInput) (RoutesResult, error) {
	data, err := p.Manifest(in.ManifestPath)
	missing := errors.Is(err, fs.ErrNotExist)
	if err != nil && !missing {
		return RoutesResult{}, &ManifestReadError{Path: in.ManifestPath, Err: err}
	}
	routes, err := skills.ResolveRoutes(skills.ReadManifestRows(data), in.ManagedOnly, in.OverlayRoot, in.Destinations)
	if err != nil {
		return RoutesResult{}, err
	}
	return RoutesResult{Routes: routes, ManifestMissing: missing}, nil
}
