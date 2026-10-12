package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The golden files under testdata/stale-golden pin what `longterm-mem stale` prints, with its exit code, for the
// paths a memory names and what the repository says of them: a path removed after the memory was written, one
// removed before (or at the instant) and so recorded by it, one renamed, one the repository never held, one
// recorded under another root of the tree, several memories and several paths in one, the layouts of the time a
// memory carries and the memory whose time cannot be read, the shapes of a path that the pattern admits and the
// history refuses, and the directories that are no repository, have no commit or cannot be walked. They were
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
	w.git(t, "init", "-q", "-b", "main")
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
			w.git(t, "init", "-q", "-b", "main")
			w.writeProjectFile(t, "kept.go", "package golden\n")
			w.memory(t, "names a path the tree does not hold", where("gone.go"), beforeTheCommits, beforeTheCommits)
		}},
		{name: "18-a-repository-with-no-commit-and-every-path-in-the-tree", setup: func(t *testing.T, w *skillsStaleWorld) {
			w.git(t, "init", "-q", "-b", "main")
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
	}
}

// gitWords are the words git itself uses for what it refuses, which are its own to change: the goldens pin the
// sentence the program builds around them and not these.
var gitWords = regexp.MustCompile(`(exit status 128: )[^\n]*`)

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
	rendering = fmt.Sprintf("args: %s\nexit: %d\n--- stdout ---\n%s--- stderr ---\n%s", strings.Join(args, " "), code, stdout, stderr)
	return w, gitWords.ReplaceAllString(w.maskForSkillsStale(rendering), "${1}<git's words>")
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
