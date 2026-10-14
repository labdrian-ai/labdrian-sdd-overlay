package main

import (
	"os"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/tui/internal/testhome"
)

// Every test of this binary runs in a home of its own: the backend script, git and the TUI would otherwise read
// the state directory, the deployment targets and the git configuration of whoever runs the tests.
func TestMain(m *testing.M) { os.Exit(testhome.Run(m)) }

func TestTheTestsRunInAHomeOfTheirOwn(t *testing.T) { testhome.Check(t) }
