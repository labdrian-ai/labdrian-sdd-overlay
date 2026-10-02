package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/shaper"
)

// freshShaperAssessment evaluates the worktree's sources the way 'shaper assess' does,
// with no clearance, and returns the assessment lookUpClearance is asked about.
func freshShaperAssessment(t *testing.T, root string) shaper.Assessment {
	t.Helper()
	o, err := parseShaperArgs(assessArgs(root)[1:], true, false)
	if err != nil {
		t.Fatalf("parseShaperArgs: %v", err)
	}
	in, _, err := loadShaperInput(o)
	if err != nil {
		t.Fatalf("loadShaperInput: %v", err)
	}
	a := shaper.Evaluate(in, nil)
	if a.Subject == nil {
		t.Fatalf("no clearance subject: blockers %+v", a.Blockers)
	}
	return a
}

// The lookup reports a clearance the store does not hold as absent, and one the store
// could not read as unavailable, and it never takes the second for the first: an
// unreadable store must not look like a clearance nobody has given. The classification
// rests on the port's shaper.ErrClearanceNotFound, so each case is one way the store
// answers.
func TestLookUpClearanceTellsAnAbsentClearanceFromAnUnavailableOne(t *testing.T) {
	root, state := shaperWorktree(t, shaperTestHandoff, shaperTestGoal("standalone-shaper-handoff", `["Extract jsonstrict."]`))
	a := freshShaperAssessment(t, root)
	s := a.Subject
	dir := filepath.Join(state, "labdrian", "shaper-clearance", s.ProjectID, s.GoalID)
	record := filepath.Join(dir, s.HandoffSHA256+".json")

	for _, tc := range []struct {
		name string
		// arrange leaves the store in one state; it may change the environment.
		arrange func(t *testing.T)
		status  string
		// detail is a text the report's detail must contain; "" means it must be empty.
		detail string
		// path is whether the report names where the record is, or would be, kept.
		path bool
	}{
		{"no store at all", func(t *testing.T) {}, "absent", "", true},
		{"the directories without the record", func(t *testing.T) {
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
		}, "absent", "", true},
		{"a directory where the record belongs", func(t *testing.T) {
			if err := os.MkdirAll(record, 0o700); err != nil {
				t.Fatal(err)
			}
		}, "unavailable", "is not a regular file", true},
		{"a symlink where the record belongs", func(t *testing.T) {
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(t.TempDir(), "elsewhere.json")
			if err := os.WriteFile(target, []byte("{}"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, record); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
		}, "unavailable", "refusing symlinked record", true},
		{"a state home that is a file", func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "state")
			if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("XDG_STATE_HOME", file)
		}, "unavailable", "not a directory", true},
		{"an environment that names no usable state home", func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", "not/absolute")
		}, "unavailable", "clearance store: ", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Each case starts from an empty store.
			if err := os.RemoveAll(filepath.Join(state, "labdrian")); err != nil {
				t.Fatal(err)
			}
			tc.arrange(t)

			vc, report := lookUpClearance(a)
			if vc != nil {
				t.Fatalf("lookUpClearance returned a verified clearance %+v from a store with no record", vc)
			}
			if report.Status != tc.status {
				t.Errorf("status = %q (detail %q), want %q", report.Status, report.Detail, tc.status)
			}
			switch {
			case tc.detail == "" && report.Detail != "":
				t.Errorf("detail = %q, want none for an absent clearance", report.Detail)
			case !strings.Contains(report.Detail, tc.detail):
				t.Errorf("detail = %q, want it to contain %q", report.Detail, tc.detail)
			}
			// Under a state home of its own the record is kept elsewhere, so the path is
			// checked by the key's place in the store, not by the whole of it.
			keyed := strings.HasSuffix(report.Path, filepath.Join("shaper-clearance", s.ProjectID, s.GoalID, s.HandoffSHA256+".json"))
			if tc.path != keyed {
				t.Errorf("path = %q, want a path to the key's record: %v", report.Path, tc.path)
			}
		})
	}
}

// A subject that could not be derived has no key to look a clearance up by, which is
// absent, with the reason; it never reaches the store.
func TestLookUpClearanceWithoutASubjectIsAbsent(t *testing.T) {
	_, report := lookUpClearance(shaper.Assessment{})
	if report.Status != "absent" || !strings.Contains(report.Detail, "subject evidence is incomplete or refused") || report.Path != "" {
		t.Errorf("lookUpClearance without a subject = %+v, want absent with the reason and no path", report)
	}
}

// The largest record the verb accepts is a record the store can load back: a record of
// exactly shaper.MaxRecordBytes is stored by the verb and 'shaper assess' then finds it
// verified, and one byte more is refused in the bound's words with nothing written. The
// verb reads stdin through the domain's bound, not through a cap of its own.
func TestShaperClearanceRecord_TheLargestRecordItAcceptsIsOneTheStoreLoadsBack(t *testing.T) {
	root, stateHome := shaperWorktree(t, shaperTestHandoff, shaperTestGoal("standalone-shaper-handoff", `["Extract jsonstrict."]`))
	a := decodeAssess(t, runShaperTest(assessArgs(root), ""))
	if len(a.Flags) != 1 {
		t.Fatalf("flags %+v, want exactly one to resolve", a.Flags)
	}
	// padded is a valid record of exactly size bytes: the one resolution's reason grows.
	padded := func(size int) string {
		base := len(recordFromAssess(t, a, "affirm", "tui", nil))
		return recordFromAssess(t, a, "affirm", "tui", map[string]any{"flag_resolutions": []map[string]string{{
			"flag_id":  a.Flags[0].ID,
			"reason":   "Intended." + strings.Repeat("x", size-base),
			"evidence": "Read the view.",
		}}})
	}

	over := padded(shaper.MaxRecordBytes + 1)
	if len(over) != shaper.MaxRecordBytes+1 {
		t.Fatalf("padded record is %d bytes, want %d", len(over), shaper.MaxRecordBytes+1)
	}
	r := runShaperTest(recordArgs(root, "--stdin"), over)
	if want := fmt.Sprintf("stdin exceeds %d bytes", shaper.MaxRecordBytes); r.code != 1 || !strings.Contains(r.stderr, want) {
		t.Fatalf("a record one byte over the bound: exit %d stderr %.200q, want a refusal %q", r.code, r.stderr, want)
	}
	if files := listFiles(t, stateHome); len(files) != 0 {
		t.Fatalf("a refused record wrote %v", files)
	}

	exact := padded(shaper.MaxRecordBytes)
	if len(exact) != shaper.MaxRecordBytes {
		t.Fatalf("padded record is %d bytes, want %d", len(exact), shaper.MaxRecordBytes)
	}
	if r := runShaperTest(recordArgs(root, "--stdin"), exact); r.code != 0 {
		t.Fatalf("a record of exactly the bound: exit %d stderr %.200q", r.code, r.stderr)
	}
	got := decodeAssess(t, runShaperTest(assessArgs(root), ""))
	if got.Clearance.Status != "verified" {
		t.Errorf("clearance %+v, want the stored record of exactly the bound verified", got.Clearance)
	}
}
