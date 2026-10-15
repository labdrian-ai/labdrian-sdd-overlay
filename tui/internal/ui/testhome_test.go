package ui

import (
	"os"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/tui/internal/testhome"
)

// Every test of this binary runs in a home of its own, though nothing here
// should reach a path derived from it: the model asks fakes.
func TestMain(m *testing.M) { os.Exit(testhome.Run(m)) }

func TestTheTestsRunInAHomeOfTheirOwn(t *testing.T) { testhome.Check(t) }
