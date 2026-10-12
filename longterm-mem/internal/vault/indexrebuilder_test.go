package vault

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
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

			if err := (IndexRebuilder{Root: root}).RebuildIndex(context.Background()); err != nil {
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

	if err := (IndexRebuilder{Root: root}).Rebuild(context.Background(), true); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	want := []string{"setup --no-llm", "contextual-prefix --all --no-llm", "bm25-index build"}
	if got := callLog(t, root); !reflect.DeepEqual(got, want) {
		t.Errorf("the scripts ran %v, want %v", got, want)
	}
}

// A step that fails is the error Rebuild reports, in its words, and never a success.
func TestIndexRebuilderReportsAFailedStepAsRebuildDoes(t *testing.T) {
	root, runner := newIndexFixtureVault(t, bm25IndexFailFixture, true)
	direct := Rebuild(context.Background(), runner, false)

	err := (IndexRebuilder{Root: root}).RebuildIndex(context.Background())

	if err == nil || direct == nil || err.Error() != direct.Error() || !strings.Contains(err.Error(), "exited 7") {
		t.Fatalf("error = %v, want what Rebuild says: %v", err, direct)
	}
}
