package gadu_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gadu"
)

// These pin the words Generate and Check say when something is wrong, whole, and the order in
// which they name the four artifacts: the command prints them, and a person reads the first
// line to know what to run.

// the four artifacts, as Generate and Check name them and in the order they report them.
var artifactPaths = []string{
	"agents/GADU.md",
	"opencode/agents/GADU.md",
	"pi/agents/GADU.md",
	"skills/gadu-operator/SKILL.md",
}

const regenerate = "run: gentle-ai-overlay gadu-generate"

func TestCheckNamesEveryArtifactThatIsMissingInTheOrderOfTheTable(t *testing.T) {
	err := gadu.Check(t.TempDir())

	want := "GADU artifacts are stale or missing:"
	for _, p := range artifactPaths {
		want += "\n  " + p + ": missing (not yet generated — " + regenerate + ")"
	}
	if err == nil || err.Error() != want {
		t.Errorf("Check error = %v\nwant\n%s", err, want)
	}
}

func TestCheckSaysWhichArtifactsAreStaleAndWhichCannotBeReadAndPassesTheOthers(t *testing.T) {
	root := t.TempDir()
	if err := gadu.Generate(root); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// The second is stale; the third cannot be read (a directory is where its file should be).
	stale := filepath.Join(root, "opencode", "agents", "GADU.md")
	if err := os.WriteFile(stale, []byte("hand-edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	unreadable := filepath.Join(root, "pi", "agents", "GADU.md")
	if err := os.Remove(unreadable); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(unreadable, 0o755); err != nil {
		t.Fatal(err)
	}

	err := gadu.Check(root)

	if err == nil {
		t.Fatal("Check returned nil")
	}
	lines := strings.Split(err.Error(), "\n")
	if len(lines) != 3 || lines[0] != "GADU artifacts are stale or missing:" {
		t.Fatalf("Check error = %q, want its heading and two lines", err)
	}
	if want := "  opencode/agents/GADU.md: stale — committed content diverges from generator output (" + regenerate + ")"; lines[1] != want {
		t.Errorf("first problem = %q, want %q", lines[1], want)
	}
	if want := "  pi/agents/GADU.md: cannot read committed file: read " + unreadable + ": is a directory"; lines[2] != want {
		t.Errorf("second problem = %q, want %q", lines[2], want)
	}
}

func TestGenerateSaysWhichArtifactCouldNotBeWrittenAndWhy(t *testing.T) {
	root := t.TempDir()
	// A file is where the directory of the first artifact should be.
	blocker := filepath.Join(root, "agents")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	err := gadu.Generate(root)

	want := "writing agents/GADU.md: creating parent dir for " + filepath.Join(blocker, "GADU.md") + ": mkdir " + blocker + ": not a directory"
	if err == nil || err.Error() != want {
		t.Errorf("Generate error = %v\nwant\n%s", err, want)
	}
}

func TestGenerateWritesTheFourArtifactsAndPutsBackWhatWasEdited(t *testing.T) {
	root := t.TempDir()
	if err := gadu.Generate(root); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	first := map[string]string{}
	for _, p := range artifactPaths {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil || len(data) == 0 {
			t.Fatalf("%s: (%d bytes, %v), want a file with something in it", p, len(data), err)
		}
		first[p] = string(data)
	}
	if err := os.WriteFile(filepath.Join(root, "pi", "agents", "GADU.md"), []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := gadu.Generate(root); err != nil {
		t.Fatalf("Generate (again): %v", err)
	}

	for _, p := range artifactPaths {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil || string(data) != first[p] {
			t.Errorf("%s changed between two generations (err %v)", p, err)
		}
	}
	if err := gadu.Check(root); err != nil {
		t.Errorf("Check of what Generate wrote: %v", err)
	}
}
