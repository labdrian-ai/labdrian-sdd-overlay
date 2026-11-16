package runtime

import (
	"fmt"
	"os"
	"path/filepath"
)

// OwnerRulesSource is where the Claude adapter gets the text of the owner's rules. The adapter owns
// the port; OverlayOwnerRules answers it from the overlay checkout, and a test hands it a fake.
type OwnerRulesSource interface {
	// OwnerRules is the text to write between the owner-rules markers.
	OwnerRules() (string, error)
}

// ownerRulesSourcePath is the owner rules file, a slash path relative to the overlay checkout.
const ownerRulesSourcePath = "instructions/owner-rules.md"

// OverlayOwnerRules reads the owner rules from the overlay checkout: the one the composition root
// was given ($LABDRIAN_OVERLAY_DIR, see core.Config.LabdrianOverlayDir), or, when it was given none,
// the nearest checkout above the working directory, as the OpenCode adapter finds its contracts.
type OverlayOwnerRules struct {
	OverlayDir string
}

// OwnerRules reads instructions/owner-rules.md of the checkout.
func (o OverlayOwnerRules) OwnerRules() (string, error) {
	root, err := locateOverlay(o.OverlayDir)
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, filepath.FromSlash(ownerRulesSourcePath))
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read the owner rules: %w", err)
	}
	return string(data), nil
}
