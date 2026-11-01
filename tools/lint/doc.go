// Package lint has no code. Its module pins the staticcheck that CI runs and the errcheck and nilerr that
// labdrian review sweep runs: go.mod names the versions and go.sum the checksums they are built from. See README.md.
//
// The package exists so that go vet ./... and the other per-module checks of the repository find something to check
// here, instead of failing on a module with no packages.
package lint
