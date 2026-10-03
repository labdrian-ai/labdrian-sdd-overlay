package registryyaml_test

import (
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/registryyaml"
)

// readRegistry reads the YAML text of a registry as the program does: the adapter decodes it and
// the domain judges what it decoded.
func readRegistry(text string) (skills.Registry, error) { return readRegistryBytes([]byte(text)) }

func readRegistryBytes(data []byte) (skills.Registry, error) {
	return skills.DecodeRegistry(registryyaml.NewRepository(nil), data)
}
