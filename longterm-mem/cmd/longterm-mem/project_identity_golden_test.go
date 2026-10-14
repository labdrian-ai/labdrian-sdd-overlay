package main

import (
	"database/sql"

	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/identityledger"
)

// The golden files under testdata/project-identity-golden pin what the commands that choose the project for
// themselves do: the name they resolve from the working directory (`--project` left out), what they say of it,
// the exit code, and what they leave in the repository's identity ledger. They were recorded from the program as
// it stood before the identity rules read the repository through a port of their own and the adoption moved out
// of the command package (Phase 9, L7), and that change must not alter one byte of them.
//
// `stale` is the command most of the cases run: it resolves the project the way the others do (query, index,
// status, promote, doctor, sync all go through the same function) and prints it. `sync --dry-run` stands for the
// commands that promise not to record anything, and `skills-stale --project-root` for the one that resolves the
// identity of a root it is handed and adopts nothing.
//
// The cases are the layouts of a repository the rules tell apart (no repository, no remote, one remote or
// several, a declared name or a broken one, a linked worktree, a submodule, a link in the way), what a
// `--project` that is given is said to correspond to, and adoption: a name the memory already lives under (in
// Engram or in the vault registry), two of them, a name the ledger remembers, and the stores and the ledger that
// cannot be read or written.
//
// Each world is the one of the stale goldens: a project root, a PATH of its own with git behind a file that
// runs it, commits made with a fixed identity and a fixed date, a repository made from no template. What the
// ledger records from the clock is the instant of the run, and it and the digest that covers it are masked
// (maskLedger); the one instant a case seeds (seededInstant) is not, so what the ledger keeps of an earlier run
// shows.
//
// Cases 50 to 52 are a ledger that cannot be read. They were recorded when it was reported twice, as unreadable and
// then as unwritable, in the same words, because writing reads the ledger first and meets the same fault; the owner
// decided on 2026-10-14 that it is reported once, as unreadable, with the words "; the ledger is left as it is and
// nothing was recorded" after the cause, and not written to, so those three were re-recorded for that and for
// nothing else (the diff of each is the two old warnings and the one new). The ledger is left as it was
// (TestProjectIdentity_ACorruptLedgerIsLeftAsItWas). Case 55 is the other fault: a ledger that reads but cannot be
// written, which keeps its own warning.
//
// To rewrite them after a change that is MEANT to alter the output:
//
//	go test ./cmd/longterm-mem -run TestProjectIdentityGolden -update-retrieval-golden
//
// and read the diff before committing it.
const (
	// seededInstant is when the ledgers a case seeds say their names were seen. The mask of the run's own
	// instants leaves exactly this one alone, so it only has to be an instant no run writes: the commands'
	// clock in these worlds is the wall clock (or fixtureInstant), and neither is the first of 2020.
	seededInstant = "2020-01-01T00:00:00Z"
	// widgetsRemote is the origin most of the cases give their repository: it normalizes to
	// github.com/acme/widgets, and the plain name "widgets" is what the memory may already live under.
	widgetsRemote = "https://github.com/acme/widgets.git"
)

type identityWorld struct {
	*skillsStaleWorld
}

type identityCase struct {
	name  string
	setup func(t *testing.T, w *identityWorld)
	// cwd is where the command runs when it is not the project root.
	cwd func(w *identityWorld) string
	// args are the command's arguments when they are not `stale` alone.
	args func(w *identityWorld) []string
	// commonDir is the repository's git common directory, where its ledger lives, when it is not the project
	// root's .git.
	commonDir func(w *identityWorld) string
}

// repo makes the project a repository on main with one commit.
func (w *identityWorld) repo(t *testing.T) {
	t.Helper()
	w.initRepo(t)
	w.git(t, "commit", "-q", "--allow-empty", "-m", "initial")
}

// declare writes the file that names the project, in the directory given.
func (w *identityWorld) declare(t *testing.T, dir, text string) {
	t.Helper()
	w.writeFile(t, filepath.Join(dir, ".longterm-mem-project"), text)
}

// holdsMemory records an observation of the project in the Engram database of the world; deleted marks it as
// one that was soft-deleted.
func (w *identityWorld) holdsMemory(t *testing.T, project string, deleted bool) {
	t.Helper()
	db, err := sql.Open("sqlite", w.dbPath)
	if err != nil {
		t.Fatalf("open the fixture database: %v", err)
	}
	defer db.Close()
	var deletedAt any
	if deleted {
		deletedAt = "2025-12-31 00:00:00"
	}
	_, err = db.Exec(`INSERT INTO observations (session_id, type, title, content, project, topic_key, deleted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, "sess-1", "decision", "a memory of "+project, "**What**: a thing\n", project, "identity/"+project, deletedAt)
	if err != nil {
		t.Fatalf("record the memory of %q: %v", project, err)
	}
}

// vaultRegistry writes the registry file the commands read (under the world's home) with a row for each name.
func (w *identityWorld) vaultRegistry(t *testing.T, names ...string) {
	t.Helper()
	rows := make([]string, len(names))
	for i, name := range names {
		rows[i] = fmt.Sprintf("%q: {\"path\": %q}", name, w.vaultRoot)
	}
	w.writeFile(t, filepath.Join(w.home, ".labdrian-overlay", "vaults.json"), `{"schema": 1, "vaults": {`+strings.Join(rows, ", ")+"}}\n")
}

// seedLedger records names in the ledger of the repository as it stood in 2020, through the package that owns
// the file.
func (w *identityWorld) seedLedger(t *testing.T, commonDir string, names ...identityledger.Name) {
	t.Helper()
	at, err := time.Parse(time.RFC3339, seededInstant)
	if err != nil {
		t.Fatal(err)
	}
	if err := identityledger.Record(commonDir, names, at); err != nil {
		t.Fatalf("seed the ledger: %v", err)
	}
}

func (w *identityWorld) commonDirOf(tc identityCase) string {
	if tc.commonDir != nil {
		return tc.commonDir(w)
	}
	return filepath.Join(w.root, ".git")
}

// aSkillIn gives the project at root a lock and the files of one skill, so that `skills-stale` gets as far as
// the report that names the project.
func (w *identityWorld) aSkillIn(t *testing.T, root string) {
	t.Helper()
	w.writeFile(t, filepath.Join(root, ".labdrian", "procedural-skills.lock.json"), lockOf("a-skill"))
	for _, dir := range []string{".claude", ".agents"} {
		w.writeFile(t, filepath.Join(root, dir, "skills", "a-skill", "SKILL.md"), skillText("Nothing to run.\n"))
	}
}

// worktreePath is where the linked worktree of a case is: beside the project, not in it.
func (w *identityWorld) worktreePath() string { return filepath.Join(w.outside, "wt") }

// addWorktree makes a linked worktree of the project, on a branch of its own.
func (w *identityWorld) addWorktree(t *testing.T) {
	t.Helper()
	w.git(t, "worktree", "add", "-q", "-b", "feature", w.worktreePath())
}

var (
	// ledgerInstant is an instant a ledger says a name was seen at.
	ledgerInstant = regexp.MustCompile(`("(?:first|last)_seen": ")([^"]*)"`)
	// sha256Digest is a digest of the ledger's entries, wherever it is printed: in the file as its revision, and
	// in the words of the refusal of a ledger that does not match it. It covers the instants of the run.
	sha256Digest = regexp.MustCompile(`sha256:[0-9a-f]{64}`)
	// ledgerRevision is that digest as the file carries it, which the goldens print without its scheme.
	ledgerRevision = regexp.MustCompile(`("revision": ")sha256:[0-9a-f]{64}"`)
)

// maskLedger masks what depends on the run in the text of a ledger or of a message about one: the instants at
// which the run recorded its names (every instant but the one the cases seed, whatever the day it is) and the
// digests, which cover them. The digests are masked in the words of a refusal too, so that a change of the digest
// scheme fails a test of the ledger and not these goldens, with a hash for a difference.
func maskLedger(text string) string {
	text = ledgerInstant.ReplaceAllStringFunc(text, func(found string) string {
		parts := ledgerInstant.FindStringSubmatch(found)
		if parts[2] == seededInstant {
			return found
		}
		return parts[1] + `<run>"`
	})
	text = ledgerRevision.ReplaceAllString(text, `${1}<digest>"`)
	return sha256Digest.ReplaceAllString(text, "sha256:<digest>")
}

// renderLedger is the ledger file of the repository as it is after the run, with what depends on the run
// masked, or the words that there is none.
func renderLedger(commonDir string) string {
	raw, err := os.ReadFile(filepath.Join(commonDir, "longterm-mem", "identity-ledger.json"))
	if err != nil {
		return "no ledger\n"
	}
	return maskLedger(string(raw))
}

// maskForIdentity masks the words of the libraries a case of this set can print, on top of what every world masks.
func (w *identityWorld) maskForIdentity(text string) string {
	text = w.maskForSkillsStale(text)
	text = strings.ReplaceAll(text, w.worktreePath(), "<worktree>")
	return maskLibraryErrors(maskLedger(text), identityJSONError, sqliteOpenError, identityNotADirectory)
}

var (
	// identityJSONError is the JSON parser's message for a file that is not JSON, as the vault registry and the
	// ledger carry it.
	identityJSONError = libraryError{regexp.MustCompile(`((?:vaultreg: parse|identityledger: ledger does not match its own digest: ).*?(?:vaults\.json|identity-ledger\.json)[^\n]*?)(?:invalid character [^\n)]*|unexpected end of JSON input)`), "${1}<json syntax error>"}
	// identityNotADirectory is the operating system's message for a file read through a path that has a file in it.
	identityNotADirectory = libraryError{regexp.MustCompile(`(identity-ledger\.json): not a directory`), "${1}: <not a directory>"}
)

func identityCases() []identityCase {
	staleIn := func(args ...string) func(w *identityWorld) []string {
		return func(w *identityWorld) []string { return append([]string{"stale"}, args...) }
	}
	inWorktree := func(w *identityWorld) string { return w.worktreePath() }
	return []identityCase{
		// Resolution, by the layout of the repository.
		{name: "01-no-repository", cwd: func(w *identityWorld) string { return w.outside }},
		{name: "02-a-repository-with-no-remote", setup: func(t *testing.T, w *identityWorld) { w.repo(t) }},
		{name: "03-a-repository-with-an-origin", setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.git(t, "remote", "add", "origin", widgetsRemote)
		}},
		{name: "04-several-remotes-origin-wins", setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.git(t, "remote", "add", "upstream", "https://github.com/upstream-org/widgets.git")
			w.git(t, "remote", "add", "origin", widgetsRemote)
			w.git(t, "remote", "add", "mirror", "https://gitlab.com/mirror/widgets.git")
		}},
		{name: "05-a-remote-that-is-not-origin", setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.git(t, "remote", "add", "upstream", "https://github.com/upstream-org/widgets.git")
		}},
		{name: "06-an-origin-in-the-scp-form", setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.git(t, "remote", "add", "origin", "git@Example.com:acme/widgets.git")
		}},
		{name: "07-a-declared-name", setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.declare(t, w.root, "declared-widgets\n")
		}},
		{name: "08-a-declared-name-beats-the-origin", setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.git(t, "remote", "add", "origin", widgetsRemote)
			w.declare(t, w.root, "  declared-widgets  \r\n")
		}},
		{name: "09-a-declaration-of-two-lines", setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.declare(t, w.root, "one\ntwo\n")
		}},
		{name: "10-a-declaration-of-nothing", setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.declare(t, w.root, " \n")
		}},
		{name: "11-a-declaration-that-is-a-directory", setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			if err := os.Mkdir(filepath.Join(w.root, ".longterm-mem-project"), 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "12-a-linked-worktree-of-a-repository-with-an-origin", cwd: inWorktree, setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.git(t, "remote", "add", "origin", widgetsRemote)
			w.addWorktree(t)
		}},
		{name: "13-a-linked-worktree-of-a-repository-with-no-remote", cwd: inWorktree, setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.addWorktree(t)
		}},
		{name: "14-a-name-declared-in-the-main-checkout-read-from-a-worktree", cwd: inWorktree, setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.declare(t, w.root, "declared-widgets\n")
			w.addWorktree(t)
		}},
		{name: "15-a-name-declared-only-in-a-worktree-is-not-read", cwd: inWorktree, setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.git(t, "remote", "add", "origin", widgetsRemote)
			w.addWorktree(t)
			w.declare(t, w.worktreePath(), "worktree-only\n")
		}},
		{name: "16-below-the-root-of-the-repository", cwd: func(w *identityWorld) string { return filepath.Join(w.root, "module", "promote") }, setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.git(t, "remote", "add", "origin", widgetsRemote)
			w.writeProjectFile(t, "module/promote/writer.go", "package promote\n")
		}},
		{name: "17-a-submodule", cwd: func(w *identityWorld) string { return filepath.Join(w.root, "vendored") },
			commonDir: func(w *identityWorld) string { return filepath.Join(w.root, ".git", "modules", "vendored") },
			setup: func(t *testing.T, w *identityWorld) {
				w.repo(t)
				source := filepath.Join(w.outside, "source")
				if err := os.Mkdir(source, 0o755); err != nil {
					t.Fatal(err)
				}
				for _, args := range [][]string{{"init", "-q", "--template=", "-b", "main"}, {"commit", "-q", "--allow-empty", "-m", "initial"}} {
					w.gitIn(t, source, args...)
				}
				// What `git submodule add` does, without the shell script that needs the tools the world's PATH
				// does not have: a clone whose git directory is kept in the superproject's, and a .git file
				// in the submodule that points back to it by a relative path.
				if err := os.MkdirAll(filepath.Join(w.root, ".git", "modules"), 0o755); err != nil {
					t.Fatal(err)
				}
				w.git(t, "clone", "-q", "--separate-git-dir", filepath.Join(w.root, ".git", "modules", "vendored"), source, "vendored")
				w.writeProjectFile(t, "vendored/.git", "gitdir: ../.git/modules/vendored\n")
			}},
		{name: "18-a-git-directory-that-is-a-link", setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			elsewhere := filepath.Join(w.outside, "elsewhere", ".git")
			if err := os.MkdirAll(filepath.Dir(elsewhere), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(filepath.Join(w.root, ".git"), elsewhere); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(elsewhere, filepath.Join(w.root, ".git")); err != nil {
				t.Fatal(err)
			}
		}, commonDir: func(w *identityWorld) string { return filepath.Join(w.outside, "elsewhere", ".git") }},
		{name: "19-a-working-directory-reached-through-a-link", cwd: func(w *identityWorld) string { return filepath.Join(w.outside, "link") }, setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.git(t, "remote", "add", "origin", widgetsRemote)
			if err := os.Symlink(w.root, filepath.Join(w.outside, "link")); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "20-a-git-link-that-leads-nowhere", setup: func(t *testing.T, w *identityWorld) {
			if err := os.Symlink(filepath.Join(w.outside, "nowhere"), filepath.Join(w.root, ".git")); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "21-a-git-file-that-names-no-directory", setup: func(t *testing.T, w *identityWorld) {
			w.writeProjectFile(t, ".git", "this is not a pointer\n")
		}},
		{name: "22-a-git-file-that-names-an-empty-directory", setup: func(t *testing.T, w *identityWorld) {
			w.writeProjectFile(t, ".git", "gitdir:   \n")
		}},
		{name: "23-a-git-file-that-names-a-directory-that-is-not-there", setup: func(t *testing.T, w *identityWorld) {
			w.writeProjectFile(t, ".git", "gitdir: "+filepath.Join(w.outside, "gone")+"\n")
		}},

		// What a --project that is given is said to correspond to.
		{name: "30-a-project-named-as-the-origin-is", args: staleIn("--project", "github.com/acme/widgets"), setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.git(t, "remote", "add", "origin", widgetsRemote)
		}},
		{name: "31-a-project-named-as-the-last-segment-of-the-origin", args: staleIn("--project", "widgets"), setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.git(t, "remote", "add", "origin", widgetsRemote)
		}},
		{name: "32-a-project-named-as-the-directory-of-a-repository-with-no-remote", args: func(w *identityWorld) []string {
			return []string{"stale", "--project", filepath.Base(w.root)}
		}, setup: func(t *testing.T, w *identityWorld) { w.repo(t) }},
		{name: "33-a-project-that-is-another-one", args: staleIn("--project", "somebody-elses"), setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.declare(t, w.root, "declared-widgets\n")
		}},
		{name: "34-a-project-that-is-given-with-no-repository", args: staleIn("--project", "somebody-elses"), cwd: func(w *identityWorld) string { return w.outside }},

		// Adoption: the memory decides which of the names the repository derives is the one used.
		{name: "40-a-name-the-memory-lives-under", setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.git(t, "remote", "add", "origin", widgetsRemote)
			w.holdsMemory(t, "widgets", false)
		}},
		{name: "41-two-names-that-hold-memory", setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.git(t, "remote", "add", "origin", widgetsRemote)
			w.declare(t, w.root, "declared-widgets\n")
			w.holdsMemory(t, "widgets", false)
			w.holdsMemory(t, "declared-widgets", false)
			w.holdsMemory(t, "github.com/acme/widgets", false)
		}},
		{name: "42-a-name-the-vault-registry-holds", setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.git(t, "remote", "add", "origin", widgetsRemote)
			w.vaultRegistry(t, "widgets", "unrelated")
		}},
		{name: "43-a-name-whose-memory-was-all-deleted", setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.git(t, "remote", "add", "origin", widgetsRemote)
			w.holdsMemory(t, "widgets", true)
		}},
		{name: "44-a-name-from-a-linked-worktree", cwd: inWorktree, setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.git(t, "remote", "add", "origin", widgetsRemote)
			w.addWorktree(t)
			w.holdsMemory(t, "widgets", false)
		}},
		{name: "45-a-name-the-ledger-remembers", setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.seedLedger(t, filepath.Join(w.root, ".git"), identityledger.Name{Name: "/moved/from/here/.git", Rule: "common_dir", Adoptable: true})
			w.holdsMemory(t, "/moved/from/here/.git", false)
		}},
		{name: "46-a-name-the-ledger-remembers-that-is-too-loose-to-adopt", setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.seedLedger(t, filepath.Join(w.root, ".git"), identityledger.Name{Name: "old-directory", Rule: "common_dir", Adoptable: false})
			w.holdsMemory(t, "old-directory", false)
		}},
		{name: "47-a-name-the-repository-derives-beats-one-the-ledger-remembers", setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.declare(t, w.root, "current-name\n")
			w.seedLedger(t, filepath.Join(w.root, ".git"), identityledger.Name{Name: "former-name", Rule: "declared", Adoptable: true})
			w.holdsMemory(t, "current-name", false)
			w.holdsMemory(t, "former-name", false)
		}},
		{name: "48-a-ledger-that-keeps-what-it-held", setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.git(t, "remote", "add", "origin", widgetsRemote)
			w.seedLedger(t, filepath.Join(w.root, ".git"),
				identityledger.Name{Name: "github.com/acme/widgets", Rule: "remote", Adoptable: true},
				identityledger.Name{Name: "former-name", Rule: "declared", Adoptable: true})
		}},

		// What is not consulted or not kept, and is said so.
		{name: "50-a-ledger-that-does-not-match-its-digest", setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.git(t, "remote", "add", "origin", widgetsRemote)
			w.seedLedger(t, filepath.Join(w.root, ".git"), identityledger.Name{Name: "former-name", Rule: "declared", Adoptable: true})
			ledger := filepath.Join(w.root, ".git", "longterm-mem", "identity-ledger.json")
			raw, err := os.ReadFile(ledger)
			if err != nil {
				t.Fatal(err)
			}
			w.writeFile(t, ledger, strings.Replace(string(raw), "former-name", "tampered-name", 1))
		}},
		{name: "51-a-ledger-that-is-not-json", setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.writeFile(t, filepath.Join(w.root, ".git", "longterm-mem", "identity-ledger.json"), "{ not json")
		}},
		{name: "52-a-ledger-directory-that-is-a-file", setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.writeFile(t, filepath.Join(w.root, ".git", "longterm-mem"), "in the way\n")
		}},
		{name: "53-a-vault-registry-that-is-not-json", setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.git(t, "remote", "add", "origin", widgetsRemote)
			w.writeFile(t, filepath.Join(w.home, ".labdrian-overlay", "vaults.json"), "{ not json")
		}},
		{name: "54-an-engram-database-that-cannot-be-opened", setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.git(t, "remote", "add", "origin", widgetsRemote)
			t.Setenv(engramDBEnvVar, filepath.Join(w.outside, "no", "such", "directory", "engram.db"))
		}},
		{name: "55-a-ledger-that-reads-but-cannot-be-written", setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.git(t, "remote", "add", "origin", widgetsRemote)
			// The ledger's directory is a link to a place that is not there. Reading through it finds no ledger,
			// which is an empty one and not a fault; writing cannot make the directory. Nothing in this depends on
			// who the process runs as, as a directory it is not allowed to write to would.
			if err := os.Symlink(filepath.Join(w.outside, "gone"), filepath.Join(w.root, ".git", "longterm-mem")); err != nil {
				t.Fatal(err)
			}
		}},

		// The commands that record nothing, and the one that adopts nothing.
		{name: "60-a-dry-run-records-nothing", args: func(w *identityWorld) []string { return []string{"sync", "--dry-run"} }, setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.git(t, "remote", "add", "origin", widgetsRemote)
			w.holdsMemory(t, "widgets", false)
		}},
		{name: "61-a-dry-run-leaves-the-ledger-as-it-was", args: func(w *identityWorld) []string { return []string{"sync", "--dry-run"} }, setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.git(t, "remote", "add", "origin", widgetsRemote)
			w.seedLedger(t, filepath.Join(w.root, ".git"), identityledger.Name{Name: "former-name", Rule: "declared", Adoptable: true})
		}},
		{name: "62-status-resolves-and-records", args: func(w *identityWorld) []string { return []string{"status"} }, setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.git(t, "remote", "add", "origin", widgetsRemote)
			w.holdsMemory(t, "widgets", false)
		}},
		{name: "63-doctor-resolves-and-records-nothing", args: func(w *identityWorld) []string { return []string{"doctor"} }, setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.git(t, "remote", "add", "origin", widgetsRemote)
			w.holdsMemory(t, "widgets", false)
		}},
		{name: "64-stale-when-the-observations-cannot-be-listed", args: func(w *identityWorld) []string {
			return []string{"stale", "--project", "github.com/acme/widgets"}
		}, setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.git(t, "remote", "add", "origin", widgetsRemote)
			db, err := sql.Open("sqlite", w.dbPath)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err := db.Exec(`DROP TABLE observations`); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "70-skills-stale-names-the-origin", args: func(w *identityWorld) []string { return []string{"skills-stale", "--project-root", w.root} }, setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.git(t, "remote", "add", "origin", widgetsRemote)
			w.holdsMemory(t, "widgets", false)
			w.aSkillIn(t, w.root)
		}},
		{name: "71-skills-stale-names-a-declared-name", args: func(w *identityWorld) []string { return []string{"skills-stale", "--project-root", w.root} }, setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.declare(t, w.root, "declared-widgets\n")
			w.aSkillIn(t, w.root)
		}},
		{name: "72-skills-stale-of-a-linked-worktree", args: func(w *identityWorld) []string { return []string{"skills-stale", "--project-root", w.worktreePath()} }, setup: func(t *testing.T, w *identityWorld) {
			w.repo(t)
			w.git(t, "remote", "add", "origin", widgetsRemote)
			w.addWorktree(t)
			w.aSkillIn(t, w.worktreePath())
		}},
		{name: "73-skills-stale-of-a-root-that-is-no-repository", args: func(w *identityWorld) []string { return []string{"skills-stale", "--project-root", w.outside} }},
	}
}

// gitIn is git run in dir, with the identity and the date of the world.
func (w *identityWorld) gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	if out, err := w.tryGitOutputIn(dir, args...); err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
}

// runIdentityCase builds the world of the case, runs the command in it and renders what it did and what it left
// in the ledger.
func newIdentityWorld(t *testing.T) *identityWorld {
	t.Helper()
	w := &identityWorld{skillsStaleWorld: newSkillsStaleWorld(t, true)}
	w.goldenDir = filepath.Join(filepath.Dir(w.goldenDir), "project-identity-golden")
	// The repository's directory has a name of its own, because the plainest name a repository derives is its
	// directory's, and a counter of the temporary directories is not a name a golden can hold.
	w.root = filepath.Join(w.root, "checkout")
	if err := os.Mkdir(w.root, 0o755); err != nil {
		t.Fatalf("create the project root: %v", err)
	}
	return w
}

func runIdentityCase(t *testing.T, tc identityCase) (w *identityWorld, rendering string) {
	t.Helper()
	w = newIdentityWorld(t)
	if tc.setup != nil {
		tc.setup(t, w)
	}
	dir := w.root
	if tc.cwd != nil {
		dir = tc.cwd(w)
	}
	t.Chdir(dir)

	args := []string{"stale"}
	if tc.args != nil {
		args = tc.args(w)
	}
	code, stdout, stderr := w.run(t, args)

	rendering = renderRun(args, code, stdout, stderr) + "--- ledger ---\n" + renderLedger(w.commonDirOf(tc))
	return w, w.maskForIdentity(rendering)
}

func TestProjectIdentityGolden(t *testing.T) {
	for _, tc := range identityCases() {
		t.Run(tc.name, func(t *testing.T) {
			w, got := runIdentityCase(t, tc)
			w.check(t, "project-identity-"+tc.name, got)
		})
	}
}

// What the commands print and leave must not depend on where the world is: every case is run in two
// directories of very different lengths and must say the same thing in both.
func TestProjectIdentityGolden_DoesNotDependOnWhereTheWorldIs(t *testing.T) {
	base := t.TempDir()
	for _, tc := range identityCases() {
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
					_, answers[name] = runIdentityCase(t, tc)
				})
			}
			if answers["short"] == "" || answers["short"] != answers["long"] {
				t.Errorf("the rendering depends on the length of the temporary directory the world is in:\n--- short ---\n%s\n--- long ---\n%s", answers["short"], answers["long"])
			}
		})
	}
}

// The mask of the run's own instants keeps the seeded one whatever the day, and the digests are masked wherever
// they are printed: in the file as its revision and in the words of the refusal of a ledger that does not match.
func TestMaskLedger_KeepsTheSeededInstantAndMasksTheRestAndTheDigests(t *testing.T) {
	digest := "sha256:" + strings.Repeat("ab", 32)
	other := "sha256:" + strings.Repeat("cd", 32)
	text := "{\n  \"revision\": \"" + digest + "\",\n" +
		"  \"first_seen\": \"" + seededInstant + "\",\n  \"last_seen\": \"2031-05-06T07:08:09.123456789Z\",\n  \"first_seen\": \"1999-12-31T23:59:59Z\"\n}\n" +
		"ledger carries " + digest + " but its entries digest to " + other + "\n"

	got := maskLedger(text)

	want := "{\n  \"revision\": \"<digest>\",\n" +
		"  \"first_seen\": \"" + seededInstant + "\",\n  \"last_seen\": \"<run>\",\n  \"first_seen\": \"<run>\"\n}\n" +
		"ledger carries sha256:<digest> but its entries digest to sha256:<digest>\n"
	if got != want {
		t.Errorf("maskLedger =\n%s\nwant\n%s", got, want)
	}
}

// A ledger that cannot be trusted is left exactly as it was, and the command says so once. The goldens show the
// ledger's text after the run; this compares its bytes before and after, so a run that rewrote it with the same
// words, or with a newline at the end, would fail here, and it counts what the command says of the ledger: one
// warning, that it could not be read, saying nothing was recorded; none that it could not be written (the owner's
// decision of 2026-10-14: a ledger that cannot be read is not written, so the same cause is not reported twice).
func TestProjectIdentity_ACorruptLedgerIsLeftAsItWas(t *testing.T) {
	for _, prefix := range []string{"50-", "51-"} {
		for _, tc := range identityCases() {
			if !strings.HasPrefix(tc.name, prefix) {
				continue
			}
			t.Run(tc.name, func(t *testing.T) {
				w := newIdentityWorld(t)
				tc.setup(t, w)
				ledger := filepath.Join(w.commonDirOf(tc), "longterm-mem", "identity-ledger.json")
				before, err := os.ReadFile(ledger)
				if err != nil {
					t.Fatalf("the case seeds no ledger: %v", err)
				}
				t.Chdir(w.root)

				code, _, stderr := w.run(t, []string{"stale"})

				after, err := os.ReadFile(ledger)
				if err != nil || string(after) != string(before) {
					t.Errorf("the ledger is %q (%v) after the run, was %q", after, err, before)
				}
				if code != exitOK {
					t.Errorf("exit %d, want the command to go on (exit %d)", code, exitOK)
				}
				if got := strings.Count(stderr, "identity ledger could not be read"); got != 1 {
					t.Errorf("the command said the ledger could not be read %d times, want once; stderr:\n%s", got, stderr)
				}
				if strings.Contains(stderr, "could not be written") {
					t.Errorf("the command says the ledger could not be written, which it did not try to do; stderr:\n%s", stderr)
				}
				if !strings.Contains(stderr, "; the ledger is left as it is and nothing was recorded\n") {
					t.Errorf("the warning does not end by saying the ledger was left alone and nothing recorded; stderr:\n%s", stderr)
				}
			})
		}
	}
}
