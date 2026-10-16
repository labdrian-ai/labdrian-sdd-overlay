package installer_test

// The repair of the registry is the engine's (`registry repair-sdd`), so a machine that has neither
// the engine binary nor Go to build it with cannot repair one. These tests hold how it stops, as
// engine_absent_test.go holds it for the commands that read the manifest: exit 1, the words that
// name the binary and say Go is not installed, and nothing written (registry_pins_world_test.go has
// the world).

import (
	"os"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/enginetest"
)

func TestWithNeitherTheEngineNorGoTheRegistryIsNotRepairedAndNothingIsWritten(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"repair-sdd-registry", []string{"repair-sdd-registry", "--no-refresh"}},
		{"repair-sdd-registry after a refresh", []string{"repair-sdd-registry"}},
		{"skill-registry refresh", []string{"skill-registry", "refresh"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := registryWorld(t)
			w.installFew("codex")
			// A gentle-ai that only records, first on a PATH that has every tool of the machine but go.
			g := w.installGentle("", 0)
			if err := os.Remove(enginetest.Path(w.home)); err != nil {
				t.Fatalf("take the engine out of the home: %v", err)
			}
			path := g.dir + string(os.PathListSeparator) + pathWithout(t, "go")
			w.changePath(func(string) string { return path })

			run := w.runIn(w.project(), tc.args...)

			if run.Exit != 1 || !strings.Contains(run.Stderr, engineNotFound) {
				t.Errorf("%s = exit %d, stderr %q, want exit 1 and %q", tc.name, run.Exit, run.Stderr, engineNotFound)
			}
			if registry := w.showRegistry(w.registryPath()); registry != "--- registry\n(absent)\n" {
				t.Errorf("%s left a registry:\n%s", tc.name, registry)
			}
		})
	}
}
