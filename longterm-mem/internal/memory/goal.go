package memory

import "strings"

// GoalPrefix opens both spellings of a goal: a topic_key of a record created under goal <id> begins with
// GoalPrefix + <id> + "/" (ADR C23 M5, decision 1), and a promoted page of that record carries the tag
// GoalPrefix + <id> (decision 2).
const GoalPrefix = "goal:"

// GoalOfTopic reports the goal a topic_key belongs to: <id> when the topic begins with `goal:<id>/`, the id ending
// at the first "/" after the prefix. Any other topic -- empty, an ordinary one, `goal:` with no "/" after it, or
// an empty id -- belongs to no goal. This is the one rule of what a topic says about a goal; every reader of a
// topic (the lexical search's SQL mirrors it, and its test holds the two together) asks it.
func GoalOfTopic(topic string) (id string, ok bool) {
	rest, found := strings.CutPrefix(topic, GoalPrefix)
	if !found {
		return "", false
	}
	id, _, slash := strings.Cut(rest, "/")
	if !slash || id == "" {
		return "", false
	}
	return id, true
}

// TopicWithoutGoal is topic with its goal prefix taken off: the prefix says only which goal a record is of, so
// every other rule of a topic (promotion's curated topics, the skill candidates) reads what follows it. A topic of
// no goal is returned as it is.
func TopicWithoutGoal(topic string) string {
	if _, ok := GoalOfTopic(topic); !ok {
		return topic
	}
	_, rest, _ := strings.Cut(topic, "/")
	return rest
}

// GoalTag is the tag a promoted page of goal id carries.
func GoalTag(id string) string { return GoalPrefix + id }

// goalOfTag reports the goal a page tag names: <id> for `goal:<id>` with a non-empty id. An empty `goal:` names no
// goal, as an empty id in a topic does.
func goalOfTag(tag string) (string, bool) {
	id, found := strings.CutPrefix(tag, GoalPrefix)
	if !found || id == "" {
		return "", false
	}
	return id, true
}

// TopicVisibleUnderGoal reports whether a record with topic is visible to a query under goal: every record is when
// goal is empty; under a goal, that goal's records and the records of no goal are, and another goal's are not.
func TopicVisibleUnderGoal(topic, goal string) bool {
	if goal == "" {
		return true
	}
	id, ok := GoalOfTopic(topic)
	return !ok || id == goal
}

// TagsVisibleUnderGoal reports whether a page with tags is visible to a query under goal: every page is when goal
// is empty; under a goal, a page whose tags name that goal, or name no goal at all, is.
func TagsVisibleUnderGoal(tags []string, goal string) bool {
	if goal == "" {
		return true
	}
	namesAGoal := false
	for _, tag := range tags {
		id, ok := goalOfTag(tag)
		if !ok {
			continue
		}
		if id == goal {
			return true
		}
		namesAGoal = true
	}
	return !namesAGoal
}
