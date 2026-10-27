package main

// What `review record` does with the lens rows of a launch (own-rdd T5, S8): --rows names the rows of
// roles/reviewer/rows.json that the reviewer was sent, the stored result records them, and a declaration the
// program cannot read is refused before anything is stored. The tests run the verb over a repository they make in a
// temporary directory, as the tests of the other review verbs do.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
)

// storedRows are the rows that the stored result called name records.
func (w *reviewWorld) storedRows(name string) []string {
	w.t.Helper()
	data, err := os.ReadFile(filepath.Join(w.results("own-rdd"), name))
	if err != nil {
		w.t.Fatalf("the result is not stored: %v", err)
	}
	var file struct {
		Rows []string `json:"rows"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		w.t.Fatalf("the stored file is not JSON: %v\n%s", err, data)
	}
	return file.Rows
}

// dropRows rewrites the stored result called name without its rows: the file of a launch that was stored before the
// rows were recorded.
func (w *reviewWorld) dropRows(name string) {
	w.t.Helper()
	path := filepath.Join(w.results("own-rdd"), name)
	data, err := os.ReadFile(path)
	if err != nil {
		w.t.Fatalf("the result is not stored: %v", err)
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(data, &members); err != nil {
		w.t.Fatalf("the stored file is not JSON: %v\n%s", err, data)
	}
	delete(members, "rows")
	rewritten, err := json.MarshalIndent(members, "", "  ")
	if err != nil {
		w.t.Fatal(err)
	}
	if err := os.WriteFile(path, append(rewritten, '\n'), 0o644); err != nil {
		w.t.Fatal(err)
	}
}

// The rows the parent of the chain declares are the rows the reviewer was sent, and the stored result records them
// as they were declared, in the order they were declared.
func TestRecordStoresTheRowsOfTheLaunchInTheOrderTheyAreDeclared(t *testing.T) {
	w := newReviewWorld(t)
	b := w.bind()

	_, stderr, exits := w.run(reply(t, b, nil), "record", "--feature", "own-rdd", "--base", w.base, "--head", w.head, "--rows", "readability,default")

	if !equalInts(exits, []int{0}) {
		t.Fatalf("exit calls = %v, stderr = %q; want [0]", exits, stderr)
	}
	if got, want := w.storedRows(w.head[:12]+"-1.json"), []string{"readability", "default"}; !reflect.DeepEqual(got, want) {
		t.Errorf("the stored rows are %q, want the rows that were declared, in order: %q", got, want)
	}
}

// A declaration that is not a list of the rows of the canonical file is an argument the program cannot read: exit 1
// (not the 2 of a reply that was refused), the rows that are accepted named on stderr, and nothing stored.
func TestRecordRefusesRowsThatAreNotRowsAndStoresNothing(t *testing.T) {
	w := newReviewWorld(t)
	ok := reply(t, w.bind(), nil)

	for _, rows := range []string{"nope", "default,default", "default,nope", "default, readability", "Default"} {
		t.Run(rows, func(t *testing.T) {
			stdout, stderr, exits := w.run(ok, "record", "--feature", "own-rdd", "--base", w.base, "--head", w.head, "--rows", rows)

			if stdout != "" || !equalInts(exits, []int{1}) || !strings.HasPrefix(stderr, "error: review record: review: invalid rows: ") {
				t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, an error of review record about the rows, [1]", stdout, stderr, exits)
			}
			if want := strings.Join(review.RowIDs(), ", "); !strings.Contains(stderr, want) {
				t.Errorf("stderr = %q, want it to name the rows that are accepted: %s", stderr, want)
			}
			if _, err := os.Stat(filepath.Join(w.root, "odd")); err == nil {
				t.Error("rows that were refused made the results directory")
			}
		})
	}
}
