package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewreceipt/receipttest"
)

// The golden files under testdata/review-receipt-golden record what the review-receipt
// verbs do: what 'review-receipt hook' (the fail-closed PreToolUse hook that runs before
// 'gentle-ai review acknowledge-approved') and 'review-receipt capture' print on stdout and
// stderr, which exit code they end with, and what they leave under openspec/ (every file
// with its mode, size and digest). The hook is the delivery path of every review: a receipt
// it fails to capture is burned by the acknowledgement that follows, so its output is
// pinned byte for byte.
//
// The verbs are run as the program: the test builds the engine binary once and runs it in a
// throwaway git repository for each case, so what is compared is what a person or a hook
// runner sees, not what a function returns. The files were recorded from the program as it
// was before engine/reviewreceipt was moved behind ports (Phase 9 unit H12, slice b), and a
// change to what a verb prints, to the files it writes, or to the modes of those files fails
// here. Rewrite them deliberately with
//
//	go test ./cmd -run TestReviewReceiptGolden -update-review-receipt-golden
//
// and read the diff before committing it.
//
// Each case is its own subtest, so a case that skips drops only its own comparison. The
// cases named git-failure-* are the ones whose text comes from git itself (the program is
// run where git cannot find a repository), which is the one place the wording is allowed to
// follow a change of how the repository is located.
var updateReviewReceiptGolden = flag.Bool("update-review-receipt-golden", false, "rewrite the golden files of the review-receipt verbs")

// The engine binary is built once per test run, from the package under test, and removed by
// TestMain when the run ends. The build is bounded by a deadline and tried again once, so a
// toolchain that hangs does not hang the run and a failure that will not repeat does not fail
// every case; one that keeps failing is reported in full by the first case and by a line
// pointing at it by the others.
var (
	reviewReceiptBinaryOnce sync.Once
	reviewReceiptBinaryPath string
	reviewReceiptBinaryDir  string
	reviewReceiptBinaryErr  error
	// reviewReceiptBuildReported says that a case already printed the failed build in full.
	reviewReceiptBuildReported atomic.Bool
)

const (
	// engineBuildAttempts is how many times the build is tried before it is reported.
	engineBuildAttempts = 2
	// engineBuildTimeout bounds one attempt. A cold build of the engine takes seconds.
	engineBuildTimeout = 5 * time.Minute
)

// buildWithRetry runs attempt (numbered from 1) until it succeeds or attempts have failed,
// and reports every failure it saw. At least one attempt is made.
func buildWithRetry(attempts int, attempt func(n int) error) error {
	if attempts < 1 {
		attempts = 1
	}
	var failures []string
	for n := 1; n <= attempts; n++ {
		err := attempt(n)
		if err == nil {
			return nil
		}
		failures = append(failures, fmt.Sprintf("attempt %d: %v", n, err))
	}
	return fmt.Errorf("the build failed %d times:\n%s", attempts, strings.Join(failures, "\n"))
}

func reviewReceiptBinary(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping the build of the engine binary under -short")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("go unavailable: %v", err)
	}
	reviewReceiptBinaryOnce.Do(func() {
		dir, err := os.MkdirTemp("", "engine-review-receipt-bin-*")
		if err != nil {
			reviewReceiptBinaryErr = err
			return
		}
		reviewReceiptBinaryDir = dir
		reviewReceiptBinaryPath = filepath.Join(dir, "engine")
		reviewReceiptBinaryErr = buildWithRetry(engineBuildAttempts, func(int) error {
			ctx, cancel := context.WithTimeout(context.Background(), engineBuildTimeout)
			defer cancel()
			build := exec.CommandContext(ctx, "go", "build", "-buildvcs=false", "-o", reviewReceiptBinaryPath, ".")
			out, err := build.CombinedOutput()
			if ctx.Err() != nil {
				return fmt.Errorf("go build did not finish within %s\n%s", engineBuildTimeout, out)
			}
			if err != nil {
				return fmt.Errorf("go build: %v\n%s", err, out)
			}
			return nil
		})
	})
	if reviewReceiptBinaryErr != nil {
		if reviewReceiptBuildReported.Swap(true) {
			t.Fatalf("the engine binary could not be built (the first case that needed it printed why)")
		}
		t.Fatalf("build the engine binary: %v", reviewReceiptBinaryErr)
	}
	return reviewReceiptBinaryPath
}

// removeReviewReceiptBinary is called by TestMain after the run.
func removeReviewReceiptBinary() {
	if reviewReceiptBinaryDir != "" {
		os.RemoveAll(reviewReceiptBinaryDir)
	}
}

// goldenEnvironment is the environment the program and the git that sets its repositories up
// run in: the test process's own, without any GIT_* variable (a git hook that runs the tests
// sets GIT_DIR, and the program refuses a repository it is redirected to), and with git told to
// read no configuration of the machine.
func goldenEnvironment() []string {
	var env []string
	for _, kv := range os.Environ() {
		if name, _, _ := strings.Cut(kv, "="); !strings.HasPrefix(name, "GIT_") {
			env = append(env, kv)
		}
	}
	return append(env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
}

// receiptWorld is the scratch space of one case: the program, the places it was told about
// (replaced by placeholders in the transcript so the text is the same wherever the test
// runs), and the transcript itself.
type receiptWorld struct {
	t      *testing.T
	bin    string
	names  map[string]string
	b      strings.Builder
	reader string // a directory that is not a repository, where the program is started
}

func newReceiptWorld(t *testing.T) *receiptWorld {
	t.Helper()
	return &receiptWorld{t: t, bin: reviewReceiptBinary(t), names: map[string]string{}, reader: t.TempDir()}
}

// name registers path to be written as placeholder in the transcript.
func (w *receiptWorld) name(path, placeholder string) {
	w.names[path] = placeholder
}

func (w *receiptWorld) text() string {
	text := w.b.String()
	paths := make([]string, 0, len(w.names))
	for p := range w.names {
		paths = append(paths, p)
	}
	// Longest first, so a path under another is replaced as itself.
	sort.Slice(paths, func(i, j int) bool { return len(paths[i]) > len(paths[j]) })
	for _, p := range paths {
		text = strings.ReplaceAll(text, p, w.names[p])
	}
	return text
}

func (w *receiptWorld) write(format string, args ...any) {
	fmt.Fprintf(&w.b, format, args...)
}

// run records one invocation of 'review-receipt <args>' with stdin: the command line, the
// exit code, and both output streams.
func (w *receiptWorld) run(stdin string, args ...string) {
	w.t.Helper()
	cmd := exec.Command(w.bin, append([]string{"review-receipt"}, args...)...)
	cmd.Dir = w.reader
	cmd.Env = goldenEnvironment()
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	code := 0
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			w.t.Fatalf("run review-receipt %v: %v", args, err)
		}
		code = exit.ExitCode()
	}
	w.write("$ review-receipt %s\n", strings.Join(args, " "))
	if stdin != "" {
		w.write("--- stdin ---\n%s", ensureNewline(stdin))
	}
	w.write("exit: %d\n--- stdout ---\n%s--- stderr ---\n%s\n", code, ensureNewline(stdout.String()), ensureNewline(stderr.String()))
}

// hook runs the hook for a Bash command, the way the runner hands it over.
func (w *receiptWorld) hook(cwd, command string) {
	w.t.Helper()
	w.run(acknowledgeHookJSON(command), "hook", "--cwd", cwd)
}

func acknowledgeHookJSON(command string) string {
	return fmt.Sprintf(`{"tool_name":"Bash","tool_input":{"command":%q}}`, command)
}

const acknowledgeCommand = "gentle-ai review acknowledge-approved --cwd /repo --lineage review-x"

// tree records what is under dir/openspec: every file with its mode, size and the start of
// its digest, in path order, and every directory.
func (w *receiptWorld) tree(label, dir string) {
	w.t.Helper()
	var lines []string
	base := filepath.Join(dir, "openspec")
	err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		switch {
		case d.IsDir():
			lines = append(lines, "dir  "+rel)
		default:
			info, err := d.Info()
			if err != nil {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(data)
			lines = append(lines, fmt.Sprintf("file %s %04o %d %s", rel, info.Mode().Perm(), info.Size(), hex.EncodeToString(sum[:])[:12]))
		}
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		w.write("=== %s ===\n(no openspec directory)\n\n", label)
		return
	}
	if err != nil {
		w.t.Fatalf("walk %s: %v", base, err)
	}
	w.write("=== %s ===\n%s\n\n", label, strings.Join(lines, "\n"))
}

// repo creates a git repository, with one commit unless unborn is set, and registers it as
// placeholder. The directory is the one git reports as its toplevel (symlinks resolved).
func (w *receiptWorld) repo(placeholder string, unborn bool) string {
	w.t.Helper()
	root, err := filepath.EvalSymlinks(w.t.TempDir())
	if err != nil {
		w.t.Fatal(err)
	}
	w.git(root, "init", "-q")
	if !unborn {
		w.git(root, "-c", "user.name=t", "-c", "user.email=t@example.invalid", "commit", "-q", "--allow-empty", "-m", "init")
	}
	w.name(root, placeholder)
	return root
}

func (w *receiptWorld) git(dir string, args ...string) {
	w.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = goldenEnvironment()
	if out, err := cmd.CombinedOutput(); err != nil {
		w.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// change creates openspec/changes/<name> under root with a tasks.md, which marks it active.
func (w *receiptWorld) change(root, name string) {
	w.t.Helper()
	w.put(filepath.Join(root, "openspec", "changes", name, "tasks.md"), "# tasks\n")
}

func (w *receiptWorld) put(path, content string) {
	w.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		w.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		w.t.Fatal(err)
	}
}

// store is the review-transaction store under a git directory.
func store(gitDir string) string {
	return filepath.Join(gitDir, "gentle-ai", "review-transactions", "v2")
}

// legacyReceipt and lifecycleState are the two documents gentle-ai leaves for a lineage, as
// the tests of the review receipt capture share them (engine/reviewreceipt/receipttest).
func legacyReceipt(lineage, schema, terminal string) string {
	return receipttest.ReceiptDocument(lineage, schema, terminal)
}

func lifecycleState(lineage, state string) string {
	return receipttest.StateDocument(lineage, state)
}

const (
	receiptSchemaV2 = receipttest.ReceiptSchema
	approved        = receipttest.Approved
)

// putReceipt and putState leave the legacy receipt, or the lifecycle state, of a lineage in
// the store under gitDir.
func (w *receiptWorld) putReceipt(gitDir, lineage, content string) {
	w.t.Helper()
	w.put(filepath.Join(store(gitDir), lineage, "review-receipt.json"), content)
}

func (w *receiptWorld) putState(gitDir, lineage, content string) {
	w.t.Helper()
	w.put(filepath.Join(store(gitDir), lineage, "review-state.json"), content)
}

// receiptGoldenCase is one scenario and the golden file its transcript is compared with.
type receiptGoldenCase struct {
	name string
	run  func(w *receiptWorld)
}

func receiptGoldenCases() []receiptGoldenCase {
	return []receiptGoldenCase{
		{"hook-passes-what-is-not-an-acknowledgement", func(w *receiptWorld) {
			root := w.repo("<ROOT>", false)
			w.change(root, "only-change")
			w.putReceipt(filepath.Join(root, ".git"), "review-untouched", legacyReceipt("review-untouched", receiptSchemaV2, approved))
			w.hook(root, "git status")
			w.run("not json", "hook", "--cwd", root)
			w.run("", "hook", "--cwd", root)
			w.run(`{"tool_name":"Bash"}`, "hook", "--cwd", root)
			w.run(`{"tool_input":{"command":42}}`, "hook", "--cwd", root)
			w.tree("after", root)
		}},
		{"hook-passes-without-an-openspec-directory", func(w *receiptWorld) {
			root := w.repo("<ROOT>", false)
			w.putReceipt(filepath.Join(root, ".git"), "review-orphan", legacyReceipt("review-orphan", receiptSchemaV2, approved))
			w.hook(root, acknowledgeCommand)
			w.tree("after", root)
		}},
		{"hook-passes-without-an-active-change", func(w *receiptWorld) {
			root := w.repo("<ROOT>", false)
			w.putReceipt(filepath.Join(root, ".git"), "review-orphan", legacyReceipt("review-orphan", receiptSchemaV2, approved))
			w.put(filepath.Join(root, "openspec", "changes", "archive", "tasks.md"), "# archived\n")
			w.put(filepath.Join(root, "openspec", "changes", "stray", "notes.txt"), "no marker\n")
			w.put(filepath.Join(root, "openspec", "changes", "a-file"), "not a directory\n")
			w.hook(root, acknowledgeCommand)
			w.tree("after", root)
		}},
		{"hook-captures-a-legacy-receipt", func(w *receiptWorld) {
			root := w.repo("<ROOT>", false)
			w.change(root, "only-change")
			w.putReceipt(filepath.Join(root, ".git"), "review-solo", legacyReceipt("review-solo", receiptSchemaV2, approved))
			w.hook(root, acknowledgeCommand)
			w.tree("after the first run", root)
			w.hook(root, "cd /x && "+acknowledgeCommand+" --json")
			w.tree("after the second run", root)
		}},
		{"hook-captures-a-lifecycle-state", func(w *receiptWorld) {
			root := w.repo("<ROOT>", false)
			w.change(root, "only-change")
			w.putState(filepath.Join(root, ".git"), "review-state-solo", lifecycleState("review-state-solo", approved))
			w.hook(root, acknowledgeCommand)
			w.tree("after the first run", root)
			w.hook(root, acknowledgeCommand)
			w.tree("after the second run", root)
		}},
		{"hook-captures-only-what-is-approved", func(w *receiptWorld) {
			root := w.repo("<ROOT>", false)
			gitDir := filepath.Join(root, ".git")
			w.change(root, "only-change")
			w.putReceipt(gitDir, "review-both", legacyReceipt("review-both", receiptSchemaV2, approved))
			w.putState(gitDir, "review-both", lifecycleState("review-both", approved))
			w.putState(gitDir, "review-state-only", lifecycleState("review-state-only", approved))
			w.putReceipt(gitDir, "review-wrong-schema", legacyReceipt("review-wrong-schema", "gentle-ai.review-receipt/v1", approved))
			w.putReceipt(gitDir, "review-declined", legacyReceipt("review-declined", receiptSchemaV2, "declined"))
			w.putState(gitDir, "review-reviewing", lifecycleState("review-reviewing", "reviewing"))
			w.putState(gitDir, "review-escalated", lifecycleState("review-escalated", "escalated"))
			w.putReceipt(gitDir, "review-garbage", "this is not json")
			w.putReceipt(gitDir, "review-no-lineage", legacyReceipt("", receiptSchemaV2, approved))
			w.putState(gitDir, "review-state-no-lineage", lifecycleState("", approved))
			w.put(filepath.Join(store(gitDir), "a-plain-file"), "not a lineage directory\n")
			w.hook(root, acknowledgeCommand)
			w.tree("after", root)
		}},
		// A review document that is there and cannot be read is never a silent absence: the
		// hook would count fewer approved receipts than exist and lose one. It denies, names
		// the document, and captures nothing.
		{"hook-denies-an-unreadable-document", func(w *receiptWorld) {
			root := w.repo("<ROOT>", false)
			gitDir := filepath.Join(root, ".git")
			w.change(root, "only-change")
			w.putState(gitDir, "review-readable", lifecycleState("review-readable", approved))
			if err := os.MkdirAll(filepath.Join(store(gitDir), "review-dir", "review-receipt.json"), 0o755); err != nil {
				w.t.Fatal(err)
			}
			w.hook(root, acknowledgeCommand)
			w.tree("after", root)
		}},
		// A lineage id is read from a document another program wrote, and the file a receipt
		// is persisted under is named from it. An id that would walk out of the change's
		// receipts folder is refused, naming the file, and the hook denies; what came before
		// it stays captured, and nothing is written outside the folder.
		{"hook-denies-a-lineage-id-that-would-leave-the-folder", func(w *receiptWorld) {
			root := w.repo("<ROOT>", false)
			gitDir := filepath.Join(root, ".git")
			w.change(root, "only-change")
			w.putReceipt(gitDir, "review-a-fine", legacyReceipt("review-a-fine", receiptSchemaV2, approved))
			w.putState(gitDir, "review-b-escape", lifecycleState("../../escaped", approved))
			w.hook(root, acknowledgeCommand)
			w.tree("after", root)
		}},
		// A receipt that cannot be kept blocks no other: the lineage after it is still
		// captured, and the hook then denies, naming the document and the remedy.
		{"hook-captures-the-rest-and-denies-an-unusable-receipt", func(w *receiptWorld) {
			root := w.repo("<ROOT>", false)
			gitDir := filepath.Join(root, ".git")
			w.change(root, "only-change")
			w.putState(gitDir, "review-a-escape", lifecycleState("../../escaped", approved))
			w.putReceipt(gitDir, "review-b-after", legacyReceipt("review-b-after", receiptSchemaV2, approved))
			w.hook(root, acknowledgeCommand)
			w.tree("after", root)
		}},
		{"hook-denies-an-ambiguous-change", func(w *receiptWorld) {
			root := w.repo("<ROOT>", false)
			w.change(root, "change-b")
			w.change(root, "change-a")
			w.putReceipt(filepath.Join(root, ".git"), "review-ambiguous", legacyReceipt("review-ambiguous", receiptSchemaV2, approved))
			w.hook(root, acknowledgeCommand)
			w.tree("after", root)
		}},
		{"hook-allows-once-another-run-captured-it", func(w *receiptWorld) {
			root := w.repo("<ROOT>", false)
			gitDir := filepath.Join(root, ".git")
			w.change(root, "change-a")
			w.change(root, "change-b")
			w.putReceipt(gitDir, "review-fixable", legacyReceipt("review-fixable", receiptSchemaV2, approved))
			w.putState(gitDir, "review-fixable-state", lifecycleState("review-fixable-state", approved))
			w.hook(root, acknowledgeCommand)
			w.run("", "capture", "--cwd", root, "--change", "change-a")
			w.hook(root, acknowledgeCommand)
			w.putState(gitDir, "review-later", lifecycleState("review-later", approved))
			w.hook(root, acknowledgeCommand)
			w.tree("after", root)
		}},
		{"hook-denies-while-one-shape-of-a-lineage-is-unpersisted", func(w *receiptWorld) {
			root := w.repo("<ROOT>", false)
			gitDir := filepath.Join(root, ".git")
			w.change(root, "change-a")
			w.change(root, "change-b")
			receipt, state := legacyReceipt("review-both", receiptSchemaV2, approved), lifecycleState("review-both", approved)
			w.putReceipt(gitDir, "review-both", receipt)
			w.putState(gitDir, "review-both", state)
			receipts := filepath.Join(root, "openspec", "changes", "change-a", "review-receipts")
			w.put(filepath.Join(receipts, "review-both.json"), receipt)
			w.hook(root, acknowledgeCommand)
			w.put(filepath.Join(receipts, "review-both.review-state.json"), state)
			w.hook(root, acknowledgeCommand)
			w.tree("after", root)
		}},
		{"hook-denies-a-destination-that-differs", func(w *receiptWorld) {
			root := w.repo("<ROOT>", false)
			w.change(root, "only-change")
			w.putReceipt(filepath.Join(root, ".git"), "review-atomic", legacyReceipt("review-atomic", receiptSchemaV2, approved))
			w.put(filepath.Join(root, "openspec", "changes", "only-change", "review-receipts", "review-atomic.json"), `{"different":true}`)
			w.hook(root, acknowledgeCommand)
			w.tree("after", root)
		}},
		{"hook-reads-both-stores-of-a-linked-worktree", func(w *receiptWorld) {
			main := w.repo("<MAIN>", false)
			linked, err := filepath.EvalSymlinks(w.t.TempDir())
			if err != nil {
				w.t.Fatal(err)
			}
			linked = filepath.Join(linked, "linked")
			w.git(main, "worktree", "add", "-q", linked, "-b", "linked-branch")
			w.name(linked, "<LINKED>")
			privateDir := filepath.Join(main, ".git", "worktrees", "linked")
			commonDir := filepath.Join(main, ".git")
			w.change(linked, "only-change")
			w.putReceipt(privateDir, "review-private", legacyReceipt("review-private", receiptSchemaV2, approved))
			w.putReceipt(commonDir, "review-common", legacyReceipt("review-common", receiptSchemaV2, approved))
			shared := legacyReceipt("review-shared", receiptSchemaV2, approved)
			w.putReceipt(privateDir, "review-shared", shared)
			w.putReceipt(commonDir, "review-shared", shared)
			w.putState(commonDir, "review-state-common", lifecycleState("review-state-common", approved))
			w.hook(linked, acknowledgeCommand)
			w.tree("linked worktree", linked)
			w.tree("main worktree", main)
		}},
		// openspec/ is found from the toplevel of the working tree the hook is started in, not from
		// the directory the runner gave it (Phase 9 batch 12a; until then a session started in a
		// subdirectory found none, passed the acknowledgement through and lost the receipt).
		{"hook-in-a-subdirectory-captures-into-the-toplevel", func(w *receiptWorld) {
			root := w.repo("<ROOT>", false)
			w.change(root, "only-change")
			if err := os.MkdirAll(filepath.Join(root, "sub", "deeper"), 0o755); err != nil {
				w.t.Fatal(err)
			}
			w.putReceipt(filepath.Join(root, ".git"), "review-from-below", legacyReceipt("review-from-below", receiptSchemaV2, approved))
			w.hook(filepath.Join(root, "sub"), acknowledgeCommand)
			w.tree("after the hook in sub", root)
			w.hook(filepath.Join(root, "sub", "deeper"), acknowledgeCommand)
			w.tree("after the hook in sub/deeper (already captured)", root)
		}},
		{"hook-in-a-subdirectory-names-paths-under-the-toplevel", func(w *receiptWorld) {
			root := w.repo("<ROOT>", false)
			w.change(root, "only-change")
			w.putReceipt(filepath.Join(root, ".git"), "review-atomic", legacyReceipt("review-atomic", receiptSchemaV2, approved))
			w.put(filepath.Join(root, "openspec", "changes", "only-change", "review-receipts", "review-atomic.json"), `{"different":true}`)
			if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
				w.t.Fatal(err)
			}
			w.hook(filepath.Join(root, "sub"), acknowledgeCommand)
			w.hook(root, acknowledgeCommand)
		}},
		{"hook-in-a-subdirectory-of-a-repository-without-openspec-passes", func(w *receiptWorld) {
			root := w.repo("<ROOT>", false)
			if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
				w.t.Fatal(err)
			}
			w.putReceipt(filepath.Join(root, ".git"), "review-orphan", legacyReceipt("review-orphan", receiptSchemaV2, approved))
			w.hook(filepath.Join(root, "sub"), acknowledgeCommand)
			w.tree("after", root)
		}},
		{"hook-in-a-subdirectory-of-a-linked-worktree-captures-into-its-toplevel", func(w *receiptWorld) {
			main := w.repo("<MAIN>", false)
			linked, err := filepath.EvalSymlinks(w.t.TempDir())
			if err != nil {
				w.t.Fatal(err)
			}
			linked = filepath.Join(linked, "linked")
			w.git(main, "worktree", "add", "-q", linked, "-b", "linked-branch")
			w.name(linked, "<LINKED>")
			w.change(linked, "only-change")
			if err := os.MkdirAll(filepath.Join(linked, "src"), 0o755); err != nil {
				w.t.Fatal(err)
			}
			w.putReceipt(filepath.Join(main, ".git", "worktrees", "linked"), "review-private", legacyReceipt("review-private", receiptSchemaV2, approved))
			w.putReceipt(filepath.Join(main, ".git"), "review-common", legacyReceipt("review-common", receiptSchemaV2, approved))
			w.hook(filepath.Join(linked, "src"), acknowledgeCommand)
			w.tree("linked worktree", linked)
			w.tree("main worktree", main)
		}},
		{"hook-in-a-subdirectory-prefers-the-toplevels-openspec", func(w *receiptWorld) {
			root := w.repo("<ROOT>", false)
			sub := filepath.Join(root, "sub")
			w.change(root, "top-change")
			w.change(sub, "sub-change")
			w.putReceipt(filepath.Join(root, ".git"), "review-from-below", legacyReceipt("review-from-below", receiptSchemaV2, approved))
			w.hook(sub, acknowledgeCommand)
			w.tree("toplevel", root)
			w.tree("sub", sub)
		}},
		// A repository whose toplevel has no openspec/changes but whose subdirectory has one is
		// served from that subdirectory, as it always was: it is the one place the hook can put
		// the receipt, and passing through would lose it.
		{"hook-in-a-subdirectory-serves-its-own-openspec-when-the-toplevel-has-none", func(w *receiptWorld) {
			root := w.repo("<ROOT>", false)
			sub := filepath.Join(root, "sub")
			w.change(sub, "sub-change")
			w.putReceipt(filepath.Join(root, ".git"), "review-from-below", legacyReceipt("review-from-below", receiptSchemaV2, approved))
			w.hook(sub, acknowledgeCommand)
			w.tree("toplevel", root)
			w.tree("sub", sub)
		}},
		{"hook-in-a-subdirectory-with-its-own-openspec", func(w *receiptWorld) {
			root := w.repo("<ROOT>", false)
			sub := filepath.Join(root, "sub")
			w.change(sub, "sub-change")
			w.putReceipt(filepath.Join(root, ".git"), "review-from-below", legacyReceipt("review-from-below", receiptSchemaV2, approved))
			w.hook(sub, acknowledgeCommand)
			w.tree("after", root)
		}},
		{"hook-in-a-repository-without-commits", func(w *receiptWorld) {
			root := w.repo("<ROOT>", true)
			w.change(root, "only-change")
			w.putReceipt(filepath.Join(root, ".git"), "review-first", legacyReceipt("review-first", receiptSchemaV2, approved))
			w.hook(root, acknowledgeCommand)
			w.tree("after", root)
		}},
		{"hook-requires-a-working-directory", func(w *receiptWorld) {
			w.run(acknowledgeHookJSON(acknowledgeCommand), "hook")
			w.run(acknowledgeHookJSON(acknowledgeCommand), "hook", "--cwd")
		}},
		{"hook-reads-the-input-it-names-and-nothing-else", func(w *receiptWorld) {
			// Each input gets a repository of its own with one active change and one approved
			// receipt, so the tree after it shows whether the hook took the input for an
			// acknowledgement and captured the receipt (a file under review-receipts) or let it
			// through without looking (no file). The hook reads tool_name and
			// tool_input.command, and only those: a field it does not read can be of any type, and
			// one it reads must be of the type it expects.
			ack := fmt.Sprintf("%q", acknowledgeCommand)
			try := func(label, stdin string) {
				root := w.repo("<ROOT>", false)
				w.change(root, "only-change")
				w.putReceipt(filepath.Join(root, ".git"), "review-one", legacyReceipt("review-one", receiptSchemaV2, approved))
				w.write("# %s\n", label)
				w.run(stdin, "hook", "--cwd", root)
				w.tree("after", root)
			}
			try("tool_name of the wrong type: the input cannot be decoded", `{"tool_name":7,"tool_input":{"command":`+ack+`}}`)
			try("tool_name an object", `{"tool_name":{},"tool_input":{"command":`+ack+`}}`)
			try("tool_name missing: the name is not consulted", `{"tool_input":{"command":`+ack+`}}`)
			try("tool_name of another tool: the name is not consulted", `{"tool_name":"Write","tool_input":{"command":`+ack+`}}`)
			try("fields of the envelope it does not read, of the wrong type", `{"cwd":5,"hook_event_name":7,"session_id":{},"tool_name":"Bash","tool_input":{"command":`+ack+`}}`)
			try("file_path of the wrong type beside the command: not a field it reads", `{"tool_name":"Bash","tool_input":{"command":`+ack+`,"file_path":5}}`)
			try("notebook_path of the wrong type beside the command", `{"tool_name":"Bash","tool_input":{"command":`+ack+`,"notebook_path":[]}}`)
			try("keys in other cases", `{"TOOL_NAME":"Bash","TOOL_INPUT":{"COMMAND":`+ack+`}}`)
			try("a key twice: the last one wins", `{"tool_name":"Bash","tool_input":{"command":"ls","command":`+ack+`}}`)
			try("white space around the object", " \n"+`{"tool_name":"Bash","tool_input":{"command":`+ack+`}}`+"\n ")
			try("the phrase inside a longer command, as a look-alike", `{"tool_name":"Bash","tool_input":{"command":"echo \"`+acknowledgeCommand+`\" >> log"}}`)
			try("tool_input a string", `{"tool_name":"Bash","tool_input":`+ack+`}`)
			try("tool_input an array", `{"tool_name":"Bash","tool_input":[`+ack+`]}`)
			try("tool_input null", `{"tool_name":"Bash","tool_input":null}`)
			try("command an array", `{"tool_name":"Bash","tool_input":{"command":[`+ack+`]}}`)
			try("command null", `{"tool_name":"Bash","tool_input":{"command":null}}`)
			try("an array in place of the object", `[{"tool_name":"Bash","tool_input":{"command":`+ack+`}}]`)
			try("null", `null`)
			try("two objects", `{"tool_name":"Bash","tool_input":{"command":`+ack+`}}{}`)
			try("text after the object", `{"tool_name":"Bash","tool_input":{"command":`+ack+`}} trailing`)
			try("a truncated object", `{"tool_name":"Bash","tool_input":{"command":`+ack)
		}},
		{"hook-denies-a-store-it-cannot-read", func(w *receiptWorld) {
			if os.Geteuid() == 0 {
				w.t.Skip("a directory without permissions does not stop root")
			}
			root := w.repo("<ROOT>", false)
			w.change(root, "only-change")
			w.putReceipt(filepath.Join(root, ".git"), "review-hidden", legacyReceipt("review-hidden", receiptSchemaV2, approved))
			dir := store(filepath.Join(root, ".git"))
			if err := os.Chmod(dir, 0); err != nil {
				w.t.Fatal(err)
			}
			w.t.Cleanup(func() { os.Chmod(dir, 0o755) })
			w.hook(root, acknowledgeCommand)
			w.tree("after", root)
		}},
		{"git-failure-hook-outside-a-repository", func(w *receiptWorld) {
			dir, err := filepath.EvalSymlinks(w.t.TempDir())
			if err != nil {
				w.t.Fatal(err)
			}
			w.name(dir, "<ROOT>")
			w.change(dir, "only-change")
			w.hook(dir, acknowledgeCommand)
			w.change(dir, "second-change")
			w.hook(dir, acknowledgeCommand)
			w.tree("after", dir)
		}},
		{"capture-requires-a-working-directory", func(w *receiptWorld) {
			w.run("", "capture")
			w.run("", "capture", "--change", "x")
		}},
		{"capture-without-an-active-change", func(w *receiptWorld) {
			root := w.repo("<ROOT>", false)
			w.putReceipt(filepath.Join(root, ".git"), "review-orphan", legacyReceipt("review-orphan", receiptSchemaV2, approved))
			w.run("", "capture", "--cwd", root)
			w.tree("after", root)
		}},
		{"capture-finds-the-single-active-change", func(w *receiptWorld) {
			root := w.repo("<ROOT>", false)
			w.change(root, "the-change")
			w.putReceipt(filepath.Join(root, ".git"), "review-one", legacyReceipt("review-one", receiptSchemaV2, approved))
			w.putState(filepath.Join(root, ".git"), "review-two", lifecycleState("review-two", approved))
			w.run("", "capture", "--cwd", root)
			w.tree("after", root)
		}},
		{"capture-into-a-named-change-twice", func(w *receiptWorld) {
			root := w.repo("<ROOT>", false)
			w.change(root, "change-a")
			w.change(root, "change-b")
			w.putReceipt(filepath.Join(root, ".git"), "review-one", legacyReceipt("review-one", receiptSchemaV2, approved))
			w.run("", "capture", "--cwd", root, "--change", "change-b")
			w.run("", "capture", "--cwd", root, "--change", "change-b")
			w.run("", "capture", "--cwd", root, "--change", "a-change-with-no-folder")
			w.tree("after", root)
		}},
		{"capture-counts-a-lineage-once-across-the-stores-of-a-worktree", func(w *receiptWorld) {
			main := w.repo("<MAIN>", false)
			linked, err := filepath.EvalSymlinks(w.t.TempDir())
			if err != nil {
				w.t.Fatal(err)
			}
			linked = filepath.Join(linked, "linked")
			w.git(main, "worktree", "add", "-q", linked, "-b", "linked-branch")
			w.name(linked, "<LINKED>")
			privateDir := filepath.Join(main, ".git", "worktrees", "linked")
			commonDir := filepath.Join(main, ".git")
			w.change(linked, "the-change")
			shared := legacyReceipt("review-shared", receiptSchemaV2, approved)
			w.putReceipt(privateDir, "review-shared", shared)
			w.putReceipt(commonDir, "review-shared", shared)
			sharedState := lifecycleState("review-shared", approved)
			w.putState(privateDir, "review-shared", sharedState)
			w.putState(commonDir, "review-shared", sharedState)
			w.putReceipt(commonDir, "review-common-only", legacyReceipt("review-common-only", receiptSchemaV2, approved))
			w.run("", "capture", "--cwd", linked)
			w.tree("after", linked)
		}},
		{"capture-with-nothing-to-capture", func(w *receiptWorld) {
			root := w.repo("<ROOT>", false)
			w.change(root, "the-change")
			w.run("", "capture", "--cwd", root)
			w.run("", "capture", "--cwd", root, "--change", "the-change")
			w.tree("after", root)
		}},
		{"capture-refuses-ambiguous-changes", func(w *receiptWorld) {
			root := w.repo("<ROOT>", false)
			w.change(root, "change-a")
			w.change(root, "change-b")
			w.putReceipt(filepath.Join(root, ".git"), "review-one", legacyReceipt("review-one", receiptSchemaV2, approved))
			w.run("", "capture", "--cwd", root)
			w.tree("after", root)
		}},
		{"capture-refuses-a-blank-change", func(w *receiptWorld) {
			root := w.repo("<ROOT>", false)
			w.putReceipt(filepath.Join(root, ".git"), "review-one", legacyReceipt("review-one", receiptSchemaV2, approved))
			w.run("", "capture", "--cwd", root, "--change", "   ")
			w.tree("after", root)
		}},
		// The change name becomes a directory under openspec/changes, so a name that is not
		// one path component is refused by name, and nothing is created for it.
		{"capture-refuses-a-change-name-that-is-not-a-path-component", func(w *receiptWorld) {
			root := w.repo("<ROOT>", false)
			w.change(root, "the-change")
			w.putReceipt(filepath.Join(root, ".git"), "review-one", legacyReceipt("review-one", receiptSchemaV2, approved))
			for _, change := range []string{"../escaped", "a/b", "..", " padded", "padded "} {
				w.run("", "capture", "--cwd", root, "--change", change)
			}
			w.tree("after", root)
		}},
		{"capture-refuses-a-destination-that-differs", func(w *receiptWorld) {
			root := w.repo("<ROOT>", false)
			w.change(root, "the-change")
			w.putReceipt(filepath.Join(root, ".git"), "review-one", legacyReceipt("review-one", receiptSchemaV2, approved))
			w.put(filepath.Join(root, "openspec", "changes", "the-change", "review-receipts", "review-one.json"), "{}")
			w.run("", "capture", "--cwd", root)
			w.tree("after", root)
		}},
		{"capture-with-a-relative-working-directory", func(w *receiptWorld) {
			root := w.repo("<ROOT>", false)
			w.change(root, "the-change")
			w.putReceipt(filepath.Join(root, ".git"), "review-one", legacyReceipt("review-one", receiptSchemaV2, approved))
			// A relative --cwd is resolved against the directory the program runs in; the
			// program here runs elsewhere, so the repository is reached by a path that
			// climbs out of it.
			rel, err := filepath.Rel(w.reader, root)
			if err != nil {
				w.t.Fatal(err)
			}
			w.name(rel, "<RELATIVE-ROOT>")
			w.run("", "capture", "--cwd", rel)
			w.tree("after", root)
		}},
		{"git-failure-capture-outside-a-repository", func(w *receiptWorld) {
			dir, err := filepath.EvalSymlinks(w.t.TempDir())
			if err != nil {
				w.t.Fatal(err)
			}
			w.name(dir, "<ROOT>")
			w.change(dir, "the-change")
			w.run("", "capture", "--cwd", dir)
			w.run("", "capture", "--cwd", dir, "--change", "the-change")
			w.tree("after", dir)
		}},
		{"verbs-must-be-named", func(w *receiptWorld) {
			w.run("")
			w.run("", "frobnicate")
			w.run("", "capture", "--unknown", "--cwd")
		}},
	}
}

var goldenFileName = regexp.MustCompile(`[^a-z0-9-]+`)

// TestReviewReceiptGolden runs every case and compares its transcript with its golden file.
func TestReviewReceiptGolden(t *testing.T) {
	for _, tc := range receiptGoldenCases() {
		t.Run(tc.name, func(t *testing.T) {
			w := newReceiptWorld(t)
			tc.run(w)
			checkReviewReceiptGolden(t, tc.name, w.text())
		})
	}
}

func checkReviewReceiptGolden(t *testing.T, name, got string) {
	t.Helper()
	if goldenFileName.MatchString(name) {
		t.Fatalf("case name %q is not a file name", name)
	}
	path := filepath.Join("testdata", "review-receipt-golden", name+".golden")
	if *updateReviewReceiptGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden file: %v (record it with -update-review-receipt-golden)", err)
	}
	if diff := goldenDifference(name, got, string(want)); diff != "" {
		t.Fatal(diff)
	}
}
