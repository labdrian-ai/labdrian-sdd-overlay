package skills

// A nil BaselineLookup is the fixed baseline, in the domain functions as well as in the use cases
// that call OrFixed: a caller that names no baseline must not crash the judgment of an approval.

import (
	"errors"
	"io/fs"
	"reflect"
	"testing"
)

func TestANilBaselineJudgesAnApprovalAsTheFixedBaselineDoes(t *testing.T) {
	inBaseline := ApprovalBaseline()[0].ID
	for _, id := range []string{inBaseline, "not-in-the-baseline"} {
		t.Run(id, func(t *testing.T) {
			skillMD := []byte("bytes that are not the pinned ones\n")
			status := ApprovalStatus{State: ApprovalAbsent}

			got := EvaluateApprovalAgainst(nil, id, "records/"+id+".json", skillMD, status)

			want := EvaluateApprovalAgainst(FixedBaseline, id, "records/"+id+".json", skillMD, status)
			if got.OK || !reflect.DeepEqual(got, want) {
				t.Errorf("EvaluateApprovalAgainst(nil) = %+v, want %+v", got, want)
			}
		})
	}
}

func TestANilBaselineChecksTheApprovalsOfARegistryAsTheFixedBaselineDoes(t *testing.T) {
	baselineID := ApprovalBaseline()[0].ID
	reg := Registry{Skills: []Entry{
		{ID: baselineID, Path: baselineID, Install: Install{DefaultScope: "global"}},
		{ID: "outside", Path: "outside", Install: Install{DefaultScope: "global"}},
	}}
	records := recordsAbsent{func(string) ([]byte, error) { return []byte("not the pinned bytes\n"), nil }}

	gotDivs, gotSum := CheckApprovalsAgainst(nil, reg, "src", records)

	wantDivs, wantSum := CheckApprovalsAgainst(FixedBaseline, reg, "src", records)
	if len(gotDivs) == 0 || !reflect.DeepEqual(gotDivs, wantDivs) || gotSum != wantSum {
		t.Errorf("CheckApprovalsAgainst(nil) = %v, %+v; want %v, %+v", gotDivs, gotSum, wantDivs, wantSum)
	}
}

// recordsAbsent has a SKILL.md for every skill and a record for none.
type recordsAbsent struct{ read FileReader }

func (r recordsAbsent) ReadSkill(sourceRoot, path string) ([]byte, error) {
	return r.read(SkillMDPath(sourceRoot, path))
}

func (recordsAbsent) ReadRecord(string, string) ([]byte, error) { return nil, fs.ErrNotExist }

func TestANilBaselineDecidesALintRefusalAsTheFixedBaselineDoes(t *testing.T) {
	legacy := []error{errors.New("[lint:body-hard-budget] too long")}
	structural := []error{errors.New("[lint:name-missing] no name")}
	for _, id := range []string{ApprovalBaseline()[0].ID, "not-in-the-baseline"} {
		for name, hard := range map[string][]error{"legacy": legacy, "structural": structural} {
			t.Run(id+" "+name, func(t *testing.T) {
				gotW, gotR := BaselineLintDecisionAgainst(nil, id, hard)

				wantW, wantR := BaselineLintDecisionAgainst(FixedBaseline, id, hard)
				if gotR != wantR || !reflect.DeepEqual(gotW, wantW) {
					t.Errorf("BaselineLintDecisionAgainst(nil) = %v, %v; want %v, %v", gotW, gotR, wantW, wantR)
				}
			})
		}
	}
}
