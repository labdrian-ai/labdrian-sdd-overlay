package shelltest

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// The modes of the evidence store (S4, T2b): skills/_shared/evidence/evidence.sh, run under sh as it is
// deployed, against a temporary repository that holds only docs/wiki/evidence.md. The script is the
// snapshot's code, so these tests pin what it does; each is shown to matter by a mutation of the script.

const (
	kindsHead    = "# Evidence\n\n## Kinds\n\n| Kind | Level | Rule | Applies when | Source | Scope |\n|------|-------|------|--------------|--------|-------|\n"
	evidenceHead = "\n## Evidence\n\n| Kind | Level | Review | Date | Finding |\n|------|-------|--------|------|---------|\n"

	stdKinds = "| alpha-kind | 1 | | | | project |\n| beta-kind | 2 | Do beta. | beta tasks | 3 reviews | project |\n| gamma-kind | 3 | Do gamma. | gamma tasks | 4 reviews | project |\n"

	// alpha has three distinct reviews; beta one review on two rows; gamma two distinct reviews.
	stdEvidence = "| alpha-kind | 1 | f/aaaa-1 | 2026-01-05 | one |\n| alpha-kind | 1 | f/bbbb-1 | 2026-02-10 | two |\n| alpha-kind | 1 | f/cccc-1 | 2026-01-20 | three |\n| beta-kind | 2 | f/dddd-1 | 2026-03-01 | four |\n| beta-kind | 2 | f/dddd-1 | 2026-03-01 | five |\n| gamma-kind | 3 | f/eeee-1 | 2026-03-05 | six |\n| gamma-kind | 3 | f/ffff-1 | 2026-03-09 | seven |\n"

	evidenceUsage = "usage: sh evidence.sh check|count|rules|index [wiki directory]\n       sh evidence.sh rule <kind> [wiki directory]\n       sh evidence.sh reviewed <identifier> [wiki directory]\n"
)

func evidencePage(kinds, evidence string) string { return kindsHead + kinds + evidenceHead + evidence }

// onePage is a valid page with the one kind, at level 1, and one review of it.
func onePage(kind string) string {
	return evidencePage("| "+kind+" | 1 | | | | project |\n", "| "+kind+" | 1 | f/aaaa-1 | 2026-01-01 | x |\n")
}

func writeEvidenceFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// evidenceWorld is the deployed layout: <tmp>/skills/_shared/evidence/ holds the four files, and
// <tmp>/repo holds only docs/wiki/evidence.md (nothing at all when page is empty).
type evidenceWorld struct {
	t      *testing.T
	tmp    string
	repo   string
	script string
}

// newEvidenceWorld also refuses a script that names /tmp, since a temporary file made there would escape the
// tree the tests watch; every evidence test calls it before its first run, so no mode of such a script runs.
func newEvidenceWorld(t *testing.T, page string) evidenceWorld {
	t.Helper()
	overlaytest.RequireShAndAwk(t)
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(overlaytest.RepoRoot(t), "skills", "_shared", "evidence")
	for _, name := range []string{"evidence.sh", "evidence-rules.md", "LICENSE", "PROVENANCE.md"} {
		data, err := os.ReadFile(filepath.Join(src, name))
		if err != nil {
			t.Fatal(err)
		}
		if name == "evidence.sh" && strings.Contains(string(data), "/tmp") {
			t.Fatalf("%s names /tmp: a write there is outside the tree the evidence tests watch, so no mode is run", filepath.Join(src, name))
		}
		writeEvidenceFile(t, filepath.Join(tmp, "skills", "_shared", "evidence", name), string(data))
	}
	w := evidenceWorld{t: t, tmp: tmp, repo: filepath.Join(tmp, "repo"), script: filepath.Join(tmp, "skills", "_shared", "evidence", "evidence.sh")}
	for _, dir := range []string{w.repo, filepath.Join(tmp, "tmpdir")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if page != "" {
		writeEvidenceFile(t, filepath.Join(w.repo, "docs", "wiki", "evidence.md"), page)
	}
	return w
}

// run runs the script under sh from dir with a home of its own, a TMPDIR inside the watched tree and
// CLAUDE_PROJECT_DIR unset, plus extra.
func (w evidenceWorld) run(dir string, extra []string, args ...string) overlaytest.Result {
	w.t.Helper()
	cmd := exec.Command("sh", append([]string{w.script}, args...)...)
	cmd.Dir = dir
	cmd.Env = append([]string{"HOME=" + w.tmp, "PATH=" + os.Getenv("PATH"), "TMPDIR=" + filepath.Join(w.tmp, "tmpdir")}, extra...)
	return overlaytest.Capture(w.t, cmd)
}

func (w evidenceWorld) inRepo(args ...string) overlaytest.Result { return w.run(w.repo, nil, args...) }

func wantResult(t *testing.T, what string, got overlaytest.Result, exit int, stdout, stderr string) {
	t.Helper()
	if got.Exit != exit || got.Stdout != stdout || got.Stderr != stderr {
		t.Errorf("%s:\n exit %d, want %d\n stdout %q, want %q\n stderr %q, want %q", what, got.Exit, exit, got.Stdout, stdout, got.Stderr, stderr)
	}
}

// B1: a valid page passes, and so does the shipped evidence-rules.md that check reads with it.
func TestEvidenceStoreCheckPassesAValidPage(t *testing.T) {
	w := newEvidenceWorld(t, evidencePage(stdKinds, stdEvidence))
	wantResult(t, "check", w.inRepo("check"), 0, "evidence check passed: docs/wiki/evidence.md\n", "")
}

// B2: every problem of a malformed page is one FAIL line, and the run ends 1.
func TestEvidenceStoreCheckReportsEachProblemOfAMalformedPage(t *testing.T) {
	bad := "| ghost-kind | 1 | f/aaaa-1 | 2026-01-01 | a |\n| alpha-kind | 1 | f/bbbb-1 | 2026-02-30 | b |\n| alpha-kind | 0 | f/cccc-1 | 2026-01-02 | c |\n"
	w := newEvidenceWorld(t, evidencePage(stdKinds, bad))
	want := "FAIL evidence.md: evidence of \"alpha-kind\" has date \"2026-02-30\", not YYYY-MM-DD\n" +
		"FAIL evidence.md: evidence of \"alpha-kind\" has level \"0\", not a positive integer\n" +
		"FAIL evidence.md: evidence of kind \"ghost-kind\" has no row in Kinds\n"
	wantResult(t, "check", w.inRepo("check"), 1, want, "")
}

// B3: no page is not a failure; check says so.
func TestEvidenceStoreCheckWithNoPageSaysSo(t *testing.T) {
	w := newEvidenceWorld(t, "")
	wantResult(t, "check", w.inRepo("check"), 0, "there is no evidence page at docs/wiki/evidence.md\n", "")
}

// B4: a kind is ready at three distinct reviews; the same review on two rows counts once; no page, no output.
func TestEvidenceStoreCountSaysWhichKindsAreReady(t *testing.T) {
	w := newEvidenceWorld(t, evidencePage(stdKinds, stdEvidence))
	want := "ready to consolidate: alpha-kind (level 1): 3 distinct reviews, last 2026-02-10\n" +
		"not ready: beta-kind (level 2): 1 distinct reviews, last 2026-03-01\n" +
		"not ready: gamma-kind (level 3): 2 distinct reviews, last 2026-03-09\n"
	wantResult(t, "count", w.inRepo("count"), 0, want, "")
	wantResult(t, "count with no page", newEvidenceWorld(t, "").inRepo("count"), 0, "", "")
}

// B5: index lists the level-2+ kinds of the page, then the 19 general kinds, one line each; a level-1 kind is absent.
func TestEvidenceStoreIndexListsTheRulesOfLevelTwoAndUp(t *testing.T) {
	w := newEvidenceWorld(t, evidencePage(stdKinds, stdEvidence))
	got := w.inRepo("index")
	lines := strings.Split(strings.TrimSuffix(got.Stdout, "\n"), "\n")
	if got.Exit != 0 || got.Stderr != "" || len(lines) != 2+19 {
		t.Fatalf("index: exit %d, stderr %q, %d lines, want 0, empty, 21:\n%s", got.Exit, got.Stderr, len(lines), got.Stdout)
	}
	if lines[0] != "- beta-kind (level 2): beta tasks" || lines[1] != "- gamma-kind (level 3): gamma tasks" {
		t.Errorf("the page kinds come first, got %q and %q", lines[0], lines[1])
	}
	if !strings.HasPrefix(lines[2], "- change-contradicts-rule-of-another-page (level 2): ") {
		t.Errorf("the general kinds follow in file order, got %q", lines[2])
	}
	for _, l := range lines {
		if !strings.HasPrefix(l, "- ") || !strings.Contains(l, " (level ") || strings.Contains(l, "alpha-kind") {
			t.Errorf("index line %q is not %q or names the level-1 kind", l, "- <kind> (level N): <applies when>")
		}
	}
}

// B6: rule prints the five lines of a kind from the page, else from evidence-rules.md; an unknown kind is an error.
func TestEvidenceStoreRulePrintsAKindOrFails(t *testing.T) {
	w := newEvidenceWorld(t, evidencePage(stdKinds, stdEvidence))
	wantResult(t, "rule of a page kind", w.inRepo("rule", "beta-kind"), 0,
		"kind: beta-kind\nlevel: 2\nrule: Do beta.\napplies when: beta tasks\nsource: 3 reviews\n", "")
	general := w.inRepo("rule", "missing-test")
	if lines := strings.Split(strings.TrimSuffix(general.Stdout, "\n"), "\n"); general.Exit != 0 || general.Stderr != "" || len(lines) != 5 ||
		lines[0] != "kind: missing-test" || lines[1] != "level: 3" || !strings.HasPrefix(lines[2], "rule: Give every guard") {
		t.Errorf("rule of a general kind: exit %d, stderr %q, stdout %q", general.Exit, general.Stderr, general.Stdout)
	}
	wantResult(t, "rule of an unknown kind", w.inRepo("rule", "no-such-kind"), 1, "",
		"evidence.sh: no kind \"no-such-kind\" in docs/wiki/evidence.md or "+filepath.Join(w.tmp, "skills", "_shared", "evidence", "evidence-rules.md")+"\n")
}

// B7: the wiki directory is the argument, else $CLAUDE_PROJECT_DIR/docs/wiki, else docs/wiki.
func TestEvidenceStoreReadsTheWikiDirectoryFromArgumentThenEnvironmentThenWorkingDirectory(t *testing.T) {
	w := newEvidenceWorld(t, onePage("from-default"))
	writeEvidenceFile(t, filepath.Join(w.tmp, "other", "wiki", "evidence.md"), onePage("from-arg"))
	writeEvidenceFile(t, filepath.Join(w.tmp, "proj", "docs", "wiki", "evidence.md"), onePage("from-env"))
	elsewhere := filepath.Join(w.tmp, "elsewhere")
	if err := os.MkdirAll(elsewhere, 0o755); err != nil {
		t.Fatal(err)
	}
	env := []string{"CLAUDE_PROJECT_DIR=" + filepath.Join(w.tmp, "proj")}
	line := func(kind string) string {
		return "not ready: " + kind + " (level 1): 1 distinct reviews, last 2026-01-01\n"
	}

	wantResult(t, "the argument wins over the variable and the directory", w.run(w.repo, env, "count", filepath.Join(w.tmp, "other", "wiki")), 0, line("from-arg"), "")
	wantResult(t, "the variable, from another directory", w.run(elsewhere, env, "count"), 0, line("from-env"), "")
	wantResult(t, "neither: docs/wiki of the working directory", w.inRepo("count"), 0, line("from-default"), "")

	// An argument that starts with a dash is a path, not an option of dirname or basename.
	writeEvidenceFile(t, filepath.Join(w.repo, "-w", "evidence.md"), onePage("from-dash"))
	wantResult(t, "an argument that starts with a dash", w.inRepo("check", "-w"), 0, "evidence check passed: -w/evidence.md\n", "")
}

// B8: a call the script cannot read exits 2 with the usage on stderr and nothing on stdout.
func TestEvidenceStoreUsageErrorsExitTwo(t *testing.T) {
	w := newEvidenceWorld(t, evidencePage(stdKinds, stdEvidence))
	for name, args := range map[string][]string{"no mode": nil, "an unknown mode": {"bogus"}, "rule with no kind": {"rule"}, "reviewed with no identifier": {"reviewed"},
		"reviewed with a blank identifier": {"reviewed", " "}, "reviewed with a leading space": {"reviewed", " f/aaaa-1"}, "reviewed with a trailing tab": {"reviewed", "f/aaaa-1\t"}} {
		wantResult(t, name, w.inRepo(args...), 2, "", evidenceUsage)
	}
}

// B11: with no awk on PATH every mode but a usage error says so on stderr and exits 3, so a caller never
// reads a missing awk as "no rules". PATH is an empty directory; sh is found by the test process, not by
// that PATH, and exec keeps the last value of a duplicated key, so extra's PATH wins.
func TestEvidenceStoreWithoutAwkSaysSoAndExitsThree(t *testing.T) {
	w := newEvidenceWorld(t, evidencePage(stdKinds, stdEvidence))
	noawk := filepath.Join(w.tmp, "noawk")
	if err := os.MkdirAll(noawk, 0o755); err != nil {
		t.Fatal(err)
	}
	env := []string{"PATH=" + noawk}
	for _, args := range [][]string{{"check"}, {"count"}, {"rules"}, {"index"}, {"rule", "beta-kind"}, {"reviewed", "f/aaaa-1"}} {
		wantResult(t, args[0]+" with no awk", w.run(w.repo, env, args...), 3, "",
			"evidence.sh: awk is not on PATH; "+args[0]+" cannot run\n")
	}
	wantResult(t, "a usage error with no awk", w.run(w.repo, env, "bogus"), 2, "", evidenceUsage)
}

// B12: reviewed prints nothing and answers by its exit: 0 when an Evidence row's Review cell is the identifier
// exactly (trimmed, "\|" read as "|"), 1 when none or when there is no page. The Review cell alone counts: the
// same text in a Kinds Source cell, or as the start of a longer identifier, does not.
func TestEvidenceStoreReviewedAnswersByExitWhetherAnEvidenceRowHoldsTheReview(t *testing.T) {
	kinds := stdKinds + "| source-kind | 1 | | | f/src-1 | project |\n"
	evidence := stdEvidence + "| alpha-kind | 1 | f/xxxx-10 | 2026-04-01 | prefix |\n| alpha-kind | 1 |  f/spaced-1  | 2026-04-02 | spaced |\n| alpha-kind | 1 | f/a\\|b-1 | 2026-04-03 | pipe |\n| alpha-kind | 1 | f/a\\b-1 | 2026-04-04 | backslash |\n"
	w := newEvidenceWorld(t, evidencePage(kinds, evidence))
	for _, c := range []struct {
		what, id string
		exit     int
	}{
		{"a review of a row", "f/aaaa-1", 0},
		{"a review held by two rows", "f/dddd-1", 0},
		{"a review no row holds", "f/zzzz-1", 1},
		{"an identifier that is the start of a row's", "f/xxxx-1", 1},
		{"a row's identifier with a longer one asked", "f/aaaa-10", 1},
		{"an identifier that is only in a Kinds Source cell", "f/src-1", 1},
		{"a cell with spaces around it", "f/spaced-1", 0},
		{"an escaped pipe in the cell", "f/a|b-1", 0},
		{"a backslash in the cell, read as the bytes given", `f/a\b-1`, 0},
	} {
		wantResult(t, c.what, w.inRepo("reviewed", c.id), c.exit, "", "")
	}

	writeEvidenceFile(t, filepath.Join(w.tmp, "other", "wiki", "evidence.md"), onePage("from-arg"))
	wantResult(t, "the wiki directory argument", w.inRepo("reviewed", "f/aaaa-1", filepath.Join(w.tmp, "other", "wiki")), 0, "", "")
	wantResult(t, "the wiki directory argument, another page", w.inRepo("reviewed", "f/bbbb-1", filepath.Join(w.tmp, "other", "wiki")), 1, "", "")
	wantResult(t, "no page", newEvidenceWorld(t, "").inRepo("reviewed", "f/aaaa-1"), 1, "", "")
	empty := newEvidenceWorld(t, "")
	if err := os.MkdirAll(filepath.Join(empty.repo, "docs", "wiki"), 0o755); err != nil {
		t.Fatal(err)
	}
	wantResult(t, "an empty wiki directory", empty.inRepo("reviewed", "f/aaaa-1"), 1, "", "")
	wantResult(t, "a wiki argument that starts with a dash and holds nothing", empty.inRepo("reviewed", "f/aaaa-1", "-w"), 1, "", "")

	// A page that fails a check of check answers 1 even when a row holds the identifier (T4f, L60 choice 2). The
	// first five pages hold a row with f/aaaa-1, so only the check makes them answer 1; the last two have no
	// Evidence table, so they hold no such row and cover the page checks (they answer 1 with or without the check).
	const kind = "| alpha-kind | 1 | | | | project |\n"
	for name, page := range map[string]string{
		"the matching row at level 0":           evidencePage(kind, "| alpha-kind | 0 | f/aaaa-1 | 2026-01-01 | x |\n"),
		"the matching row with date 2026/01/01": evidencePage(kind, "| alpha-kind | 1 | f/aaaa-1 | 2026/01/01 | x |\n"),
		"the matching row's kind not in Kinds":  evidencePage(kind, "| ghost-kind | 1 | f/aaaa-1 | 2026-01-01 | x |\n"),
		"a row after the Evidence table ended":  evidencePage(kind, "| alpha-kind | 1 | f/aaaa-1 | 2026-01-01 | x |\n\n| alpha-kind | 1 | f/bbbb-1 | 2026-01-02 | y |\n"),
		"no # Evidence title":                   strings.TrimPrefix(evidencePage(kind, "| alpha-kind | 1 | f/aaaa-1 | 2026-01-01 | x |\n"), "# Evidence\n\n"),
		"no ## Evidence heading":                kindsHead + kind,
		"the ## Evidence heading and no table":  kindsHead + kind + "\n## Evidence\n\nnothing yet\n",
	} {
		wantResult(t, name, newEvidenceWorld(t, page).inRepo("reviewed", "f/aaaa-1"), 1, "", "")
	}

	// The wiki directory comes from CLAUDE_PROJECT_DIR when there is no argument.
	writeEvidenceFile(t, filepath.Join(w.tmp, "proj", "docs", "wiki", "evidence.md"), onePage("from-env"))
	elsewhere := filepath.Join(w.tmp, "elsewhere")
	if err := os.MkdirAll(elsewhere, 0o755); err != nil {
		t.Fatal(err)
	}
	wantResult(t, "the variable, from another directory", w.run(elsewhere, []string{"CLAUDE_PROJECT_DIR=" + filepath.Join(w.tmp, "proj")}, "reviewed", "f/aaaa-1"), 0, "", "")
}

// B13: reviewed never answers for a page it cannot read: it exits 4 with one line on stderr, and 2 stays the
// usage error. Anything at the page's path that is not a readable regular file, or a directory on the way that
// cannot be searched, stops before awk runs; an awk that fails, or gives no answer, on a readable page stops
// after, with awk's own stderr dropped (T4f, L60 choice 1; T4g, L62 choices 1 and 2).
func TestEvidenceStoreReviewedCannotAnswerForAPageItCannotRead(t *testing.T) {
	const cannotRead = "evidence.sh: cannot read docs/wiki/evidence.md; reviewed cannot answer\n"
	// stubAwk puts an awk that runs body first on PATH and returns the environment that says so.
	stubAwk := func(w evidenceWorld, body string) []string {
		stub := filepath.Join(w.tmp, "stubawk")
		writeEvidenceFile(t, filepath.Join(stub, "awk"), "#!/bin/sh\n"+body)
		if err := os.Chmod(filepath.Join(stub, "awk"), 0o755); err != nil {
			t.Fatal(err)
		}
		return []string{"PATH=" + stub + string(os.PathListSeparator) + os.Getenv("PATH")}
	}
	t.Run("a page that cannot be read", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root reads a page of mode 000")
		}
		w := newEvidenceWorld(t, "# Evidence\n")
		page := filepath.Join(w.repo, "docs", "wiki", "evidence.md")
		if err := os.Chmod(page, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(page, 0o644) })
		wantResult(t, "reviewed", w.inRepo("reviewed", "f/aaaa-1"), 4, "", cannotRead)
	})
	t.Run("a page path that is a directory", func(t *testing.T) {
		w := newEvidenceWorld(t, "")
		if err := os.MkdirAll(filepath.Join(w.repo, "docs", "wiki", "evidence.md"), 0o755); err != nil {
			t.Fatal(err)
		}
		wantResult(t, "reviewed", w.inRepo("reviewed", "f/aaaa-1"), 4, "", cannotRead)
	})
	t.Run("a page path that is a symbolic link to a missing file", func(t *testing.T) {
		w := newEvidenceWorld(t, "")
		if err := os.MkdirAll(filepath.Join(w.repo, "docs", "wiki"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(w.tmp, "missing.md"), filepath.Join(w.repo, "docs", "wiki", "evidence.md")); err != nil {
			t.Fatal(err)
		}
		wantResult(t, "reviewed", w.inRepo("reviewed", "f/aaaa-1"), 4, "", cannotRead)
	})
	t.Run("a wiki directory that cannot be searched", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root searches a directory of mode 600")
		}
		w := newEvidenceWorld(t, onePage("alpha-kind"))
		wiki := filepath.Join(w.repo, "docs", "wiki")
		if err := os.Chmod(wiki, 0o600); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(wiki, 0o755) })
		wantResult(t, "reviewed", w.inRepo("reviewed", "f/aaaa-1"), 4, "", cannotRead)
	})
	t.Run("an awk that fails on the page", func(t *testing.T) {
		w := newEvidenceWorld(t, onePage("alpha-kind"))
		env := stubAwk(w, "echo 'awk: fatal: simulated' >&2\nexit 2\n")
		wantResult(t, "reviewed", w.run(w.repo, env, "reviewed", "f/aaaa-1"), 4, "", "evidence.sh: awk failed on docs/wiki/evidence.md (exit 2); reviewed cannot answer\n")
	})
	t.Run("an awk that exits 1", func(t *testing.T) {
		w := newEvidenceWorld(t, onePage("alpha-kind"))
		env := stubAwk(w, "echo 'awk: fatal: simulated' >&2\nexit 1\n")
		wantResult(t, "reviewed", w.run(w.repo, env, "reviewed", "f/aaaa-1"), 4, "", "evidence.sh: awk failed on docs/wiki/evidence.md (exit 1); reviewed cannot answer\n")
	})
	t.Run("an awk that gives no answer", func(t *testing.T) {
		w := newEvidenceWorld(t, onePage("alpha-kind"))
		env := stubAwk(w, "exit 0\n")
		wantResult(t, "reviewed", w.run(w.repo, env, "reviewed", "f/aaaa-1"), 4, "", "evidence.sh: awk gave no answer on docs/wiki/evidence.md; reviewed cannot answer\n")
	})
	t.Run("a wiki argument that starts with a dash and cannot be searched", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root searches a directory of mode 600")
		}
		w := newEvidenceWorld(t, "")
		dash := filepath.Join(w.repo, "-w")
		if err := os.Mkdir(dash, 0o600); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(dash, 0o755) })
		wantResult(t, "reviewed", w.inRepo("reviewed", "f/aaaa-1", "-w"), 4, "", "evidence.sh: cannot read -w/evidence.md; reviewed cannot answer\n")
	})
	t.Run("a regular file where the wiki directory should be", func(t *testing.T) {
		// Mode 755 so that [ -x ] alone holds; only [ -d ] refuses it. Root is not skipped: -d is false for root too.
		w := newEvidenceWorld(t, "")
		if err := os.MkdirAll(filepath.Join(w.repo, "docs"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(w.repo, "docs", "wiki"), []byte("not a directory\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		wantResult(t, "reviewed", w.inRepo("reviewed", "f/aaaa-1"), 4, "", cannotRead)
	})
}

// fileTree is every entry under dir but dir itself, by path relative to dir: a regular file as "file "
// and its content, a directory as "dir", a symbolic link as "link " and its target, anything else as
// "other " and its type.
func fileTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || path == dir {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		switch {
		case d.IsDir():
			tree[rel] = "dir"
		case d.Type()&fs.ModeSymlink != 0:
			target, err := os.Readlink(path)
			tree[rel] = "link " + target
			return err
		case d.Type().IsRegular():
			data, err := os.ReadFile(path)
			tree[rel] = "file " + string(data)
			return err
		default:
			tree[rel] = "other " + d.Type().String()
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

// B9: no mode writes a file, and the repository needs no wiki file but the page (S4).
func TestEvidenceStoreWritesNothingAndNeedsNoOtherWikiFile(t *testing.T) {
	w := newEvidenceWorld(t, evidencePage(stdKinds, stdEvidence))
	want := map[string]string{
		"docs":                        "dir",
		filepath.Join("docs", "wiki"): "dir",
		filepath.Join("docs", "wiki", "evidence.md"): "file " + evidencePage(stdKinds, stdEvidence),
	}
	if got := fileTree(t, w.repo); !reflect.DeepEqual(got, want) {
		t.Fatalf("the repository holds %v, want only docs/wiki/evidence.md", got)
	}
	before := fileTree(t, w.tmp)
	for _, args := range [][]string{{"check"}, {"count"}, {"rules"}, {"index"}, {"rule", "beta-kind"}, {"rule", "no-such-kind"}, {"reviewed", "f/aaaa-1"}, {"reviewed", "f/none-1"}, {"bogus"}} {
		w.inRepo(args...)
		if after := fileTree(t, w.tmp); !reflect.DeepEqual(after, before) {
			t.Fatalf("after %v the tree under %s changed:\n before %v\n after  %v", args, w.tmp, before, after)
		}
	}
}
