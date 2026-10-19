package query

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// The lexical source hands the goal of the call to the memory's search as part of its filter, beside the types to
// leave out; a call without a goal hands an empty one, so the search is the one it was before goals.
func TestTheLexicalSourceSearchesUnderTheCallsGoal(t *testing.T) {
	for _, tc := range []struct{ name, goal string }{{"under a goal", "g-42"}, {"without a goal", ""}} {
		t.Run(tc.name, func(t *testing.T) {
			mem := &fakeMemory{search: memory.SearchResult{Rows: ftsRows(), MatchMode: memory.MatchAll}}
			_, err := Run(context.Background(), Deps{Memory: mem, ResolveLink: NoLinkResolver}, Request{
				Project: "p", Query: "alpha", Top: 3, ExcludeTypes: []string{"session_summary"}, Goal: tc.goal, Sources: []string{SourceEngramFTS},
			})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			want := memory.SearchFilter{ExcludeTypes: []string{"session_summary"}, Goal: tc.goal}
			if !reflect.DeepEqual(mem.searchedFilter, want) || mem.searchedLimit != 3 {
				t.Fatalf("the search was asked with filter %+v and limit %d, want %+v and 3", mem.searchedFilter, mem.searchedLimit, want)
			}
		})
	}
}

// goalEmbedFixture is a memory and an index of four observations, in falling cosine order to the query vector
// {1, 0}: another goal's (best), this goal's, another goal's, and one of no goal.
func goalEmbedFixture() (*fakeMemory, *fakeIndexes) {
	observations := []memory.Observation{
		{ID: 1, Title: "other best", Content: "a", Project: "p", TopicKey: "goal:g-7/a"},
		{ID: 2, Title: "mine", Content: "b", Project: "p", TopicKey: "goal:g-42/a"},
		{ID: 3, Title: "other again", Content: "c", Project: "p", TopicKey: "goal:g-7/b"},
		{ID: 4, Title: "no goal", Content: "d", Project: "p", TopicKey: "architecture/x"},
	}
	vectors := [][]float32{{1, 0}, {0.9, 0.1}, {0.8, 0.2}, {0.7, 0.3}}
	idx := &EmbeddingIndex{Model: "m", Dimension: 2, InputLimit: 100, BuiltAt: "2026-10-19T00:00:00Z"}
	liveByID := map[int64]memory.Observation{}
	for i, o := range observations {
		idx.Entries = append(idx.Entries, IndexedObservation{EngramID: o.ID, Fingerprint: testFingerprint("m", 2, 100, o.Title, o.Content)})
		idx.Vectors = append(idx.Vectors, vectors[i])
		liveByID[o.ID] = o
	}
	indexes := &fakeIndexes{}
	indexes.hold("p", idx)
	return &fakeMemory{live: len(observations), liveByID: liveByID}, indexes
}

func engramIDs(result Result) []int64 {
	var ids []int64
	for _, r := range result.Results {
		ids = append(ids, r.EngramID)
	}
	return ids
}

// The embedding source leaves out another goal's observations before it cuts at top, so the rows it returns under a
// goal are the first top rows of its cosine order that are this goal's or no goal's; without a goal the order and
// the cut are what they were.
func TestTheEmbeddingSourceKeepsTheGoalBeforeItsCut(t *testing.T) {
	for _, tc := range []struct {
		name string
		goal string
		want []int64
	}{
		{"under a goal", "g-42", []int64{2, 4}},
		{"under another goal", "g-7", []int64{1, 3}},
		{"without a goal", "", []int64{1, 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mem, indexes := goalEmbedFixture()
			deps := Deps{Memory: mem, Index: indexes, Embed: fakeEmbed([]float32{1, 0}, nil), ResolveLink: NoLinkResolver}
			result, err := Run(context.Background(), deps, Request{Project: "p", Query: "q", Top: 2, Goal: tc.goal, Sources: []string{SourceEngramEmbed}})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if got := engramIDs(result); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("embedding rows = %v, want %v", got, tc.want)
			}
		})
	}
}

// recordingTags is a VaultPageTags over tags held by page address, recording which pages it was asked about; a page
// it holds no tags for cannot be read.
type recordingTags struct {
	tags  map[string][]string
	asked []string
}

var errTagsUnreadable = errors.New("the page's frontmatter cannot be read")

func (r *recordingTags) PageTags(_ context.Context, c VaultCandidate) ([]string, error) {
	r.asked = append(r.asked, c.PageAddress)
	tags, ok := r.tags[c.PageAddress]
	if !ok {
		return nil, errTagsUnreadable
	}
	return tags, nil
}

func vaultCandidates(addresses ...string) []VaultCandidate {
	candidates := make([]VaultCandidate, len(addresses))
	for i, a := range addresses {
		candidates[i] = VaultCandidate{PageAddress: a, AbsolutePath: "/vault/" + a + ".md", Snippet: a}
	}
	return candidates
}

func pageAddresses(result Result) []string {
	var addresses []string
	for _, r := range result.Results {
		addresses = append(addresses, r.PageAddress)
	}
	return addresses
}

// Under a goal the vault is asked for three times top candidates; the pages whose tags name another goal are left
// out, and the first top of the rest are kept in the vault's order.
func TestTheVaultSourceAsksForMoreUnderAGoalAndKeepsTheFirstTop(t *testing.T) {
	var calls []recordedVaultCall
	tags := &recordingTags{tags: map[string][]string{
		"c-000001": {"memory", "decision", "goal:g-7"},
		"c-000002": {"memory", "decision", "goal:g-42"},
		"c-000003": {"memory", "goal:g-7"},
		"c-000004": {"memory", "decision"},
		"c-000005": {"goal:g-42"},
		"c-000006": {"memory"},
	}}
	deps := Deps{
		Memory:      &fakeMemory{},
		Vault:       recordingVault(&calls, vaultCandidates("c-000001", "c-000002", "c-000003", "c-000004", "c-000005", "c-000006"), nil),
		VaultTags:   tags,
		ResolveLink: NoLinkResolver,
	}
	result, err := Run(context.Background(), deps, Request{Project: "p", Query: "q", Top: 2, Goal: "g-42", Sources: []string{SourceVault}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if want := []recordedVaultCall{{"p", "q", 6}}; !reflect.DeepEqual(calls, want) {
		t.Errorf("the vault was asked %+v, want %+v (three times top)", calls, want)
	}
	if got, want := pageAddresses(result), []string{"c-000002", "c-000004"}; !reflect.DeepEqual(got, want) {
		t.Errorf("vault rows = %v, want %v", got, want)
	}
	if result.VaultStatus != VaultStatusOK || len(result.Diagnostics) != 0 {
		t.Errorf("VaultStatus = %q, Diagnostics = %+v, want ok and none", result.VaultStatus, result.Diagnostics)
	}
}

// The enlarged retrieval is bounded: three times a large top asks for VaultGoalCandidateBound candidates, not more.
func TestTheVaultSourcesEnlargedRetrievalIsBounded(t *testing.T) {
	var calls []recordedVaultCall
	deps := Deps{Memory: &fakeMemory{}, Vault: recordingVault(&calls, nil, nil), VaultTags: &recordingTags{}, ResolveLink: NoLinkResolver}
	if _, err := Run(context.Background(), deps, Request{Project: "p", Query: "q", Top: 50, Goal: "g-42", Sources: []string{SourceVault}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if want := []recordedVaultCall{{"p", "q", VaultGoalCandidateBound}}; !reflect.DeepEqual(calls, want) {
		t.Errorf("the vault was asked %+v, want %+v", calls, want)
	}
	if VaultGoalCandidateBound >= 3*50 || VaultGoalCandidateBound < 3*DefaultTopN {
		t.Errorf("VaultGoalCandidateBound = %d: it must bound a large top and leave the default top its threefold", VaultGoalCandidateBound)
	}
}

// Without a goal the vault is asked exactly as before goals (top candidates), no page's tags are read, and every
// candidate is kept whatever goal its tags name.
func TestTheVaultSourceWithoutAGoalIsAskedAsBefore(t *testing.T) {
	var calls []recordedVaultCall
	tags := &recordingTags{tags: map[string][]string{"c-000001": {"goal:g-7"}}}
	deps := Deps{Memory: &fakeMemory{}, Vault: recordingVault(&calls, vaultCandidates("c-000001", "c-000002"), nil), VaultTags: tags, ResolveLink: NoLinkResolver}
	result, err := Run(context.Background(), deps, Request{Project: "p", Query: "q", Top: 2, Sources: []string{SourceVault}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if want := []recordedVaultCall{{"p", "q", 2}}; !reflect.DeepEqual(calls, want) {
		t.Errorf("the vault was asked %+v, want %+v", calls, want)
	}
	if len(tags.asked) != 0 {
		t.Errorf("tags were read for %v without a goal", tags.asked)
	}
	if got, want := pageAddresses(result), []string{"c-000001", "c-000002"}; !reflect.DeepEqual(got, want) {
		t.Errorf("vault rows = %v, want %v", got, want)
	}
}

// Under a goal a page whose tags cannot be read is left out -- it may be another goal's -- and the answer says
// which pages were left out and why; the vault's status stays ok. With no tag reader wired, every page is such a
// page.
func TestUnderAGoalAVaultPageWhoseTagsCannotBeReadIsLeftOutAndNamed(t *testing.T) {
	for _, tc := range []struct {
		name string
		tags VaultPageTags
		want []string
		left []string
	}{
		{"an unreadable page", &recordingTags{tags: map[string][]string{"c-000002": {"memory"}}}, []string{"c-000002"}, []string{"c-000001"}},
		{"no tag reader", nil, nil, []string{"c-000001", "c-000002"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps := Deps{Memory: &fakeMemory{}, Vault: fakeVault(vaultCandidates("c-000001", "c-000002"), nil), VaultTags: tc.tags, ResolveLink: NoLinkResolver}
			result, err := Run(context.Background(), deps, Request{Project: "p", Query: "q", Top: 2, Goal: "g-42", Sources: []string{SourceVault}})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if got := pageAddresses(result); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("vault rows = %v, want %v", got, tc.want)
			}
			if result.VaultStatus != VaultStatusOK {
				t.Errorf("VaultStatus = %q, want ok", result.VaultStatus)
			}
			var found bool
			for _, d := range result.Diagnostics {
				if d.Code != DiagnosticVaultPageTagsUnreadable {
					continue
				}
				found = true
				for _, address := range tc.left {
					if !strings.Contains(d.Detail, address) {
						t.Errorf("detail %q does not name the page left out, %s", d.Detail, address)
					}
				}
			}
			if !found {
				t.Errorf("Diagnostics = %+v, want %q", result.Diagnostics, DiagnosticVaultPageTagsUnreadable)
			}
		})
	}
}
