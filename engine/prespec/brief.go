package prespec

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// crockfordAlphabet is the Crockford base32 character set (uppercase, R-014).
// Excludes I, L, O, U to avoid ambiguity.
const crockfordAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// NewIDFrom generates a ULID from the given timestamp and random source.
// The random source is an io.Reader (injectable for deterministic tests).
// Format: 10-char timestamp ‖ 16-char random, all Crockford base32 uppercase,
// 26 chars total — structurally impossible to confuse with a kebab slug (R-012, R-014).
func NewIDFrom(ts time.Time, rnd io.Reader) string {
	// Timestamp: 48-bit milliseconds since Unix epoch, encoded in 10 base32 chars.
	ms := uint64(ts.UnixMilli()) //nolint:gosec // milliseconds fit in uint64
	var tsBuf [8]byte
	binary.BigEndian.PutUint64(tsBuf[:], ms)
	// Use the low 6 bytes (48 bits) from position 2..7.
	ts6 := tsBuf[2:]

	// Random: 10 bytes (80 bits) → encoded in 16 base32 chars.
	var rndBuf [10]byte
	if _, err := io.ReadFull(rnd, rndBuf[:]); err != nil {
		// minimal: forced — ADR-3 requires zero external deps; panic is the
		// only option when the random source is genuinely broken.
		panic(fmt.Sprintf("prespec: ULID random source failed: %v", err))
	}

	// Encode: concatenate ts6 (6 bytes) + rndBuf (10 bytes) = 16 bytes total.
	// 16 bytes = 128 bits; we need 130 bits for 26×5-bit chars; pad the first
	// 5-bit group from the high side of ts6.
	//
	// Standard ULID encoding packs 128 bits into 26 base32 chars (5 bits each).
	// Layout (bit positions, MSB first):
	//   chars 0-9  : 48-bit timestamp (ts6, 6 bytes)
	//   chars 10-25: 80-bit random    (rndBuf, 10 bytes)
	//
	// We encode by shifting through a 128-bit window the same way the spec does.
	var result [26]byte

	// Encode timestamp (48 bits → 10 chars, 5 bits each).
	result[0] = crockfordAlphabet[(ts6[0]>>3)&0x1F]
	result[1] = crockfordAlphabet[((ts6[0]<<2)|(ts6[1]>>6))&0x1F]
	result[2] = crockfordAlphabet[(ts6[1]>>1)&0x1F]
	result[3] = crockfordAlphabet[((ts6[1]<<4)|(ts6[2]>>4))&0x1F]
	result[4] = crockfordAlphabet[((ts6[2]<<1)|(ts6[3]>>7))&0x1F]
	result[5] = crockfordAlphabet[(ts6[3]>>2)&0x1F]
	result[6] = crockfordAlphabet[((ts6[3]<<3)|(ts6[4]>>5))&0x1F]
	result[7] = crockfordAlphabet[ts6[4]&0x1F]
	result[8] = crockfordAlphabet[(ts6[5]>>3)&0x1F]
	result[9] = crockfordAlphabet[(ts6[5]<<2)&0x1F]

	// Encode random (80 bits → 16 chars, 5 bits each).
	r := rndBuf[:]
	result[10] = crockfordAlphabet[(r[0]>>3)&0x1F]
	result[11] = crockfordAlphabet[((r[0]<<2)|(r[1]>>6))&0x1F]
	result[12] = crockfordAlphabet[(r[1]>>1)&0x1F]
	result[13] = crockfordAlphabet[((r[1]<<4)|(r[2]>>4))&0x1F]
	result[14] = crockfordAlphabet[((r[2]<<1)|(r[3]>>7))&0x1F]
	result[15] = crockfordAlphabet[(r[3]>>2)&0x1F]
	result[16] = crockfordAlphabet[((r[3]<<3)|(r[4]>>5))&0x1F]
	result[17] = crockfordAlphabet[r[4]&0x1F]
	result[18] = crockfordAlphabet[(r[5]>>3)&0x1F]
	result[19] = crockfordAlphabet[((r[5]<<2)|(r[6]>>6))&0x1F]
	result[20] = crockfordAlphabet[(r[6]>>1)&0x1F]
	result[21] = crockfordAlphabet[((r[6]<<4)|(r[7]>>4))&0x1F]
	result[22] = crockfordAlphabet[((r[7]<<1)|(r[8]>>7))&0x1F]
	result[23] = crockfordAlphabet[(r[8]>>2)&0x1F]
	result[24] = crockfordAlphabet[((r[8]<<3)|(r[9]>>5))&0x1F]
	result[25] = crockfordAlphabet[r[9]&0x1F]

	return string(result[:])
}

// NewID generates a fresh ULID using the current UTC time and crypto/rand.
func NewID() string {
	return NewIDFrom(time.Now().UTC(), rand.Reader)
}

// Section keys for the six discovery sections (in order).
const (
	SectionProblemStatement = 0
	SectionOutcomeGap       = 1
	SectionConstraints      = 2
	SectionHypothesis       = 3
	SectionContext          = 4
	SectionSuccessSignal    = 5
)

// Brief is the structured discovery output produced after a completed interview.
// DiscoveryID (not ChangeName) is the unique identifier — R-012 forbids ChangeName
// because a brief may cover multiple future changes.
// Spec field mapping: JobStatement→Job, Section1..Section6→Sections[0..5].
type Brief struct {
	// DiscoveryID is the hand-rolled ULID that uniquely identifies this brief (R-014).
	DiscoveryID string
	// Project is the project key used to construct the Engram topic key.
	Project string
	// CreatedAt is the wall-clock time of brief generation.
	CreatedAt time.Time
	// Job is a one-sentence summary of the job-to-be-done (Section 1 header).
	Job string
	// Sections holds the six discovery sections in order.
	Sections [6]string
	// Transcript is the sanitised conversation transcript appended to the brief.
	Transcript string
}

// TopicKey returns the Engram key for this brief: project/{project}/prespec/{ULID}.
// Panics if Project is empty or starts with "sdd/" to enforce the namespace guard (R-017).
// An empty Project would silently produce the malformed key "project//prespec/<ULID>".
func (b Brief) TopicKey() string {
	if strings.TrimSpace(b.Project) == "" {
		panic("prespec: Brief.Project must not be empty")
	}
	if strings.HasPrefix(b.Project, "sdd/") {
		panic(fmt.Sprintf("prespec: Brief.Project must not start with 'sdd/'; got %q", b.Project))
	}
	return fmt.Sprintf("project/%s/prespec/%s", b.Project, b.DiscoveryID)
}

// Validate checks the brief is ready for persistence.
// Returns a non-nil error if the readiness gate fails or required fields are empty.
func (b Brief) Validate(cells []Cell) error {
	score := Readiness(cells)
	if !score.Passes() {
		return fmt.Errorf(
			"prespec: readiness score %.2f is below gate %.2f; interview is not complete",
			score.Value, ReadinessGate,
		)
	}
	if strings.TrimSpace(b.Project) == "" {
		return errors.New("prespec: Brief.Project is required")
	}
	if strings.TrimSpace(b.Job) == "" {
		return errors.New("prespec: Brief.Job is required")
	}
	if strings.TrimSpace(b.Transcript) == "" {
		return errors.New("prespec: Brief.Transcript is required")
	}
	return nil
}

// sectionHeaders are the human-readable names for the six discovery sections.
var sectionHeaders = [6]string{
	"1. Problem Statement",
	"2. Outcome Gap",
	"3. Constraints",
	"4. Hypothesis",
	"5. Context",
	"6. Success Signal",
}

// RenderBrief produces the canonical Markdown representation of a Brief (R-011, R-018).
// The output is deterministic given the same input — safe for golden-file tests.
func RenderBrief(b Brief) string {
	var sb strings.Builder

	// Header block.
	sb.WriteString("# Discovery Brief\n\n")
	sb.WriteString(fmt.Sprintf("**ID**: %s  \n", b.DiscoveryID))
	sb.WriteString(fmt.Sprintf("**Project**: %s  \n", b.Project))
	sb.WriteString(fmt.Sprintf("**Created**: %s  \n", b.CreatedAt.UTC().Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("**Job**: %s\n\n", b.Job))

	// Six discovery sections.
	for i, header := range sectionHeaders {
		sb.WriteString(fmt.Sprintf("## %s\n\n", header))
		content := b.Sections[i]
		if strings.TrimSpace(content) == "" {
			content = "_not captured_"
		}
		sb.WriteString(content)
		sb.WriteString("\n\n")
	}

	// Transcript.
	sb.WriteString("## Interview Transcript\n\n")
	sb.WriteString("```\n")
	sb.WriteString(b.Transcript)
	sb.WriteString("\n```\n")

	return sb.String()
}
