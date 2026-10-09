package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// The golden files under testdata/repo-golden record how the repository a verb or a hook is run
// in is found, from the outside: the key that names the binding of a directory, and the worktree
// root and HEAD a workflow created there records. The repositories are laid out by hand (a
// .git directory, a .git file that points at another, a worktree that names its common
// directory), and the program is never given git.

// hash is the key the design specifies for the path of a git common directory that cannot be
// resolved: the digest of the cleaned path itself.
func hashOfPath(path string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(path)))
	return hex.EncodeToString(sum[:])
}

// rawKey registers the digest of the cleaned path commonDir, with no symbolic link resolved, as
// label: the key of a common directory that cannot be resolved.
func (w *repoWorld) rawKey(commonDir, label string) { w.keys[hashOfPath(commonDir)] = label }

// linked adds a linked worktree to the repository at mainRoot, in the layout `git worktree add`
// creates (the worktree's .git file points at .git/worktrees/<name>, whose commondir file leads
// back to the main .git, written as commondir), and returns the worktree, written as label.
func (w *repoWorld) linked(mainRoot, name, commondir, label string) string {
	w.t.Helper()
	gitDir := filepath.Join(mainRoot, ".git", "worktrees", name)
	writeFixtureFile(w.t, filepath.Join(gitDir, "HEAD"), strings.Repeat("b", 40)+"\n")
	if commondir != "" {
		writeFixtureFile(w.t, filepath.Join(gitDir, "commondir"), commondir+"\n")
	}
	worktree := w.place(label)
	writeFixtureFile(w.t, filepath.Join(worktree, ".git"), "gitdir: "+gitDir+"\n")
	return worktree
}

// probe creates a workflow from cwd and says what it recorded of the repository (the worktree
// root and HEAD of its first event), then binds it from cwd, says what binding reports and
// unbinds. Creating needs no repository: outside one the record is empty.
func (w *repoWorld) probe(label, cwd string) {
	w.t.Helper()
	w.probed++
	wf := "wf-" + strconv.Itoa(w.probed)
	w.workflowIn(cwd, "proj-1", wf, "running")
	w.provenance(label, "proj-1", wf)
	w.run(label+": bind", cwd, "bind", "--project", "proj-1", "--workflow", wf)
	w.run(label+": binding", cwd, "binding")
	if r := runWorkflowTest([]string{"unbind"}, cwd); r.code != 0 && !strings.Contains(r.stderr, "needs a git repository") {
		w.t.Fatalf("unbind after the probe: exit %d, stderr %q", r.code, r.stderr)
	}
}

// provenance records the worktree root and HEAD the first event of a workflow holds.
func (w *repoWorld) provenance(label, project, wf string) {
	w.t.Helper()
	data, err := os.ReadFile(phase6WorkflowLogPath(w.state, project, wf))
	if err != nil {
		w.t.Fatal(err)
	}
	first, _, _ := bytes.Cut(data, []byte("\n"))
	var event struct {
		Provenance workflow.Provenance `json:"provenance"`
	}
	if err := json.Unmarshal(first, &event); err != nil {
		w.t.Fatal(err)
	}
	fmt.Fprintf(&w.b, "# %s: the workflow records worktree_root=%q git_head=%s\n", label, event.Provenance.WorktreeRoot, shownHead(event.Provenance.GitHead))
}

// shownHead writes a HEAD the fixtures made of one repeated digit as the count and the digit, so
// that the transcript keeps the value and not a <DIGEST>; the empty one is written as "".
func shownHead(head string) string {
	if head == "" {
		return `""`
	}
	if strings.Count(head, head[:1]) == len(head) {
		return fmt.Sprintf("<%d of %s>", len(head), head[:1])
	}
	return strconv.Quote(head)
}

// hook records one run of 'projection hook --event <event>' with stdin, in the process directory
// processCwd.
func (w *repoWorld) hook(label, event, stdin, processCwd string) {
	w.t.Helper()
	var stdout, stderr bytes.Buffer
	var exits []int
	runProjectionCore([]string{"hook", "--event", event}, processCwd, strings.NewReader(stdin), &stdout, &stderr, func(c int) { exits = append(exits, c) })
	fmt.Fprintf(&w.b, "$ projection hook --event %s\n# %s (stdin %s, in %s)\n--- exit ---\n%v\n--- stdout ---\n%s--- stderr ---\n%s\n",
		event, label, stdin, w.cwdName(processCwd), exits, w.shown(stdout.String()), w.shown(stderr.String()))
}

func repoGoldenCases() []repoGoldenCase {
	return []repoGoldenCase{
		{"key-of-a-plain-repository", func(w *repoWorld) {
			repo := w.plainRepo("<REPO>", "<KEY>")
			w.probe("at the root", repo)
			deep := filepath.Join(repo, "a", "b", "c")
			if err := os.MkdirAll(deep, 0o700); err != nil {
				w.t.Fatal(err)
			}
			w.name(deep, "<REPO>/a/b/c")
			w.probe("in a directory inside it", deep)
			outside := w.place("<OUTSIDE>")
			alias := filepath.Join(outside, "alias")
			if err := os.Symlink(repo, alias); err != nil {
				w.t.Skipf("symbolic links are not available: %v", err)
			}
			w.name(alias, "<ALIAS>")
			w.probe("through a symbolic link to the repository", alias)
			w.probe("through a symbolic link, from a directory inside it", filepath.Join(alias, "a", "b"))
		}},
		{"key-of-linked-worktrees", func(w *repoWorld) {
			main := w.plainRepo("<MAIN>", "<KEY>")
			relative := w.linked(main, "relative", "../..", "<WORKTREE relative>")
			absolute := w.linked(main, "absolute", filepath.Join(main, ".git"), "<WORKTREE absolute>")
			unclean := w.linked(main, "unclean", "../.././/", "<WORKTREE unclean>")
			w.probe("the main worktree", main)
			w.probe("a linked worktree with a relative commondir", relative)
			w.probe("a linked worktree with an absolute commondir", absolute)
			w.probe("a linked worktree with a commondir that needs cleaning", unclean)
			sub := filepath.Join(relative, "src")
			if err := os.MkdirAll(sub, 0o700); err != nil {
				w.t.Fatal(err)
			}
			w.name(sub, "<WORKTREE relative>/src")
			w.probe("a directory inside a linked worktree", sub)
		}},
		{"git-files-that-point-at-a-git-directory", func(w *repoWorld) {
			base := w.place("<BASE>")
			gitDir := filepath.Join(base, "modules", "gitdir")
			writeFixtureFile(w.t, filepath.Join(gitDir, "HEAD"), strings.Repeat("c", 40)+"\n")
			w.name(gitDir, "<GITDIR>")
			w.key(gitDir, "<KEY gitdir>")
			for i, p := range []struct{ label, content string }{
				{"a pointer", "gitdir: " + gitDir + "\n"},
				{"a pointer without a line break", "gitdir: " + gitDir},
				{"a pointer with spaces around it", "  gitdir:    " + gitDir + "   \n\n"},
				{"a pointer with a carriage return", "gitdir: " + gitDir + "\r\n"},
				{"a pointer with no space after the colon", "gitdir:" + gitDir + "\n"},
				{"a pointer to a path relative to the checkout", "gitdir: ../modules/gitdir\n"},
				{"a pointer to a path that needs cleaning", "gitdir: ../modules/../modules/./gitdir/\n"},
			} {
				dir := filepath.Join(base, "checkout"+strconv.Itoa(i))
				writeFixtureFile(w.t, filepath.Join(dir, ".git"), p.content)
				w.name(dir, "<CHECKOUT>")
				w.probe(p.label, dir)
			}
		}},
		{"git-files-and-entries-that-cannot-be-used", func(w *repoWorld) {
			base := w.place("<BASE>")
			notDir := filepath.Join(base, "a-file")
			writeFixtureFile(w.t, notDir, "x")
			outer := w.plainRepo("<OUTER>", "<KEY outer>")
			cases := []struct{ label, content string }{
				{"no gitdir line", "not a gitdir pointer\n"},
				{"an empty file", ""},
				{"a gitdir line with no path", "gitdir:\n"},
				{"a gitdir line with only spaces", "gitdir:    \n"},
				{"a path that does not exist", "gitdir: " + filepath.Join(base, "gone") + "\n"},
				{"a path that is a file", "gitdir: " + notDir + "\n"},
				{"a pointer followed by another line", "gitdir: " + base + "\nsecond line\n"},
				{"a capital G", "Gitdir: " + base + "\n"},
			}
			for i, c := range cases {
				dir := filepath.Join(outer, "inner"+strconv.Itoa(i))
				writeFixtureFile(w.t, filepath.Join(dir, ".git"), c.content)
				w.name(dir, "<INNER>")
				w.probe(c.label+" (inside a repository that has a .git of its own)", dir)
			}
			broken := filepath.Join(outer, "broken")
			if err := os.MkdirAll(broken, 0o700); err != nil {
				w.t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(base, "gone"), filepath.Join(broken, ".git")); err != nil {
				w.t.Skipf("symbolic links are not available: %v", err)
			}
			w.name(broken, "<INNER>")
			w.probe("a .git that is a broken symbolic link", broken)
		}},
		{"a-git-entry-that-is-a-symbolic-link", func(w *repoWorld) {
			base := w.place("<BASE>")
			actual := filepath.Join(base, "actual")
			writeFixtureFile(w.t, filepath.Join(actual, "HEAD"), strings.Repeat("d", 40)+"\n")
			w.name(actual, "<ACTUAL>")
			w.key(actual, "<KEY actual>")
			toDir := filepath.Join(base, "to-dir")
			if err := os.MkdirAll(toDir, 0o700); err != nil {
				w.t.Fatal(err)
			}
			if err := os.Symlink(actual, filepath.Join(toDir, ".git")); err != nil {
				w.t.Skipf("symbolic links are not available: %v", err)
			}
			w.name(toDir, "<CHECKOUT to a directory>")
			w.probe("a .git that is a link to a git directory", toDir)

			pointer := filepath.Join(base, "pointer-file")
			writeFixtureFile(w.t, pointer, "gitdir: "+actual+"\n")
			toFile := filepath.Join(base, "to-file")
			if err := os.MkdirAll(toFile, 0o700); err != nil {
				w.t.Fatal(err)
			}
			if err := os.Symlink(pointer, filepath.Join(toFile, ".git")); err != nil {
				w.t.Fatal(err)
			}
			w.name(toFile, "<CHECKOUT to a file>")
			w.probe("a .git that is a link to a pointer file", toFile)
		}},
		{"a-common-directory-that-cannot-be-resolved", func(w *repoWorld) {
			base := w.place("<BASE>")
			gitDir := filepath.Join(base, "gitdir")
			missing := filepath.Join(base, "missing", "common")
			writeFixtureFile(w.t, filepath.Join(gitDir, "HEAD"), strings.Repeat("e", 40)+"\n")
			writeFixtureFile(w.t, filepath.Join(gitDir, "commondir"), missing+"\n")
			w.name(gitDir, "<GITDIR>")
			w.name(missing, "<MISSING>")
			w.rawKey(missing, "<KEY of the path as written>")
			checkout := w.place("<CHECKOUT>")
			writeFixtureFile(w.t, filepath.Join(checkout, ".git"), "gitdir: "+gitDir+"\n")
			w.probe("a commondir that names a directory that does not exist", checkout)

			blank := filepath.Join(base, "blank")
			writeFixtureFile(w.t, filepath.Join(blank, "HEAD"), strings.Repeat("e", 40)+"\n")
			writeFixtureFile(w.t, filepath.Join(blank, "commondir"), "   \n")
			w.name(blank, "<BLANK>")
			w.key(blank, "<KEY blank>")
			second := w.place("<CHECKOUT blank>")
			writeFixtureFile(w.t, filepath.Join(second, ".git"), "gitdir: "+blank+"\n")
			w.probe("an empty commondir: the git directory is its own common directory", second)
		}},
		{"head-is-read-without-running-git", func(w *repoWorld) {
			layouts := []struct {
				label string
				put   func(git string)
			}{
				{"a detached HEAD of 40 hexadecimal digits", func(git string) { writeFixtureFile(w.t, filepath.Join(git, "HEAD"), strings.Repeat("a", 40)+"\n") }},
				{"a detached HEAD of 64 hexadecimal digits", func(git string) { writeFixtureFile(w.t, filepath.Join(git, "HEAD"), strings.Repeat("b", 64)+"\n") }},
				{"a detached HEAD in capitals", func(git string) { writeFixtureFile(w.t, filepath.Join(git, "HEAD"), strings.Repeat("A", 40)+"\n") }},
				{"a detached HEAD of 41 digits", func(git string) { writeFixtureFile(w.t, filepath.Join(git, "HEAD"), strings.Repeat("a", 41)+"\n") }},
				{"a branch with a loose ref", func(git string) {
					writeFixtureFile(w.t, filepath.Join(git, "HEAD"), "ref: refs/heads/main\n")
					writeFixtureFile(w.t, filepath.Join(git, "refs", "heads", "main"), strings.Repeat("c", 40)+"\n")
				}},
				{"a branch with a packed ref", func(git string) {
					writeFixtureFile(w.t, filepath.Join(git, "HEAD"), "ref: refs/heads/main\n")
					writeFixtureFile(w.t, filepath.Join(git, "packed-refs"),
						"# pack-refs with: peeled fully-peeled sorted\n"+strings.Repeat("1", 40)+" refs/heads/other\n^"+strings.Repeat("2", 40)+"\n"+strings.Repeat("d", 40)+" refs/heads/main\n")
				}},
				{"a loose ref that wins over a packed one", func(git string) {
					writeFixtureFile(w.t, filepath.Join(git, "HEAD"), "ref: refs/heads/main\n")
					writeFixtureFile(w.t, filepath.Join(git, "refs", "heads", "main"), strings.Repeat("e", 40)+"\n")
					writeFixtureFile(w.t, filepath.Join(git, "packed-refs"), strings.Repeat("f", 40)+" refs/heads/main\n")
				}},
				{"a loose ref that holds no object name, with a packed one", func(git string) {
					writeFixtureFile(w.t, filepath.Join(git, "HEAD"), "ref: refs/heads/main\n")
					writeFixtureFile(w.t, filepath.Join(git, "refs", "heads", "main"), "garbage\n")
					writeFixtureFile(w.t, filepath.Join(git, "packed-refs"), strings.Repeat("f", 40)+" refs/heads/main\n")
				}},
				{"a branch that has no ref", func(git string) { writeFixtureFile(w.t, filepath.Join(git, "HEAD"), "ref: refs/heads/none\n") }},
				{"a ref with two dots", func(git string) { writeFixtureFile(w.t, filepath.Join(git, "HEAD"), "ref: refs/heads/../main\n") }},
				{"an absolute ref", func(git string) { writeFixtureFile(w.t, filepath.Join(git, "HEAD"), "ref: /etc/passwd\n") }},
				{"an empty ref", func(git string) { writeFixtureFile(w.t, filepath.Join(git, "HEAD"), "ref:\n") }},
				{"a HEAD that is neither", func(git string) { writeFixtureFile(w.t, filepath.Join(git, "HEAD"), "garbage\n") }},
				{"a git directory with no HEAD", func(git string) {
					if err := os.MkdirAll(git, 0o700); err != nil {
						w.t.Fatal(err)
					}
				}},
			}
			for i, l := range layouts {
				repo := w.place("<REPO " + strconv.Itoa(i+1) + ">")
				l.put(filepath.Join(repo, ".git"))
				w.workflowIn(repo, "proj-1", "wf-"+strconv.Itoa(i+1), "running")
				w.provenance(l.label, "proj-1", "wf-"+strconv.Itoa(i+1))
			}
			main := w.place("<MAIN>")
			writeFixtureFile(w.t, filepath.Join(main, ".git", "HEAD"), "ref: refs/heads/main\n")
			writeFixtureFile(w.t, filepath.Join(main, ".git", "refs", "heads", "feature"), strings.Repeat("9", 40)+"\n")
			worktree := w.linked(main, "wt", "../..", "<WORKTREE>")
			writeFixtureFile(w.t, filepath.Join(main, ".git", "worktrees", "wt", "HEAD"), "ref: refs/heads/feature\n")
			w.workflowIn(worktree, "proj-1", "wf-worktree", "running")
			w.provenance("a linked worktree whose branch lives in the common directory", "proj-1", "wf-worktree")
			second := w.linked(main, "other", "../..", "<WORKTREE other>")
			writeFixtureFile(w.t, filepath.Join(main, ".git", "worktrees", "other", "HEAD"), "ref: refs/heads/feature\n")
			writeFixtureFile(w.t, filepath.Join(main, ".git", "worktrees", "other", "refs", "heads", "feature"), strings.Repeat("8", 40)+"\n")
			w.workflowIn(second, "proj-1", "wf-other", "running")
			w.provenance("a linked worktree with a ref of its own: its git directory is looked in first", "proj-1", "wf-other")
			outside := w.place("<OUTSIDE>")
			w.workflowIn(outside, "proj-1", "wf-outside", "running")
			w.provenance("a directory outside every repository", "proj-1", "wf-outside")
			w.workflowIn("", "proj-1", "wf-nowhere", "running")
			w.provenance("an unknown working directory", "proj-1", "wf-nowhere")
			w.workflowIn("relative/dir", "proj-1", "wf-relative", "running")
			w.provenance("a relative working directory", "proj-1", "wf-relative")
		}},
		{"hooks-find-the-repository-the-way-the-verbs-do", func(w *repoWorld) {
			main := w.plainRepo("<MAIN>", "<KEY>")
			linked := w.linked(main, "wt", "../..", "<WORKTREE>")
			outside := w.place("<OUTSIDE>")
			inner := w.place("<INNER>")
			writeFixtureFile(w.t, filepath.Join(inner, ".git"), "not a gitdir pointer\n")
			sub := filepath.Join(linked, "src", "pkg")
			if err := os.MkdirAll(sub, 0o700); err != nil {
				w.t.Fatal(err)
			}
			w.name(sub, "<WORKTREE>/src/pkg")
			w.workflowIn(main, "proj-1", "wf-1", "running")
			w.run("binds the workflow from the main worktree", main, "bind", "--project", "proj-1", "--workflow", "wf-1")

			prompt := func(cwd string) string {
				return `{"hook_event_name":"UserPromptSubmit","cwd":` + strconv.Quote(cwd) + `}`
			}
			tool := func(cwd string) string {
				return `{"hook_event_name":"PreToolUse","tool_name":"Edit","cwd":` + strconv.Quote(cwd) + `}`
			}
			w.hook("the prompt in the main worktree", "UserPromptSubmit", prompt(main), outside)
			w.hook("the prompt in a linked worktree", "UserPromptSubmit", prompt(linked), outside)
			w.hook("the prompt in a directory inside a linked worktree", "UserPromptSubmit", prompt(sub), outside)
			w.hook("the prompt with no directory in the input falls back to the process directory", "UserPromptSubmit", `{"hook_event_name":"UserPromptSubmit"}`, linked)
			w.hook("the prompt outside every repository says nothing", "UserPromptSubmit", prompt(outside), main)
			w.hook("the prompt in a checkout whose .git cannot be used says nothing", "UserPromptSubmit", prompt(inner), main)
			w.hook("the prompt with a relative directory in the input uses the process directory, the decoder drops the relative one", "UserPromptSubmit", prompt("relative/dir"), main)
			w.setup(main, "pause", "--project", "proj-1", "--workflow", "wf-1")
			w.hook("an edit in a linked worktree of a paused workflow is denied", "PreToolUse", tool(linked), outside)
			w.hook("an edit outside every repository is allowed", "PreToolUse", tool(outside), main)
			w.hook("an edit in a checkout whose .git cannot be used is allowed", "PreToolUse", tool(inner), main)
		}},
	}
}

// TestRepoGolden runs every case of the finding of a repository and compares its transcript with
// its golden file.
func TestRepoGolden(t *testing.T) {
	checkRepoGoldenCases(t, "repo-golden", repoGoldenCases(), *updateRepoGolden)
}
