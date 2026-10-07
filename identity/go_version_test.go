package identity

import (
	"os"
	"strings"
	"testing"
)

// goDirective is the version a go.mod asks for ("go 1.21").
func goDirective(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if fields := strings.Fields(line); len(fields) == 2 && fields[0] == "go" {
			return fields[1]
		}
	}
	t.Fatalf("%s has no go directive", path)
	return ""
}

// The job that tests this module takes its Go version from this go.mod, so it tests on the oldest
// toolchain the module says it supports. The engine imports the module through a local replace and
// is built with its own go.mod's version: the two are held equal, so that the module is tested on
// the toolchain its main consumer builds with, and neither moves without the other.
func TestTheModuleAsksForTheGoVersionTheEngineDoes(t *testing.T) {
	if got, want := goDirective(t, "go.mod"), goDirective(t, "../engine/go.mod"); got != want {
		t.Errorf("identity/go.mod says go %s and engine/go.mod says go %s: they are held equal", got, want)
	}
}
