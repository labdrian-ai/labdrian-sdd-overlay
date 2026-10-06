package app

import (
	"sort"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// ListInput is what `skills list` is asked: which registry.
type ListInput struct {
	RegistryPath string
}

// ListedSkill is one entry of the registry as list tells it.
type ListedSkill struct {
	ID             string
	SourceType     string
	UpdateStrategy string
	Targets        []string
}

// ListResult is the skills of a registry, sorted by id, and the warning the reader gave of what
// it left out of the registry (empty when it read it whole).
type ListResult struct {
	Skills        []ListedSkill
	UnreadWarning string
}

// ListSkills reads the registry and lists its entries sorted by id, so that the same registry
// is always listed in the same order.
func ListSkills(registries skills.RegistryRepository, in ListInput) (ListResult, error) {
	reg, warning, err := readRegistry(registries, in.RegistryPath)
	if err != nil {
		return ListResult{}, err
	}
	entries := make([]skills.Entry, len(reg.Skills))
	copy(entries, reg.Skills)
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	listed := make([]ListedSkill, 0, len(entries))
	for _, e := range entries {
		listed = append(listed, ListedSkill{
			ID:             e.ID,
			SourceType:     e.Source.Type,
			UpdateStrategy: e.Lifecycle.UpdateStrategy,
			Targets:        append([]string(nil), e.Install.Targets...),
		})
	}
	return ListResult{Skills: listed, UnreadWarning: warning}, nil
}

// StatusInput is what `skills status` is asked: which registry.
type StatusInput struct {
	RegistryPath string
}

// StatusResult counts the entries of a registry by where their source is.
type StatusResult struct {
	Total         int
	Core          int
	Custom        int
	UnreadWarning string
}

// RegistryStatus reads only the registry (never overlay.manifest) and counts its entries.
func RegistryStatus(registries skills.RegistryRepository, in StatusInput) (StatusResult, error) {
	reg, warning, err := readRegistry(registries, in.RegistryPath)
	if err != nil {
		return StatusResult{}, err
	}
	res := StatusResult{Total: len(reg.Skills), UnreadWarning: warning}
	for _, e := range reg.Skills {
		switch e.Source.Type {
		case "core":
			res.Core++
		case "custom":
			res.Custom++
		}
	}
	return res, nil
}
