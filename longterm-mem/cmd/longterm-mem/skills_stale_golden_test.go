package main

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// The golden files under testdata/skills-stale-golden pin what `longterm-mem skills-stale` prints, with its exit
// code, in the worlds that matter to the report: a skill that is fresh, one nobody has observed for half a year,
// one whose command is not on PATH, a lock that is missing, corrupt or refused, a skill file that is missing or
// that the lock points out of the project, a repository where a path a skill names was removed or moved, and the
// refusals of the command's own flags. They were recorded from the program as it stood before the detector took
// the lock and the skill files, the repository's evidence, the commands on PATH and the time as inputs (Phase 9,
// L5), and that change must not alter one byte of them.
//
// Each world is deterministic. The project root, the home and the Engram database are the world's own temporary
// directories, replaced by name; the commands a skill names are resolved against a PATH that is a directory of
// the test's own (and, where the repository's history is read, a file that runs git); the repository's commits are
// made with a fixed identity and a fixed date, so their ids are the same on every machine; and no case depends on
// the day it runs: what is "observed long ago" is the year 2000 and what is "observed recently" is the year 2099.
// The recording runs under two lengths of TMPDIR (TestSkillsStaleGolden_DoesNotDependOnWhereTheWorldIs).
//
// To rewrite them after a change that is MEANT to alter the output:
//
//	go test ./cmd/longterm-mem -run TestSkillsStaleGolden -update-retrieval-golden
//
// and read the diff before committing it.
const (
	skillsStaleProject = "skillsgolden"
	// longAgo is a LastObserved older than any clock the program can read; recently is newer than the year 2099.
	longAgo  = "2000-01-01T00:00:00Z"
	recently = "2099-01-01T00:00:00Z"
)

// skillsStaleWorld is the world of one case: the retrieval world (HOME, Engram database, vault, state directory)
// with a project root of its own and the directory that is its PATH.
type skillsStaleWorld struct {
	*retrievalWorld
	root   string
	binDir string
	// outside is a directory beside the project, for a link that leaves it.
	outside string
}

type skillsStaleCase struct {
	name  string
	setup func(t *testing.T, w *skillsStaleWorld)
	// args are the command's arguments when the case does not run the usual ones.
	args func(w *skillsStaleWorld) []string
	// pathNeedsGit puts a file that runs git in the directory that is PATH, for the cases that read the repository's history.
	pathNeedsGit bool
	// maskGitWords replaces the words git itself uses for what it refuses, which are its own to change, in a case
	// whose report holds them. (The cases recorded before them keep git's words as they were written.)
	maskGitWords bool
}

// writeProjectFile writes content at rel in the project root, creating its directory.
func (w *skillsStaleWorld) writeProjectFile(t *testing.T, rel, content string) {
	t.Helper()
	w.writeFile(t, filepath.Join(w.root, filepath.FromSlash(rel)), content)
}

// skillFiles writes the skill file of a skill at both of the targets a lock records for it.
func (w *skillsStaleWorld) skillFiles(t *testing.T, id, text string) {
	t.Helper()
	for _, dir := range []string{".claude", ".agents"} {
		w.writeProjectFile(t, dir+"/skills/"+id+"/SKILL.md", text)
	}
}

func lockEntry(id string) string {
	return fmt.Sprintf(`{"id": %q, "provenance": "procedural", "candidate": "procedural/candidates/repeated-success/%s", "sha256": %q, "revision": 1, "targets": [".claude/skills/%s/SKILL.md", ".agents/skills/%s/SKILL.md"]}`,
		id, id, strings.Repeat("a", 64), id, id)
}

// lockOf is a project lock that lists the skills, in the order given.
func lockOf(ids ...string) string {
	entries := make([]string, len(ids))
	for i, id := range ids {
		entries[i] = lockEntry(id)
	}
	return `{"version": 1, "skills": [` + strings.Join(entries, ", ") + "]}\n"
}

func (w *skillsStaleWorld) lock(t *testing.T, content string) {
	t.Helper()
	w.writeProjectFile(t, ".labdrian/procedural-skills.lock.json", content)
}

// observed records the candidate of the skill in the Engram database of the world, last observed at the given
// instant (empty: the record says nothing of it).
func (w *skillsStaleWorld) observed(t *testing.T, id, lastObserved string) {
	t.Helper()
	content := "**Status**: registered\n"
	if lastObserved != "" {
		content += "**LastObserved**: " + lastObserved + "\n"
	}
	db, err := sql.Open("sqlite", w.dbPath)
	if err != nil {
		t.Fatalf("open the fixture database: %v", err)
	}
	defer db.Close()
	_, err = db.Exec(`INSERT INTO observations (session_id, type, title, content, project, topic_key, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, "sess-1", "pattern", id, content, skillsStaleProject,
		"procedural/candidates/repeated-success/"+id, "2025-12-31 00:00:00", "2025-12-31 00:00:00")
	if err != nil {
		t.Fatalf("record the candidate of %q: %v", id, err)
	}
}

func skillText(body string) string {
	return "---\nname: golden\ndescription: \"Trigger: golden.\"\n---\n" + body
}

const fence = "```"

// standIn writes an executable file named name in the directory that is the PATH of the world.
func (w *skillsStaleWorld) standIn(t *testing.T, name string) {
	t.Helper()
	path := filepath.Join(w.binDir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write the stand-in for %s: %v", name, err)
	}
}

// git runs git in the project root with an identity and a date of the test's own, so that the commits it makes
// have the same ids on every machine.
func (w *skillsStaleWorld) git(t *testing.T, args ...string) {
	t.Helper()
	w.gitOutput(t, args...)
}

// gitOutput is git as git() runs it, and what it printed.
func (w *skillsStaleWorld) gitOutput(t *testing.T, args ...string) string {
	t.Helper()
	out, err := w.tryGitOutput(args...)
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

// worldInstant is the instant every commit of every world is made at (author and committer), in the layout git
// reads from its environment.
const worldInstant = "2026-01-01T00:00:00Z"

// tryGitOutput runs git in the project root in an environment that holds nothing but what an id is made of: no
// configuration of the system or of the user, the identity and the instant of author and committer, and the
// world's own PATH and HOME. Its output and its error are returned.
//
// The system and the user's configuration are each shut out in two ways, because no one way holds for every
// version of git. GIT_CONFIG_NOSYSTEM is honoured by all of them for the system file; GIT_CONFIG_SYSTEM and
// GIT_CONFIG_GLOBAL, which name a file to read instead (here, none), are honoured from git 2.32 on, and an older
// git ignores them and finds the user's file under HOME, which is the world's own empty directory. Drop one
// and a version of git reads the machine's configuration.
func (w *skillsStaleWorld) tryGitOutput(args ...string) (string, error) {
	return w.tryGitOutputIn(w.root, args...)
}

// tryGitOutputIn is tryGitOutput run in dir instead of the project root.
func (w *skillsStaleWorld) tryGitOutputIn(dir string, args ...string) (string, error) {
	command := exec.Command("git", args...)
	command.Dir = dir
	command.Env = []string{
		"PATH=" + os.Getenv("PATH"), "HOME=" + w.home,
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=golden", "GIT_AUTHOR_EMAIL=golden@example.com", "GIT_AUTHOR_DATE=" + worldInstant,
		"GIT_COMMITTER_NAME=golden", "GIT_COMMITTER_EMAIL=golden@example.com", "GIT_COMMITTER_DATE=" + worldInstant,
	}
	out, err := command.CombinedOutput()
	return string(out), err
}

// initRepo makes the project a repository on main, from no template: a template of the machine could put hooks
// in it, and a hook can rewrite a commit's message.
func (w *skillsStaleWorld) initRepo(t *testing.T) {
	t.Helper()
	w.git(t, "init", "-q", "--template=", "-b", "main")
}

// lockedTarget is a case whose lock records one skill with the one target given.
func lockedTarget(name, target string) skillsStaleCase {
	return skillsStaleCase{name: name, setup: func(t *testing.T, w *skillsStaleWorld) {
		w.lock(t, `{"version": 1, "skills": [{"id": "pointed", "provenance": "procedural", "candidate": "c", "sha256": "`+strings.Repeat("a", 64)+`", "revision": 1, "targets": [`+fmt.Sprintf("%q", target)+`]}]}`+"\n")
	}}
}

func skillsStaleCases() []skillsStaleCase {
	return []skillsStaleCase{
		{name: "01-a-fresh-skill", setup: func(t *testing.T, w *skillsStaleWorld) {
			w.standIn(t, "present-command")
			w.writeProjectFile(t, "present.go", "package golden\n")
			w.lock(t, lockOf("fresh-skill"))
			w.skillFiles(t, "fresh-skill", skillText("Use `present.go`.\n"+fence+"sh\npresent-command present.go\n"+fence+"\n"))
			w.observed(t, "fresh-skill", recently)
		}},
		{name: "02-a-skill-nobody-observed-for-half-a-year", setup: func(t *testing.T, w *skillsStaleWorld) {
			w.lock(t, lockOf("quiet-skill"))
			w.skillFiles(t, "quiet-skill", skillText("Nothing to run.\n"))
			w.observed(t, "quiet-skill", longAgo)
		}},
		{name: "03-a-skill-whose-record-says-nothing-of-when", setup: func(t *testing.T, w *skillsStaleWorld) {
			w.lock(t, lockOf("undated-skill"))
			w.skillFiles(t, "undated-skill", skillText("Nothing to run.\n"))
			w.observed(t, "undated-skill", "")
		}},
		{name: "04-a-skill-no-observation-names", setup: func(t *testing.T, w *skillsStaleWorld) {
			w.lock(t, lockOf("unrecorded-skill"))
			w.skillFiles(t, "unrecorded-skill", skillText(fence+"sh\ndefinitely-missing-command run\n"+fence+"\n"))
		}},
		{name: "05-a-command-missing-from-path", setup: func(t *testing.T, w *skillsStaleWorld) {
			w.standIn(t, "present-command")
			w.lock(t, lockOf("command-skill"))
			w.skillFiles(t, "command-skill", skillText(fence+"sh\ndefinitely-missing-command one\n# a comment\n$ present-command two\n  definitely-missing-command again\n"+fence+"\n"))
			w.observed(t, "command-skill", recently)
		}},
		{name: "06-the-shapes-a-skill-file-can-have", setup: func(t *testing.T, w *skillsStaleWorld) {
			w.lock(t, lockOf("a-no-front-matter", "b-unclosed-front-matter", "c-bom-and-longer-fence", "d-two-blocks", "e-nothing-fenced"))
			w.skillFiles(t, "a-no-front-matter", fence+"sh\nmissing-one `ticks`\n"+fence+"\n")
			w.skillFiles(t, "b-unclosed-front-matter", "---\nname: b\n"+fence+"sh\nmissing-two\n"+fence+"\n")
			w.skillFiles(t, "c-bom-and-longer-fence", "\xef\xbb\xbf"+skillText("````sh\n`missing-three`; next\n````\n"))
			w.skillFiles(t, "d-two-blocks", skillText(fence+"\nmissing-four a\n"+fence+"\ntext between\n"+fence+"sh\nmissing-four b\nmissing-five;\n"+fence+"\n"))
			w.skillFiles(t, "e-nothing-fenced", skillText("missing-six is only prose here.\n"))
			for _, id := range []string{"a-no-front-matter", "b-unclosed-front-matter", "c-bom-and-longer-fence", "d-two-blocks", "e-nothing-fenced"} {
				w.observed(t, id, recently)
			}
		}},
		{name: "07-a-quiet-skill-and-a-missing-command-and-a-clean-one-in-lock-order", setup: func(t *testing.T, w *skillsStaleWorld) {
			w.standIn(t, "present-command")
			w.writeProjectFile(t, "present.go", "package golden\n")
			w.lock(t, lockOf("zeta-quiet", "alpha-clean", "mid-both"))
			w.skillFiles(t, "zeta-quiet", skillText("Use `present.go`.\n"))
			w.skillFiles(t, "alpha-clean", skillText("Use `present.go`.\n"+fence+"sh\npresent-command\n"+fence+"\n"))
			w.skillFiles(t, "mid-both", skillText(fence+"sh\ndefinitely-missing-command\n"+fence+"\n"))
			w.observed(t, "zeta-quiet", longAgo)
			w.observed(t, "alpha-clean", recently)
			w.observed(t, "mid-both", longAgo)
		}},
		{name: "08-a-path-removed-and-a-path-moved-in-a-repository", pathNeedsGit: true, setup: func(t *testing.T, w *skillsStaleWorld) {
			w.initRepo(t)
			w.writeProjectFile(t, "removed.go", "package golden\n")
			w.writeProjectFile(t, "moved.go", "package golden\n// distinctive moved body\n")
			w.writeProjectFile(t, "present.go", "package golden\n")
			w.lock(t, lockOf("history-skill", "present-skill"))
			w.skillFiles(t, "history-skill", skillText("See `removed.go`, `moved.go`, `present.go` and `never-existed.go`.\n"))
			w.skillFiles(t, "present-skill", skillText("See `present.go`.\n"))
			w.git(t, "add", "-A")
			w.git(t, "commit", "-q", "-m", "initial")
			w.git(t, "rm", "-q", "removed.go")
			w.git(t, "commit", "-q", "-m", "remove a referenced path")
			w.git(t, "mv", "moved.go", "moved-new.go")
			w.git(t, "commit", "-q", "-m", "rename a referenced path")
			w.observed(t, "history-skill", recently)
			w.observed(t, "present-skill", recently)
		}},
		{name: "09-a-path-missing-from-a-directory-that-is-no-repository", pathNeedsGit: true, setup: func(t *testing.T, w *skillsStaleWorld) {
			w.lock(t, lockOf("lost-skill"))
			w.skillFiles(t, "lost-skill", skillText("See `lost.go`.\n"))
			w.observed(t, "lost-skill", recently)
		}},
		{name: "10-no-lock", setup: func(t *testing.T, w *skillsStaleWorld) {}},
		{name: "11-a-lock-that-is-not-json", setup: func(t *testing.T, w *skillsStaleWorld) { w.lock(t, "{not json") }},
		{name: "12-an-empty-lock-file", setup: func(t *testing.T, w *skillsStaleWorld) { w.lock(t, "") }},
		{name: "13-a-lock-of-another-version", setup: func(t *testing.T, w *skillsStaleWorld) { w.lock(t, `{"version": 2, "skills": []}`+"\n") }},
		{name: "14-a-lock-with-a-field-nobody-knows", setup: func(t *testing.T, w *skillsStaleWorld) {
			w.lock(t, `{"version": 1, "skills": [], "note": "x"}`+"\n")
		}},
		{name: "15-a-lock-with-a-value-after-the-lock", setup: func(t *testing.T, w *skillsStaleWorld) {
			w.lock(t, `{"version": 1, "skills": []} {}`+"\n")
		}},
		{name: "16-a-lock-that-lists-a-skill-twice", setup: func(t *testing.T, w *skillsStaleWorld) { w.lock(t, lockOf("twice", "twice")) }},
		{name: "17-a-lock-that-is-a-directory", setup: func(t *testing.T, w *skillsStaleWorld) {
			if err := os.MkdirAll(filepath.Join(w.root, ".labdrian", "procedural-skills.lock.json"), 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "18-a-lock-with-install-records-and-no-skills", setup: func(t *testing.T, w *skillsStaleWorld) {
			w.lock(t, `{"version": 1, "skills": [], "installs": [{"id": "pdf", "files": [{"path": "SKILL.md", "sha256": "`+strings.Repeat("b", 64)+`"}]}]}`+"\n")
		}},
		{name: "19-a-lock-with-no-skills", setup: func(t *testing.T, w *skillsStaleWorld) { w.lock(t, `{"version": 1, "skills": []}`+"\n") }},
		{name: "20-a-skill-file-that-is-missing", setup: func(t *testing.T, w *skillsStaleWorld) {
			w.lock(t, lockOf("first-skill", "gone-skill"))
			w.skillFiles(t, "first-skill", skillText("Nothing.\n"))
			w.observed(t, "first-skill", longAgo)
		}},
		{name: "21-a-lock-entry-without-targets", setup: func(t *testing.T, w *skillsStaleWorld) {
			w.lock(t, `{"version": 1, "skills": [{"id": "bare", "provenance": "procedural", "candidate": "c", "sha256": "`+strings.Repeat("a", 64)+`", "revision": 1, "targets": []}]}`+"\n")
		}},
		lockedTarget("22a-a-target-that-climbs-out-of-the-project", "../outside/SKILL.md"),
		lockedTarget("22b-a-target-that-climbs-out-from-inside", "a/../../outside/SKILL.md"),
		lockedTarget("22c-a-target-that-is-absolute", "/etc/hosts"),
		lockedTarget("22d-a-target-that-is-empty", ""),
		{name: "23-a-skill-file-that-is-a-link-out-of-the-project", setup: func(t *testing.T, w *skillsStaleWorld) {
			if err := os.WriteFile(filepath.Join(w.outside, "SKILL.md"), []byte(skillText("Outside.\n")), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(w.root, ".claude", "skills", "linked"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(w.outside, "SKILL.md"), filepath.Join(w.root, ".claude", "skills", "linked", "SKILL.md")); err != nil {
				t.Skipf("cannot make a link here: %v", err)
			}
			w.lock(t, lockOf("linked"))
		}},
		{name: "24-a-skill-file-that-is-a-link-inside-the-project", setup: func(t *testing.T, w *skillsStaleWorld) {
			w.writeProjectFile(t, "shared/SKILL.md", skillText(fence+"sh\ndefinitely-missing-command\n"+fence+"\n"))
			if err := os.MkdirAll(filepath.Join(w.root, ".claude", "skills", "linked"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(w.root, "shared", "SKILL.md"), filepath.Join(w.root, ".claude", "skills", "linked", "SKILL.md")); err != nil {
				t.Skipf("cannot make a link here: %v", err)
			}
			w.lock(t, lockOf("linked"))
			w.observed(t, "linked", recently)
		}},
		{name: "25-an-engram-database-that-is-not-there", setup: func(t *testing.T, w *skillsStaleWorld) {
			w.lock(t, lockOf("any-skill"))
			t.Setenv(engramDBEnvVar, filepath.Join(w.home, "absent", "engram.db"))
		}},
		{name: "26-a-project-root-that-is-relative", args: func(w *skillsStaleWorld) []string {
			return []string{"skills-stale", "--project-root", "relative/root", "--project", skillsStaleProject}
		}},
		{name: "27-a-project-root-that-does-not-exist", args: func(w *skillsStaleWorld) []string {
			return []string{"skills-stale", "--project-root", filepath.Join(w.root, "absent"), "--project", skillsStaleProject}
		}},
		{name: "28-a-project-root-that-is-a-file", setup: func(t *testing.T, w *skillsStaleWorld) { w.writeProjectFile(t, "a-file", "x") },
			args: func(w *skillsStaleWorld) []string {
				return []string{"skills-stale", "--project-root", filepath.Join(w.root, "a-file"), "--project", skillsStaleProject}
			}},
		{name: "29-no-project-root", args: func(w *skillsStaleWorld) []string { return []string{"skills-stale", "--project", skillsStaleProject} }},
		{name: "30-a-project-nobody-names-in-a-directory-that-is-no-repository", args: func(w *skillsStaleWorld) []string {
			return []string{"skills-stale", "--project-root", w.root}
		}},
		{name: "31-a-flag-nobody-knows", args: func(w *skillsStaleWorld) []string { return []string{"skills-stale", "--nope"} }},
		{name: "32-a-path-recorded-under-another-root-of-the-tree", pathNeedsGit: true, setup: func(t *testing.T, w *skillsStaleWorld) {
			w.initRepo(t)
			w.writeProjectFile(t, "module/promote/writer.go", "package promote\n")
			w.lock(t, lockOf("suffix-skill"))
			w.skillFiles(t, "suffix-skill", skillText("See `promote/writer.go`, `writer.go` and `module/promote/writer.go`.\n"))
			w.git(t, "add", "-A")
			w.git(t, "commit", "-q", "-m", "initial")
			w.observed(t, "suffix-skill", recently)
		}},
		{name: "33-a-path-that-climbs-out-of-the-project-in-a-repository", pathNeedsGit: true, maskGitWords: true, setup: func(t *testing.T, w *skillsStaleWorld) {
			w.initRepo(t)
			w.writeProjectFile(t, "present.go", "package golden\n")
			w.lock(t, lockOf("climbing-skill"))
			w.skillFiles(t, "climbing-skill", skillText("See `present.go` and `a/../../outside.go`.\n"))
			w.git(t, "add", "-A")
			w.git(t, "commit", "-q", "-m", "initial")
			w.observed(t, "climbing-skill", recently)
		}},
		{name: "34-a-path-that-climbs-and-comes-back-in-a-repository", pathNeedsGit: true, setup: func(t *testing.T, w *skillsStaleWorld) {
			w.initRepo(t)
			w.writeProjectFile(t, "removed.go", "package golden\n")
			w.lock(t, lockOf("detour-skill"))
			w.skillFiles(t, "detour-skill", skillText("See `a/../removed.go`.\n"))
			w.git(t, "add", "-A")
			w.git(t, "commit", "-q", "-m", "initial")
			w.git(t, "rm", "-q", "removed.go")
			w.git(t, "commit", "-q", "-m", "remove a recorded path")
			w.observed(t, "detour-skill", recently)
		}},
		{name: "35-a-repository-with-no-commit", pathNeedsGit: true, maskGitWords: true, setup: func(t *testing.T, w *skillsStaleWorld) {
			w.initRepo(t)
			w.lock(t, lockOf("early-skill"))
			w.skillFiles(t, "early-skill", skillText("See `lost.go`.\n"))
			w.observed(t, "early-skill", recently)
		}},
		{name: "36-a-removed-path-two-skills-name", pathNeedsGit: true, setup: func(t *testing.T, w *skillsStaleWorld) {
			w.initRepo(t)
			w.writeProjectFile(t, "removed.go", "package golden\n")
			w.lock(t, lockOf("first-skill", "second-skill"))
			w.skillFiles(t, "first-skill", skillText("See `removed.go` and `removed.go` again.\n"))
			w.skillFiles(t, "second-skill", skillText("See `removed.go`.\n"))
			w.git(t, "add", "-A")
			w.git(t, "commit", "-q", "-m", "initial")
			w.git(t, "rm", "-q", "removed.go")
			w.git(t, "commit", "-q", "-m", "remove a recorded path")
			w.observed(t, "first-skill", recently)
			w.observed(t, "second-skill", recently)
		}},
		{name: "37-paths-the-tree-holds-in-a-directory-that-is-no-repository", pathNeedsGit: true, setup: func(t *testing.T, w *skillsStaleWorld) {
			w.writeProjectFile(t, "docs/guide.md", "# guide\n")
			w.lock(t, lockOf("plain-skill"))
			w.skillFiles(t, "plain-skill", skillText("See `docs/guide.md` and `guide.md`.\n"))
			w.observed(t, "plain-skill", recently)
		}},
		{name: "38-a-path-that-climbs-out-beside-a-removed-one", pathNeedsGit: true, setup: func(t *testing.T, w *skillsStaleWorld) {
			w.initRepo(t)
			w.writeProjectFile(t, "removed.go", "package golden\n")
			w.lock(t, lockOf("climbing-skill"))
			w.skillFiles(t, "climbing-skill", skillText("See `a/../../outside.go`, `removed.go` and `../../elsewhere.go`.\n"))
			w.git(t, "add", "-A")
			w.git(t, "commit", "-q", "-m", "initial")
			w.git(t, "rm", "-q", "removed.go")
			w.git(t, "commit", "-q", "-m", "remove a recorded path")
			w.observed(t, "climbing-skill", recently)
		}},
	}
}

// libraryJSONWords are the words the JSON parser of the toolchain uses for text that is not JSON: they are the
// toolchain's to change, so the goldens pin the sentence the program builds around them and not these.
var libraryJSONWords = regexp.MustCompile(`(parse project lock: )(invalid character [^\n]*|unexpected end of JSON input|unexpected EOF)`)

// newSkillsStaleWorld builds a world with a project root of its own and a PATH that is a directory of its own
// (and, for the cases that read the history of a repository, a file that runs git): the commands a skill names
// are found in that PATH or nowhere.
func newSkillsStaleWorld(t *testing.T, pathNeedsGit bool) *skillsStaleWorld {
	t.Helper()
	base := newRetrievalWorld(t)
	w := &skillsStaleWorld{retrievalWorld: base, root: realTempDir(t), binDir: realTempDir(t), outside: realTempDir(t)}
	w.goldenDir = filepath.Join(filepath.Dir(base.goldenDir), "skills-stale-golden")
	if pathNeedsGit {
		gitPath, err := exec.LookPath("git")
		if err != nil {
			t.Skip("git is not on PATH: this case reads the history of a repository")
		}
		w.lendGit(t, gitPath)
	}
	t.Setenv("PATH", w.binDir)
	return w
}

// lendGit puts a file named git in the directory that is the PATH of the world, which runs the git at gitPath.
// The directory git lives in is not put on the PATH instead: it holds whatever else the host keeps beside git,
// and a command a case treats as missing could be found there. (A file that runs git, not a link to it, because
// a git built to find its own files by where it was started from finds them from the real one.)
//
// The file is a POSIX shell script, so a host with no /bin/sh cannot run these cases and the test says so and
// skips them. A host that has one and a file that still does not run git is a fault of this harness, not of the
// program, and fails with that said, instead of surfacing as a golden that differs.
func (w *skillsStaleWorld) lendGit(t *testing.T, gitPath string) {
	t.Helper()
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skipf("the file that runs git is a shell script and there is no /bin/sh here: %v", err)
	}
	wrapper := filepath.Join(w.binDir, "git")
	script := "#!/bin/sh\nexec '" + strings.ReplaceAll(gitPath, "'", `'\''`) + "' \"$@\"\n"
	if err := os.WriteFile(wrapper, []byte(script), 0o755); err != nil {
		t.Fatalf("write the file that runs git: %v", err)
	}
	if err := wrapperRunsGit(wrapper); err != nil {
		t.Fatalf("the test harness's file that runs git does not run it (a fault of the harness, not of the program): %v", err)
	}
}

// wrapperRunsGit runs the file at path as `git --version` and says whether it answered as git does.
func wrapperRunsGit(path string) error {
	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		return fmt.Errorf("%s --version: %w", path, err)
	}
	if !strings.HasPrefix(string(out), "git version ") {
		return fmt.Errorf("%s --version printed %q, want git's version", path, out)
	}
	return nil
}

// The commands a skill names are found in the world's directory or nowhere, in the cases that read a repository's
// history as in the others: the directory git lives in (which holds whatever else the host keeps beside it) is
// never on the world's PATH, and git is reached through a file of the world's own that runs it.
func TestSkillsStaleWorld_PathIsTheWorldsOwnDirectoryAlone(t *testing.T) {
	for _, needsGit := range []bool{false, true} {
		t.Run(fmt.Sprintf("git=%v", needsGit), func(t *testing.T) {
			w := newSkillsStaleWorld(t, needsGit)

			if got := os.Getenv("PATH"); got != w.binDir {
				t.Errorf("PATH = %q, want the world's directory %q alone", got, w.binDir)
			}
			entries, err := os.ReadDir(w.binDir)
			if err != nil {
				t.Fatal(err)
			}
			var names []string
			for _, entry := range entries {
				names = append(names, entry.Name())
			}
			var want []string
			if needsGit {
				want = []string{"git"}
			}
			if !reflect.DeepEqual(names, want) {
				t.Errorf("the world's directory holds %v, want %v", names, want)
			}
			if needsGit {
				found, err := exec.LookPath("git")
				if err != nil || found != filepath.Join(w.binDir, "git") {
					t.Errorf("git is found at %q (%v), want the file of the world's own", found, err)
				} else if err := wrapperRunsGit(found); err != nil {
					t.Errorf("the file of the world's own does not run git: %v", err)
				}
			}
			if found, err := exec.LookPath("definitely-missing-command"); err == nil {
				t.Errorf("a command nobody provided is found at %q", found)
			}
		})
	}
}

// runSkillsStaleCase builds the world of the case, runs the command in it and renders what it did.
func runSkillsStaleCase(t *testing.T, tc skillsStaleCase) (w *skillsStaleWorld, rendering string) {
	t.Helper()
	w = newSkillsStaleWorld(t, tc.pathNeedsGit)
	if tc.setup != nil {
		tc.setup(t, w)
	}
	before := w.listing(t)

	args := []string{"skills-stale", "--project-root", w.root, "--project", skillsStaleProject}
	if tc.args != nil {
		args = tc.args(w)
	}
	code, stdout, stderr := w.run(t, args)

	if after := w.listing(t); after != before {
		t.Errorf("the report changed the project:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	rendering = renderRun(args, code, stdout, stderr)
	masked := w.maskForSkillsStale(rendering)
	if tc.maskGitWords {
		masked = withGitWordsMasked(masked)
	}
	return w, masked
}

// renderRun is what a golden of these worlds holds for one run of a command: its arguments, its exit code and
// both streams.
func renderRun(args []string, code int, stdout, stderr string) string {
	return fmt.Sprintf("args: %s\nexit: %d\n--- stdout ---\n%s--- stderr ---\n%s", strings.Join(args, " "), code, stdout, stderr)
}

// gitWords are the words git itself uses for what it refuses, which are its own to change: the goldens pin the
// sentence the program builds around them and not these.
var gitWords = regexp.MustCompile(`(exit status 128: )[^\n]*`)

// withGitWordsMasked replaces git's words in a rendering. The cases of skills-stale recorded before it existed
// keep git's words as they were written (the per-case flag), and every case of stale, all new, uses it.
func withGitWordsMasked(text string) string {
	return gitWords.ReplaceAllString(text, "${1}<git's words>")
}

// maskForSkillsStale replaces what differs from one machine to the next: the world's directories, and the
// words of the JSON parser and of the SQLite driver.
func (w *skillsStaleWorld) maskForSkillsStale(text string) string {
	text = strings.ReplaceAll(text, w.root, "<project>")
	text = strings.ReplaceAll(text, w.outside, "<outside>")
	text = strings.ReplaceAll(text, w.binDir, "<bin>")
	text = w.normalise(text)
	return maskLibraryErrors(libraryJSONWords.ReplaceAllString(text, "${1}<json syntax error>"), sqliteOpenError)
}

// listing names every file of the project root outside .git, with its size, so that a report that wrote
// anything fails the case.
func (w *skillsStaleWorld) listing(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	err := filepath.WalkDir(w.root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(w.root, path)
		fmt.Fprintf(&b, "%s %s %d\n", rel, info.Mode(), info.Size())
		return nil
	})
	if err != nil {
		t.Fatalf("list the project: %v", err)
	}
	return b.String()
}

func TestSkillsStaleGolden(t *testing.T) {
	for _, tc := range skillsStaleCases() {
		t.Run(tc.name, func(t *testing.T) {
			w, got := runSkillsStaleCase(t, tc)
			w.check(t, "skills-stale-"+tc.name, got)
		})
	}
}

// What the command prints must not depend on where the world is: every case is run in two directories of very
// different lengths and must say the same thing in both.
func TestSkillsStaleGolden_DoesNotDependOnWhereTheWorldIs(t *testing.T) {
	base := t.TempDir()
	for _, tc := range skillsStaleCases() {
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
					_, answers[name] = runSkillsStaleCase(t, tc)
				})
			}
			if answers["short"] == "" || answers["short"] != answers["long"] {
				t.Errorf("the rendering depends on the length of the temporary directory the world is in:\n--- short ---\n%s\n--- long ---\n%s", answers["short"], answers["long"])
			}
		})
	}
}
