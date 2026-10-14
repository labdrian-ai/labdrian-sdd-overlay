package mcpserver

import (
	"os"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/testhome"
)

// TestMain runs the tests of this package in a home directory of their own. They build the program and start it
// as a child with the environment of the test binary, and the go command and git read the configuration they find
// under HOME and the XDG roots: without this the child works in the home of whoever runs the tests.
func TestMain(m *testing.M) { os.Exit(testhome.Run(m)) }

// The tests run in the home TestMain made for them, not in the machine's.
func TestTheTestsRunInAHomeOfTheirOwn(t *testing.T) { testhome.Check(t) }
