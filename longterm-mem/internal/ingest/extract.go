package ingest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// Request is one Extract call. Text and Path are mutually exclusive. Path is as the operator typed it: a relative
// one begins at the SourceReader's working directory.
type Request struct {
	Text string
	Path string
	// Origin is what the records record as their source. For a Path request its Kind is KindFile or KindDirectory,
	// and its Override and Title are the caller's; the path itself comes from Path.
	Origin         Origin
	Recursive      bool // directory scans are shallow unless set
	Now            func() time.Time
	IngestedBy     string
	MaxChunkBytes  int // 0 -> DefaultMaxChunkBytes
	MinChunkBytes  int // 0 -> DefaultMinChunkBytes
	MaxSourceBytes int // 0 -> DefaultMaxSourceBytes
}

// RecordRole says which part of a source a Record is.
type RecordRole string

const (
	RoleStandalone RecordRole = "standalone" // N == 1: manifest and content in one
	RoleManifest   RecordRole = "manifest"
	RoleChunk      RecordRole = "chunk"
)

// Record is exactly one mem_save call's arguments. A caller passes Content through unchanged; inventing or amending
// metadata here is the failure R-007 exists to prevent.
type Record struct {
	TopicKey string
	Title    string
	Type     string // always "discovery"
	Role     RecordRole
	Content  string // body -> "---" -> provenance block
	Header   Header
	Bytes    int
}

// Source is one extracted document: its origin, its id, the digest of its normalized text and its records.
type Source struct {
	Origin  Origin
	ID      string
	Digest  string   // Source-SHA256
	Records []Record // [0] is the manifest when len > 1
}

// Skip is one item that yielded no record, with a stable Code.
type Skip struct {
	Path   string
	Code   string // unreadable|binary|unsupported_extension|too_large|empty|symlink|outside_root
	Detail string
}

// Result is what Extract resolved a request into.
type Result struct {
	Sources []Source
	Skipped []Skip // always rendered; absent and "nothing skipped" must not look alike
}

// Extractor is the ingestion use case: it extracts through the reader it is handed and reaches for nothing else. A
// Text request never asks the reader.
type Extractor struct {
	Reader SourceReader
}

// Extract resolves req into records. It returns an error only when the request itself cannot be resolved; every
// per-item failure is a Skip and the call still succeeds, possibly with zero Sources.
func (e Extractor) Extract(req Request) (Result, error) {
	if err := validate(req); err != nil {
		return Result{}, err
	}
	origin, text := req.Origin, req.Text
	skipped := []Skip{}
	if req.Path != "" {
		var skip *Skip
		var err error
		if origin, text, skip, err = e.readFile(req); err != nil {
			return Result{}, err
		}
		if skip != nil {
			return Result{Skipped: append(skipped, *skip)}, nil
		}
	}
	// The id is a function of the origin alone, so it is derived before any content is looked at.
	id, err := SourceID(origin)
	if err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(text) == "" {
		return Result{Skipped: append(skipped, Skip{Path: skipPath(origin), Code: "empty", Detail: "no text but whitespace"})}, nil
	}
	chunks, err := ChunkText(text, req.MaxChunkBytes, req.MinChunkBytes)
	if err != nil {
		return Result{}, err
	}
	if len(chunks) > 1 {
		return Result{}, errors.New("ingest: a source of several chunks is not built yet")
	}
	return Result{Sources: []Source{buildSource(req, origin, id, text, chunks)}, Skipped: skipped}, nil
}

// textExtensions are the file extensions read as text.
var textExtensions = map[string]bool{".md": true, ".markdown": true, ".txt": true, ".text": true, ".rst": true, ".adoc": true}

// binarySniffBytes is how far into a file a NUL byte marks it as binary.
const binarySniffBytes = 8 << 10

// readFile reads the one file a Path request names, through the reader. It returns the origin made absolute and
// the file's text, or the Skip that says why the file yields none; the error is for a path that is not a file.
func (e Extractor) readFile(req Request) (Origin, string, *Skip, error) {
	origin := req.Origin
	origin.URI = req.Path
	origin, err := ResolveOrigin(e.Reader, origin)
	if err != nil {
		return Origin{}, "", nil, err
	}
	entry, err := e.Reader.Lstat(origin.URI)
	if err != nil {
		return Origin{}, "", nil, fmt.Errorf("ingest: %w", err)
	}
	switch entry.Kind {
	case EntryFile:
	case EntryDirectory:
		return Origin{}, "", nil, fmt.Errorf("ingest: %s is a directory, and scanning one is not built yet", origin.URI)
	default:
		// A link at Path is a request error too: the operator names the real path (Phase 9 C22, D1).
		return Origin{}, "", nil, fmt.Errorf("ingest: %s is neither file nor directory (it is of kind %s)", origin.URI, entry.Kind)
	}
	origin.Kind = KindFile
	skip := func(code, detail string) (Origin, string, *Skip, error) {
		return origin, "", &Skip{Path: origin.URI, Code: code, Detail: detail}, nil
	}
	if ext := strings.ToLower(filepath.Ext(origin.URI)); !textExtensions[ext] {
		return skip("unsupported_extension", fmt.Sprintf("extension %q is not a text extension", ext))
	}
	limit := req.MaxSourceBytes
	if limit <= 0 {
		limit = DefaultMaxSourceBytes
	}
	if entry.Size > int64(limit) {
		return skip("too_large", fmt.Sprintf("%d bytes, above the bound of %d", entry.Size, limit))
	}
	data, err := e.Reader.ReadFile(origin.URI)
	if err != nil {
		return skip("unreadable", err.Error())
	}
	if bytes.IndexByte(data[:min(len(data), binarySniffBytes)], 0) >= 0 {
		return skip("binary", "a NUL byte within the first 8 KiB")
	}
	return origin, string(data), nil, nil
}

// skipPath names an item that yielded nothing: its path, or for pasted text its label.
func skipPath(o Origin) string {
	if o.URI != "" {
		return o.URI
	}
	return o.Label
}

// sourceURI is what the records say the source is: the override when a file or directory has one, so that no
// machine path reaches a record, and otherwise the origin's URI.
func sourceURI(o Origin) string {
	if (o.Kind == KindFile || o.Kind == KindDirectory) && o.Override != "" {
		return o.Override
	}
	return o.URI
}

func digest(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// buildSource turns a source's one chunk into its standalone record.
func buildSource(req Request, origin Origin, id, text string, chunks []Chunk) Source {
	base := Header{
		SourceKind:   origin.Kind,
		SourceURI:    sourceURI(origin),
		SourceID:     id,
		SourceTitle:  origin.Title,
		SourceSHA256: digest(normalizeCRLF(text)),
		IngestedAt:   req.Now(),
		IngestedBy:   req.IngestedBy,
		Status:       StatusComplete,
	}
	topic := "ingested/" + string(origin.Kind) + "/" + id
	title := "Ingested: " + origin.Title
	src := Source{Origin: origin, ID: id, Digest: base.SourceSHA256}

	for _, c := range chunks {
		header := c.Header(base)
		header.ContentSHA256 = digest(c.Text)
		record := Record{TopicKey: topic, Title: title, Type: "discovery", Role: RoleStandalone, Header: header}
		record.Content = c.Text + "\n" + header.Render()
		record.Bytes = len(record.Content)
		src.Records = append(src.Records, record)
	}
	return src
}

// validate refuses a request that cannot be resolved, before anything is asked of a reader.
func validate(req Request) error {
	switch {
	case req.Text != "" && req.Path != "":
		return errors.New("ingest: a request holds either Text or Path, not both")
	case req.Text == "" && req.Path == "":
		return errors.New("ingest: a request holds Text or Path, and both are empty")
	case req.Now == nil:
		return errors.New("ingest: a request needs Now: this package reads no clock")
	}
	switch req.Origin.Kind {
	case KindURL, KindFile, KindDirectory:
	case KindPasted:
		if req.Origin.Label == "" {
			return errors.New("ingest: pasted origin requires a Label")
		}
	default:
		return fmt.Errorf("ingest: unknown source kind %q", req.Origin.Kind)
	}
	if req.Path != "" && req.Origin.Kind != KindFile && req.Origin.Kind != KindDirectory {
		return fmt.Errorf("ingest: a Path request needs a file or directory origin, got %q", req.Origin.Kind)
	}
	if req.Path != "" && req.Origin.URI != "" {
		return errors.New("ingest: a Path request names its path in Path, not in Origin.URI")
	}
	return nil
}
