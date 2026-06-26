package skills

// Registry is the top-level parsed form of skills.registry.yaml.
type Registry struct {
	Version string
	Skills  []Entry
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
	Type     string    // "core" or "custom"
	Upstream *Upstream // optional; only valid when Type == "core"
}

// Upstream describes the upstream provider for a core skill.
type Upstream struct {
	Owner string
}

// Install describes installation parameters for a skill.
type Install struct {
	DefaultScope    string   // "global" | "project" (enum; validated in parse.go)
	Targets         []string // non-empty subset of {"claude","opencode","codex"} (A-1)
	AllowedProjects []string // optional; required when DefaultScope == "project"
}

// Lifecycle describes how a skill is updated over time.
type Lifecycle struct {
	UpdateStrategy string // "vendor-merge" or "overlay-only"
}
