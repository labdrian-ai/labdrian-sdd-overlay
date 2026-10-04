package skills_test

// Registers the YAML adapter of the skills registry with the tests of package skills, which cannot
// import it themselves (see export_test.go). It runs before any test does.

import (
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/registryyaml"
)

func init() {
	skills.UseYAMLRegistries(func(read func(path string) ([]byte, error)) skills.RegistryRepository {
		return registryyaml.NewRepository(read)
	})
}
