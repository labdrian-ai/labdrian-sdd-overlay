package main

import (
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/assets"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/status"
)

// The markers of the two blocks the verbs scope into a registry, written out. They are what the
// registries in the field already hold, and a registry that holds the old text is not found by a
// marker that reads differently, so no test derives them from the table they check.
const (
	minimalismBeginMarker = "<!-- BEGIN: minimalism-contract-scope (auto-generated) -->"
	minimalismEndMarker   = "<!-- END: minimalism-contract-scope -->"
	designBeginMarker     = "<!-- BEGIN: anti-generic-design-scope (auto-generated) -->"
	designEndMarker       = "<!-- END: anti-generic-design-scope -->"
)

func TestTheManagedContractsAreTheTwoTheRegistriesHold(t *testing.T) {
	type want struct {
		name, defaultPath, begin, end string
		embedded                      bool
	}
	wants := []want{
		{"minimalism-contract", "skills/_shared/minimalism-contract.md", minimalismBeginMarker, minimalismEndMarker, false},
		{"anti-generic-design", "skills/_shared/anti-generic-design.md", designBeginMarker, designEndMarker, true},
	}
	got := managedContracts()
	if len(got) != len(wants) {
		t.Fatalf("%d managed contracts, want %d", len(got), len(wants))
	}
	for i, w := range wants {
		c := got[i]
		if c.name != w.name || c.defaultPath != w.defaultPath || c.beginMarker() != w.begin || c.endMarker() != w.end {
			t.Errorf("managed contract %d = {%q %q %q %q}, want {%q %q %q %q}", i,
				c.name, c.defaultPath, c.beginMarker(), c.endMarker(), w.name, w.defaultPath, w.begin, w.end)
		}
		if (c.content != nil) != w.embedded {
			t.Errorf("%s: carries its text in the binary = %v, want %v", c.name, c.content != nil, w.embedded)
		}
	}
	if text := got[1].content(); text != assets.AntiGenericDesign() || text == "" {
		t.Error("the text the binary carries for anti-generic-design is not the embedded asset")
	}
}

// Two contracts with a marker or a label in common would overwrite each other's block, and a
// marker that does not open (or close) a block is one the scan of the registry cannot see.
func TestTheManagedContractsDoNotShareAMarkerOrALabelAndEachMarkerIsAMarker(t *testing.T) {
	seen := map[string]string{}
	claim := func(kind, text, owner string) {
		t.Helper()
		if text == "" {
			t.Errorf("%s: the %s is empty", owner, kind)
		}
		if other, taken := seen[text]; taken {
			t.Errorf("%s and %s have the same text for a %s: %q", owner, other, kind, text)
		}
		seen[text] = owner + " " + kind
	}
	for _, c := range managedContracts() {
		claim("begin marker", c.beginMarker(), c.name)
		claim("end marker", c.endMarker(), c.name)
		claim("row label", c.name, c.name)
		if !strings.HasPrefix(c.beginMarker(), "<!-- BEGIN:") || !strings.HasPrefix(c.endMarker(), "<!-- END:") {
			t.Errorf("%s: markers %q and %q do not open and close a block", c.name, c.beginMarker(), c.endMarker())
		}
	}
}

func TestOnlyAContractWithTextInTheBinaryIsAnEmbeddedOne(t *testing.T) {
	if c, ok := embeddedContract("anti-generic-design"); !ok || c.name != "anti-generic-design" || c.content() != assets.AntiGenericDesign() {
		t.Errorf("embeddedContract(anti-generic-design) = (%+v, %v)", c.name, ok)
	}
	for _, name := range []string{"minimalism-contract", "", "Anti-Generic-Design", "nope"} {
		if _, ok := embeddedContract(name); ok {
			t.Errorf("embeddedContract(%q) is a contract; only the one whose text the binary carries is", name)
		}
	}
}

func TestTheRegistryCheckOfStatusLooksForEachManagedBlockByItsOpeningMarker(t *testing.T) {
	want := []status.ScopedBlock{
		{Name: "minimalism-contract-scope", BeginMarker: minimalismBeginMarker},
		{Name: "anti-generic-design-scope", BeginMarker: designBeginMarker},
	}
	got := scopedBlocks()
	if len(got) != len(want) {
		t.Fatalf("%d blocks, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("block %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestAManagedContractScopesItsBlockWithItsOwnMarkersAndTheGivenPath(t *testing.T) {
	cfg := managedContracts()[1].config("custom/path.md")
	if cfg.ContractPath != "custom/path.md" || cfg.BeginMarker != designBeginMarker || cfg.EndMarker != designEndMarker || cfg.RowLabel != "anti-generic-design" {
		t.Errorf("config = %+v", cfg)
	}
}
