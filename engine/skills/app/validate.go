package app

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// ErrSourceRootRequired is why validate was not asked a question it can answer: it has no default
// source root and no fallback to the working directory, so a caller that omits it is refused
// instead of getting a silent scan of wherever it stands.
var ErrSourceRootRequired = errors.New("skills validate requires --source-root <skills-dir>")

// ValidateInput is what `skills validate` is asked: the registry, the manifest it must agree with,
// and the tree of skills on disk it must match.
type ValidateInput struct {
	RegistryPath string
	ManifestPath string
	SourceRoot   string
}

// ValidatePorts is what validate reads through.
type ValidatePorts struct {
	// Registries reads the registry.
	Registries skills.RegistryRepository
	// Manifest reads the manifest file.
	Manifest skills.FileReader
	// Tree lists the files of the skills tree.
	Tree skills.SkillTree
	// Approvals reads the evidence of the approval of every global skill.
	Approvals skills.ApprovalRecordStore
	// Baseline says which skills are grandfathered; nil is the baseline the domain pins.
	Baseline skills.BaselineLookup
}

// ManifestReadError is a manifest that could not be read.
type ManifestReadError struct {
	Path string
	Err  error
}

func (e *ManifestReadError) Error() string {
	return fmt.Sprintf("reading manifest %q: %v", e.Path, e.Err)
}
func (e *ManifestReadError) Unwrap() error { return e.Err }

// ManifestParseError is a manifest that was read and that the on-disk check cannot parse.
type ManifestParseError struct{ Err error }

func (e *ManifestParseError) Error() string {
	return fmt.Sprintf("parsing manifest for on-disk check: %v", e.Err)
}
func (e *ManifestParseError) Unwrap() error { return e.Err }

// ScanError is a skills tree that could not be listed.
type ScanError struct {
	Root string
	Err  error
}

func (e *ScanError) Error() string {
	return fmt.Sprintf("scanning skills directory %q: %v", e.Root, e.Err)
}
func (e *ScanError) Unwrap() error { return e.Err }

// ValidateResult is everything validate found, in the order it found it. A failure that stops the
// check (the manifest cannot be read, the tree cannot be listed) is returned with the result so
// far: what was found before it is told before it, so that a failure never hides a divergence
// that was already known.
type ValidateResult struct {
	// UnreadWarning says what the reader left out of the registry; empty when it read it whole.
	UnreadWarning string
	// SharedPathNotes say which ids share a path, which is accepted and changes no exit code.
	SharedPathNotes []string
	// RegistryDiverges says the registry and the manifest disagree, and RegistryDivergences how.
	RegistryDiverges    bool
	RegistryDivergences []skills.Divergence
	// OnDisk are the files of the tree and the manifest that disagree.
	OnDisk []skills.Divergence
	// Unapproved are the global skills whose approval is missing, stale, malformed or
	// unverifiable.
	Unapproved []skills.Divergence
	// NotVerifiable is why validate cannot vouch for a registry the reader did not read whole.
	NotVerifiable error
	// SkillCount is how many skills the registry has, DiskFiles how many files the tree has, and
	// Approvals the tally of the approval check.
	SkillCount int
	DiskFiles  int
	Approvals  skills.ApprovalSummary
}

// Passed reports whether every check is clean: nothing diverges, and the registry was read whole.
func (r ValidateResult) Passed() bool {
	return !r.RegistryDiverges && len(r.OnDisk) == 0 && len(r.Unapproved) == 0 && r.NotVerifiable == nil
}

// ValidateOverlay runs every check of `skills validate`: the registry against the manifest, the
// tree on disk against the manifest, and the approval of every global skill. It is a full scan: it
// reports every divergence of every check and never stops at the first. An error is a check that
// could not be made (ErrSourceRootRequired, a *RegistryError, a *ManifestReadError, a
// *ManifestParseError or a *ScanError); the result is what was found before it.
func ValidateOverlay(p ValidatePorts, in ValidateInput) (ValidateResult, error) {
	var res ValidateResult
	if in.SourceRoot == "" {
		return res, ErrSourceRootRequired
	}
	reg, warning, err := readRegistry(p.Registries, in.RegistryPath)
	if err != nil {
		return res, err
	}
	res.UnreadWarning = warning
	res.SkillCount = len(reg.Skills)
	// Two ids on one path are accepted and not a failure: validate says so.
	for _, shared := range reg.SharedPaths() {
		res.SharedPathNotes = append(res.SharedPathNotes, shared.Note())
	}

	// The manifest is read once, and the registry is compared with what was read. A manifest that
	// cannot be read has no divergence to tell: the refusal says why.
	manifestData, readErr := p.Manifest(in.ManifestPath)
	if readErr == nil {
		divs, regErr := skills.ValidateAgainstManifest(reg, manifestData)
		if regErr != nil {
			res.RegistryDiverges = true
			res.RegistryDivergences = divs
		}
	}
	if readErr != nil {
		return res, &ManifestReadError{Path: in.ManifestPath, Err: readErr}
	}
	manifestPaths, err := skills.DeployableManifestPaths(bytes.NewReader(manifestData))
	if err != nil {
		return res, &ManifestParseError{Err: err}
	}
	diskPaths, err := p.Tree.ScanSkillFiles(in.SourceRoot)
	if err != nil {
		return res, &ScanError{Root: in.SourceRoot, Err: err}
	}
	res.DiskFiles = len(diskPaths)
	res.OnDisk = skills.DiffOnDisk(diskPaths, manifestPaths)

	// Every global skill needs a valid human-approval record for its exact SKILL.md bytes, unless
	// it is the grandfathered baseline's.
	res.Unapproved, res.Approvals = skills.CheckApprovalsAgainst(p.Baseline.OrFixed(), reg, in.SourceRoot, p.Approvals)

	// A registry the reader did not read whole is not passed: the reason is told last.
	res.NotVerifiable = reg.CheckVerifiable()
	return res, nil
}
