package app_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewresult"
)

const wantFirstPath = "odd/review-results/own-rdd/222222222222-1.json"

// Record recomputes the candidate from git, holds the reply to it, and stores the reply with the binding
// in the next free file of the feature. What it returns is what the command prints: where the file is,
// its digest, and the skeleton of the record of the chain.
func TestRecordStoresTheValidatedReplyWithTheBindingItWasHeldTo(t *testing.T) {
	g, store := newFakeGit(), newFakeResults()
	c := candidateOf(t, g)
	reply := replyFor(t, c)
	svc := app.New(g, store)

	got, err := svc.Record("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", reply)
	if err != nil {
		t.Fatalf("Record() error = %v, want nil", err)
	}

	wantFile, err := review.EncodeResult(c, reply)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if stored := store.files["own-rdd"]["222222222222-1.json"]; string(stored) != string(wantFile) {
		t.Errorf("stored file = %q, want the reply with its binding: %q", stored, wantFile)
	}
	if got.Path != wantFirstPath || got.Number != 1 {
		t.Errorf("Path = %q, Number = %d; want %q and 1", got.Path, got.Number, wantFirstPath)
	}
	if want := sha256Of(wantFile); got.SHA256 != want {
		t.Errorf("SHA256 = %q, want the digest of the stored bytes %q", got.SHA256, want)
	}
	if want := review.Handoff(c, wantFirstPath, sha256Of(wantFile)); !reflect.DeepEqual(got.Handoff, want) {
		t.Errorf("Handoff = %+v, want %+v", got.Handoff, want)
	}
	if !reflect.DeepEqual(got.Candidate, c) {
		t.Errorf("Candidate = %+v, want %+v", got.Candidate, c)
	}
	// It stores in the root of the working tree the command ran in, not in the main checkout.
	if want := "Names " + fakeRepo.Root + " own-rdd"; len(store.calls) == 0 || store.calls[0] != want {
		t.Errorf("store calls = %q, want the first to be %q", store.calls, want)
	}
}

// A second launch of the same head takes the next number.
func TestRecordTakesTheNextFreeNumber(t *testing.T) {
	g, store := newFakeGit(), newFakeResults()
	store.put("own-rdd", "222222222222-1.json", []byte("first launch"))
	store.put("own-rdd", "222222222222-2.verifier.md", []byte("verifier report"))
	reply := replyFor(t, candidateOf(t, g))
	svc := app.New(g, store)

	got, err := svc.Record("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", reply)
	if err != nil {
		t.Fatalf("Record() error = %v, want nil", err)
	}

	if want := "odd/review-results/own-rdd/222222222222-3.json"; got.Path != want || got.Number != 3 {
		t.Errorf("Path = %q, Number = %d; want %q and 3", got.Path, got.Number, want)
	}
	if string(store.files["own-rdd"]["222222222222-1.json"]) != "first launch" {
		t.Errorf("the first launch was changed: %q", store.files["own-rdd"]["222222222222-1.json"])
	}
}

// Two records racing for one number: the one that lost lists the directory again and takes the next.
func TestRecordTakesTheNextNumberWhenAnotherWriterTookItFirst(t *testing.T) {
	g, store := newFakeGit(), newFakeResults()
	store.races = 1
	reply := replyFor(t, candidateOf(t, g))
	svc := app.New(g, store)

	got, err := svc.Record("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", reply)
	if err != nil {
		t.Fatalf("Record() error = %v, want nil", err)
	}

	if want := "odd/review-results/own-rdd/222222222222-2.json"; got.Path != want || got.Number != 2 {
		t.Errorf("Path = %q, Number = %d; want %q and 2", got.Path, got.Number, want)
	}
	if string(store.files["own-rdd"]["222222222222-1.json"]) != otherWriter {
		t.Errorf("the file of the other writer was changed: %q", store.files["own-rdd"]["222222222222-1.json"])
	}
}

// A directory that never has a free number is an error, not a loop, and the number of tries is part of the
// contract: sixteen, each one listing the directory again.
func TestRecordGivesUpWhenNoNumberStaysFree(t *testing.T) {
	g := newFakeGit()
	store := newFakeResults()
	store.fail["Create"] = app.ErrExists
	svc := app.New(g, store)

	_, err := svc.Record("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", replyFor(t, candidateOf(t, g)))

	if !errors.Is(err, app.ErrNoFreeNumber) {
		t.Fatalf("Record() error = %v, want one that is %v", err, app.ErrNoFreeNumber)
	}
	if !strings.Contains(err.Error(), "16 tries") {
		t.Errorf("Record() error = %q, want it to say how many tries were made: 16", err)
	}
	if n := store.created(); n != 16 {
		t.Errorf("Create was called %d times, want exactly 16 tries", n)
	}
	if n := store.listed(); n != 16 {
		t.Errorf("the directory was listed %d times, want once per try (16)", n)
	}
}

// A directory in which another writer takes each number just before this one does still gives a free number
// as long as that happens fewer times than there are tries. Every try lists the directory again, which is how
// the number moves on, and the files of the other writers are left as they are.
func TestRecordLandsOnAFreeNumberWhenAnotherWriterTakesEachNumberItTries(t *testing.T) {
	for _, tc := range []struct {
		name   string
		races  int
		number int // the number stored; 0 when none stays free
	}{
		{"three writers in a row", 3, 4},
		{"as many writers as the tries allow, but one", 15, 16},
		{"as many writers as there are tries", 16, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store := newFakeGit(), newFakeResults()
			store.races = tc.races
			svc := app.New(g, store)

			got, err := svc.Record("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", replyFor(t, candidateOf(t, g)))

			stored := store.files["own-rdd"]
			others := 0
			for _, content := range stored {
				if string(content) == otherWriter {
					others++
				}
			}
			if others != tc.races {
				t.Errorf("%d files of the other writers are there, want the %d they wrote, untouched", others, tc.races)
			}
			if n, want := store.created(), min(tc.races+1, 16); n != want {
				t.Errorf("Create was called %d times, want %d", n, want)
			}
			if store.listed() != store.created() {
				t.Errorf("the directory was listed %d times for %d tries, want it listed again before each", store.listed(), store.created())
			}
			if tc.number == 0 {
				if !errors.Is(err, app.ErrNoFreeNumber) {
					t.Fatalf("Record() error = %v, want one that is %v", err, app.ErrNoFreeNumber)
				}
				if len(stored) != tc.races {
					t.Errorf("%d files are stored, want only the %d of the other writers", len(stored), tc.races)
				}
				return
			}
			if err != nil {
				t.Fatalf("Record() error = %v, want nil: the number %d is free", err, tc.number)
			}
			name := fmt.Sprintf("222222222222-%d.json", tc.number)
			if got.Number != tc.number || got.Path != "odd/review-results/own-rdd/"+name || len(stored[name]) == 0 || string(stored[name]) == otherWriter {
				t.Errorf("Number = %d, Path = %q, stored %q; want number %d at %s holding the reply", got.Number, got.Path, stored[name], tc.number, name)
			}
		})
	}
}

// A writer that lost the race for a number waits before it tries again, and waits longer each time, so that
// writers that keep colliding do not hammer a directory the others are writing to. The wait is short and its
// total bounded. There is one between two tries: none before the first, none after the last, which gives up.
func TestRecordWaitsBetweenTriesAndLongerEachTime(t *testing.T) {
	for _, tc := range []struct {
		name      string
		races     int  // writers that take the number first
		noneFree  bool // every number is taken
		wantTries int
		wantErr   error
	}{
		{"no other writer", 0, false, 1, nil},
		{"one other writer", 1, false, 2, nil},
		{"as many writers as the tries allow, but one", 15, false, 16, nil},
		{"no number stays free", 0, true, 16, app.ErrNoFreeNumber},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store, waits := newFakeGit(), newFakeResults(), &pauses{}
			store.races = tc.races
			if tc.noneFree {
				store.fail["Create"] = app.ErrExists
			}
			svc := app.New(g, store).WithSleep(waits.sleep)

			_, err := svc.Record("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", replyFor(t, candidateOf(t, g)))

			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Record() error = %v, want one that is %v", err, tc.wantErr)
			}
			if n := store.created(); n != tc.wantTries {
				t.Fatalf("Create was called %d times, want %d", n, tc.wantTries)
			}
			if len(waits.asked) != tc.wantTries-1 {
				t.Fatalf("waited %d times (%v), want %d: one between two tries, none before the first or after the last", len(waits.asked), waits.asked, tc.wantTries-1)
			}
			var total time.Duration
			for i, d := range waits.asked {
				total += d
				if d <= 0 || d > 100*time.Millisecond {
					t.Errorf("wait %d is %v, want a short one: more than nothing and at most 100ms", i+1, d)
				}
				if i > 0 && d < waits.asked[i-1] {
					t.Errorf("wait %d is %v, shorter than the one before it (%v)", i+1, d, waits.asked[i-1])
				}
			}
			if total > time.Second {
				t.Errorf("waited %v in all, want the total bounded by a second", total)
			}
			if n := len(waits.asked); n > 1 && waits.asked[n-1] <= waits.asked[0] {
				t.Errorf("the last wait is %v and the first %v, want waits that grow", waits.asked[n-1], waits.asked[0])
			}
		})
	}
}

// The clock is not the application's to read, so it is handed in (the command hands in the real one): a
// Service that was given none, or was told to wait with nothing, still takes the next number.
func TestRecordWithoutAClockStillTakesTheNextNumber(t *testing.T) {
	for _, tc := range []struct {
		name string
		svc  func(g *fakeGit, store *fakeResults) app.Service
	}{
		{"as made", func(g *fakeGit, store *fakeResults) app.Service { return app.New(g, store) }},
		{"told to wait with nothing", func(g *fakeGit, store *fakeResults) app.Service { return app.New(g, store).WithSleep(nil) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store := newFakeGit(), newFakeResults()
			store.races = 2

			got, err := tc.svc(g, store).Record("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", replyFor(t, candidateOf(t, g)))

			if err != nil || got.Number != 3 {
				t.Errorf("Record() = number %d, error %v; want number 3 and no error after two lost races", got.Number, err)
			}
		})
	}
}

// Whatever the reply is, the binding is the one git gave: a reply that echoes another candidate, did not
// inspect it, lists other paths or is not the object of the contract is refused, and nothing is stored.
func TestRecordRefusesAReplyThatDoesNotHoldToTheBindingAndStoresNothing(t *testing.T) {
	g := newFakeGit()
	c := candidateOf(t, g)
	for _, tc := range []struct {
		name  string
		reply []byte
		want  error
	}{
		{"it echoes another diff", replyWith(t, baseSHA, headSHA, sha256Of([]byte("another diff")), "completed", c.Paths), reviewresult.ErrCandidateMismatch},
		{"it echoes another head", replyWith(t, baseSHA, otherSHA, c.DiffSHA256, "completed", c.Paths), reviewresult.ErrCandidateMismatch},
		{"it did not finish", replyWith(t, baseSHA, headSHA, c.DiffSHA256, "incomplete", c.Paths), reviewresult.ErrInspectionNotCompleted},
		{"it lists the paths of another diff", replyWith(t, baseSHA, headSHA, c.DiffSHA256, "completed", []string{"other.go"}), reviewresult.ErrPathsMismatch},
		{"it is prose", []byte("The diff looks fine to me."), reviewresult.ErrUnparseable},
		{"it is nothing", nil, reviewresult.ErrUnparseable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store := newFakeGit(), newFakeResults()
			svc := app.New(g, store)

			_, err := svc.Record("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", tc.reply)

			var refused *app.ReplyRefusedError
			if !errors.As(err, &refused) {
				t.Fatalf("Record() error = %v, want a *app.ReplyRefusedError", err)
			}
			if !errors.Is(err, tc.want) {
				t.Errorf("Record() error = %v, want one that is %v", err, tc.want)
			}
			if !strings.Contains(err.Error(), tc.want.Error()) {
				t.Errorf("Record() error = %q, want the message to carry the named error %q", err, tc.want)
			}
			if len(store.calls) != 0 {
				t.Errorf("store calls = %q, want none for a refused reply", store.calls)
			}
		})
	}
}

// The size of a reply is bounded by the command, and the rule is the domain's: one byte over is refused.
func TestRecordRefusesAReplyAboveTheLimit(t *testing.T) {
	g, store := newFakeGit(), newFakeResults()
	svc := app.New(g, store)
	reply := []byte(strings.Repeat(" ", review.MaxReplyBytes+1))

	_, err := svc.Record("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", reply)

	var refused *app.ReplyRefusedError
	if !errors.As(err, &refused) || !errors.Is(err, review.ErrReplyTooLarge) {
		t.Fatalf("Record() error = %v, want a refusal that is %v", err, review.ErrReplyTooLarge)
	}
	if len(store.calls) != 0 {
		t.Errorf("store calls = %q, want none", store.calls)
	}
}

// The limit is on what is above it: a reply of exactly MaxReplyBytes bytes is read like any other. Here it is
// the reply of a reviewer with white space after it, which the rules of the reply allow.
func TestRecordAcceptsAReplyOfExactlyTheLimit(t *testing.T) {
	g, store := newFakeGit(), newFakeResults()
	reply := replyFor(t, candidateOf(t, g))
	reply = append(reply, strings.Repeat(" ", review.MaxReplyBytes-len(reply))...)
	if len(reply) != review.MaxReplyBytes {
		t.Fatalf("setup: the reply has %d bytes, want %d", len(reply), review.MaxReplyBytes)
	}
	svc := app.New(g, store)

	got, err := svc.Record("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", reply)

	if err != nil {
		t.Fatalf("Record() error = %v, want nil for a reply of exactly %d bytes", err, review.MaxReplyBytes)
	}
	if got.Path != wantFirstPath || len(store.files["own-rdd"]["222222222222-1.json"]) == 0 {
		t.Errorf("Path = %q and stored %d bytes, want %q stored", got.Path, len(store.files["own-rdd"]["222222222222-1.json"]), wantFirstPath)
	}
}

// A feature or a revision that is not safe is refused before git or the store is asked for anything.
func TestRecordRefusesAnUnsafeFeatureOrRevisionBeforeAskingAnyone(t *testing.T) {
	for _, tc := range []struct {
		name                string
		feature, base, head string
		want                error
	}{
		{"a way up", "../x", "HEAD~1", "HEAD", review.ErrInvalidFeature},
		{"a separator", "a/b", "HEAD~1", "HEAD", review.ErrInvalidFeature},
		{"no feature", "", "HEAD~1", "HEAD", review.ErrInvalidFeature},
		{"an option as the base", "own-rdd", "--output=x", "HEAD", review.ErrInvalidRev},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store := newFakeGit(), newFakeResults()
			svc := app.New(g, store)

			_, err := svc.Record("/work/wt/feature", tc.feature, tc.base, tc.head, replyFor(t, candidateOf(t, g)))

			if !errors.Is(err, tc.want) {
				t.Errorf("Record() error = %v, want one that is %v", err, tc.want)
			}
			if len(g.calls) != 0 || len(store.calls) != 0 {
				t.Errorf("git calls %q and store calls %q, want none", g.calls, store.calls)
			}
		})
	}
}

// What git or the store could not do is the error of the command, with the cause, and nothing is
// created after it.
func TestRecordReturnsWhatGitOrTheStoreCouldNotDo(t *testing.T) {
	boom := errors.New("boom")
	for _, tc := range []struct {
		name        string
		failGit     string
		failStore   string
		wantCreates int
	}{
		{"git cannot locate the repository", "Locate", "", 0},
		{"git cannot read the diff", "Diff", "", 0},
		{"the store cannot list the directory", "", "Names", 0},
		{"the store cannot create the file", "", "Create", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store := newFakeGit(), newFakeResults()
			if tc.failGit != "" {
				g.fail[tc.failGit] = boom
			}
			if tc.failStore != "" {
				store.fail[tc.failStore] = boom
			}
			svc := app.New(g, store)

			_, err := svc.Record("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", replyFor(t, candidateOf(t, g)))

			if !errors.Is(err, boom) {
				t.Errorf("Record() error = %v, want one that wraps %v", err, boom)
			}
			if n := store.created(); n != tc.wantCreates {
				t.Errorf("Create was called %d times, want %d (an error that is not a taken name is not retried)", n, tc.wantCreates)
			}
		})
	}
}
