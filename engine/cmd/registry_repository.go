package main

// The skills registry repository of the program. engine/skills owns the port
// (skills.RegistryRepository) and engine/skills/registryyaml is its adapter, the YAML file
// skills.registry.yaml; the composition root is the one place that builds it, with the one way the
// program has of reading a file, and hands it to everything that reads or writes a registry: the
// skills verbs, the Pi package ('pipkg build|check') and the Pi runtime adapter.

import (
	"io"
	"os"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/registryyaml"
)

// newRegistryRepository returns the repository of the registry file, read from the file system.
func newRegistryRepository() skills.RegistryRepository {
	return registryyaml.NewRepository(os.ReadFile)
}

// newWarningRegistryRepository is newRegistryRepository for what reads a registry through the
// repository it is given and has no stderr of its own, the Pi package and the Pi runtime adapter:
// it tells stderr what the reader left out of the registry, as the skills verbs do.
func newWarningRegistryRepository(stderr io.Writer) skills.RegistryRepository {
	return skills.WarnOfUnread(newRegistryRepository(), stderr)
}
