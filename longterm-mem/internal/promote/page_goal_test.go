package promote

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

func goalTestObservation(topic string) memory.Observation {
	return memory.Observation{
		ID: 200, SyncID: "sync-200", Type: "architecture", Title: "Read-Only Store",
		Content: "The store opens Engram read-only.", Project: "labdrian-sdd-overlay",
		RevisionCount: 2, TopicKey: topic,
	}
}

func emitGoalTestPage(t *testing.T, topic string) string {
	t.Helper()
	page, err := EmitPage(goalTestObservation(topic), "c-000200", []Link{{Address: "c-000099", Title: "Other Page"}}, time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("EmitPage: %v", err)
	}
	return page.Frontmatter + page.Body
}

// An observation whose topic carries the goal prefix is promoted with the tag goal:<id> after memory and its type
// (ADR C23 M5, decision 2); nothing else of the page changes.
func TestEmitPage_AGoalsTopicAddsTheGoalTag(t *testing.T) {
	got := emitGoalTestPage(t, "goal:g-42/architecture/store")
	want := strings.Replace(emitGoalTestPage(t, ""), "  - \"architecture\"\n", "  - \"architecture\"\n  - \"goal:g-42\"\n", 1)
	if got != want {
		t.Fatalf("page of a goal's observation:\n%s\nwant:\n%s", got, want)
	}
	if !strings.Contains(got, "tags:\n  - \"memory\"\n  - \"architecture\"\n  - \"goal:g-42\"\n") {
		t.Fatalf("tags are not memory, the type and the goal, in that order:\n%s", got)
	}
}

// An observation with no goal prefix in its topic -- an ordinary topic, a malformed goal topic, or none -- renders
// the page it rendered before goals, byte for byte (the golden).
func TestEmitPage_ATopicWithoutTheGoalPrefixRendersThePageAsBefore(t *testing.T) {
	want, err := os.ReadFile(filepath.Join("testdata", "pages", "architecture.golden.md"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	for _, topic := range []string{"", "architecture/store", "goal:g-42", "goal:/x", "x/goal:g-42/y"} {
		if got := emitGoalTestPage(t, topic); got != string(want) {
			t.Errorf("topic %q rendered:\n%s\nwant the golden:\n%s", topic, got, want)
		}
	}
}
