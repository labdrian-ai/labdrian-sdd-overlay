package main

import (
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The golden files under testdata/stale-golden pin what `longterm-mem stale` prints, with its exit code, for the
// paths a memory names and what the repository says of them: a path removed after the memory was written, one
// removed before (or at the instant) and so recorded by it, one renamed, one the repository never held, one
// recorded under another root of the tree, several memories and several paths in one, the layouts of the time a
// memory carries and the memory whose time cannot be read, the shapes of a path that the pattern admits (a path
// that climbs out of the project is unknown and the rest of the report prints: the owner's decision of
// 2026-10-13, which re-recorded goldens 13 here and 33 of skills-stale), and the directories that are no repository, have no commit or cannot be walked. They were
// recorded from the program as it stood before the detector took the repository's evidence (the working tree and
// its history) as a port of its own (Phase 9, L6), and that change must not alter one byte of them.
//
// Each world is the one of the skills-stale goldens (a project root, a PATH of the world's own with git behind a
// file that runs it, commits made with a fixed identity and a fixed date), and the command runs in the project
// root as it does for its users. Where the words of git itself are part of the output, they are replaced.
//
// To rewrite them after a change that is MEANT to alter the output:
//
//	go test ./cmd/longterm-mem -run TestStaleGolden -update-retrieval-golden
//
// and read the diff before committing it.
const (
	// beforeTheCommits, atTheCommits and afterTheCommits are the times a memory is written at, in the layout
	// Engram uses, around the date every commit of the worlds carries (2026-01-01T00:00:00Z).
	beforeTheCommits = "2025-12-31 00:00:00"
	atTheCommits     = "2026-01-01 00:00:00"
	afterTheCommits  = "2027-01-01 00:00:00"
)

type staleCase struct {
	name  string
	setup func(t *testing.T, w *skillsStaleWorld)
	// cwd is the directory the command runs in when it is not the project root.
	cwd func(w *skillsStaleWorld) string
	// noListing leaves the project's listing out of the comparison, for a world whose tree cannot be walked.
	noListing bool
}

// memory records an observation of the project in the Engram database of the world.
func (w *skillsStaleWorld) memory(t *testing.T, title, content, createdAt, updatedAt string) {
	t.Helper()
	db, err := sql.Open("sqlite", w.dbPath)
	if err != nil {
		t.Fatalf("open the fixture database: %v", err)
	}
	defer db.Close()
	_, err = db.Exec(`INSERT INTO observations (session_id, type, title, content, project, topic_key, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, "sess-1", "decision", title, content, skillsStaleProject, "stale/"+title, createdAt, updatedAt)
	if err != nil {
		t.Fatalf("record the memory %q: %v", title, err)
	}
}

// where is the content of a memory that names the paths, in the layout Engram's save format gives it.
func where(paths ...string) string {
	return "**What**: a thing\n**Where**: " + strings.Join(paths, ", ") + "\n**Learned**: nothing\n"
}

// history makes the project a repository whose history has the shapes the report tells apart: kept.go is in the
// tree, gone.go was removed and moved.go renamed to moved-new.go (each by a commit of its own), and the tree holds
// module/promote/writer.go, a path a memory may record without its module.
func (w *skillsStaleWorld) history(t *testing.T) {
	t.Helper()
	w.initRepo(t)
	w.writeProjectFile(t, "kept.go", "package golden\n")
	w.writeProjectFile(t, "gone.go", "package golden\n// distinctive body of the removed file\n")
	w.writeProjectFile(t, "moved.go", "package golden\n// distinctive body of the moved file\n")
	w.writeProjectFile(t, "module/promote/writer.go", "package promote\n")
	w.git(t, "add", "-A")
	w.git(t, "commit", "-q", "-m", "initial")
	w.git(t, "rm", "-q", "gone.go")
	w.git(t, "commit", "-q", "-m", "remove a recorded path")
	w.git(t, "mv", "moved.go", "moved-new.go")
	w.git(t, "commit", "-q", "-m", "rename a recorded path")
}

// foreignConfig lists the configuration entries git sees that are not the repository's own, and counts all the
// entries it listed. It asks for the scope of each entry (git 2.26 and later); a git that cannot is asked for
// the origin of each, which it names relative to where it ran or absolute, so the origin is resolved to a file
// and compared with the repository's own config file.
func (w *skillsStaleWorld) foreignConfig(t *testing.T) (foreign []string, entries int) {
	t.Helper()
	if out, err := w.tryGitOutput("config", "--list", "--show-scope"); err == nil {
		return nonLocalScopes(out)
	}
	return w.foreignConfigByOrigin(t)
}

// foreignConfigByOrigin is the way foreignConfig asks when git cannot say the scope of an entry. It is a method
// of its own so that a test can run it against the git of the host, which can say the scope.
func (w *skillsStaleWorld) foreignConfigByOrigin(t *testing.T) (foreign []string, entries int) {
	t.Helper()
	return foreignOrigins(w.gitOutput(t, "config", "--list", "--show-origin"), w.root)
}

// nonLocalScopes are the lines of `git config --list --show-scope` ("<scope><TAB><key>=<value>") whose scope is
// not the repository's, and the number of lines.
func nonLocalScopes(listing string) (foreign []string, entries int) {
	for _, line := range strings.Split(listing, "\n") {
		if line == "" {
			continue
		}
		entries++
		if !strings.HasPrefix(line, "local\t") {
			foreign = append(foreign, line)
		}
	}
	return foreign, entries
}

// foreignOrigins are the lines of `git config --list --show-origin` ("<origin><TAB><key>=<value>") whose origin
// is not the repository's own config file, wherever git says that file is: a path relative to root or absolute,
// through links or not; and the number of lines.
func foreignOrigins(listing, root string) (foreign []string, entries int) {
	// A repository whose own config file cannot be resolved (it is not there) has no file an entry could come from,
	// so own stays empty and nothing matches it: every entry is foreign. (A fallback to the unresolved path would
	// match nothing either, because an entry's path is resolved the same way and fails the same way.)
	own, _ := filepath.EvalSymlinks(filepath.Join(root, ".git", "config"))
	for _, line := range strings.Split(listing, "\n") {
		if line == "" {
			continue
		}
		entries++
		origin, _, _ := strings.Cut(line, "\t")
		if path, isFile := strings.CutPrefix(origin, "file:"); isFile {
			path = unquoteOrigin(path)
			if !filepath.IsAbs(path) {
				path = filepath.Join(root, path)
			}
			if resolved, err := filepath.EvalSymlinks(path); err == nil && resolved == own {
				continue
			}
		}
		foreign = append(foreign, line)
	}
	return foreign, entries
}

// unquoteOrigin is the path of an origin as git printed it: git puts a path that has a quote, a backslash or a
// control character in it between double quotes, with a backslash before each such character, and prints any
// other path as it is. A quoted path that cannot be read back is returned as printed, so that it is foreign.
func unquoteOrigin(path string) string {
	if !strings.HasPrefix(path, `"`) {
		return path
	}
	if unquoted, err := strconv.Unquote(path); err == nil {
		return unquoted
	}
	return path
}

// The goldens print the ids of commits (twelve characters of them). An id is the SHA-1 of the commit's own text,
// so it is the same on every machine exactly when that text is: the tree (the files this history writes, in the
// default object format), the parent, the identity and the date of author and committer, and the message, with
// nothing added by a signature, an encoding header, a hook that rewrites the message or a configuration of the
// machine. This test does not look at an id: it checks that the world gives none of those a way in, from the
// text of the commit and the configuration git sees, and it names the cause when one does. (It skips with the
// rest when there is no git: newSkillsStaleWorld does.)
func TestStaleWorld_PinsEverythingACommitIdIsMadeOf(t *testing.T) {
	w := newSkillsStaleWorld(t, true)
	w.history(t)

	// No configuration but the repository's own: no system, no global, no include, no command line. The
	// repository always has entries of its own (git init writes them), so an empty list means that git was not
	// asked the question the way this test thinks (or that the world is not a repository) and proves nothing.
	if foreign, entries := w.foreignConfig(t); entries == 0 {
		t.Error("git lists no configuration at all, not even the repository's own: the check below would prove nothing")
	} else {
		for _, line := range foreign {
			t.Errorf("git sees configuration that is not the repository's own: %q", line)
		}
	}
	// No hooks: a hook (from a template of the machine) could rewrite a message or refuse a commit. A
	// repository made from no template has no hooks directory at all, which is the state wanted; any other
	// failure to look is a failure.
	switch entries, err := os.ReadDir(filepath.Join(w.root, ".git", "hooks")); {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		t.Errorf("reading .git/hooks: %v", err)
	case len(entries) != 0:
		t.Errorf(".git/hooks holds %d entries: the repository was made with a template of the machine", len(entries))
	}
	// The default object format. (Git older than the question cannot be asked, and only has the one.)
	if format, err := w.tryGitOutput("rev-parse", "--show-object-format"); err == nil && strings.TrimSpace(format) != "sha1" {
		t.Errorf("object format is %q, want sha1", format)
	}
	// The text of every commit of the history, whatever their number: the fixed identity at the fixed instant,
	// and nothing else a machine could add.
	revisions := strings.Fields(w.gitOutput(t, "rev-list", "HEAD"))
	if len(revisions) == 0 {
		t.Fatal("the history has no commit")
	}
	instant, err := time.Parse(time.RFC3339, worldInstant)
	if err != nil {
		t.Fatal(err)
	}
	identity := fmt.Sprintf("golden <golden@example.com> %d +0000", instant.Unix())
	for _, revision := range revisions {
		text := w.gitOutput(t, "cat-file", "commit", revision)
		header, message, _ := strings.Cut(text, "\n\n")
		for _, line := range strings.Split(header, "\n") {
			switch {
			case strings.HasPrefix(line, "tree "), strings.HasPrefix(line, "parent "):
			case line == "author "+identity, line == "committer "+identity:
			default:
				t.Errorf("%s: the commit has a header line the world does not pin: %q", revision, line)
			}
		}
		if strings.Count(message, "\n") != 1 {
			t.Errorf("%s: the message is %q, want one line", revision, message)
		}
	}
}

// The three commits of history() have the ids the goldens name. They are here so that the goldens' ids are not
// only in the goldens: with the test above passing, a difference here is a change of the history or of the
// files it writes. To regenerate them after meaning to change either, run
//
//	go test ./cmd/longterm-mem -run TestStaleWorld_TheCommitsOfTheHistoryHaveTheIdsTheGoldensName
//
// copy the three ids from the failure messages, and rewrite the goldens with -update-retrieval-golden (the
// short ids they print are the first twelve characters).
func TestStaleWorld_TheCommitsOfTheHistoryHaveTheIdsTheGoldensName(t *testing.T) {
	w := newSkillsStaleWorld(t, true)
	w.history(t)

	for revision, want := range map[string]string{
		"HEAD~2": "f02a2770779f3b8ae337d9d69e54b42c5a1ac319",
		"HEAD~1": "7cc3e33e09a274e807dd309d3a7b27ba8ba84c22", // the removal the goldens name
		"HEAD":   "b3ad2c8c2bbca4bad9e45bc805a931dfecbd4a16", // the rename the goldens name
	} {
		if got := strings.TrimSpace(w.gitOutput(t, "rev-parse", revision)); got != want {
			t.Errorf("%s is %s, want %s", revision, got, want)
		}
	}
}

func staleCases() []staleCase {
	return []staleCase{
		{name: "01-nothing-this-repository-disagrees-with", setup: func(t *testing.T, w *skillsStaleWorld) {
			w.history(t)
			w.memory(t, "names no file", "**What**: nothing about files\n", beforeTheCommits, beforeTheCommits)
			w.memory(t, "names a file that is there", where("kept.go"), beforeTheCommits, beforeTheCommits)
		}},
		{name: "02-a-path-removed-after-the-memory-was-written", setup: func(t *testing.T, w *skillsStaleWorld) {
			w.history(t)
			w.memory(t, "how gone.go works", where("gone.go"), beforeTheCommits, beforeTheCommits)
		}},
		{name: "03-a-path-removed-before-the-memory-was-written", setup: func(t *testing.T, w *skillsStaleWorld) {
			w.history(t)
			w.memory(t, "we removed gone.go on purpose", where("gone.go"), afterTheCommits, afterTheCommits)
		}},
		{name: "04-a-path-removed-at-the-instant-the-memory-was-written", setup: func(t *testing.T, w *skillsStaleWorld) {
			w.history(t)
			w.memory(t, "written as gone.go was removed", where("gone.go"), atTheCommits, atTheCommits)
		}},
		{name: "05-a-path-renamed", setup: func(t *testing.T, w *skillsStaleWorld) {
			w.history(t)
			w.memory(t, "written before the rename", where("moved.go"), beforeTheCommits, beforeTheCommits)
			w.memory(t, "written after the rename", where("moved.go"), afterTheCommits, afterTheCommits)
		}},
		{name: "06-a-path-this-repository-never-held", setup: func(t *testing.T, w *skillsStaleWorld) {
			w.history(t)
			w.memory(t, "about another project", where("internal/cli/review_facade.go"), beforeTheCommits, beforeTheCommits)
		}},
		{name: "07-a-path-recorded-under-another-root", setup: func(t *testing.T, w *skillsStaleWorld) {
			w.history(t)
			w.memory(t, "recorded without its module", where("promote/writer.go", "writer.go", "./promote/writer.go"), beforeTheCommits, beforeTheCommits)
		}},
		{name: "08-several-memories-and-several-paths-in-order", setup: func(t *testing.T, w *skillsStaleWorld) {
			w.history(t)
			w.memory(t, "first: nothing to say", where("kept.go"), beforeTheCommits, beforeTheCommits)
			w.memory(t, "second: a removal and a move and a path there", where("kept.go", "gone.go", "moved.go", "never.go"), beforeTheCommits, beforeTheCommits)
			w.memory(t, "third: a removal recorded after it", where("gone.go"), afterTheCommits, afterTheCommits)
			w.memory(t, "fourth: a move", where("moved.go"), afterTheCommits, afterTheCommits)
			w.memory(t, "fifth: a removal", where("gone.go"), beforeTheCommits, beforeTheCommits)
		}},
		{name: "09-the-same-path-in-two-memories-and-twice-in-one", setup: func(t *testing.T, w *skillsStaleWorld) {
			w.history(t)
			w.memory(t, "names it twice", where("gone.go", "gone.go"), beforeTheCommits, beforeTheCommits)
			w.memory(t, "names it too", where("gone.go"), beforeTheCommits, beforeTheCommits)
		}},
		{name: "10-the-layouts-of-the-time-of-a-memory", setup: func(t *testing.T, w *skillsStaleWorld) {
			w.history(t)
			w.memory(t, "updated in RFC 3339", where("gone.go"), afterTheCommits, "2025-12-31T00:00:00Z")
			w.memory(t, "updated with a T and no zone", where("gone.go"), afterTheCommits, "2025-12-31T00:00:00")
			w.memory(t, "no update, created before", where("gone.go"), beforeTheCommits, "")
			w.memory(t, "no update, created after", where("gone.go"), afterTheCommits, "  ")
			w.memory(t, "updated after, created before", where("gone.go"), beforeTheCommits, afterTheCommits)
			w.memory(t, "updated in no layout, created before", where("gone.go"), beforeTheCommits, "yesterday")
		}},
		{name: "11-a-memory-whose-time-cannot-be-read", setup: func(t *testing.T, w *skillsStaleWorld) {
			w.history(t)
			w.memory(t, "undatable, a removal", where("gone.go"), "last week", "yesterday")
			w.memory(t, "undatable, a move", where("moved.go"), "last week", "yesterday")
			w.memory(t, "undatable, both", where("gone.go", "moved.go"), "", "")
		}},
		{name: "12-what-the-where-field-admits", setup: func(t *testing.T, w *skillsStaleWorld) {
			w.history(t)
			w.memory(t, "no where field", "**What**: gone.go is described here\n", beforeTheCommits, beforeTheCommits)
			w.memory(t, "punctuation and prose", "**Where**: `gone.go`, (moved.go); see https://example.test/a/b.go and #139/#138 and +11/-1 and notes.txt and gone.go.\n**Learned**: x\n", beforeTheCommits, beforeTheCommits)
			w.memory(t, "the field runs to the next one", "**Where**: kept.go\nnever.go\n**Learned**: gone.go\n", beforeTheCommits, beforeTheCommits)
		}},
		{name: "13-a-path-that-climbs-out-of-the-project", setup: func(t *testing.T, w *skillsStaleWorld) {
			w.history(t)
			w.memory(t, "names a path above the project", where("a/../../outside.go"), beforeTheCommits, beforeTheCommits)
		}},
		{name: "14-a-path-that-climbs-and-comes-back", setup: func(t *testing.T, w *skillsStaleWorld) {
			w.history(t)
			w.memory(t, "names a removed file by a detour", where("a/../gone.go"), beforeTheCommits, beforeTheCommits)
		}},
		{name: "15-a-path-that-climbs-to-a-file-beside-the-project", setup: func(t *testing.T, w *skillsStaleWorld) {
			w.history(t)
			if err := os.WriteFile(filepath.Join(w.outside, "beside.go"), []byte("package beside\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			w.memory(t, "names a file beside the project", where("a/../../"+filepath.Base(w.outside)+"/beside.go"), beforeTheCommits, beforeTheCommits)
		}},
		{name: "16-a-project-with-no-memory", setup: func(t *testing.T, w *skillsStaleWorld) { w.history(t) }},
		{name: "17-a-repository-with-no-commit", setup: func(t *testing.T, w *skillsStaleWorld) {
			w.initRepo(t)
			w.writeProjectFile(t, "kept.go", "package golden\n")
			w.memory(t, "names a path the tree does not hold", where("gone.go"), beforeTheCommits, beforeTheCommits)
		}},
		{name: "18-a-repository-with-no-commit-and-every-path-in-the-tree", setup: func(t *testing.T, w *skillsStaleWorld) {
			w.initRepo(t)
			w.writeProjectFile(t, "kept.go", "package golden\n")
			w.memory(t, "names a path the tree holds", where("kept.go"), beforeTheCommits, beforeTheCommits)
		}},
		{name: "19-a-tree-with-a-directory-that-cannot-be-read", noListing: true, setup: func(t *testing.T, w *skillsStaleWorld) {
			w.history(t)
			locked := filepath.Join(w.root, "locked")
			if err := os.Mkdir(locked, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(locked, 0); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
			if _, err := os.ReadDir(locked); err == nil {
				// A process with root's capability reads any directory, so the file system cannot be made to
				// refuse here. What the program says of a walk that fails is not left to this case alone: the
				// words are built by a function that internal/repoevidence tests with the error a refused
				// directory gives, whoever runs the tests.
				t.Skip("directory permissions are not enforced for this process (root or an equivalent capability): the tree cannot be made unreadable")
			}
			w.memory(t, "names a removed path", where("gone.go"), beforeTheCommits, beforeTheCommits)
		}},
		{name: "20-a-working-directory-that-is-no-repository", cwd: func(w *skillsStaleWorld) string { return w.outside }},
		{name: "21-a-working-directory-below-the-root-of-the-repository", cwd: func(w *skillsStaleWorld) string { return filepath.Join(w.root, "module", "promote") },
			setup: func(t *testing.T, w *skillsStaleWorld) {
				w.history(t)
				w.memory(t, "how gone.go works", where("gone.go"), beforeTheCommits, beforeTheCommits)
			}},
		{name: "22-a-path-that-climbs-out-beside-paths-the-repository-has-a-word-for", setup: func(t *testing.T, w *skillsStaleWorld) {
			w.history(t)
			w.memory(t, "names a climb, a removed file and a moved one", where("a/../../outside.go", "gone.go", "moved.go", "kept.go"), beforeTheCommits, beforeTheCommits)
			w.memory(t, "names only a climb", where("../../elsewhere.go"), beforeTheCommits, beforeTheCommits)
		}},
	}
}

// runStaleCase builds the world of the case, runs the command in it and renders what it did.
func runStaleCase(t *testing.T, tc staleCase) (w *skillsStaleWorld, rendering string) {
	t.Helper()
	w = newSkillsStaleWorld(t, true)
	w.goldenDir = filepath.Join(filepath.Dir(w.goldenDir), "stale-golden")
	if tc.setup != nil {
		tc.setup(t, w)
	}
	var before string
	if !tc.noListing {
		before = w.listing(t)
	}
	dir := w.root
	if tc.cwd != nil {
		dir = tc.cwd(w)
	}
	t.Chdir(dir)

	args := []string{"stale", "--project", skillsStaleProject}
	code, stdout, stderr := w.run(t, args)

	if !tc.noListing {
		if after := w.listing(t); after != before {
			t.Errorf("the report changed the project:\nbefore:\n%s\nafter:\n%s", before, after)
		}
	}
	return w, withGitWordsMasked(w.maskForSkillsStale(renderRun(args, code, stdout, stderr)))
}

func TestStaleGolden(t *testing.T) {
	for _, tc := range staleCases() {
		t.Run(tc.name, func(t *testing.T) {
			w, got := runStaleCase(t, tc)
			w.check(t, "stale-"+tc.name, got)
		})
	}
}

// What the command prints must not depend on where the world is: every case is run in two directories of very
// different lengths and must say the same thing in both.
func TestStaleGolden_DoesNotDependOnWhereTheWorldIs(t *testing.T) {
	base := t.TempDir()
	for _, tc := range staleCases() {
		t.Run(tc.name, func(t *testing.T) {
			answers := map[string]string{}
			for name, dir := range map[string]string{
				"short": filepath.Join(base, "s"),
				"long":  filepath.Join(base, strings.Repeat("a-long-directory-", 6)),
			} {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatalf("create the %s temporary directory: %v", name, err)
				}
				t.Run(name, func(t *testing.T) {
					t.Setenv("TMPDIR", dir)
					_, answers[name] = runStaleCase(t, tc)
				})
			}
			if answers["short"] != answers["long"] {
				t.Errorf("the rendering depends on the length of the temporary directory the world is in:\n--- short ---\n%s\n--- long ---\n%s", answers["short"], answers["long"])
			}
		})
	}
}
