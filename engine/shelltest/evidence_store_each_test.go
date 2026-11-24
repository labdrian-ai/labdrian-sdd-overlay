package shelltest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// B14: reviewed-each <wiki directory> <identifier>... answers every identifier from one read of the page, one line
// "yes" or "no" each, in the order given, and exits 0. It agrees with reviewed on every page state: "yes" where
// reviewed exits 0, "no" where it exits 1, and the same exit 4 and stderr line where reviewed exits 4 (T4k, L72).
func TestEvidenceStore_ReviewedEachAnswersEveryIdentifierFromOneRead(t *testing.T) {
	const cannotRead = "evidence.sh: cannot read docs/wiki/evidence.md; reviewed cannot answer\n"
	t.Run("three identifiers, the page holding the first and the third", func(t *testing.T) {
		w := newEvidenceWorld(t, evidencePage(stdKinds, stdEvidence))
		wantResult(t, "reviewed-each", w.inRepo("reviewed-each", "docs/wiki", "f/aaaa-1", "f/zzzz-1", "f/ffff-1"), 0, "yes\nno\nyes\n", "")
	})
	t.Run("no page under a directory that can be searched", func(t *testing.T) {
		w := newEvidenceWorld(t, "")
		wantResult(t, "reviewed-each", w.inRepo("reviewed-each", "docs/wiki", "f/aaaa-1", "f/bbbb-1"), 0, "no\nno\n", "")
	})
	t.Run("a page that fails a check", func(t *testing.T) {
		for name, page := range failingEvidencePages {
			wantResult(t, name, newEvidenceWorld(t, page).inRepo("reviewed-each", "docs/wiki", "f/aaaa-1", "f/bbbb-1"), 0, "no\nno\n", "")
		}
	})
	t.Run("a directory at the page path", func(t *testing.T) {
		w := newEvidenceWorld(t, "")
		if err := os.MkdirAll(filepath.Join(w.repo, "docs", "wiki", "evidence.md"), 0o755); err != nil {
			t.Fatal(err)
		}
		wantResult(t, "reviewed-each", w.inRepo("reviewed-each", "docs/wiki", "f/aaaa-1", "f/bbbb-1"), 4, "", cannotRead)
	})
	t.Run("an awk that gives one line fewer than there are identifiers", func(t *testing.T) {
		w := newEvidenceWorld(t, onePage("alpha-kind"))
		env := stubAwk(t, w, "echo yes\n")
		wantResult(t, "reviewed-each", w.run(w.repo, env, "reviewed-each", "docs/wiki", "f/aaaa-1", "f/bbbb-1"), 4, "",
			"evidence.sh: awk gave no answer on docs/wiki/evidence.md; reviewed cannot answer\n")
	})
	t.Run("an awk that gives an empty line among its answers", func(t *testing.T) {
		w := newEvidenceWorld(t, onePage("alpha-kind"))
		env := stubAwk(t, w, "printf 'yes\\n\\nno\\nno\\n'\n")
		wantResult(t, "reviewed-each", w.run(w.repo, env, "reviewed-each", "docs/wiki", "f/aaaa-1", "f/bbbb-1", "f/cccc-1"), 4, "",
			"evidence.sh: awk gave no answer on docs/wiki/evidence.md; reviewed cannot answer\n")
	})
	t.Run("an identifier with a backslash is compared as its bytes", func(t *testing.T) {
		w := newEvidenceWorld(t, evidencePage(stdKinds, stdEvidence+"| alpha-kind | 1 | f/a\\b-1 | 2026-04-04 | backslash |\n"))
		wantResult(t, "reviewed-each", w.inRepo("reviewed-each", "docs/wiki", `f/a\b-1`, "f/a\bb-1"), 0, "yes\nno\n", "")
	})
	t.Run("a call it cannot read is a usage error", func(t *testing.T) {
		w := newEvidenceWorld(t, evidencePage(stdKinds, stdEvidence))
		for name, args := range map[string][]string{"no wiki directory": {"reviewed-each"}, "an empty wiki directory": {"reviewed-each", "", "f/aaaa-1"},
			"no identifier": {"reviewed-each", "docs/wiki"}, "a blank identifier": {"reviewed-each", "docs/wiki", "f/aaaa-1", " "},
			"a tab in an identifier": {"reviewed-each", "docs/wiki", "f/a\taaa-1"}, "a line break in an identifier": {"reviewed-each", "docs/wiki", "f/a\naaa-1"},
			"an empty identifier": {"reviewed-each", "docs/wiki", "f/aaaa-1", ""}} {
			wantResult(t, name, w.inRepo(args...), 2, "", evidenceUsage)
		}
	})
	t.Run("a repeated identifier gives a line each", func(t *testing.T) {
		w := newEvidenceWorld(t, evidencePage(stdKinds, stdEvidence))
		wantResult(t, "reviewed-each", w.inRepo("reviewed-each", "docs/wiki", "f/aaaa-1", "f/zzzz-1", "f/aaaa-1"), 0, "yes\nno\nyes\n", "")
	})

	// Parity with reviewed on the page states of B12 and B13.
	kinds := stdKinds + "| source-kind | 1 | | | f/src-1 | project |\n"
	evidence := stdEvidence + "| alpha-kind | 1 | f/xxxx-10 | 2026-04-01 | prefix |\n| alpha-kind | 1 |  f/spaced-1  | 2026-04-02 | spaced |\n| alpha-kind | 1 | f/a\\|b-1 | 2026-04-03 | pipe |\n"
	states := map[string]func(t *testing.T) (evidenceWorld, []string){
		"a valid page": func(t *testing.T) (evidenceWorld, []string) {
			return newEvidenceWorld(t, evidencePage(kinds, evidence)), nil
		},
		"no page": func(t *testing.T) (evidenceWorld, []string) { return newEvidenceWorld(t, ""), nil },
		"a directory at the page path": func(t *testing.T) (evidenceWorld, []string) {
			w := newEvidenceWorld(t, "")
			if err := os.MkdirAll(filepath.Join(w.repo, "docs", "wiki", "evidence.md"), 0o755); err != nil {
				t.Fatal(err)
			}
			return w, nil
		},
		"a link to a missing file at the page path": func(t *testing.T) (evidenceWorld, []string) {
			w := newEvidenceWorld(t, "")
			if err := os.MkdirAll(filepath.Join(w.repo, "docs", "wiki"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(w.tmp, "missing.md"), filepath.Join(w.repo, "docs", "wiki", "evidence.md")); err != nil {
				t.Fatal(err)
			}
			return w, nil
		},
		"a regular file where the wiki directory should be": func(t *testing.T) (evidenceWorld, []string) {
			w := newEvidenceWorld(t, "")
			if err := os.MkdirAll(filepath.Join(w.repo, "docs"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(w.repo, "docs", "wiki"), []byte("not a directory\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			return w, nil
		},
		"an awk that fails": func(t *testing.T) (evidenceWorld, []string) {
			w := newEvidenceWorld(t, onePage("alpha-kind"))
			return w, stubAwk(t, w, "exit 2\n")
		},
		"an awk that gives no answer": func(t *testing.T) (evidenceWorld, []string) {
			w := newEvidenceWorld(t, onePage("alpha-kind"))
			return w, stubAwk(t, w, "exit 0\n")
		},
	}
	for name, page := range failingEvidencePages {
		page := page // go.mod says go 1.21: the closure below runs after the loop
		states["a page that fails a check: "+name] = func(t *testing.T) (evidenceWorld, []string) { return newEvidenceWorld(t, page), nil }
	}
	ids := []string{"f/aaaa-1", "f/zzzz-1", "f/dddd-1", "f/spaced-1", "f/a|b-1", "f/xxxx-1", "f/xxxx-10", "f/src-1"}
	for name, state := range states {
		t.Run("parity with reviewed: "+name, func(t *testing.T) {
			w, env := state(t)
			each := w.run(w.repo, env, append([]string{"reviewed-each", "docs/wiki"}, ids...)...)
			var want []string
			exit4 := 0
			for _, id := range ids {
				one := w.run(w.repo, env, "reviewed", id, "docs/wiki")
				switch one.Exit {
				case 0:
					want = append(want, "yes")
				case 1:
					want = append(want, "no")
				case 4:
					exit4++
					if each.Exit != 4 || each.Stdout != "" || each.Stderr != one.Stderr {
						t.Errorf("reviewed %s exits 4 with %q; reviewed-each gave exit %d, stdout %q, stderr %q", id, one.Stderr, each.Exit, each.Stdout, each.Stderr)
					}
				default:
					t.Fatalf("reviewed %s exits %d, which is not an answer", id, one.Exit)
				}
			}
			if exit4 != 0 {
				if exit4 != len(ids) {
					t.Errorf("reviewed exits 4 for %d of %d identifiers, but all or none must", exit4, len(ids))
				}
				return
			}
			wantResult(t, "reviewed-each", each, 0, strings.Join(want, "\n")+"\n", "")
		})
	}
}
