package skills

import (
	"bytes"
	"fmt"
	"io"
)

// RenderStatus writes a count summary of registry entries to w.
//
// Output format:
//
//	Total:  <n>
//	Core:   <n>
//	Custom: <n>
//	Status: OK
func RenderStatus(w io.Writer, reg Registry) {
	var core, custom int
	for _, e := range reg.Skills {
		switch e.Source.Type {
		case "core":
			core++
		case "custom":
			custom++
		}
	}
	total := len(reg.Skills)
	fmt.Fprintf(w, "Total:  %d\n", total)
	fmt.Fprintf(w, "Core:   %d\n", core)
	fmt.Fprintf(w, "Custom: %d\n", custom)
	fmt.Fprintln(w, "Status: OK")
}

// RenderStatusCore is the testable CLI core for `engine skills status`.
// Reads only the registry file (R-026: never reads overlay.manifest).
// All I/O is injected so every branch is unit-testable without OS access.
func RenderStatusCore(args []string, readFile readFileFn, stdout, stderr io.Writer, exit func(int)) {
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
	RenderStatus(stdout, reg)
}
