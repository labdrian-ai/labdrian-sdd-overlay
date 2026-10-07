package main

// The skills registry repository of the program. engine/skills owns the port
// (skills.RegistryRepository) and engine/skills/registryyaml is its adapter, the YAML file
// skills.registry.yaml; the composition root is the one place that builds it, with the one way the
// program has of reading a file, and hands it to everything that reads or writes a registry: the
// skills verbs, the Pi package ('pipkg build|check') and the Pi runtime adapter.

import (
	"fmt"
	"io"
	"os"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/registryyaml"
)

// newRegistryRepository returns the repository of the registry file, read from the file system.
func newRegistryRepository() skills.RegistryRepository {
	return registryyaml.NewRepository(readRegistryFile)
}

// readRegistryFile reads the registry file at path, and at most registryyaml.MaxFileBytes of it:
// one byte more is read to see that the file is over the bound, and the rest of it never is, so
// asking for a file that is not a registry cannot make the program hold it whole. A file the
// system will not give is told in the system's words, as os.ReadFile told it.
func readRegistryFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, registryyaml.MaxFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > registryyaml.MaxFileBytes {
		return nil, fmt.Errorf("read %s: more than %d bytes, the most a registry may have", path, registryyaml.MaxFileBytes)
	}
	return data, nil
}

// newWarningRegistryRepository is newRegistryRepository for what reads a registry through the
// repository it is given and has no stderr of its own, the Pi package and the Pi runtime adapter:
// it tells stderr what the reader left out of the registry, as the skills verbs do.
func newWarningRegistryRepository(stderr io.Writer) skills.RegistryRepository {
	return skills.WarnOfUnread(newRegistryRepository(), stderr)
}
