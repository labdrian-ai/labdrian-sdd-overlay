package app_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewverdict"
)

const findingsResult = "222222222222-1.json"

// disposed is finding with another causal disposition.
func disposed(id, severity, disposition string) map[string]any {
	f := finding(id, severity)
	f["causal_disposition"] = disposition
	return f
}

// withVerdict stores the verdict of a verifier beside the result called findingsResult.
func withVerdict(t *testing.T, store *fakeResults, reply []byte) {
	t.Helper()
	file, err := review.EncodeVerdict(reply)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	store.put("own-rdd", "222222222222-1.verifier.json", file)
}

func idsOf(f app.Findings) (kept []string, left map[string]string) {
	left = map[string]string{}
	for _, k := range f.Kept {
		kept = append(kept, k.ID)
	}
	for _, l := range f.LeftOut {
		left[l.ID] = l.Reason
	}
	return kept, left
}

// A result with no verdict keeps every finding the diff may have caused, under the identifier of the result, in
// the order the reviewer wrote them, and reads nothing but the files.
func TestFindingsKeepsTheFindingsOfAResultWithNoVerdictUnderItsIdentifier(t *testing.T) {
	g, store := newFakeGit(), newFakeResults()
	name := reviewed(t, store, baseSHA, headSHA, 1, finding("F2", "WARNING"), finding("F1", "BLOCKER"))

	got, err := app.New(g, store).Findings("/work/wt/feature", "own-rdd", name)

	kept, left := idsOf(got)
	if err != nil || got.Review != "own-rdd/222222222222-1" || got.Verdict != "" {
		t.Fatalf("Findings() = review %q, verdict %q, %v; want own-rdd/222222222222-1, no verdict and no error", got.Review, got.Verdict, err)
	}
	if !reflect.DeepEqual(kept, []string{"F2", "F1"}) || len(left) != 0 {
		t.Errorf("kept %v, left out %v; want F2 and F1 kept in the order of the reply, none left out", kept, left)
	}
	if k := got.Kept[1]; k.Severity != "BLOCKER" || k.Location != "engine/guard.go:10" || k.Claim != "the guard is skipped" || k.EvidenceClass != "deterministic" || k.Disposition != "introduced" {
		t.Errorf("kept finding = %+v, want the fields of the reply", k)
	}
	if want := []string{"Locate /work/wt/feature"}; !reflect.DeepEqual(g.calls, want) || store.created() != 0 {
		t.Errorf("git calls %q and %d creates, want only the Locate and no create", g.calls, store.created())
	}
}

// A result with no finding is a listing with nothing in it, not a refusal.
func TestFindingsListsAResultWithNoFindingAsEmpty(t *testing.T) {
	store := newFakeResults()
	name := reviewed(t, store, baseSHA, headSHA, 1)

	got, err := app.New(newFakeGit(), store).Findings("/work/wt/feature", "own-rdd", name)

	if err != nil || len(got.Kept) != 0 || len(got.LeftOut) != 0 {
		t.Errorf("Findings() = %+v, %v; want an empty listing and no error", got, err)
	}
}

// A finding the diff did not cause is not evidence, whatever its severity.
func TestFindingsLeavesOutAFindingThatPreExistsOrIsBaseOnly(t *testing.T) {
	store := newFakeResults()
	name := reviewed(t, store, baseSHA, headSHA, 1,
		disposed("F1", "BLOCKER", "pre-existing"), disposed("F2", "WARNING", "base-only"),
		disposed("F3", "SUGGESTION", "worsened"), disposed("F4", "WARNING", "unknown"), disposed("F5", "WARNING", "behavior-activated"))

	got, err := app.New(newFakeGit(), store).Findings("/work/wt/feature", "own-rdd", name)

	kept, left := idsOf(got)
	wantLeft := map[string]string{"F1": "causal_disposition pre-existing", "F2": "causal_disposition base-only"}
	if err != nil || !reflect.DeepEqual(kept, []string{"F3", "F4", "F5"}) || !reflect.DeepEqual(left, wantLeft) {
		t.Errorf("kept %v, left out %v, %v; want F3, F4 and F5 kept and %v left out", kept, left, err, wantLeft)
	}
}

// A severe finding whose verdict did not reproduce it on head is left out; a verdict that reproduced it or did not
// run keeps it, and a finding that is not severe is never judged by its verdict.
func TestFindingsLeavesOutASevereFindingTheVerdictDidNotReproduce(t *testing.T) {
	store := newFakeResults()
	name := reviewed(t, store, baseSHA, headSHA, 1,
		finding("F1", "BLOCKER"), finding("F2", "CRITICAL"), finding("F3", "CRITICAL"), finding("F4", "BLOCKER"), finding("F5", "WARNING"), finding("F6", "SUGGESTION"))
	withVerdict(t, store, verdictReply(verdict("F1", "not-reproduced"), verdict("F2", "reproduced"), verdict("F3", "not-run"),
		verdict("F4", "not-reproduced"), verdict("F5", "not-reproduced"), verdict("F6", "not-reproduced")))

	got, err := app.New(newFakeGit(), store).Findings("/work/wt/feature", "own-rdd", name)

	kept, left := idsOf(got)
	wantLeft := map[string]string{"F1": "verdict not-reproduced", "F4": "verdict not-reproduced"}
	if err != nil || !reflect.DeepEqual(kept, []string{"F2", "F3", "F5", "F6"}) || !reflect.DeepEqual(left, wantLeft) {
		t.Errorf("kept %v, left out %v, %v; want F2, F3, F5 and F6 kept and %v left out", kept, left, err, wantLeft)
	}
	if got.Verdict != "odd/review-results/own-rdd/222222222222-1.verifier.json" {
		t.Errorf("Verdict = %q, want the stored verdict that was read", got.Verdict)
	}
}

// A finding that breaks both rules is left out once, for the disposition, which is read first.
func TestFindingsNamesOneReasonForAFindingThatBreaksBothRules(t *testing.T) {
	store := newFakeResults()
	name := reviewed(t, store, baseSHA, headSHA, 1, disposed("F1", "BLOCKER", "pre-existing"))
	withVerdict(t, store, verdictReply(verdict("F1", "not-reproduced")))

	got, err := app.New(newFakeGit(), store).Findings("/work/wt/feature", "own-rdd", name)

	_, left := idsOf(got)
	if err != nil || len(got.Kept) != 0 || len(got.LeftOut) != 1 || left["F1"] != "causal_disposition pre-existing" {
		t.Errorf("Findings() = %+v, %v; want F1 left out once, for its disposition", got, err)
	}
}

// What cannot be read is refused with the reason and never listed as empty: a result that is not stored or is not
// one, and a verdict that is stored and does not hold to its result.
func TestFindingsRefusesWhatItCannotReadAndNeverListsItAsEmpty(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(store *fakeResults)
		want  error
	}{
		{"the result is not stored", func(*fakeResults) {}, app.ErrNotFound},
		{"the result is not a stored file", func(s *fakeResults) { s.put("own-rdd", findingsResult, []byte("not a result")) }, review.ErrInvalidStored},
		{"the result records no rows", func(s *fakeResults) {
			s.put("own-rdd", findingsResult, storedWithoutRows(t, candidateOf(t, newFakeGit())))
		}, review.ErrInvalidStored},
		{"the verdict leaves a finding without an answer", func(s *fakeResults) {
			reviewed(t, s, baseSHA, headSHA, 1, finding("F1", "BLOCKER"), finding("F2", "BLOCKER"))
			withVerdict(t, s, verdictReply(verdict("F1", "reproduced")))
		}, reviewverdict.ErrUnanswered},
		{"the verdict is not JSON", func(s *fakeResults) {
			reviewed(t, s, baseSHA, headSHA, 1, finding("F1", "BLOCKER"))
			s.put("own-rdd", "222222222222-1.verifier.json", []byte("nope"))
		}, reviewverdict.ErrUnparseable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeResults()
			tc.setup(store)

			got, err := app.New(newFakeGit(), store).Findings("/work/wt/feature", "own-rdd", findingsResult)

			if !errors.Is(err, tc.want) || !strings.Contains(err.Error(), "222222222222-1") || len(got.Kept) != 0 || len(got.LeftOut) != 0 {
				t.Errorf("Findings() = %+v, %v; want a refusal that is %v, names the file and lists nothing", got, err, tc.want)
			}
		})
	}
}

// A feature or a name that is not safe is refused before git or the store is asked for anything, and the name of a
// verdict is not the name of a result.
func TestFindingsRefusesAnUnsafeFeatureOrNameBeforeAskingAnyone(t *testing.T) {
	for _, tc := range []struct {
		feature, stored string
		want            error
	}{
		{"../x", findingsResult, review.ErrInvalidFeature},
		{"own-rdd", "../222222222222-1.json", review.ErrInvalidResultName},
		{"own-rdd", "222222222222-1.verifier.json", review.ErrInvalidResultName},
	} {
		g, store := newFakeGit(), newFakeResults()

		_, err := app.New(g, store).Findings("/work/wt/feature", tc.feature, tc.stored)

		if !errors.Is(err, tc.want) || len(g.calls) != 0 || len(store.calls) != 0 {
			t.Errorf("Findings(%q, %q) error = %v with git calls %q and store calls %q, want one that is %v and no call", tc.feature, tc.stored, err, g.calls, store.calls, tc.want)
		}
	}
}
