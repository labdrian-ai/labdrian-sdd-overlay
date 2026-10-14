package installer_test

import (
	"os"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/testhome"
)

// TestMain runs the tests of this package in a home directory of their own. They make repositories with git, run
// the overlay script against them and build the engine with go; git reads the global configuration and the global
// ignore and attributes files it finds under HOME and the XDG roots, and the script derives the overlay and state
// directories from HOME and from the variables that override them. Without this the answers depend on how whoever
// runs the tests has configured git and on what they exported. The tests that need a sandboxed home of their own
// pass one to the script (HOME=...) as before; the go command is kept on the caches it had (testhome).
func TestMain(m *testing.M) { os.Exit(testhome.Run(m)) }

// The tests run in the home TestMain made for them, not in the machine's.
func TestTheTestsRunInAHomeOfTheirOwn(t *testing.T) { testhome.Check(t) }
