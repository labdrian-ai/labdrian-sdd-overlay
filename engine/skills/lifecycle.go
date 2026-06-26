package skills

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"reflect"
	"regexp"
	"strings"
)

// slugRe matches valid skill identifiers: lowercase alphanumeric, may contain
// hyphens, must start with a letter or digit (ADR-8 slug guard).
var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// AddEntry returns a new Registry with a new entry appended for id, using the
// inferred defaults defined in ADR-8. It fails loudly if:
//   - id fails the slug guard (R-062)
//   - id is already present in reg.Skills (R-061)
//
// repo and ref control the source type (ADR-14):
//   - repo == "" → Source.Type = "custom" (backward-compatible default, R-128)
//   - repo != "" → Source.Type = "external" with Repo/Ref set (R-127)
//
// The input registry is never mutated (pure function).
func AddEntry(reg Registry, id, repo, ref string) (Registry, error) {
	if !slugRe.MatchString(id) {
		return Registry{}, fmt.Errorf("id %q: invalid slug (must match ^[a-z0-9][a-z0-9-]*$)", id)
	}
	for _, e := range reg.Skills {
		if e.ID == id {
			return Registry{}, fmt.Errorf("id %q: already registered", id)
		}
	}

	src := Source{Type: "custom"}
	if repo != "" {
		src = Source{Type: "external", Repo: repo, Ref: ref}
	}

	newEntry := Entry{
		ID:     id,
		Path:   id,
		Source: src,
		Install: Install{
			DefaultScope:    "global",
			Targets:         []string{"claude", "opencode", "codex"},
			AllowedProjects: nil, // must be nil, not []string{} — ADR-8, ADR-7
		},
		Lifecycle: Lifecycle{
			UpdateStrategy: "overlay-only",
		},
	}

	// Copy input slice to avoid mutating the caller's backing array.
	skills := make([]Entry, len(reg.Skills), len(reg.Skills)+1)
	copy(skills, reg.Skills)
	skills = append(skills, newEntry)

	return Registry{Version: reg.Version, Skills: skills}, nil
}

// RemoveEntry returns a new Registry with the entry for id removed. It fails
// loudly if id is not present in reg.Skills (R-069). The relative order of
// remaining entries is preserved (R-070). The input registry is never mutated.
func RemoveEntry(reg Registry, id string) (Registry, error) {
	found := false
	for _, e := range reg.Skills {
		if e.ID == id {
			found = true
			break
		}
	}
	if !found {
		return Registry{}, fmt.Errorf("id %q: not found in registry", id)
	}

	skills := make([]Entry, 0, len(reg.Skills)-1)
	for _, e := range reg.Skills {
		if e.ID != id {
			skills = append(skills, e)
		}
	}

	// Normalize to nil when empty so serialize→parse round-trip holds:
	// ParseRegistry returns nil Skills for an empty sequence (ADR-8).
	if len(skills) == 0 {
		skills = nil
	}

	return Registry{Version: reg.Version, Skills: skills}, nil
}

// ── I/O helpers ─────────────────────────────────────────────────────────────

// writeFileAtomic writes data to a temp file in the same directory as path,
// syncs, and returns the temp file path. The caller is responsible for the
// final os.Rename. This pattern ensures each file is written atomically
// (ADR-9 dual-temp + rename).
func writeFileAtomic(path string, data []byte) (string, error) {
	dir := path[:strings.LastIndex(path, string(os.PathSeparator))+1]
	if dir == "" {
		dir = "."
	}
	tmp, err := os.CreateTemp(dir, ".tmp-skills-*")
	if err != nil {
		return "", fmt.Errorf("writeFileAtomic: create temp: %w", err)
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return "", fmt.Errorf("writeFileAtomic: write: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(name)
		return "", fmt.Errorf("writeFileAtomic: sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return "", fmt.Errorf("writeFileAtomic: close: %w", err)
	}
	return name, nil
}

// appendManifestLine returns src with "<id>/SKILL.md custom" appended,
// ensuring exactly one trailing newline before appending (ADR-5).
func appendManifestLine(src []byte, id string) []byte {
	out := make([]byte, len(src))
	copy(out, src)
	if len(out) > 0 && out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	return append(out, []byte(id+"/SKILL.md custom\n")...)
}

// filterManifestLines returns src with all lines whose first field equals
// "<id>/SKILL.md" removed. All other lines are preserved verbatim (ADR-5).
func filterManifestLines(src []byte, id string) []byte {
	prefix := id + "/SKILL.md"
	var out []byte
	for _, line := range strings.Split(string(src), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == prefix {
			continue
		}
		out = append(out, []byte(line+"\n")...)
	}
	// Remove the extra trailing newline that the split+join adds.
	if len(out) > 0 && bytes.HasSuffix(out, []byte("\n\n")) && !bytes.HasSuffix(src, []byte("\n\n")) {
		out = out[:len(out)-1]
	}
	return out
}

// parseFlags extracts --registry, --manifest, --source-root, --repo, --ref flag
// values and the first non-flag positional argument (the skill id) from args.
func parseFlags(args []string) (registryPath, manifestPath, sourceRoot, id, repo, ref string) {
	registryPath = "skills.registry.yaml"
	manifestPath = "overlay.manifest"
	sourceRoot = "skills"
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--registry":
			if i+1 < len(args) {
				registryPath = args[i+1]
				i++
			}
		case "--manifest":
			if i+1 < len(args) {
				manifestPath = args[i+1]
				i++
			}
		case "--source-root":
			if i+1 < len(args) {
				sourceRoot = args[i+1]
				i++
			}
		case "--repo":
			if i+1 < len(args) {
				repo = args[i+1]
				i++
			}
		case "--ref":
			if i+1 < len(args) {
				ref = args[i+1]
				i++
			}
		default:
			if !strings.HasPrefix(args[i], "--") && id == "" {
				id = args[i]
			}
		}
	}
	return registryPath, manifestPath, sourceRoot, id, repo, ref
}

// AddCore is the testable CLI core for `engine skills add <id>`.
// It reads the registry and manifest, adds the new entry (pure), validates
// the in-memory state, then writes both files atomically (manifest-first per
// ADR-9). All side effects are injected; no global state is used.
//
// Preconditions checked before any write:
//   - id slug is valid (delegated to AddEntry)
//   - id is not already registered (delegated to AddEntry)
//   - <sourceRoot>/<id>/SKILL.md exists (R-060)
//   - Serialize + re-parse of new registry is consistent (R-063)
//   - registry + updated manifest cross-check has zero divergences (ADR-9 step 7)
func AddCore(args []string, readFile readFileFn, statFile func(string) (os.FileInfo, error), stdout, stderr io.Writer, exit func(int)) {
	registryPath, manifestPath, sourceRoot, id, repo, ref := parseFlags(args)

	if id == "" {
		fmt.Fprintln(stderr, "error: skills add requires an <id> argument")
		exit(1)
		return
	}

	// Defensive guard: --ref without --repo is a usability error (ADR-14).
	// A lone ref on a custom entry would be silently dropped — reject loudly instead.
	if ref != "" && repo == "" {
		fmt.Fprintln(stderr, "error: --ref requires --repo (ref is only valid for external entries)")
		exit(1)
		return
	}

	// 1. Load and parse the registry.
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

	// 2. Apply the pure transform.
	newReg, err := AddEntry(reg, id, repo, ref)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		exit(1)
		return
	}

	// 3. Precondition: <sourceRoot>/<id>/SKILL.md must exist (R-060).
	skillMDPath := sourceRoot + string(os.PathSeparator) + id + string(os.PathSeparator) + "SKILL.md"
	if _, err := statFile(skillMDPath); err != nil {
		fmt.Fprintf(stderr, "error: skill %q: SKILL.md not found at %q: %v\n", id, skillMDPath, err)
		exit(1)
		return
	}

	// 4. Serialize the new registry.
	regBytes, err := Serialize(newReg)
	if err != nil {
		fmt.Fprintf(stderr, "error: serializing registry: %v\n", err)
		exit(1)
		return
	}

	// 5. Validate-before-write: re-parse must equal in-memory state (R-063).
	reg2, err := ParseRegistry(bytes.NewReader(regBytes))
	if err != nil {
		fmt.Fprintf(stderr, "error: validate-before-write re-parse failed: %v\n", err)
		exit(1)
		return
	}
	if !reflect.DeepEqual(newReg, reg2) {
		fmt.Fprintln(stderr, "error: validate-before-write: serialize→parse round-trip mismatch")
		exit(1)
		return
	}

	// 6. Build updated manifest bytes and cross-check registry ↔ manifest (ADR-9 step 7).
	manData, err := readFile(manifestPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: reading manifest %q: %v\n", manifestPath, err)
		exit(1)
		return
	}
	manBytes := appendManifestLine(manData, id)
	mv, err := loadManifestViewReader(bytes.NewReader(manBytes))
	if err != nil {
		fmt.Fprintf(stderr, "error: parsing manifest: %v\n", err)
		exit(1)
		return
	}
	if divs := Diff(reg2, mv); len(divs) > 0 {
		for _, d := range divs {
			fmt.Fprintf(stderr, "[%s] %s: %s\n", d.Class, d.Path, d.Detail)
		}
		exit(1)
		return
	}

	// 7. Dual-temp atomic write: manifest first, then registry (ADR-9).
	manTemp, err := writeFileAtomic(manifestPath, manBytes)
	if err != nil {
		fmt.Fprintf(stderr, "error: writing manifest: %v\n", err)
		exit(1)
		return
	}
	regTemp, err := writeFileAtomic(registryPath, regBytes)
	if err != nil {
		os.Remove(manTemp)
		fmt.Fprintf(stderr, "error: writing registry: %v\n", err)
		exit(1)
		return
	}

	// 8. Rename: manifest first, then registry.
	if err := os.Rename(manTemp, manifestPath); err != nil {
		os.Remove(manTemp)
		os.Remove(regTemp)
		fmt.Fprintf(stderr, "error: finalizing manifest: %v\n", err)
		exit(1)
		return
	}
	if err := os.Rename(regTemp, registryPath); err != nil {
		os.Remove(regTemp)
		fmt.Fprintf(stderr, "error: finalizing registry: %v\n", err)
		exit(1)
		return
	}

	fmt.Fprintf(stdout, "added: %s\n", id)
	exit(0)
}

// RemoveCore is the testable CLI core for `engine skills remove <id>`.
// It reads the registry and manifest, removes the entry (pure), validates
// the in-memory state, then writes both files atomically (manifest-first per
// ADR-9). All side effects are injected; no global state is used.
func RemoveCore(args []string, readFile readFileFn, stdout, stderr io.Writer, exit func(int)) {
	registryPath, manifestPath, _, id, _, _ := parseFlags(args)

	if id == "" {
		fmt.Fprintln(stderr, "error: skills remove requires an <id> argument")
		exit(1)
		return
	}

	// 1. Load and parse the registry.
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

	// 2. Apply the pure transform.
	newReg, err := RemoveEntry(reg, id)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		exit(1)
		return
	}

	// 3. Serialize the new registry.
	regBytes, err := Serialize(newReg)
	if err != nil {
		fmt.Fprintf(stderr, "error: serializing registry: %v\n", err)
		exit(1)
		return
	}

	// 4. Validate-before-write: re-parse must equal in-memory state (R-063).
	reg2, err := ParseRegistry(bytes.NewReader(regBytes))
	if err != nil {
		fmt.Fprintf(stderr, "error: validate-before-write re-parse failed: %v\n", err)
		exit(1)
		return
	}
	if !reflect.DeepEqual(newReg, reg2) {
		fmt.Fprintln(stderr, "error: validate-before-write: serialize→parse round-trip mismatch")
		exit(1)
		return
	}

	// 5. Build updated manifest bytes and cross-check registry ↔ manifest (ADR-9 step 7).
	manData, err := readFile(manifestPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: reading manifest %q: %v\n", manifestPath, err)
		exit(1)
		return
	}
	manBytes := filterManifestLines(manData, id)
	mv, err := loadManifestViewReader(bytes.NewReader(manBytes))
	if err != nil {
		fmt.Fprintf(stderr, "error: parsing manifest: %v\n", err)
		exit(1)
		return
	}
	if divs := Diff(reg2, mv); len(divs) > 0 {
		for _, d := range divs {
			fmt.Fprintf(stderr, "[%s] %s: %s\n", d.Class, d.Path, d.Detail)
		}
		exit(1)
		return
	}

	// 6. Dual-temp atomic write: manifest first, then registry (ADR-9).
	manTemp, err := writeFileAtomic(manifestPath, manBytes)
	if err != nil {
		fmt.Fprintf(stderr, "error: writing manifest: %v\n", err)
		exit(1)
		return
	}
	regTemp, err := writeFileAtomic(registryPath, regBytes)
	if err != nil {
		os.Remove(manTemp)
		fmt.Fprintf(stderr, "error: writing registry: %v\n", err)
		exit(1)
		return
	}

	// 7. Rename: manifest first, then registry.
	if err := os.Rename(manTemp, manifestPath); err != nil {
		os.Remove(manTemp)
		os.Remove(regTemp)
		fmt.Fprintf(stderr, "error: finalizing manifest: %v\n", err)
		exit(1)
		return
	}
	if err := os.Rename(regTemp, registryPath); err != nil {
		os.Remove(regTemp)
		fmt.Fprintf(stderr, "error: finalizing registry: %v\n", err)
		exit(1)
		return
	}

	fmt.Fprintf(stdout, "removed: %s\n", id)
	exit(0)
}
