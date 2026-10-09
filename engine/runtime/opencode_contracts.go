package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/assets"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/opencodeprompt"
)

// promptConfig derives the prompt config from the contracts of the overlay this adapter is
// pointed at. The parsing is opencodeprompt's; what this does is read the files.
func (a OpenCodeAdapter) promptConfig() (opencodeprompt.PromptConfig, error) {
	return opencodeprompt.Derive(&overlayContracts{overlayDir: a.options.OverlayDir})
}

// overlayContracts is the ContractSource over an overlay checkout on disk, plus the guard the
// program embeds. The checkout is the directory the composition root was given
// (OpenCodeOptions.OverlayDir), or, when it was given none, the nearest directory above the
// working directory that holds the minimalism contract.
type overlayContracts struct {
	overlayDir string
	// root is the checkout once it has been resolved, so the contracts of one load come from one
	// directory.
	root string
}

// resolve finds the overlay checkout the first time it is asked.
func (c *overlayContracts) resolve() (string, error) {
	if c.root != "" {
		return c.root, nil
	}
	root, err := locateOverlay(c.overlayDir)
	if err != nil {
		return "", err
	}
	c.root = root
	return root, nil
}

// path is where the contract known as name (a slash path relative to the overlay) is on disk.
func (c *overlayContracts) path(root, name string) string {
	return filepath.Join(root, filepath.FromSlash(name))
}

func (c *overlayContracts) Minimalism() (string, error) {
	root, err := c.resolve()
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(c.path(root, opencodeprompt.MinimalismContractPath))
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func (c *overlayContracts) AntiGenericDesign() string { return assets.AntiGenericDesign }

// OOQuality is optional: a checkout without the file has no such contract, and any other failure
// to read it is an error.
func (c *overlayContracts) OOQuality() (string, bool, error) {
	root, err := c.resolve()
	if err != nil {
		return "", false, err
	}
	content, err := os.ReadFile(c.path(root, opencodeprompt.OOQualityContractPath))
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	return string(content), true, nil
}

// locateOverlay is the overlay checkout the contracts are read from: configured when the
// composition root was given one, which must then be an absolute path; otherwise the nearest
// directory from the working directory upward that has the minimalism contract.
func locateOverlay(configured string) (string, error) {
	if root := strings.TrimSpace(configured); root != "" {
		if filepath.IsAbs(root) {
			return root, nil
		}
		return "", fmt.Errorf("LABDRIAN_OVERLAY_DIR must be absolute, got %q", root)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for dir := cwd; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, filepath.FromSlash(opencodeprompt.MinimalismContractPath))
		if _, err := os.Stat(candidate); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return "", fmt.Errorf("could not locate %s; set LABDRIAN_OVERLAY_DIR", opencodeprompt.MinimalismContractPath)
}
