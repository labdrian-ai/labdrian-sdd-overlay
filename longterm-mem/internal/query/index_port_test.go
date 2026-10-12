package query

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// oneRowIndex is an index of one observation under the test contract, with the fingerprint its text has under it.
func oneRowIndex(id int64, title, content string, vector []float32) *EmbeddingIndex {
	return &EmbeddingIndex{
		Model: testEmbedModel, Dimension: testEmbedDimension, InputLimit: testEmbedInputLimit, BuiltAt: "2026-01-01T00:00:00Z",
		Entries: []IndexedObservation{{EngramID: id, Fingerprint: testFingerprint(testEmbedModel, testEmbedDimension, testEmbedInputLimit, title, content)}},
		Vectors: [][]float32{vector},
	}
}

// filteringMemory is a memory whose coverage snapshot answers, like the real one, about the ids it is asked:
// the count of the project's live observations, and which of those ids resolve to live ones.
type filteringMemory struct {
	fakeMemory
	observations []memory.Observation
}

func (m *filteringMemory) CoverageSnapshot(project string, indexedIDs []int64) (int, map[int64]memory.Observation, error) {
	m.snapshotProject, m.snapshotIDs = project, indexedIDs
	byID := map[int64]memory.Observation{}
	for _, id := range indexedIDs {
		for _, o := range m.observations {
			if o.ID == id {
				byID[id] = o
			}
		}
	}
	return len(m.observations), byID, nil
}

// memoryOf is a memory whose project has the given live observations and nothing else to say.
func memoryOf(observations ...memory.Observation) *filteringMemory {
	return &filteringMemory{observations: observations}
}

// The index is read through the port, for the project of the call, and only the index the port answers is
// searched: nothing else in the process says where an index is.
func TestTheEmbeddingArmReadsTheProjectsIndexThroughThePort(t *testing.T) {
	indexes := &fakeIndexes{}
	indexes.hold("p", oneRowIndex(7, "alpha", "alpha body", []float32{1, 0, 0}))
	indexes.hold("other", oneRowIndex(9, "beta", "beta body", []float32{0, 1, 0}))
	mem := memoryOf(memory.Observation{ID: 7, Title: "alpha", Content: "alpha body"})
	deps := Deps{Memory: mem, Index: indexes, Embed: fakeEmbed([]float32{1, 0, 0}, nil), ResolveLink: NoLinkResolver}

	result, err := Run(context.Background(), deps, Request{Project: "p", Query: "alpha", Sources: []string{SourceEngramEmbed}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !reflect.DeepEqual(indexes.loads, []string{"p"}) {
		t.Errorf("the port was asked for %v, want the index of the project of the call once", indexes.loads)
	}
	if len(result.Results) != 1 || result.Results[0].EngramID != 7 {
		t.Fatalf("Results = %+v, want the one row of the project's index", result.Results)
	}
	if !reflect.DeepEqual(mem.snapshotIDs, []int64{7}) {
		t.Errorf("the coverage snapshot was asked about %v, want the ids of the index the port answered", mem.snapshotIDs)
	}
}

// Whether a vector still describes its row is the index's to say, under the contract the index was built with: the
// fingerprint is asked of the port with the model, the dimension and the input limit read off the index, and the
// title and content of the row as the memory holds it now.
func TestTheFingerprintIsAskedUnderTheIndexsOwnContract(t *testing.T) {
	indexes := &fakeIndexes{}
	built := oneRowIndex(7, "alpha", "alpha body", []float32{1, 0, 0})
	built.Model, built.Dimension, built.InputLimit = "other-model", 3, 123
	built.Entries[0].Fingerprint = testFingerprint("other-model", 3, 123, "alpha", "alpha body")
	indexes.hold("p", built)
	deps := Deps{
		Memory: memoryOf(memory.Observation{ID: 7, Title: "alpha", Content: "alpha body"}), Index: indexes,
		Embed: fakeEmbed([]float32{1, 0, 0}, nil), ResolveLink: NoLinkResolver,
	}

	result, err := Run(context.Background(), deps, Request{Project: "p", Query: "alpha", Sources: []string{SourceEngramEmbed}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	want := []fingerprintCall{{"other-model", 3, 123, "alpha", "alpha body"}}
	if !reflect.DeepEqual(indexes.fingerprints, want) {
		t.Errorf("the fingerprint was asked as %+v, want %+v", indexes.fingerprints, want)
	}
	if len(result.Results) != 1 {
		t.Errorf("Results = %+v, want the row whose fingerprint matches", result.Results)
	}
}

// A row whose text changed since it was embedded is not served under the vector of the text it had.
func TestARowWhoseTextChangedSinceItWasEmbeddedIsNotServed(t *testing.T) {
	indexes := &fakeIndexes{}
	indexes.hold("p", oneRowIndex(7, "alpha", "old body", []float32{1, 0, 0}))
	deps := Deps{
		Memory: memoryOf(memory.Observation{ID: 7, Title: "alpha", Content: "new body"}), Index: indexes,
		Embed: fakeEmbed([]float32{1, 0, 0}, nil), ResolveLink: NoLinkResolver,
	}

	result, err := Run(context.Background(), deps, Request{Project: "p", Query: "alpha", Sources: []string{SourceEngramEmbed}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Results) != 0 {
		t.Fatalf("Results = %+v, want none: the vector describes the old text", result.Results)
	}
}

// With no index port wired, or one that holds a nil pointer, there is no index: the arm degrades like a project
// nothing was built for (zero rows, Coverage saying "never"), the default sources stay the lexical one, and
// nothing panics.
func TestWithoutAnIndexPortThereIsNoIndex(t *testing.T) {
	var typedNil *fakeIndexes
	for name, port := range map[string]EmbeddingIndexRepository{"no port": nil, "a nil pointer": typedNil} {
		t.Run(name, func(t *testing.T) {
			mem := &fakeMemory{search: memory.SearchResult{MatchMode: memory.MatchAll}, live: 2}
			deps := Deps{Memory: mem, Index: port, Embed: fakeEmbed([]float32{1, 0, 0}, nil), ResolveLink: NoLinkResolver}

			named, err := Run(context.Background(), deps, Request{Project: "p", Query: "alpha", Sources: []string{SourceEngramEmbed}})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			want := []Coverage{{Source: SourceEngramEmbed, Live: 2, Indexed: 0, Unindexed: 2, BuiltAt: embeddingIndexNeverBuilt}}
			if !reflect.DeepEqual(named.Coverage, want) {
				t.Errorf("Coverage = %+v, want %+v", named.Coverage, want)
			}

			defaulted, err := Run(context.Background(), deps, Request{Project: "p", Query: "alpha"})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if len(defaulted.Coverage) != 0 {
				t.Errorf("Coverage = %+v, want none: with no index the default is the lexical source alone", defaulted.Coverage)
			}
		})
	}
}

// Whatever the port says to refuse an index (none built, corrupted, unreadable), the arm answers the same: no
// rows, Coverage "never", no diagnostic of its own. The words of the error are not read.
func TestAnyRefusalOfTheIndexDegradesTheSameWay(t *testing.T) {
	for name, refusal := range map[string]error{
		"none built": errNoFakeIndex, "corrupted": errors.New("index is corrupted"), "unreadable": fmt.Errorf("read: %w", errors.New("permission denied")),
	} {
		t.Run(name, func(t *testing.T) {
			mem := &fakeMemory{search: memory.SearchResult{MatchMode: memory.MatchAll}, live: 3}
			deps := Deps{Memory: mem, Index: &fakeIndexes{loadErr: refusal}, Embed: fakeEmbed([]float32{1, 0, 0}, nil), ResolveLink: NoLinkResolver}

			result, err := Run(context.Background(), deps, Request{Project: "p", Query: "alpha", Sources: []string{SourceEngramEmbed}})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			want := []Coverage{{Source: SourceEngramEmbed, Live: 3, Unindexed: 3, BuiltAt: embeddingIndexNeverBuilt}}
			if !reflect.DeepEqual(result.Coverage, want) {
				t.Errorf("Coverage = %+v, want %+v", result.Coverage, want)
			}
			for _, d := range result.Diagnostics {
				if d.Code != DiagnosticEmbeddingIndexIncomplete {
					t.Errorf("diagnostic %+v: the only thing to say is that the index is incomplete", d)
				}
			}
		})
	}
}

// The default sources include the embedding one the moment the port answers an index, and not otherwise.
func TestTheDefaultSourcesFollowWhetherThePortAnswersAnIndex(t *testing.T) {
	built := &fakeIndexes{}
	built.hold("p", oneRowIndex(7, "alpha", "alpha body", []float32{1, 0, 0}))

	for _, tc := range []struct {
		name  string
		index EmbeddingIndexRepository
		want  []string
	}{
		{"an index is there", built, []string{SourceEngramFTS, SourceEngramEmbed}},
		{"none is", &fakeIndexes{}, []string{SourceEngramFTS}},
		{"the port refuses", &fakeIndexes{loadErr: errors.New("boom")}, []string{SourceEngramFTS}},
		{"no port", nil, []string{SourceEngramFTS}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := defaultSources(Deps{Index: tc.index}, "p"); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("defaultSources = %v, want %v", got, tc.want)
			}
		})
	}
}

// A small gap is closed through the builder port, asked for the project and the contract read off the index it
// is closing the gap in; a builder that is not wired, or holds a nil pointer, leaves the gap as it is.
func TestTheTopUpAsksTheBuilderForTheIndexsOwnContract(t *testing.T) {
	indexes := &fakeIndexes{}
	built := oneRowIndex(7, "alpha", "alpha body", []float32{1, 0, 0})
	built.Model, built.Dimension, built.InputLimit = "other-model", 3, 123
	indexes.hold("p", built)
	mem := memoryOf(memory.Observation{ID: 7, Title: "alpha", Content: "alpha body"}, memory.Observation{ID: 8, Title: "fresh", Content: "fresh body"})
	builder := &fakeBuilder{}

	arm := embeddingArm{Memory: mem, Index: indexes, Builder: builder, Embed: fakeEmbed([]float32{1, 0, 0}, nil)}
	if _, coverage, _ := arm.run(context.Background(), "p", "alpha", 5); coverage.Unindexed != 1 {
		t.Fatalf("Unindexed = %d, want the one row the index lacks", coverage.Unindexed)
	}

	if want := []builderCall{{"p", "other-model", 3, 123}}; !reflect.DeepEqual(builder.calls, want) {
		t.Errorf("the builder was asked %+v, want %+v", builder.calls, want)
	}
}

func TestWithoutABuilderTheGapIsLeftAsItIs(t *testing.T) {
	var typedNil *fakeBuilder
	for name, builder := range map[string]EmbeddingIndexBuilder{"no builder": nil, "a nil pointer": typedNil} {
		t.Run(name, func(t *testing.T) {
			indexes := &fakeIndexes{}
			indexes.hold("p", oneRowIndex(7, "alpha", "alpha body", []float32{1, 0, 0}))
			mem := memoryOf(memory.Observation{ID: 7, Title: "alpha", Content: "alpha body"}, memory.Observation{ID: 8, Title: "fresh", Content: "fresh body"})

			arm := embeddingArm{Memory: mem, Index: indexes, Builder: builder, Embed: fakeEmbed([]float32{1, 0, 0}, nil)}
			_, coverage, diags := arm.run(context.Background(), "p", "alpha", 5)

			if coverage.Unindexed != 1 || hasDiagCode(diags, DiagnosticEmbeddingToppedUp) || hasDiagCode(diags, DiagnosticEmbeddingTopUpFailed) {
				t.Errorf("coverage = %+v, diagnostics = %+v, want the gap of one left, with nothing said of a top-up", coverage, diags)
			}
		})
	}
}

// modelMissing and backendDown are errors of an embedding backend as the port's contract describes them, with no
// dependency on the package that makes the real ones.
type modelMissing struct{ model string }

func (e *modelMissing) Error() string      { return fmt.Sprintf("the model %q is not pulled", e.model) }
func (e *modelMissing) EmbedModelMissing() {}

type backendDown struct{ cause error }

func (e *backendDown) Error() string            { return "the backend is down: " + e.cause.Error() }
func (e *backendDown) Unwrap() error            { return e.cause }
func (e *backendDown) EmbedBackendUnreachable() {}

// The embedding backend's two failures are told apart by what the error says it is (an interface the package
// owns), however the error is wrapped, and not by the type of the package that makes them or by their words; an
// error that says neither is reported as an unreachable backend, with its own words.
func TestEmbeddingFailuresAreToldApartByWhatTheErrorSaysItIs(t *testing.T) {
	for _, tc := range []struct {
		name       string
		err        error
		wantCode   string
		wantDetail string
	}{
		{"a model that is not pulled", &modelMissing{"m"}, DiagnosticEmbeddingModelMissing, `the model "m" is not pulled`},
		{"the same, wrapped (the detail is the error that says what it is)", fmt.Errorf("embedding: %w", &modelMissing{"m"}), DiagnosticEmbeddingModelMissing, `the model "m" is not pulled`},
		{"a backend that is down", &backendDown{errors.New("refused")}, DiagnosticEmbeddingBackendUnreachable, "the backend is down: refused"},
		{"the same, wrapped", fmt.Errorf("embedding: %w", &backendDown{errors.New("refused")}), DiagnosticEmbeddingBackendUnreachable, "the backend is down: refused"},
		{"an error that says neither", errors.New("redirect refused"), DiagnosticEmbeddingBackendUnreachable, "redirect refused"},
		{"words that look like a missing model", errors.New("the model is not pulled"), DiagnosticEmbeddingBackendUnreachable, "the model is not pulled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := embeddingDegradationDiagnostic(tc.err)
			if got.Code != tc.wantCode || got.Detail != tc.wantDetail {
				t.Errorf("diagnostic = %+v, want {%s %q}", got, tc.wantCode, tc.wantDetail)
			}
		})
	}
}
