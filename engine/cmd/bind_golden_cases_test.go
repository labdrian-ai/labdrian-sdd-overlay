package main

import (
	"os"
	"path/filepath"
	"strings"
)

// plainRepo makes a hand-made repository (a .git directory with a detached HEAD) written as
// label in the transcript, and registers its key as keyLabel.
func (w *repoWorld) plainRepo(label, keyLabel string) string {
	w.t.Helper()
	root := w.place(label)
	writeFixtureFile(w.t, filepath.Join(root, ".git", "HEAD"), strings.Repeat("a", 40)+"\n")
	w.key(filepath.Join(root, ".git"), keyLabel)
	return root
}

// bindingFile is where the binding of the repository whose common directory is commonDir is kept.
func (w *repoWorld) bindingFile(commonDir string) string {
	w.t.Helper()
	return filepath.Join(w.state, "labdrian", "bindings", wantRepoKey(w.t, commonDir)+".json")
}

func bindGoldenCases() []repoGoldenCase {
	return []repoGoldenCase{
		{"bind-binds-an-open-workflow-and-keeps-it", func(w *repoWorld) {
			repo := w.plainRepo("<REPO>", "<KEY>")
			w.workflowIn(repo, "proj-1", "wf-1", "running")
			w.run("unbind with nothing bound removes nothing", repo, "unbind")
			w.run("binds a running workflow", repo, "bind", "--project", "proj-1", "--workflow", "wf-1")
			w.run("binding names it and the workflow", repo, "binding")
			file := w.bindingFile(filepath.Join(repo, ".git"))
			first, err := os.ReadFile(file)
			if err != nil {
				w.t.Fatal(err)
			}
			w.run("binding the same workflow again changes nothing, bound_at included", repo, "bind", "--project", "proj-1", "--workflow", "wf-1")
			second, err := os.ReadFile(file)
			if err != nil {
				w.t.Fatal(err)
			}
			w.note("the binding file is byte for byte the first one: %v", string(first) == string(second))
			w.storeFiles("after binding twice")
			w.workflowIn(repo, "proj-1", "wf-2", "created")
			w.run("unbind removes the binding", repo, "unbind")
			w.run("unbind a second time removes nothing", repo, "unbind")
			w.storeFiles("after unbinding")
			w.run("binds a created workflow", repo, "bind", "--project", "proj-1", "--workflow", "wf-2")
			w.run("binding shows its status", repo, "binding")
		}},
		{"bind-refuses-a-workflow-it-cannot-bind", func(w *repoWorld) {
			repo := w.plainRepo("<REPO>", "<KEY>")
			w.workflowIn(repo, "proj-1", "wf-closed", "closed")
			w.workflowIn(repo, "proj-1", "wf-drifted", "running")
			w.setup(repo, "pause", "--project", "proj-1", "--workflow", "wf-drifted")
			driftWorkflowLog(w.t, phase6WorkflowLogPath(w.state, "proj-1", "wf-drifted"))
			w.run("a workflow that does not exist", repo, "bind", "--project", "proj-1", "--workflow", "wf-none")
			w.run("a closed workflow", repo, "bind", "--project", "proj-1", "--workflow", "wf-closed")
			w.run("a workflow whose log drifted", repo, "bind", "--project", "proj-1", "--workflow", "wf-drifted")
			w.run("a project id that is not valid", repo, "bind", "--project", "no spaces", "--workflow", "wf-1")
			w.run("a workflow id that is not valid", repo, "bind", "--project", "proj-1", "--workflow", "../wf")
			w.run("nothing was bound", repo, "binding")
			w.storeFiles("after the refusals")
		}},
		{"bind-refuses-to-replace-a-binding-that-may-be-active", func(w *repoWorld) {
			repo := w.plainRepo("<REPO>", "<KEY>")
			for _, status := range []string{"created", "running", "paused"} {
				w.workflowIn(repo, "proj-1", "wf-"+status, status)
			}
			w.workflowIn(repo, "proj-1", "wf-next", "running")
			for _, status := range []string{"created", "running", "paused"} {
				w.run("bound to a "+status+" workflow", repo, "bind", "--project", "proj-1", "--workflow", "wf-"+status)
				w.run("another workflow is refused while it is "+status, repo, "bind", "--project", "proj-1", "--workflow", "wf-next")
				w.run("unbind frees the repository", repo, "unbind")
			}
			w.run("the repository is free", repo, "binding")
		}},
		{"bind-replaces-a-binding-that-can-never-be-followed-again", func(w *repoWorld) {
			repo := w.plainRepo("<REPO>", "<KEY>")
			w.workflowIn(repo, "proj-1", "wf-closed", "running")
			w.workflowIn(repo, "proj-1", "wf-gone", "running")
			w.workflowIn(repo, "proj-1", "wf-drifted", "running")
			w.setup(repo, "pause", "--project", "proj-1", "--workflow", "wf-drifted")
			w.workflowIn(repo, "proj-1", "wf-new", "running")

			w.run("bound to a workflow that is later closed", repo, "bind", "--project", "proj-1", "--workflow", "wf-closed")
			w.setup(repo, "close", "--project", "proj-1", "--workflow", "wf-closed", "--outcome", "abandoned", "--reason", "test")
			w.run("binding reports the workflow as closed", repo, "binding")
			w.run("replaced by the next one", repo, "bind", "--project", "proj-1", "--workflow", "wf-new")
			w.run("unbind", repo, "unbind")

			w.run("bound to a workflow whose log is later removed", repo, "bind", "--project", "proj-1", "--workflow", "wf-gone")
			if err := os.Remove(phase6WorkflowLogPath(w.state, "proj-1", "wf-gone")); err != nil {
				w.t.Fatal(err)
			}
			w.run("binding reports the workflow as absent", repo, "binding")
			w.run("replaced by the next one", repo, "bind", "--project", "proj-1", "--workflow", "wf-new")
			w.run("unbind", repo, "unbind")

			w.run("bound to a workflow whose log later drifts", repo, "bind", "--project", "proj-1", "--workflow", "wf-drifted")
			driftWorkflowLog(w.t, phase6WorkflowLogPath(w.state, "proj-1", "wf-drifted"))
			w.run("binding reports the workflow as drifted", repo, "binding")
			w.run("replaced by the next one", repo, "bind", "--project", "proj-1", "--workflow", "wf-new")
			w.storeFiles("after the replacements")
		}},
		{"bind-refuses-a-binding-whose-workflow-cannot-be-read", func(w *repoWorld) {
			repo := w.plainRepo("<REPO>", "<KEY>")
			w.workflowIn(repo, "proj-1", "wf-1", "running")
			w.workflowIn(repo, "proj-1", "wf-next", "running")
			w.run("bound to a running workflow", repo, "bind", "--project", "proj-1", "--workflow", "wf-1")
			log := phase6WorkflowLogPath(w.state, "proj-1", "wf-1")
			if err := os.Remove(log); err != nil {
				w.t.Fatal(err)
			}
			if err := os.Mkdir(log, 0o700); err != nil {
				w.t.Fatal(err)
			}
			w.run("binding reports the workflow as unavailable", repo, "binding")
			w.run("another workflow is refused, the log may still be active", repo, "bind", "--project", "proj-1", "--workflow", "wf-next")
		}},
		{"binding-describes-what-is-bound", func(w *repoWorld) {
			repo := w.plainRepo("<REPO>", "<KEY>")
			w.run("nothing is bound", repo, "binding")
			w.workflowIn(repo, "proj-1", "wf-1", "running")
			w.run("binds", repo, "bind", "--project", "proj-1", "--workflow", "wf-1")
			w.setup(repo, "pause", "--project", "proj-1", "--workflow", "wf-1")
			w.run("the workflow is paused", repo, "binding")
			w.setup(repo, "close", "--project", "proj-1", "--workflow", "wf-1", "--outcome", "abandoned", "--reason", "done")
			w.run("the workflow is closed", repo, "binding")
			sub := filepath.Join(repo, "a", "b")
			if err := os.MkdirAll(sub, 0o700); err != nil {
				w.t.Fatal(err)
			}
			w.name(sub, "<REPO>/a/b")
			w.run("from a directory inside the repository", sub, "binding")
		}},
		{"verbs-refuse-a-binding-file-that-is-not-ours", func(w *repoWorld) {
			repo := w.plainRepo("<REPO>", "<KEY>")
			w.workflowIn(repo, "proj-1", "wf-1", "running")
			file := w.bindingFile(filepath.Join(repo, ".git"))
			for _, doc := range []struct{ label, content string }{
				{"a foreign file", `{"version": 2, "other": true}`},
				{"a malformed file", "this is not a binding"},
				{"an empty file", ""},
			} {
				writeFixtureFile(w.t, file, doc.content)
				w.run("binding reports "+doc.label, repo, "binding")
				w.run("bind refuses "+doc.label, repo, "bind", "--project", "proj-1", "--workflow", "wf-1")
				w.run("unbind refuses "+doc.label, repo, "unbind")
				w.storeFiles(doc.label + " is left as it was")
			}
		}},
		{"verbs-need-a-repository", func(w *repoWorld) {
			outside := w.place("<OUTSIDE>")
			repo := w.plainRepo("<REPO>", "<KEY>")
			w.workflowIn(repo, "proj-1", "wf-1", "running")
			w.run("bind outside every repository", outside, "bind", "--project", "proj-1", "--workflow", "wf-1")
			w.run("unbind outside every repository", outside, "unbind")
			w.run("binding outside every repository", outside, "binding")
			w.run("binding with an unknown working directory", "", "binding")
			w.run("bind with a relative working directory", "repo", "bind", "--project", "proj-1", "--workflow", "wf-1")
			w.run("a bad command line is reported before the repository is looked for", outside, "bind", "--project", "proj-1")
		}},
		{"verbs-reject-bad-command-lines", func(w *repoWorld) {
			repo := w.plainRepo("<REPO>", "<KEY>")
			for _, args := range [][]string{
				{"bind"},
				{"bind", "--project", "p"},
				{"bind", "--workflow", "w"},
				{"bind", "--project"},
				{"bind", "--project", "p", "--workflow", "w", "--goal", "g.json"},
				{"bind", "--project", "p", "--workflow", "w", "stray"},
				{"bind", "--project", "p", "--project", "q", "--workflow", "w"},
				{"unbind", "--project", "p"},
				{"unbind", "stray"},
				{"binding", "--json"},
				{"binding", "stray"},
			} {
				w.run("a command line the verb does not understand", repo, args...)
			}
		}},
		{"verbs-report-a-store-that-cannot-be-used", func(w *repoWorld) {
			repo := w.plainRepo("<REPO>", "<KEY>")
			w.workflowIn(repo, "proj-1", "wf-1", "running")
			if err := os.MkdirAll(filepath.Join(w.state, "labdrian"), 0o700); err != nil {
				w.t.Fatal(err)
			}
			writeFixtureFile(w.t, filepath.Join(w.state, "labdrian", "bindings"), "a file where the store should be")
			w.run("bind", repo, "bind", "--project", "proj-1", "--workflow", "wf-1")
			w.run("unbind", repo, "unbind")
			w.run("binding", repo, "binding")
		}},
	}
}
