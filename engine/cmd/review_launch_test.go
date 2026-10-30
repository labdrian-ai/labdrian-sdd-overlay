package main

// `review launch` (own-rdd T8c, S9 a-d): the verb that starts the Codex reviewer read-only over the bound candidate and
// records its reply as `review record` does. The tests run it over the real git of a reviewWorld, with a HOME of their
// own that holds the Codex reviewer agent file the generator writes, and a fake in deps.reviewRunner: no test starts the
// real codex (TestNoTestOfThisPackageUsesTheProcessAdapter).

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/agentgen"
)

// fakeCodex is the codexexec.Runner of the tests. It records what it was asked to start and, as codex does, writes its
// reply to the path that follows --output-last-message.
type fakeCodex struct {
	calls int
	bin   string
	args  []string
	stdin string

	reply *string // the content of the last-message file, none when nil
	err   error
}

func (f *fakeCodex) OutputStdin(_ context.Context, _ []string, stdin []byte, bin string, args ...string) ([]byte, []byte, error) {
	f.calls++
	f.bin, f.args, f.stdin = bin, append([]string{}, args...), string(stdin)
	if f.reply != nil {
		for i, a := range args {
			if a == "--output-last-message" && i+1 < len(args) {
				if err := os.WriteFile(args[i+1], []byte(*f.reply), 0o644); err != nil {
					return nil, nil, err
				}
			}
		}
	}
	return nil, []byte("codex said something"), f.err
}

// launchWorld is a reviewWorld whose HOME holds the Codex reviewer agent file of this checkout.
type launchWorld struct {
	*reviewWorld
	fake *fakeCodex
	home string
}

func newLaunchWorld(t *testing.T, withReviewerFile bool) *launchWorld {
	t.Helper()
	w := &launchWorld{reviewWorld: newReviewWorld(t), fake: &fakeCodex{}, home: t.TempDir()}
	t.Setenv("HOME", w.home)
	if withReviewerFile {
		artifacts, err := agentgen.Artifacts(filepath.Join("..", ".."))
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range artifacts {
			if a.Path == "codex/agents/reviewer.toml" {
				dir := filepath.Join(w.home, ".codex", "agents")
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "reviewer.toml"), []byte(a.Content), 0o644); err != nil {
					t.Fatal(err)
				}
				return w
			}
		}
		t.Fatal("agentgen.Artifacts has no codex/agents/reviewer.toml")
	}
	return w
}

// launch runs `review launch <args>` in-process over the fake runner.
func (w *launchWorld) launch(stdin string, args ...string) (stdout, stderr string, exits []int) {
	w.t.Helper()
	p := newCapturedProcess()
	p.stdin = strings.NewReader(stdin)
	d := testDeps()
	d.getwd = func() (string, error) { return w.cwd, nil }
	d.environ = goldenEnvironment
	d.reviewRunner = w.fake
	run(p.process, d, append([]string{"review", "launch"}, args...))
	return p.out.String(), p.err.String(), p.exits
}

// args are the arguments of a well-formed launch of the candidate of the world.
func (w *launchWorld) args(b reviewBinding) []string {
	return []string{"--runtime", "codex", "--feature", "f", "--base", b.Base, "--head", b.Head, "--rows", "default", "--model", "gpt-5.5"}
}

// brief is a brief that names the whole binding.
func briefOf(b reviewBinding) string {
	return "Review " + b.Base + ".." + b.Head + " (diff_sha256 " + b.DiffSHA256 + ") in " + b.Worktree + "\n"
}

func (w *launchWorld) storedNothing() bool { return len(listNames(w.t, w.results("f"))) == 0 }

func TestLaunchRunsTheReviewerReadOnlyInTheWorktreeAndRecordsItsReply(t *testing.T) {
	w := newLaunchWorld(t, true)
	b := w.bind()
	good := reply(t, b, nil)
	w.fake.reply = &good

	stdout, stderr, exits := w.launch(briefOf(b), w.args(b)...)

	if w.fake.calls != 1 || w.fake.bin != "codex" {
		t.Fatalf("codex started %d times as %q; want once as codex", w.fake.calls, w.fake.bin)
	}
	a := w.fake.args
	if len(a) != 13 || strings.Join(a[:10], " ") != "exec --sandbox read-only --model gpt-5.5 --cd "+b.Worktree+" --skip-git-repo-check --ephemeral --ignore-user-config" ||
		a[10] != "--output-last-message" || filepath.Base(a[11]) != "last-message.txt" || a[12] != "-" {
		t.Errorf("codex arguments = %q; want those of S9 (b) over the worktree %s", a, b.Worktree)
	}
	if !strings.HasSuffix(w.fake.stdin, "\n\n"+briefOf(b)) {
		t.Errorf("codex stdin = %q; want the instructions, a blank line and the brief", w.fake.stdin)
	}
	if !equalInts(exits, []int{0}) {
		t.Fatalf("exits = %v, stderr = %q; want [0]", exits, stderr)
	}
	head12 := b.Head[:12]
	if names := listNames(t, w.results("f")); len(names) != 1 || names[0] != head12+"-1.json" {
		t.Errorf("stored %v; want [%s-1.json]", names, head12)
	}
	// The twin world records the same reply with `review record`: stdout and stderr are byte for byte the same.
	twin := newReviewWorld(t)
	tb := twin.bind()
	if tb.Head != b.Head || tb.DiffSHA256 != b.DiffSHA256 {
		t.Skipf("the twin world is not the same repository (head %s, want %s)", tb.Head, b.Head)
	}
	wantOut, wantErr, wantExits := twin.run(good, "record", "--feature", "f", "--base", b.Base, "--head", b.Head, "--rows", "default")
	if !equalInts(wantExits, []int{0}) || stdout != wantOut || stderr != wantErr {
		t.Errorf("launch printed %q / %q; review record prints %q / %q", stdout, stderr, wantOut, wantErr)
	}
}

func TestLaunchRefusesARuntimeOtherThanCodex(t *testing.T) {
	for _, runtime := range []string{"claude", "pi", ""} {
		t.Run("runtime "+runtime, func(t *testing.T) {
			w := newLaunchWorld(t, true)
			b := w.bind()
			args := w.args(b)
			args[1] = runtime

			stdout, stderr, exits := w.launch(briefOf(b), args...)

			want := "error: review launch: --runtime must be codex: on Claude Code and Pi the parent launches the reviewer agent natively\n"
			if stdout != "" || stderr != want || !equalInts(exits, []int{1}) || w.fake.calls != 0 || !w.storedNothing() {
				t.Errorf("stdout = %q, stderr = %q, exits = %v, codex calls = %d; want nothing, %q, [1], 0", stdout, stderr, exits, w.fake.calls, want)
			}
		})
	}
}

func TestLaunchRefusesABriefOfAnotherCandidate(t *testing.T) {
	w := newLaunchWorld(t, true)
	b := w.bind()
	brief := strings.ReplaceAll(briefOf(b), b.Worktree, "/elsewhere")

	stdout, stderr, exits := w.launch(brief, w.args(b)...)

	want := "error: review launch: the brief does not name the worktree " + b.Worktree + " of the binding, so it is not the brief of this candidate\n"
	if stdout != "" || stderr != want || !equalInts(exits, []int{1}) || w.fake.calls != 0 || !w.storedNothing() {
		t.Errorf("stdout = %q, stderr = %q, exits = %v, codex calls = %d; want nothing, %q, [1], 0", stdout, stderr, exits, w.fake.calls, want)
	}
}

func TestLaunchStoresNothingWhenCodexDoesNotAnswer(t *testing.T) {
	w := newLaunchWorld(t, true)
	b := w.bind()
	w.fake.err = os.ErrDeadlineExceeded

	stdout, stderr, exits := w.launch(briefOf(b), w.args(b)...)

	if stdout != "" || !equalInts(exits, []int{1}) || !strings.HasPrefix(stderr, "error: review launch: ") || !strings.Contains(stderr, "codex") || !w.storedNothing() {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, error: review launch: ... codex ..., [1], nothing stored", stdout, stderr, exits)
	}
}

func TestLaunchRefusesAReplyAsRecordDoes(t *testing.T) {
	w := newLaunchWorld(t, true)
	b := w.bind()
	other := reply(t, b, func(o map[string]any) { o["candidate"].(map[string]any)["diff_sha256"] = strings.Repeat("0", 64) })
	w.fake.reply = &other

	stdout, stderr, exits := w.launch(briefOf(b), w.args(b)...)

	if stdout != "" || !equalInts(exits, []int{2}) || !strings.HasPrefix(stderr, "error: review launch: ") || !w.storedNothing() {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, error: review launch: ..., [2], nothing stored", stdout, stderr, exits)
	}
}

func TestLaunchNamesTheMissingCodexReviewer(t *testing.T) {
	w := newLaunchWorld(t, false)
	b := w.bind()

	stdout, stderr, exits := w.launch(briefOf(b), w.args(b)...)

	if stdout != "" || !equalInts(exits, []int{1}) || !strings.Contains(stderr, "run: labdrian-overlay apply --target codex") || w.fake.calls != 0 || !w.storedNothing() {
		t.Errorf("stdout = %q, stderr = %q, exits = %v, codex calls = %d; want nothing, the apply command, [1], 0", stdout, stderr, exits, w.fake.calls)
	}
}

// A command line the verb cannot read is a usage error: exit 1, one line, before git is asked anything.
func TestReviewLaunchRefusesACommandLineItCannotRead(t *testing.T) {
	full := []string{"--runtime", "codex", "--feature", "f", "--base", "a", "--head", "b", "--rows", "default"}
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"no model", full, "error: review launch: --model is required\n"},
		{"the model twice", append(append([]string{}, full...), "--model", "m", "--model", "n"), "error: review launch: --model given twice\n"},
		{"an empty model", append(append([]string{}, full...), "--model", ""), "error: review launch: --model is required\n"},
		{"a model with no value", append(append([]string{}, full...), "--model"), "error: review launch: --model requires a value\n"},
		{"no runtime", []string{"--feature", "f", "--base", "a", "--head", "b", "--rows", "default", "--model", "m"}, "error: review launch: --runtime is required\n"},
		{"an unknown flag", append(append([]string{}, full...), "--model", "m", "--onto", "x"), "error: review launch: unknown flag \"--onto\"\n"},
		{"a positional argument", append(append([]string{}, full...), "--model", "m", "extra"), "error: review launch: unexpected argument \"extra\"\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newLaunchWorld(t, true)

			stdout, stderr, exits := w.launch("", tc.args...)

			if stdout != "" || stderr != tc.want || !equalInts(exits, []int{1}) || w.fake.calls != 0 {
				t.Errorf("stdout = %q, stderr = %q, exits = %v, codex calls = %d; want nothing, %q, [1], 0", stdout, stderr, exits, w.fake.calls, tc.want)
			}
		})
	}
}

// --runtime and --model belong to `review launch` alone: every other review verb still refuses them as flags it does not
// know.
func TestOtherReviewVerbsStillRefuseRuntimeAndModel(t *testing.T) {
	for _, verb := range []string{"bind", "record", "verify", "check", "exempt", "sweep", "anchor"} {
		for _, flag := range [][]string{{"--runtime", "codex"}, {"--model", "x"}} {
			t.Run(verb+" "+flag[0], func(t *testing.T) {
				w := newCheckedWorld(t)

				stdout, stderr, exits := w.run("", append([]string{verb}, flag...)...)

				want := "error: review " + verb + ": unknown flag \"" + flag[0] + "\"\n"
				if stdout != "" || stderr != want || !equalInts(exits, []int{1}) {
					t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, %q, [1]", stdout, stderr, exits, want)
				}
			})
		}
	}
}
