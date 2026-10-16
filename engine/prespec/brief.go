package prespec

import (
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

// NewIDFrom generates a ULID from the given timestamp and random source. The
// domain takes both as values and reads neither the clock nor the system entropy
// itself: the composition root (engine/cmd) supplies time.Now and crypto/rand.
// The random source is an io.Reader (injectable for deterministic tests).
// Format: 10-char timestamp ‖ 16-char random, all Crockford base32 uppercase,
// 26 chars total — structurally impossible to confuse with a kebab slug (R-012, R-014).
//
// The timestamp is the low 48 bits of the milliseconds since the Unix epoch, written
// most significant bit first in ten characters of five bits (the last two bits of the
// tenth are zero); the random part is ten bytes read from rnd, 80 bits in sixteen
// characters.
func NewIDFrom(ts time.Time, rnd io.Reader) string {
	ms := uint64(ts.UnixMilli()) //nolint:gosec // milliseconds fit in uint64
	var tsBuf [8]byte
	binary.BigEndian.PutUint64(tsBuf[:], ms)

	var rndBuf [10]byte
	if _, err := io.ReadFull(rnd, rndBuf[:]); err != nil {
		// minimal: forced — ADR-3 requires zero external deps; panic is the
		// only option when the random source is genuinely broken.
		panic(fmt.Sprintf("prespec: ULID random source failed: %v", err))
	}

	var id [26]byte
	encodeBase32(id[:10], tsBuf[2:]) // the low 6 of the 8 bytes: 48 bits
	encodeBase32(id[10:], rndBuf[:])
	return string(id[:])
}

// encodeBase32 writes src into dst as Crockford base32, most significant bit first, five
// bits to a character. When the bits of src are not a multiple of five the last character
// is padded with zero bits. dst must be as long as the characters src comes to.
func encodeBase32(dst, src []byte) {
	var window uint16 // the bits read and not yet written are its low `pending` bits
	pending, n := 0, 0
	for _, b := range src {
		window = window<<8 | uint16(b)
		pending += 8
		for pending >= 5 {
			pending -= 5
			dst[n] = crockfordAlphabet[(window>>uint(pending))&0x1F]
			n++
		}
	}
	if pending > 0 {
		dst[n] = crockfordAlphabet[(window<<uint(5-pending))&0x1F]
	}
}

// Section keys for the six discovery sections (in order).
const (
	SectionJobStatement            = 0
	SectionCurrentGap              = 1
	SectionWhyNow                  = 2
	SectionUsersStakeholders       = 3
	SectionConstraintsAlternatives = 4
	SectionAssumptions             = 5
)

// Brief is the structured discovery output produced after a completed interview.
// DiscoveryID (not ChangeName) is the unique identifier — R-012 forbids ChangeName
// because a brief may cover multiple future changes.
// Spec field mapping: Job→Job statement summary, Sections[0..5]→JobStatement/CurrentGap/WhyNow/UsersStakeholders/ConstraintsAlternatives/Assumptions.
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
	"1. Job Statement",
	"2. Current Gap & Problem Evidence",
	"3. Why Now",
	"4. Users & Stakeholders",
	"5. Constraints & Alternatives",
	"6. Assumptions",
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
