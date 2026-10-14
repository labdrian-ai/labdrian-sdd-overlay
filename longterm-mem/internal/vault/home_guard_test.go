package vault

import (
	"os"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/testhome"
)

// TestMain runs the tests of this package in a home directory of their own. The runner hands the vault's scripts
// the HOME it has (it passes PATH, HOME and LANG through), and the interpreter looks under it for the user's site
// packages, so without this a test imports whatever whoever runs the tests has installed there.
func TestMain(m *testing.M) { os.Exit(testhome.Run(m)) }

// The tests run in the home TestMain made for them, not in the machine's.
func TestTheTestsRunInAHomeOfTheirOwn(t *testing.T) { testhome.Check(t) }
