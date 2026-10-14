package projectidfs_test

import (
	"os"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/testhome"
)

// TestMain runs the tests of this package in a home directory of their own. They make repositories with git, and
// git reads the global configuration it finds under HOME and the XDG roots (user.name, init.defaultBranch, the
// global ignore file): without this the repositories are made the way whoever runs the tests has configured git.
func TestMain(m *testing.M) { os.Exit(testhome.Run(m)) }

// The tests run in the home TestMain made for them, not in the machine's.
func TestTheTestsRunInAHomeOfTheirOwn(t *testing.T) { testhome.Check(t) }
