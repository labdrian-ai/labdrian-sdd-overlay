package fsstore_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gitprov"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewreceipt"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewreceipt/fsstore"
)

// These tests run the whole capture, the domain's Service over this package's Store, against
// real git repositories in temporary directories (skipped under -short, like the other
// git-in-t.TempDir suites of the engine). The domain's own rules are proved in package
// reviewreceipt against in-memory ports; the files and git are proved here.

func gitUnavailable(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping git-in-TempDir fixture under -short")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git unavailable: %v", err)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// gitFixtureRepo initializes a real git repository with one commit and returns its toplevel,
// symlinks resolved.
func gitFixtureRepo(t *testing.T) string {
	t.Helper()
	gitUnavailable(t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("fixture\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-q", "-m", "initial")
	return dir
}

// serviceFor builds the Service over a Store for root, wired the way the program wires it.
func serviceFor(t *testing.T, root string) *reviewreceipt.Service {
	t.Helper()
	store, err := fsstore.New(root, gitprov.Observer{Run: gitprov.ExecRunner, Environ: os.Environ()})
	if err != nil {
		t.Fatalf("fsstore.New: %v", err)
	}
	return reviewreceipt.NewService(reviewreceipt.Ports{Stores: store, Source: store, Sink: store, Changes: store})
}

func storeFor(t *testing.T, root string) fsstore.Store {
	t.Helper()
	store, err := fsstore.New(root, gitprov.Observer{Run: gitprov.ExecRunner, Environ: os.Environ()})
	if err != nil {
		t.Fatalf("fsstore.New: %v", err)
	}
	return store
}

// transactionStore is the store under a git directory.
func transactionStore(gitDir string) string {
	return filepath.Join(gitDir, "gentle-ai", "review-transactions", "v2")
}

// writeReceipt writes a review-receipt.json under gitDir's transaction store for the given
// lineage, with the given schema/terminal_state.
func writeReceipt(t *testing.T, gitDir, lineage, schema, terminalState string) {
	t.Helper()
	lineageDir := filepath.Join(transactionStore(gitDir), lineage)
	if err := os.MkdirAll(lineageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	payload := map[string]interface{}{
		"schema":               schema,
		"lineage_id":           lineage,
		"final_candidate_tree": "deadbeef",
		"selected_lenses":      []string{"review-risk"},
		"terminal_state":       terminalState,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lineageDir, "review-receipt.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}

// stateJSON builds a minimal gentle-ai 2.7.0+ review-state.json payload.
func stateJSON(lineage, state string) []byte {
	return []byte(fmt.Sprintf(
		`{"schema":"gentle-ai.review-transaction/v2","revision":3,"state":{`+
			`"schema":"gentle-ai.review-state/v2","lineage_id":%q,"generation":1,"state":%q,`+
			`"risk_level":"medium","selected_lenses":["review-risk","review-readability"],`+
			`"initial_snapshot":{"base_tree":"basetree1"},`+
			`"current_snapshot":{"kind":"candidate","base_tree":"basetree1","candidate_tree":"candidatetree1"}}}`,
		lineage, state))
}

// writeReviewState writes review-state.json (gentle-ai 2.7.0+ shape) under gitDir's
// transaction store.
func writeReviewState(t *testing.T, gitDir, lineage, state string) {
	t.Helper()
	lineageDir := filepath.Join(transactionStore(gitDir), lineage)
	if err := os.MkdirAll(lineageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lineageDir, "review-state.json"), stateJSON(lineage, state), 0644); err != nil {
		t.Fatal(err)
	}
}

// seedActiveChange creates openspec/changes/<change>/tasks.md under root so the change is
// active.
func seedActiveChange(t *testing.T, root, change string) {
	t.Helper()
	dir := filepath.Join(root, "openspec", "changes", change)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tasks.md"), []byte("# tasks\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

func receiptsOf(root, change string) string {
	return filepath.Join(root, "openspec", "changes", change, "review-receipts")
}

func gitDirOf(root string) string { return filepath.Join(root, ".git") }

// Capture persists only receipts whose schema is exactly gentle-ai.review-receipt/v2 AND
// whose terminal_state is exactly "approved" -- any other schema or a non-terminal (or
// non-approved terminal) state is silently skipped, not an error.
func TestCaptureSchemaAndTerminalState(t *testing.T) {
	repo := gitFixtureRepo(t)
	seedActiveChange(t, repo, "my-change")
	writeReceipt(t, gitDirOf(repo), "review-approved1", "gentle-ai.review-receipt/v2", "approved")
	writeReceipt(t, gitDirOf(repo), "review-wrongschema", "gentle-ai.review-receipt/v1", "approved")
	writeReceipt(t, gitDirOf(repo), "review-notapproved", "gentle-ai.review-receipt/v2", "declined")

	captured, err := serviceFor(t, repo).Capture("my-change")
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if len(captured) != 1 || captured[0].LineageID != "review-approved1" {
		t.Fatalf("captured %#v, want exactly review-approved1", captured)
	}

	destDir := receiptsOf(repo, "my-change")
	if captured[0].Path != filepath.Join(destDir, "review-approved1.json") {
		t.Errorf("Path = %q, want the persisted file", captured[0].Path)
	}
	if _, err := os.Stat(filepath.Join(destDir, "review-approved1.json")); err != nil {
		t.Errorf("expected approved receipt persisted: %v", err)
	}
	for _, name := range []string{"review-wrongschema.json", "review-notapproved.json"} {
		if _, err := os.Stat(filepath.Join(destDir, name)); !os.IsNotExist(err) {
			t.Errorf("%s should NOT be persisted, stat err=%v", name, err)
		}
	}
}

// Capture also captures an approved review-state.json (2.7.0+ writes no review-receipt.json),
// and skips one still in review.
func TestCaptureReviewState(t *testing.T) {
	repo := gitFixtureRepo(t)
	seedActiveChange(t, repo, "my-change")
	writeReviewState(t, gitDirOf(repo), "review-state-approved1", "approved")
	writeReviewState(t, gitDirOf(repo), "review-state-reviewing", "reviewing")
	writeReviewState(t, gitDirOf(repo), "review-state-escalated", "escalated")

	captured, err := serviceFor(t, repo).Capture("my-change")
	if err != nil || len(captured) != 1 || captured[0].LineageID != "review-state-approved1" {
		t.Fatalf("Capture: got %#v, err=%v", captured, err)
	}
	if _, err := os.Stat(filepath.Join(receiptsOf(repo, "my-change"), "review-state-approved1.review-state.json")); err != nil {
		t.Errorf("expected review-state.json persisted: %v", err)
	}
}

// A lineage carrying both an approved legacy receipt AND an approved review-state.json (a
// mixed-version transition) captures both, to distinct destinations.
func TestCaptureBothFormatsPresent(t *testing.T) {
	repo := gitFixtureRepo(t)
	seedActiveChange(t, repo, "my-change")
	writeReceipt(t, gitDirOf(repo), "review-both1", "gentle-ai.review-receipt/v2", "approved")
	writeReviewState(t, gitDirOf(repo), "review-both1", "approved")

	captured, err := serviceFor(t, repo).Capture("my-change")
	if err != nil || len(captured) != 2 {
		t.Fatalf("expected 2 captured, got %#v, err=%v", captured, err)
	}
	for _, name := range []string{"review-both1.json", "review-both1.review-state.json"} {
		if _, err := os.Stat(filepath.Join(receiptsOf(repo, "my-change"), name)); err != nil {
			t.Errorf("expected %s persisted: %v", name, err)
		}
	}
}

// Capture writes a byte-identical copy of the source receipt, re-running it on the same
// source is a no-op, and a differing existing destination file is an error rather than a
// silent overwrite.
func TestCaptureAtomicWrite(t *testing.T) {
	repo := gitFixtureRepo(t)
	seedActiveChange(t, repo, "my-change")
	writeReceipt(t, gitDirOf(repo), "review-atomic1", "gentle-ai.review-receipt/v2", "approved")

	srcBytes, err := os.ReadFile(filepath.Join(transactionStore(gitDirOf(repo)), "review-atomic1", "review-receipt.json"))
	if err != nil {
		t.Fatal(err)
	}
	svc := serviceFor(t, repo)
	if _, err := svc.Capture("my-change"); err != nil {
		t.Fatalf("Capture (first run): %v", err)
	}

	destPath := filepath.Join(receiptsOf(repo, "my-change"), "review-atomic1.json")
	destBytes, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read captured receipt: %v", err)
	}
	if string(destBytes) != string(srcBytes) {
		t.Errorf("captured receipt is not byte-identical:\nsrc:  %s\ndest: %s", srcBytes, destBytes)
	}
	if info, err := os.Stat(destPath); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("mode of the captured receipt = %v, %v, want 0600", info, err)
	}

	if _, err := svc.Capture("my-change"); err != nil {
		t.Fatalf("Capture (second run, idempotent): %v", err)
	}

	if err := os.WriteFile(destPath, []byte(`{"different":true}`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Capture("my-change"); err == nil {
		t.Error("expected Capture to error on a differing existing destination file, got nil")
	}
	if got, _ := os.ReadFile(destPath); string(got) != `{"different":true}` {
		t.Errorf("the differing file was replaced by %q", got)
	}
}

const ackHookInput = `{"tool_name":"Bash","tool_input":{"command":"gentle-ai review acknowledge-approved --cwd /repo --lineage review-x"}}`

func TestHookPassesThroughWithoutOpenspecChanges(t *testing.T) {
	repo := gitFixtureRepo(t)
	writeReceipt(t, gitDirOf(repo), "review-orphan", "gentle-ai.review-receipt/v2", "approved")

	if code, message := serviceFor(t, repo).RunHook([]byte(ackHookInput)); code != 0 || message != "" {
		t.Errorf("RunHook = (%d, %q), want (0, \"\")", code, message)
	}
	if _, err := os.Stat(filepath.Join(repo, "openspec")); !os.IsNotExist(err) {
		t.Errorf("openspec/ should not have been created by the pass-through hook, stat err=%v", err)
	}
}

// With more than one active change and a surviving approved receipt that was captured
// nowhere, the hook denies and names the remedy; once the remedy ran for every receipt it
// allows; and capturing one of two receipts does not clear the deny for the other.
func TestHookMultipleActiveChanges(t *testing.T) {
	repo := gitFixtureRepo(t)
	seedActiveChange(t, repo, "change-a")
	seedActiveChange(t, repo, "change-b")
	writeReceipt(t, gitDirOf(repo), "review-captured", "gentle-ai.review-receipt/v2", "approved")
	svc := serviceFor(t, repo)

	code, message := svc.RunHook([]byte(ackHookInput))
	if code != 2 || !strings.Contains(message, "review-receipt capture --change <name>") {
		t.Fatalf("first run: RunHook = (%d, %q), want a deny that names the remedy", code, message)
	}

	if _, err := svc.Capture("change-a"); err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if code, message := svc.RunHook([]byte(ackHookInput)); code != 0 {
		t.Fatalf("after the remedy: RunHook = (%d, %q), want (0, \"\")", code, message)
	}

	writeReceipt(t, gitDirOf(repo), "review-uncaptured", "gentle-ai.review-receipt/v2", "approved")
	if code, message := svc.RunHook([]byte(ackHookInput)); code != 2 || !strings.Contains(message, "review-receipt capture --change <name>") {
		t.Errorf("one lineage still uncaptured: RunHook = (%d, %q), want a deny", code, message)
	}
}

func TestHookCapturesIntoTheSingleActiveChange(t *testing.T) {
	repo := gitFixtureRepo(t)
	seedActiveChange(t, repo, "only-change")
	writeReceipt(t, gitDirOf(repo), "review-solo", "gentle-ai.review-receipt/v2", "approved")

	if code, message := serviceFor(t, repo).RunHook([]byte(ackHookInput)); code != 0 || message != "" {
		t.Fatalf("RunHook = (%d, %q), want (0, \"\")", code, message)
	}
	if _, err := os.Stat(filepath.Join(receiptsOf(repo, "only-change"), "review-solo.json")); err != nil {
		t.Errorf("expected receipt captured: %v", err)
	}
}

func TestHookIgnoresACommandThatIsNotAnAcknowledgement(t *testing.T) {
	repo := gitFixtureRepo(t)
	seedActiveChange(t, repo, "only-change")
	writeReceipt(t, gitDirOf(repo), "review-untouched", "gentle-ai.review-receipt/v2", "approved")

	input := `{"tool_name":"Bash","tool_input":{"command":"git status"}}`
	if code, message := serviceFor(t, repo).RunHook([]byte(input)); code != 0 || message != "" {
		t.Fatalf("RunHook = (%d, %q), want (0, \"\")", code, message)
	}
	if _, err := os.Stat(receiptsOf(repo, "only-change")); !os.IsNotExist(err) {
		t.Errorf("a command that is not an acknowledgement must not capture, stat err=%v", err)
	}
}

// The hook's working directory is not always the toplevel of the working tree ($CLAUDE_PROJECT_DIR
// is where the session started, which can be any directory inside a repository), so the stores
// are found from wherever it is: the same stores from the toplevel and from a directory inside it,
// in a plain repository and in a linked working tree.
func TestTheStoresAreTheSameFromTheToplevelAndFromInsideIt(t *testing.T) {
	repo := gitFixtureRepo(t)
	sub := filepath.Join(repo, "deep", "er")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(filepath.Dir(repo), filepath.Base(repo)+"-linked")
	runGit(t, repo, "worktree", "add", "-q", linked, "-b", "linked-branch")
	linkedSub := filepath.Join(linked, "src")
	if err := os.MkdirAll(linkedSub, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name      string
		toplevel  string
		inside    string
		wantCount int
	}{
		{"a plain repository", repo, sub, 1},
		{"a linked working tree", linked, linkedSub, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fromTop, err := storeFor(t, tc.toplevel).Stores()
			if err != nil {
				t.Fatalf("Stores from the toplevel: %v", err)
			}
			fromInside, err := storeFor(t, tc.inside).Stores()
			if err != nil {
				t.Fatalf("Stores from inside: %v", err)
			}
			if !reflect.DeepEqual(fromTop, fromInside) || len(fromTop) != tc.wantCount {
				t.Errorf("stores from the toplevel %v and from inside %v, want the same %d", fromTop, fromInside, tc.wantCount)
			}
		})
	}
}

// openspec/ is looked for under the directory the hook was started in, as it always was. A
// directory inside a repository whose toplevel has the active change finds none and passes
// through; one that has its own openspec captures there, from the same stores. The first is
// the long-standing behavior for a session started below its project, kept as it was.
func TestAHookStartedInsideTheRepositoryLooksForOpenspecWhereItStarted(t *testing.T) {
	repo := gitFixtureRepo(t)
	seedActiveChange(t, repo, "top-change")
	writeReceipt(t, gitDirOf(repo), "review-from-below", "gentle-ai.review-receipt/v2", "approved")

	plain := filepath.Join(repo, "plain")
	own := filepath.Join(repo, "own")
	for _, dir := range []string{plain, own} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	seedActiveChange(t, own, "own-change")

	if code, message := serviceFor(t, plain).RunHook([]byte(ackHookInput)); code != 0 || message != "" {
		t.Errorf("from a directory with no openspec: RunHook = (%d, %q), want it to pass through", code, message)
	}
	if _, err := os.Stat(receiptsOf(repo, "top-change")); !os.IsNotExist(err) {
		t.Errorf("a hook started below the project captured into the toplevel's change (stat err=%v)", err)
	}

	if code, message := serviceFor(t, own).RunHook([]byte(ackHookInput)); code != 0 || message != "" {
		t.Fatalf("from a directory with its own openspec: RunHook = (%d, %q)", code, message)
	}
	if _, err := os.Stat(filepath.Join(receiptsOf(own, "own-change"), "review-from-below.json")); err != nil {
		t.Errorf("expected the receipt captured under the directory's own openspec: %v", err)
	}
}

// A repository with no commit yet has its git directories all the same, and a review can be
// acknowledged in it.
func TestARepositoryWithNoCommitIsServed(t *testing.T) {
	gitUnavailable(t)
	repo, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "init", "-q")
	seedActiveChange(t, repo, "first")
	writeReceipt(t, gitDirOf(repo), "review-first", "gentle-ai.review-receipt/v2", "approved")

	if code, message := serviceFor(t, repo).RunHook([]byte(ackHookInput)); code != 0 || message != "" {
		t.Fatalf("RunHook = (%d, %q), want (0, \"\")", code, message)
	}
	if _, err := os.Stat(filepath.Join(receiptsOf(repo, "first"), "review-first.json")); err != nil {
		t.Errorf("expected the receipt captured: %v", err)
	}
}

// Outside a repository there are no stores: capturing is an error in git's words, and the
// hook denies an acknowledgement it was going to capture for, but it needs no git to pass a
// project that has no change to capture into.
func TestOutsideARepositoryCaptureFailsAndAHookWithNothingToCaptureDoesNot(t *testing.T) {
	gitUnavailable(t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := serviceFor(t, dir)

	if code, message := svc.RunHook([]byte(ackHookInput)); code != 0 || message != "" {
		t.Errorf("no change to capture into: RunHook = (%d, %q), want (0, \"\")", code, message)
	}

	seedActiveChange(t, dir, "orphan")
	code, message := svc.RunHook([]byte(ackHookInput))
	if code != 2 || !strings.HasPrefix(message, "review-receipt: capture failed: reviewreceipt: git rev-parse ") {
		t.Errorf("a change to capture into and no repository: RunHook = (%d, %q), want a deny in git's words", code, message)
	}
	if _, err := svc.Capture("orphan"); err == nil {
		t.Error("Capture outside a repository succeeded")
	}
}

// A root that is a symlink to the repository, or a relative path to it, finds the same
// stores as the real path.
func TestARootGivenAsASymlinkOrARelativePathFindsTheSameStores(t *testing.T) {
	repo := gitFixtureRepo(t)
	want, err := storeFor(t, repo).Stores()
	if err != nil {
		t.Fatalf("Stores: %v", err)
	}

	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(repo, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(cwd, repo)
	if err != nil {
		t.Skipf("no relative path from %s to %s: %v", cwd, repo, err)
	}

	for name, root := range map[string]string{"a symlink": link, "a relative path": rel} {
		got, err := storeFor(t, root).Stores()
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("Stores from %s = %v, %v, want %v", name, got, err, want)
		}
	}
}

// A repository that gitprov refuses is refused by the hook too, in gitprov's words, not
// trusted: a gitfile naming a git directory that does not name this tree back (a forged or
// copied one), and a GIT_DIR in the environment that points git elsewhere. The hook is
// fail-closed, so it denies the acknowledgement.
func TestARepositoryGitprovRefusesIsRefusedByTheHook(t *testing.T) {
	repo := gitFixtureRepo(t)
	writeReceipt(t, gitDirOf(repo), "review-one", "gentle-ai.review-receipt/v2", "approved")

	t.Run("a forged gitfile", func(t *testing.T) {
		forged := filepath.Join(filepath.Dir(repo), filepath.Base(repo)+"-forged")
		if err := os.MkdirAll(forged, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(forged, ".git"), []byte("gitdir: "+gitDirOf(repo)+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		seedActiveChange(t, forged, "c")

		code, message := serviceFor(t, forged).RunHook([]byte(ackHookInput))
		if code != 2 || !strings.Contains(message, "core.worktree") {
			t.Errorf("RunHook = (%d, %q), want a deny that says why the worktree is not trusted", code, message)
		}
		if _, err := os.Stat(receiptsOf(forged, "c")); !os.IsNotExist(err) {
			t.Errorf("a receipt was captured from the repository a forged gitfile names (stat err=%v)", err)
		}
	})
	t.Run("GIT_DIR in the environment", func(t *testing.T) {
		seedActiveChange(t, repo, "c")
		t.Setenv("GIT_DIR", gitDirOf(repo))

		code, message := serviceFor(t, repo).RunHook([]byte(ackHookInput))
		if code != 2 || !strings.Contains(message, "GIT_DIR") {
			t.Errorf("RunHook = (%d, %q), want a deny naming GIT_DIR", code, message)
		}
	})
}
