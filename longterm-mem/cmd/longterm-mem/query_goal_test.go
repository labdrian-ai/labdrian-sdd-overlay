package main

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vault"
)

// goalQueryDatabase is an Engram database of four observations of project goalproj that all match "needle": one of
// goal g-42, one of goal g-7, one with an ordinary topic and one with none.
func goalQueryDatabase(t *testing.T) string {
	t.Helper()
	schema, err := os.ReadFile(filepath.Join("..", "..", "internal", "engram", "testdata", "schema.sql"))
	if err != nil {
		t.Fatalf("read the engram schema fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "engram.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open the fixture database: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatalf("apply the schema: %v", err)
	}
	for _, row := range []struct {
		title string
		topic any
	}{
		{"mine", "goal:g-42/architecture"},
		{"theirs", "goal:g-7/architecture"},
		{"plain", "architecture/auth"},
		{"untopiced", nil},
	} {
		if _, err := db.Exec(`INSERT INTO observations (session_id, type, title, content, project, topic_key) VALUES (?, ?, ?, ?, ?, ?)`,
			"sess-1", "decision", row.title, "a needle in "+row.title, "goalproj", row.topic); err != nil {
			t.Fatalf("insert %s: %v", row.title, err)
		}
	}
	return path
}

// `query --goal` keeps that goal's records and the records of no goal and leaves out another goal's; without
// --goal every record is returned, as before.
func TestQueryGoalFlag(t *testing.T) {
	dbPath := goalQueryDatabase(t)
	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{
		{"under a goal", []string{"query", "--project", "goalproj", "--goal", "g-42", "--json", "needle"}, []string{"mine", "plain", "untopiced"}},
		{"without a goal", []string{"query", "--project", "goalproj", "--json", "needle"}, []string{"mine", "plain", "theirs", "untopiced"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("LONGTERM_MEM_ENGRAM_DB", dbPath)
			t.Setenv("LONGTERM_MEM_VAULT", t.TempDir())
			t.Setenv("LONGTERM_MEM_VAULTS_FILE", "")
			t.Chdir(t.TempDir())

			code, stdout, stderr := runCaptured(t, tc.args)
			if code != exitOK {
				t.Fatalf("exit %d, stderr:\n%s", code, stderr)
			}
			var out struct {
				Results []struct {
					Title string `json:"title"`
				} `json:"results"`
			}
			if err := json.Unmarshal([]byte(stdout), &out); err != nil {
				t.Fatalf("decode the result: %v\n%s", err, stdout)
			}
			var titles []string
			for _, r := range out.Results {
				titles = append(titles, r.Title)
			}
			slices.Sort(titles)
			if !reflect.DeepEqual(titles, tc.want) {
				t.Fatalf("titles = %q, want %q", titles, tc.want)
			}
		})
	}
}

// The query's dependencies read a vault page's tags through the vault adapter over the same vault root the
// retriever runs in.
func TestQueryDepsReadVaultPageTagsFromTheVaultRoot(t *testing.T) {
	deps := queryDeps(nil, "/some/vault", queryWiring{})
	if got, want := deps.VaultTags, (vault.PageTagReader{Root: "/some/vault"}); got != want {
		t.Fatalf("VaultTags = %#v, want %#v", got, want)
	}
}
