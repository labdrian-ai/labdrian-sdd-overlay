package vault

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/query"
)

// The adapter is what the query package's port asks for.
var _ query.VaultRetriever = Retriever{}

// The query package and this one bound a result alike (D8: both sources share one bound), and each names it for
// itself; this holds the two numbers to one another.
func TestTheQueryAndTheVaultShareTheirDefaultBound(t *testing.T) {
	if query.DefaultTopN != DefaultTopN {
		t.Fatalf("query.DefaultTopN = %d, vault.DefaultTopN = %d, want one bound", query.DefaultTopN, DefaultTopN)
	}
}

func TestRetrieverAnswersTheCandidatesInTheQuerysTerms(t *testing.T) {
	vaultRoot, _ := newFixtureVault(t, retrieveOKFixture)

	got, err := Retriever{Root: vaultRoot}.RetrieveVault(context.Background(), "labdrian-sdd-overlay", "what is dragonscale?", 7)
	if err != nil {
		t.Fatalf("RetrieveVault: %v", err)
	}

	want := []query.VaultCandidate{
		{PageAddress: "c-000001", AbsolutePath: "/vault/wiki/memory/c-000001.md", BM25Score: 7.125, RerankScore: 0.81, Snippet: "first snippet"},
		{PageAddress: "c-000002", AbsolutePath: "/vault/wiki/memory/c-000002.md", BM25Score: 5.44, RerankScore: 0.62, Snippet: "second snippet"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("candidates = %+v, want %+v", got, want)
	}
	if args, wantArgs := capturedArgs(t, vaultRoot), []string{"what is dragonscale?", "--top", "7"}; !reflect.DeepEqual(args, wantArgs) {
		t.Errorf("the script was run with %v, want %v: the bound of the call reaches it", args, wantArgs)
	}
}

// A call with no bound is the vault's default, as it always was.
func TestRetrieverWithNoBoundAsksTheDefault(t *testing.T) {
	vaultRoot, _ := newFixtureVault(t, retrieveOKFixture)

	if _, err := (Retriever{Root: vaultRoot}).RetrieveVault(context.Background(), "p", "q", 0); err != nil {
		t.Fatalf("RetrieveVault: %v", err)
	}

	if args, want := capturedArgs(t, vaultRoot), []string{"q", "--top", "5"}; !reflect.DeepEqual(args, want) {
		t.Errorf("the script was run with %v, want %v", args, want)
	}
}

// The vault's exit code for a never-indexed vault is a typed outcome, not an error whose words are read: it is
// query.ErrVaultNotProvisioned, with no candidates.
func TestRetrieverNamesAVaultThatWasNeverIndexed(t *testing.T) {
	vaultRoot, _ := newFixtureVault(t, retrieveNotProvisionedFixture)

	got, err := Retriever{Root: vaultRoot}.RetrieveVault(context.Background(), "p", "q", 5)

	if !errors.Is(err, query.ErrVaultNotProvisioned) {
		t.Fatalf("error = %v, want query.ErrVaultNotProvisioned", err)
	}
	if got != nil {
		t.Errorf("candidates = %+v, want none", got)
	}
}

// Every other failure is the retrieval's own error, in the words it has always had: the query reports them as the
// diagnostic of a degraded vault.
func TestRetrieverPassesEveryOtherFailureOnUntouched(t *testing.T) {
	for name, tc := range map[string]struct {
		script string
		want   string
	}{
		"a script that exits non-zero": {"import sys\nsys.stderr.write('index is missing\\n')\nsys.exit(3)\n", "vault: retrieve entrypoint exited 3: index is missing\n"},
		"output that is not JSON":      {"print('not json')\n", "vault: parse retrieve entrypoint output: "},
	} {
		t.Run(name, func(t *testing.T) {
			vaultRoot, runner := newFixtureVault(t, tc.script)
			_, direct := Retrieve(context.Background(), runner, "p", "q", 5)

			got, err := Retriever{Root: vaultRoot}.RetrieveVault(context.Background(), "p", "q", 5)

			if err == nil || got != nil {
				t.Fatalf("RetrieveVault = (%+v, %v), want no candidates and an error", got, err)
			}
			if errors.Is(err, query.ErrVaultNotProvisioned) {
				t.Errorf("error %q is taken for a vault that was never indexed", err)
			}
			if !strings.HasPrefix(err.Error(), tc.want) || err.Error() != direct.Error() {
				t.Errorf("error = %q, want the retrieval's own %q", err, direct)
			}
		})
	}
}

// A script that is not there fails as the retrieval fails.
func TestRetrieverOverAVaultWithNoScriptFails(t *testing.T) {
	_, err := Retriever{Root: t.TempDir()}.RetrieveVault(context.Background(), "p", "q", 5)

	if err == nil || !strings.HasPrefix(err.Error(), "vault: invoke retrieve entrypoint: ") {
		t.Fatalf("error = %v, want the retrieval's failure to invoke the entrypoint", err)
	}
}
