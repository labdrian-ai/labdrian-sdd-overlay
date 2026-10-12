package query

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
)

// recordedVaultCall is what the vault retriever was asked.
type recordedVaultCall struct {
	project, query string
	top            int
}

// recordingVault answers like fakeVault and remembers what it was asked.
func recordingVault(calls *[]recordedVaultCall, candidates []VaultCandidate, err error) VaultRetriever {
	return vaultFunc(func(_ context.Context, project, query string, top int) ([]VaultCandidate, error) {
		*calls = append(*calls, recordedVaultCall{project, query, top})
		return candidates, err
	})
}

// The vault is asked once, for the project and the query of the call, with the bound of the call (the default when
// the caller gave none), and not at all when the sources do not name it.
func TestTheVaultIsAskedForTheCallsProjectQueryAndBound(t *testing.T) {
	for _, tc := range []struct {
		name    string
		top     int
		sources []string
		want    []recordedVaultCall
	}{
		{"the bound of the call", 3, []string{SourceVault}, []recordedVaultCall{{"proj-a", "zephyr keyword", 3}}},
		{"the default bound", 0, []string{SourceVault}, []recordedVaultCall{{"proj-a", "zephyr keyword", DefaultTopN}}},
		{"not named", 3, []string{SourceEngramFTS}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []recordedVaultCall
			deps := Deps{Memory: &fakeMemory{}, Vault: recordingVault(&calls, nil, nil), ResolveLink: NoLinkResolver}

			if _, err := Run(context.Background(), deps, Request{Project: "proj-a", Query: "zephyr keyword", Top: tc.top, Sources: tc.sources}); err != nil {
				t.Fatalf("Run: %v", err)
			}

			if !reflect.DeepEqual(calls, tc.want) {
				t.Errorf("the vault was asked %+v, want %+v", calls, tc.want)
			}
		})
	}
}

// A vault that was never indexed is an outcome the retriever names with ErrVaultNotProvisioned, however it wraps
// it: the query is answered from the memory alone, with no diagnostic, and the status says why the vault gave
// nothing. The words of the error are never read.
func TestAVaultThatIsNotProvisionedIsAnOutcomeWhateverWrapsIt(t *testing.T) {
	for name, err := range map[string]error{
		"as it is":  ErrVaultNotProvisioned,
		"wrapped":   fmt.Errorf("vault: retrieve entrypoint: %w", ErrVaultNotProvisioned),
		"joined":    errors.Join(errors.New("other"), ErrVaultNotProvisioned),
		"twice too": fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", ErrVaultNotProvisioned)),
	} {
		t.Run(name, func(t *testing.T) {
			deps := Deps{Memory: &fakeMemory{}, Vault: fakeVault(nil, err), ResolveLink: NoLinkResolver}

			got, runErr := Run(context.Background(), deps, Request{Project: "proj-a", Query: "q", Sources: []string{SourceVault}})
			if runErr != nil {
				t.Fatalf("Run: %v", runErr)
			}
			if got.VaultStatus != VaultStatusNotProvisioned {
				t.Errorf("VaultStatus = %q, want %q", got.VaultStatus, VaultStatusNotProvisioned)
			}
			if len(got.Diagnostics) != 0 {
				t.Errorf("Diagnostics = %+v, want none: a vault that was never indexed has nothing to report", got.Diagnostics)
			}
		})
	}
}

// Any other failure of the retriever is a degraded vault: the call is answered, the status is "error" and the
// diagnostic carries the retriever's own words, untouched.
func TestAVaultThatFailsDegradesTheCallAndSaysWhy(t *testing.T) {
	failure := errors.New("vault: retrieve entrypoint exited 3: the index is missing")
	deps := Deps{Memory: &fakeMemory{}, Vault: fakeVault(nil, failure), ResolveLink: NoLinkResolver}

	got, err := Run(context.Background(), deps, Request{Project: "proj-a", Query: "q", Sources: []string{SourceVault}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.VaultStatus != VaultStatusError {
		t.Errorf("VaultStatus = %q, want %q", got.VaultStatus, VaultStatusError)
	}
	want := []Diagnostic{{Code: DiagnosticVaultSubprocessFailed, Detail: failure.Error()}}
	if !reflect.DeepEqual(got.Diagnostics, want) {
		t.Errorf("Diagnostics = %+v, want %+v", got.Diagnostics, want)
	}
}

// A call that names the vault with no retriever wired, or one that holds a nil pointer, is refused in the query's
// own words instead of panicking in a retriever that is not there; a call that does not name the vault does not
// need one.
func TestACallThatNamesTheVaultNeedsARetriever(t *testing.T) {
	var typedNil *recordedVault
	for name, retriever := range map[string]VaultRetriever{"no retriever": nil, "a nil pointer": typedNil} {
		t.Run(name, func(t *testing.T) {
			deps := Deps{Memory: &fakeMemory{}, Vault: retriever, ResolveLink: NoLinkResolver}

			if _, err := Run(context.Background(), deps, Request{Project: "proj-a", Query: "q", Sources: []string{SourceVault}}); !errors.Is(err, ErrNoVault) {
				t.Fatalf("Run = %v, want ErrNoVault", err)
			}
			if _, err := Run(context.Background(), deps, Request{Project: "proj-a", Query: "q", Sources: []string{SourceEngramFTS}}); err != nil {
				t.Errorf("a call that does not name the vault failed for lack of one: %v", err)
			}
		})
	}
}

// recordedVault is a retriever that can be a nil pointer.
type recordedVault struct{}

func (*recordedVault) RetrieveVault(context.Context, string, string, int) ([]VaultCandidate, error) {
	return nil, nil
}
