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

	evidenceUsage = "usage: sh evidence.sh check|count|rules|index [wiki directory]\n       sh evidence.sh rule <kind> [wiki directory]\n"
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
		writeEvidenceFile(t, filepath.Join(tmp, "skills", "_shared", "evidence", name), string(data))
	}
	w := evidenceWorld{t: t, tmp: tmp, repo: filepath.Join(tmp, "repo"), script: filepath.Join(tmp, "skills", "_shared", "evidence", "evidence.sh")}
	if err := os.MkdirAll(w.repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if page != "" {
		writeEvidenceFile(t, filepath.Join(w.repo, "docs", "wiki", "evidence.md"), page)
	}
	return w
}

// run runs the script under sh from dir with a home of its own and CLAUDE_PROJECT_DIR unset, plus extra.
func (w evidenceWorld) run(dir string, extra []string, args ...string) overlaytest.Result {
	w.t.Helper()
	cmd := exec.Command("sh", append([]string{w.script}, args...)...)
	cmd.Dir = dir
	cmd.Env = append([]string{"HOME=" + w.tmp, "PATH=" + os.Getenv("PATH")}, extra...)
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
}

// B8: a call the script cannot read exits 2 with the usage on stderr and nothing on stdout.
func TestEvidenceStoreUsageErrorsExitTwo(t *testing.T) {
	w := newEvidenceWorld(t, evidencePage(stdKinds, stdEvidence))
	for name, args := range map[string][]string{"no mode": nil, "an unknown mode": {"bogus"}, "rule with no kind": {"rule"}} {
		wantResult(t, name, w.inRepo(args...), 2, "", evidenceUsage)
	}
}

// fileTree is every file under dir with its content, by path relative to dir.
func fileTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		rel, _ := filepath.Rel(dir, path)
		tree[rel] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

// B9: no mode writes a file, and the repository needs no wiki file but the page (S4).
func TestEvidenceStoreWritesNothingAndNeedsNoOtherWikiFile(t *testing.T) {
	w := newEvidenceWorld(t, evidencePage(stdKinds, stdEvidence))
	if got, want := fileTree(t, w.repo), map[string]string{filepath.Join("docs", "wiki", "evidence.md"): evidencePage(stdKinds, stdEvidence)}; !reflect.DeepEqual(got, want) {
		t.Fatalf("the repository holds %v, want only docs/wiki/evidence.md", got)
	}
	before := fileTree(t, w.tmp)
	for _, args := range [][]string{{"check"}, {"count"}, {"rules"}, {"index"}, {"rule", "beta-kind"}, {"rule", "no-such-kind"}, {"bogus"}} {
		w.inRepo(args...)
		if after := fileTree(t, w.tmp); !reflect.DeepEqual(after, before) {
			t.Fatalf("after %v the tree under %s changed:\n before %v\n after  %v", args, w.tmp, before, after)
		}
	}
}
