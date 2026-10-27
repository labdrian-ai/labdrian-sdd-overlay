package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// helpFact is one fact a caller acts on, as the help of the engine words it and as the help of the wrapper words it.
type helpFact struct{ fact, engine, inWrapper string }

// requireHelpFacts requires every fact to be stated in the help of the engine and in the help of the wrapper
// (bin/lib/usage.sh), and returns the words of the engine's for a check that is about it alone. The help of the
// wrapper is shorter than the engine's and written by hand, so the two texts are not compared as wholes: each fact is
// a phrase of the one and the phrase that says it in the other, and a fact that no pair names can differ between
// them. Both texts wrap at their own width, so their words are compared with the line breaks taken out.
func requireHelpFacts(t *testing.T, facts []helpFact) (engineText string) {
	t.Helper()
	wrapper, err := os.ReadFile("../../bin/lib/usage.sh")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	usage(&out)
	engineText, wrapperText := strings.Join(strings.Fields(out.String()), " "), strings.Join(strings.Fields(string(wrapper)), " ")
	for _, f := range facts {
		if !strings.Contains(engineText, f.engine) {
			t.Errorf("the engine usage does not state %s: it lacks %q", f.fact, f.engine)
		}
		if !strings.Contains(wrapperText, f.inWrapper) {
			t.Errorf("bin/lib/usage.sh does not state %s: it lacks %q, which the engine usage says as %q", f.fact, f.inWrapper, f.engine)
		}
	}
	return engineText
}

// The help of `roles append` states, in the engine and in the wrapper, what a caller acts on when a record closes a
// review: the signature, what a review-report is held to, and what a refusal is.
func TestUsageDocumentsTheEvidenceRolesAppendChecks(t *testing.T) {
	requireHelpFacts(t, []helpFact{
		{"the signature of the verb",
			"engine roles append --project <id> --goal <id> --chain <id> --stdin", "append --project ID --goal ID --chain ID --stdin"},
		{"what a completed review-report is held to, and where",
			"A completed review-report record is held to the files 'review record' and 'review verify' stored in the repository of the working directory",
			"a completed review-report record is held to the files review record and verify stored in the repository of the working directory"},
		{"what the review-diff is held to",
			"the <base>..<head> range and the diff_sha256 of the stored result", "the range and the diff_sha256 of the stored result"},
		{"what a refusal is",
			"is exit 1, the message names the evidence kind and its ref, and nothing is stored", "is exit 1, and nothing is stored"},
	})
}
