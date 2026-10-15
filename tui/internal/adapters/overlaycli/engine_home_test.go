package overlaycli

// The guard of provisionEngine (selfupdate_backend_test.go): the engine is built in a directory
// under the HOME of the process, and the tests may write there only because TestMain gave them a
// home of their own (testhome.Run), which goes when the run ends.

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/tui/internal/testhome"
)

// homeProbe is a testing.TB that keeps what testhome.Check finds wrong instead of failing the test
// that asked, so that the answer can be acted on (and tested).
type homeProbe struct {
	testing.TB
	problems []string
}

func (p *homeProbe) Errorf(format string, args ...any) {
	p.problems = append(p.problems, fmt.Sprintf(format, args...))
}

// ownHomeProblems says what is wrong with the home of the process, as testhome.Check judges it:
// nothing when it is the one testhome.Run made for this test binary.
func ownHomeProblems(t testing.TB) []string {
	t.Helper()
	probe := &homeProbe{TB: t}
	testhome.Check(probe)
	return probe.problems
}

// engineBuildDir makes the directory the engine is built in: under the home of the run, and in no
// other home. A process whose HOME is not the one testhome.Run made (a TestMain that forgot it, a
// test that set HOME) gets an error and no directory, so the build can never leave a directory in a
// home that is somebody's.
func engineBuildDir(t testing.TB) (string, error) {
	t.Helper()
	if problems := ownHomeProblems(t); len(problems) > 0 {
		return "", fmt.Errorf("not building the engine: the tests are not in a home of their own:\n%s", strings.Join(problems, "\n"))
	}
	return os.MkdirTemp(os.Getenv("HOME"), "tui-engine-")
}

func TestTheEngineIsBuiltUnderTheHomeOfTheRun(t *testing.T) {
	dir, err := engineBuildDir(t)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(dir, os.Getenv("HOME")+string(os.PathSeparator)) {
		t.Errorf("the engine is built in %q, want a directory under the home of the run %q", dir, os.Getenv("HOME"))
	}
}

func TestTheEngineIsNotBuiltUnderAHomeThatIsNotTheRunsOwn(t *testing.T) {
	notOurs := t.TempDir() // a directory the run did not make as its home (not named for testhome)
	t.Setenv("HOME", notOurs)

	dir, err := engineBuildDir(t)

	if err == nil || !strings.Contains(err.Error(), "not in a home of their own") {
		t.Errorf("engineBuildDir() = %q, %v, want it to refuse a home that testhome.Run did not make", dir, err)
	}
	if entries, readErr := os.ReadDir(notOurs); readErr != nil || len(entries) != 0 {
		t.Errorf("the refusal left %d entries in the home it refused (%v), want the home as it was", len(entries), readErr)
	}
}
