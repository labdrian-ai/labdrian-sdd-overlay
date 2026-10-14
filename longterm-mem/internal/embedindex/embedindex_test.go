package embedindex

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// These cases run the use case over tables that stand for everything it reads through a port: the memory, the
// factory of the embedder and the builder of the index. Nothing here opens a database, reaches a backend or writes
// an index, so each case says what the USE CASE does: what it asks, in what order, and what it hands on.

type memoryOf struct {
	observations []memory.Observation
	err          error
	listed       []string
	trace        *[]string
}

func (m *memoryOf) ListObservations(project string) ([]memory.Observation, error) {
	m.listed = append(m.listed, project)
	if m.trace != nil {
		*m.trace = append(*m.trace, "list")
	}
	return m.observations, m.err
}

type builderOf struct {
	result Result
	err    error
	trace  *[]string

	project  string
	contract Contract
	rows     []Row
	embedder Embedder
	calls    int
}

func (b *builderOf) BuildRows(_ context.Context, project string, contract Contract, rows []Row, embedder Embedder) (Result, error) {
	b.calls++
	b.project, b.contract, b.rows, b.embedder = project, contract, rows, embedder
	if b.trace != nil {
		*b.trace = append(*b.trace, "build")
	}
	return b.result, b.err
}

type embedderOf struct{ id string }

func (embedderOf) Embed(context.Context, string) ([]float32, error) { return nil, nil }

var contract = Contract{Model: "nomic-embed-text", Dimension: 768, InputLimit: 2000}

// A row carries what the index is built from, and nothing else of an observation: its id, its title and its
// content, in the order the memory listed them.
func TestRowsOf_CarriesTheIdTheTitleAndTheContentInOrder(t *testing.T) {
	got := RowsOf([]memory.Observation{
		{ID: 3, Title: "Third", Content: "c3", Type: "decision", Project: "p", Pinned: true, RevisionCount: 4},
		{ID: 1, Title: "First", Content: "c1"},
	})

	want := []Row{{EngramID: 3, Title: "Third", Content: "c3"}, {EngramID: 1, Title: "First", Content: "c1"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RowsOf = %+v, want %+v", got, want)
	}
}

// No observation is no row, and the answer is a list, not nothing: what builds from it never sees a nil.
func TestRowsOf_NoObservationsIsAnEmptyListNotNil(t *testing.T) {
	got := RowsOf(nil)

	if got == nil || len(got) != 0 {
		t.Errorf("RowsOf(nil) = %#v, want an empty, non-nil list", got)
	}
}

// The rows of a project are its live observations as the memory lists them (the memory leaves out what was
// deleted), asked for by project, and a failure to list is the memory's own, untouched.
func TestLiveRows_AreTheListedObservationsOfTheProject(t *testing.T) {
	mem := &memoryOf{observations: []memory.Observation{{ID: 9, Title: "T", Content: "C"}}}

	got, err := LiveRows(mem, "proj")

	if err != nil || !reflect.DeepEqual(got, []Row{{EngramID: 9, Title: "T", Content: "C"}}) {
		t.Errorf("LiveRows = %+v, %v; want the one row", got, err)
	}
	if !reflect.DeepEqual(mem.listed, []string{"proj"}) {
		t.Errorf("the memory was asked for %v, want the project", mem.listed)
	}

	boom := errors.New("memory offline")
	if _, err := LiveRows(&memoryOf{err: boom}, "proj"); err != boom {
		t.Errorf("LiveRows error = %v, want the memory's own", err)
	}
}

// The use case lists the project's live observations, makes the embedder, and hands the builder the project,
// the contract, the rows and that embedder, in that order; what the builder says is what it answers.
func TestBuild_ListsThenMakesTheEmbedderThenBuilds(t *testing.T) {
	var trace []string
	mem := &memoryOf{observations: []memory.Observation{{ID: 1, Title: "A", Content: "a"}}, trace: &trace}
	embedder := embedderOf{id: "made"}
	builder := &builderOf{result: Result{Embedded: 1, Reused: 2, Removed: 3}, trace: &trace}
	u := UseCase{
		Observations: mem,
		NewEmbedder: func() (Embedder, error) {
			trace = append(trace, "embedder")
			return embedder, nil
		},
		Builder: builder,
	}

	got, err := u.Build(t.Context(), "proj", contract)

	if err != nil || got != (Result{Embedded: 1, Reused: 2, Removed: 3}) {
		t.Errorf("Build = %+v, %v; want the builder's result", got, err)
	}
	if want := []string{"list", "embedder", "build"}; !reflect.DeepEqual(trace, want) {
		t.Errorf("order = %v, want %v", trace, want)
	}
	if builder.project != "proj" || builder.contract != contract || builder.embedder != Embedder(embedder) {
		t.Errorf("the builder was given %q, %+v and %v; want the project, the contract and the embedder that was made", builder.project, builder.contract, builder.embedder)
	}
	if want := []Row{{EngramID: 1, Title: "A", Content: "a"}}; !reflect.DeepEqual(builder.rows, want) {
		t.Errorf("the builder was given %+v, want the rows of the listing %+v", builder.rows, want)
	}
}

// A run that stopped says where: the listing, the making of the embedder, or the build. The words are the
// failure's own, whichever stage it was, and nothing after the stage that failed runs.
func TestBuild_AFailureNamesItsStageAndKeepsItsWords(t *testing.T) {
	boom := errors.New("it broke")
	tests := []struct {
		name          string
		list, embed   error
		build         error
		want          Stage
		wantEmbedders int
		wantBuilds    int
	}{
		{"the listing", boom, nil, nil, StageObservations, 0, 0},
		{"the embedder", nil, boom, nil, StageEmbedder, 1, 0},
		{"the build", nil, nil, boom, StageBuild, 1, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			embedders := 0
			builder := &builderOf{err: tc.build}
			u := UseCase{
				Observations: &memoryOf{err: tc.list},
				NewEmbedder:  func() (Embedder, error) { embedders++; return embedderOf{}, tc.embed },
				Builder:      builder,
			}

			_, err := u.Build(t.Context(), "proj", contract)

			var failure *Error
			if !errors.As(err, &failure) || failure.Stage != tc.want {
				t.Fatalf("Build error = %v, want a failure of stage %v", err, tc.want)
			}
			if !errors.Is(err, boom) || err.Error() != boom.Error() {
				t.Errorf("Build error reads %q, want the failure's own words %q and to wrap it", err, boom)
			}
			if embedders != tc.wantEmbedders || builder.calls != tc.wantBuilds {
				t.Errorf("%d embedders made and %d builds, want %d and %d: nothing after the failed stage runs", embedders, builder.calls, tc.wantEmbedders, tc.wantBuilds)
			}
		})
	}
}
