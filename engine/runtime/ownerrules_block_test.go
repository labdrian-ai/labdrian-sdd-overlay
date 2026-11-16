package runtime_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
)

var testBlock = engineRuntime.MarkedBlock{Open: "<!-- t:x -->", Close: "<!-- /t:x -->"}

const testBlockText = "<!-- t:x -->\nbody line\n<!-- /t:x -->"

// A file shaped like the live ones: blocks of other tools, the owner's own block and unmarked text.
const liveShapedFile = "<!-- gentle-ai:persona -->\n## Rules\n\n- one\n<!-- /gentle-ai:persona -->\n\n" +
	"<!-- gentle-ai:orchestrator -->\ntext\n<!-- /gentle-ai:orchestrator -->\n" +
	"<!-- user:memorycard-store -->\nstore\n<!-- /user:memorycard-store -->\n"

func TestMarkedBlockIsAppendedAfterOneBlankLineWhenAbsent(t *testing.T) {
	got, change, err := testBlock.Apply("A\n", "body line\n")
	if err != nil || change != engineRuntime.BlockAdded || got != "A\n\n"+testBlockText+"\n" {
		t.Errorf("Apply = %q %v %v", got, change, err)
	}
}

func TestMarkedBlockIsAppendedAfterABlankLineAndLeavesNoFinalNewlineWhenTheFileHadNone(t *testing.T) {
	got, change, err := testBlock.Apply("A", "body line")
	if err != nil || change != engineRuntime.BlockAdded || got != "A\n\n"+testBlockText {
		t.Errorf("Apply = %q %v %v", got, change, err)
	}
}

func TestMarkedBlockOfAnEmptyFileIsTheBlockAlone(t *testing.T) {
	got, change, err := testBlock.Apply("", "body line")
	if err != nil || change != engineRuntime.BlockAdded || got != testBlockText+"\n" {
		t.Errorf("Apply = %q %v %v", got, change, err)
	}
}

func TestMarkedBlockIsReplacedInPlaceAndEveryByteOutsideItIsKept(t *testing.T) {
	before, after := "\n\n"+liveShapedFile+"\nmy text\n", "\ntail without newline"
	stale := before + "<!-- t:x -->\nold\nlines\n<!-- /t:x -->" + after
	got, change, err := testBlock.Apply(stale, "body line")
	want := before + testBlockText + after
	if err != nil || change != engineRuntime.BlockReplaced || got != want {
		t.Errorf("Apply = %q %v %v, want %q", got, change, err, want)
	}
}

func TestMarkedBlockAppliedTwiceIsUnchanged(t *testing.T) {
	once, _, _ := testBlock.Apply(liveShapedFile, "body line")
	twice, change, err := testBlock.Apply(once, "body line")
	if err != nil || change != engineRuntime.BlockUnchanged || twice != once {
		t.Errorf("second Apply = %q %v %v", twice, change, err)
	}
}

func TestMarkedBlockRemoveUndoesApplyExactly(t *testing.T) {
	for name, original := range map[string]string{
		"empty file":             "",
		"no final newline":       "A",
		"blank lines at the end": "A\n\n",
		"crlf":                   "A\r\nB\r\n",
		"crlf no final newline":  "A\r\nB",
		"live shaped":            liveShapedFile,
		"two lines":              "A\nB\n",
		"only a blank line":      "\n",
		"unmarked single line":   "one line\n",
	} {
		applied, _, err := testBlock.Apply(original, "body line")
		if err != nil {
			t.Fatal(err)
		}
		got, removed, err := testBlock.Remove(applied)
		if err != nil || !removed || got != original {
			t.Errorf("%s: Remove(Apply) = %q %v %v, want %q", name, got, removed, err, original)
		}
	}
}

func TestMarkedBlockRemoveKeepsTheTextOnBothSidesOfABlockInTheMiddle(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"blank lines on both sides are the owner's": {"A\n\n" + testBlockText + "\n\nB\n", "A\n\n\nB\n"},
		"no blank lines": {"A\n" + testBlockText + "\nB\n", "A\nB\n"},
		"at the start, the blank line after it is the owner's": {testBlockText + "\n\nB\n", "\nB\n"},
		"no newline at the end":                                {"A\n" + testBlockText, "A\n"},
	} {
		got, removed, err := testBlock.Remove(tc.in)
		if err != nil || !removed || got != tc.want {
			t.Errorf("%s: Remove = %q %v %v, want %q", name, got, removed, err, tc.want)
		}
	}
}

func TestMarkedBlockRemoveOfAFileWithoutTheBlockChangesNothing(t *testing.T) {
	got, removed, err := testBlock.Remove(liveShapedFile)
	if err != nil || removed || got != liveShapedFile {
		t.Errorf("Remove = %q %v %v", got, removed, err)
	}
}

func TestMarkedBlockWithMarkersThatAreNotOnePairIsNeverWritten(t *testing.T) {
	for name, content := range map[string]string{
		"open without close": "A\n<!-- t:x -->\nB\n",
		"close without open": "A\n<!-- /t:x -->\nB\n",
		"close before open":  "<!-- /t:x -->\n<!-- t:x -->\n",
		"two pairs":          testBlockText + "\n" + testBlockText + "\n",
	} {
		if got, change, err := testBlock.Apply(content, "x"); err == nil || got != content || change != engineRuntime.BlockUnchanged {
			t.Errorf("%s: Apply = %q %v %v, want the content back and an error", name, got, change, err)
		}
		if got, removed, err := testBlock.Remove(content); err == nil || got != content || removed {
			t.Errorf("%s: Remove = %q %v %v, want the content back and an error", name, got, removed, err)
		}
		if _, err := testBlock.State(content, "x"); err == nil {
			t.Errorf("%s: State has no error", name)
		}
	}
}

func TestMarkedBlockMarkersWithACarriageReturnAreFound(t *testing.T) {
	crlf := "A\r\n<!-- t:x -->\r\nold\r\n<!-- /t:x -->\r\nB\r\n"
	got, change, err := testBlock.Apply(crlf, "body line")
	want := "A\r\n<!-- t:x -->\r\nbody line\r\n<!-- /t:x -->\r\nB\r\n"
	if err != nil || change != engineRuntime.BlockReplaced || got != want {
		t.Errorf("Apply = %q %v %v, want %q", got, change, err, want)
	}
}

func TestMarkedBlockStateSaysMissingStaleOrCurrent(t *testing.T) {
	for name, tc := range map[string]struct {
		content string
		want    engineRuntime.BlockState
	}{
		"missing": {liveShapedFile, engineRuntime.BlockMissing},
		"stale":   {"A\n<!-- t:x -->\nold\n<!-- /t:x -->\n", engineRuntime.BlockStale},
		"current": {"A\n" + testBlockText + "\n", engineRuntime.BlockCurrent},
	} {
		if got, err := testBlock.State(tc.content, "body line"); err != nil || got != tc.want {
			t.Errorf("%s: State = %v %v, want %v", name, got, err, tc.want)
		}
	}
}

func readFileText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestMarkedBlockWriteFileCreatesAMissingFileWithTheBlockAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "CLAUDE.md")
	change, err := testBlock.WriteFile(path, "body line")
	if err != nil || change != engineRuntime.BlockAdded || readFileText(t, path) != testBlockText+"\n" {
		t.Fatalf("WriteFile = %v %v, file %q", change, err, readFileText(t, path))
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v, want 0644", info.Mode().Perm())
	}
}

func TestMarkedBlockWriteFileKeepsTheModeOfTheFileItReplaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "CLAUDE.md")
	if err := os.WriteFile(path, []byte(liveShapedFile), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := testBlock.WriteFile(path, "body line"); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
	if got := readFileText(t, path); !strings.HasPrefix(got, liveShapedFile+"\n"+testBlockText) {
		t.Errorf("file = %q", got)
	}
}

func TestMarkedBlockWriteFileFollowsASymbolicLinkAndKeepsIt(t *testing.T) {
	dir := t.TempDir()
	real, link := filepath.Join(dir, "real.md"), filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(real, []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real.md", link); err != nil {
		t.Fatal(err)
	}
	if _, err := testBlock.WriteFile(link, "body line"); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Lstat(link); info.Mode()&os.ModeSymlink == 0 {
		t.Error("the symbolic link was replaced by a file")
	}
	if got := readFileText(t, real); got != "mine\n\n"+testBlockText+"\n" {
		t.Errorf("the linked file = %q", got)
	}
}

func TestMarkedBlockWriteFileOfACurrentBlockDoesNotWriteTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "CLAUDE.md")
	if _, err := testBlock.WriteFile(path, "body line"); err != nil {
		t.Fatal(err)
	}
	old := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	change, err := testBlock.WriteFile(path, "body line")
	info, _ := os.Stat(path)
	if err != nil || change != engineRuntime.BlockUnchanged || !info.ModTime().Equal(old) {
		t.Errorf("WriteFile = %v %v, mtime %v", change, err, info.ModTime())
	}
}

func TestMarkedBlockWriteFileLeavesNoTemporaryFileAndNoPartialWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")
	if _, err := testBlock.WriteFile(path, "body line"); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("the directory holds %d entries, want only CLAUDE.md", len(entries))
	}
}

func TestMarkedBlockWriteFileNamesTheFileAndKeepsItWhenTheMarkersAreBroken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "CLAUDE.md")
	broken := "mine\n<!-- t:x -->\nno close\n"
	if err := os.WriteFile(path, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := testBlock.WriteFile(path, "body line"); err == nil || !strings.Contains(err.Error(), path) {
		t.Errorf("WriteFile error = %v, want one naming %s", err, path)
	}
	if _, err := testBlock.RemoveFile(path); err == nil || !strings.Contains(err.Error(), path) {
		t.Errorf("RemoveFile error = %v, want one naming %s", err, path)
	}
	if got := readFileText(t, path); got != broken {
		t.Errorf("the file changed to %q", got)
	}
}

func TestMarkedBlockRemoveFileDeletesAFileThatHeldOnlyTheBlockAndNothingForAMissingOne(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")
	if removed, err := testBlock.RemoveFile(path); err != nil || removed {
		t.Errorf("RemoveFile of a missing file = %v %v", removed, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("RemoveFile created the missing file")
	}
	if _, err := testBlock.WriteFile(path, "body line"); err != nil {
		t.Fatal(err)
	}
	if removed, err := testBlock.RemoveFile(path); err != nil || !removed {
		t.Errorf("RemoveFile = %v %v", removed, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("the file that held only the block is still there")
	}
}

func TestMarkedBlockStateOfFileOfAMissingFileIsMissing(t *testing.T) {
	if got, err := testBlock.StateOfFile(filepath.Join(t.TempDir(), "none.md"), "x"); err != nil || got != engineRuntime.BlockMissing {
		t.Errorf("StateOfFile = %v %v", got, err)
	}
}

func TestOwnerRulesBlockUsesTheLabdrianMarkers(t *testing.T) {
	if engineRuntime.OwnerRulesBlock.Open != "<!-- labdrian:owner-rules -->" || engineRuntime.OwnerRulesBlock.Close != "<!-- /labdrian:owner-rules -->" {
		t.Errorf("OwnerRulesBlock = %#v", engineRuntime.OwnerRulesBlock)
	}
}
