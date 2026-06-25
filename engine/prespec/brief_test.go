package prespec

import (
	"bytes"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

// ulidRe matches the Crockford base32 uppercase ULID format (R-014).
var ulidRe = regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{26}$`)

// fixedTime is the anchor for golden-file tests so output is deterministic.
var fixedTime = time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

// zeroBits is an io.Reader that always returns zero bytes — produces a
// predictable ULID random component for golden tests.
type zeroBits struct{}

func (z zeroBits) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

// TestNewIDFormat verifies the generated ULID matches R-014 format exactly.
func TestNewIDFormat(t *testing.T) {
	id := NewID()
	if !ulidRe.MatchString(id) {
		t.Errorf("NewID() = %q; does not match ULID regex %s", id, ulidRe)
	}
	if len(id) != 26 {
		t.Errorf("NewID() length = %d; want 26", len(id))
	}
}

// TestNewIDFromDeterministic verifies NewIDFrom with fixed seed produces a
// stable, decodable ULID for golden-file tests.
func TestNewIDFromDeterministic(t *testing.T) {
	id := NewIDFrom(fixedTime, zeroBits{})
	if !ulidRe.MatchString(id) {
		t.Errorf("NewIDFrom() = %q; does not match ULID regex", id)
	}
	// Calling again with the same seed must produce the same value.
	id2 := NewIDFrom(fixedTime, zeroBits{})
	if id != id2 {
		t.Errorf("NewIDFrom is not deterministic: %q != %q", id, id2)
	}
}

// TestNewIDUniqueness verifies two real NewID() calls produce different values
// (crypto/rand makes collision astronomically unlikely).
func TestNewIDUniqueness(t *testing.T) {
	a, b := NewID(), NewID()
	if a == b {
		t.Errorf("NewID() produced identical values: %q", a)
	}
}

// TestTopicKeyFormat verifies TopicKey returns the correct path.
func TestTopicKeyFormat(t *testing.T) {
	b := Brief{DiscoveryID: "01HXXXXXXXXXXXXXXXXXXXXXX", Project: "my-project"}
	key := b.TopicKey()
	want := "project/my-project/prespec/01HXXXXXXXXXXXXXXXXXXXXXX"
	if key != want {
		t.Errorf("TopicKey() = %q; want %q", key, want)
	}
}

// TestTopicKeySDDPanic verifies TopicKey panics when Project starts with "sdd/" (R-017).
func TestTopicKeySDDPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for sdd/ prefixed project; got none")
		}
	}()
	b := Brief{DiscoveryID: "01HXXXXXXXXXXXXXXXXXXXXXX", Project: "sdd/some-change"}
	_ = b.TopicKey()
}

// TestTopicKeyEmptyProjectPanic verifies TopicKey panics when Project is empty (W-3).
// Without this guard an empty Project silently produces the malformed key
// "project//prespec/<ULID>".
func TestTopicKeyEmptyProjectPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for empty Project; got none")
		}
	}()
	b := Brief{DiscoveryID: "01HXXXXXXXXXXXXXXXXXXXXXX", Project: ""}
	_ = b.TopicKey()
}

// TestValidatePassesGate verifies Validate returns nil when readiness passes.
func TestValidatePassesGate(t *testing.T) {
	cells := make([]Cell, 10)
	// 6 Clear cells → score 0.6 → passes gate.
	for i := range cells {
		cells[i].State = Missing
	}
	for i := 0; i < 6; i++ {
		cells[i].State = Clear
	}
	b := Brief{
		DiscoveryID: NewIDFrom(fixedTime, zeroBits{}),
		Project:     "test-project",
		Job:         "Get thing done",
		Sections:    [6]string{"s1", "s2", "s3", "s4", "s5", "s6"},
		Transcript:  "Q: what? A: stuff",
	}
	if err := b.Validate(cells); err != nil {
		t.Errorf("Validate() unexpected error: %v", err)
	}
}

// TestValidateFailsGate verifies Validate returns an error when readiness fails.
func TestValidateFailsGate(t *testing.T) {
	cells := make([]Cell, 10)
	// 0 Clear cells → score 0.0 → fails gate.
	b := Brief{
		DiscoveryID: NewIDFrom(fixedTime, zeroBits{}),
		Project:     "test-project",
		Job:         "Get thing done",
		Sections:    [6]string{"s1", "s2", "s3", "s4", "s5", "s6"},
		Transcript:  "Q: what?",
	}
	if err := b.Validate(cells); err == nil {
		t.Error("Validate() should fail when readiness score is below gate")
	}
}

// TestValidateRequiresJob verifies Validate returns an error when Job is empty.
func TestValidateRequiresJob(t *testing.T) {
	cells := make([]Cell, 10)
	for i := 0; i < 6; i++ {
		cells[i].State = Clear
	}
	b := Brief{
		DiscoveryID: NewIDFrom(fixedTime, zeroBits{}),
		Project:     "test-project",
		Job:         "", // empty
		Sections:    [6]string{"s1", "s2", "s3", "s4", "s5", "s6"},
		Transcript:  "Q: what?",
	}
	if err := b.Validate(cells); err == nil {
		t.Error("Validate() should fail when Job is empty")
	}
}

// TestValidateRequiresTranscript verifies Validate returns an error when Transcript is empty.
func TestValidateRequiresTranscript(t *testing.T) {
	cells := make([]Cell, 10)
	for i := 0; i < 6; i++ {
		cells[i].State = Clear
	}
	b := Brief{
		DiscoveryID: NewIDFrom(fixedTime, zeroBits{}),
		Project:     "test-project",
		Job:         "Do something",
		Sections:    [6]string{"s1", "s2", "s3", "s4", "s5", "s6"},
		Transcript:  "", // empty
	}
	if err := b.Validate(cells); err == nil {
		t.Error("Validate() should fail when Transcript is empty")
	}
}

// TestValidateRequiresProject verifies Validate returns an error when Project is empty (W-3).
func TestValidateRequiresProject(t *testing.T) {
	cells := make([]Cell, 10)
	for i := 0; i < 6; i++ {
		cells[i].State = Clear
	}
	b := Brief{
		DiscoveryID: NewIDFrom(fixedTime, zeroBits{}),
		Project:     "", // empty
		Job:         "Do something",
		Sections:    [6]string{"s1", "s2", "s3", "s4", "s5", "s6"},
		Transcript:  "Q: what? A: this.",
	}
	if err := b.Validate(cells); err == nil {
		t.Error("Validate() should fail when Project is empty")
	}
}

// TestRenderBriefGolden verifies RenderBrief output against a golden file.
// Run with -update to regenerate the golden file.
func TestRenderBriefGolden(t *testing.T) {
	id := NewIDFrom(fixedTime, zeroBits{})
	b := Brief{
		DiscoveryID: id,
		Project:     "my-project",
		CreatedAt:   fixedTime,
		Job:         "Ship faster without breaking things",
		Sections: [6]string{
			"Engineers need to deploy 10×/day but manual QA is the bottleneck.",
			"Current pipeline takes 3h; teams want under 15 min.",
			"Hiring freeze means we can't add QA headcount.",
			"Incremental automated coverage with human sign-off on critical paths.",
			"The current CI system already supports parallelism.",
			"Reduced deploy cycle time and zero Sev-1 regressions post-deploy.",
		},
		Transcript: "Q: What is the main obstacle? A: Manual QA takes 3 hours.",
	}

	got := RenderBrief(b)

	const goldenPath = "testdata/brief.golden"
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(goldenPath, []byte(got), 0644); err != nil {
			t.Fatalf("writing golden file: %v", err)
		}
		t.Logf("golden file updated: %s", goldenPath)
		return
	}

	raw, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("golden file not found (%s); run with UPDATE_GOLDEN=1 to create it", goldenPath)
	}

	want := string(raw)
	if got != want {
		// Show a compact diff (line-by-line).
		gotLines := strings.Split(got, "\n")
		wantLines := strings.Split(want, "\n")
		var diff bytes.Buffer
		maxLen := len(gotLines)
		if len(wantLines) > maxLen {
			maxLen = len(wantLines)
		}
		for i := 0; i < maxLen; i++ {
			gl, wl := "", ""
			if i < len(gotLines) {
				gl = gotLines[i]
			}
			if i < len(wantLines) {
				wl = wantLines[i]
			}
			if gl != wl {
				diff.WriteString("line " + string(rune('0'+i+1)) + ":\n")
				diff.WriteString("  got:  " + gl + "\n")
				diff.WriteString("  want: " + wl + "\n")
			}
		}
		t.Errorf("RenderBrief output does not match golden file.\nDiff:\n%s\nFull got:\n%s", diff.String(), got)
	}
}
