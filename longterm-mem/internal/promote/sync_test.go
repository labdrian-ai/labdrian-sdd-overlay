package promote

import (
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// seedPromotedPage writes obs as an already-promoted page at address addr,
// records its precedence entry in store, and returns the written Page, so a
// test starts from a promotion the walk must decide whether to touch.
func seedPromotedPage(t *testing.T, vaultRoot string, store PrecedenceStore, obs memory.Observation, addr string) Page {
	t.Helper()
	page, err := EmitPage(obsMemory(obs), addr, nil, testInstant)
	if err != nil {
		t.Fatalf("EmitPage (seed): %v", err)
	}
	writePromotedPage(t, vaultRoot, page)
	seedPrecedence(store, page)
	return page
}
