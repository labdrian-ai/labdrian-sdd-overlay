package reviewreceipt_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewreceipt"
)

// The change name and the name of a receipt file are each joined into a path by whatever
// keeps the receipts, so each must be one path component and nothing else: the domain
// refuses anything else before an adapter is asked anything.
func TestCheckPathComponent(t *testing.T) {
	for _, tc := range []struct {
		name   string
		value  string
		reason string // empty: accepted
	}{
		{"a change name", "phase-9-batch-8", ""},
		{"a receipt file name", "review-0a1b2c.review-state.json", ""},
		{"a name with dots inside", "v1.2.3", ""},
		{"a name that starts with a dot", ".hidden", ""},
		{"a name with a space inside", "two words", ""},
		{"empty", "", "it is empty"},
		{"a single dot", ".", "it is a dot segment"},
		{"two dots", "..", "it is a dot segment"},
		{"a slash", "a/b", "it holds a path separator"},
		{"a leading slash", "/etc", "it holds a path separator"},
		{"a trailing slash", "a/", "it holds a path separator"},
		{"a traversal", "../x", "it holds a path separator"},
		{"a backslash", `a\b`, "it holds a path separator"},
		{"a drive-relative traversal", `..\x`, "it holds a path separator"},
		{"a leading space", " x", "it has leading or trailing white space"},
		{"a trailing space", "x ", "it has leading or trailing white space"},
		{"a trailing newline", "x\n", "it has leading or trailing white space"},
		{"a leading tab", "\tx", "it has leading or trailing white space"},
		{"only white space", "   ", "it has leading or trailing white space"},
		{"a non-breaking space at the end", "x ", "it has leading or trailing white space"},
		{"a NUL byte", "a\x00b", "it holds a control character"},
		{"a newline inside", "a\nb", "it holds a control character"},
		{"an escape inside", "a\x1bb", "it holds a control character"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := reviewreceipt.CheckPathComponent("change name", tc.value)
			if tc.reason == "" {
				if err != nil {
					t.Fatalf("CheckPathComponent(%q) = %v, want it accepted", tc.value, err)
				}
				return
			}
			var unsafe *reviewreceipt.UnsafeNameError
			if !errors.As(err, &unsafe) {
				t.Fatalf("CheckPathComponent(%q) = %v, want an *UnsafeNameError", tc.value, err)
			}
			if unsafe.Kind != "change name" || unsafe.Name != tc.value || unsafe.Reason != tc.reason {
				t.Errorf("CheckPathComponent(%q) = %+v, want kind %q, name %q, reason %q", tc.value, *unsafe, "change name", tc.value, tc.reason)
			}
		})
	}
}

// The refusal names what was refused and why, the name quoted so that white space and
// control characters can be read.
func TestAnUnsafeNameErrorNamesTheNameAndTheReason(t *testing.T) {
	err := reviewreceipt.CheckPathComponent("receipt file name", "../escape.json")
	want := `reviewreceipt: receipt file name "../escape.json" is not a safe path component: it holds a path separator`
	if err == nil || err.Error() != want {
		t.Errorf("error = %v, want %q", err, want)
	}
	if err := reviewreceipt.CheckPathComponent("change name", "x\n"); err == nil || !strings.Contains(err.Error(), `"x\n"`) {
		t.Errorf("error = %v, want the name quoted so the newline is visible", err)
	}
}
