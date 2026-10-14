package skillstalefs

import (
	"os"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/testhome"
)

// TestMain runs the tests of this package in a home directory of their own. They make repositories with git and
// ask it about them, and git reads the global configuration and the global ignore and attributes files it finds
// under HOME and the XDG roots: without this the answers depend on how whoever runs the tests has configured git.
func TestMain(m *testing.M) { os.Exit(testhome.Run(m)) }

// The tests run in the home TestMain made for them, not in the machine's.
func TestTheTestsRunInAHomeOfTheirOwn(t *testing.T) { testhome.Check(t) }
