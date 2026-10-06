package app

import (
	"errors"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// registries is a RegistryRepository that holds registries by location, and fails for the rest
// with the error of a store that cannot be read.
type registries map[string]skills.Registry

func (r registries) Load(location string) (skills.Registry, error) {
	reg, ok := r[location]
	if !ok {
		return skills.Registry{}, &skills.RegistryReadError{Err: errors.New("open " + location + ": no such file or directory")}
	}
	return reg, nil
}

func (registries) Decode([]byte) (skills.Registry, error) {
	return skills.Registry{}, errors.New("not used")
}

func (registries) Encode(skills.Registry) ([]byte, error) { return nil, errors.New("not used") }

// entry is a valid entry of a registry: global, custom, and installed for the given targets.
func entry(id, sourceType string, targets ...string) skills.Entry {
	e := skills.Entry{
		ID:        id,
		Path:      id,
		Source:    skills.Source{Type: sourceType},
		Install:   skills.Install{DefaultScope: "global", Targets: targets},
		Lifecycle: skills.Lifecycle{UpdateStrategy: "overlay-only"},
	}
	if sourceType == "core" {
		e.Source.Upstream = &skills.Upstream{Owner: "upstream"}
		e.Lifecycle.UpdateStrategy = "vendor-merge"
	}
	return e
}

func registryOf(entries ...skills.Entry) skills.Registry {
	return skills.Registry{Version: "1", Skills: entries}
}
