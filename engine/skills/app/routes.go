package app

import (
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

// ManifestRoutes reads the manifest and says what the overlay does with each row for the target.
// An error is a manifest that could not be read (a *ManifestReadError) or a row that could not be
// placed (a *skills.UnroutedRowError, a *skills.MissingRootError or a *skills.UnknownTargetError);
// there is no partial answer, so that a caller never acts on part of a manifest.
func ManifestRoutes(p RoutesPorts, in RoutesInput) ([]skills.ResolvedRoute, error) {
	data, err := p.Manifest(in.ManifestPath)
	if err != nil {
		return nil, &ManifestReadError{Path: in.ManifestPath, Err: err}
	}
	return skills.ResolveRoutes(skills.ReadManifestRows(data), in.ManagedOnly, in.OverlayRoot, in.Destinations)
}
