package ops

import "testing"

// nothingBuilt is what fakeEmbeddingIndex answers for a project it holds nothing of: an error that says what it
// is, an index nobody built, with words of its own (the diagnostics never read them to tell it).
type nothingBuilt struct{ words string }

func (e nothingBuilt) Error() string   { return e.words }
func (nothingBuilt) NoEmbeddingIndex() {}

// modelNotPulled and backendDown are errors of an embedding backend as the diagnostics' contract describes
// them, with no dependency on the package that makes the real ones.
type modelNotPulled struct{ model string }

func (e *modelNotPulled) Error() string      { return "the model " + e.model + " is not pulled" }
func (e *modelNotPulled) EmbedModelMissing() {}

type backendDown struct{ cause error }

func (e *backendDown) Error() string { return "the backend is down: " + e.cause.Error() }
func (e *backendDown) Unwrap() error { return e.cause }

// fakeEmbeddingIndex is an EmbeddingIndexReader over one index held in memory, with no disk and no adapter: the
// diagnostics depend on the port and on the words of its answers, not on how an index is stored.
type fakeEmbeddingIndex struct {
	// location is the directory the index is said to be kept in; each project has its own under it.
	location string
	builtAt  string
	ids      []int64
	// err, when set, is the answer of every read of the index.
	err error
	// asked lists the projects the reads were for, in order.
	asked []string
}

func (f *fakeEmbeddingIndex) EmbeddingIndexLocation(project string) string {
	return f.location + "/" + project
}

func (f *fakeEmbeddingIndex) EmbeddingIndexBuiltAt(project string) (string, error) {
	f.asked = append(f.asked, project)
	if f.err != nil {
		return "", f.err
	}
	return f.builtAt, nil
}

func (f *fakeEmbeddingIndex) IndexedEngramIDs(project string) ([]int64, error) {
	f.asked = append(f.asked, project)
	if f.err != nil {
		return nil, f.err
	}
	return append([]int64(nil), f.ids...), nil
}

// fakeIndexLocation is where the fakes say their index is kept.
const fakeIndexLocation = "/state/index"

// newHealthyEmbeddingDeps builds an embedding index that exactly matches liveIDs -- present and fresh -- so the
// three embedding checks (R-064) default to healthy the same way newHealthyDeps' vault fixture defaults its own
// four checks to healthy. Each subtest that exercises one embedding check breaks exactly that one piece,
// mirroring the vault fixtures' own one-broken-piece convention.
func newHealthyEmbeddingDeps(t *testing.T) (index *fakeEmbeddingIndex, liveIDs []int64) {
	t.Helper()
	liveIDs = []int64{1, 2, 3}
	return &fakeEmbeddingIndex{location: fakeIndexLocation, builtAt: "2026-01-01T00:00:00Z", ids: liveIDs}, liveIDs
}

// noEmbeddingIndexBuilt is an index reader for a project nothing was built for.
func noEmbeddingIndexBuilt() *fakeEmbeddingIndex {
	return &fakeEmbeddingIndex{location: fakeIndexLocation, err: nothingBuilt{"fake: nothing was built"}}
}
