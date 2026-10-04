package main

// The skills registry repository of the program. engine/skills owns the port
// (skills.RegistryRepository) and engine/skills/registryyaml is its adapter, the YAML file
// skills.registry.yaml; the composition root is the one place that builds it, with the one way the
// program has of reading a file, and hands it to everything that reads or writes a registry: the
// skills verbs, the Pi package ('pipkg build|check') and the Pi runtime adapter.

import (
	"os"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/registryyaml"
)

// newRegistryRepository returns the repository of the registry file, read from the file system.
func newRegistryRepository() skills.RegistryRepository {
	return registryyaml.NewRepository(os.ReadFile)
}
