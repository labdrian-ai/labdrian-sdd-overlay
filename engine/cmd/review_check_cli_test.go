package main

// The command line of `review check` (own-rdd T4, S5): what the verb refuses before it asks git anything, what it
// does when its answer cannot be written, the help that documents it, and the built program that the delivery
// role runs. What it decides about a head is tested in review_check_test.go.

import (
	"bytes"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// A command line the verb cannot read is a usage error: exit 1 with one line on standard error, before git is asked
// anything. --head is optional and defaults to HEAD, but an empty one that was given is not the default.
func TestReviewCheckRefusesACommandLineItCannotRead(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"no feature", []string{"check"}, "error: review check: --feature is required\n"},
		{"an empty head", []string{"check", "--feature", "f", "--head", ""}, "error: review check: --head is required\n"},
		{"the head twice", []string{"check", "--feature", "f", "--head", "a", "--head", "b"}, "error: review check: --head given twice\n"},
		{"the feature twice", []string{"check", "--feature", "f", "--feature", "g"}, "error: review check: --feature given twice\n"},
		{"the working directory twice", []string{"check", "--feature", "f", "--cwd", "a", "--cwd", "b"}, "error: review check: --cwd given twice\n"},
		{"a flag with no value", []string{"check", "--feature"}, "error: review check: --feature requires a value\n"},
		{"a head with no value", []string{"check", "--feature", "f", "--head"}, "error: review check: --head requires a value\n"},
		{"the base given to check", []string{"check", "--feature", "f", "--base", "b"}, "error: review check: unknown flag \"--base\"\n"},
		{"the stored result given to check", []string{"check", "--feature", "f", "--stored", "s.json"}, "error: review check: unknown flag \"--stored\"\n"},
		{"a positional argument", []string{"check", "--feature", "f", "extra"}, "error: review check: unexpected argument \"extra\"\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newReviewWorld(t)

			stdout, stderr, exits := w.run("", tc.args...)

			if stdout != "" || stderr != tc.want || !equalInts(exits, []int{1}) {
				t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, %q, [1]", stdout, stderr, exits, tc.want)
			}
		})
	}
}

// A feature that is not one safe path segment is refused with the rule it breaks, and nothing leaves the results
// directory.
func TestCheckRefusesAFeatureThatIsNotOneSafePathSegment(t *testing.T) {
	w := newReviewWorld(t)

	stdout, stderr, exits := w.run("", "check", "--feature", "../own-rdd")

	if stdout != "" || !equalInts(exits, []int{1}) || !strings.HasPrefix(stderr, "error: review check: review: invalid feature name") {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, the named rule, [1]", stdout, stderr, exits)
	}
}

// Standard output is where the verb hands over the files it used. When it cannot be written (a pipe whose reader
// has gone) the parent never received that the head is cleared, and exit 0 would tell it that it did.
func TestCheckFailsWithExit1WhenTheAnswerCannotBeWritten(t *testing.T) {
	w := newReviewWorld(t)
	w.recordClean()
	p := newCapturedProcess()
	p.stdout = brokenStdout{errors.New("broken pipe")}

	w.runOn(p, "check", "--feature", "own-rdd")

	want := "error: review check: the head is cleared, but the answer could not be written: write to stdout: broken pipe\n"
	if !equalInts(p.exits, []int{1}) || p.err.String() != want || p.out.Len() != 0 {
		t.Errorf("exits = %v, stderr = %q, stdout = %q; want [1], %q and nothing", p.exits, p.err.String(), p.out.String(), want)
	}
}

// The verb is in the help of the engine, with what a caller acts on.
func TestUsageDocumentsReviewCheck(t *testing.T) {
	var out bytes.Buffer
	usage(&out)
	text := strings.Join(strings.Fields(out.String()), " ")

	for _, want := range []string{
		"engine review check --feature <name> [--head <rev>] [--cwd <repo>]",
		"recomputes from git the candidate of the base the result names and that head",
		"at least one result binds the head and every one is clear",
		"a result that blocks keeps the head blocked whatever was recorded after it",
		"a check that did not run never passes",
		"The delivery role runs it before it closes the feature",
		"exit 0 cleared, 1 usage error, refusal or any other failure",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("usage lacks %q", want)
		}
	}
}

// The built program: a head is refused until a stored clean result binds it, cleared by that result, and refused
// again once another launch of the same head has a finding nobody checked and once a verifier reproduces it. The
// streams and the exit codes are the ones the delivery role reads.
func TestReviewCheckOfTheBuiltProgramClearsAReviewedHeadAndRefusesAWrongOne(t *testing.T) {
	w := newReviewWorld(t)
	bin := engineBinary(t)
	run := func(stdin string, args ...string) (stdout, stderr string, code int) {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Dir = w.root
		cmd.Env = goldenEnvironment()
		cmd.Stdin = strings.NewReader(stdin)
		var o, e bytes.Buffer
		cmd.Stdout, cmd.Stderr = &o, &e
		err := cmd.Run()
		code = 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatalf("run %v: %v", args, err)
		}
		return o.String(), e.String(), code
	}

	if out, errText, code := run("", "review", "check", "--feature", "own-rdd"); code != 1 || out != "" || !strings.Contains(errText, "no stored result binds the head") {
		t.Fatalf("check of a head never reviewed: exit %d, stdout %q, stderr %q; want 1, nothing and the reason", code, out, errText)
	}
	out, errText, code := run("", "review", "bind", "--base", "HEAD~1", "--head", "HEAD")
	if code != 0 {
		t.Fatalf("review bind: exit %d, stderr %q", code, errText)
	}
	b := decodeBinding(t, out)
	if _, errText, code := run(reply(t, b, nil), "review", "record", "--feature", "own-rdd", "--base", "HEAD~1", "--head", "HEAD"); code != 0 {
		t.Fatalf("review record: exit %d, stderr %q", code, errText)
	}

	out, errText, code = run("", "review", "check", "--feature", "own-rdd")
	var cleared struct {
		Head    string `json:"head"`
		Outcome string `json:"outcome"`
		Results []struct {
			Result     string `json:"result"`
			Base       string `json:"base"`
			DiffSHA256 string `json:"diff_sha256"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(out), &cleared); err != nil || code != 0 || errText != "" {
		t.Fatalf("check of a reviewed head: exit %d, stderr %q, stdout %q (%v); want 0, nothing on stderr and a JSON object", code, errText, out, err)
	}
	if cleared.Head != w.head || cleared.Outcome != "clear" || len(cleared.Results) != 1 ||
		cleared.Results[0].Result != "odd/review-results/own-rdd/"+w.head[:12]+"-1.json" || cleared.Results[0].Base != w.base || cleared.Results[0].DiffSHA256 != b.DiffSHA256 {
		t.Errorf("check = %+v, want the head %s cleared by launch 1 of the base %s with the digest %s", cleared, w.head, w.base, b.DiffSHA256)
	}
	if again, _, code := run("", "review", "check", "--feature", "own-rdd", "--head", w.head, "--cwd", w.root); code != 0 || again != out {
		t.Errorf("check of the head by its full name: exit %d, stdout %q; want 0 and the same answer as for HEAD: %q", code, again, out)
	}

	if _, errText, code := run(reply(t, b, withBlocker), "review", "record", "--feature", "own-rdd", "--base", "HEAD~1", "--head", "HEAD"); code != 0 {
		t.Fatalf("record of a blocker: exit %d, stderr %q", code, errText)
	}
	if out, errText, code := run("", "review", "check", "--feature", "own-rdd"); code != 1 || out != "" || !strings.Contains(errText, "a stored result is not clear") || !strings.Contains(errText, "findings not checked F1") {
		t.Errorf("check with a finding nobody checked: exit %d, stdout %q, stderr %q; want 1, nothing and the reason", code, out, errText)
	}
	if _, errText, code := run(verdictOn("reproduced"), "review", "verify", "--feature", "own-rdd", "--stored", w.head[:12]+"-2.json"); code != 0 {
		t.Fatalf("verify: exit %d, stderr %q", code, errText)
	}
	if out, errText, code := run("", "review", "check", "--feature", "own-rdd"); code != 1 || out != "" || !strings.Contains(errText, "a stored result blocks the head") || !strings.Contains(errText, w.head[:12]+"-2.verifier.json") {
		t.Errorf("check with a blocking verdict: exit %d, stdout %q, stderr %q; want 1, nothing and the reason", code, out, errText)
	}
}
