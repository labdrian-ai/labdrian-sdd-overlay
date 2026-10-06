package fsstore_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gitprov"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewreceipt"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewreceipt/fsstore"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewreceipt/receipttest"
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
	data := []byte(receipttest.ReceiptDocument(lineage, schema, terminalState))
	if err := os.WriteFile(filepath.Join(lineageDir, "review-receipt.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}

// writeReviewState writes review-state.json (gentle-ai 2.7.0+ shape) under gitDir's
// transaction store.
func writeReviewState(t *testing.T, gitDir, lineage, state string) {
	t.Helper()
	lineageDir := filepath.Join(transactionStore(gitDir), lineage)
	if err := os.MkdirAll(lineageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lineageDir, "review-state.json"), []byte(receipttest.StateDocument(lineage, state)), 0644); err != nil {
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
	writeReceipt(t, gitDirOf(repo), "review-approved1", receipttest.ReceiptSchema, receipttest.Approved)
	writeReceipt(t, gitDirOf(repo), "review-wrongschema", "gentle-ai.review-receipt/v1", "approved")
	writeReceipt(t, gitDirOf(repo), "review-notapproved", receipttest.ReceiptSchema, "declined")

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
	writeReceipt(t, gitDirOf(repo), "review-both1", receipttest.ReceiptSchema, receipttest.Approved)
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
	writeReceipt(t, gitDirOf(repo), "review-atomic1", receipttest.ReceiptSchema, receipttest.Approved)

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

const ackCommand = "gentle-ai review acknowledge-approved --cwd /repo --lineage review-x"

func TestHookPassesThroughWithoutOpenspecChanges(t *testing.T) {
	repo := gitFixtureRepo(t)
	writeReceipt(t, gitDirOf(repo), "review-orphan", receipttest.ReceiptSchema, receipttest.Approved)

	if v := serviceFor(t, repo).CheckCommand(ackCommand); v.Deny || v.Reason != "" {
		t.Errorf("CheckCommand = %+v, want an allow", v)
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
	writeReceipt(t, gitDirOf(repo), "review-captured", receipttest.ReceiptSchema, receipttest.Approved)
	svc := serviceFor(t, repo)

	v := svc.CheckCommand(ackCommand)
	if !v.Deny || !strings.Contains(v.Reason, "review-receipt capture --change <name>") {
		t.Fatalf("first run: CheckCommand = %+v, want a deny that names the remedy", v)
	}

	if _, err := svc.Capture("change-a"); err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if v := svc.CheckCommand(ackCommand); v.Deny {
		t.Fatalf("after the remedy: CheckCommand = %+v, want an allow", v)
	}

	writeReceipt(t, gitDirOf(repo), "review-uncaptured", receipttest.ReceiptSchema, receipttest.Approved)
	if v := svc.CheckCommand(ackCommand); !v.Deny || !strings.Contains(v.Reason, "review-receipt capture --change <name>") {
		t.Errorf("one lineage still uncaptured: CheckCommand = %+v, want a deny", v)
	}
}

func TestHookCapturesIntoTheSingleActiveChange(t *testing.T) {
	repo := gitFixtureRepo(t)
	seedActiveChange(t, repo, "only-change")
	writeReceipt(t, gitDirOf(repo), "review-solo", receipttest.ReceiptSchema, receipttest.Approved)

	if v := serviceFor(t, repo).CheckCommand(ackCommand); v.Deny || v.Reason != "" {
		t.Fatalf("CheckCommand = %+v, want an allow", v)
	}
	if _, err := os.Stat(filepath.Join(receiptsOf(repo, "only-change"), "review-solo.json")); err != nil {
		t.Errorf("expected receipt captured: %v", err)
	}
}

func TestHookIgnoresACommandThatIsNotAnAcknowledgement(t *testing.T) {
	repo := gitFixtureRepo(t)
	seedActiveChange(t, repo, "only-change")
	writeReceipt(t, gitDirOf(repo), "review-untouched", receipttest.ReceiptSchema, receipttest.Approved)

	if v := serviceFor(t, repo).CheckCommand("git status"); v.Deny || v.Reason != "" {
		t.Fatalf("CheckCommand = %+v, want an allow", v)
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

// openspec/ is found from the toplevel of the working tree the hook is started in, not from the
// directory it was given: a session started below its project captures into the project's
// change, from the same stores. A directory inside the repository that has an openspec of its
// own is served by it only when the toplevel has none (the receipt has nowhere else to go);
// when both have one, the toplevel's is the project's. Until Phase 9 batch 12a the directory
// the hook was given decided, and a session started in a subdirectory passed the
// acknowledgement through and lost the receipt.
func TestAHookStartedInsideTheRepositoryFindsOpenspecFromTheToplevel(t *testing.T) {
	repo := gitFixtureRepo(t)
	seedActiveChange(t, repo, "top-change")
	writeReceipt(t, gitDirOf(repo), "review-from-below", receipttest.ReceiptSchema, receipttest.Approved)

	plain := filepath.Join(repo, "plain", "deeper")
	own := filepath.Join(repo, "own")
	for _, dir := range []string{plain, own} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	seedActiveChange(t, own, "own-change")

	for name, dir := range map[string]string{"a directory with no openspec": plain, "a directory with its own openspec": own, "the toplevel": repo} {
		t.Run(name, func(t *testing.T) {
			if err := os.RemoveAll(receiptsOf(repo, "top-change")); err != nil {
				t.Fatal(err)
			}
			if v := serviceFor(t, dir).CheckCommand(ackCommand); v.Deny || v.Reason != "" {
				t.Fatalf("CheckCommand = %+v, want an allow", v)
			}
			if _, err := os.Stat(filepath.Join(receiptsOf(repo, "top-change"), "review-from-below.json")); err != nil {
				t.Errorf("expected the receipt captured into the toplevel's change: %v", err)
			}
			if _, err := os.Stat(receiptsOf(own, "own-change")); !os.IsNotExist(err) {
				t.Errorf("a receipt was captured under the subdirectory's own openspec although the toplevel has one (stat err=%v)", err)
			}
		})
	}
}

// With no openspec/changes at the toplevel the directory the hook was given is the only place
// that can hold the change, and it is served from there as it always was; with none anywhere the
// acknowledgement passes through and nothing is created.
func TestAHookInARepositoryWithoutOpenspecAtTheToplevelFallsBackToItsOwnDirectory(t *testing.T) {
	repo := gitFixtureRepo(t)
	own := filepath.Join(repo, "own")
	bare := filepath.Join(repo, "bare")
	for _, dir := range []string{own, bare} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	seedActiveChange(t, own, "own-change")
	writeReceipt(t, gitDirOf(repo), "review-from-below", receipttest.ReceiptSchema, receipttest.Approved)

	if v := serviceFor(t, bare).CheckCommand(ackCommand); v.Deny || v.Reason != "" {
		t.Errorf("with no openspec anywhere: CheckCommand = %+v, want it to pass through", v)
	}
	if _, err := os.Stat(filepath.Join(repo, "openspec")); !os.IsNotExist(err) {
		t.Errorf("an openspec directory was created at the toplevel (stat err=%v)", err)
	}
	if v := serviceFor(t, own).CheckCommand(ackCommand); v.Deny || v.Reason != "" {
		t.Fatalf("from a directory with its own openspec: CheckCommand = %+v", v)
	}
	if _, err := os.Stat(filepath.Join(receiptsOf(own, "own-change"), "review-from-below.json")); err != nil {
		t.Errorf("expected the receipt captured under the directory's own openspec: %v", err)
	}
}

// A linked working tree has its own toplevel, with its own openspec: a hook started below it
// captures into that one, from both stores of the repository.
func TestAHookStartedInsideALinkedWorkingTreeCapturesIntoItsToplevel(t *testing.T) {
	repo := gitFixtureRepo(t)
	linked := filepath.Join(filepath.Dir(repo), filepath.Base(repo)+"-linked")
	runGit(t, repo, "worktree", "add", "-q", linked, "-b", "linked-branch")
	sub := filepath.Join(linked, "src")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	seedActiveChange(t, linked, "linked-change")
	writeReceipt(t, filepath.Join(gitDirOf(repo), "worktrees", filepath.Base(linked)), "review-private", receipttest.ReceiptSchema, receipttest.Approved)
	writeReceipt(t, gitDirOf(repo), "review-common", receipttest.ReceiptSchema, receipttest.Approved)

	if v := serviceFor(t, sub).CheckCommand(ackCommand); v.Deny || v.Reason != "" {
		t.Fatalf("CheckCommand = %+v, want an allow", v)
	}
	for _, name := range []string{"review-private.json", "review-common.json"} {
		if _, err := os.Stat(filepath.Join(receiptsOf(linked, "linked-change"), name)); err != nil {
			t.Errorf("expected %s captured into the linked working tree's change: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(repo, "openspec")); !os.IsNotExist(err) {
		t.Errorf("something was created under the main working tree's openspec (stat err=%v)", err)
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
	writeReceipt(t, gitDirOf(repo), "review-first", receipttest.ReceiptSchema, receipttest.Approved)

	if v := serviceFor(t, repo).CheckCommand(ackCommand); v.Deny || v.Reason != "" {
		t.Fatalf("CheckCommand = %+v, want an allow", v)
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

	if v := svc.CheckCommand(ackCommand); v.Deny || v.Reason != "" {
		t.Errorf("no change to capture into: CheckCommand = %+v, want an allow", v)
	}

	seedActiveChange(t, dir, "orphan")
	v := svc.CheckCommand(ackCommand)
	if !v.Deny || !strings.HasPrefix(v.Reason, "review-receipt: capture failed: reviewreceipt: git rev-parse ") {
		t.Errorf("a change to capture into and no repository: CheckCommand = %+v, want a deny in git's words", v)
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
	writeReceipt(t, gitDirOf(repo), "review-one", receipttest.ReceiptSchema, receipttest.Approved)

	t.Run("a forged gitfile", func(t *testing.T) {
		forged := filepath.Join(filepath.Dir(repo), filepath.Base(repo)+"-forged")
		if err := os.MkdirAll(forged, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(forged, ".git"), []byte("gitdir: "+gitDirOf(repo)+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		seedActiveChange(t, forged, "c")

		v := serviceFor(t, forged).CheckCommand(ackCommand)
		if !v.Deny || !strings.Contains(v.Reason, "core.worktree") {
			t.Errorf("CheckCommand = %+v, want a deny that says why the worktree is not trusted", v)
		}
		if _, err := os.Stat(receiptsOf(forged, "c")); !os.IsNotExist(err) {
			t.Errorf("a receipt was captured from the repository a forged gitfile names (stat err=%v)", err)
		}
	})
	t.Run("GIT_DIR in the environment", func(t *testing.T) {
		seedActiveChange(t, repo, "c")
		t.Setenv("GIT_DIR", gitDirOf(repo))

		v := serviceFor(t, repo).CheckCommand(ackCommand)
		if !v.Deny || !strings.Contains(v.Reason, "GIT_DIR") {
			t.Errorf("CheckCommand = %+v, want a deny naming GIT_DIR", v)
		}
	})
}

// The hook is fail-closed: a toplevel whose openspec/ it cannot look at is not a toplevel without
// one. Treated as an absence, the hook would serve the directory it was started in, capture the
// receipt there and let the acknowledgement through, when the change the receipt belongs to is
// at the toplevel it could not look at.
func TestAHookDeniesWhenTheToplevelOpenspecCannotBeLookedAt(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("a directory without permissions does not stop root")
	}
	repo := gitFixtureRepo(t)
	sub := filepath.Join(repo, "sub")
	seedActiveChange(t, repo, "top-change")
	seedActiveChange(t, sub, "sub-change")
	writeReceipt(t, gitDirOf(repo), "review-from-below", receipttest.ReceiptSchema, receipttest.Approved)
	closed := filepath.Join(repo, "openspec")
	if err := os.Chmod(closed, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(closed, 0o755) })

	v := serviceFor(t, sub).CheckCommand(ackCommand)
	if !v.Deny || !strings.Contains(v.Reason, "stat "+filepath.Join(repo, "openspec", "changes")) {
		t.Errorf("CheckCommand = %+v, want a denial that names the path it could not look at", v)
	}
	if _, err := os.Stat(receiptsOf(sub, "sub-change")); !os.IsNotExist(err) {
		t.Errorf("a receipt was captured under the subdirectory's own openspec (stat err=%v)", err)
	}
}
