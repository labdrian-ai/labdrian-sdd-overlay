package skills

// Registry is the top-level parsed form of skills.registry.yaml.
type Registry struct {
	Version string
	Skills  []Entry

	// Unread says what the reader left out of the stored form, one note per field, in the order
	// the store has them: a field it does not know, or does not need in the shape the store gave
	// it. It is empty when the registry is the whole of what is stored. A registry that left
	// fields out can be read and used, and cannot be written back, because writing it would drop
	// them (AddEntry and RemoveEntry refuse it).
	Unread []string
}

// Entry is a single skill entry in the registry.
type Entry struct {
	ID        string
	Path      string
	Source    Source
	Install   Install
	Lifecycle Lifecycle
}

// Source describes where a skill originates.
type Source struct {
	Type     string    // "core", "custom", or "external"
	Upstream *Upstream // optional; only valid when Type == "core"
	Repo     string    // origin URL; non-empty only for external entries (A-8)
	Ref      string    // vendored commit SHA or tag; optional, non-empty only for external entries (A-8)
}

// Upstream describes the upstream provider for a core skill.
type Upstream struct {
	Owner string
}

// Install describes installation parameters for a skill.
type Install struct {
	DefaultScope    string   // "global" | "project" (enum; Registry.Validate, and the YAML reader on its line)
	Targets         []string // non-empty subset of {"claude","opencode","codex","pi"} (A-1)
	AllowedProjects []string // optional; required when DefaultScope == "project"
}

// Lifecycle describes how a skill is updated over time.
type Lifecycle struct {
	UpdateStrategy string // "vendor-merge" or "overlay-only"
}
