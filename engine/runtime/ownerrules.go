package runtime

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

// OwnerRulesSource is where an adapter gets the text of the owner's rules, which it writes into the
// file its runtime reads them from (CLAUDE.md, the AGENTS.md files). The adapter owns the port;
// OverlayOwnerRules answers it from the overlay checkout, and a test hands it a fake.
type OwnerRulesSource interface {
	// OwnerRules is the text to write between the owner-rules markers. An error that wraps
	// ErrNoOverlayCheckout says no checkout holds the rules; any other says there is one and it
	// could not be read.
	OwnerRules() (string, error)
}

// ErrNoOverlayCheckout is what OwnerRules answers when no overlay checkout is configured and none is
// above the working directory: a machine where the engine runs apart from a checkout, which has no
// rules to install and is not an error of the install.
var ErrNoOverlayCheckout = errors.New("no overlay checkout found; set " + core.LabdrianOverlayDirVariable)

// ownerRulesSourcePath is the owner rules file, a slash path relative to the overlay checkout.
const ownerRulesSourcePath = "instructions/owner-rules.md"

// OverlayOwnerRules reads the owner rules from the overlay checkout: the one the composition root
// was given ($LABDRIAN_OVERLAY_DIR, see core.Config.LabdrianOverlayDir), or, when it was given none,
// the nearest one above the working directory, as the OpenCode adapter finds its contracts.
type OverlayOwnerRules struct {
	OverlayDir string
}

// OwnerRules reads instructions/owner-rules.md of the checkout.
func (o OverlayOwnerRules) OwnerRules() (string, error) {
	root, err := locateOverlay(o.OverlayDir)
	if err != nil {
		if strings.TrimSpace(o.OverlayDir) == "" {
			return "", ErrNoOverlayCheckout
		}
		return "", err
	}
	path := filepath.Join(root, filepath.FromSlash(ownerRulesSourcePath))
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read the owner rules: %w", err)
	}
	return string(data), nil
}
