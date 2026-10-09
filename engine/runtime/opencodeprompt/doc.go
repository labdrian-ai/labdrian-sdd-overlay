// Package opencodeprompt is the part of the OpenCode runtime that decides what the plugin injects
// into a prompt and says whether a recorded decision is still current, without touching a file.
//
// The plugin carries a PromptConfig: for the minimalism contract and for each other contract the
// overlay ships, the phases it applies to and excludes, the line it is injected under, and the
// context it needs. Derive builds it from the text of those contracts, which a ContractSource
// hands over: the adapter (engine/runtime) reads files, this package parses text. Verify compares
// a config recorded on disk with the one the contracts give today, and Hash fingerprints one.
//
// It is a domain package: the pure standard library, the contract parser, nothing of the machine.
package opencodeprompt
