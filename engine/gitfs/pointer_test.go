package gitfs

import (
	"path/filepath"
	"strings"
	"testing"
)

// The reader of a .git file is lenient about the whitespace around the line and around the path,
// and exact about the word "gitdir:" and about the path leading to an existing directory. What
// the path means (a relative one is taken relative to the directory of the .git file) is
// gitprov.PointerTarget's; this pins what this reader does to the line before and to the path after.
func TestTheGitFileIsReadLeniently(t *testing.T) {
	gitDir := filepath.Join(t.TempDir(), "modules", "gitdir")
	writeFixtureFile(t, filepath.Join(gitDir, "HEAD"), strings.Repeat("c", 40)+"\n")
	checkout := filepath.Join(filepath.Dir(filepath.Dir(gitDir)), "checkout")
	want := wantRepoKey(t, gitDir)

	accepted := map[string]string{
		"a pointer":                                   "gitdir: " + gitDir + "\n",
		"no line break":                               "gitdir: " + gitDir,
		"spaces around the line":                      "  gitdir:    " + gitDir + "   \n\n",
		"a carriage return":                           "gitdir: " + gitDir + "\r\n",
		"no space after the colon":                    "gitdir:" + gitDir + "\n",
		"a path relative to the checkout":             "gitdir: ../modules/gitdir\n",
		"a path that needs cleaning":                  "gitdir: ../modules/../modules/./gitdir/\n",
		"a tab between the colon and the path":        "gitdir:\t" + gitDir + "\n",
		"a path relative to the checkout, with a dot": "gitdir: ./../modules/gitdir\n",
	}
	for name, content := range accepted {
		t.Run("accepts "+name, func(t *testing.T) {
			writeFixtureFile(t, filepath.Join(checkout, ".git"), content)
			key, ok := locator.RepoKey(checkout)
			if !ok || key != want {
				t.Fatalf("RepoKey = %q, %v, want %q, true", key, ok, want)
			}
			if p := locator.Provenance(checkout); p.WorktreeRoot != checkout || p.GitHead != strings.Repeat("c", 40) {
				t.Fatalf("Provenance = %+v, want the checkout and the HEAD of the git directory", p)
			}
		})
	}

	notADirectory := filepath.Join(t.TempDir(), "a-file")
	writeFixtureFile(t, notADirectory, "x")
	refused := map[string]string{
		"no gitdir word":                 "not a gitdir pointer\n",
		"an empty file":                  "",
		"a gitdir word and no path":      "gitdir:\n",
		"a gitdir word and only spaces":  "gitdir:    \n",
		"a path that does not exist":     "gitdir: " + filepath.Join(filepath.Dir(gitDir), "gone") + "\n",
		"a path that is a file":          "gitdir: " + notADirectory + "\n",
		"a second line":                  "gitdir: " + gitDir + "\nsecond line\n",
		"the word in capitals":           "GITDIR: " + gitDir + "\n",
		"the word and a space before it": "git dir: " + gitDir + "\n",
	}
	for name, content := range refused {
		t.Run("refuses "+name, func(t *testing.T) {
			writeFixtureFile(t, filepath.Join(checkout, ".git"), content)
			if key, ok := locator.RepoKey(checkout); ok || key != "" {
				t.Fatalf("RepoKey = %q, %v, want \"\", false", key, ok)
			}
			if p := locator.Provenance(checkout); p.WorktreeRoot != "" || p.GitHead != "" {
				t.Fatalf("Provenance = %+v, want an empty one", p)
			}
		})
	}
}
