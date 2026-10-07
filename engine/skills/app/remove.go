package app

import "github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"

// RemoveInput is what `skills remove` is asked: the skill to take out of the registry and the
// manifest, and where they are.
type RemoveInput struct {
	RegistryPath string
	ManifestPath string
	ID           string
}

// RemovePorts is what remove reads and writes through.
type RemovePorts struct {
	// Registries reads the registry and encodes the one it writes.
	Registries skills.RegistryRepository
	// Files reads the manifest.
	Files skills.FileReader
	// Staged writes the manifest and the registry.
	Staged skills.StagedWrites
}

// RemoveResult is what remove did, or how far it got: UnreadWarning says what the reader left out
// of the registry it read, and is set even when remove then refused, except when the refusal is
// that very thing (the refusal says it).
type RemoveResult struct {
	ID            string
	UnreadWarning string
}

// RemoveSkill takes a skill out of the registry and the manifest, in the same order and with the
// same checks as AddSkill. It deletes no file of the skill. An error is a *IDRequiredError, a
// *RegistryError, the refusal of skills.RemoveEntry (the id is not registered, or the registry
// has fields its reader left out), an *EncodeError, a *ReparseError, ErrRoundTripMismatch, a
// *ManifestReadError, a *ManifestSyntaxError, a *DivergenceError or a *skills.StagedWriteError.
func RemoveSkill(p RemovePorts, in RemoveInput) (RemoveResult, error) {
	var res RemoveResult
	if in.ID == "" {
		return res, &IDRequiredError{Verb: "remove"}
	}
	reg, warning, err := readRegistry(p.Registries, in.RegistryPath)
	if err != nil {
		return res, err
	}
	// A registry the reader left fields out of cannot be written back whole. The refusal says
	// what was left out, so the warning that says it is not also set: a person is told once.
	if err := reg.CheckWritable(); err != nil {
		return res, err
	}
	res.UnreadWarning = warning
	newReg, err := skills.RemoveEntry(reg, in.ID)
	if err != nil {
		return res, err
	}
	regBytes, reread, err := encodeVerified(p.Registries, newReg)
	if err != nil {
		return res, err
	}
	manifest, err := p.Files(in.ManifestPath)
	if err != nil {
		return res, &ManifestReadError{Path: in.ManifestPath, Err: err}
	}
	newManifest := skills.ManifestWithoutSkill(manifest, in.ID)
	if err := crossCheck(reread, newManifest); err != nil {
		return res, err
	}
	if err := skills.CommitStaged(p.Staged, skills.OverlayFileMode,
		skills.StagedFile{Name: "manifest", Path: in.ManifestPath, Data: newManifest},
		skills.StagedFile{Name: "registry", Path: in.RegistryPath, Data: regBytes}); err != nil {
		return res, err
	}
	res.ID = in.ID
	return res, nil
}
