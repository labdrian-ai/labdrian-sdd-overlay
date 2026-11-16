package runtime_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
)

func TestMarkedBlockInACrlfFileIsWrittenWithCrlfAndSeparatedWithIt(t *testing.T) {
	got, change, err := testBlock.Apply("A\r\n", "line one\nline two\n")
	want := "A\r\n\r\n<!-- t:x -->\r\nline one\r\nline two\r\n<!-- /t:x -->\r\n"
	if err != nil || change != engineRuntime.BlockAdded || got != want {
		t.Errorf("Apply = %q %v %v, want %q", got, change, err, want)
	}
	if state, err := testBlock.State(got, "line one\nline two\n"); err != nil || state != engineRuntime.BlockCurrent {
		t.Errorf("State = %v %v, want current", state, err)
	}
}

func TestMarkedBlockMarkersWithTrailingSpacesOrTabsAreFound(t *testing.T) {
	content := "A\n<!-- t:x -->  \nold\n<!-- /t:x -->\t \nB\n"
	got, change, err := testBlock.Apply(content, "body line")
	if want := "A\n" + testBlockText + "\nB\n"; err != nil || change != engineRuntime.BlockReplaced || got != want {
		t.Errorf("Apply = %q %v %v, want %q", got, change, err, want)
	}
}

func TestMarkedBlockBrokenMarkersErrorNamesTheLines(t *testing.T) {
	for name, tc := range map[string]struct{ content, want string }{
		"open without close": {"A\n<!-- t:x -->\nB\n", "open: line 2; close: none"},
		"close without open": {"A\nB\n<!-- /t:x -->\n", "open: none; close: line 3"},
		"two pairs":          {testBlockText + "\nx\n" + testBlockText + "\n", "open: lines 1, 5; close: lines 3, 7"},
		"close first":        {"<!-- /t:x -->\n<!-- t:x -->\n", "the close line comes first"},
	} {
		_, _, err := testBlock.Apply(tc.content, "x")
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error = %v, want one saying %q", name, err, tc.want)
		}
	}
}

func TestMarkedBlockFilesThroughADanglingSymbolicLinkAreRefused(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "CLAUDE.md")
	if err := os.Symlink("missing.md", link); err != nil {
		t.Fatal(err)
	}
	if _, err := testBlock.WriteFile(link, "body line"); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Errorf("WriteFile error = %v", err)
	}
	if _, err := testBlock.RemoveFile(link); err == nil {
		t.Error("RemoveFile did not refuse")
	}
	if _, err := testBlock.StateOfFile(link, "x"); err == nil {
		t.Error("StateOfFile did not refuse")
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Error("the link was replaced")
	}
	if _, err := os.Stat(filepath.Join(dir, "missing.md")); !os.IsNotExist(err) {
		t.Error("the file behind the link was created")
	}
}

func TestMarkedBlockFileOfOnlyTheBlockIsKeptEmptyWhenItIsASymbolicLink(t *testing.T) {
	dir := t.TempDir()
	real, link := filepath.Join(dir, "real.md"), filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(real, []byte(testBlockText+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real.md", link); err != nil {
		t.Fatal(err)
	}
	if removed, err := testBlock.RemoveFile(link); err != nil || !removed {
		t.Fatalf("RemoveFile = %v %v", removed, err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 || readFileText(t, real) != "" {
		t.Error("the link or the file behind it was removed")
	}
}

func TestMarkedBlockFileWithOtherTextIsGivenBackAsItWasWhenTheBlockGoes(t *testing.T) {
	for _, original := range []string{"A", "A\n", "A\r\nB\r\n"} {
		path := filepath.Join(t.TempDir(), "CLAUDE.md")
		if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := testBlock.WriteFile(path, "body line"); err != nil {
			t.Fatal(err)
		}
		if removed, err := testBlock.RemoveFile(path); err != nil || !removed || readFileText(t, path) != original {
			t.Errorf("%q: RemoveFile = %v %v, file %q", original, removed, err, readFileText(t, path))
		}
	}
}
