package skills

import (
	"fmt"
	"path/filepath"
	"strings"
)

// The vocabulary of a registry: the words its fields take. The domain owns it; an adapter that
// stores a registry in a file says each word in the file's way and maps it to these.
const (
	// The scope an entry is installed in: for every project (global), or for the projects it names.
	ScopeGlobal  = "global"
	ScopeProject = "project"

	// What an entry's source is: a skill of the upstream distribution, one of the overlay's own,
	// or one vendored from another repository.
	SourceCore     = "core"
	SourceCustom   = "custom"
	SourceExternal = "external"
)

// registryTargets are the runtimes a skill can be projected to, in the order a refusal lists them.
// They are the one list: what an entry may name is what a refusal says it may name.
var registryTargets = []string{"claude", "opencode", "codex", "pi"}

// RegistryTargets are the runtimes a skill can be projected to, in the order a refusal lists them.
// Each call returns a list of its own.
func RegistryTargets() []string { return append([]string(nil), registryTargets...) }

// isRegistryTarget says whether a skill can be projected to the runtime.
func isRegistryTarget(target string) bool {
	for _, t := range registryTargets {
		if t == target {
			return true
		}
	}
	return false
}

var (
	validSourceTypes      = map[string]bool{SourceCore: true, SourceCustom: true, SourceExternal: true}
	validUpdateStrategies = map[string]bool{"vendor-merge": true, "overlay-only": true}
)

// Validate says whether the registry may hold what it holds, and, when it may not, the first
// thing that is wrong: an entry that is not valid, or an id that was already taken. It is a pure
// rule of the model and the only one: it looks at no file and no format, and it is the domain
// that applies it to every registry an adapter hands over (ReadRegistry, DecodeRegistry).
//
// (Validate, the package function of validate.go, is another thing: it compares a registry with the
// manifest.)
//
// The entries are judged in order, and each is judged before the check that its id is new, which
// is the order a registry has always been refused in.
func (r Registry) Validate() error {
	seen := make(map[string]bool, len(r.Skills))
	for i := range r.Skills {
		e := &r.Skills[i]
		if err := validateEntry(e); err != nil {
			return err
		}
		if seen[e.ID] {
			return fmt.Errorf("skills: duplicate id %q", e.ID)
		}
		seen[e.ID] = true
	}
	return nil
}

// validateEntry enforces the constraints on a single Entry. It stays free of the file system by
// design (D4): the path rules are string logic.
func validateEntry(e *Entry) error {
	if e.ID == "" {
		return fmt.Errorf("skills: entry is missing required field 'id'")
	}
	// R-003: path must be non-empty, relative, contain no ".." component,
	// and already be Clean — this is the shared containment boundary
	// pipkg relies on for both the source (overlayRoot/skills/<path>) and
	// destination (skillsDir/<path>) joins.
	if e.Path == "" {
		return fmt.Errorf("skills: entry %q: path must not be empty", e.ID)
	}
	if filepath.IsAbs(e.Path) {
		return fmt.Errorf("skills: entry %q: path %q must be relative, not absolute", e.ID, e.Path)
	}
	if filepath.Clean(e.Path) != e.Path {
		return fmt.Errorf("skills: entry %q: path %q must already be a clean relative path", e.ID, e.Path)
	}
	for _, part := range strings.Split(e.Path, "/") {
		if part == ".." {
			return fmt.Errorf("skills: entry %q: path %q must not contain a %q component", e.ID, e.Path, "..")
		}
	}
	if !validSourceTypes[e.Source.Type] {
		return fmt.Errorf("skills: entry %q: source.type %q is not valid; must be 'core', 'custom', or 'external'", e.ID, e.Source.Type)
	}
	if e.Source.Type == SourceCustom && e.Source.Upstream != nil {
		return fmt.Errorf("skills: entry %q: source.upstream is not allowed when source.type is 'custom'", e.ID)
	}
	// WARNING-1: external entries must not carry an upstream block (ADR-11).
	if e.Source.Type == SourceExternal && e.Source.Upstream != nil {
		return fmt.Errorf("skills: entry %q: source.upstream is not allowed when source.type is 'external'", e.ID)
	}
	if e.Source.Type == SourceCore && e.Source.Upstream != nil && e.Source.Upstream.Owner == "" {
		return fmt.Errorf("skills: entry %q: source.upstream.owner must not be empty", e.ID)
	}
	// An entry that says no scope is accepted, as it always was, and is read as neither: the rule
	// refuses a scope that is a word outside the vocabulary. (A file's adapter words the same
	// refusal with the line it is on, and refuses it before this rule is asked.)
	if s := e.Install.DefaultScope; s != "" && s != ScopeGlobal && s != ScopeProject {
		return fmt.Errorf("skills: entry %q: install.defaultScope %q is not valid; must be 'global' or 'project'", e.ID, s)
	}
	if e.Install.DefaultScope == ScopeGlobal && len(e.Install.AllowedProjects) > 0 {
		return fmt.Errorf("skills: entry %q: allowedProjects is only valid for project-scoped entries", e.ID)
	}
	if len(e.Install.Targets) == 0 {
		return fmt.Errorf("skills: entry %q: install.targets must not be empty (R-007)", e.ID)
	}
	for _, target := range e.Install.Targets {
		if !isRegistryTarget(target) {
			return fmt.Errorf("skills: entry %q: install.targets contains invalid value %q; must be one of: %s", e.ID, target, strings.Join(registryTargets, ", "))
		}
	}
	if !validUpdateStrategies[e.Lifecycle.UpdateStrategy] {
		return fmt.Errorf("skills: entry %q: lifecycle.updateStrategy %q is not valid; must be 'vendor-merge' or 'overlay-only'", e.ID, e.Lifecycle.UpdateStrategy)
	}
	return nil
}

// SharedPath is a path that more than one entry of a registry holds, with the ids that hold it in
// the order of the registry.
type SharedPath struct {
	Path string
	IDs  []string
}

// SharedPaths are the paths that two or more entries hold, in the order the registry first says
// each. Two ids on one path are accepted (decision 6 of the owner, as they always were: Validate
// does not refuse them), and validate says so in a note (Note), because it is a shape worth a look
// and not a fault.
func (r Registry) SharedPaths() []SharedPath {
	var shared []SharedPath
	index := make(map[string]int, len(r.Skills))
	for _, e := range r.Skills {
		i, seen := index[e.Path]
		if !seen {
			index[e.Path] = len(shared)
			shared = append(shared, SharedPath{Path: e.Path, IDs: []string{e.ID}})
			continue
		}
		shared[i].IDs = append(shared[i].IDs, e.ID)
	}
	held := shared[:0]
	for _, s := range shared {
		if len(s.IDs) > 1 {
			held = append(held, s)
		}
	}
	if len(held) == 0 {
		return nil
	}
	return held
}

// Note is what a person is told of a shared path: the ids, and the path they share.
func (s SharedPath) Note() string {
	ids := make([]string, len(s.IDs))
	for i, id := range s.IDs {
		ids[i] = fmt.Sprintf("%q", id)
	}
	last := len(ids) - 1
	return fmt.Sprintf("note: the skills %s and %s share the path %q", strings.Join(ids[:last], ", "), ids[last], s.Path)
}
