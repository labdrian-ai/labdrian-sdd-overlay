package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sha256hex"
)

// The tests of the evidence that `roles append` holds a review-report to run the program over a repository they
// make in a temporary directory (reviewWorld) and a role chain store under a temporary state home
// (rolesTestStateHome), so that no test reads or writes the machine's repositories or its state.

const rolesProject, rolesGoal = "proj-1", "goal-1"

// rolesDeps is the deps of a run of `roles append` in the repository of the world: the working directory is
// the world's, and the git the check asks reads no configuration of the machine.
func (w *reviewWorld) rolesDeps() deps {
	d := testDeps()
	d.getwd = func() (string, error) { return w.cwd, nil }
	d.environ = goldenEnvironment
	return d
}

// decodeSkeleton reads the record that `review record` or `review verify` printed.
func decodeSkeleton(t *testing.T, stdout string) roles.RoleHandoff {
	t.Helper()
	var h roles.RoleHandoff
	if err := json.Unmarshal([]byte(stdout), &h); err != nil {
		t.Fatalf("stdout is not the skeleton of a RoleHandoff: %v\n%s", err, stdout)
	}
	return h
}

// recordedSkeleton records the reply of a reviewer who found nothing in the candidate of the world and returns
// the skeleton `review record` printed for it.
func (w *reviewWorld) recordedSkeleton() roles.RoleHandoff {
	w.t.Helper()
	b := w.bind()
	stdout, stderr, exits := w.run(reply(w.t, b, nil), "record", "--feature", "own-rdd", "--base", w.base, "--head", w.head, "--rows", "default")
	if !equalInts(exits, []int{0}) {
		w.t.Fatalf("setup: review record exit calls = %v, stderr = %q; want [0]", exits, stderr)
	}
	return decodeSkeleton(w.t, stdout)
}

// verifiedSkeleton records a blocker of a reviewer and the reply of a verifier that does not reproduce it, and
// returns the skeleton `review verify` printed: a review that is clear, with the verdict bound as evidence.
func (w *reviewWorld) verifiedSkeleton() roles.RoleHandoff {
	w.t.Helper()
	_, stored := w.recordBlocker(w.base, w.head)
	stdout, stderr, exits := w.run(verdictOn("not-reproduced"), "verify", "--feature", "own-rdd", "--stored", stored)
	if !equalInts(exits, []int{0}) {
		w.t.Fatalf("setup: review verify exit calls = %v, stderr = %q; want [0]", exits, stderr)
	}
	return decodeSkeleton(w.t, stdout)
}

func rolesAppendArgs(chain string) []string {
	return []string{"append", "--project", rolesProject, "--goal", rolesGoal, "--chain", chain, "--stdin"}
}

func rolesResumeArgs(chain string) []string {
	return []string{"resume", "--project", rolesProject, "--goal", rolesGoal, "--chain", chain}
}

// rolesRecordJSON is the document of a record, as the parent of the chain writes it.
func rolesRecordJSON(t *testing.T, h roles.RoleHandoff) string {
	t.Helper()
	data, err := json.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// reviewerChain stores the three records that put the work of the chain in the hands of the reviewer, through
// check, and returns the digest the record that follows them chains to.
func reviewerChain(t *testing.T, check evidenceCheck, chain string) string {
	t.Helper()
	prev := roles.EmptyChainDigest
	for i, hop := range [][2]roles.Role{{roles.RoleShaper, roles.RoleEstimator}, {roles.RoleEstimator, roles.RoleBuilder}, {roles.RoleBuilder, roles.RoleReviewer}} {
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

// fillSkeleton is the skeleton with the members only the parent of the chain knows filled: the chain it follows
// after the three records of reviewerChain, and the summary. Who is next stays as the skeleton says it, and is the
// delivery when it says nobody.
func fillSkeleton(h roles.RoleHandoff, chain, prev string) roles.RoleHandoff {
	h.ProjectID, h.GoalID, h.ChainID, h.Seq, h.PrevSHA256 = rolesProject, rolesGoal, chain, 4, prev
	if h.ToRole == "" {
		h.ToRole = roles.RoleDelivery
	}
	h.Context.Summary = "reviewed"
	return h
}

// evidenceOf is the piece of evidence of the kind that h carries.
func evidenceOf(t *testing.T, h roles.RoleHandoff, kind string) roles.Evidence {
	t.Helper()
	for _, e := range h.Evidence {
		if e.Kind == kind {
			return e
		}
	}
	t.Fatalf("setup: the record carries no %s evidence: %+v", kind, h.Evidence)
	return roles.Evidence{}
}

// rolesAt says the role that holds the work of the chain, as `roles resume` reads it from the store: the
// stored records are what tells whether a record was appended.
func rolesAt(t *testing.T, chain string) string {
	t.Helper()
	r := runRolesTest(rolesResumeArgs(chain), "")
	if r.code != 0 {
		t.Fatalf("roles resume: code=%d stderr=%q", r.code, r.stderr)
	}
	var out struct {
		Role string `json:"role"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &out); err != nil {
		t.Fatalf("roles resume printed %q: %v", r.stdout, err)
	}
	return out.Role
}

// 'roles append' asks the check of the evidence once it has read the record and matched it to its flags, and
// before it stores it. A record the check refuses is refused with exit 1 and the words of the check, and is
// not stored; one that the check accepts is stored; one that the flags do not match is refused without the check
// being asked.
func TestRolesAppendAsksTheEvidenceCheckBeforeItStoresTheRecord(t *testing.T) {
	record := rolesHandoffJSON(1, "", "shaper", "estimator", "completed", "")
	refusal := errors.New(`evidence review-result "odd/review-results/own-rdd/222222222222-1.json": differs from the stored file`)
	for _, tc := range []struct {
		name       string
		verdict    error
		args       []string
		wantCode   int
		wantAsked  int
		wantStderr string
		wantStored bool
	}{
		{"the check refuses the record", refusal, rolesAppendArgs("chain-1"), 1, 1, "error: roles append: " + refusal.Error() + "\n", false},
		{"the check accepts the record", nil, rolesAppendArgs("chain-1"), 0, 1, "", true},
		{"the flags do not match the record", refusal, []string{"append", "--project", "other", "--goal", rolesGoal, "--chain", "chain-1", "--stdin"}, 1, 0,
			"error: roles append: --project \"other\" does not match the record's project_id \"proj-1\"\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rolesTestStateHome(t)
			var asked []roles.RoleHandoff
			check := func(h roles.RoleHandoff) error {
				asked = append(asked, h)
				return tc.verdict
			}

			r := runRolesChecked(check, tc.args, record)

			if r.code != tc.wantCode || (tc.wantStderr != "" && r.stderr != tc.wantStderr) || (tc.wantStderr == "" && r.stderr != "") {
				t.Errorf("code = %d, stderr = %q; want %d and %q", r.code, r.stderr, tc.wantCode, tc.wantStderr)
			}
			if len(asked) != tc.wantAsked || (len(asked) == 1 && (asked[0].Seq != 1 || asked[0].FromRole != roles.RoleShaper)) {
				t.Errorf("the check was asked about %+v, want %d record(s), the one that was read", asked, tc.wantAsked)
			}
			stored := runRolesTest(rolesResumeArgs("chain-1"), "").code == 0
			if stored != tc.wantStored {
				t.Errorf("stored = %v, want %v", stored, tc.wantStored)
			}
		})
	}
}

// The records that `review record` and `review verify` print, once the parent of the chain has filled the
// members only it knows, are records `roles append` stores: the evidence they carry is the evidence of the files
// the two verbs stored.
func TestRolesAppendStoresTheRecordsThatReviewRecordAndReviewVerifyPrint(t *testing.T) {
	for _, tc := range []struct {
		name      string
		skeleton  func(w *reviewWorld) roles.RoleHandoff
		wantKinds []string
	}{
		{"review record", (*reviewWorld).recordedSkeleton, []string{review.EvidenceDiff, review.EvidenceResult}},
		{"review verify", (*reviewWorld).verifiedSkeleton, []string{review.EvidenceDiff, review.EvidenceResult, review.EvidenceVerifier}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rolesTestStateHome(t)
			w := newReviewWorld(t)
			check := reviewEvidenceCheck(w.rolesDeps())
			record := tc.skeleton(w)
			var kinds []string
			for _, e := range record.Evidence {
				kinds = append(kinds, e.Kind)
			}
			if !review.NeedsEvidence(record) || !slices.Equal(kinds, tc.wantKinds) {
				t.Fatalf("setup: the skeleton is a record with evidence %q (held to evidence: %v), want a completed review-report with %q", kinds, review.NeedsEvidence(record), tc.wantKinds)
			}
			prev := reviewerChain(t, check, "chain-1")

			r := runRolesChecked(check, rolesAppendArgs("chain-1"), rolesRecordJSON(t, fillSkeleton(record, "chain-1", prev)))

			if r.code != 0 || r.stderr != "" || !strings.Contains(r.stdout, "roles append: stored record at") {
				t.Fatalf("code = %d, stdout = %q, stderr = %q; want the record stored", r.code, r.stdout, r.stderr)
			}
			if at := rolesAt(t, "chain-1"); at != string(roles.RoleDelivery) {
				t.Errorf("the chain is at %q, want the delivery: the record is the last one", at)
			}
		})
	}
}

// A review-report whose evidence is missing, or whose sha256 differs from the stored files it names, is refused
// with exit 1: the message names the kind and the ref of the evidence and what is wrong with it, nothing is
// printed on stdout, and the record is not stored, so the chain stays with the reviewer.
func TestRolesAppendRefusesAReviewReportWhoseEvidenceIsNotTheStoredFiles(t *testing.T) {
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
	for _, tc := range []struct {
		name     string
		skeleton func(w *reviewWorld) roles.RoleHandoff
		damage   func(w *reviewWorld, h *roles.RoleHandoff)
		kind     string
		want     string // what the message says of the evidence, after its kind and its ref
	}{
		{"it has no review-diff", (*reviewWorld).recordedSkeleton, drop(review.EvidenceDiff), review.EvidenceDiff, "missing"},
		{"it has no review-result", (*reviewWorld).recordedSkeleton, drop(review.EvidenceResult), review.EvidenceResult, "missing"},
		{"its review-result has the digest of another file", (*reviewWorld).recordedSkeleton, flipDigest(review.EvidenceResult), review.EvidenceResult, "differs from the stored file"},
		{"its review-diff has the digest of another diff", (*reviewWorld).recordedSkeleton, flipDigest(review.EvidenceDiff), review.EvidenceDiff, "differs from the stored file"},
		{"its review-result was changed after the record was made", (*reviewWorld).recordedSkeleton, func(w *reviewWorld, h *roles.RoleHandoff) {
			path := filepath.Join(w.root, filepath.FromSlash(evidenceOf(w.t, *h, review.EvidenceResult).Ref))
			stored, err := os.ReadFile(path)
			if err != nil {
				w.t.Fatal(err)
			}
			if err := os.WriteFile(path, append(stored, '\n'), 0o644); err != nil {
				w.t.Fatal(err)
			}
		}, review.EvidenceResult, "differs from the stored file"},
		{"its verifier-report has the digest of another file", (*reviewWorld).verifiedSkeleton, flipDigest(review.EvidenceVerifier), review.EvidenceVerifier, "differs from the stored file"},
		{"its stored verdict is not a verdict, and the record carries the digest of that file", (*reviewWorld).verifiedSkeleton, func(w *reviewWorld, h *roles.RoleHandoff) {
			notAVerdict := []byte("this is not a verdict")
			path := filepath.Join(w.root, filepath.FromSlash(evidenceOf(w.t, *h, review.EvidenceVerifier).Ref))
			if err := os.WriteFile(path, notAVerdict, 0o644); err != nil {
				w.t.Fatal(err)
			}
			for i := range h.Evidence {
				if h.Evidence[i].Kind == review.EvidenceVerifier {
					h.Evidence[i].SHA256 = sum(notAVerdict)
				}
			}
		}, review.EvidenceVerifier, "unparseable reply"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rolesTestStateHome(t)
			w := newReviewWorld(t)
			check := reviewEvidenceCheck(w.rolesDeps())
			record := tc.skeleton(w)
			named := evidenceOf(t, record, tc.kind) // the piece the message must name, as the skeleton has it
			tc.damage(w, &record)
			prev := reviewerChain(t, check, "chain-1")

			r := runRolesChecked(check, rolesAppendArgs("chain-1"), rolesRecordJSON(t, fillSkeleton(record, "chain-1", prev)))

			want := "error: roles append: evidence " + tc.kind
			if tc.want != "missing" {
				want += fmt.Sprintf(" %q", named.Ref)
			}
			if r.code != 1 || r.stdout != "" || !strings.HasPrefix(r.stderr, want+": ") || !strings.Contains(r.stderr, tc.want) {
				t.Errorf("code = %d, stdout = %q, stderr = %q; want exit 1, nothing on stdout, and %q followed by %q", r.code, r.stdout, r.stderr, want, tc.want)
			}
			if at := rolesAt(t, "chain-1"); at != string(roles.RoleReviewer) {
				t.Errorf("the chain is at %q, want the reviewer: the record must not be stored", at)
			}
		})
	}
}

// A review-report that says the review did not run stores nothing and names nothing, so it has no evidence to be
// held to: `roles append` stores it as it always did, and does not ask the repository (the deps here have no
// working directory to ask it in).
func TestRolesAppendStoresAnInterruptedReviewReportWithoutAskingTheRepository(t *testing.T) {
	rolesTestStateHome(t)
	check := reviewEvidenceCheck(deps{})
	prev := reviewerChain(t, check, "chain-1")
	reason := "review not run: the inspection was incomplete"
	record := roles.RoleHandoff{
		Version: roles.HandoffVersion, ProjectID: rolesProject, GoalID: rolesGoal, ChainID: "chain-1", Seq: 4,
		FromRole: roles.RoleReviewer, ToRole: roles.RoleBuilder, PrevSHA256: prev, PayloadKind: review.PayloadKind, PayloadSHA256: strings.Repeat("a", 64),
		Evidence: []roles.Evidence{}, Context: roles.Context{Summary: "s", Decisions: []string{}, OpenQuestions: []string{}},
		Status: roles.StatusInterrupted, ResumeReason: &reason,
	}

	r := runRolesChecked(check, rolesAppendArgs("chain-1"), rolesRecordJSON(t, record))

	if r.code != 0 || r.stderr != "" || !strings.Contains(r.stdout, "roles append: stored record at") {
		t.Errorf("code = %d, stdout = %q, stderr = %q; want the interrupted record stored", r.code, r.stdout, r.stderr)
	}
}

// The dispatcher builds the evidence check of `roles append` from the deps it was handed, as it does the other
// verbs: the repository is the one that holds the working directory of those deps, not the program's own. From the
// directory of the world the record whose evidence is the stored files is stored, and from a directory that holds
// no repository the same record is refused for want of one, with nothing stored.
func TestRunHandsRolesAppendTheDepsItWasHanded(t *testing.T) {
	rolesTestStateHome(t)
	w := newReviewWorld(t)
	prev := reviewerChain(t, reviewEvidenceCheck(deps{}), "chain-1")
	record := rolesRecordJSON(t, fillSkeleton(w.recordedSkeleton(), "chain-1", prev))
	appendFrom := func(dir string) *capturedProcess {
		d := w.rolesDeps()
		d.getwd = func() (string, error) { return dir, nil }
		p := newCapturedProcess()
		p.stdin = strings.NewReader(record)
		run(p.process, d, append([]string{"roles"}, rolesAppendArgs("chain-1")...))
		return p
	}
	outside := t.TempDir()

	refused := appendFrom(outside)

	if !equalInts(refused.exits, []int{1}) || refused.out.String() != "" || !strings.Contains(refused.err.String(), "locate the repository of "+outside) {
		t.Errorf("from %s: exits = %v, stdout = %q, stderr = %q; want exit 1 and the repository of the deps' directory looked for", outside, refused.exits, refused.out.String(), refused.err.String())
	}
	if at := rolesAt(t, "chain-1"); at != string(roles.RoleReviewer) {
		t.Fatalf("the chain is at %q after the refusal, want the reviewer: nothing was stored", at)
	}

	stored := appendFrom(w.root)

	if len(stored.exits) != 0 || stored.err.String() != "" || !strings.Contains(stored.out.String(), "roles append: stored record at") {
		t.Errorf("from %s: exits = %v, stdout = %q, stderr = %q; want the record stored", w.root, stored.exits, stored.out.String(), stored.err.String())
	}
	if at := rolesAt(t, "chain-1"); at != string(roles.RoleDelivery) {
		t.Errorf("the chain is at %q, want the delivery", at)
	}
}

// The built program reads the repository from its working directory: a review-report whose evidence is the files
// stored there is stored, one whose evidence is another file is refused, and from a directory that holds no
// repository there are no stored files to hold it to, so it is refused too and nothing is stored.
func TestRolesAppendOfTheBuiltProgramHoldsAReviewReportToTheFilesStoredInTheRepository(t *testing.T) {
	rolesTestStateHome(t)
	w := newReviewWorld(t)
	bin := engineBinary(t)
	run := func(dir, stdin string, args ...string) (stdout, stderr string, code int) {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Dir = dir
		cmd.Env = goldenEnvironment() // carries the XDG_STATE_HOME that rolesTestStateHome set
		cmd.Stdin = strings.NewReader(stdin)
		var o, e bytes.Buffer
		cmd.Stdout, cmd.Stderr = &o, &e
		if err := cmd.Run(); err != nil {
			ee, ok := err.(*exec.ExitError)
			if !ok {
				t.Fatalf("run %v: %v", args, err)
			}
			return o.String(), e.String(), ee.ExitCode()
		}
		return o.String(), e.String(), 0
	}
	record := w.recordedSkeleton()
	prev := reviewerChain(t, reviewEvidenceCheck(deps{}), "chain-1")
	good := rolesRecordJSON(t, fillSkeleton(record, "chain-1", prev))
	other := fillSkeleton(record, "chain-1", prev)
	other.Evidence = slices.Clone(other.Evidence)
	for i := range other.Evidence {
		if other.Evidence[i].Kind == review.EvidenceResult {
			other.Evidence[i].SHA256 = strings.Repeat("e", 64)
		}
	}
	appendArgs := rolesAppendArgs("chain-1")
	args := append([]string{"roles"}, appendArgs...)

	_, stderr, code := run(w.root, rolesRecordJSON(t, other), args...)
	if code != 1 || !strings.HasPrefix(stderr, "error: roles append: evidence review-result \"") || !strings.Contains(stderr, "differs from the stored file") {
		t.Errorf("a record whose review-result is another file: exit %d, stderr %q; want 1 and the evidence named", code, stderr)
	}
	outside := t.TempDir()
	_, stderr, code = run(outside, good, args...)
	if code != 1 || !strings.Contains(stderr, "the evidence of the review-report could not be checked") || !strings.Contains(stderr, "locate the repository of "+outside) {
		t.Errorf("a record appended from a directory with no repository: exit %d, stderr %q; want 1 and the repository named", code, stderr)
	}
	if at := rolesAt(t, "chain-1"); at != string(roles.RoleReviewer) {
		t.Fatalf("the chain is at %q after two refusals, want the reviewer: nothing was stored", at)
	}

	stdout, stderr, code := run(w.root, good, args...)
	if code != 0 || stderr != "" || !strings.Contains(stdout, "roles append: stored record at") {
		t.Errorf("the record with the stored files as evidence: exit %d, stdout %q, stderr %q; want 0 and the record stored", code, stdout, stderr)
	}
	if at := rolesAt(t, "chain-1"); at != string(roles.RoleDelivery) {
		t.Errorf("the chain is at %q, want the delivery", at)
	}
}
