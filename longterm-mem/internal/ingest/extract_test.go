package ingest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"time"
)

// memTree is a SourceReader over a map of path to entry, bytes and failure, so a case sets exactly what the
// machine would answer. It touches nothing and records what it is asked, which is how a case proves that the use
// case never read what it must not read.
type memTree struct {
	wd    string
	files map[string]memFile
	// lstats and reads are the paths asked of Lstat and ReadFile, in order.
	lstats []string
	reads  []string
	asked  int
}

type memFile struct {
	entry   Entry
	data    []byte
	readErr error
	// lstatErr is what Lstat fails with for this path instead of answering.
	lstatErr error
}

var _ SourceReader = (*memTree)(nil)

func (m *memTree) WorkingDirectory() (string, error) {
	m.asked++
	return m.wd, nil
}

func (m *memTree) Lstat(path string) (Entry, error) {
	m.lstats = append(m.lstats, path)
	f, ok := m.files[path]
	if !ok {
		return Entry{}, fmt.Errorf("lstat %s: %w", path, fs.ErrNotExist)
	}
	return f.entry, f.lstatErr
}

func (m *memTree) ReadFile(path string) ([]byte, error) {
	m.reads = append(m.reads, path)
	f, ok := m.files[path]
	if !ok {
		return nil, fmt.Errorf("open %s: %w", path, fs.ErrNotExist)
	}
	return f.data, f.readErr
}

func (m *memTree) ReadDir(path string) ([]string, error) {
	return nil, errors.New("memTree: no case lists a directory yet")
}

// textFile is a regular file holding text, its size the length of its bytes.
func textFile(text string) memFile {
	return memFile{entry: Entry{Kind: EntryFile, Size: int64(len(text))}, data: []byte(text)}
}

var ingestedAt = time.Date(2026, 11, 6, 10, 4, 0, 0, time.UTC)

// fileRequest is a valid request for one file, which a case then bends.
func fileRequest(path string) Request {
	return Request{
		Path:       path,
		Origin:     Origin{Kind: KindFile, Title: "Guide"},
		Now:        func() time.Time { return ingestedAt },
		IngestedBy: "tester",
	}
}

// pastedRequest is a valid request for pasted text.
func pastedRequest(text string) Request {
	return Request{
		Text:       text,
		Origin:     Origin{Kind: KindPasted, Label: "Meeting notes", Title: "Meeting"},
		Now:        func() time.Time { return ingestedAt },
		IngestedBy: "tester",
	}
}

func TestExtractRefusesARequestThatCannotBeResolved(t *testing.T) {
	var manySections strings.Builder
	for i := 0; i <= MaxChunks+10; i++ {
		fmt.Fprintf(&manySections, "## H%d\n\nshort body\n\n", i)
	}
	cases := []struct {
		name    string
		request func() Request
		wantIs  error
	}{
		{"Text and Path both set", func() Request {
			r := pastedRequest("some text")
			r.Path = "/notes/a.md"
			return r
		}, nil},
		{"Text and Path both empty", func() Request {
			r := pastedRequest("")
			return r
		}, nil},
		{"an invalid Origin.Kind", func() Request {
			r := pastedRequest("some text")
			r.Origin.Kind = "ftp"
			return r
		}, nil},
		{"a pasted origin with no Label", func() Request {
			r := pastedRequest("some text")
			r.Origin.Label = ""
			return r
		}, nil},
		{"a Path request with a url origin", func() Request {
			r := fileRequest("/notes/a.md")
			r.Origin.Kind = KindURL
			return r
		}, nil},
		{"a Path request that also sets Origin.URI", func() Request {
			r := fileRequest("/notes/a.md")
			r.Origin.URI = "/notes/b.md"
			return r
		}, nil},
		{"no Now", func() Request {
			r := pastedRequest("some text")
			r.Now = nil
			return r
		}, nil},
		{"a source that would exceed MaxChunks", func() Request {
			return pastedRequest(manySections.String())
		}, ErrTooManyChunks},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reader := &memTree{wd: "/work", files: map[string]memFile{"/notes/a.md": textFile("# A\n")}}

			got, err := Extractor{Reader: reader}.Extract(c.request())

			if err == nil {
				t.Fatalf("Extract = %+v, want an error", got)
			}
			if c.wantIs != nil && !errors.Is(err, c.wantIs) {
				t.Errorf("Extract error = %v, want errors.Is %v", err, c.wantIs)
			}
			if len(got.Sources) != 0 || len(got.Skipped) != 0 {
				t.Errorf("Extract = %+v with an error, want an empty Result", got)
			}
			if len(reader.lstats)+len(reader.reads)+reader.asked != 0 {
				t.Errorf("the reader was asked (%d Lstat, %d ReadFile, %d WorkingDirectory) before the request was refused",
					len(reader.lstats), len(reader.reads), reader.asked)
			}
		})
	}
}

func TestExtractSkipsWhatOneFileCannotYield(t *testing.T) {
	nul := func(at, total int) string {
		b := []byte(strings.Repeat("a", total))
		b[at] = 0
		return string(b)
	}
	readErr := errors.New("open /notes/a.md: permission denied")
	cases := []struct {
		name     string
		path     string
		file     memFile
		maxBytes int
		wantCode string // "" when the file yields one source
		wantRead bool
	}{
		{"unreadable", "/notes/a.md", memFile{entry: Entry{Kind: EntryFile, Size: 5}, readErr: readErr}, 0, "unreadable", true},
		{"binary", "/notes/a.md", textFile(nul(100, 200)), 0, "binary", true},
		{"a NUL byte past the first 8 KiB is not binary", "/notes/a.md", textFile(nul(8192, 9000)), 0, "", true},
		{"unsupported_extension", "/notes/a.pdf", textFile("# A\n"), 0, "unsupported_extension", false},
		{"too_large", "/notes/a.md", memFile{entry: Entry{Kind: EntryFile, Size: 11}, data: []byte("# A\n")}, 10, "too_large", false},
		// Lstat's size is only what the reader saw first: the bytes it returns are held to the bound too.
		{"bytes over the bound though Lstat reported less", "/notes/a.md", memFile{entry: Entry{Kind: EntryFile, Size: 4}, data: []byte("0123456789A")}, 10, "too_large", true},
		{"as large as the bound is not too_large", "/notes/a.md", textFile("0123456789"), 10, "", true},
		{"empty", "/notes/a.md", textFile(""), 0, "empty", true},
		{"whitespace only", "/notes/a.md", textFile(" \n\t\r\n "), 0, "empty", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reader := &memTree{files: map[string]memFile{c.path: c.file}}
			req := fileRequest(c.path)
			req.MaxSourceBytes = c.maxBytes
			req.MaxChunkBytes = 10000 // one chunk even for the 9000-byte file

			got, err := Extractor{Reader: reader}.Extract(req)

			if err != nil {
				t.Fatalf("Extract: %v: a per-item failure is a skip, not an error", err)
			}
			if wantRead := len(reader.reads) == 1; wantRead != c.wantRead {
				t.Errorf("ReadFile was asked %d times, want it asked: %v", len(reader.reads), c.wantRead)
			}
			if c.wantCode == "" {
				if len(got.Sources) != 1 || len(got.Skipped) != 0 {
					t.Fatalf("Extract = %+v, want one source and no skip", got)
				}
				return
			}
			if len(got.Sources) != 0 || len(got.Skipped) != 1 {
				t.Fatalf("Extract = %+v, want zero sources and one skip: success with nothing extracted", got)
			}
			skip := got.Skipped[0]
			if skip.Path != c.path || skip.Code != c.wantCode || skip.Detail == "" {
				t.Errorf("skip = %+v, want path %q, code %q and a detail", skip, c.path, c.wantCode)
			}
			if c.wantCode == "unreadable" && skip.Detail != readErr.Error() {
				t.Errorf("unreadable detail = %q, want the reader's failure %q", skip.Detail, readErr.Error())
			}
		})
	}
}

// guideBody is the text of the file the one-file cases read; its digest and the ids below were computed with
// sha256sum from the contract's formula, not by running the code:
//
//	printf '# Guide\n\nHello.\n' | sha256sum                  = 39d6427f...cdefcded
//	printf '%s' '/notes/Guide v2.md' | sha256sum               = 94059a5c...
//	printf '%s' '/work/notes/a.md' | sha256sum                 = 4a027ea7...
//	printf '%s' '/elsewhere/notes/a.md' | sha256sum            = 143a46ec...
//	printf '%s' 'repo://openspec/specs/x.md' | sha256sum       = 8bfa69fe...
//	printf '%s' 'https://example.com/docs/config' | sha256sum  = 71df4c1f...
const (
	guideBody   = "# Guide\n\nHello.\n"
	guideDigest = "39d6427fb9901e48098b4735301ed400cd936d40e8cdbb5778983cca0defcded"
)

func TestExtractOneFileIntoOneStandaloneRecord(t *testing.T) {
	path := "/notes/Guide v2.md"
	reader := &memTree{files: map[string]memFile{path: textFile(guideBody)}}

	got, err := Extractor{Reader: reader}.Extract(fileRequest(path))

	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(got.Sources) != 1 {
		t.Fatalf("Extract = %+v, want one source", got)
	}
	if got.Skipped == nil || len(got.Skipped) != 0 {
		t.Errorf("Skipped = %#v, want an empty list that is not nil: nothing skipped must not look like absent", got.Skipped)
	}
	if out, err := json.Marshal(got); err != nil || !strings.Contains(string(out), `"Skipped":[]`) {
		t.Errorf("Result renders as %s (%v), want it to carry \"Skipped\":[]", out, err)
	}
	src := got.Sources[0]
	id := "guide-v2-94059a5c"
	if want := (Origin{Kind: KindFile, URI: path, Title: "Guide"}); src.Origin != want {
		t.Errorf("Origin = %+v, want %+v", src.Origin, want)
	}
	if src.ID != id || src.Digest != guideDigest {
		t.Errorf("ID, Digest = %q, %q, want %q, %q", src.ID, src.Digest, id, guideDigest)
	}
	if len(src.Records) != 1 {
		t.Fatalf("Records = %+v, want exactly one: a single chunk writes one record", src.Records)
	}
	rec := src.Records[0]
	wantHeader := Header{
		SourceKind: KindFile, SourceURI: path, SourceID: id, SourceTitle: "Guide",
		SourceSHA256: guideDigest, ContentSHA256: guideDigest, IngestedAt: ingestedAt, IngestedBy: "tester",
		ChunkN: 1, ChunkTotal: 1, ChunkSpanStart: 0, ChunkSpanEnd: len(guideBody), ChunkPath: "Guide",
		Split: SplitNone, Status: StatusComplete,
	}
	if !reflect.DeepEqual(rec.Header, wantHeader) {
		t.Errorf("Header = %+v, want %+v", rec.Header, wantHeader)
	}
	if want := guideBody + "\n" + rec.Header.Render(); rec.Content != want || rec.Bytes != len(want) {
		t.Errorf("Content (%d bytes) = %q, want the body, a newline and the rendered header (%d bytes)", rec.Bytes, rec.Content, len(want))
	}
	if !reflect.DeepEqual(reader.lstats, []string{path}) || !reflect.DeepEqual(reader.reads, []string{path}) {
		t.Errorf("the reader was asked Lstat %v and ReadFile %v, want the one path in each", reader.lstats, reader.reads)
	}
}

func TestExtractBeginsARelativePathAtTheReadersWorkingDirectory(t *testing.T) {
	for _, c := range []struct{ wd, id string }{{"/work", "a-4a027ea7"}, {"/elsewhere", "a-143a46ec"}} {
		t.Run(c.wd, func(t *testing.T) {
			abs := c.wd + "/notes/a.md"
			reader := &memTree{wd: c.wd, files: map[string]memFile{abs: textFile(guideBody)}}

			got, err := Extractor{Reader: reader}.Extract(fileRequest("notes/a.md"))

			if err != nil || len(got.Sources) != 1 {
				t.Fatalf("Extract = %+v, %v, want one source", got, err)
			}
			if src := got.Sources[0]; src.Origin.URI != abs || src.ID != c.id || src.Records[0].Header.SourceURI != abs {
				t.Errorf("origin path, id, Source-URI = %q, %q, %q, want %q, %q, %q",
					src.Origin.URI, src.ID, src.Records[0].Header.SourceURI, abs, c.id, abs)
			}
			if !reflect.DeepEqual(reader.lstats, []string{abs}) {
				t.Errorf("Lstat was asked %v, want the absolute path %q", reader.lstats, abs)
			}
		})
	}
}

func TestExtractIdentifiesAFileByItsOverrideWhateverTheMachine(t *testing.T) {
	const override = "repo://openspec/specs/x.md"
	for _, wd := range []string{"/work", "/elsewhere"} {
		t.Run(wd, func(t *testing.T) {
			abs := wd + "/specs/x.md"
			reader := &memTree{wd: wd, files: map[string]memFile{abs: textFile(guideBody)}}
			req := fileRequest("specs/x.md")
			req.Origin.Override = override

			got, err := Extractor{Reader: reader}.Extract(req)

			if err != nil || len(got.Sources) != 1 {
				t.Fatalf("Extract = %+v, %v, want one source", got, err)
			}
			src := got.Sources[0]
			if src.ID != "x-8bfa69fe" {
				t.Errorf("ID = %q, want x-8bfa69fe: the override's, in every working directory", src.ID)
			}
			if src.Origin.Override != override || src.Origin.URI != abs {
				t.Errorf("Origin = %+v, want the override %q kept and the path %q read", src.Origin, override, abs)
			}
			rec := src.Records[0]
			if rec.Header.SourceURI != override || strings.Contains(rec.Content, wd) {
				t.Errorf("Source-URI = %q, and the content names %q: want the override and no machine path", rec.Header.SourceURI, wd)
			}
		})
	}
}

func TestExtractATextRequestNeverAsksTheReader(t *testing.T) {
	reader := &directory{dir: "/work", err: errors.New("the reader must not be asked")}
	req := Request{
		Text:       guideBody,
		Origin:     Origin{Kind: KindURL, URI: "https://example.com/docs/config", Title: "Config"},
		Now:        func() time.Time { return ingestedAt },
		IngestedBy: "tester",
	}

	got, err := Extractor{Reader: reader}.Extract(req)

	if err != nil || len(got.Sources) != 1 {
		t.Fatalf("Extract = %+v, %v, want one source", got, err)
	}
	src := got.Sources[0]
	if src.ID != "example-com-docs-config-71df4c1f" || src.Records[0].TopicKey != "ingested/url/example-com-docs-config-71df4c1f" ||
		src.Records[0].Header.SourceURI != "https://example.com/docs/config" {
		t.Errorf("ID, TopicKey, Source-URI = %q, %q, %q", src.ID, src.Records[0].TopicKey, src.Records[0].Header.SourceURI)
	}
	if reader.asked != 0 || reader.reads != 0 {
		t.Errorf("the reader was asked %d times for a directory and %d times to read, want neither", reader.asked, reader.reads)
	}
}

func TestExtractRefusesAPathThatIsNotAFile(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]memFile
		want  string
	}{
		{"a path that is not there", nil, ""},
		// The owner's answer to D1: a link at Path is a request error, however it points.
		{"a link", map[string]memFile{"/notes/a.md": {entry: Entry{Kind: EntrySymlink}, data: []byte(guideBody)}}, "neither file nor directory"},
		{"any other kind", map[string]memFile{"/notes/a.md": {entry: Entry{Kind: EntryOther}, data: []byte(guideBody)}}, "neither file nor directory"},
		// A scan is the next task's; until it is built a directory is refused, not read as a file.
		{"a directory", map[string]memFile{"/notes/a.md": {entry: Entry{Kind: EntryDirectory}}}, "directory"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reader := &memTree{files: c.files}

			got, err := Extractor{Reader: reader}.Extract(fileRequest("/notes/a.md"))

			if err == nil {
				t.Fatalf("Extract = %+v, want an error", got)
			}
			if c.files == nil && !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("error = %v, want errors.Is fs.ErrNotExist", err)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %q, want it to say %q", err, c.want)
			}
			if len(got.Sources) != 0 || len(got.Skipped) != 0 || len(reader.reads) != 0 {
				t.Errorf("Extract = %+v and %d reads: a request error carries no result and reads nothing", got, len(reader.reads))
			}
		})
	}
}

// threeSections is a file the chunker cuts into three chunks, one per heading.
const threeSections = "## One\n\nalpha alpha alpha.\n\n## Two\n\nbeta beta beta.\n\n## Three\n\ngamma gamma.\n"

// extractThreeChunks extracts threeSections from the file the one-file cases use, with the bounds that keep the
// three sections apart.
func extractThreeChunks(t *testing.T) Source {
	t.Helper()
	path := "/notes/Guide v2.md"
	reader := &memTree{files: map[string]memFile{path: textFile(threeSections)}}
	req := fileRequest(path)
	req.MinChunkBytes = 1

	got, err := Extractor{Reader: reader}.Extract(req)

	if err != nil || len(got.Sources) != 1 {
		t.Fatalf("Extract = %+v, %v, want one source", got, err)
	}
	return got.Sources[0]
}

// provenance is the part of a record that R-007 says every record carries, as the record's own content states it.
type provenance struct {
	marker         bool
	kind           SourceKind
	uri, id, sha   string
	title, by      string
	ingestedAtUnix int64
}

func provenanceOf(t *testing.T, rec Record) provenance {
	t.Helper()
	h, ok := ParseHeader(rec.Content)
	return provenance{ok, h.SourceKind, h.SourceURI, h.SourceID, h.SourceSHA256, h.SourceTitle, h.IngestedBy, h.IngestedAt.Unix()}
}

func TestExtractEveryRecordCarriesOriginTimestampAndTheIngestedMarker(t *testing.T) {
	want := provenance{true, KindFile, "/notes/Guide v2.md", "guide-v2-94059a5c", "", "Guide", "tester", ingestedAt.Unix()}

	t.Run("a single-unit extraction", func(t *testing.T) {
		path := "/notes/Guide v2.md"
		reader := &memTree{files: map[string]memFile{path: textFile(guideBody)}}
		got, err := Extractor{Reader: reader}.Extract(fileRequest(path))
		if err != nil || len(got.Sources) != 1 || len(got.Sources[0].Records) != 1 {
			t.Fatalf("Extract = %+v, %v, want one source of one record", got, err)
		}
		w := want
		w.sha = guideDigest
		if p := provenanceOf(t, got.Sources[0].Records[0]); p != w {
			t.Errorf("the record's provenance = %+v, want %+v", p, w)
		}
	})

	t.Run("a multi-chunk extraction", func(t *testing.T) {
		src := extractThreeChunks(t)
		if len(src.Records) != 4 {
			t.Fatalf("Records = %d, want the manifest and three chunks", len(src.Records))
		}
		if src.Records[0].Role != RoleManifest {
			t.Fatalf("Records[0] is a %q, want the manifest first", src.Records[0].Role)
		}
		w := want
		w.sha = src.Digest
		seenSpans := map[[2]int]bool{}
		seenContent := map[string]bool{}
		for i, rec := range src.Records {
			if p := provenanceOf(t, rec); p != w {
				t.Errorf("record %d provenance = %+v, want %+v: identical across the manifest and every chunk", i, p, w)
			}
			seenContent[rec.Content] = true
			if i > 0 {
				seenSpans[[2]int{rec.Header.ChunkSpanStart, rec.Header.ChunkSpanEnd}] = true
			}
		}
		if len(seenSpans) != 3 || len(seenContent) != 4 {
			t.Errorf("%d distinct spans and %d distinct contents, want 3 and 4: records differ only in chunk-specific fields", len(seenSpans), len(seenContent))
		}
	})
}

func TestExtractShapesEveryRecordAsOneMemSaveCall(t *testing.T) {
	const topic = "ingested/file/guide-v2-94059a5c"

	t.Run("one chunk is standalone", func(t *testing.T) {
		path := "/notes/Guide v2.md"
		reader := &memTree{files: map[string]memFile{path: textFile(guideBody)}}
		got, err := Extractor{Reader: reader}.Extract(fileRequest(path))
		if err != nil || len(got.Sources) != 1 || len(got.Sources[0].Records) != 1 {
			t.Fatalf("Extract = %+v, %v, want one source of one record", got, err)
		}
		rec := got.Sources[0].Records[0]
		if rec.TopicKey != topic || rec.Title != "Ingested: Guide" || rec.Type != "discovery" || rec.Role != RoleStandalone {
			t.Errorf("record = %q, %q, %q, %q, want the manifest key, \"Ingested: Guide\", discovery and standalone",
				rec.TopicKey, rec.Title, rec.Type, rec.Role)
		}
	})

	t.Run("several chunks are a manifest and the chunks", func(t *testing.T) {
		src := extractThreeChunks(t)
		manifest := src.Records[0]
		if manifest.TopicKey != topic || manifest.Title != "Ingested: Guide" || manifest.Type != "discovery" || manifest.Role != RoleManifest {
			t.Errorf("manifest = %q, %q, %q, %q, want the manifest key, \"Ingested: Guide\", discovery and manifest",
				manifest.TopicKey, manifest.Title, manifest.Type, manifest.Role)
		}
		h := manifest.Header
		if !h.IsManifest || h.ChunksExpected != 3 || h.ChunksSaved != 0 || h.Status != StatusPending ||
			!reflect.DeepEqual(h.ChunkStatus, []string{"c0001 pending", "c0002 pending", "c0003 pending"}) {
			t.Errorf("manifest header = %+v, want a pending manifest of three chunks, none saved", h)
		}
		if manifest.Content != h.Render() {
			t.Errorf("manifest content = %q, want the provenance block alone: a manifest carries no source body", manifest.Content)
		}
		for i, rec := range src.Records[1:] {
			n := i + 1
			wantKey, wantTitle := fmt.Sprintf("%s/c%04d", topic, n), fmt.Sprintf("Ingested: Guide (%d/3)", n)
			if rec.TopicKey != wantKey || rec.Title != wantTitle || rec.Type != "discovery" || rec.Role != RoleChunk ||
				rec.Header.ChunkN != n || rec.Header.ChunkTotal != 3 || rec.Header.Status != StatusComplete {
				t.Errorf("chunk %d = %q, %q, %q, %q, %d/%d, %q, want %q, %q, discovery, chunk, %d/3, complete",
					n, rec.TopicKey, rec.Title, rec.Type, rec.Role, rec.Header.ChunkN, rec.Header.ChunkTotal, rec.Header.Status, wantKey, wantTitle, n)
			}
		}
	})
}

func TestExtractSkipsAPathItCannotLookAt(t *testing.T) {
	reader := &memTree{files: map[string]memFile{"/notes/a.md": {lstatErr: fs.ErrPermission}}}

	got, err := Extractor{Reader: reader}.Extract(fileRequest("/notes/a.md"))

	if err != nil {
		t.Fatalf("Extract: %v: a path that cannot be looked at is a skip, only a missing one is a request error", err)
	}
	want := []Skip{{Path: "/notes/a.md", Code: "unreadable", Detail: fs.ErrPermission.Error()}}
	if len(got.Sources) != 0 || !reflect.DeepEqual(got.Skipped, want) {
		t.Errorf("Extract = %+v, want no source and the skip %+v", got, want)
	}
	if len(reader.reads) != 0 {
		t.Errorf("ReadFile was asked %d times after Lstat failed, want none", len(reader.reads))
	}
}

func TestExtractRefusesAFileNamedByADirectoryOrigin(t *testing.T) {
	reader := &memTree{files: map[string]memFile{"/notes/a.md": textFile(guideBody)}}
	req := fileRequest("/notes/a.md")
	req.Origin.Kind = KindDirectory

	got, err := Extractor{Reader: reader}.Extract(req)

	if err == nil || !strings.Contains(err.Error(), "the origin is a directory but the path names a file") {
		t.Fatalf("Extract = %+v, %v, want the request refused, not reclassified as a file", got, err)
	}
	if len(got.Sources) != 0 || len(reader.reads) != 0 {
		t.Errorf("Extract = %+v and %d reads, want an empty Result and no read", got, len(reader.reads))
	}
}

func TestExtractSkipsPastedTextThatIsOnlyWhitespace(t *testing.T) {
	reader := &directory{err: errors.New("the reader must not be asked")}

	got, err := Extractor{Reader: reader}.Extract(pastedRequest(" \n\t\n "))

	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	want := []Skip{{Path: "Meeting notes", Code: "empty", Detail: "no text but whitespace"}}
	if len(got.Sources) != 0 || !reflect.DeepEqual(got.Skipped, want) {
		t.Errorf("Extract = %+v, want no source and the skip %+v, its path the label", got, want)
	}
}

func TestExtractReadsEveryExtensionOfTheTextAllowlist(t *testing.T) {
	for _, ext := range []string{".md", ".markdown", ".txt", ".text", ".rst", ".adoc"} {
		t.Run(ext, func(t *testing.T) {
			path := "/notes/a" + ext
			reader := &memTree{files: map[string]memFile{path: textFile("# A\n")}}

			got, err := Extractor{Reader: reader}.Extract(fileRequest(path))

			if err != nil || len(got.Sources) != 1 || len(got.Skipped) != 0 {
				t.Errorf("Extract = %+v, %v, want one source and no skip", got, err)
			}
		})
	}
}
