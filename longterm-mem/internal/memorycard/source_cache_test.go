package memorycard

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

func writeFixture(t *testing.T, path string, statements ...string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("close the fixture: %v", err)
		}
	}()
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
}

// A read of a project's imported observations that fails is reported, and it is not remembered: the next call
// reads again. The part number of an import_map row that is not a number makes the read fail here (the row names
// memory 1, which is in project p, so the read of p's imports takes it); the listing, which never reads it, does not.
// The failure must be that read's: an error of any other kind would not show that a failed read is not remembered.
func TestSourceReadsTheImportsAgainAfterAReadThatFailed(t *testing.T) {
	path := kinFixture(t)
	writeFixture(t, path, `insert into import_map values ('engram', '990', 'x', 1)`)
	source := openSource(t, path)

	m, ok, err := source.ByID("22")
	if err == nil {
		t.Fatalf("ByID(22) with an unreadable import_map row = %+v, %v, nil error, want an error", m, ok)
	}
	if want := `read an import_map row of project "p"`; !strings.Contains(err.Error(), want) {
		t.Fatalf("ByID(22) failed with %q, want the error of the read of the imports: %q", err, want)
	}
	writeFixture(t, path, `delete from import_map where source_key = '990'`)

	want := []memory.Identity{card("19"), card("20"), card("21"), engramObservation("700")}
	if got := memoryOf(t, source, "22").Mirrored; !reflect.DeepEqual(got, want) {
		t.Errorf("mirrors of memory 22 after the failed read = %v, want %v", got, want)
	}
}

// A Source may be shared by goroutines (run with -race): every reader of the same listing gets the same memories.
func TestSourceIsSafeToShareAcrossGoroutines(t *testing.T) {
	path := kinFixture(t)
	// The answer comes from another Source, so that the shared one is cold when the goroutines start.
	want, err := openSource(t, path).Memories("p")
	if err != nil {
		t.Fatal(err)
	}
	source := openSource(t, path)

	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := source.Memories("p")
			if err == nil && !reflect.DeepEqual(got, want) {
				err = fmt.Errorf("a shared Source listed %+v, want %+v", got, want)
			}
			if _, _, byIDErr := source.ByID("22"); err == nil {
				err = byIDErr
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("a shared Source answered differently or failed: %v", err)
		}
	}
}
