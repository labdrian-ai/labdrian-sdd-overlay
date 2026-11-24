package shelltest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// B14: reviewed-each <wiki directory> <identifier>... answers every identifier from one read of the page, one line
// "yes" or "no" each, in the order given, and exits 0. It agrees with reviewed on every page state: "yes" where
// reviewed exits 0, "no" where it exits 1, and the same exit 4 and stderr line where reviewed exits 4 (T4k, L72).
func TestEvidenceStore_ReviewedEachAnswersEveryIdentifierFromOneRead(t *testing.T) {
	const cannotRead = "evidence.sh: cannot read docs/wiki/evidence.md; reviewed-each cannot answer\n"
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
			"evidence.sh: awk gave no answer on docs/wiki/evidence.md; reviewed-each cannot answer\n")
	})
	t.Run("an awk that gives an empty line among its answers", func(t *testing.T) {
		w := newEvidenceWorld(t, onePage("alpha-kind"))
		env := stubAwk(t, w, "printf 'yes\\n\\nno\\nno\\n'\n")
		wantResult(t, "reviewed-each", w.run(w.repo, env, "reviewed-each", "docs/wiki", "f/aaaa-1", "f/bbbb-1", "f/cccc-1"), 4, "",
			"evidence.sh: awk gave no answer on docs/wiki/evidence.md; reviewed-each cannot answer\n")
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

	// The bound of one environment string (T4q, L78 choice 4; T4r, L80 choice 1): the identifiers reach awk joined in
	// one, which Linux limits to MAX_ARG_STRLEN, 32 pages (128 KiB with 4 KiB pages). Past it awk cannot start and the
	// mode exits 4, one stderr line; under it, it answers. The page size is read from the host, so the test holds on
	// every page size.
	t.Run("the identifiers of one call past the bound of one environment string", func(t *testing.T) {
		if runtime.GOOS != "linux" {
			t.Skip("MAX_ARG_STRLEN, the limit of one environment string, is the limit of Linux")
		}
		getconf, err := exec.LookPath("getconf")
		if err != nil {
			t.Skip("getconf is not on PATH: the page size, and so the bound, is unknown")
		}
		sysconf := func(name string) int {
			t.Helper()
			out, err := exec.Command(getconf, name).Output()
			if err != nil {
				t.Fatalf("getconf %s: %v", name, err)
			}
			n, err := strconv.Atoi(strings.TrimSpace(string(out)))
			if err != nil || n < 1 {
				t.Fatalf("getconf %s printed %q, want a number of 1 or more", name, out)
			}
			return n
		}
		bound := 32 * sysconf("PAGESIZE")
		argMax := sysconf("ARG_MAX")
		// An identifier "f/review-%08d" is 17 bytes, 18 joined with its line break. Each case sits 64 identifiers (1,152
		// bytes) from the bound, the EVIDENCE_WANT= prefix and the end byte included.
		const joined = 18
		pastN, underN := bound/joined+64, bound/joined-64
		t.Logf("bound %d bytes (32 pages), ARG_MAX %d, %d identifiers past, %d under", bound, argMax, pastN, underN)
		manyIDs := func(n int) []string {
			ids := make([]string, n)
			for i := range ids {
				ids[i] = fmt.Sprintf("f/review-%08d", i)
			}
			return ids
		}
		w := newEvidenceWorld(t, onePage("alpha-kind"))
		// The identifiers also reach sh as arguments, and exec limits arguments and environment together (ARG_MAX): when
		// the joined bytes reach half of it, the exec of sh itself may fail, not awk's, so the past case is not run.
		if pastN*joined >= argMax/2 {
			t.Logf("past case skipped: %d joined bytes reach half of ARG_MAX (%d)", pastN*joined, argMax)
		} else {
			past := w.inRepo(append([]string{"reviewed-each", "docs/wiki"}, manyIDs(pastN)...)...)
			if !regexp.MustCompile(`^evidence\.sh: awk failed on docs/wiki/evidence\.md \(exit [0-9]+\); reviewed-each cannot answer\n$`).MatchString(past.Stderr) || past.Exit != 4 || past.Stdout != "" {
				t.Errorf("%d identifiers: exit %d, stdout of %d bytes, stderr %q, want exit 4, no stdout and one awk-failed line", pastN, past.Exit, len(past.Stdout), past.Stderr)
			}
		}
		under := w.inRepo(append([]string{"reviewed-each", "docs/wiki"}, manyIDs(underN)...)...)
		wantResult(t, fmt.Sprintf("%d identifiers", underN), under, 0, strings.Repeat("no\n", underN), "")
	})

	// Parity with reviewed on the page states of B12 and B13. A state returns its world, the environment it adds and
	// the wiki directory argument (a dash directory is run with -w).
	kinds := stdKinds + "| source-kind | 1 | | | f/src-1 | project |\n"
	evidence := stdEvidence + "| alpha-kind | 1 | f/xxxx-10 | 2026-04-01 | prefix |\n| alpha-kind | 1 |  f/spaced-1  | 2026-04-02 | spaced |\n| alpha-kind | 1 | f/a\\|b-1 | 2026-04-03 | pipe |\n| alpha-kind | 1 | f/a\\b-1 | 2026-04-04 | backslash |\n"
	type state func(t *testing.T) (w evidenceWorld, env []string, wiki string)
	plain := func(page string) state {
		return func(t *testing.T) (evidenceWorld, []string, string) {
			return newEvidenceWorld(t, page), nil, "docs/wiki"
		}
	}
	notRoot := func(t *testing.T) {
		t.Helper()
		if os.Geteuid() == 0 {
			t.Skip("root reads a page and searches a directory whatever their mode")
		}
	}
	chmod := func(t *testing.T, path string, mode os.FileMode) {
		t.Helper()
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(path, 0o755) })
	}
	states := map[string]state{
		"a valid page": plain(evidencePage(kinds, evidence)),
		"no page":      plain(""),
		"a directory at the page path": func(t *testing.T) (evidenceWorld, []string, string) {
			w := newEvidenceWorld(t, "")
			if err := os.MkdirAll(filepath.Join(w.repo, "docs", "wiki", "evidence.md"), 0o755); err != nil {
				t.Fatal(err)
			}
			return w, nil, "docs/wiki"
		},
		"a link to a missing file at the page path": func(t *testing.T) (evidenceWorld, []string, string) {
			w := newEvidenceWorld(t, "")
			if err := os.MkdirAll(filepath.Join(w.repo, "docs", "wiki"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(w.tmp, "missing.md"), filepath.Join(w.repo, "docs", "wiki", "evidence.md")); err != nil {
				t.Fatal(err)
			}
			return w, nil, "docs/wiki"
		},
		"a regular file where the wiki directory should be": func(t *testing.T) (evidenceWorld, []string, string) {
			w := newEvidenceWorld(t, "")
			if err := os.MkdirAll(filepath.Join(w.repo, "docs"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(w.repo, "docs", "wiki"), []byte("not a directory\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			return w, nil, "docs/wiki"
		},
		"an awk that fails": func(t *testing.T) (evidenceWorld, []string, string) {
			w := newEvidenceWorld(t, onePage("alpha-kind"))
			return w, stubAwk(t, w, "exit 2\n"), "docs/wiki"
		},
		"an awk that exits 1": func(t *testing.T) (evidenceWorld, []string, string) {
			w := newEvidenceWorld(t, onePage("alpha-kind"))
			return w, stubAwk(t, w, "exit 1\n"), "docs/wiki"
		},
		"an awk that gives no answer": func(t *testing.T) (evidenceWorld, []string, string) {
			w := newEvidenceWorld(t, onePage("alpha-kind"))
			return w, stubAwk(t, w, "exit 0\n"), "docs/wiki"
		},
		"a page of mode 000": func(t *testing.T) (evidenceWorld, []string, string) {
			notRoot(t)
			w := newEvidenceWorld(t, onePage("alpha-kind"))
			chmod(t, filepath.Join(w.repo, "docs", "wiki", "evidence.md"), 0)
			return w, nil, "docs/wiki"
		},
		"a wiki directory of mode 600": func(t *testing.T) (evidenceWorld, []string, string) {
			notRoot(t)
			w := newEvidenceWorld(t, onePage("alpha-kind"))
			chmod(t, filepath.Join(w.repo, "docs", "wiki"), 0o600)
			return w, nil, "docs/wiki"
		},
		"a dash wiki directory of mode 600": func(t *testing.T) (evidenceWorld, []string, string) {
			notRoot(t)
			w := newEvidenceWorld(t, "")
			if err := os.Mkdir(filepath.Join(w.repo, "-w"), 0o755); err != nil {
				t.Fatal(err)
			}
			chmod(t, filepath.Join(w.repo, "-w"), 0o600)
			return w, nil, "-w"
		},
		"an empty wiki directory": func(t *testing.T) (evidenceWorld, []string, string) {
			w := newEvidenceWorld(t, "")
			if err := os.MkdirAll(filepath.Join(w.repo, "docs", "wiki"), 0o755); err != nil {
				t.Fatal(err)
			}
			return w, nil, "docs/wiki"
		},
		"a dash wiki directory that holds nothing": func(t *testing.T) (evidenceWorld, []string, string) {
			w := newEvidenceWorld(t, "")
			if err := os.Mkdir(filepath.Join(w.repo, "-w"), 0o755); err != nil {
				t.Fatal(err)
			}
			return w, nil, "-w"
		},
	}
	for name, page := range failingEvidencePages {
		states["a page that fails a check: "+name] = plain(page)
	}
	ids := []string{"f/aaaa-1", "f/zzzz-1", "f/dddd-1", "f/spaced-1", "f/a|b-1", "f/xxxx-1", "f/xxxx-10", "f/src-1", `f/a\b-1`}
	for name, build := range states {
		t.Run("parity with reviewed: "+name, func(t *testing.T) {
			w, env, wiki := build(t)
			each := w.run(w.repo, env, append([]string{"reviewed-each", wiki}, ids...)...)
			var want []string
			exit4 := 0
			for _, id := range ids {
				one := w.run(w.repo, env, "reviewed", id, wiki)
				switch one.Exit {
				case 0:
					want = append(want, "yes")
				case 1:
					want = append(want, "no")
				case 4:
					exit4++
					// The line says which mode ran: reviewed-each's is reviewed's with its own name.
					named := strings.Replace(one.Stderr, "; reviewed cannot answer", "; reviewed-each cannot answer", 1)
					if each.Exit != 4 || each.Stdout != "" || each.Stderr != named {
						t.Errorf("reviewed %s exits 4 with %q; reviewed-each gave exit %d, stdout %q, stderr %q, want %q", id, one.Stderr, each.Exit, each.Stdout, each.Stderr, named)
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
