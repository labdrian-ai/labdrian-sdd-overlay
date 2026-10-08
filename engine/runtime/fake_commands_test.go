package runtime_test

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pipkg"
	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
)

// newPiPackages is the package builder of the tests: the real one, over the registry files of the
// test. It builds into temporary directories and runs no process of the Pi CLI.
func newPiPackages() engineRuntime.PackageBuilder {
	return pipkg.Packages{Registries: fileRegistries}
}

// fakeCommands is the CommandRunner every test of the Pi adapter hands it: it starts no process.
// It records what the adapter asked to run -- the program, its arguments and the deadline of the
// context -- and answers from the fields a test sets. Nothing in this package reaches the real
// `pi` (see the guard in live_guard_test.go).
type fakeCommands struct {
	mu sync.Mutex
	// missing makes LookPath report that pi is not installed.
	missing bool
	// lookups are the names LookPath was asked for.
	lookups []string
	// fail, when it is not nil, decides the error of a run from its arguments; output is what it
	// printed. A nil fail means every command succeeds.
	fail   func(args []string) error
	output string
	// calls are the runs, in order.
	calls []fakeCall
}

// fakeCall is one run the adapter asked for.
type fakeCall struct {
	Bin         string
	Args        []string
	HasDeadline bool
	// Remaining is the time left to the deadline when the run was asked for.
	Remaining time.Duration
}

// fakePiPath is the path the fake finds pi at.
const fakePiPath = "/fake/bin/pi"

func (f *fakeCommands) LookPath(name string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lookups = append(f.lookups, name)
	if f.missing {
		return "", errors.New(`exec: "` + name + `": executable file not found in $PATH`)
	}
	return "/fake/bin/" + name, nil
}

func (f *fakeCommands) Run(ctx context.Context, bin string, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	call := fakeCall{Bin: bin, Args: append([]string(nil), args...)}
	if deadline, ok := ctx.Deadline(); ok {
		call.HasDeadline = true
		call.Remaining = time.Until(deadline)
	}
	f.calls = append(f.calls, call)
	if f.fail != nil {
		if err := f.fail(args); err != nil {
			return []byte(f.output), err
		}
	}
	return []byte(f.output), nil
}

// invocations are the arguments of every run, one slice per run, in order.
func (f *fakeCommands) invocations() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var all [][]string
	for _, call := range f.calls {
		all = append(all, call.Args)
	}
	return all
}
