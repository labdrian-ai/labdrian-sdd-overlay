package vecindex

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/durable"
)

// The three constants below are this module's default embedding contract
// (openspec/decisions/union-retrieval.md's own measured values): the
// production Ollama model, its output dimension, and the character count
// embedded per row. A caller may override any of them (e.g. a future
// model change), which is exactly what Fingerprint's contract-inside-the-
// hash design exists to make safe.
const (
	DefaultModel      = "nomic-embed-text"
	DefaultDimension  = 768
	DefaultInputLimit = 2000
)

// manifestFileName and blobFileName are the two files one index directory
// holds. Both are this module's own state (R-066/R-067): never a vault
// path, never a write against Engram's database.
const (
	manifestFileName = "manifest.json"
	blobFileName     = "vectors.blob"
)

// manifestCreatePerm is the mode a freshly written manifest/blob get:
// per-user state, owner-only, matching vaultreg's vaults.json convention.
const manifestCreatePerm = 0o600

// ErrNoIndex is returned by Load when dir holds no manifest yet -- distinct
// from a corruption error, since Build (build.go) must build fresh on "no
// index" and refuse outright on "corrupted index".
var ErrNoIndex = errors.New("vecindex: no index at this path")

// ErrCorrupted is returned by Load when the manifest's recorded revision
// does not match a digest of its own entries (R-066's "A corrupted
// manifest is reported, not silently trusted"), or when the vector blob's
// size does not match what the manifest's entry count and dimension
// require.
var ErrCorrupted = errors.New("vecindex: index is corrupted")

// ManifestEntry is one indexed row: which observation it embeds, and the
// fingerprint (see Fingerprint) that was true when it was embedded. Its
// position in Manifest.Entries is also its vector's position in the fixed-
// stride blob -- entry i's vector occupies the i-th Dimension-float32
// stride.
type ManifestEntry struct {
	EngramID    int64  `json:"engram_id"`
	Fingerprint string `json:"fingerprint"`
}

// Manifest is the self-digesting JSON record R-066 requires: the embedding
// contract (Model, Dimension, InputLimit), when the index was last built,
// every indexed row's fingerprint, and Revision -- a digest of everything
// else in the manifest, recomputed and checked on every Load so a manifest
// that was hand-edited or partially written is reported as corrupted
// rather than trusted.
type Manifest struct {
	Model      string          `json:"model"`
	Dimension  int             `json:"dimension"`
	InputLimit int             `json:"input_limit"`
	BuiltAt    string          `json:"built_at"`
	Entries    []ManifestEntry `json:"entries"`
	Revision   string          `json:"revision"`
}

// Index is one project's loaded embedding index: the manifest plus its
// vectors, Vectors[i] corresponding to Manifest.Entries[i].
type Index struct {
	Manifest Manifest
	Vectors  [][]float32
}

// ProjectDir turns a project name into a filesystem-safe directory
// component: '/' becomes '_' (a project name is never expected to contain
// one, but Engram's own project column is an arbitrary string, and this
// index's directory must never let one escape <state-dir>/index/), and a
// literal ".." component is refused the same way, rather than silently
// walking up the tree.
func ProjectDir(project string) string {
	replaced := strings.ReplaceAll(project, "/", "_")
	replaced = strings.ReplaceAll(replaced, "..", "_")
	if replaced == "" {
		replaced = "_"
	}
	return replaced
}

// Dir returns the directory one project's embedding index lives under:
// <state-dir>/index/<project-dir>/ (R-066).
func Dir(stateDir, project string) string {
	return filepath.Join(stateDir, "index", ProjectDir(project))
}

// computeRevision digests everything in a Manifest except Revision itself,
// so Save can set it and Load can check it.
func computeRevision(m Manifest) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%d\x00%d\x00%s\x00", m.Model, m.Dimension, m.InputLimit, m.BuiltAt)
	for _, e := range m.Entries {
		fmt.Fprintf(h, "%d\x00%s\x00", e.EngramID, e.Fingerprint)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Save writes idx to dir as a manifest.json (self-digested) and a
// vectors.blob (fixed-stride float32, little-endian), both through
// durable.WriteFile so a reader never observes a partially written index
// (mirroring vaultreg's own writeJSONAtomic convention).
//
// It refuses a manifest whose entry count does not match len(Vectors), or
// any vector whose length does not match Manifest.Dimension: writing an
// inconsistent index would just move R-066's corruption detection from
// Load's problem to some later reader's, and Save is this package's only
// writer -- catching it here is strictly cheaper.
func (idx *Index) Save(dir string) error {
	if len(idx.Vectors) != len(idx.Manifest.Entries) {
		return fmt.Errorf("vecindex: save: %d entries but %d vectors", len(idx.Manifest.Entries), len(idx.Vectors))
	}
	dim := idx.Manifest.Dimension
	blob := make([]byte, 0, len(idx.Vectors)*dim*4)
	for i, v := range idx.Vectors {
		if len(v) != dim {
			return fmt.Errorf("vecindex: save: entry %d has vector length %d, want dimension %d", i, len(v), dim)
		}
		for _, f := range v {
			blob = binary.LittleEndian.AppendUint32(blob, math.Float32bits(f))
		}
	}

	idx.Manifest.Revision = computeRevision(idx.Manifest)
	manifestData, err := json.MarshalIndent(idx.Manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("vecindex: marshal manifest: %w", err)
	}
	manifestData = append(manifestData, '\n')

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("vecindex: create %s: %w", dir, err)
	}
	if err := durable.WriteFile(filepath.Join(dir, blobFileName), blob, manifestCreatePerm); err != nil {
		return fmt.Errorf("vecindex: write vector blob: %w", err)
	}
	// The manifest is written last: on a crash between the two writes, a
	// reader sees either the OLD manifest with the OLD blob (both durably
	// atomic on their own, per durable.WriteFile) or, at worst, the old
	// manifest with a new blob whose entry count may disagree with it --
	// which Load's own length check below reports as corruption rather
	// than silently reading past the blob's end.
	if err := durable.WriteFile(filepath.Join(dir, manifestFileName), manifestData, manifestCreatePerm); err != nil {
		return fmt.Errorf("vecindex: write manifest: %w", err)
	}
	return nil
}

// Load reads dir's manifest and vector blob. It returns ErrNoIndex when no
// manifest exists yet (nothing has been built), and wraps ErrCorrupted when
// the manifest's recorded revision does not match a digest of its own
// entries, or when the blob's size does not match what the manifest
// requires -- R-066's "loading fails with a corruption error rather than
// serving stale or invalid vectors."
func Load(dir string) (*Index, error) {
	manifestPath := filepath.Join(dir, manifestFileName)
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoIndex
		}
		return nil, fmt.Errorf("vecindex: read %s: %w", manifestPath, err)
	}

	var m Manifest
	if err := json.Unmarshal(manifestData, &m); err != nil {
		return nil, fmt.Errorf("%w: %s: parse manifest: %v", ErrCorrupted, manifestPath, err)
	}

	recorded := m.Revision
	if want := computeRevision(m); want != recorded {
		return nil, fmt.Errorf("%w: %s: recorded revision %q does not match a digest of its own entries (%q)", ErrCorrupted, manifestPath, recorded, want)
	}

	blobPath := filepath.Join(dir, blobFileName)
	blobData, err := os.ReadFile(blobPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s: manifest exists but the vector blob does not", ErrCorrupted, blobPath)
		}
		return nil, fmt.Errorf("vecindex: read %s: %w", blobPath, err)
	}

	stride := m.Dimension * 4
	wantLen := stride * len(m.Entries)
	if len(blobData) != wantLen {
		return nil, fmt.Errorf("%w: %s: blob is %d bytes, want %d for %d entries at dimension %d", ErrCorrupted, blobPath, len(blobData), wantLen, len(m.Entries), m.Dimension)
	}

	vectors := make([][]float32, len(m.Entries))
	for i := range m.Entries {
		vectors[i] = make([]float32, m.Dimension)
		base := i * stride
		for j := 0; j < m.Dimension; j++ {
			bits := binary.LittleEndian.Uint32(blobData[base+j*4 : base+j*4+4])
			vectors[i][j] = math.Float32frombits(bits)
		}
	}

	return &Index{Manifest: m, Vectors: vectors}, nil
}
