package vault

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultlayout"
)

// The adapter is what the promotion's port asks for.
var _ promote.IndexRebuilder = IndexRebuilder{}

// RebuildIndex refreshes an already provisioned vault and provisions a never-indexed one first, as Rebuild does
// without force.
func TestIndexRebuilderRebuildsLikeRebuildWithoutForce(t *testing.T) {
	for name, tc := range map[string]struct {
		provisioned bool
		want        []string
	}{
		"provisioned":   {true, []string{"contextual-prefix --all --no-llm", "bm25-index build"}},
		"never indexed": {false, []string{"setup --no-llm", "contextual-prefix --all --no-llm", "bm25-index build"}},
	} {
		t.Run(name, func(t *testing.T) {
			root, _ := newIndexFixtureVault(t, bm25IndexOKFixture, tc.provisioned)

			if err := (IndexRebuilder{Root: root, Clock: indexClock}).RebuildIndex(context.Background()); err != nil {
				t.Fatalf("RebuildIndex: %v", err)
			}

			if got := callLog(t, root); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("the scripts ran %v, want %v", got, tc.want)
			}
		})
	}
}

// Rebuild with force provisions a vault that is already provisioned again.
func TestIndexRebuilderRebuildWithForceProvisionsAgain(t *testing.T) {
	root, _ := newIndexFixtureVault(t, bm25IndexOKFixture, true)

	if err := (IndexRebuilder{Root: root, Clock: indexClock}).Rebuild(context.Background(), true); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	want := []string{"setup --no-llm", "contextual-prefix --all --no-llm", "bm25-index build"}
	if got := callLog(t, root); !reflect.DeepEqual(got, want) {
		t.Errorf("the scripts ran %v, want %v", got, want)
	}
}

// The rebuilder is the adapter of a vault and the clock of the program together: one built without a clock
// rebuilds nothing, and says so, rather than provisioning a vault it cannot date.
func TestIndexRebuilderWithoutAClockRebuildsNothing(t *testing.T) {
	root, _ := newIndexFixtureVault(t, bm25IndexOKFixture, false)

	err := (IndexRebuilder{Root: root}).RebuildIndex(context.Background())

	if !errors.Is(err, errNoClock) {
		t.Fatalf("RebuildIndex = %v, want the missing clock to be said", err)
	}
	if got := callLog(t, root); len(got) != 0 {
		t.Errorf("the scripts ran %v, want none", got)
	}
}

// The clock the rebuilder holds is the one that dates the vault it provisions.
func TestIndexRebuilderDatesTheSentinelWithItsClock(t *testing.T) {
	root, _ := newIndexFixtureVault(t, bm25IndexOKFixture, false)

	if err := (IndexRebuilder{Root: root, Clock: indexClock}).Rebuild(context.Background(), false); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	got, err := os.ReadFile(vaultlayout.Layout{Root: root}.Path(vaultlayout.ProvisionedSentinel))
	if err != nil || string(got) != indexClockSentinel {
		t.Errorf("sentinel = (%q, %v), want %q", got, err, indexClockSentinel)
	}
}

// A step that fails is the error Rebuild reports, in its words, and never a success.
func TestIndexRebuilderReportsAFailedStepAsRebuildDoes(t *testing.T) {
	root, runner := newIndexFixtureVault(t, bm25IndexFailFixture, true)
	direct := Rebuild(context.Background(), runner, false, indexClock)

	err := (IndexRebuilder{Root: root, Clock: indexClock}).RebuildIndex(context.Background())

	if err == nil || direct == nil || err.Error() != direct.Error() || !strings.Contains(err.Error(), "exited 7") {
		t.Fatalf("error = %v, want what Rebuild says: %v", err, direct)
	}
}
