package skills

import (
	"fmt"
	"regexp"
	"strings"
)

// The candidate record is the Engram observation of skills/_shared/procedural-candidate-detection.md
// sections 2 and 9: a block of `**Field**: value` lines, with two lists (Occurrences and History). The
// agent writes it by hand, so a record may be wrong in ways a program would not make it. The parser
// keeps what it can read and reports what is wrong as anomalies; it repairs nothing (ADR C23 M10,
// decision 1).

// Anomaly is one thing wrong with a record, named by the topic key it was read under.
type Anomaly struct {
	Key     string
	Message string
}

// HistoryEntry is one line of a record's History: `<at> | <from> -> <to> | <actor> | <evidence>`.
type HistoryEntry struct {
	At       string
	From, To CandidateStatus
	Actor    Actor
	Evidence string
}

// CandidateRecord is what the engine reads of a candidate's observation.
type CandidateRecord struct {
	// Key is the candidate's topic key without the goal prefix it may have been created under.
	Key string
	// Goal is the goal the record was created under, "" for a record of no goal.
	Goal            string
	Kind            string
	Aliases         []string
	Status          CandidateStatus
	Threshold       int
	OccurrenceCount int
	// Occurrences are the `engram:<id>` references listed, in order.
	Occurrences []string
	Disposition string
	// Draft is the topic key of the candidate's draft, "" when it has none.
	Draft   string
	History []HistoryEntry
}

const goalTopicPrefix = "goal:"

// splitGoalPrefix takes the goal prefix off a topic: `goal:<id>/` with a non-empty id names a goal; any
// other topic is of no goal and is returned as it is. It is the rule longterm-mem's TopicWithoutGoal states
// (the two modules cannot share it), and a test holds the cases of both.
func splitGoalPrefix(topic string) (goal, rest string) {
	after, found := strings.CutPrefix(topic, goalTopicPrefix)
	if !found {
		return "", topic
	}
	id, rest, slash := strings.Cut(after, "/")
	if !slash || id == "" {
		return "", topic
	}
	return id, rest
}

var fieldLine = regexp.MustCompile(`^\*\*([A-Za-z]+)\*\*:[ \t]*(.*)$`)

// recordFields is a record block split into its scalar fields, its lists and the fields given twice.
type recordFields struct {
	scalars map[string]string
	lists   map[string][]string
	twice   []string
}

// readRecordFields reads the `**Name**: value` lines of content. A line that begins with "- " belongs to the
// list of the field before it. Reading stops at the line of `stopAt` (the Body of a draft is not fields).
func readRecordFields(content, stopAt string) recordFields {
	f := recordFields{scalars: map[string]string{}, lists: map[string][]string{}}
	current := ""
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimRight(line, "\r")
		if m := fieldLine.FindStringSubmatch(line); m != nil {
			current = m[1]
			if current == stopAt {
				break
			}
			if _, seen := f.scalars[current]; seen {
				f.twice = append(f.twice, current)
			}
			f.scalars[current] = strings.TrimSpace(m[2])
			continue
		}
		if item, ok := strings.CutPrefix(line, "- "); ok && current != "" {
			f.lists[current] = append(f.lists[current], item)
		}
	}
	return f
}

// ParseCandidateRecord reads the candidate observation stored under topicKey. It always returns the record
// it could read, and the anomalies it found; a record with an anomaly is not repaired.
func ParseCandidateRecord(topicKey, content string) (CandidateRecord, []Anomaly) {
	goal, key := splitGoalPrefix(topicKey)
	rec := CandidateRecord{Key: key, Goal: goal}
	var anomalies []Anomaly
	add := func(format string, args ...any) {
		anomalies = append(anomalies, Anomaly{Key: topicKey, Message: fmt.Sprintf(format, args...)})
	}
	keyKind := ""
	if err := ValidateCandidateKey(key); err != nil {
		add("%v", err)
	} else {
		keyKind = strings.Split(strings.TrimPrefix(key, candidateKeyPrefix), "/")[0]
	}

	f := readRecordFields(content, "")
	for _, name := range f.twice {
		add("the field %s is given twice", name)
	}

	rec.Kind = f.scalars["Kind"]
	switch {
	case rec.Kind == "":
		add("no Kind")
	case keyKind != "" && rec.Kind != keyKind:
		add("Kind %q is not the kind of the key (%s)", rec.Kind, keyKind)
	}
	rec.Disposition = f.scalars["Disposition"]
	rec.Draft = f.scalars["Draft"]
	if aliases := f.scalars["Aliases"]; aliases != "" && aliases != "none" {
		for _, a := range strings.Split(aliases, ";") {
			if a = strings.TrimSpace(a); a != "" {
				rec.Aliases = append(rec.Aliases, a)
			}
		}
	}

	if raw, ok := f.scalars["Status"]; !ok || raw == "" {
		add("no Status")
	} else if status, err := ParseCandidateStatus(raw); err != nil {
		add("%v", err)
	} else {
		rec.Status = status
	}

	rec.Threshold = number(f.scalars, "Threshold", add)
	rec.OccurrenceCount = number(f.scalars, "OccurrenceCount", add)
	distinct := map[string]bool{}
	for _, item := range f.lists["Occurrences"] {
		id, _, _ := strings.Cut(item, " ")
		rec.Occurrences = append(rec.Occurrences, id)
		distinct[id] = true
	}
	if _, stated := f.scalars["OccurrenceCount"]; stated && rec.OccurrenceCount != len(distinct) {
		add("OccurrenceCount is %d but %d distinct occurrences are listed", rec.OccurrenceCount, len(distinct))
	}

	rec.History = readHistory(f.lists["History"], add)
	checkHistory(rec, add)
	return rec, anomalies
}

// number reads a scalar that must be a whole number; an absent field is 0, a field that is not a number is
// an anomaly.
func number(scalars map[string]string, name string, add func(string, ...any)) int {
	raw, ok := scalars[name]
	if !ok || raw == "" {
		return 0
	}
	// Digits only and at most nine of them: engine/skills may not import strconv (zero_fetch_test), and a count or a
	// threshold of a billion is not one an agent wrote.
	n := 0
	for _, r := range raw {
		if r < '0' || r > '9' || len(raw) > 9 {
			add("%s %q is not a whole number", name, raw)
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// readHistory reads the History lines; one that does not have four columns or whose states or actor are not
// of the vocabularies is an anomaly and is left out.
func readHistory(lines []string, add func(string, ...any)) []HistoryEntry {
	var history []HistoryEntry
	for _, line := range lines {
		cols := strings.SplitN(line, " | ", 4)
		if len(cols) != 4 {
			add("History line %q does not have the four columns at | from -> to | actor | evidence", line)
			continue
		}
		fromRaw, toRaw, arrow := strings.Cut(cols[1], " -> ")
		if !arrow {
			add("History line %q does not say `from -> to`", line)
			continue
		}
		from, errFrom := historyState(fromRaw)
		to, errTo := historyState(toRaw)
		actor, errActor := ParseActor(strings.TrimSpace(cols[2]))
		if err := firstError(errFrom, errTo, errActor); err != nil {
			add("History line %q: %v", line, err)
			continue
		}
		history = append(history, HistoryEntry{At: cols[0], From: from, To: to, Actor: actor, Evidence: cols[3]})
	}
	return history
}

func historyState(raw string) (CandidateStatus, error) {
	if raw = strings.TrimSpace(raw); raw == "none" {
		return noStatus, nil
	}
	return ParseCandidateStatus(raw)
}

func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// checkHistory judges the lines read: each is a move of the table, each starts where the one before ended,
// and the last ends at the record's Status.
func checkHistory(rec CandidateRecord, add func(string, ...any)) {
	for i, h := range rec.History {
		if err := CheckTransition(h.From, h.To, h.Actor); err != nil {
			add("History line %d: %v", i+1, err)
		}
		if i > 0 && h.From != rec.History[i-1].To {
			add("History does not chain: line %d starts at %s and line %d ended at %s",
				i+1, statusWord(h.From), i, statusWord(rec.History[i-1].To))
		}
	}
	if n := len(rec.History); n > 0 && rec.Status != "" && rec.History[n-1].To != rec.Status {
		add("History ends at %s but Status is %s", statusWord(rec.History[n-1].To), rec.Status)
	}
}
