package memory_test

import (
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// A topic belongs to goal <id> only when it begins with `goal:<id>/`; the id ends at the first `/` after `goal:`
// (ADR C23 M5, decision 1). Every other topic belongs to no goal.
func TestGoalOfTopic(t *testing.T) {
	for _, tt := range []struct {
		name, topic, wantID string
		wantOK              bool
	}{
		{name: "a goal's topic", topic: "goal:g-42/architecture/auth", wantID: "g-42", wantOK: true},
		{name: "the id ends at the first slash", topic: "goal:g-42/x/goal:other/y", wantID: "g-42", wantOK: true},
		{name: "a goal's topic with nothing after the slash", topic: "goal:g-42/", wantID: "g-42", wantOK: true},
		{name: "an empty topic", topic: "", wantOK: false},
		{name: "an ordinary topic", topic: "architecture/auth", wantOK: false},
		{name: "goal: with no slash", topic: "goal:g-42", wantOK: false},
		{name: "goal: with an empty id", topic: "goal:/architecture", wantOK: false},
		{name: "goal: in the middle", topic: "architecture/goal:g-42/auth", wantOK: false},
		{name: "a different case", topic: "Goal:g-42/auth", wantOK: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			id, ok := memory.GoalOfTopic(tt.topic)
			if id != tt.wantID || ok != tt.wantOK {
				t.Fatalf("GoalOfTopic(%q) = (%q, %v), want (%q, %v)", tt.topic, id, ok, tt.wantID, tt.wantOK)
			}
		})
	}
}

// The goal prefix only says which goal a record is of: the topic under it is the topic every other rule reads.
func TestTopicWithoutGoal(t *testing.T) {
	for _, tt := range []struct{ name, topic, want string }{
		{name: "a goal's topic", topic: "goal:g-42/sdd/x", want: "sdd/x"},
		{name: "a goal's topic with nothing after the slash", topic: "goal:g-42/", want: ""},
		{name: "an ordinary topic", topic: "architecture/auth", want: "architecture/auth"},
		{name: "goal: with no slash", topic: "goal:g-42", want: "goal:g-42"},
		{name: "goal: with an empty id", topic: "goal:/sdd/x", want: "goal:/sdd/x"},
		{name: "an empty topic", topic: "", want: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := memory.TopicWithoutGoal(tt.topic); got != tt.want {
				t.Fatalf("TopicWithoutGoal(%q) = %q, want %q", tt.topic, got, tt.want)
			}
		})
	}
}

// Under a goal a record is visible when it is that goal's or no goal's; without a goal every record is.
func TestTopicVisibleUnderGoal(t *testing.T) {
	for _, tt := range []struct {
		name, topic, goal string
		want              bool
	}{
		{name: "that goal's record", topic: "goal:g-42/auth", goal: "g-42", want: true},
		{name: "a record with no goal", topic: "architecture/auth", goal: "g-42", want: true},
		{name: "a record with no topic", topic: "", goal: "g-42", want: true},
		{name: "a malformed goal topic belongs to no goal", topic: "goal:g-7", goal: "g-42", want: true},
		{name: "another goal's record", topic: "goal:g-7/auth", goal: "g-42", want: false},
		{name: "a goal whose id is a prefix of the record's", topic: "goal:g-421/auth", goal: "g-42", want: false},
		{name: "a goal with a slash matches no goal's record", topic: "goal:a/b/c", goal: "a/b", want: false},
		{name: "no goal sees another goal's record", topic: "goal:g-7/auth", goal: "", want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := memory.TopicVisibleUnderGoal(tt.topic, tt.goal); got != tt.want {
				t.Fatalf("TopicVisibleUnderGoal(%q, %q) = %v, want %v", tt.topic, tt.goal, got, tt.want)
			}
		})
	}
}

// A page's tags carry its goal as `goal:<id>`; a page is visible under a goal when its tags name that goal or name
// no goal at all. An empty `goal:` tag names no goal, as an empty id in a topic does.
func TestTagsVisibleUnderGoal(t *testing.T) {
	for _, tt := range []struct {
		name string
		tags []string
		goal string
		want bool
	}{
		{name: "that goal's page", tags: []string{"memory", "decision", "goal:g-42"}, goal: "g-42", want: true},
		{name: "a page with no goal tag", tags: []string{"memory", "decision"}, goal: "g-42", want: true},
		{name: "a page with no tags", tags: nil, goal: "g-42", want: true},
		{name: "a page with an empty goal tag", tags: []string{"goal:"}, goal: "g-42", want: true},
		{name: "another goal's page", tags: []string{"memory", "goal:g-7"}, goal: "g-42", want: false},
		{name: "a page of two goals, one of them this", tags: []string{"goal:g-7", "goal:g-42"}, goal: "g-42", want: true},
		{name: "no goal sees every page", tags: []string{"goal:g-7"}, goal: "", want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := memory.TagsVisibleUnderGoal(tt.tags, tt.goal); got != tt.want {
				t.Fatalf("TagsVisibleUnderGoal(%q, %q) = %v, want %v", tt.tags, tt.goal, got, tt.want)
			}
		})
	}
}

// The tag a promoted page of goal <id> carries.
func TestGoalTag(t *testing.T) {
	if got := memory.GoalTag("g-42"); got != "goal:g-42" {
		t.Fatalf("GoalTag(g-42) = %q, want goal:g-42", got)
	}
}
