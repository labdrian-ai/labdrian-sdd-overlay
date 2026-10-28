package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// The tests of `review record --role sweeper|polisher` (own-rdd T6b4, S6 c) and of the default role, which stays what
// `review record` always was.

// recordedRole is what `review record --role` prints on stdout.
type recordedRole struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// firedSweep leaves a file in the working tree and sweeps the world, so that the head has a fired sweep whose only
// candidate is "T3: ?? stray.txt".
func firedSweep(t *testing.T, w *reviewWorld, state string) {
	t.Helper()
	overlaytest.WriteFile(t, w.root, "stray.txt", "left behind\n")
	if _, stderr, exits := w.sweepRun(state, "", "sweep", "--feature", "own-rdd", "--base", w.base, "--head", w.head); !equalInts(exits, []int{0}) {
		t.Fatalf("setup: review sweep exit calls = %v, stderr = %q", exits, stderr)
	}
}

const strayReport = `{"commit":null,"candidates":[{"candidate":"T3: ?? stray.txt","disposition":"live","evidence":"the owner keeps it"}]}`

func TestRecordRoleSweeperStoresTheReportAndPrintsThePathAndTheDigest(t *testing.T) {
	w := newReviewWorld(t)
	state := t.TempDir()
	firedSweep(t, w, state)

	stdout, stderr, exits := w.sweepRun(state, strayReport, "record", "--role", "sweeper", "--feature", "own-rdd", "--base", w.base, "--head", w.head)

	if !equalInts(exits, []int{0}) {
		t.Fatalf("exit calls = %v, stderr = %q; want [0]", exits, stderr)
	}
	short := w.head[:12]
	stored, err := os.ReadFile(filepath.Join(w.results("own-rdd"), short+"-2.sweeper.json"))
	if err != nil {
		t.Fatalf("the sweeper record is not stored as number 2 (the sweep is 1): %v", err)
	}
	var got recordedRole
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
	}
	path := "odd/review-results/own-rdd/" + short + "-2.sweeper.json"
	if got.Path != path || got.SHA256 != sum(stored) || strings.Contains(stdout, "payload_kind") {
		t.Errorf("stdout = %s, want only the path %s and the digest of the stored file, and no role-handoff skeleton", stdout, path)
	}
	if !strings.Contains(stderr, "review record: stored "+path+" (sha256 "+sum(stored)+")") {
		t.Errorf("stderr = %q, want it to say where the record is stored", stderr)
	}
	var file struct {
		Kind  string `json:"kind"`
		Head  string `json:"head"`
		Sweep string `json:"sweep"`
	}
	if err := json.Unmarshal(stored, &file); err != nil || file.Kind != "sweeper" || file.Head != w.head || file.Sweep != short+"-1.sweep.json" {
		t.Errorf("stored file = %s (%v), want a sweeper record of the head that names %s-1.sweep.json", stored, err, short)
	}
}

func TestRecordRoleSweeperIsRefusedWithExitTwoWhenNoFiredSweepBindsTheHead(t *testing.T) {
	w := newReviewWorld(t)

	stdout, stderr, exits := w.run(strayReport, "record", "--role", "sweeper", "--feature", "own-rdd", "--base", w.base, "--head", w.head)

	if stdout != "" || !equalInts(exits, []int{2}) || !strings.HasPrefix(stderr, "error: review record: the reply is refused and nothing was stored: ") {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, a refusal, [2]", stdout, stderr, exits)
	}
	if names := listNames(t, w.results("own-rdd")); len(names) != 0 {
		t.Errorf("stored %v, want nothing", names)
	}
}

func TestRecordRoleSweeperIsRefusedWithExitTwoForACommitGitDoesNotHave(t *testing.T) {
	w := newReviewWorld(t)
	state := t.TempDir()
	firedSweep(t, w, state)
	missing := strings.Repeat("a", 40)
	report := `{"commit":"` + missing + `","candidates":[{"candidate":"T3: ?? stray.txt","disposition":"removed","evidence":"gone"}]}`

	stdout, stderr, exits := w.sweepRun(state, report, "record", "--role", "sweeper", "--feature", "own-rdd", "--base", w.base, "--head", w.head)

	if stdout != "" || !equalInts(exits, []int{2}) || !strings.HasPrefix(stderr, "error: review record: the reply is refused and nothing was stored: ") || !strings.Contains(stderr, missing) {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, a refusal naming the commit, [2]", stdout, stderr, exits)
	}
	if names := listNames(t, w.results("own-rdd")); len(names) != 1 || !strings.HasSuffix(names[0], "-1.sweep.json") {
		t.Errorf("stored %v, want only the sweep", names)
	}
}

func TestRecordRolePolisherStoresTheFindingsOfOneJudgeAndReadsAResultFile(t *testing.T) {
	w := newReviewWorld(t)
	findings := `{"findings":[{"severity":"nit","evidence":["read it"]}]}`
	file := filepath.Join(t.TempDir(), "judge.json")
	if err := os.WriteFile(file, []byte(findings), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	stdout, stderr, exits := w.run("", "record", "--role", "polisher", "--feature", "own-rdd", "--base", w.base, "--head", w.head, "--result", file)

	if !equalInts(exits, []int{0}) {
		t.Fatalf("exit calls = %v, stderr = %q; want [0]", exits, stderr)
	}
	stored, err := os.ReadFile(filepath.Join(w.results("own-rdd"), w.head[:12]+"-1.polisher.json"))
	if err != nil {
		t.Fatalf("the polisher record is not stored: %v", err)
	}
	var got recordedRole
	if err := json.Unmarshal([]byte(stdout), &got); err != nil || got.SHA256 != sum(stored) || got.Path != "odd/review-results/own-rdd/"+w.head[:12]+"-1.polisher.json" {
		t.Errorf("stdout = %s (%v), want the path of the record and the digest of the stored file", stdout, err)
	}
}

func TestRecordRolePolisherIsRefusedWithExitTwoForFindingsThatAreNotTheObject(t *testing.T) {
	w := newReviewWorld(t)

	stdout, stderr, exits := w.run(`["not", "an", "object"]`, "record", "--role", "polisher", "--feature", "own-rdd", "--base", w.base, "--head", w.head)

	if stdout != "" || !equalInts(exits, []int{2}) || !strings.HasPrefix(stderr, "error: review record: the reply is refused and nothing was stored: ") {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, a refusal, [2]", stdout, stderr, exits)
	}
	if names := listNames(t, w.results("own-rdd")); len(names) != 0 {
		t.Errorf("stored %v, want nothing", names)
	}
}

func TestRecordRoleRefusesACommandLineItCannotReadWithExitOne(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"rows with the sweeper", []string{"record", "--role", "sweeper", "--feature", "f", "--base", "b", "--head", "h", "--rows", "default"}, "error: review record: --rows is only for --role reviewer\n"},
		{"rows with the polisher", []string{"record", "--role", "polisher", "--feature", "f", "--base", "b", "--head", "h", "--rows", "default"}, "error: review record: --rows is only for --role reviewer\n"},
		{"a role that does not exist", []string{"record", "--role", "judge", "--feature", "f", "--base", "b", "--head", "h"}, "error: review record: --role must be reviewer, sweeper or polisher, not \"judge\"\n"},
		{"an empty role", []string{"record", "--role", "", "--feature", "f", "--base", "b", "--head", "h"}, "error: review record: --role must be reviewer, sweeper or polisher, not \"\"\n"},
		{"the role twice", []string{"record", "--role", "sweeper", "--role", "polisher", "--feature", "f", "--base", "b", "--head", "h"}, "error: review record: --role given twice\n"},
		{"no feature for a role", []string{"record", "--role", "sweeper", "--base", "b", "--head", "h"}, "error: review record: --feature is required\n"},
		{"no head for a role", []string{"record", "--role", "polisher", "--feature", "f", "--base", "b"}, "error: review record: --head is required\n"},
		{"rows still required for the reviewer", []string{"record", "--role", "reviewer", "--feature", "f", "--base", "b", "--head", "h"}, "error: review record: --rows is required\n"},
		{"the role given to bind", []string{"bind", "--role", "sweeper", "--base", "b", "--head", "h"}, "error: review bind: unknown flag \"--role\"\n"},
		{"the role given to check", []string{"check", "--role", "sweeper", "--feature", "f"}, "error: review check: unknown flag \"--role\"\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newReviewWorld(t)

			stdout, stderr, exits := w.run("", tc.args...)

			if stdout != "" || stderr != tc.want || !equalInts(exits, []int{1}) {
				t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, %q, [1]", stdout, stderr, exits, tc.want)
			}
			if names := listNames(t, w.results("f")); len(names) != 0 {
				t.Errorf("stored %v for a command line that was refused", names)
			}
		})
	}
}

// The default role is today's `review record` byte for byte, whether or not --role reviewer is spelled out: two worlds
// are recorded the same reply, one with the flag and one without, and everything they print is the same once the names
// that belong to a world (its commits) are replaced.
func TestRecordWithRoleReviewerIsReviewRecordByteForByte(t *testing.T) {
	record := func(extra ...string) (stdout, stderr string, exits []int, stored string) {
		w := newReviewWorld(t)
		b := w.bind()
		args := append([]string{"record"}, extra...)
		args = append(args, "--feature", "own-rdd", "--base", w.base, "--head", w.head, "--rows", "default")
		out, errOut, exits := w.run(reply(t, b, nil), args...)
		data, err := os.ReadFile(filepath.Join(w.results("own-rdd"), w.head[:12]+"-1.json"))
		if err != nil {
			t.Fatalf("the result is not stored: %v", err)
		}
		same := strings.NewReplacer(w.head, "<head>", w.head[:12], "<head12>", w.base, "<base>")
		return same.Replace(out), same.Replace(errOut), exits, same.Replace(string(data))
	}

	defOut, defErr, defExits, defStored := record()
	roleOut, roleErr, roleExits, roleStored := record("--role", "reviewer")

	if defExits[0] != 0 || !equalInts(defExits, roleExits) {
		t.Fatalf("exits = %v and %v, want [0] twice", defExits, roleExits)
	}
	// The digest of the diff and the sha256 of the stored file differ with the commits of each world, so only what
	// does not depend on a digest is compared byte for byte.
	skeleton := func(s string) string {
		var out []string
		for _, line := range strings.Split(s, "\n") {
			if !strings.Contains(line, "sha256") && !strings.Contains(line, "\"ref\"") {
				out = append(out, line)
			}
		}
		return strings.Join(out, "\n")
	}
	if skeleton(defOut) != skeleton(roleOut) || skeleton(defErr) != skeleton(roleErr) || len(defStored) != len(roleStored) {
		t.Errorf("--role reviewer printed\nstdout %q\nstderr %q\nbut the default printed\nstdout %q\nstderr %q", roleOut, roleErr, defOut, defErr)
	}
}
