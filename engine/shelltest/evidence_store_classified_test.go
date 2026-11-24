package shelltest

import (
	"testing"
)

// B16: the evidence page may hold a second, optional table under "## Classified without evidence",
// | Review | Date |, for a review that was classified and yielded no evidence row. check holds it; reviewed and
// reviewed-each answer yes for a row of it, on a page that passes check; count, index, rules and rule never read it
// (T4d-b, L98 choice 3).

const (
	classifiedHead = "\n## Classified without evidence\n\n| Review | Date |\n|--------|------|\n"

	classifiedRows = "| f/zzzz-1 | 2026-11-24 |\n| f/spaced-1 |  2026-11-25  |\n"
)

// withClassified is the standard page with the table of classified reviews after the Evidence table.
func withClassified(table string) string {
	return evidencePage(stdKinds, stdEvidence) + table
}

func TestEvidenceStoreHoldsTheTableOfReviewsClassifiedWithoutEvidence(t *testing.T) {
	t.Run("check passes a page with the table and a page without it", func(t *testing.T) {
		wantResult(t, "with the table", newEvidenceWorld(t, withClassified(classifiedHead+classifiedRows)).inRepo("check"), 0, "evidence check passed: docs/wiki/evidence.md\n", "")
		wantResult(t, "without the table", newEvidenceWorld(t, withClassified("")).inRepo("check"), 0, "evidence check passed: docs/wiki/evidence.md\n", "")
		wantResult(t, "with the heading and a header-only table", newEvidenceWorld(t, withClassified(classifiedHead)).inRepo("check"), 0, "evidence check passed: docs/wiki/evidence.md\n", "")
	})

	const fail = "FAIL evidence.md: "
	for _, c := range []struct{ name, table, want string }{
		{"a wrong header", "\n## Classified without evidence\n\n| Review | When |\n|--------|------|\n| f/zzzz-1 | 2026-11-24 |\n",
			fail + "the header row of Classified without evidence is not: | Review|Date |\n"},
		{"a missing separator", "\n## Classified without evidence\n\n| Review | Date |\n| f/zzzz-1 | 2026-11-24 |\n",
			fail + "the second row of Classified without evidence is not the separator row\n"},
		{"a row of 3 cells", classifiedHead + "| f/zzzz-1 | 2026-11-24 | x |\n",
			fail + "a row of Classified without evidence has 3 cells, not 2: f/zzzz-1\n"},
		{"a row of 5 cells, as an Evidence row is written", classifiedHead + "| alpha-kind | 1 | f/zzzz-1 | 2026-11-24 | x |\n",
			fail + "a row of Classified without evidence has 5 cells, not 2: alpha-kind\n"},
		{"a blank review", classifiedHead + "|  | 2026-11-24 |\n",
			fail + "a row of Classified without evidence has no review\n"},
		{"a date 2026/11/19", classifiedHead + "| f/zzzz-1 | 2026/11/19 |\n",
			fail + "classified review \"f/zzzz-1\" has date \"2026/11/19\", not YYYY-MM-DD\n"},
		{"a row after the table ended", classifiedHead + "| f/zzzz-1 | 2026-11-24 |\n\n| f/yyyy-1 | 2026-11-24 |\n",
			fail + "a table row after the table of Classified without evidence ended, by a line that is not a row: | f/yyyy-1 | 2026-11-24 |\n"},
	} {
		t.Run("check fails on "+c.name, func(t *testing.T) {
			wantResult(t, "check", newEvidenceWorld(t, withClassified(c.table)).inRepo("check"), 1, c.want, "")
		})
	}

	t.Run("a row of 5 cells is not read as evidence", func(t *testing.T) {
		page := evidencePage(stdKinds, "") + classifiedHead + "| alpha-kind | 1 | f/zzzz-1 | 2026-11-24 | x |\n"
		wantResult(t, "count", newEvidenceWorld(t, page).inRepo("count"), 0, "", "")
	})

	t.Run("reviewed answers yes for a row of the table and for an Evidence row", func(t *testing.T) {
		w := newEvidenceWorld(t, withClassified(classifiedHead+classifiedRows))
		for _, c := range []struct {
			what, id string
			exit     int
		}{
			{"a review only in the table", "f/zzzz-1", 0},
			{"a review in the table with blanks around its date", "f/spaced-1", 0},
			{"a review in an Evidence row", "f/aaaa-1", 0},
			{"a review no table holds", "f/yyyy-1", 1},
			{"the start of a review of the table", "f/zzzz", 1},
			{"a date of the table", "2026-11-24", 1},
		} {
			wantResult(t, c.what, w.inRepo("reviewed", c.id), c.exit, "", "")
		}
	})

	t.Run("reviewed answers no for a page that fails a check, whatever the table holds", func(t *testing.T) {
		page := withClassified(classifiedHead + "| f/zzzz-1 | 2026/11/19 |\n")
		wantResult(t, "reviewed", newEvidenceWorld(t, page).inRepo("reviewed", "f/zzzz-1"), 1, "", "")
		wantResult(t, "reviewed-each", newEvidenceWorld(t, page).inRepo("reviewed-each", "docs/wiki", "f/zzzz-1", "f/aaaa-1"), 0, "no\nno\n", "")
	})

	t.Run("reviewed-each answers each identifier, in order", func(t *testing.T) {
		w := newEvidenceWorld(t, withClassified(classifiedHead+classifiedRows))
		wantResult(t, "reviewed-each", w.inRepo("reviewed-each", "docs/wiki", "f/zzzz-1", "f/yyyy-1", "f/aaaa-1", "f/spaced-1", "f/zzzz-1"), 0, "yes\nno\nyes\nyes\nyes\n", "")
	})

	t.Run("count, index, rules and rule print the same with and without the table", func(t *testing.T) {
		with := newEvidenceWorld(t, withClassified(classifiedHead+classifiedRows+"| beta-kind | 2026-11-24 |\n"))
		without := newEvidenceWorld(t, withClassified(""))
		for _, args := range [][]string{{"count"}, {"index"}, {"rules"}, {"rule", "beta-kind"}} {
			base := without.inRepo(args...)
			if base.Exit != 0 || base.Stdout == "" {
				t.Fatalf("%v with no table: exit %d, stdout %q, want a non-empty answer to compare with", args, base.Exit, base.Stdout)
			}
			wantResult(t, args[0]+" with the table", with.inRepo(args...), base.Exit, base.Stdout, base.Stderr)
		}
	})
}
