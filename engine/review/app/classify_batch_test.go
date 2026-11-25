package app_test

// What Stop asks of the Classified port and what it keeps (S6, L72): the port is asked once for all the kept results at
// every stop, nothing it says is remembered, a refusal is named once for each session and stored file, and an answer that does
// not fit its question is an error. A refusal is named once for each session and stored file (L74), and only once a stop
// has returned it with its ruling.

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
)

// hasFailsWithPrefix is an Asked store whose Has fails for every key that starts with prefix, and for no other.
type hasFailsWithPrefix struct {
	*fakeAsked
	prefix string
	err    error
}

func (a *hasFailsWithPrefix) Has(session, key string) (bool, error) {
	if strings.HasPrefix(key, a.prefix) {
		return false, a.err
	}
	return a.fakeAsked.Has(session, key)
}

// M11: a refusal is named once for each session and stored file.
func TestStopNamesARefusalOnceForEachSession(t *testing.T) {
	refused := func(t *testing.T) (*fakeGit, *fakeResults) {
		g, store, _ := classifyWorld(t)
		store.put("own-rdd", "333333333333-9.json", []byte("not a result"))
		return g, store
	}
	named := func(err error) bool { return err != nil && strings.Contains(err.Error(), "333333333333-9.json") }

	t.Run("once for each session", func(t *testing.T) {
		g, store := refused(t)
		service := app.New(g, store).WithAsked(newFakeAsked()).WithClassified(newFakeClassified())

		first, firstErr := service.Stop(evidenceDir, "session-1", false)
		second, secondErr := service.Stop(evidenceDir, "session-1", false)
		other, otherErr := service.Stop(evidenceDir, "session-2", false)

		if !first.Block || !reflect.DeepEqual(reviewsOf(first.Classify), []string{otherReview}) || !named(firstErr) {
			t.Errorf("stop 1 = %+v, %v; want a block on %s and an error naming -9.json", first, firstErr, otherReview)
		}
		if second.Block || secondErr != nil {
			t.Errorf("stop 2 = %+v, %v; want no block and no error", second, secondErr)
		}
		if !other.Block || !named(otherErr) {
			t.Errorf("stop 1 of session-2 = %+v, %v; want a block and the error again", other, otherErr)
		}
	})
	t.Run("a record that fails names it again", func(t *testing.T) {
		g, store := refused(t)
		asked := &addFailsWithPrefix{fakeAsked: newFakeAsked(), prefix: "refused-", err: errors.New("disk full")}
		service := app.New(g, store).WithAsked(asked).WithClassified(newFakeClassified())

		_, firstErr := service.Stop(evidenceDir, "session-1", false)
		_, secondErr := service.Stop(evidenceDir, "session-1", false)

		if !named(firstErr) || !named(secondErr) || !strings.Contains(firstErr.Error(), "record that a refusal was named") {
			t.Errorf("errors = %v and %v; want the refusal named at both stops and the failed record at the first", firstErr, secondErr)
		}
	})
	t.Run("a read that fails is no block", func(t *testing.T) {
		g, store := refused(t)
		cause := errors.New("boom")
		asked := &hasFailsWithPrefix{fakeAsked: newFakeAsked(), prefix: "refused-", err: cause}
		port := newFakeClassified()

		got, err := app.New(g, store).WithAsked(asked).WithClassified(port).Stop(evidenceDir, "session-1", false)

		if got.Block || !errors.Is(err, cause) || len(port.calls) != 0 {
			t.Errorf("Stop() = %+v, %v with %q asked of the port; want no block, an error that wraps the cause and no call", got, err, port.calls)
		}
	})
}

// M12: the port is asked at every stop, once, for every kept result in order, and what it says is not remembered.
func TestStopAsksThePortForEveryKeptResultInOneCallAtEveryStop(t *testing.T) {
	g, store, _ := classifyWorld(t)
	reviewed(t, store, baseSHA, otherSHA, 2, warning("F2"))
	port := newFakeClassified()
	port.done[otherReview] = true
	service := app.New(g, store).WithAsked(newFakeAsked()).WithClassified(port)

	for i, step := range []struct {
		session string
		block   bool
	}{{"session-1", true}, {"session-1", false}, {"session-1", false}, {"session-2", true}} {
		got, err := service.Stop(evidenceDir, step.session, false)
		if err != nil || got.Block != step.block || len(port.calls) != i+1 {
			t.Fatalf("stop %d of %s = %+v, %v with %d port calls; want block=%v, no error and %d calls", i+1, step.session, got, err, len(port.calls), step.block, i+1)
		}
		if step.block && !reflect.DeepEqual(reviewsOf(got.Classify), []string{secondReview}) {
			t.Errorf("stop %d names %v, want only %s", i+1, reviewsOf(got.Classify), secondReview)
		}
	}
	for i, calls := range port.calls {
		if !reflect.DeepEqual(calls, []string{otherReview, secondReview}) {
			t.Errorf("call %d asked %q, want both results in order", i+1, calls)
		}
	}
}

// M13: a port that gives another number of answers than it was asked is an error with no block, and nothing is recorded.
func TestStopRefusesAnswersThatDoNotMatchItsResults(t *testing.T) {
	for _, tc := range []struct {
		name    string
		answers []bool
		want    string
	}{
		{"none", []bool{}, "gave 0 answers for 1 results"},
		{"too many", []bool{false, false}, "gave 2 answers for 1 results"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store, _ := classifyWorld(t)
			asked, port := newFakeAsked(), newFakeClassified()
			port.answers = tc.answers

			got, err := app.New(g, store).WithAsked(asked).WithClassified(port).Stop(evidenceDir, "session-1", false)

			if got.Block || err == nil || !strings.Contains(err.Error(), tc.want) || len(asked.put) != 0 {
				t.Errorf("Stop() = %+v, %v with %v remembered; want no block, an error naming %q and nothing remembered", got, err, asked.put, tc.want)
			}
		})
	}
}

// M14: the session keeps only its questions (`5276daf542e3-2` F3): whatever the port says at ten stops, the store holds
// at most one classify- key for each kept result.
func TestStopKeepsOnlyItsQuestionsInTheSession(t *testing.T) {
	g, store, _ := classifyWorld(t)
	reviewed(t, store, baseSHA, otherSHA, 2, warning("F2"))
	asked, port := newFakeAsked(), newFakeClassified()
	service := app.New(g, store).WithAsked(asked).WithClassified(port)

	for i := 0; i < 10; i++ {
		port.answers = []bool{i%2 == 0, i%2 == 1}
		if _, err := service.Stop(evidenceDir, "session-1", false); err != nil {
			t.Fatalf("stop %d: %v", i+1, err)
		}
	}

	for key := range asked.put {
		if !strings.HasPrefix(key, "session-1 classify-") {
			t.Errorf("the store holds %q, want only classify- keys", key)
		}
	}
	if len(asked.put) > 2 {
		t.Errorf("the store holds %d keys, want at most two", len(asked.put))
	}
}

// namesNine says whether a stop's error names the refused file -9.json.
func namesNine(err error) bool {
	return err != nil && strings.Contains(err.Error(), "333333333333-9.json")
}

// refusedKeysOf is the refused- keys a store of the questions holds.
func refusedKeysOf(asked *fakeAsked) []string {
	var keys []string
	for key := range asked.put {
		if strings.Contains(key, " refused-") {
			keys = append(keys, key)
		}
	}
	return keys
}

// M15: a refusal counts as named only when a stop returned it beside its ruling (`83424e3c72e2-3` F1, the verifier's
// probe): a stop that ends in an error of the pass names it and records nothing, so the next stop names it again.
func TestStopNamesARefusalAgainAfterAStopThatFailed(t *testing.T) {
	world := func(t *testing.T) (*fakeGit, *fakeResults) {
		g, store, _ := classifyWorld(t)
		store.put("own-rdd", "333333333333-9.json", []byte("not a result"))
		return g, store
	}
	// failThenHeal runs stop 1 as arranged, calls heal, and runs stops 2 and 3 of the same session.
	failThenHeal := func(t *testing.T, service app.Service, asked *fakeAsked, mustName string, heal func()) {
		t.Helper()
		first, firstErr := service.Stop(evidenceDir, "session-1", false)
		if first.Block || !namesNine(firstErr) || !strings.Contains(firstErr.Error(), mustName) {
			t.Errorf("stop 1 = %+v, %v; want no block and an error that names %q and -9.json", first, firstErr, mustName)
		}
		if keys := refusedKeysOf(asked); len(keys) != 0 {
			t.Errorf("stop 1 recorded %v, want no refused- key", keys)
		}
		heal()
		second, secondErr := service.Stop(evidenceDir, "session-1", false)
		if !second.Block || !namesNine(secondErr) {
			t.Errorf("stop 2 = %+v, %v; want a block and an error that names -9.json again", second, secondErr)
		}
		third, thirdErr := service.Stop(evidenceDir, "session-1", false)
		if third.Block || thirdErr != nil {
			t.Errorf("stop 3 = %+v, %v; want no block and no error", third, thirdErr)
		}
	}

	t.Run("(a) the port fails", func(t *testing.T) {
		g, store := world(t)
		asked, port := newFakeAsked(), newFakeClassified()
		port.err = errors.New("script failed")
		service := app.New(g, store).WithAsked(asked).WithClassified(port)

		failThenHeal(t, service, asked, "script failed", func() { port.err = nil })
	})
	t.Run("(b) the port gives no answer", func(t *testing.T) {
		g, store := world(t)
		asked, port := newFakeAsked(), newFakeClassified()
		port.answers = []bool{}
		service := app.New(g, store).WithAsked(asked).WithClassified(port)

		failThenHeal(t, service, asked, "gave 0 answers for 1 results", func() { port.answers = nil })
	})
	t.Run("(c) the questions of the session cannot be read", func(t *testing.T) {
		g, store := world(t)
		fake := newFakeAsked()
		failing := &hasFailsWithPrefix{fakeAsked: fake, prefix: "classify-", err: errors.New("disk gone")}
		service := app.New(g, store).WithAsked(failing).WithClassified(newFakeClassified())

		failThenHeal(t, service, fake, "disk gone", func() { failing.prefix = "never-" })
	})
	t.Run("(d) every result is classified", func(t *testing.T) {
		g, store := world(t)
		asked, port := newFakeAsked(), newFakeClassified()
		port.done[otherReview] = true
		service := app.New(g, store).WithAsked(asked).WithClassified(port)

		first, firstErr := service.Stop(evidenceDir, "session-1", false)
		second, secondErr := service.Stop(evidenceDir, "session-1", false)

		if first.Block || !namesNine(firstErr) || second.Block || secondErr != nil {
			t.Errorf("stops = %+v, %v then %+v, %v; want -9.json named at stop 1 with no block, and nothing at stop 2", first, firstErr, second, secondErr)
		}
	})
	t.Run("(e) no result has a kept finding", func(t *testing.T) {
		g, store := sweptWorld(t)
		reviewed(t, store, baseSHA, otherSHA, 1)
		store.put("own-rdd", "333333333333-9.json", []byte("not a result"))
		service := app.New(g, store).WithAsked(newFakeAsked()).WithClassified(newFakeClassified())

		first, firstErr := service.Stop(evidenceDir, "session-1", false)
		second, secondErr := service.Stop(evidenceDir, "session-1", false)

		if first.Block || !namesNine(firstErr) || second.Block || secondErr != nil {
			t.Errorf("stops = %+v, %v then %+v, %v; want -9.json named at stop 1 with no block, and nothing at stop 2", first, firstErr, second, secondErr)
		}
	})
}

// M16: a refusal is named once for each session and stored file, whatever its text (`83424e3c72e2-5` F3): a read error
// whose text changes at every stop is named at the first and recorded once; another file has a key of its own.
func TestStopNamesARefusalOnceForEachFileWhateverItsText(t *testing.T) {
	g, store, _ := classifyWorld(t)
	store.put("own-rdd", "333333333333-9.json", []byte("{}"))
	store.put("own-rdd", "333333333333-8.json", []byte("not a result"))
	asked := newFakeAsked()
	service := app.New(g, store).WithAsked(asked).WithClassified(newFakeClassified())

	for i := 1; i <= 3; i++ {
		store.fail["Read 333333333333-9.json"] = fmt.Errorf("transient failure #%d", i)
		_, err := service.Stop(evidenceDir, "session-1", false)
		if named := namesNine(err); named != (i == 1) {
			t.Errorf("stop %d names -9.json = %v (error %v), want %v", i, named, err, i == 1)
		}
		if err != nil && i > 1 && strings.Contains(err.Error(), "-8.json") {
			t.Errorf("stop %d names -8.json again: %v", i, err)
		}
	}

	if keys := refusedKeysOf(asked); len(keys) != 2 {
		t.Errorf("the store holds %d refused- keys %v, want two: one for each file", len(keys), keys)
	}
}
