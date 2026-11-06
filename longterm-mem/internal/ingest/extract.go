package ingest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
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
// Text request never asks the reader, and is bounded only by MaxChunks: the caller already holds the text, so
// MaxSourceBytes, which bounds what is read from a path, does not apply to it.
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
	if req.Path != "" {
		root, entry, skip, err := e.locate(req)
		if err != nil {
			return Result{}, err
		}
		if skip != nil {
			return Result{Skipped: []Skip{*skip}}, nil
		}
		if entry.Kind == EntryDirectory {
			return e.scan(req, root)
		}
		if origin, text, skip = e.readText(req, root, entry); skip != nil {
			return Result{Skipped: []Skip{*skip}}, nil
		}
	}
	source, skip, err := extractOne(req, origin, text)
	if err != nil {
		return Result{}, err
	}
	if skip != nil {
		return Result{Skipped: []Skip{*skip}}, nil
	}
	return Result{Sources: []Source{*source}, Skipped: []Skip{}}, nil
}

// extractOne turns one source's text into its Source, or into the Skip that says it holds none.
func extractOne(req Request, origin Origin, text string) (*Source, *Skip, error) {
	// The id is a function of the origin alone, so it is derived before any content is looked at.
	id, err := SourceID(origin)
	if err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(text) == "" {
		return nil, &Skip{Path: skipPath(origin), Code: "empty", Detail: "no text but whitespace"}, nil
	}
	chunks, err := ChunkText(text, req.MaxChunkBytes, req.MinChunkBytes)
	if err != nil {
		return nil, nil, err
	}
	source := buildSource(req, origin, id, text, chunks)
	return &source, nil, nil
}

// textExtensions are the file extensions read as text.
var textExtensions = map[string]bool{".md": true, ".markdown": true, ".txt": true, ".text": true, ".rst": true, ".adoc": true}

// binarySniffBytes is how far into a file a NUL byte marks it as binary.
const binarySniffBytes = 8 << 10

// locate resolves the path a Path request names and looks at it through the reader. It returns the origin made
// absolute and the entry found there, or the Skip that says the entry cannot be looked at; the error is for a
// path that is neither a file nor a directory, or that does not fit the origin's kind.
func (e Extractor) locate(req Request) (Origin, Entry, *Skip, error) {
	origin := req.Origin
	origin.URI = req.Path
	origin, err := ResolveOrigin(e.Reader, origin)
	if err != nil {
		return Origin{}, Entry{}, nil, err
	}
	entry, err := e.Reader.Lstat(origin.URI)
	if errors.Is(err, fs.ErrPermission) {
		// Being denied a look at a path that is there is a per-item failure, as a failed read is.
		return origin, Entry{}, &Skip{Path: origin.URI, Code: "unreadable", Detail: err.Error()}, nil
	}
	if err != nil {
		// Anything else, a missing path or one that is not usable as given (ENOTDIR, ELOOP, ENAMETOOLONG), is a
		// request error, so a mistyped Path never ends in success with no source (design.md, OQ-6).
		return Origin{}, Entry{}, nil, fmt.Errorf("ingest: %w", err)
	}
	switch entry.Kind {
	case EntryFile:
		if origin.Kind == KindDirectory {
			return Origin{}, Entry{}, nil, fmt.Errorf("ingest: the origin is a directory but the path names a file: %s", origin.URI)
		}
	case EntryDirectory:
		if origin.Kind == KindFile {
			return Origin{}, Entry{}, nil, fmt.Errorf("ingest: the origin is a file but the path names a directory: %s", origin.URI)
		}
	default:
		// A link at Path is a request error too: the operator names the real path (Phase 9 C22, D1).
		return Origin{}, Entry{}, nil, fmt.Errorf("ingest: %s is neither file nor directory (it is of kind %s)", origin.URI, entry.Kind)
	}
	return origin, entry, nil, nil
}

// readText reads the one regular file at origin.URI, whose entry Lstat reported, through the reader. It returns
// the origin as a file's and the file's text, or the Skip that says why the file yields none. A scan reads each of
// its files here, as a Path request reads its one.
func (e Extractor) readText(req Request, origin Origin, entry Entry) (Origin, string, *Skip) {
	origin.Kind = KindFile
	skip := func(code, detail string) (Origin, string, *Skip) {
		return origin, "", &Skip{Path: origin.URI, Code: code, Detail: detail}
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
	// A correctness check, not a memory bound: the Lstat Size above is what keeps a large file from being read,
	// and this catches the file that grew between Lstat and ReadFile, after its bytes are already in memory.
	if len(data) > limit {
		return skip("too_large", fmt.Sprintf("%d bytes read, above the bound of %d", len(data), limit))
	}
	if bytes.IndexByte(data[:min(len(data), binarySniffBytes)], 0) >= 0 {
		return skip("binary", "a NUL byte within the first 8 KiB")
	}
	return origin, string(data), nil
}

// scanItem is one thing a scan found, or one reason it found nothing: rel is its path below the root, the key the
// scan orders by; a file has an entry, anything else has the skip.
type scanItem struct {
	rel   string
	path  string
	entry Entry
	skip  *Skip
}

// scan extracts every file of the directory root names, in lexical order of path below the root. A link is never
// followed, listed through or read, and no entry whose cleaned path is not below the root is looked at; the paths
// alone decide that, as no link is ever resolved. Each file is a source of its own: a file origin with no
// override, titled by its name. Every failure of one entry is a Skip, and only a root that cannot be listed for a
// reason other than permission fails the request.
func (e Extractor) scan(req Request, rootOrigin Origin) (Result, error) {
	root := filepath.Clean(rootOrigin.URI)
	names, err := e.Reader.ReadDir(root)
	if errors.Is(err, fs.ErrPermission) {
		return Result{Skipped: []Skip{{Path: root, Code: "unreadable", Detail: err.Error()}}}, nil
	}
	if err != nil {
		return Result{}, fmt.Errorf("ingest: %w", err)
	}
	items := e.walk(root, root, "", names, req.Recursive)
	slices.SortStableFunc(items, func(a, b scanItem) int { return strings.Compare(a.rel, b.rel) })
	result := Result{Skipped: []Skip{}}
	for _, item := range items {
		if item.skip != nil {
			result.Skipped = append(result.Skipped, *item.skip)
			continue
		}
		base := filepath.Base(item.path)
		origin := Origin{Kind: KindFile, URI: item.path, Title: strings.TrimSuffix(base, filepath.Ext(base))}
		origin, text, skip := e.readText(req, origin, item.entry)
		var source *Source
		if skip == nil {
			if source, skip, err = extractOne(req, origin, text); err != nil {
				return Result{}, err
			}
		}
		if skip != nil {
			result.Skipped = append(result.Skipped, *skip)
			continue
		}
		result.Sources = append(result.Sources, *source)
	}
	return result, nil
}

// walk looks at each name listed in dir, whose path below the root is rel, and returns what it found.
func (e Extractor) walk(root, dir, rel string, names []string, recursive bool) []scanItem {
	var items []scanItem
	for _, name := range names {
		key := name
		if rel != "" {
			key = rel + string(filepath.Separator) + name
		}
		path := filepath.Join(dir, name)
		unreadable := func(err error) scanItem {
			return scanItem{rel: key, skip: &Skip{Path: path, Code: "unreadable", Detail: err.Error()}}
		}
		if !below(root, path) {
			items = append(items, scanItem{rel: key, skip: &Skip{
				Path: path, Code: "outside_root", Detail: fmt.Sprintf("the entry %q leaves the scan root %s", name, root),
			}})
			continue
		}
		entry, err := e.Reader.Lstat(path)
		if err != nil {
			items = append(items, unreadable(err))
			continue
		}
		switch entry.Kind {
		case EntryFile:
			items = append(items, scanItem{rel: key, path: path, entry: entry})
		case EntryDirectory:
			if !recursive {
				continue
			}
			children, err := e.Reader.ReadDir(path)
			if err != nil {
				items = append(items, unreadable(err))
				continue
			}
			items = append(items, e.walk(root, path, key, children, recursive)...)
		case EntrySymlink:
			items = append(items, scanItem{rel: key, skip: &Skip{Path: path, Code: "symlink", Detail: "a link is never followed"}})
		default:
			items = append(items, unreadable(fmt.Errorf("neither file nor directory (it is of kind %s)", entry.Kind)))
		}
	}
	return items
}

// below reports whether path, already cleaned, is strictly inside root, already cleaned.
func below(root, path string) bool {
	prefix := root
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	return strings.HasPrefix(path, prefix)
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

// chunkSuffix is the four-digit ordinal of a chunk (c0001...), as the topic-key grammar writes it.
func chunkSuffix(index int) string {
	return fmt.Sprintf("c%04d", index)
}

func digest(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// buildSource turns a source's chunks into its records: one standalone record for a single chunk, otherwise a
// manifest followed by the chunks. Every record carries the same origin, id, digest and timestamp.
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

	n := len(chunks)
	if n > 1 {
		manifest := base
		manifest.IsManifest = true
		manifest.ChunksExpected = n
		manifest.Status = StatusPending
		for i := range chunks {
			manifest.ChunkStatus = append(manifest.ChunkStatus, chunkSuffix(i+1)+" pending")
		}
		content := manifest.Render()
		src.Records = append(src.Records, Record{
			TopicKey: topic, Title: title, Type: "discovery", Role: RoleManifest,
			Content: content, Header: manifest, Bytes: len(content),
		})
	}
	for _, c := range chunks {
		header := c.Header(base)
		header.ContentSHA256 = digest(c.Text)
		record := Record{TopicKey: topic, Title: title, Type: "discovery", Role: RoleStandalone, Header: header}
		if n > 1 {
			record.TopicKey = topic + "/" + chunkSuffix(c.Index)
			record.Title = fmt.Sprintf("%s (%d/%d)", title, c.Index, c.Total)
			record.Role = RoleChunk
		}
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
	// One literal override cannot be the identity of every file a scan finds, and a directory's is never inherited
	// by them (Phase 9 C22, D3).
	if req.Path != "" && req.Origin.Kind == KindDirectory && req.Origin.Override != "" {
		return errors.New("ingest: a directory scan cannot carry an Origin.Override: it would name every file it finds")
	}
	return nil
}
