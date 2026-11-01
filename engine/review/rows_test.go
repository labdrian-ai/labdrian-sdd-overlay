package review_test

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/jsonstrict"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
)

// rowsFile is roles/reviewer/rows.json, the canonical file of the lens rows (own-rdd S8): the one place where
// the sentences of the rows are written, and the one the parent of the chain copies them from into the prompt
// of the reviewer.
type rowsFile struct {
	Rows []struct {
		ID       string `json:"id"`
		Sentence string `json:"sentence"`
	} `json:"rows"`
}

// readRowsFile reads the canonical file the way the files of the repository are read: one JSON object, no key
// written twice and no member nobody declared.
func readRowsFile(t *testing.T) rowsFile {
	t.Helper()
	const path = "../../roles/reviewer/rows.json"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the canonical file of the lens rows is not there: %v", err)
	}
	var file rowsFile
	if err := jsonstrict.CheckNoDuplicateKeys(data); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	if err := jsonstrict.DecodeStrict(data, path, &file); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return file
}

// The sentences of the rows live once, in the canonical file, and the program holds the ids of the rows it
// accepts: the two agree in the ids and in their order, so a row written in one and not in the other is found
// here and not by a launch that was refused, or accepted, for a reason nobody can see.
func TestTheRowsTheProgramAcceptsAreTheRowsOfTheCanonicalFile(t *testing.T) {
	file := readRowsFile(t)
	var ids []string
	for _, row := range file.Rows {
		ids = append(ids, row.ID)
		if row.Sentence == "" || strings.TrimSpace(row.Sentence) != row.Sentence || strings.ContainsAny(row.Sentence, "\r\n") {
			t.Errorf("row %q has the sentence %q, want text on one line with no space around it, for the parent to copy", row.ID, row.Sentence)
		}
	}

	if want := []string{"default", "readability", "risk-line", "hunt"}; !reflect.DeepEqual(ids, want) {
		t.Errorf("the rows of the canonical file are %q, want the three rows that own-rdd S8 names and the hunt row of phase9-c25-review-hunt S5: %q", ids, want)
	}
	if got := review.RowIDs(); !reflect.DeepEqual(got, ids) {
		t.Errorf("RowIDs() = %q, want the ids of the canonical file, in its order: %q", got, ids)
	}
}

// A launch declares the rows it was sent as ids separated by commas, and the declaration is read in the order
// it is written.
func TestParseRowsReadsTheRowsOfALaunchInTheOrderTheyAreWritten(t *testing.T) {
	for _, tc := range []struct {
		declared string
		want     []string
	}{
		{"default", []string{"default"}},
		{"default,readability", []string{"default", "readability"}},
		{"readability,default", []string{"readability", "default"}},
		{"default,readability,risk-line", []string{"default", "readability", "risk-line"}},
		{"default,hunt", []string{"default", "hunt"}},
		{"risk-line", []string{"risk-line"}},
	} {
		if got, err := review.ParseRows(tc.declared); err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("ParseRows(%q) = %q, %v, want %q and no error", tc.declared, got, err, tc.want)
		}
	}
}

// What is not a list of the rows of the canonical file is refused, and the refusal says what is accepted: an
// empty declaration, an id nobody wrote, a row sent twice and any spelling the grammar does not have.
func TestParseRowsRefusesWhatIsNotAListOfTheRowsOfTheCanonicalFile(t *testing.T) {
	accepted := strings.Join(review.RowIDs(), ", ")
	for _, declared := range []string{
		"", "nope", "default,nope", "default,default", "default,readability,default",
		"default, readability", "hunt,hunt", " default", "default ", "default,", ",default", "default,,readability", "Default", "default;readability",
	} {
		got, err := review.ParseRows(declared)
		if !errors.Is(err, review.ErrInvalidRows) || got != nil {
			t.Errorf("ParseRows(%q) = %q, %v, want no rows and an error wrapping ErrInvalidRows", declared, got, err)
			continue
		}
		if !strings.Contains(err.Error(), accepted) || !strings.Contains(err.Error(), fmt.Sprintf("%q", declared)) {
			t.Errorf("ParseRows(%q) says %q, want it to name the declaration and the rows that are accepted (%s)", declared, err, accepted)
		}
	}
}

// The table of the rows that are accepted is the program's: a caller that changes the slice it was handed does
// not change which rows are accepted.
func TestRowIDsHandsOutACopy(t *testing.T) {
	got := review.RowIDs()
	got[0] = "changed"

	if again := review.RowIDs(); again[0] == "changed" {
		t.Errorf("RowIDs() = %q after a caller changed an earlier result, want the table untouched", again)
	}
}
