package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// The help of `roles append` says what the verb holds a review-report to, in the engine and in the wrapper, and
// the two say the same: both texts wrap at their own width, so their words are compared with the line breaks
// taken out.
func TestUsageDocumentsTheEvidenceRolesAppendChecks(t *testing.T) {
	wrapper, err := os.ReadFile("../../bin/lib/usage.sh")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	usage(&out)
	engineText, wrapperText := strings.Join(strings.Fields(out.String()), " "), strings.Join(strings.Fields(string(wrapper)), " ")
	for _, pair := range []struct{ engine, inWrapper string }{
		{"engine roles append --project <id> --goal <id> --chain <id> --stdin", "append --project ID --goal ID --chain ID --stdin"},
		{"A completed review-report record is held to the files 'review record' and 'review verify' stored", "a completed review-report record is held to the files review record and verify stored"},
		{"repository of the working directory", "repository of the working directory"},
		{"the <base>..<head> range and the diff_sha256 of the stored result", "the range and the diff_sha256 of the stored result"},
		{"is exit 1, the message names the evidence kind and its ref, and nothing is stored", "is exit 1, and nothing is stored"},
	} {
		if !strings.Contains(engineText, pair.engine) {
			t.Errorf("the engine usage lacks %q", pair.engine)
		}
		if !strings.Contains(wrapperText, pair.inWrapper) {
			t.Errorf("bin/lib/usage.sh lacks %q, which the engine usage says as %q", pair.inWrapper, pair.engine)
		}
	}
}
