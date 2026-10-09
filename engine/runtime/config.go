package runtime

import (
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

// Config is core.Config, kept under its old name while the adapters and the command are switched
// to the core; the commit that has switched the last of them deletes this file.
type Config = core.Config

// underHome joins elems under home, and is empty when there is no home.
func underHome(home string, elems ...string) string {
	if home == "" {
		return ""
	}
	return filepath.Join(append([]string{home}, elems...)...)
}
