package skills

import (
	"fmt"
	"os"
)

// Load opens the file at path and parses it as a YAML registry subset.
// It fails loud on any I/O or parse error (R-022).
// No logic beyond the file boundary: open → defer close → ParseRegistry.
func Load(path string) (Registry, error) {
	f, err := os.Open(path)
	if err != nil {
		return Registry{}, fmt.Errorf("skills: %w", err)
	}
	defer f.Close()
	return ParseRegistry(f)
}
