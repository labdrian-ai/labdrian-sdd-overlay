package main

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// routedReviewVerbs returns the verbs runReview routes, read from the case expressions of its switch on args[0], in
// source order. A verb that is a string literal is its unquoted value, the one that is reviewStopVerb is that
// constant; any other expression stops the test, because the check would not know what it routes.
func routedReviewVerbs(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "review.go", nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse review.go: %v", err)
	}
	var verbs []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "runReview" {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			sw, ok := n.(*ast.SwitchStmt)
			if !ok {
				return true
			}
			tag, ok := sw.Tag.(*ast.IndexExpr)
			if !ok {
				return true
			}
			if x, ok := tag.X.(*ast.Ident); !ok || x.Name != "args" {
				return true
			}
			for _, stmt := range sw.Body.List {
				clause := stmt.(*ast.CaseClause)
				for _, expr := range clause.List {
					switch e := expr.(type) {
					case *ast.BasicLit:
						verb, err := strconv.Unquote(e.Value)
						if err != nil {
							t.Fatalf("runReview routes %s, which is not a string", e.Value)
						}
						verbs = append(verbs, verb)
					case *ast.Ident:
						if e.Name != "reviewStopVerb" {
							t.Fatalf("runReview routes the identifier %s, which this test does not know", e.Name)
						}
						verbs = append(verbs, reviewStopVerb)
					default:
						t.Fatalf("runReview routes an expression of type %T this test does not know", expr)
					}
				}
			}
			return false
		})
	}
	if len(verbs) < 13 {
		t.Fatalf("found %d routed review verbs (%v), fewer than 13: the parse found nothing it should", len(verbs), verbs)
	}
	return verbs
}

// The review verbs are documented in five places: the engine usage, the wrapper help, the README, the package comment
// of review.go and the CHANGELOG. A verb runReview routes that one of them lacks is a known error class (the
// wrapper help lacked review ci), so every verb the switch routes is looked up in each of them.
func TestEveryRoutedReviewVerbIsDocumentedWhereTheOtherVerbsAre(t *testing.T) {
	verbs := routedReviewVerbs(t)

	var out bytes.Buffer
	usage(&out)
	engineUsage := out.String()

	wrapper := readRepositoryFile(t, "bin/lib/usage.sh")
	wrapperStart := strings.Index(wrapper, "\n  review <verb>")
	wrapperEnd := strings.Index(wrapper, "\n  memory <verb>")
	if wrapperStart < 0 || wrapperEnd < wrapperStart {
		t.Fatalf("bin/lib/usage.sh: the review section, from the line %q to the line %q, is not there", "  review <verb>", "  memory <verb>")
	}
	wrapperReview := wrapper[wrapperStart:wrapperEnd]
	column := -1
	for _, line := range strings.Split(wrapperReview, "\n") {
		if strings.Contains(line, "bind --base REV") {
			column = len(line) - len(strings.TrimLeft(line, " "))
			break
		}
	}
	if column < 0 {
		t.Fatal("bin/lib/usage.sh: the line with bind --base REV, which gives the column of the verbs, is not there")
	}

	readme := readRepositoryFile(t, "README.md")
	readmeStart := strings.Index(readme, "overlay review <verb>\n")
	readmeEnd := strings.Index(readme, "\noverlay memory <verb>\n")
	if readmeStart < 0 || readmeEnd < readmeStart {
		t.Fatal("README.md: the review section, from overlay review <verb> to overlay memory <verb>, is not there")
	}
	readmeReview := readme[readmeStart:readmeEnd]

	source := readRepositoryFile(t, "engine/cmd/review.go")
	importAt := strings.Index(source, "\nimport (")
	if importAt < 0 {
		t.Fatal("engine/cmd/review.go: no import block, so the package comment cannot be cut")
	}
	packageComment := normalizeDocText(strings.ReplaceAll(source[:importAt], "//", ""))

	changelog := readRepositoryFile(t, "CHANGELOG.md")

	for _, verb := range verbs {
		quoted := regexp.QuoteMeta(verb)
		if !regexp.MustCompile(`(?m)^  engine review ` + quoted + `( |$)`).MatchString(engineUsage) {
			t.Errorf("the engine usage has no line for review %s, a verb runReview routes", verb)
		}
		if !regexp.MustCompile(`(?m)^ {` + strconv.Itoa(column) + `}` + quoted + `( |$)`).MatchString(wrapperReview) {
			t.Errorf("bin/lib/usage.sh has no entry for review %s, a verb runReview routes", verb)
		}
		if !regexp.MustCompile(`(?m)^    ` + quoted + `( |$)`).MatchString(readmeReview) {
			t.Errorf("README.md has no entry for review %s, a verb runReview routes", verb)
		}
		if !regexp.MustCompile(`'review ` + quoted + `[ ']`).MatchString(packageComment) {
			t.Errorf("the package comment of engine/cmd/review.go does not mention review %s, a verb runReview routes", verb)
		}
		if !regexp.MustCompile(`review ` + quoted + `([^A-Za-z0-9-]|$)`).MatchString(changelog) {
			t.Errorf("CHANGELOG.md does not mention review %s, a verb runReview routes", verb)
		}
	}
}

// The wrapper help of review ci says the same facts as the engine usage: the signature, what it says, the
// output, the tree it reads, the markers, the refusal and the exits.
func TestWrapperUsageOfReviewCISaysWhatTheEngineUsageSays(t *testing.T) {
	var out bytes.Buffer
	usage(&out)
	flatEngine := strings.Join(strings.Fields(out.String()), " ")
	flat := docSection(t, readRepositoryFile(t, "bin/lib/usage.sh"), "ci --landing", "findings --feature")
	for _, pair := range []struct{ engine, inWrapper string }{
		{"engine review ci --landing <rev> --head <rev> [--cwd <repo>]", "ci --landing REV --head REV [--cwd REPO]"},
		{"says whether the repository has no CI for the commit a delivery landed", "say whether the repository has no CI for the commit a delivery landed"},
		{"starts 'CI: none configured' and names the full landing and head ids", "starts CI: none configured and names the full landing and head ids"},
		{"the tree of the landing commit (never the working tree) holds no CI marker", "the tree of the landing commit (never the working tree) holds no CI marker"},
		{"appveyor.yml or .appveyor.yml", "appveyor.yml or .appveyor.yml"},
		{"the no-CI closure does not apply", "the no-CI closure does not apply"},
		{"never 'CI: none configured'", "never CI: none configured"},
		{"exit 0 no CI configured, 1 refusal, usage error or any other failure", "exit 0 no CI configured, 1 refusal, usage error or any other failure"},
	} {
		if !strings.Contains(flatEngine, pair.engine) {
			t.Errorf("the engine usage lacks %q", pair.engine)
		}
		if !strings.Contains(flat, pair.inWrapper) {
			t.Errorf("bin/lib/usage.sh lacks %q, which the engine usage says as %q", pair.inWrapper, pair.engine)
		}
	}
}

// The review ci entry of the README says the signature, the line it prints, that the tree is read and not the
// working tree, that the closure does not apply on a refusal, and the exits.
func TestReadmeCIEntrySaysWhatTheEngineUsageSays(t *testing.T) {
	flat := docSection(t, readRepositoryFile(t, "README.md"), "ci --landing <rev>", "findings --feature")
	for _, want := range []string{
		"ci --landing <rev> --head <rev> [--cwd <repo>]",
		"CI: none configured",
		"never the working tree",
		"the no-CI closure does not apply",
		"Exit 0 no CI configured, 1 refusal, usage error or any other failure",
	} {
		if !strings.Contains(flat, want) {
			t.Errorf("the README entry of review ci lacks %q", want)
		}
	}
}

// Regression pin of a review suggestion: the #6179 item says what startsInBase asks, the oldest commit the range
// covers, and not the commit the range ends at.
func TestChangelogItemOf6179SaysWhatTheChainChecks(t *testing.T) {
	item := docSection(t, readRepositoryFile(t, "CHANGELOG.md"), "known error #6179", "\n")
	if want := "the oldest commit the range covers, the one whose first parent is that start"; !strings.Contains(item, want) {
		t.Errorf("the #6179 item of CHANGELOG.md lacks %q", want)
	}
	if stale := "the commit the range ends at"; strings.Contains(item, stale) {
		t.Errorf("the #6179 item of CHANGELOG.md still says %q", stale)
	}
}

// Regression pin of a review suggestion: the doc comment of topicMadeOnAMainMerge names the four commits it returns.
func TestTopicMadeOnAMainMergeSaysTheFourCommitsItReturns(t *testing.T) {
	section := docSection(t, readRepositoryFile(t, "engine/cmd/review_check_chain_test.go"),
		"// topicMadeOnAMainMerge makes", "func (w *reviewWorld) topicMadeOnAMainMerge")
	comment := strings.ReplaceAll(section, "// ", "")
	want := "It returns M, the first commit of topic, the merge of main into topic and the last commit of topic."
	if !strings.Contains(comment, want) {
		t.Errorf("the doc comment of topicMadeOnAMainMerge lacks %q; it reads %q", want, comment)
	}
}
