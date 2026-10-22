// Package lint has no code. Its module pins the staticcheck that CI runs: go.mod
// names the version and go.sum the checksums it is built from. See README.md.
//
// The package exists so that go vet ./... and the other per-module checks of the
// repository find something to check here, instead of failing on a module with no
// packages.
package lint
