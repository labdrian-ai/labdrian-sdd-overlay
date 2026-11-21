package agentgen

import "testing"

// The foreman has no Pi or Codex file, and its Claude file has no effort field, so the row holds none: an
// effort written there would be read by nothing.
func TestTheForemanRowHoldsNoEffort(t *testing.T) {
	if foreman.effort != "" {
		t.Errorf("the foreman row holds the effort %q, which no file of the foreman emits", foreman.effort)
	}
}
