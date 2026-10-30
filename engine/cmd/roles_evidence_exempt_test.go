package main

// The evidence that `roles append` holds a review-exemption to (own-rdd T4, S5, L16). The skeleton `review exempt`
// prints, once the parent of the chain has filled the members only it knows, is a record `roles append` stores when
// its evidence is the exemption the verb stored, and refuses when it is not. The tests run the program over a
// repository they make in a temporary directory (reviewWorld) and a role chain store under a temporary state home
// (rolesTestStateHome), as the tests of the evidence of a review-report do.

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sha256hex"
)

// printedExemption exempts the head of the world on a clear report and returns the skeleton `review exempt` printed,
// with from_role as the command leaves it: empty, for the parent to fill.
func (w *reviewWorld) printedExemption() roles.RoleHandoff {
	w.t.Helper()
	stdout, stderr, exits := w.exempt(verifierReport, "none")
	if !equalInts(exits, []int{0}) {
		w.t.Fatalf("setup: review exempt exit calls = %v, stderr = %q; want [0]", exits, stderr)
	}
	return decodeSkeleton(w.t, stdout)
}

// exemptedSkeleton is that skeleton with from_role filled as the parent of a chain that is at the reviewer fills it
// (the chain of reviewerChain), so a test of the evidence appends a record the chain accepts.
func (w *reviewWorld) exemptedSkeleton() roles.RoleHandoff {
	w.t.Helper()
	h := w.printedExemption()
	h.FromRole = roles.RoleReviewer
	return h
}

// chainAtSweeper stores the three records that put the work of the chain in the hands of the sweeper (the reviewer
// never ran) and returns the digest the record that follows them chains to.
func chainAtSweeper(t *testing.T, check evidenceCheck, chain string) string {
	t.Helper()
	prev := roles.EmptyChainDigest
	for i, hop := range [][2]roles.Role{{roles.RoleShaper, roles.RoleEstimator}, {roles.RoleEstimator, roles.RoleBuilder}, {roles.RoleBuilder, roles.RoleSweeper}} {
		raw := rolesRecordJSON(t, roles.RoleHandoff{
			Version: roles.HandoffVersion, ProjectID: rolesProject, GoalID: rolesGoal, ChainID: chain, Seq: i + 1,
			FromRole: hop[0], ToRole: hop[1], PrevSHA256: prev, PayloadKind: "diff", PayloadSHA256: strings.Repeat("a", 64),
			Evidence: []roles.Evidence{}, Context: roles.Context{Summary: "s", Decisions: []string{}, OpenQuestions: []string{}},
			Status: roles.StatusCompleted,
		})
		if r := runRolesChecked(check, rolesAppendArgs(chain), raw); r.code != 0 {
			t.Fatalf("setup: append of seq %d: code=%d stderr=%q", i+1, r.code, r.stderr)
		}
		prev = sha256hex.Sum([]byte(raw))
	}
	return prev
}

// An exemption closes a head the reviewer never saw: the skeleton `review exempt` prints leaves from_role to the
// parent, and once the parent fills it with the role the chain is at (here the sweeper) `roles append` stores the
// record and the chain ends at the delivery. The reviewer is not the role the chain is at, so the same record with
// from_role reviewer is refused by the chain and nothing is stored.
func TestRolesAppendStoresAnExemptionFromTheChainsRoleToTheDelivery(t *testing.T) {
	rolesTestStateHome(t)
	w := newCheckedWorld(t)
	check := reviewEvidenceCheck(w.rolesDeps())
	printed := w.printedExemption()
	if printed.FromRole != "" {
		t.Fatalf("setup: the skeleton has from_role %q, want it empty", printed.FromRole)
	}

	refused := printed
	refused.FromRole = roles.RoleReviewer
	prev := chainAtSweeper(t, check, "chain-2")
	r := runRolesChecked(check, rolesAppendArgs("chain-2"), rolesRecordJSON(t, fillSkeleton(refused, "chain-2", prev)))
	if want := "from_role \"reviewer\" must equal seq 3 to_role \"sweeper\""; r.code != 1 || r.stdout != "" || !strings.Contains(r.stderr, want) {
		t.Errorf("code = %d, stdout = %q, stderr = %q; want exit 1, nothing on stdout, and %q", r.code, r.stdout, r.stderr, want)
	}
	if at := rolesAt(t, "chain-2"); at != string(roles.RoleSweeper) {
		t.Errorf("the chain is at %q, want the sweeper: the refused record must not be stored", at)
	}

	stored := printed
	stored.FromRole = roles.RoleSweeper
	prev = chainAtSweeper(t, check, "chain-1")
	r = runRolesChecked(check, rolesAppendArgs("chain-1"), rolesRecordJSON(t, fillSkeleton(stored, "chain-1", prev)))
	if r.code != 0 || r.stderr != "" || !strings.Contains(r.stdout, "roles append: stored record at") {
		t.Fatalf("code = %d, stdout = %q, stderr = %q; want the exemption stored", r.code, r.stdout, r.stderr)
	}
	if at := rolesAt(t, "chain-1"); at != string(roles.RoleDelivery) {
		t.Errorf("the chain is at %q, want the delivery: the exemption is the last record", at)
	}
}

// The record `review exempt` prints is a record `roles append` stores: the evidence it carries is the exemption the
// verb stored, and the record goes from the reviewer to the delivery.
func TestRolesAppendStoresTheRecordThatReviewExemptPrints(t *testing.T) {
	rolesTestStateHome(t)
	w := newCheckedWorld(t)
	check := reviewEvidenceCheck(w.rolesDeps())
	record := w.exemptedSkeleton()
	var kinds []string
	for _, e := range record.Evidence {
		kinds = append(kinds, e.Kind)
	}
	if !review.NeedsEvidence(record) || !slices.Equal(kinds, []string{review.EvidenceDiff, review.EvidenceExemption}) {
		t.Fatalf("setup: the skeleton is a record with evidence %q (held to evidence: %v), want a completed review-exemption with review-diff and review-exemption", kinds, review.NeedsEvidence(record))
	}
	prev := reviewerChain(t, check, "chain-1")

	r := runRolesChecked(check, rolesAppendArgs("chain-1"), rolesRecordJSON(t, fillSkeleton(record, "chain-1", prev)))

	if r.code != 0 || r.stderr != "" || !strings.Contains(r.stdout, "roles append: stored record at") {
		t.Fatalf("code = %d, stdout = %q, stderr = %q; want the record stored", r.code, r.stdout, r.stderr)
	}
	if at := rolesAt(t, "chain-1"); at != string(roles.RoleDelivery) {
		t.Errorf("the chain is at %q, want the delivery: the record is the last one", at)
	}
}

// A review-exemption whose evidence is missing, or is not the stored exemption, is refused with exit 1: the message
// names the kind and the ref of the evidence and what is wrong with it, nothing is printed on stdout, and the record
// is not stored, so the chain stays with the reviewer. An exemption that was edited after it was made, and whose
// digest the record carries, is refused for what it now says: the declaration names a high-risk item.
func TestRolesAppendRefusesAReviewExemptionWhoseEvidenceIsNotTheStoredExemption(t *testing.T) {
	flipDigest := func(kind string) func(*reviewWorld, *roles.RoleHandoff) {
		return func(_ *reviewWorld, h *roles.RoleHandoff) {
			for i := range h.Evidence {
				if h.Evidence[i].Kind == kind {
					h.Evidence[i].SHA256 = strings.Repeat("e", 64)
				}
			}
		}
	}
	drop := func(kind string) func(*reviewWorld, *roles.RoleHandoff) {
		return func(_ *reviewWorld, h *roles.RoleHandoff) {
			h.Evidence = slices.DeleteFunc(slices.Clone(h.Evidence), func(e roles.Evidence) bool { return e.Kind == kind })
		}
	}
	editStored := func(from, to string, keepDigest bool) func(*reviewWorld, *roles.RoleHandoff) {
		return func(w *reviewWorld, h *roles.RoleHandoff) {
			path := filepath.Join(w.root, filepath.FromSlash(evidenceOf(w.t, *h, review.EvidenceExemption).Ref))
			stored, err := os.ReadFile(path)
			if err != nil {
				w.t.Fatal(err)
			}
			edited := []byte(strings.Replace(string(stored), from, to, 1))
			if string(edited) == string(stored) {
				w.t.Fatalf("setup: %s does not hold %q", path, from)
			}
			if err := os.WriteFile(path, edited, 0o644); err != nil {
				w.t.Fatal(err)
			}
			if keepDigest {
				return
			}
			for i := range h.Evidence {
				if h.Evidence[i].Kind == review.EvidenceExemption {
					h.Evidence[i].SHA256 = sha256hex.Sum(edited)
				}
			}
		}
	}
	for _, tc := range []struct {
		name   string
		damage func(w *reviewWorld, h *roles.RoleHandoff)
		kind   string
		want   []string // what the message says of the evidence, after its kind and its ref
	}{
		{"it has no review-diff", drop(review.EvidenceDiff), review.EvidenceDiff, []string{"missing", "a completed review-exemption record carries one"}},
		{"it has no review-exemption", drop(review.EvidenceExemption), review.EvidenceExemption, []string{"missing", "a completed review-exemption record carries one"}},
		{"its review-exemption has the digest of another file", flipDigest(review.EvidenceExemption), review.EvidenceExemption, []string{"differs from the stored file"}},
		{"its review-diff has the digest of another diff", flipDigest(review.EvidenceDiff), review.EvidenceDiff, []string{"differs from the stored file"}},
		{"its exemption was changed after the record was made", editStored(`"risk": "none"`, `"risk": "none "`, true), review.EvidenceExemption, []string{"differs from the stored file"}},
		{"its exemption now declares a risk and the record has the digest of that file", editStored(`"risk": "none"`, `"risk": "2"`, false), review.EvidenceExemption,
			[]string{"never exempt from review", "2 (security)"}},
		{"its exemption now carries a report that is not clear and the record has the digest of that file", editStored(`"result": "pass"`, `"result": "fail"`, false), review.EvidenceExemption,
			[]string{"the report is not clear", "is fail"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rolesTestStateHome(t)
			w := newCheckedWorld(t)
			check := reviewEvidenceCheck(w.rolesDeps())
			record := w.exemptedSkeleton()
			named := evidenceOf(t, record, tc.kind) // the piece the message must name, as the skeleton has it
			tc.damage(w, &record)
			prev := reviewerChain(t, check, "chain-1")

			r := runRolesChecked(check, rolesAppendArgs("chain-1"), rolesRecordJSON(t, fillSkeleton(record, "chain-1", prev)))

			want := "error: roles append: evidence " + tc.kind
			if tc.want[0] != "missing" {
				want += " \"" + named.Ref + "\""
			}
			if r.code != 1 || r.stdout != "" || !strings.HasPrefix(r.stderr, want+": ") {
				t.Errorf("code = %d, stdout = %q, stderr = %q; want exit 1, nothing on stdout, and %q followed by the reason", r.code, r.stdout, r.stderr, want)
			}
			for _, say := range tc.want {
				if !strings.Contains(r.stderr, say) {
					t.Errorf("stderr = %q, want it to say %q", r.stderr, say)
				}
			}
			if at := rolesAt(t, "chain-1"); at != string(roles.RoleReviewer) {
				t.Errorf("the chain is at %q, want the reviewer: the record must not be stored", at)
			}
		})
	}
}

// A review-exemption that is appended where there is no repository to hold it to is refused, and the message says
// which record it could not check: a record whose evidence cannot be checked is not stored. A review-exemption that
// says the exemption was not made names no file, so it is stored as it always was, and the repository is not asked.
func TestRolesAppendSaysItCouldNotCheckTheEvidenceOfAReviewExemption(t *testing.T) {
	rolesTestStateHome(t)
	w := newCheckedWorld(t)
	record := w.exemptedSkeleton()
	outside := t.TempDir()
	w.cwd = outside
	check := reviewEvidenceCheck(w.rolesDeps())
	prev := reviewerChain(t, check, "chain-1")

	r := runRolesChecked(check, rolesAppendArgs("chain-1"), rolesRecordJSON(t, fillSkeleton(record, "chain-1", prev)))

	if r.code != 1 || r.stdout != "" || !strings.HasPrefix(r.stderr, "error: roles append: the evidence of the review-exemption could not be checked: ") || !strings.Contains(r.stderr, "locate the repository of "+outside) {
		t.Errorf("code = %d, stdout = %q, stderr = %q; want exit 1 and the review-exemption named", r.code, r.stdout, r.stderr)
	}
	if at := rolesAt(t, "chain-1"); at != string(roles.RoleReviewer) {
		t.Errorf("the chain is at %q, want the reviewer: the record must not be stored", at)
	}

	reason := "exemption not made: the verifier did not run"
	interrupted := fillSkeleton(record, "chain-1", prev)
	interrupted.Status, interrupted.ResumeReason, interrupted.Evidence = roles.StatusInterrupted, &reason, []roles.Evidence{}
	interrupted.ToRole = roles.RoleBuilder
	r = runRolesChecked(reviewEvidenceCheck(deps{}), rolesAppendArgs("chain-1"), rolesRecordJSON(t, interrupted))
	if r.code != 0 || r.stderr != "" || !strings.Contains(r.stdout, "roles append: stored record at") {
		t.Errorf("an interrupted review-exemption: code = %d, stdout = %q, stderr = %q; want it stored without asking the repository", r.code, r.stdout, r.stderr)
	}
}

// The help of `roles append` states, in the engine and in the wrapper, what the verb holds a review-exemption to, and
// in the engine what the exemption it holds it to is and which records are stored as before.
func TestUsageDocumentsTheEvidenceRolesAppendChecksOfAReviewExemption(t *testing.T) {
	engineText := requireHelpFacts(t, []helpFact{
		{"what a completed review-exemption is held to",
			"A completed review-exemption record, the one 'review exempt' prints, is held in the same way to the exemption it stored", "a completed review-exemption record is held in the same way to the exemption review exempt stored"},
		{"the evidence a review-exemption needs",
			"it needs one review-diff and one review-exemption piece of evidence", "review-diff and review-exemption evidence"},
		{"that the exemption must still hold",
			"the exemption must still hold to the rules it was made under", "the exemption must still hold"},
	})
	for _, want := range []string{
		"the review-exemption is a stored odd/review-results/<feature>/<head12>-<n>.exempt.json carrying the sha256 of the stored file",
		"a review-report or a review-exemption that is interrupted, is stored as before",
	} {
		if !strings.Contains(engineText, want) {
			t.Errorf("the engine usage lacks %q", want)
		}
	}
}
