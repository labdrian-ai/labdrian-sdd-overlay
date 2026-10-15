package domain

import (
	"errors"
	"os"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/tui/internal/testhome"
)

func TestMain(m *testing.M) { os.Exit(testhome.Run(m)) }

func TestTheTestsRunInAHomeOfTheirOwn(t *testing.T) { testhome.Check(t) }

// threeCopyTargets is the catalog most tests run against: the three per-file
// targets, as the backend lists them.
func threeCopyTargets() []Target {
	return []Target{
		{Name: "claude", Kind: KindCopy},
		{Name: "opencode", Kind: KindCopy},
		{Name: "codex", Kind: KindCopy},
	}
}

// fourTargets adds the package target, as the backend's own catalog does.
func fourTargets() []Target {
	return append(threeCopyTargets(), Target{Name: "pi", Kind: KindPackage})
}

// fakeCatalog is a TargetCatalog that answers from memory and counts how many
// times it was asked.
type fakeCatalog struct {
	targets []Target
	err     error
	asked   int
}

func (f *fakeCatalog) Targets() ([]Target, error) {
	f.asked++
	if f.err != nil {
		return nil, f.err
	}
	return append([]Target(nil), f.targets...), nil
}

var errCatalogUnreadable = errors.New("catalog unreadable")
