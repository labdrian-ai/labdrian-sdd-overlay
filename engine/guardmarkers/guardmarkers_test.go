package guardmarkers_test

import (
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/guardmarkers"
)

// The two texts are written into the hook commands and the deny rule that settings.json holds, and
// the guards of the Pi package match the same words. Changing one is a change to every installed
// settings.json, so the value is pinned here and not left to be read off the constants.
func TestTheMarkersAreTheTextsTheInstalledGuardsMatch(t *testing.T) {
	if guardmarkers.Command != "shaper clearance record" {
		t.Errorf("Command = %q, want the clearance record entry point the guards refuse", guardmarkers.Command)
	}
	if guardmarkers.Store != "labdrian/shaper-clearance" {
		t.Errorf("Store = %q, want the clearance store path segment the guards refuse", guardmarkers.Store)
	}
}
