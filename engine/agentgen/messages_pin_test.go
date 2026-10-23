package agentgen_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/agentgen"
)

// These pin the words Generate and Check say when something is wrong, whole, and the order in
// which they name the artifacts: the command prints them, and a person reads the first
// line to know what to run.

// allPaths are the files of the table in order, as Generate and Check name them: the Claude, Pi and
// Codex file of each role in the order of the roles.
func allPaths() []string {
	var paths []string
	for _, r := range wantRoles {
		paths = append(paths, "agents/"+r.name+".md", "pi/agents/"+r.name+".md", "codex/agents/"+r.name+".toml")
	}
	return paths
}

const regenerate = "run: gentle-ai-overlay gadu-generate"

// A root with nothing in it has no artifact and no source of a role: Check names the source of each
// role, in the order of the table, and nothing else. A role whose source is missing has no files
// named: they cannot be compared with a body that is not there.
func TestCheckNamesEveryArtifactThatIsMissingInTheOrderOfTheTable(t *testing.T) {
	err := agentgen.Check(t.TempDir())

	want := "generated agent artifacts are stale or missing:"
	for _, r := range wantRoles {
		want += "\n  roles/" + r.name + "/body.md: missing (the source of the role is not in the repository)"
	}
	if err == nil || err.Error() != want {
		t.Errorf("Check error = %v\nwant\n%s", err, want)
	}
}

func TestCheckSaysWhichArtifactsAreStaleAndWhichCannotBeReadAndPassesTheOthers(t *testing.T) {
	root := generated(t)
	// The second is stale; the third cannot be read (a directory is where its file should be).
	stale := filepath.Join(root, "pi", "agents", "builder.md")
	if err := os.WriteFile(stale, []byte("hand-edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	unreadable := filepath.Join(root, "codex", "agents", "builder.toml")
	if err := os.Remove(unreadable); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(unreadable, 0o755); err != nil {
		t.Fatal(err)
	}
	// What the machine says of reading a directory is its own words, and differs between systems.
	// Check says it after its own, so the expectation asks the machine the same question.
	_, readErr := os.ReadFile(unreadable)
	if readErr == nil {
		t.Fatal("reading a directory as a file succeeded, so the case does not make an unreadable file")
	}

	err := agentgen.Check(root)

	if err == nil {
		t.Fatal("Check returned nil")
	}
	lines := strings.Split(err.Error(), "\n")
	if len(lines) != 3 || lines[0] != "generated agent artifacts are stale or missing:" {
		t.Fatalf("Check error = %q, want its heading and two lines", err)
	}
	if want := "  pi/agents/builder.md: stale — committed content diverges from generator output (" + regenerate + ")"; lines[1] != want {
		t.Errorf("first problem = %q, want %q", lines[1], want)
	}
	if want := "  codex/agents/builder.toml: cannot read committed file: " + readErr.Error(); lines[2] != want {
		t.Errorf("second problem = %q, want %q", lines[2], want)
	}
}

func TestGenerateSaysWhichArtifactCouldNotBeWrittenAndWhy(t *testing.T) {
	root := seededRoot(t)
	// A file is where the directory of the first artifact should be.
	blocker := filepath.Join(root, "agents")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	// The words of the failure to make a directory under a file are the machine's, and differ
	// between systems; the generator says them after its own, so the expectation asks the machine
	// the same question (the directory of the first artifact) and keeps the generator's words.
	mkdirErr := os.MkdirAll(blocker, 0o755)
	if mkdirErr == nil {
		t.Fatal("making a directory where a file is succeeded, so the case does not make a failure")
	}

	err := agentgen.Generate(root)

	want := "writing agents/builder.md: creating parent dir for " + filepath.Join(blocker, "builder.md") + ": " + mkdirErr.Error()
	if err == nil || err.Error() != want {
		t.Errorf("Generate error = %v\nwant\n%s", err, want)
	}
}

// A role file that was edited by hand is named by its path and said to be stale, like any other
// artifact: the Check of CI is what keeps the files of the roles what the generator writes.
func TestCheckSaysWhichRoleFileIsStale(t *testing.T) {
	root := generated(t)
	edited := filepath.Join(root, "codex", "agents", "polisher.toml")
	if err := os.WriteFile(edited, []byte("hand-edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := agentgen.Check(root)

	want := "generated agent artifacts are stale or missing:\n  codex/agents/polisher.toml: stale — committed content diverges from generator output (" + regenerate + ")"
	if err == nil || err.Error() != want {
		t.Errorf("Check error = %v\nwant\n%s", err, want)
	}
}

// The source of a role that is not there is said by its path, and the three files of that role are
// not compared with a body that cannot be read. Generate refuses, and writes nothing: a table with a
// source missing is not written half.
func TestARoleSourceThatIsMissingIsNamedAndNothingIsWritten(t *testing.T) {
	root := seededRoot(t)
	if err := os.Remove(filepath.Join(root, "roles", "reviewer", "body.md")); err != nil {
		t.Fatal(err)
	}

	checkErr := agentgen.Check(root)
	generateErr := agentgen.Generate(root)

	const missing = "  roles/reviewer/body.md: missing (the source of the role is not in the repository)"
	if checkErr == nil || !strings.HasSuffix(checkErr.Error(), "\n"+missing) || strings.Contains(checkErr.Error(), "/reviewer.") {
		t.Errorf("Check error = %v\nwant it to end with\n%s and to name no file of the reviewer", checkErr, missing)
	}
	if generateErr == nil || !strings.HasPrefix(generateErr.Error(), "reading roles/reviewer/body.md: ") {
		t.Errorf("Generate error = %v, want one that starts %q", generateErr, "reading roles/reviewer/body.md: ")
	}
	if _, err := os.Stat(filepath.Join(root, "agents")); err == nil {
		t.Error("Generate wrote files although a source is missing")
	}
}

func TestGenerateWritesEveryArtifactAndPutsBackWhatWasEdited(t *testing.T) {
	root := seededRoot(t)
	if err := agentgen.Generate(root); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	first := map[string]string{}
	for _, p := range allPaths() {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil || len(data) == 0 {
			t.Fatalf("%s: (%d bytes, %v), want a file with something in it", p, len(data), err)
		}
		first[p] = string(data)
	}
	if err := os.WriteFile(filepath.Join(root, "pi", "agents", "builder.md"), []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := agentgen.Generate(root); err != nil {
		t.Fatalf("Generate (again): %v", err)
	}

	for _, p := range allPaths() {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil || string(data) != first[p] {
			t.Errorf("%s changed between two generations (err %v)", p, err)
		}
	}
	if err := agentgen.Check(root); err != nil {
		t.Errorf("Check of what Generate wrote: %v", err)
	}
}
