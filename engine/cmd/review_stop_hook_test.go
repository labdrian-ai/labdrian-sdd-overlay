package main

// What 'review stop-hook' decides at the stop of an agent (own-rdd T6a, S6, L18). The tests run the verb over a repository
// they make in a temporary directory, as the tests of the other review verbs do, with the state home the hook keeps its
// questions in under a temporary directory of its own, and hand it the JSON Claude Code hands a Stop hook. The rules
// themselves (which commit is next, when a question is asked once) are tested in review, review/app and review/fsstore;
// these tests hold what a caller sees: the stop that is blocked and the words of the block, the stop that is not, and the
// exit code of the hook, which is 0 whatever goes wrong because any other would block the stop.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/hookwire"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// stopWorld is a review world on a branch of its own, feature/stop, with two commits the default branch lacks, in a tree
// that keeps its features: the repository an agent stops in after it worked on a branch.
type stopWorld struct {
	*reviewWorld
	home   string // the state home the hook keeps its questions in
	c3, c4 string // the two commits of the branch, oldest first; the branch was made on w.head
}

// newStopWorld is the world where each commit of the branch carries the trailer Risk: none and a clean sweep, so it lacks
// only its review (T6b6b).
func newStopWorld(t *testing.T) *stopWorld {
	t.Helper()
	return newStopWorldWith(t, func(s *stopWorld) { s.sweepAll() })
}

// newStopWorldWith makes the branch and its two commits, runs before (nil runs nothing) and then writes the task document,
// an untracked line of the working tree that fires any sweep run after it (T6b6c, L70). A case builds the state it names
// in before.
func newStopWorldWith(t *testing.T, before func(s *stopWorld)) *stopWorld {
	t.Helper()
	w := newReviewWorld(t)
	w.git("switch", "-q", "-c", "feature/stop")
	s := &stopWorld{reviewWorld: w, home: t.TempDir()}
	s.c3 = w.commitMore("c3.txt")
	s.c4 = w.commitMore("c4.txt")
	if before != nil {
		before(s)
	}
	overlaytest.WriteFile(t, w.root, "odd/tasks/own-rdd.md", "# own-rdd\n")
	return s
}

// recommit replaces the two commits of the branch by a c3 whose message holds the given paragraphs after its subject (none:
// no trailer) and a c4 that declares Risk: none; s.c3 and s.c4 name the new commits.
func (s *stopWorld) recommit(trailer ...string) {
	s.t.Helper()
	s.git("reset", "-q", "--hard", s.head)
	overlaytest.WriteFile(s.t, s.root, "c3.txt", "c3.txt\n")
	s.git("add", "-A")
	args := []string{"commit", "-q", "-m", "c3.txt"}
	for _, line := range trailer {
		args = append(args, "-m", line)
	}
	s.git(args...)
	s.c3 = s.name("HEAD")
	s.c4 = s.commitMore("c4.txt")
}

// blockLine is the whole stdout of a hook that blocks a stop for the reason: one JSON object and a line feed, with <, > and
// & as they are.
func blockLine(t *testing.T, reason string) string {
	t.Helper()
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}{"block", reason}); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// requireReason requires the hook to block the stop of the session with exactly the reason: the whole stdout, exit 0 and
// nothing on stderr.
func (s *stopWorld) requireReason(session, reason string) {
	s.t.Helper()
	stdout, stderr, exits := s.stop(session, false)
	if want := blockLine(s.t, reason); stdout != want || stderr != "" || !equalInts(exits, []int{0}) {
		s.t.Errorf("stdout = %q, stderr = %q, exits = %v; want stdout %q, nothing on stderr and [0]", stdout, stderr, exits, want)
	}
}

// blockReasonOf is the reason of a block for the commit head after base, with the lines in the order given. The words are the
// ones S6 (e) gives, written out here and not taken from the code.
func blockReasonOf(head, base string, lines ...string) string {
	return "Commit " + head + " of branch feature/stop has unfinished review steps. Before you stop, for that commit alone (base " + base + "):\n\n" +
		strings.Join(lines, "\n") + "\n\n" +
		"<feature> is the feature whose odd/tasks/<feature>.md holds the task of the commit. A record counts once it is stored and binds the commit, " +
		"whatever its outcome; 'labdrian review check' judges the outcome. The stop hook asks for each thing a commit misses once for each session and commit."
}

const (
	riskLineNoTrailer = `- Risk: it has no "Risk:" trailer in the final trailer block of its message. End the commit message with a final trailer block that holds the one line "Risk: none" or "Risk: <items>" (for example "Risk: 3,5"); when the commit is HEAD: git commit --amend --trailer "Risk: <none|items>". The rest of what the commit needs is asked once it declares its risk.`
	exemptClause      = `; or, since the commit declares Risk: none, 'labdrian review exempt --risk none' it`
)

func sweepLine(base, head string) string {
	return "- Sweep: labdrian review sweep --feature <feature> --base " + base + " --head " + head
}

func reviewLine(base, head, clause string) string {
	return "- Review: labdrian review bind --base " + base + " --head " + head + ", then 'labdrian review record' the reply of the reviewer and 'labdrian review verify' it" + clause
}

// hookDeps is the deps the hook runs over: the working directory of the world, the environment of its git and the state
// home of the world.
func (s *stopWorld) hookDeps() deps {
	d := testDeps()
	d.getwd = func() (string, error) { return s.cwd, nil }
	d.environ = goldenEnvironment
	d.getenv = environmentOf(map[string]string{"XDG_STATE_HOME": s.home})
	return d
}

// stopInput is the JSON Claude Code hands a Stop hook.
func stopInput(session, cwd string, active bool) string {
	return fmt.Sprintf(`{"session_id":%q,"cwd":%q,"hook_event_name":"Stop","stop_hook_active":%t,"transcript_path":"/x/y.jsonl"}`, session, cwd, active)
}

// runStopHook runs `review stop-hook args` in-process over d, handed input on stdin.
func runStopHook(d deps, input string, args ...string) (stdout, stderr string, exits []int) {
	p := newCapturedProcess()
	p.stdin = strings.NewReader(input)
	run(p.process, d, append([]string{"review", "stop-hook"}, args...))
	return p.out.String(), p.err.String(), p.exits
}

// stop runs the hook at a stop of the session's agent.
func (s *stopWorld) stop(session string, active bool) (stdout, stderr string, exits []int) {
	s.t.Helper()
	return runStopHook(s.hookDeps(), stopInput(session, s.root, active))
}

// blocked stops the agent of the session and requires the stop to be blocked. It returns the reason of the block.
func (s *stopWorld) blocked(session string) string {
	s.t.Helper()
	stdout, stderr, exits := s.stop(session, false)
	var fields map[string]string
	if err := json.Unmarshal([]byte(stdout), &fields); err != nil || !strings.HasSuffix(stdout, "}\n") || strings.Count(stdout, "\n") != 1 {
		s.t.Fatalf("stdout = %q (%v), stderr = %q, exits = %v; want the one JSON line that blocks the stop", stdout, err, stderr, exits)
	}
	if !equalInts(exits, []int{0}) || stderr != "" || len(fields) != 2 || fields["decision"] != "block" || fields["reason"] == "" {
		s.t.Fatalf("stdout = %q, stderr = %q, exits = %v; want {\"decision\":\"block\",\"reason\":...}, nothing on stderr and [0]", stdout, stderr, exits)
	}
	return fields["reason"]
}

// allowsTheStop requires a run of the hook to let the agent stop: exit 0 and nothing on either stream.
func allowsTheStop(t *testing.T, stdout, stderr string, exits []int) {
	t.Helper()
	if stdout != "" || stderr != "" || !equalInts(exits, []int{0}) {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing on either stream and [0]", stdout, stderr, exits)
	}
}

// allowed stops the agent of the session and requires the stop to happen.
func (s *stopWorld) allowed(session string) {
	s.t.Helper()
	stdout, stderr, exits := s.stop(session, false)
	allowsTheStop(s.t, stdout, stderr, exits)
}

// asked lists the questions the hook remembers for the session: the names in its directory of the state home.
func (s *stopWorld) asked(session string) []string {
	s.t.Helper()
	return listNames(s.t, filepath.Join(s.home, "labdrian", "review-stop", sum([]byte(session))))
}

// requireNothingRemembered fails the test when the hook made anything in the state home.
func (s *stopWorld) requireNothingRemembered(t *testing.T) {
	t.Helper()
	if _, err := os.Lstat(filepath.Join(s.home, "labdrian")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the state home holds %s/labdrian (%v), want nothing made", s.home, err)
	}
}

// bindCommand is the command the block names for the commit after base.
func bindCommand(base, head string) string {
	return "labdrian review bind --base " + base + " --head " + head
}

// A stop is blocked when the branch has a commit no review covers, and the block names the command that binds the next
// one: the commit after the last that a review reached, here the first of the branch, and not the whole branch.
func TestReviewStopHookBlocksAStopWhoseNextCommitHasNoReview(t *testing.T) {
	s := newStopWorld(t)
	stored := listNames(t, s.results("own-rdd")) // the sweeps the world made

	reason := s.blocked("s1")

	if want := bindCommand(s.head, s.c3); !strings.Contains(reason, want) {
		t.Errorf("the reason is %q, want it to name %q", reason, want)
	}
	if strings.Contains(reason, s.c4) {
		t.Errorf("the reason is %q, want it to ask for the next commit only, and not name the later %s", reason, s.c4)
	}
	for _, want := range []string{"feature/stop", "labdrian review record", "labdrian review verify", "labdrian review exempt", "once for each session and commit"} {
		if !strings.Contains(reason, want) {
			t.Errorf("the reason is %q, want it to say %q", reason, want)
		}
	}
	if got, want := s.asked("s1"), []string{s.head[:12] + "-" + s.c3[:12] + "-review"}; !reflect.DeepEqual(got, want) {
		t.Errorf("the questions remembered for the session = %q, want %q", got, want)
	}
	if names := listNames(t, s.results("own-rdd")); !reflect.DeepEqual(names, stored) {
		t.Errorf("the results directory holds %q after the hook, want it to store nothing and hold %q", names, stored)
	}
	if _, err := os.Lstat(filepath.Join(s.parent, "repo-worktrees")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a worktree was made (%v), want the hook to bind nothing: the agent runs the command", err)
	}
}

// The question is put once for each session: the stop that follows it is not blocked, and another session is asked.
func TestReviewStopHookAsksEachQuestionOnceForEachSession(t *testing.T) {
	s := newStopWorld(t)
	question := []string{s.head[:12] + "-" + s.c3[:12] + "-review"}

	first := s.blocked("s1")
	s.allowed("s1")
	other := s.blocked("s2")

	if other != first {
		t.Errorf("the reason for the second session is %q, want the reason of the first, %q", other, first)
	}
	for _, session := range []string{"s1", "s2"} {
		if got := s.asked(session); !reflect.DeepEqual(got, question) {
			t.Errorf("the questions remembered for %s = %q, want %q", session, got, question)
		}
	}
}

// Each thing a commit misses is asked once for each session (T6b6d, L70): a sweep that fires after the first block makes
// the commit miss its sweeper, and that is asked in the same session, once.
func TestReviewStopHookAsksTheSweeperOfACommitInTheSameSessionOnceItsSweepFires(t *testing.T) {
	s := newStopWorld(t)
	prefix := s.head[:12] + "-" + s.c3[:12] + "-"
	if reason := s.blocked("s1"); !strings.Contains(reason, bindCommand(s.head, s.c3)) {
		t.Fatalf("the first reason is %q, want it to ask for %s", reason, s.c3)
	}

	// The task document is untracked, so T3 fires.
	stdout, stderr, exits := s.sweepRun(t.TempDir(), "", "sweep", "--feature", "own-rdd", "--base", s.head, "--head", s.c3)
	if !equalInts(exits, []int{0}) || decodeSwept(t, stdout).Outcome != "fired" {
		t.Fatalf("setup: review sweep exit calls = %v, stdout = %q, stderr = %q; want [0] and a fired sweep", exits, stdout, stderr)
	}
	reason := s.blocked("s1")

	if want := bindCommand(s.head, s.c3); !strings.Contains(reason, want) {
		t.Errorf("the second reason is %q, want it to name %q", reason, want)
	}
	if got, want := s.asked("s1"), []string{prefix + "review", prefix + "sweeper"}; !reflect.DeepEqual(got, want) {
		t.Errorf("the questions remembered for the session = %q, want %q", got, want)
	}
	s.allowed("s1")
}

// The reason lists, for the first commit that misses anything, the line of each thing it misses and nothing else (T6b6c,
// S6 e, L63, L66, L70). Each case builds the state it names before the task document is written, which is an untracked
// file: a sweep run in the case fires on it only when the case says so.
func TestReviewStopHookListsEachThingTheCommitMisses(t *testing.T) {
	t.Run("a head with no trailer", func(t *testing.T) {
		s := newStopWorldWith(t, func(s *stopWorld) { s.recommit() })

		s.requireReason("s1", blockReasonOf(s.c3, s.head, riskLineNoTrailer, sweepLine(s.head, s.c3)))
	})
	t.Run("a head with Risk: none and no sweep", func(t *testing.T) {
		s := newStopWorldWith(t, nil)

		s.requireReason("s1", blockReasonOf(s.c3, s.head, sweepLine(s.head, s.c3), reviewLine(s.head, s.c3, exemptClause)))
	})
	t.Run("a head whose sweep fired and has no sweeper record", func(t *testing.T) {
		s := newStopWorldWith(t, func(s *stopWorld) {
			overlaytest.WriteFile(s.t, s.root, "stray.txt", "stray\n")
			stdout, stderr, exits := s.sweepRun(s.t.TempDir(), "", "sweep", "--feature", "own-rdd", "--base", s.head, "--head", s.c3)
			if !equalInts(exits, []int{0}) || decodeSwept(s.t, stdout).Outcome != "fired" {
				s.t.Fatalf("setup: review sweep exit calls = %v, stdout = %q, stderr = %q; want [0] and a fired sweep", exits, stdout, stderr)
			}
			if err := os.Remove(filepath.Join(s.root, "stray.txt")); err != nil {
				s.t.Fatal(err)
			}
		})
		record := "odd/review-results/own-rdd/" + s.c3[:12] + "-1.sweep.json"

		s.requireReason("s1", blockReasonOf(s.c3, s.head,
			"- Sweeper: the sweep record "+record+" fired; launch the sweeper with its candidates, then: labdrian review record --role sweeper --feature <feature> --base "+s.head+" --head "+s.c3,
			reviewLine(s.head, s.c3, exemptClause)))
	})
	t.Run("a head declaring Risk: 3,5 with a clean sweep", func(t *testing.T) {
		s := newStopWorldWith(t, func(s *stopWorld) {
			s.recommit("Risk: 3,5")
			s.sweepAll()
		})

		s.requireReason("s1", blockReasonOf(s.c3, s.head,
			"- Polisher: the commit declares Risk: 3,5; launch the polisher judges on the commit and record each: labdrian review record --role polisher --feature <feature> --base "+s.head+" --head "+s.c3,
			reviewLine(s.head, s.c3, "")))
	})
	t.Run("a head with everything but its review", func(t *testing.T) {
		s := newStopWorld(t)

		s.requireReason("s1", blockReasonOf(s.c3, s.head, reviewLine(s.head, s.c3, exemptClause)))
	})
}

// A passive commit is never asked: a commit that only adds the task document, above a branch whose commits a review
// reaches, lets the stop happen.
func TestReviewStopHookNeverAsksAPassiveCommit(t *testing.T) {
	s := newStopWorld(t)
	s.recordRange(s.head, s.c4)
	s.git("add", "odd/tasks/own-rdd.md")
	s.git("commit", "-q", "-m", "docs", "-m", "Risk: none")

	s.allowed("s1")

	s.requireNothingRemembered(t)
}

// A passive commit below an unreviewed one needs no record and is never asked: the first commit no review reaches is the
// one above it, and the question names the passive commit as its base (T6b6b, L72).
func TestReviewStopHookNeverAsksAPassiveCommitBelowAnUnreviewedOne(t *testing.T) {
	s := newStopWorldWith(t, func(s *stopWorld) {
		s.git("reset", "-q", "--hard", s.head)
		overlaytest.WriteFile(s.t, s.root, "odd/tasks/own-rdd.md", "# own-rdd\n")
		s.git("add", "-A")
		s.git("commit", "-q", "-m", "docs", "-m", "Risk: none")
		s.c3 = s.name("HEAD")
		s.c4 = s.commitMore("c4.txt")
		stdout, stderr, exits := s.sweepRun(s.t.TempDir(), "", "sweep", "--feature", "own-rdd", "--base", s.c3, "--head", s.c4)
		if !equalInts(exits, []int{0}) || decodeSwept(s.t, stdout).Outcome != "clean" {
			s.t.Fatalf("setup: review sweep of %s exit calls = %v, stdout = %q, stderr = %q; want [0] and a clean sweep", s.c4, exits, stdout, stderr)
		}
	})

	s.requireReason("s1", blockReasonOf(s.c4, s.c3, reviewLine(s.c3, s.c4, exemptClause)))

	if got, want := s.asked("s1"), []string{s.c3[:12] + "-" + s.c4[:12] + "-review"}; !reflect.DeepEqual(got, want) {
		t.Errorf("the hook remembers %v, want %v", got, want)
	}
	if stored, _ := filepath.Glob(filepath.Join(s.results("own-rdd"), s.c3[:12]+"-*")); len(stored) > 0 {
		t.Errorf("the passive commit %s has records %v, want none", s.c3, stored)
	}
}

// The question of a repository that does not tell its default branch is worded as it always was.
func TestReviewStopHookWordsTheNoDefaultBranchReasonAsItAlwaysDid(t *testing.T) {
	s := newStopWorld(t)
	s.git("branch", "-m", "main", "trunk")

	s.requireReason("s1", "The repository does not tell the default branch of branch feature/stop (origin/HEAD is not set, and there is no branch main or master), "+
		"so the commits of the branch that need a review cannot be told. Run 'git remote set-head origin --auto' (or make the default branch main) "+
		"and the stop hook will name the next commit to review. The stop hook asks this once for each session.")
}

// A stop the agent makes while a stop hook is already keeping it going is never blocked, and it does not use up the
// question: the next stop that is not a continuation is asked.
func TestReviewStopHookNeverBlocksWhileAStopHookIsMakingTheAgentContinue(t *testing.T) {
	s := newStopWorld(t)

	stdout, stderr, exits := s.stop("s1", true)

	allowsTheStop(t, stdout, stderr, exits)
	s.requireNothingRemembered(t)
	if reason := s.blocked("s1"); !strings.Contains(reason, bindCommand(s.head, s.c3)) {
		t.Errorf("the reason at the next stop is %q, want the question that the continuation did not ask", reason)
	}
}

// The next commit is the one after the last that a review reached (L18): once the first commit has a review the hook
// asks for the second, with the first as the base, and never for the first again.
func TestReviewStopHookAsksOnlyForTheCommitAfterTheLastOneAReviewReached(t *testing.T) {
	s := newStopWorld(t)
	s.recordRange(s.head, s.c3)

	reason := s.blocked("s1")

	if want := bindCommand(s.c3, s.c4); !strings.Contains(reason, want) {
		t.Errorf("the reason is %q, want it to name %q", reason, want)
	}
	if strings.Contains(reason, "--head "+s.c3) {
		t.Errorf("the reason is %q, want it not to ask again for %s, which has a review", reason, s.c3)
	}
}

// A stop is not blocked when a review reaches the last commit of the branch, whether it is one review of both commits
// or one for each.
func TestReviewStopHookAllowsAStopWhenAReviewReachesTheLastCommit(t *testing.T) {
	for _, tc := range []struct {
		name   string
		record func(s *stopWorld)
	}{
		{"one review of the whole branch", func(s *stopWorld) { s.recordRange(s.head, s.c4) }},
		{"one review of each commit", func(s *stopWorld) { s.recordRange(s.head, s.c3); s.recordRange(s.c3, s.c4) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newStopWorld(t)
			tc.record(s)

			s.allowed("s1")

			s.requireNothingRemembered(t)
		})
	}
}

// A head cleared by an exemption counts as reviewed.
func TestReviewStopHookCountsAnExemptionAsAReview(t *testing.T) {
	s := newStopWorld(t)
	if _, stderr, exits := s.exemptRange(s.head, s.c3); !equalInts(exits, []int{0}) {
		t.Fatalf("setup: review exempt exit calls = %v, stderr = %q; want [0]", exits, stderr)
	}

	reason := s.blocked("s1")

	if want := bindCommand(s.c3, s.c4); !strings.Contains(reason, want) {
		t.Errorf("the reason is %q, want it to name %q: the exempted commit is reviewed", reason, want)
	}
	if _, stderr, exits := s.exemptRange(s.c3, s.c4); !equalInts(exits, []int{0}) {
		t.Fatalf("setup: review exempt exit calls = %v, stderr = %q; want [0]", exits, stderr)
	}
	s.allowed("s2")
}

// A commit is reviewed when a stored result or exemption binds it, whatever its outcome: the hook asks whether it was
// reviewed, and 'review check' judges how the review turned out. A result the verifier reproduced as a blocker is
// not asked for again.
func TestReviewStopHookCountsAReviewThatBlocksAsAReview(t *testing.T) {
	s := newStopWorld(t)
	_, stored := s.recordBlocker(s.head, s.c3)
	if _, stderr, exits := s.run(verdictOn("reproduced"), "verify", "--feature", "own-rdd", "--stored", stored); !equalInts(exits, []int{0}) {
		t.Fatalf("setup: review verify exit calls = %v, stderr = %q; want [0]", exits, stderr)
	}

	reason := s.blocked("s1")

	if want := bindCommand(s.c3, s.c4); !strings.Contains(reason, want) {
		t.Errorf("the reason is %q, want it to name %q: the commit whose review blocks was reviewed", reason, want)
	}
}

// There is nothing to ask on the default branch, on a detached HEAD, and in a tree that keeps no features, which is every
// repository the user works in that is not run by this workflow: the stop happens and nothing is remembered.
func TestReviewStopHookAllowsAStopWhenThereIsNothingToAsk(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(s *stopWorld)
	}{
		{"on the default branch", func(s *stopWorld) { s.git("switch", "-q", "main") }},
		{"on a detached HEAD", func(s *stopWorld) { s.git("checkout", "-q", "--detach") }},
		{"in a tree that keeps no features", func(s *stopWorld) {
			if err := os.RemoveAll(filepath.Join(s.root, "odd")); err != nil {
				s.t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newStopWorld(t)
			tc.setup(s)

			s.allowed("s1")

			s.requireNothingRemembered(t)
		})
	}
}

// A repository that does not tell its default branch cannot tell which commits of the branch need a review: that is the
// one question it puts, once for the session, and it says how to answer it.
func TestReviewStopHookBlocksOnceWhenTheRepositoryDoesNotTellItsDefaultBranch(t *testing.T) {
	s := newStopWorld(t)
	s.git("branch", "-m", "main", "trunk")

	reason := s.blocked("s1")
	s.allowed("s1")

	for _, want := range []string{"feature/stop", "git remote set-head origin --auto", "once"} {
		if !strings.Contains(reason, want) {
			t.Errorf("the reason is %q, want it to say %q", reason, want)
		}
	}
	if strings.Contains(reason, "labdrian review bind") {
		t.Errorf("the reason is %q, want no commit to bind: there is no telling which commits are the branch's own", reason)
	}
	if got, want := s.asked("s1"), []string{"no-default-branch"}; !reflect.DeepEqual(got, want) {
		t.Errorf("the questions remembered for the session = %q, want %q", got, want)
	}
}

// The hook works in the directory of the process when the input names none, and counts an input with no session as one
// session of its own.
func TestReviewStopHookReadsAMissingDirectoryAndAMissingSessionAsTheProcessAndOneSession(t *testing.T) {
	s := newStopWorld(t)
	input := `{"hook_event_name":"Stop"}`

	first, firstErr, firstExits := runStopHook(s.hookDeps(), input)
	second, secondErr, secondExits := runStopHook(s.hookDeps(), input)

	if !strings.Contains(first, bindCommand(s.head, s.c3)) || firstErr != "" || !equalInts(firstExits, []int{0}) {
		t.Errorf("the first stop: stdout = %q, stderr = %q, exits = %v; want the block that names %q", first, firstErr, firstExits, bindCommand(s.head, s.c3))
	}
	allowsTheStop(t, second, secondErr, secondExits)
	if got, want := s.asked(""), []string{s.head[:12] + "-" + s.c3[:12] + "-review"}; !reflect.DeepEqual(got, want) {
		t.Errorf("the questions remembered for the session with no name = %q, want %q", got, want)
	}
}

// Input the hook cannot use is a stop that happens, in silence: a hook that blocked what it cannot read would keep the
// agent from ever stopping.
func TestReviewStopHookIsSilentOnInputItCannotUse(t *testing.T) {
	s := newStopWorld(t)
	for _, tc := range []struct{ name, input string }{
		{"no input", ""},
		{"input that is not JSON", "stop"},
		{"JSON that is not an object", "[]"},
		{"a session that is not a string", `{"session_id":7,"cwd":` + fmt.Sprintf("%q", s.root) + `}`},
		{"the input of another event", strings.Replace(stopInput("s1", s.root, false), `"Stop"`, `"SessionStart"`, 1)},
		{"input over the bound", `{"session_id":"` + strings.Repeat("a", hookwire.MaxEnvelopeBytes) + `"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, exits := runStopHook(s.hookDeps(), tc.input)

			allowsTheStop(t, stdout, stderr, exits)
			s.requireNothingRemembered(t)
		})
	}
}

// endlessInput is a stdin that never ends and counts what is read of it. Past limit bytes it fails, as a pipe that breaks
// does, so that a hook that reads it to its end returns instead of hanging.
type endlessInput struct{ read, limit int }

func (e *endlessInput) Read(p []byte) (int, error) {
	if e.read >= e.limit {
		return 0, io.ErrUnexpectedEOF
	}
	n := min(len(p), e.limit-e.read)
	for i := 0; i < n; i++ {
		p[i] = 'a'
	}
	e.read += n
	return n, nil
}

// The hook reads no more of its input than the bound and one byte to see it is passed: an endless stdin is not read to
// its end, and the stop happens.
func TestReviewStopHookReadsNoMoreOfItsInputThanTheBound(t *testing.T) {
	s := newStopWorld(t)
	input := &endlessInput{limit: 8 * hookwire.MaxEnvelopeBytes}
	p := newCapturedProcess()
	p.stdin = input

	run(p.process, s.hookDeps(), []string{"review", "stop-hook"})

	allowsTheStop(t, p.out.String(), p.err.String(), p.exits)
	if want := hookwire.MaxEnvelopeBytes + 1; input.read != want {
		t.Errorf("the hook read %d bytes of an endless input, want %d", input.read, want)
	}
}

// What the hook cannot tell it does not block on: the stop happens, nothing is remembered, and one line on stderr says
// why, which Claude Code shows only in verbose mode.
func TestReviewStopHookNeverBlocksOnWhatItCannotTell(t *testing.T) {
	s := newStopWorld(t)
	outside := t.TempDir()
	for _, tc := range []struct {
		name  string
		deps  func() deps
		input string
		say   string
	}{
		{"a state home it cannot find", func() deps { d := s.hookDeps(); d.getenv = environmentOf(nil); return d }, stopInput("s1", s.root, false), "XDG_STATE_HOME is unset"},
		{"a directory outside every repository", s.hookDeps, stopInput("s1", outside, false), "locate the repository of " + outside},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, exits := runStopHook(tc.deps(), tc.input)

			if stdout != "" || !equalInts(exits, []int{0}) || !strings.HasPrefix(stderr, "review stop-hook: ") || !strings.Contains(stderr, tc.say) || strings.Count(stderr, "\n") != 1 {
				t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, one line of review stop-hook: ... that says %q, [0]", stdout, stderr, exits, tc.say)
			}
			s.requireNothingRemembered(t)
		})
	}
}

// A Go panic ends the process with status 2, which Claude Code reads as "block this stop": the hook catches it, says it on
// stderr and exits 0.
func TestReviewStopHookExitsZeroEvenWhenItPanics(t *testing.T) {
	s := newStopWorld(t)
	d := s.hookDeps()
	d.getenv = func(string) string { panic("boom") }

	stdout, stderr, exits := runStopHook(d, stopInput("s1", s.root, false))

	if stdout != "" || !equalInts(exits, []int{0}) || !strings.Contains(stderr, "review stop-hook: internal error: boom") {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, the panic on stderr and [0]", stdout, stderr, exits)
	}
}

// The hook takes no argument: the input is the JSON on stdin. A command line it cannot read is exit 1 with one line on
// stderr, before stdin is read.
func TestReviewStopHookTakesNoArguments(t *testing.T) {
	s := newStopWorld(t)
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"an argument", []string{"extra"}, "error: review stop-hook: unexpected argument \"extra\"\n"},
		{"a flag", []string{"--cwd", "x"}, "error: review stop-hook: unknown flag \"--cwd\"\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, exits := runStopHook(s.hookDeps(), stopInput("s1", s.root, false), tc.args...)

			if stdout != "" || stderr != tc.want || !equalInts(exits, []int{1}) {
				t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, %q, [1]", stdout, stderr, exits, tc.want)
			}
			s.requireNothingRemembered(t)
		})
	}
}

// The verb is in the help of the engine, with what a caller acts on. It is an internal hook command like 'skills
// guard-hook', so the lists of the verbs a person runs do not name it (TestReviewRefusesACommandLineItCannotRead).
func TestUsageDocumentsReviewStopHook(t *testing.T) {
	var out bytes.Buffer
	usage(&out)
	text := strings.Join(strings.Fields(out.String()), " ")
	for _, want := range []string{
		"engine review stop-hook",
		"internal Claude Code Stop hook command (install-hooks installs it",
		"blocks the stop with {\"decision\":\"block\",\"reason\":...}",
		"misses anything 'review check' asks of it",
		"is never asked",
		"labdrian review sweep",
		"for one commit and never the whole branch",
		"whatever its outcome",
		"once for each session and commit",
		"stop_hook_active",
		"never on what it cannot tell",
		"exit 0 always, 1 usage error",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the engine usage lacks %q", want)
		}
	}
}
