// Package registryyaml is the YAML file of the skills registry (skills.registry.yaml), as an
// adapter of skills.RegistryRepository: it reads the file into the registry model the skills
// domain owns, and writes the model back as the one form of the file the program writes.
//
// It knows the file and nothing of what a registry means. Whether a registry may hold what it
// holds is the domain's rule (skills.Registry.Validate), applied by the domain to what this package
// returns (skills.ReadRegistry); nothing in this package calls it, and a test fails if something
// does. The two checks it keeps that name a line (a scope outside its two words, a source that
// contradicts its type) are checks of the file, worded with the line they are on, and the domain
// holds every registry to the same rules again.
//
// The file format is a strict subset of YAML (see Decode for what is refused and why, Encode for
// the one form that is written).
package registryyaml
