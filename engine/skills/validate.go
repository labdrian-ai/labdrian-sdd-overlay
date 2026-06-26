package skills

import (
	"fmt"
	"strings"
)

// DivergenceClass names the category of a registry/manifest disagreement.
type DivergenceClass string

const (
	// DivMissingInManifest reports a registry entry whose path has no manifest skill-dir.
	DivMissingInManifest DivergenceClass = "MISSING_IN_MANIFEST"
	// DivMissingInRegistry reports a manifest skill-dir with no registry entry.
	DivMissingInRegistry DivergenceClass = "MISSING_IN_REGISTRY"
	// DivTagMismatch reports a disagreement between registry source.type and manifest tag.
	DivTagMismatch DivergenceClass = "TAG_MISMATCH"
	// DivMixedTag reports a manifest skill-dir whose rows carry both managed and custom tags.
	DivMixedTag DivergenceClass = "MIXED_TAG"
)

// Divergence is a single discrepancy found between the registry and the manifest.
type Divergence struct {
	Class  DivergenceClass
	Path   string // skill directory involved
	Detail string // human-readable explanation
}

// tagMatchesSourceType reports whether a manifest tag agrees with a registry source type.
// Delegates to manifestTagFor (ADR-13) — single source of truth for the mapping.
func tagMatchesSourceType(tag, sourceType string) bool {
	return tag == manifestTagFor(sourceType)
}

// Diff performs a bidirectional cross-check between a Registry and a ManifestView.
// It returns every divergence found (full scan — never stops at first error).
//
// Rules:
//   - For each registry entry: if its path is absent from mv → MISSING_IN_MANIFEST.
//   - For each registry entry present in mv: if tag disagrees with source.type → TAG_MISMATCH.
//   - For each manifest dir in mv: if no registry entry has path == dir → MISSING_IN_REGISTRY.
func Diff(reg Registry, mv ManifestView) []Divergence {
	var divs []Divergence

	// Track which manifest dirs were seen while processing registry entries.
	covered := make(map[string]bool, len(mv))

	for _, entry := range reg.Skills {
		me, ok := mv[entry.Path]
		if !ok {
			divs = append(divs, Divergence{
				Class:  DivMissingInManifest,
				Path:   entry.Path,
				Detail: fmt.Sprintf("registry entry %q has no corresponding skill directory in manifest", entry.Path),
			})
			continue
		}
		covered[entry.Path] = true
		if !tagMatchesSourceType(me.Tag, entry.Source.Type) {
			divs = append(divs, Divergence{
				Class:  DivTagMismatch,
				Path:   entry.Path,
				Detail: fmt.Sprintf("registry source.type=%q but manifest tag=%q for dir %q", entry.Source.Type, me.Tag, entry.Path),
			})
		}
	}

	// Any manifest dir not covered by a registry entry is MISSING_IN_REGISTRY.
	// Separately, any manifest dir with conflicting tags emits DivMixedTag (full scan).
	for dir, me := range mv {
		if !covered[dir] {
			divs = append(divs, Divergence{
				Class:  DivMissingInRegistry,
				Path:   dir,
				Detail: fmt.Sprintf("manifest skill dir %q has no entry in registry", dir),
			})
		}
		if me.HasConflict {
			divs = append(divs, Divergence{
				Class:  DivMixedTag,
				Path:   dir,
				Detail: fmt.Sprintf("manifest skill dir %q has rows with both managed and custom tags", dir),
			})
		}
	}

	return divs
}

// Validate loads the manifest at manifestPath, calls Diff, and returns the
// divergences together with a non-nil error when any divergences exist.
// Zero divergences → nil error.
func Validate(reg Registry, manifestPath string) ([]Divergence, error) {
	mv, err := LoadManifestView(manifestPath)
	if err != nil {
		return nil, err
	}
	divs := Diff(reg, mv)
	if len(divs) == 0 {
		return nil, nil
	}
	msgs := make([]string, len(divs))
	for i, d := range divs {
		msgs[i] = fmt.Sprintf("[%s] %s: %s", d.Class, d.Path, d.Detail)
	}
	return divs, fmt.Errorf("validate: %d divergence(s) found:\n%s", len(divs), strings.Join(msgs, "\n"))
}
