// Package vecindex implements longterm-mem's embedding index over Engram's
// own observations for one project (R-066): a fixed-stride vector blob plus
// a self-digesting JSON manifest, stored under
// <state-dir>/index/<project-dir>/, never inside the vault and never as a
// write against Engram (R-067). It never talks to a network itself -- the
// caller supplies vectors through the Embedder seam (internal/embed.Client
// in production) -- and it is not read by anything until a later PR of
// union-retrieval wires the embedding query arm.
package vecindex

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// EmbedInput builds the exact text embedded for one observation: title, a
// NUL separator, then content truncated to inputLimit characters.
//
// This shape is measured, not assumed (design §0.5 / phase0.md's arm-D
// reproduction): a newline separator instead of NUL routes worse. Both
// Fingerprint and the production embedding call site (a later PR's
// embedding-arm and the incremental builder here) must build the embedded
// text through this one function, so the shape can never silently drift
// between what was measured and what ships.
func EmbedInput(title, content string, inputLimit int) string {
	truncated := content
	if inputLimit >= 0 && len(truncated) > inputLimit {
		truncated = truncated[:inputLimit]
	}
	return title + "\x00" + truncated
}

// Fingerprint hashes the embedding contract together with the exact bytes
// that were embedded: model, dimension, and input_limit are inside the
// hash, not recorded beside it, so changing the model, its dimension, or
// the truncation limit invalidates every fingerprint mechanically rather
// than by anyone remembering to bump a version field.
//
// Content beyond inputLimit is never hashed (via EmbedInput): a row edited
// only past that point keeps a valid fingerprint and a valid vector, and
// that is correct under R-066/R-068 -- the embedding never saw that part of
// the content in the first place.
func Fingerprint(model string, dim, inputLimit int, title, content string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%d\x00%d\x00", model, dim, inputLimit)
	fmt.Fprint(h, EmbedInput(title, content, inputLimit))
	return hex.EncodeToString(h.Sum(nil))
}
