package query

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// TestDefaultSources_UnionWhenIndexExists is this issue's core behavior: an
// omitted Sources must widen to engram-fts + engram-embed the moment a
// project's embedding index actually exists, rather than requiring a
// caller to name engram-embed by hand forever.
//
// Coverage is populated only on the wantEmbed branch of Run (query.go), so
// a non-nil Coverage entry for engram-embed is proof the embedding arm
// actually ran -- not just that the default sources slice happens to
// mention it.
func TestDefaultSources_UnionWhenIndexExists(t *testing.T) {
	store, _, _, index := newEmbedArmFixture(t, []embedArmFixture{
		{title: "indexed row", content: "zephyr content", project: "proj-default", vec: []float32{1, 0, 0}},
	})
	deps := Deps{
		Memory:      store,
		Vault:       fakeVault(nil, nil),
		ResolveLink: NoLinkResolver,
		Index:       index,
		Embed:       fakeEmbed([]float32{1, 0, 0}, nil),
	}

	got, err := Run(context.Background(), deps, Request{Project: "proj-default", Query: "zephyr"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got.Coverage) != 1 || got.Coverage[0].Source != SourceEngramEmbed {
		t.Fatalf("Coverage = %+v, want one engram-embed entry proving the embedding arm ran by default when an index exists", got.Coverage)
	}
}

// TestDefaultSources_FTSOnlyWhenNoIndex is the "otherwise" half: a project
// with no embedding index yet must not silently ask the embedding arm to
// run every query, since it has nothing to search and nothing to learn
// from doing so on every call.
func TestDefaultSources_FTSOnlyWhenNoIndex(t *testing.T) {
	store := newFixtureEngramStore(t, []fixtureObservation{
		{title: "engram row", content: "zephyr keyword", project: "proj-no-index"},
	})
	deps := Deps{
		Memory:      store,
		Vault:       fakeVault(nil, nil),
		ResolveLink: NoLinkResolver,
		Index:       &fakeIndexes{},
	}

	got, err := Run(context.Background(), deps, Request{Project: "proj-no-index", Query: "zephyr"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got.Coverage) != 0 {
		t.Fatalf("Coverage = %+v, want none: no index exists, so the embedding arm must not be defaulted on", got.Coverage)
	}
	if len(got.Results) != 1 || !hasSource(got.Results[0], SourceEngramFTS) {
		t.Fatalf("Results = %+v, want the one engram-fts row", got.Results)
	}
}

// The project given to defaultSources is the one the diagnostic of an unreadable index names: it is not a
// parameter that cannot matter (a reading of an index says nothing of which project it was read for).
func TestDefaultSourcesNameTheProjectOfAnUnreadableIndex(t *testing.T) {
	sources, diags := defaultSources(indexReading{err: errors.New("boom")}, "my-proj")

	if len(sources) != 1 || sources[0] != SourceEngramFTS {
		t.Errorf("sources = %v, want engram-fts alone", sources)
	}
	if len(diags) != 1 || diags[0].Code != DiagnosticEmbeddingIndexUnreadable || !strings.Contains(diags[0].Detail, `project "my-proj"`) {
		t.Errorf("diagnostics = %+v, want one unreadable-index diagnostic naming project %q", diags, "my-proj")
	}
}

// scriptedIndexes is an EmbeddingIndexRepository whose loads answer, in order, what the script says, and the
// last answer once the script is over: a repository whose index changes between two reads of it.
type scriptedIndexes struct {
	fakeIndexes
	script []indexAnswer
}

type indexAnswer struct {
	index *EmbeddingIndex
	err   error
}

func (s *scriptedIndexes) LoadEmbeddingIndex(project string) (*EmbeddingIndex, error) {
	s.loads = append(s.loads, project)
	answer := s.script[len(s.script)-1]
	if len(s.loads) <= len(s.script) {
		answer = s.script[len(s.loads)-1]
	}
	return answer.index, answer.err
}

// The sources a call defaults to are chosen on one reading of the project's index, and the embedding arm, when
// the choice includes it, searches that reading: the index is read once for the call, so the choice and the
// answer cannot describe two indexes, and an index that changes (or fails) between two reads has no second
// read to disagree with the first.
func TestTheDefaultSourcesAndTheEmbeddingArmAreOneReadingOfTheIndex(t *testing.T) {
	healthy := oneRowIndex(7, "alpha", "alpha body", []float32{1, 0, 0})
	mem := func() *filteringMemory {
		return memoryOf(memory.Observation{ID: 7, Title: "alpha", Content: "alpha body"})
	}

	t.Run("an index that reads", func(t *testing.T) {
		indexes := &scriptedIndexes{script: []indexAnswer{{index: healthy}, {err: errors.New("unreadable the second time")}}}
		deps := Deps{Memory: mem(), Index: indexes, Embed: fakeEmbed([]float32{1, 0, 0}, nil), ResolveLink: NoLinkResolver}

		result, err := Run(context.Background(), deps, Request{Project: "p", Query: "alpha"})
		if err != nil {
			t.Fatalf("Run: %v", err)
		}

		if len(indexes.loads) != 1 {
			t.Errorf("the index was read %d times for one call, want once", len(indexes.loads))
		}
		if len(result.Coverage) != 1 || result.Coverage[0].Indexed != 1 || hasDiagCode(result.Diagnostics, DiagnosticEmbeddingIndexUnreadable) {
			t.Errorf("coverage = %+v, diagnostics = %+v, want the arm to have searched the index the sources were chosen on, with nothing said of it being unreadable", result.Coverage, result.Diagnostics)
		}
	})

	t.Run("an index that does not read", func(t *testing.T) {
		indexes := &scriptedIndexes{script: []indexAnswer{{err: errors.New("unreadable the first time")}, {index: healthy}}}
		deps := Deps{Memory: mem(), Index: indexes, Embed: fakeEmbed([]float32{1, 0, 0}, nil), ResolveLink: NoLinkResolver}

		result, err := Run(context.Background(), deps, Request{Project: "p", Query: "alpha"})
		if err != nil {
			t.Fatalf("Run: %v", err)
		}

		if len(indexes.loads) != 1 {
			t.Errorf("the index was read %d times for one call, want once", len(indexes.loads))
		}
		if len(result.Coverage) != 0 {
			t.Errorf("Coverage = %+v, want none: the sources were chosen on a reading that failed, so the embedding source did not run", result.Coverage)
		}
		said := 0
		for _, d := range result.Diagnostics {
			if d.Code == DiagnosticEmbeddingIndexUnreadable {
				said++
			}
		}
		if said != 1 {
			t.Errorf("the unreadable index is said %d times in %+v, want once", said, result.Diagnostics)
		}
	})

	t.Run("sources named by the caller are read by the arm alone", func(t *testing.T) {
		indexes := &scriptedIndexes{script: []indexAnswer{{index: healthy}}}
		deps := Deps{Memory: mem(), Index: indexes, Embed: fakeEmbed([]float32{1, 0, 0}, nil), ResolveLink: NoLinkResolver}

		if _, err := Run(context.Background(), deps, Request{Project: "p", Query: "alpha", Sources: []string{SourceEngramFTS, SourceEngramEmbed}}); err != nil {
			t.Fatalf("Run: %v", err)
		}

		if len(indexes.loads) != 1 {
			t.Errorf("the index was read %d times for a call that names the embedding source, want once (by the arm)", len(indexes.loads))
		}
	})
}
