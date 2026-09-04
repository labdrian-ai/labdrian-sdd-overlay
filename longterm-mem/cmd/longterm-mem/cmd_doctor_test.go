package main

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/ops"
)

// countWords maps a check count onto the English word a doc comment would
// spell it with. Only the range a diagnostic suite could plausibly occupy
// is listed; a count outside it fails the test loudly rather than silently
// finding nothing to compare.
var countWords = map[int]string{
	1: "one", 2: "two", 3: "three", 4: "four", 5: "five",
	6: "six", 7: "seven", 8: "eight", 9: "nine", 10: "ten",
}

// funcDocComment returns the doc comment attached to the named top-level
// function in file, as plain text.
func funcDocComment(t *testing.T, file, funcName string) string {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != funcName || fn.Doc == nil {
			continue
		}
		return fn.Doc.Text()
	}
	t.Fatalf("%s has no documented top-level func %s", file, funcName)
	return ""
}

// TestCmdDoctor_DocumentedCheckCountMatchesOpsDoctor derives the number of
// checks ops.Doctor actually runs and holds cmd_doctor.go's own doc
// comment to it.
//
// The comment said "the four read-only diagnostic checks ... reports all
// four checks" while ops.Doctor ran five. A previous repair round reported
// that four-vs-five contradiction as fixed, having changed it only in the
// spec file -- the source comment, in the same lane, kept saying four. A
// prose assertion is the only kind that could have caught that, because
// the command's behaviour was right the whole time: it prints whatever
// Doctor returns.
//
// The count is not hardcoded here. It is read out of a real ops.Doctor
// run, so adding or removing a check makes this test name the new number
// instead of quietly agreeing with a stale one.
func TestCmdDoctor_DocumentedCheckCountMatchesOpsDoctor(t *testing.T) {
	report, err := ops.Doctor(context.Background(), ops.DoctorDeps{
		VaultRoot:           t.TempDir(),
		PrerequisitePresent: func(string) bool { return true },
	}, "doc-count-project")
	if err != nil {
		t.Fatalf("ops.Doctor: %v", err)
	}
	actual := len(report.Checks)
	want, ok := countWords[actual]
	if !ok {
		t.Fatalf("ops.Doctor returned %d checks, outside the range this test can spell", actual)
	}

	doc := funcDocComment(t, "cmd_doctor.go", "cmdDoctor")
	if !numberedChecks(want).MatchString(doc) {
		t.Fatalf("cmdDoctor's doc comment never counts the checks as %q, but ops.Doctor runs %d of them:\n%s", want, actual, doc)
	}
	for n, word := range countWords {
		if n == actual {
			continue
		}
		if numberedChecks(word).MatchString(doc) {
			t.Fatalf("cmdDoctor's doc comment counts the checks as %q while ops.Doctor runs %d (%q):\n%s", word, actual, want, doc)
		}
	}
}

// numberedChecks matches a number word used to COUNT checks -- the word
// itself, then up to three intervening adjectives, then "check"/"checks".
// It deliberately does not match a bare number word: English prose uses
// "one" as a pronoun ("reports each one individually") and "no single
// one's failure"), and a matcher that flagged those would force the
// comment to be written around the test instead of the other way round.
func numberedChecks(word string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(word) + `\b(?:\s+[a-z-]+){0,3}\s+checks?\b`)
}

// TestCmdDoctor_PrintsEveryCheckOpsDoctorReturns is the behavioural half
// the comment is describing: the command must print one line per check,
// for every check Doctor returns, not a fixed list it believes in.
func TestCmdDoctor_PrintsEveryCheckOpsDoctorReturns(t *testing.T) {
	vaultRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LONGTERM_MEM_VAULT", vaultRoot)

	report, err := ops.Doctor(context.Background(), ops.DoctorDeps{
		VaultRoot:           vaultRoot,
		PrerequisitePresent: func(string) bool { return true },
	}, "doctor-print-project")
	if err != nil {
		t.Fatalf("ops.Doctor: %v", err)
	}

	stdout := captureStdout(t, func() {
		run([]string{"doctor", "--project", "doctor-print-project"})
	})
	for _, check := range report.Checks {
		if !strings.Contains(stdout, check.Name) {
			t.Fatalf("doctor output omits the %q check:\n%s", check.Name, stdout)
		}
	}
}
