package skills

import (
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
// Reads only the registry (R-026: never reads overlay.manifest), through the repository.
// All I/O is injected so every branch is unit-testable without OS access.
func RenderStatusCore(args []string, registries RegistryRepository, stdout, stderr io.Writer, exit func(int)) {
	reg, ok := readRegistryForVerb(registries, parseRegistryFlag(args), false, stderr, exit)
	if !ok {
		return
	}
	RenderStatus(stdout, reg)
}
