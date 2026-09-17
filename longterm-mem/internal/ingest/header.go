// Package ingest turns already-fetched text and already-local files into
// records ready to hand to Engram's mem_save unchanged (R-005..R-007).
//
// It never fetches anything. It imports neither net nor net/http (R-071);
// the module-wide static guard in net_allowlist_test.go covers this
// package without amendment. The record shape it emits is defined
// normatively in skills/_shared/ingested-observation-contract.md, which
// this package implements rather than re-specifies.
//
// This file (header.go) covers the provenance-header/Source-Id half of the
// package. The chunk-boundary algorithm lands in a follow-up slice.
package ingest

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// SourceKind is one of the four origin kinds the ingested-observation
// contract recognizes (skills/_shared/ingested-observation-contract.md).
type SourceKind string

const (
	KindURL       SourceKind = "url"
	KindFile      SourceKind = "file"
	KindDirectory SourceKind = "directory"
	KindPasted    SourceKind = "pasted"
)

// Status is the provenance block's **Status** value. Chunk records use only
// StatusComplete or StatusRetired; manifest records may additionally use
// StatusPending or StatusPartial.
type Status string

const (
	StatusPending  Status = "pending"
	StatusComplete Status = "complete"
	StatusPartial  Status = "partial"
	StatusRetired  Status = "retired"
)

// SplitKind is the provenance block's **Split** value, recording which
// force-split strategy (if any) produced a chunk.
type SplitKind string

const (
	SplitNone           SplitKind = "none"
	SplitForcedSentence SplitKind = "forced-sentence"
	SplitForcedLine     SplitKind = "forced-line"
	SplitForcedByte     SplitKind = "forced-byte"
)

// Origin is what a record records as its source of record. The agent
// supplies it; this package never learns an origin by reaching for it.
type Origin struct {
	Kind  SourceKind
	URI   string // canonical origin; for KindFile/KindDirectory, the file path (or an override)
	Title string
	Label string // required for KindPasted, ignored otherwise
}

// Header is the provenance block's field set (the ingested-observation
// contract, skills/_shared/ingested-observation-contract.md), which this
// package implements rather than re-specifies.
type Header struct {
	SourceKind     SourceKind
	SourceURI      string
	SourceID       string
	SourceTitle    string
	SourceSHA256   string
	ContentSHA256  string
	IngestedAt     time.Time
	IngestedBy     string
	ChunkN         int
	ChunkTotal     int
	ChunkSpanStart int
	ChunkSpanEnd   int
	ChunkPath      string
	Split          SplitKind
	Status         Status

	// IsManifest selects the manifest-only field set (Chunks-Expected,
	// Chunks-Saved, Chunk-Status) in place of the chunk-specific fields
	// (Content-SHA256, Chunk, Chunk-Span, Chunk-Path, Split), matching the
	// contract's "Manifest-only additions" shape. A single-chunk
	// (standalone) record is NOT a manifest: it renders Chunk: 1/1 like any
	// other chunk record (contract rule 2).
	IsManifest     bool
	ChunksExpected int
	ChunksSaved    int
	ChunkStatus    []string
}

// Render writes h as the contract's verbatim provenance-block markdown,
// the last element of a record's content.
func (h Header) Render() string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "**Ingested**: true\n")
	fmt.Fprintf(&b, "**Source-Kind**: %s\n", h.SourceKind)
	fmt.Fprintf(&b, "**Source-URI**: %s\n", h.SourceURI)
	fmt.Fprintf(&b, "**Source-Id**: %s\n", h.SourceID)
	fmt.Fprintf(&b, "**Source-Title**: %s\n", h.SourceTitle)
	fmt.Fprintf(&b, "**Source-SHA256**: %s\n", h.SourceSHA256)
	if !h.IsManifest {
		fmt.Fprintf(&b, "**Content-SHA256**: %s\n", h.ContentSHA256)
	}
	fmt.Fprintf(&b, "**Ingested-At**: %s\n", h.IngestedAt.UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "**Ingested-By**: %s\n", h.IngestedBy)
	if !h.IsManifest {
		fmt.Fprintf(&b, "**Chunk**: %d/%d\n", h.ChunkN, h.ChunkTotal)
		fmt.Fprintf(&b, "**Chunk-Span**: %d-%d\n", h.ChunkSpanStart, h.ChunkSpanEnd)
		fmt.Fprintf(&b, "**Chunk-Path**: %s\n", h.ChunkPath)
		fmt.Fprintf(&b, "**Split**: %s\n", h.Split)
	}
	fmt.Fprintf(&b, "**Status**: %s\n", h.Status)
	if h.IsManifest {
		fmt.Fprintf(&b, "**Chunks-Expected**: %d\n", h.ChunksExpected)
		fmt.Fprintf(&b, "**Chunks-Saved**: %d\n", h.ChunksSaved)
		if len(h.ChunkStatus) > 0 {
			b.WriteString("**Chunk-Status**:\n")
			for _, line := range h.ChunkStatus {
				fmt.Fprintf(&b, "- %s\n", line)
			}
		}
	}
	return b.String()
}

var headerFieldRE = regexp.MustCompile(`^\*\*([A-Za-z0-9-]+)\*\*:\s?(.*)$`)

// ParseHeader reads a rendered provenance block back out of content, so a
// re-ingestion can compare digests without re-deriving them. It returns
// false if content contains no recognizable block.
func ParseHeader(content string) (Header, bool) {
	block := content
	if idx := strings.LastIndex(content, "\n---\n"); idx >= 0 {
		block = content[idx+1:]
	} else if !strings.HasPrefix(content, "---\n") {
		return Header{}, false
	}

	var h Header
	found := false
	lines := strings.Split(block, "\n")
	inChunkStatus := false
	for _, line := range lines {
		if line == "---" {
			continue
		}
		if inChunkStatus && strings.HasPrefix(line, "- ") {
			h.ChunkStatus = append(h.ChunkStatus, strings.TrimPrefix(line, "- "))
			continue
		}
		inChunkStatus = false

		m := headerFieldRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		found = true
		key, val := m[1], m[2]
		switch key {
		case "Source-Kind":
			h.SourceKind = SourceKind(val)
		case "Source-URI":
			h.SourceURI = val
		case "Source-Id":
			h.SourceID = val
		case "Source-Title":
			h.SourceTitle = val
		case "Source-SHA256":
			h.SourceSHA256 = val
		case "Content-SHA256":
			h.ContentSHA256 = val
		case "Ingested-At":
			if t, err := time.Parse(time.RFC3339, val); err == nil {
				h.IngestedAt = t
			}
		case "Ingested-By":
			h.IngestedBy = val
		case "Chunk":
			fmt.Sscanf(val, "%d/%d", &h.ChunkN, &h.ChunkTotal)
		case "Chunk-Span":
			fmt.Sscanf(val, "%d-%d", &h.ChunkSpanStart, &h.ChunkSpanEnd)
		case "Chunk-Path":
			h.ChunkPath = val
		case "Split":
			h.Split = SplitKind(val)
		case "Status":
			h.Status = Status(val)
		case "Chunks-Expected":
			h.IsManifest = true
			fmt.Sscanf(val, "%d", &h.ChunksExpected)
		case "Chunks-Saved":
			h.IsManifest = true
			fmt.Sscanf(val, "%d", &h.ChunksSaved)
		case "Chunk-Status":
			h.IsManifest = true
			inChunkStatus = true
		}
	}
	if !found {
		return Header{}, false
	}
	return h, true
}

// NormalizeSlug is the single definition of slug normalization for
// ingestion identity: lowercase ASCII, runs outside [a-z0-9] collapsed to
// one '-', leading/trailing '-' trimmed, truncated at the last '-'
// boundary at or before limit (or hard-cut at limit if no such boundary
// exists within it). limit <= 0 disables truncation.
func NormalizeSlug(s string, limit int) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case b.Len() > 0 && !lastDash:
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.TrimRight(b.String(), "-")
	if limit > 0 && len(out) > limit {
		cut := out[:limit]
		if idx := strings.LastIndexByte(cut, '-'); idx >= 0 {
			cut = cut[:idx]
		}
		out = cut
	}
	return out
}

// canonicalOrigin derives the canonical origin string and the "human part"
// used for the readable slug prefix, per the contract's per-kind
// derivation table. The canonical string is never a function of content.
func canonicalOrigin(o Origin) (canonical, human string, err error) {
	switch o.Kind {
	case KindURL:
		return canonicalURL(o.URI)
	case KindFile, KindDirectory:
		if o.URI == "" {
			return "", "", errors.New("ingest: file/directory origin requires URI")
		}
		abs, err := filepath.Abs(o.URI)
		if err != nil {
			return "", "", err
		}
		canonical = filepath.Clean(abs)
		base := filepath.Base(canonical)
		human = strings.TrimSuffix(base, filepath.Ext(base))
		return canonical, human, nil
	case KindPasted:
		if o.Label == "" {
			return "", "", errors.New("ingest: pasted origin requires a Label")
		}
		return "pasted:" + o.Label, o.Label, nil
	default:
		return "", "", fmt.Errorf("ingest: unknown source kind %q", o.Kind)
	}
}

// canonicalURL implements the url row of the Source-Id derivation table:
// lowercase scheme+host, default port stripped, fragment stripped, query
// kept, trailing '/' stripped unless the path is exactly "/".
func canonicalURL(raw string) (canonical, human string, err error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", err
	}
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	if port := u.Port(); port != "" && !isDefaultPort(scheme, port) {
		host = host + ":" + port
	}
	path := u.Path
	if path != "/" {
		path = strings.TrimRight(path, "/")
	}
	canonical = scheme + "://" + host + path
	if u.RawQuery != "" {
		canonical += "?" + u.RawQuery
	}
	human = strings.ToLower(u.Hostname()) + path
	return canonical, human, nil
}

func isDefaultPort(scheme, port string) bool {
	switch scheme {
	case "http":
		return port == "80"
	case "https":
		return port == "443"
	}
	return false
}

// SourceID derives the origin-only identity described in the contract:
// Source-Id = NormalizeSlug(<human part>)[:40] + "-" + sha256(<canonical origin>)[:8].
// It never reads content — only o.Title changing must never change the id.
func SourceID(o Origin) (string, error) {
	canonical, human, err := canonicalOrigin(o)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(canonical))
	slug := NormalizeSlug(human, 40)
	return slug + "-" + hex.EncodeToString(sum[:])[:8], nil
}
