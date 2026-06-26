package skills

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
)

// ChangeReport records the dirs added, dropped, or retagged by a SyncManifest call.
type ChangeReport struct {
	Added    []string // dirs newly emitted (MISSING_IN_MANIFEST → inserted)
	Dropped  []string // orphan skill dirs removed (MISSING_IN_REGISTRY → dropped)
	Retagged []string // dirs whose tag changed (TAG_MISMATCH or MIXED_TAG → resolved)
}

// isSkillRow classifies a raw manifest line. It mirrors loadManifestViewReader's
// classification exactly: trim, skip blank / '#', ≥2 fields, path ends with
// '/SKILL.md', has '/', and the first path component is not an infra prefix.
// Returns the skill directory name and true when the line is a skill row.
func isSkillRow(line string) (dir string, ok bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", false
	}
	fields := strings.Fields(trimmed)
	if len(fields) < 2 {
		return "", false
	}
	rowPath := fields[0]
	if !strings.HasSuffix(rowPath, "/SKILL.md") {
		return "", false
	}
	slashIdx := strings.IndexByte(rowPath, '/')
	if slashIdx < 0 {
		return "", false
	}
	dirName := rowPath[:slashIdx]
	if isInfraDir(dirName) {
		return "", false
	}
	return dirName, true
}

// registryTag returns the manifest tag implied by an entry's source.type.
// "core" → "managed", anything else → "custom".
func registryTag(e Entry) string {
	if e.Source.Type == "core" {
		return "managed"
	}
	return "custom"
}

// SyncManifest regenerates */SKILL.md rows in manifest from reg, preserving every
// non-skill line byte-for-byte. Returns the new manifest bytes, a ChangeReport, and
// a non-nil error only when the internal post-condition check fails (a bug in the
// regen would prevent any output from reaching disk).
//
// Algorithm (anchor-replacement):
//  1. Walk lines; the first skill row is the anchor.
//  2. Emit non-skill lines before the anchor verbatim (preservedBefore).
//  3. At the anchor, emit the full registry-ordered skill block once.
//  4. Skip all remaining original skill rows.
//  5. Emit non-skill lines after the anchor verbatim (preservedAfter).
//  6. If no skill rows exist, append the skill block after all preserved lines.
//
// Post-condition: Diff(reg, loadManifestViewReader(newText)) must be empty.
// If the output is byte-identical to the input, (manifest, ChangeReport{}, nil)
// is returned — the caller can detect the no-op and skip a write.
func SyncManifest(reg Registry, manifest []byte) ([]byte, ChangeReport, error) {
	// Parse original manifest view for ChangeReport computation.
	origMV, err := loadManifestViewReader(bytes.NewReader(manifest))
	if err != nil {
		return nil, ChangeReport{}, fmt.Errorf("sync: reading original manifest: %w", err)
	}

	// Index registry by path for O(1) lookups.
	regMap := make(map[string]Entry, len(reg.Skills))
	for _, e := range reg.Skills {
		regMap[e.Path] = e
	}

	// ── Build ChangeReport ─────────────────────────────────────────────────────
	var report ChangeReport

	// Added: in registry but absent from original manifest.
	for _, e := range reg.Skills {
		if _, ok := origMV[e.Path]; !ok {
			report.Added = append(report.Added, e.Path)
		}
	}

	// Dropped: in original manifest but absent from registry.
	for dir := range origMV {
		if _, ok := regMap[dir]; !ok {
			report.Dropped = append(report.Dropped, dir)
		}
	}

	// Retagged: present in both but tag doesn't agree with registry, or MIXED_TAG.
	for dir, me := range origMV {
		e, ok := regMap[dir]
		if !ok {
			continue // counted as dropped above
		}
		if me.HasConflict || me.Tag != registryTag(e) {
			report.Retagged = append(report.Retagged, dir)
		}
	}

	// ── Partition lines ────────────────────────────────────────────────────────
	// strings.Split on a \n-terminated file always yields a trailing "" element.
	rawLines := strings.Split(string(manifest), "\n")

	var preservedBefore, preservedAfter []string
	anchorFound := false

	for _, line := range rawLines {
		if _, ok := isSkillRow(line); ok {
			if !anchorFound {
				anchorFound = true
			}
			// Skip every original skill row; the block is rebuilt from the registry.
		} else {
			if !anchorFound {
				preservedBefore = append(preservedBefore, line)
			} else {
				preservedAfter = append(preservedAfter, line)
			}
		}
	}

	// ── Build skill block ──────────────────────────────────────────────────────
	skillBlock := make([]string, 0, len(reg.Skills))
	for _, e := range reg.Skills {
		skillBlock = append(skillBlock, e.Path+"/SKILL.md "+registryTag(e))
	}

	// ── Assemble output lines ──────────────────────────────────────────────────
	var outLines []string

	if anchorFound {
		outLines = append(outLines, preservedBefore...)
		outLines = append(outLines, skillBlock...)
		outLines = append(outLines, preservedAfter...)
	} else {
		// No anchor: strip trailing "" artifacts (trailing newline from split) from
		// preservedBefore so we don't emit a blank line before the skill block.
		pb := preservedBefore
		for len(pb) > 0 && pb[len(pb)-1] == "" {
			pb = pb[:len(pb)-1]
		}
		outLines = append(outLines, pb...)
		outLines = append(outLines, skillBlock...)
	}

	// Normalise to exactly one trailing newline:
	// remove any trailing "" elements, then add exactly one.
	for len(outLines) > 0 && outLines[len(outLines)-1] == "" {
		outLines = outLines[:len(outLines)-1]
	}
	outLines = append(outLines, "")

	newText := []byte(strings.Join(outLines, "\n"))

	// ── Post-condition self-check ──────────────────────────────────────────────
	// A buggy regen must NEVER reach disk. If Diff is non-empty, we return an
	// error — SyncCore will exit 1 without touching overlay.manifest.
	mv2, err := loadManifestViewReader(bytes.NewReader(newText))
	if err != nil {
		return nil, ChangeReport{}, fmt.Errorf("sync: post-condition parse: %w", err)
	}
	if divs := Diff(reg, mv2); len(divs) > 0 {
		return nil, ChangeReport{}, fmt.Errorf("sync: post-condition failed: %v", divs)
	}

	// ── Byte no-op ─────────────────────────────────────────────────────────────
	if bytes.Equal(newText, manifest) {
		return manifest, ChangeReport{}, nil
	}

	return newText, report, nil
}

// SyncCore is the testable CLI core for `engine skills sync-manifest`.
// It reads the registry and manifest, regenerates the */SKILL.md rows via
// SyncManifest (pure), validates the result, then atomically writes the
// manifest file (registry is never modified — R-109). All side effects are
// injected; no global state is used.
//
// Exit semantics:
//   - exit 0: success (manifest updated) or "already in sync" (no-op)
//   - exit 1: any error (registry/manifest read, parse, post-condition, write)
//
// On any failure after the temp file is created, the temp file is removed and
// overlay.manifest is left byte-unchanged (R-102).
func SyncCore(args []string, readFile readFileFn, stdout, stderr io.Writer, exit func(int)) {
	registryPath, manifestPath, _, _ := parseFlags(args)

	// 1. Read and parse the registry.
	regData, err := readFile(registryPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: reading registry %q: %v\n", registryPath, err)
		exit(1)
		return
	}
	reg, err := ParseRegistry(bytes.NewReader(regData))
	if err != nil {
		fmt.Fprintf(stderr, "error: parsing registry: %v\n", err)
		exit(1)
		return
	}

	// 2. Read manifest.
	manifestData, err := readFile(manifestPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: reading manifest %q: %v\n", manifestPath, err)
		exit(1)
		return
	}

	// 3. Pure regen (includes internal post-condition self-check).
	newText, report, err := SyncManifest(reg, manifestData)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		exit(1)
		return
	}

	// 4. No-op: manifest is already in sync.
	if bytes.Equal(newText, manifestData) {
		fmt.Fprintln(stdout, "overlay.manifest already in sync")
		exit(0)
		return
	}

	// 5. Belt-and-suspenders validate-before-write (ADR-9 step 7).
	// SyncManifest already ran this check; this guard catches any unexpected
	// divergence that might slip through between calls.
	mv, err := loadManifestViewReader(bytes.NewReader(newText))
	if err != nil {
		fmt.Fprintf(stderr, "error: parsing regenerated manifest: %v\n", err)
		exit(1)
		return
	}
	if divs := Diff(reg, mv); len(divs) > 0 {
		for _, d := range divs {
			fmt.Fprintf(stderr, "[%s] %s: %s\n", d.Class, d.Path, d.Detail)
		}
		exit(1)
		return
	}

	// 6. Atomic write: temp file + rename (R-101, R-102).
	tmpName, err := writeFileAtomic(manifestPath, newText)
	if err != nil {
		fmt.Fprintf(stderr, "error: writing manifest: %v\n", err)
		exit(1)
		return
	}
	if err := os.Rename(tmpName, manifestPath); err != nil {
		os.Remove(tmpName)
		fmt.Fprintf(stderr, "error: finalizing manifest: %v\n", err)
		exit(1)
		return
	}

	// 7. Print summary (R-107).
	fmt.Fprintf(stdout, "sync-manifest: %d added, %d dropped, %d retagged\n",
		len(report.Added), len(report.Dropped), len(report.Retagged))
	exit(0)
}
