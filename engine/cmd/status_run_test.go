package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func strPtr(s string) *string { return &s }

// runStatus asks the deps where the installation is: the HOME of the environment they give and the
// working directory they give, never the process's own.
func TestRunStatusLooksWhereTheDepsSay(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	d := testDeps()
	d.getenv = environmentOf(map[string]string{"HOME": home})
	d.getwd = func() (string, error) { return project, nil }
	p := newCapturedProcess()

	runStatus(p.process, d, nil)

	out := p.out.String()
	for _, want := range []string{
		filepath.Join(home, ".claude", "settings.json"),
		filepath.Join(project, ".atl", "skill-registry.md"),
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the report does not name %s:\n%s", want, out)
		}
	}
	if len(p.exits) != 1 || p.exits[0] != 1 {
		t.Errorf("exit calls = %v, want exactly [1] (an empty home fails its hard checks)", p.exits)
	}
}

// A working directory that cannot be determined is none: the registry is then not looked for.
func TestRunStatusWithoutAWorkingDirectoryDoesNotLookForTheRegistry(t *testing.T) {
	d := testDeps()
	d.getenv = environmentOf(map[string]string{"HOME": t.TempDir()})
	d.getwd = func() (string, error) { return "", errors.New("no working directory") }
	p := newCapturedProcess()

	runStatus(p.process, d, nil)

	if strings.Contains(p.out.String(), "registry:") {
		t.Errorf("the report has a registry line though there is no working directory:\n%s", p.out.String())
	}
}

// The options of status that narrow it, for the doctor of the overlay script: the registry of a project
// alone, in the directory it is told of, with the exit code of that one check. The words are the
// ones the full report says of the registry.
func TestRunStatusOnlyRegistryReportsThatRegistryAloneForTheDirectoryItIsTold(t *testing.T) {
	bothBlocks := "<!-- BEGIN: minimalism-contract-scope (auto-generated) -->\n<!-- BEGIN: anti-generic-design-scope (auto-generated) -->\n"
	for _, tc := range []struct {
		name      string
		registry  *string
		wantLine  string
		wantExits []int
	}{
		{"no registry", nil, "[OK  ] registry: %s — not present (project may not use the overlay)\n", nil},
		{"an empty registry", strPtr(""), "[FAIL] registry: %s — present but EMPTY — run skill-registry refresh; do NOT conclude skills are absent (an empty registry is inconclusive, not zero)\n", []int{1}},
		{"a registry without the scoped blocks", strPtr("# r\n"), "[WARN] registry: %s — present but scoped block(s) missing: minimalism-contract-scope, anti-generic-design-scope (run 'labdrian install-hooks' or propagate)\n", []int{2}},
		{"a registry with both", strPtr(bothBlocks), "[OK  ] registry: %s — scoped block present\n", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			project := t.TempDir()
			if tc.registry != nil {
				if err := os.MkdirAll(filepath.Join(project, ".atl"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(project, ".atl", "skill-registry.md"), []byte(*tc.registry), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			d := testDeps()
			d.getenv = environmentOf(map[string]string{"HOME": t.TempDir()}) // an empty home: nothing else would pass
			d.getwd = func() (string, error) { return t.TempDir(), nil }     // not the project
			p := newCapturedProcess()

			runStatus(p.process, d, []string{"--only", "registry", "--cwd", project})

			want := fmt.Sprintf(tc.wantLine, filepath.Join(project, ".atl", "skill-registry.md"))
			if p.out.String() != want || p.err.String() != "" || !equalInts(p.exits, tc.wantExits) {
				t.Errorf("status --only registry = %q, %q, exits %v, want %q, nothing on stderr and exits %v", p.out.String(), p.err.String(), p.exits, want, tc.wantExits)
			}
		})
	}
}

func TestRunStatusOnlyRegistryLooksInTheWorkingDirectoryWhenNoneIsGiven(t *testing.T) {
	project := t.TempDir()
	d := testDeps()
	d.getenv = environmentOf(map[string]string{"HOME": t.TempDir()})
	d.getwd = func() (string, error) { return project, nil }
	p := newCapturedProcess()

	runStatus(p.process, d, []string{"--only", "registry"})

	if want := "[OK  ] registry: " + filepath.Join(project, ".atl", "skill-registry.md") + " — not present (project may not use the overlay)\n"; p.out.String() != want {
		t.Errorf("status --only registry = %q, want %q", p.out.String(), want)
	}
}

func TestRunStatusCwdAloneMovesTheRegistryOfTheFullReport(t *testing.T) {
	project := t.TempDir()
	d := testDeps()
	d.getenv = environmentOf(map[string]string{"HOME": t.TempDir()})
	d.getwd = func() (string, error) { return t.TempDir(), nil }
	p := newCapturedProcess()

	runStatus(p.process, d, []string{"--cwd", project})

	if !strings.Contains(p.out.String(), filepath.Join(project, ".atl", "skill-registry.md")) || !strings.Contains(p.out.String(), "binary:") {
		t.Errorf("the full report does not look for the registry of --cwd:\n%s", p.out.String())
	}
}

func TestRunStatusRefusesAnOptionItCannotUse(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		cwd  bool // the working directory can be determined
		want string
	}{
		{"--only with no value", []string{"--only"}, true, "error: status: flag \"--only\" requires a value\n"},
		{"--cwd with no value", []string{"--only", "registry", "--cwd"}, true, "error: status: flag \"--cwd\" requires a value\n"},
		{"--only of a check there is not", []string{"--only", "binary"}, true, "error: status: --only \"binary\" names no check; expected registry\n"},
		{"no directory to look in", []string{"--only", "registry"}, false, "error: status --only registry: the working directory cannot be determined; give --cwd\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := testDeps()
			d.getenv = environmentOf(map[string]string{"HOME": t.TempDir()})
			d.getwd = func() (string, error) { return "", errors.New("no working directory") }
			if tc.cwd {
				d.getwd = func() (string, error) { return t.TempDir(), nil }
			}
			p := newCapturedProcess()

			runStatus(p.process, d, tc.args)

			if p.err.String() != tc.want || p.out.String() != "" || !equalInts(p.exits, []int{1}) {
				t.Errorf("status %v = %q, %q, exits %v, want %q on stderr, nothing on stdout and exit 1", tc.args, p.out.String(), p.err.String(), p.exits, tc.want)
			}
		})
	}
}

// What status was given and does not know it ignores, as it always did: a word, a flag nobody named.
func TestRunStatusIgnoresWhatItDoesNotKnow(t *testing.T) {
	project := t.TempDir()
	d := testDeps()
	d.getenv = environmentOf(map[string]string{"HOME": t.TempDir()})
	d.getwd = func() (string, error) { return project, nil }
	p := newCapturedProcess()

	runStatus(p.process, d, []string{"garbage", "--frobnicate", "registry"})

	if !strings.Contains(p.out.String(), "binary:") || !strings.Contains(p.out.String(), filepath.Join(project, ".atl", "skill-registry.md")) || p.err.String() != "" {
		t.Errorf("status with arguments it does not know = %q (stderr %q), want the full report", p.out.String(), p.err.String())
	}
}

// The usage text and the README both say the options of status and what the registry check judges.
func TestStatusOptionsAreDocumentedInTheUsageAndTheREADME(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	for name, text := range map[string]string{
		"README":  docSection(t, string(readme), "gentle-ai-overlay status [--only registry]", "gentle-ai-overlay runtime capabilities"),
		"usage()": docSection(t, captureUsage(t), "  engine status [--only registry]", "  engine prespec"),
	} {
		for _, want := range []string{"[--only registry] [--cwd <dir>]", "--only registry", "--cwd", "exit 0", "1", "2"} {
			if !strings.Contains(strings.ToLower(text), strings.ToLower(want)) {
				t.Errorf("%s of status does not say %q", name, want)
			}
		}
	}
}
