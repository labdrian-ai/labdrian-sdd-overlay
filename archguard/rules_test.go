package archguard

import "testing"

// importable is the table the checker consults, and it consults it only for a ring
// it judges. A row for a ring that is never judged is dead data that reads as a
// rule (it once listed what test support "may import" although test support has no
// rule), and a judged ring without a row would be refused everything.
func TestImportableHasARowForEveryJudgedRingAndForNoOther(t *testing.T) {
	for _, r := range []Ring{Domain, Application, Adapter, Root, Support} {
		_, hasRow := importable[r]
		if hasRow != r.judged() {
			t.Errorf("%s: row in importable = %v, but judged = %v; the table must hold exactly the judged rings", r, hasRow, r.judged())
		}
	}
}

func TestOnlyTestSupportIsNotJudged(t *testing.T) {
	for _, r := range []Ring{Domain, Application, Adapter, Root} {
		if !r.judged() {
			t.Errorf("%s is not judged, want every production ring judged", r)
		}
	}
	if Support.judged() {
		t.Error("support is judged, want test support to have no rule")
	}
}
