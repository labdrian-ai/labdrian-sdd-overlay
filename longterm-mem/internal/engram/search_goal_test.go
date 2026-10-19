package engram

import (
	"database/sql"
	"slices"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// insertGoalSearchRow inserts one fixture row with caller-controlled content and topic_key (NULL when topic is not
// valid), so a search under a goal can be told which rows it should keep.
func insertGoalSearchRow(t *testing.T, dbPath, title, content string, topic sql.NullString) {
	t.Helper()
	setup, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open fixture setup connection: %v", err)
	}
	defer setup.Close()
	if _, err := setup.Exec(
		`INSERT INTO observations (session_id, type, title, content, project, topic_key) VALUES (?, ?, ?, ?, ?, ?)`,
		"sess-1", "discovery", title, content, "p", topic,
	); err != nil {
		t.Fatalf("insert fixture observation %q: %v", title, err)
	}
}

func openGoalFixture(t *testing.T, rows map[string]sql.NullString) *Store {
	t.Helper()
	dbPath := newFixtureDB(t, t.TempDir())
	titles := make([]string, 0, len(rows))
	for title := range rows {
		titles = append(titles, title)
	}
	slices.Sort(titles)
	for _, title := range titles {
		insertGoalSearchRow(t, dbPath, title, "needle in "+title, rows[title])
	}
	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func titlesOf(rows []memory.Row) []string {
	titles := make([]string, len(rows))
	for i, r := range rows {
		titles[i] = r.Title
	}
	slices.Sort(titles)
	return titles
}

func topic(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }

// Under a goal the lexical search keeps that goal's records and the records of no goal, and leaves out another
// goal's; the SQL that does it says what memory.TopicVisibleUnderGoal says, for every spelling of a topic.
func TestSearch_UnderAGoalKeepsThatGoalAndNoGoal(t *testing.T) {
	rows := map[string]sql.NullString{
		"this goal":        topic("goal:g-42/architecture"),
		"this goal bare":   topic("goal:g-42/"),
		"other goal":       topic("goal:g-7/architecture"),
		"prefix goal":      topic("goal:g-421/architecture"),
		"no goal":          topic("architecture/auth"),
		"null topic":       {},
		"empty topic":      topic(""),
		"goal no slash":    topic("goal:g-7"),
		"goal empty id":    topic("goal:/architecture"),
		"goal in middle":   topic("x/goal:g-7/y"),
		"underscore goal":  topic("goal:g_42/architecture"),
		"percent goal":     topic("goal:g%/architecture"),
		"slashed goal arg": topic("goal:a/b/c"),
	}
	store := openGoalFixture(t, rows)

	for _, goal := range []string{"g-42", "g-7", "a/b", "g_42", "g%"} {
		t.Run(goal, func(t *testing.T) {
			got, err := store.Search("p", "needle", 50, memory.SearchFilter{Goal: goal})
			if err != nil {
				t.Fatalf("Search: %v", err)
			}
			var want []string
			for title, tk := range rows {
				if memory.TopicVisibleUnderGoal(tk.String, goal) {
					want = append(want, title)
				}
			}
			slices.Sort(want)
			if gotTitles := titlesOf(got.Rows); !slices.Equal(gotTitles, want) {
				t.Fatalf("Search under goal %q kept %q, want %q", goal, gotTitles, want)
			}
		})
	}
}

// The goal is applied before the limit, so the rows a filtered search returns are the first `limit` rows of the
// unfiltered bm25 order that the goal keeps, in that order.
func TestSearch_UnderAGoalHoldsTheRankOrderAndTheLimit(t *testing.T) {
	dbPath := newFixtureDB(t, t.TempDir())
	// Relevance falls with each row: more occurrences of the term rank higher under bm25.
	fixtures := []struct {
		title string
		reps  int
		topic sql.NullString
	}{
		{"other best", 6, topic("goal:g-7/a")},
		{"mine second", 5, topic("goal:g-42/a")},
		{"other third", 4, topic("goal:g-7/b")},
		{"plain fourth", 3, sql.NullString{}},
		{"mine fifth", 2, topic("goal:g-42/b")},
	}
	for _, f := range fixtures {
		insertGoalSearchRow(t, dbPath, f.title, strings.Repeat("needle filler ", f.reps)+strings.Repeat("padding ", 10-f.reps), f.topic)
	}
	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	all, err := store.Search("p", "needle", 50, memory.SearchFilter{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	var want []string
	for _, r := range all.Rows {
		if r.Title != "other best" && r.Title != "other third" && len(want) < 2 {
			want = append(want, r.Title)
		}
	}
	got, err := store.Search("p", "needle", 2, memory.SearchFilter{Goal: "g-42"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	var gotTitles []string
	for _, r := range got.Rows {
		gotTitles = append(gotTitles, r.Title)
	}
	if !slices.Equal(gotTitles, want) {
		t.Fatalf("Search under goal g-42, limit 2 = %q, want %q (unfiltered order %q)", gotTitles, want, titlesInOrder(all.Rows))
	}
	if len(all.Rows) != 5 || all.Rows[0].Title != "other best" {
		t.Fatalf("the fixture's bm25 order is not the one the test assumes: %q", titlesInOrder(all.Rows))
	}
}

func titlesInOrder(rows []memory.Row) []string {
	titles := make([]string, len(rows))
	for i, r := range rows {
		titles[i] = r.Title
	}
	return titles
}

// Without a goal the search keeps every goal's records, as it did before goals existed.
func TestSearch_WithoutAGoalKeepsEveryGoal(t *testing.T) {
	store := openGoalFixture(t, map[string]sql.NullString{
		"g42": topic("goal:g-42/a"), "g7": topic("goal:g-7/a"), "none": {},
	})
	got, err := store.Search("p", "needle", 50, memory.SearchFilter{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if titles := titlesOf(got.Rows); !slices.Equal(titles, []string{"g42", "g7", "none"}) {
		t.Fatalf("Search without a goal = %q, want every row", titles)
	}
}
