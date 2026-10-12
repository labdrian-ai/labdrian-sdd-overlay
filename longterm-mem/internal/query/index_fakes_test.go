package query

import (
	"context"
	"fmt"
)

// errNoFakeIndex is what fakeIndexes answers for a project it holds no index of: an index nobody built.
var errNoFakeIndex = fmt.Errorf("%w: the fake holds none", ErrNoEmbeddingIndex)

// fakeIndexes is an EmbeddingIndexRepository over indexes held in memory, by project, with no disk and no
// adapter: the use case depends on the port and on the model it speaks, not on how an index is stored.
type fakeIndexes struct {
	indexes map[string]*EmbeddingIndex
	// loadErr, when set, is every load's answer.
	loadErr error
	// loads lists the projects asked for, in order.
	loads []string
	// fingerprints lists the contracts the fingerprint was asked under, in order.
	fingerprints []fingerprintCall
}

type fingerprintCall struct {
	model            string
	dimension, limit int
	title, content   string
}

func (f *fakeIndexes) LoadEmbeddingIndex(project string) (*EmbeddingIndex, error) {
	f.loads = append(f.loads, project)
	if f.loadErr != nil {
		return nil, f.loadErr
	}
	idx, ok := f.indexes[project]
	if !ok {
		return nil, errNoFakeIndex
	}
	return idx, nil
}

func (f *fakeIndexes) Fingerprint(model string, dimension, inputLimit int, title, content string) string {
	f.fingerprints = append(f.fingerprints, fingerprintCall{model, dimension, inputLimit, title, content})
	return testFingerprint(model, dimension, inputLimit, title, content)
}

// testFingerprint is a fingerprint over everything the real one covers (the contract and the text), readable
// in a failure message.
func testFingerprint(model string, dimension, inputLimit int, title, content string) string {
	return fmt.Sprintf("%s/%d/%d/%q/%q", model, dimension, inputLimit, title, content)
}

// hold stores idx as project's index.
func (f *fakeIndexes) hold(project string, idx *EmbeddingIndex) {
	if f.indexes == nil {
		f.indexes = map[string]*EmbeddingIndex{}
	}
	f.indexes[project] = idx
}

// builderCall is what an EmbeddingIndexBuilder was asked.
type builderCall struct {
	project, model   string
	dimension, limit int
}

// fakeBuilder is an EmbeddingIndexBuilder that counts and records its calls, runs apply for each successful one
// (to change the index the way a build does), and answers err.
type fakeBuilder struct {
	calls []builderCall
	apply func()
	err   error
}

func (b *fakeBuilder) BuildEmbeddingIndex(_ context.Context, project, model string, dimension, inputLimit int) error {
	b.calls = append(b.calls, builderCall{project, model, dimension, inputLimit})
	if b.err == nil && b.apply != nil {
		b.apply()
	}
	return b.err
}
