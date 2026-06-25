package skills

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"
)

// readFileFn is the type of a function that reads a file by path.
// Injected for testability; production code uses os.ReadFile.
type readFileFn func(string) ([]byte, error)

// parseRegistryFlag extracts --registry <path> from args.
// Defaults to "skills.registry.yaml" (CWD-relative, per R-023).
func parseRegistryFlag(args []string) string {
	path := "skills.registry.yaml"
	for i := 0; i < len(args); i++ {
		if args[i] == "--registry" && i+1 < len(args) {
			path = args[i+1]
			i++
		}
	}
	return path
}

// RenderList writes one sorted line per registry entry to w.
// Format per line: id<TAB>source.type<TAB>lifecycle.updateStrategy<TAB>targets (comma-joined).
// Entries are sorted by id for deterministic output (R-021).
func RenderList(w io.Writer, reg Registry) {
	entries := make([]Entry, len(reg.Skills))
	copy(entries, reg.Skills)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ID < entries[j].ID
	})
	for _, e := range entries {
		targets := strings.Join(e.Install.Targets, ",")
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.ID, e.Source.Type, e.Lifecycle.UpdateStrategy, targets)
	}
}

// RenderListCore is the testable CLI core for `engine skills list`.
// All I/O is injected so every branch is unit-testable without OS access.
//
// Reads registry via readFile(path), sorts entries by id, writes tab-separated
// lines to stdout. Exits 1 on read or parse errors (R-022).
func RenderListCore(args []string, readFile readFileFn, stdout, stderr io.Writer, exit func(int)) {
	registryPath := parseRegistryFlag(args)
	data, err := readFile(registryPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: reading registry %q: %v\n", registryPath, err)
		exit(1)
		return
	}
	reg, err := ParseRegistry(bytes.NewReader(data))
	if err != nil {
		fmt.Fprintf(stderr, "error: parsing registry: %v\n", err)
		exit(1)
		return
	}
	RenderList(stdout, reg)
}
