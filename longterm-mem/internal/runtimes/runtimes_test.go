package runtimes

import (
	"errors"
	"fmt"
	"io/fs"
	"reflect"
	"strings"
	"testing"
)

// fakeMachine is a Machine that answers what a case says and remembers what it was asked.
type fakeMachine struct {
	roots    map[string]string // target -> default config root; absent is unresolvable
	stateDir string
	binary   string
	// installedPi is the package directory the machine says pi is installed at.
	installedPi string

	piProbes []string
}

func (m *fakeMachine) ConfigRoot(target string) string { return m.roots[target] }
func (m *fakeMachine) StateDir() string                { return m.stateDir }
func (m *fakeMachine) Binary() string                  { return m.binary }
func (m *fakeMachine) PiInstalled(packageDir string) bool {
	m.piProbes = append(m.piProbes, packageDir)
	return m.installedPi != "" && packageDir == m.installedPi
}

// aMachine is a machine that has everything: a root for every runtime, a state directory and a binary.
func aMachine() *fakeMachine {
	return &fakeMachine{
		roots:    map[string]string{Claude: "/home", Opencode: "/home/.config/opencode", Codex: "/home/.codex", Pi: "/state/pi/labdrian-pi"},
		stateDir: "/state/longterm-mem",
		binary:   "/state/bin/longterm-mem",
	}
}

// call is one thing a fakeRegistrar was asked to do.
type call struct {
	verb                              string
	target, configRoot, stateDir, bin string
}

// fakeRegistrar is a Registrar that answers per target and remembers what it was asked, in order.
type fakeRegistrar struct {
	registerErr   map[string]error
	unregisterErr map[string]error
	removal       map[string]Removal

	calls []call
}

func (r *fakeRegistrar) Register(target, configRoot, stateDir, binary string) error {
	r.calls = append(r.calls, call{"register", target, configRoot, stateDir, binary})
	return r.registerErr[target]
}

func (r *fakeRegistrar) Unregister(target, configRoot, stateDir string) (Removal, error) {
	r.calls = append(r.calls, call{"unregister", target, configRoot, stateDir, ""})
	return r.removal[target], r.unregisterErr[target]
}

func using(registrar *fakeRegistrar, machine *fakeMachine) Runtimes {
	return Runtimes{Registrar: registrar, Machine: machine}
}

func targetsOf(results []Result) []string {
	var targets []string
	for _, result := range results {
		targets = append(targets, result.Target)
	}
	return targets
}

func outcomesOf(results []Result) []Outcome {
	var outcomes []Outcome
	for _, result := range results {
		outcomes = append(outcomes, result.Outcome)
	}
	return outcomes
}

func TestExpand(t *testing.T) {
	for name, tc := range map[string]struct {
		target string
		want   []string
		known  bool
	}{
		"claude":   {Claude, []string{Claude}, true},
		"opencode": {Opencode, []string{Opencode}, true},
		"codex":    {Codex, []string{Codex}, true},
		"pi":       {Pi, []string{Pi}, true},
		"all is every runtime with a configuration file of its own, and not pi": {All, []string{Claude, Opencode, Codex}, true},
		"a name nobody knows": {"bogus", nil, false},
		"nothing":             {"", nil, false},
		"a name in capitals":  {"Claude", nil, false},
	} {
		t.Run(name, func(t *testing.T) {
			got, known := Expand(tc.target)
			if known != tc.known || !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Expand(%q) = (%v, %v), want (%v, %v)", tc.target, got, known, tc.want, tc.known)
			}
		})
	}
}

// The request is refused before any runtime is touched, in the order the questions are asked: a target is named,
// it is one that exists, a config root goes with a single one, the state directory and the binary can be told,
// and pi is installed when it is asked for by name. The first that fails is the answer, and nothing was attempted.
func TestRegister_RefusesTheRequestBeforeTouchingAnyRuntime(t *testing.T) {
	cannotTell := func() *fakeMachine { m := aMachine(); m.stateDir, m.binary = "", ""; return m }

	for name, tc := range map[string]struct {
		request Request
		machine *fakeMachine
		want    Refusal
	}{
		"no target": {Request{}, aMachine(), Refusal{Kind: TargetRequired}},
		"no target is said before the state directory is missed": {Request{}, cannotTell(), Refusal{Kind: TargetRequired}},
		"an unknown target":                                {Request{Target: "bogus"}, aMachine(), Refusal{Kind: UnknownTarget, Target: "bogus"}},
		"all with a config root":                           {Request{Target: All, ConfigRoot: "/x"}, aMachine(), Refusal{Kind: ConfigRootNeedsOneTarget}},
		"an unknown target is said before the config root": {Request{Target: "bogus", ConfigRoot: "/x"}, aMachine(), Refusal{Kind: UnknownTarget, Target: "bogus"}},
		"no state directory":                               {Request{Target: Claude, Binary: "/b"}, cannotTell(), Refusal{Kind: StateDirUnresolved}},
		"no state directory is said before the binary":     {Request{Target: Claude}, cannotTell(), Refusal{Kind: StateDirUnresolved}},
		"no binary": {Request{Target: Claude, StateDir: "/s"}, cannotTell(), Refusal{Kind: BinaryUnresolved}},
		"pi, asked for by name, where it is not installed": {
			Request{Target: Pi}, aMachine(), Refusal{Kind: PiNotInstalled, Root: "/state/pi/labdrian-pi"},
		},
		"pi, in the package directory it was given, where it is not installed": {
			Request{Target: Pi, ConfigRoot: "/pkg"}, aMachine(), Refusal{Kind: PiNotInstalled, Root: "/pkg"},
		},
		"pi, with no package directory to be told": {
			Request{Target: Pi}, func() *fakeMachine { m := aMachine(); m.roots = nil; return m }(), Refusal{Kind: PiNotInstalled, Root: ""},
		},
		"the state directory is said before pi": {
			Request{Target: Pi}, func() *fakeMachine { m := cannotTell(); m.binary = "/b"; return m }(), Refusal{Kind: StateDirUnresolved},
		},
	} {
		t.Run(name, func(t *testing.T) {
			registrar := &fakeRegistrar{}

			report := using(registrar, tc.machine).Register(tc.request)

			if report.Refusal == nil || *report.Refusal != tc.want {
				t.Errorf("Refusal = %+v, want %+v", report.Refusal, tc.want)
			}
			if len(report.Results) != 0 || len(registrar.calls) != 0 {
				t.Errorf("a refused request attempted %v (results %v), want nothing", registrar.calls, report.Results)
			}
		})
	}
}

// The overrides win over the machine, per question, and the machine answers the rest.
func TestRegister_UsesWhatIsGivenAndAsksTheMachineForTheRest(t *testing.T) {
	for name, tc := range map[string]struct {
		request Request
		want    call
	}{
		"everything from the machine": {
			Request{Target: Claude}, call{"register", Claude, "/home", "/state/longterm-mem", "/state/bin/longterm-mem"},
		},
		"everything given": {
			Request{Target: Claude, ConfigRoot: "/c", StateDir: "/s", Binary: "/b"}, call{"register", Claude, "/c", "/s", "/b"},
		},
		"only the binary given": {
			Request{Target: Codex, Binary: "/b"}, call{"register", Codex, "/home/.codex", "/state/longterm-mem", "/b"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			registrar := &fakeRegistrar{}

			report := using(registrar, aMachine()).Register(tc.request)

			if report.Refusal != nil || !reflect.DeepEqual(registrar.calls, []call{tc.want}) {
				t.Errorf("registrar was asked %v (refusal %+v), want %v", registrar.calls, report.Refusal, tc.want)
			}
			if len(report.Results) != 1 || report.Results[0].Outcome != Done || report.Results[0].Root != tc.want.configRoot {
				t.Errorf("Results = %+v, want one Done at the root %q", report.Results, tc.want.configRoot)
			}
		})
	}
}

// All is every runtime that has a configuration file of its own, in the order they are listed, each attempted with
// the root of its own, whatever happened to the ones before it; pi is not among them unless it is installed, and
// when it is it is the last.
func TestRegister_AllAttemptsEveryRuntimeInOrder(t *testing.T) {
	t.Run("pi not installed", func(t *testing.T) {
		registrar := &fakeRegistrar{registerErr: map[string]error{Claude: errors.New("boom")}}
		machine := aMachine()

		report := using(registrar, machine).Register(Request{Target: All})

		if want := []string{Claude, Opencode, Codex}; !reflect.DeepEqual(targetsOf(report.Results), want) {
			t.Errorf("attempted %v, want %v: a failure does not stop the others", targetsOf(report.Results), want)
		}
		wantRoots := []string{"/home", "/home/.config/opencode", "/home/.codex"}
		for i, result := range report.Results {
			if result.Root != wantRoots[i] {
				t.Errorf("%s was attempted at %q, want %q", result.Target, result.Root, wantRoots[i])
			}
		}
		if !reflect.DeepEqual(machine.piProbes, []string{"/state/pi/labdrian-pi"}) {
			t.Errorf("pi was probed at %v, want once, at the package directory of the machine", machine.piProbes)
		}
	})

	t.Run("pi installed", func(t *testing.T) {
		registrar := &fakeRegistrar{}
		machine := aMachine()
		machine.installedPi = "/state/pi/labdrian-pi"

		report := using(registrar, machine).Register(Request{Target: All})

		if want := []string{Claude, Opencode, Codex, Pi}; !reflect.DeepEqual(targetsOf(report.Results), want) {
			t.Errorf("attempted %v, want %v", targetsOf(report.Results), want)
		}
	})

	t.Run("pi, by name, where it is installed", func(t *testing.T) {
		registrar := &fakeRegistrar{}
		machine := aMachine()
		machine.installedPi = "/pkg"

		report := using(registrar, machine).Register(Request{Target: Pi, ConfigRoot: "/pkg"})

		want := []call{{"register", Pi, "/pkg", "/state/longterm-mem", "/state/bin/longterm-mem"}}
		if report.Refusal != nil || !reflect.DeepEqual(registrar.calls, want) {
			t.Errorf("registrar was asked %v (refusal %+v), want %v", registrar.calls, report.Refusal, want)
		}
	})
}

// A runtime whose root cannot be told is not attempted, and is told apart from one that was attempted and failed:
// the remedy is the caller's environment, not the runtime's configuration.
func TestRegister_ARuntimeWithNoRootIsNotAttempted(t *testing.T) {
	registrar := &fakeRegistrar{}
	machine := aMachine()
	delete(machine.roots, Opencode)

	report := using(registrar, machine).Register(Request{Target: All})

	want := []Outcome{Done, RootUnresolved, Done}
	if !reflect.DeepEqual(outcomesOf(report.Results), want) {
		t.Errorf("outcomes = %v, want %v", outcomesOf(report.Results), want)
	}
	if !reflect.DeepEqual(targetsOf(report.Results), []string{Claude, Opencode, Codex}) {
		t.Errorf("results are for %v, want every runtime, the one with no root included", targetsOf(report.Results))
	}
	for _, c := range registrar.calls {
		if c.target == Opencode {
			t.Errorf("opencode was attempted though no root could be told: %+v", c)
		}
	}
}

// What the registrar says of a runtime is one of four things, and which is decided by its error and by whether the
// run was --target all: the entry written, a configuration that is not there (skipped, but only when the run asked
// for every runtime and so for those that exist), an entry that is not ours, or a failure.
func TestRegister_ClassifiesWhatTheRegistrarSays(t *testing.T) {
	notThere := fmt.Errorf("register: claude: read /x: %w", fs.ErrNotExist)
	conflict := fmt.Errorf("register: claude: /x: %w", ErrConflict)
	both := fmt.Errorf("%w and %w", ErrConflict, fs.ErrNotExist)
	boom := errors.New("boom")

	for name, tc := range map[string]struct {
		request Request
		err     error
		want    Outcome
	}{
		"written":                         {Request{Target: Claude}, nil, Done},
		"an entry that is not ours":       {Request{Target: Claude}, conflict, NotOurs},
		"a failure":                       {Request{Target: Claude}, boom, Failed},
		"no configuration, asked by name": {Request{Target: Claude}, notThere, Failed},
		"no configuration, of every one":  {Request{Target: All}, notThere, Skipped},
		"a conflict is a conflict even where a missing file is a skip": {Request{Target: All}, both, NotOurs},
		"a failure of every one": {Request{Target: All}, boom, Failed},
	} {
		t.Run(name, func(t *testing.T) {
			registrar := &fakeRegistrar{registerErr: map[string]error{Claude: tc.err}}

			report := using(registrar, aMachine()).Register(tc.request)

			if got := report.Results[0]; got.Target != Claude || got.Outcome != tc.want {
				t.Fatalf("first result = %+v, want claude %v", got, tc.want)
			}
			if tc.err != nil && tc.want != Skipped && !errors.Is(report.Results[0].Err, tc.err) {
				t.Errorf("Err = %v, want the registrar's error to be carried", report.Results[0].Err)
			}
			if tc.want == Skipped && report.Results[0].Err != nil {
				t.Errorf("a skipped runtime carries the error %v, want none", report.Results[0].Err)
			}
		})
	}
}

// The failure of a run is one verdict among the outcomes of the targets, and the harder outranks the softer: a
// target attempted and broken, then a path that could not be told (nothing attempted, the environment's fault),
// then an entry that is not ours (recoverable by hand). Skipped runtimes and written ones are not failures.
func TestReport_FailureIsTheHardestOutcome(t *testing.T) {
	for name, tc := range map[string]struct {
		outcomes []Outcome
		want     Failure
	}{
		"nothing attempted":                       {nil, NoFailure},
		"all written":                             {[]Outcome{Done, Done}, NoFailure},
		"written and skipped":                     {[]Outcome{Done, Skipped, Skipped}, NoFailure},
		"an entry that is not ours":               {[]Outcome{Done, NotOurs}, SomeEntryNotOurs},
		"a path that could not be told":           {[]Outcome{Done, RootUnresolved}, SomePathUnresolved},
		"a failure":                               {[]Outcome{Done, Failed}, SomeTargetFailed},
		"a path beside an entry that is not ours": {[]Outcome{NotOurs, RootUnresolved}, SomePathUnresolved},
		"a failure beside a path":                 {[]Outcome{RootUnresolved, Failed}, SomeTargetFailed},
		"a failure beside an entry not ours":      {[]Outcome{NotOurs, Failed}, SomeTargetFailed},
		"all three, in the order they were met":   {[]Outcome{NotOurs, RootUnresolved, Failed}, SomeTargetFailed},
		"all three, the other way":                {[]Outcome{Failed, RootUnresolved, NotOurs}, SomeTargetFailed},
	} {
		t.Run(name, func(t *testing.T) {
			var report Report
			for _, outcome := range tc.outcomes {
				report.Results = append(report.Results, Result{Outcome: outcome})
			}
			if got := report.Failure(); got != tc.want {
				t.Errorf("Failure = %v, want %v", got, tc.want)
			}
		})
	}
}

// Unregister asks fewer questions than Register (it writes no entry, so it needs no binary, and pi is not part of
// all because it is never in the list a machine's configuration files make), in the same order.
func TestUnregister_RefusesWhatIsRefusedAndAsksNoMore(t *testing.T) {
	noState := func() *fakeMachine { m := aMachine(); m.stateDir = ""; return m }
	noBinary := func() *fakeMachine { m := aMachine(); m.binary = ""; return m }

	for name, tc := range map[string]struct {
		request Request
		machine *fakeMachine
		want    *Refusal
	}{
		"no target":              {Request{}, aMachine(), &Refusal{Kind: TargetRequired}},
		"an unknown target":      {Request{Target: "bogus"}, aMachine(), &Refusal{Kind: UnknownTarget, Target: "bogus"}},
		"all with a config root": {Request{Target: All, ConfigRoot: "/x"}, aMachine(), &Refusal{Kind: ConfigRootNeedsOneTarget}},
		"no state directory":     {Request{Target: Claude}, noState(), &Refusal{Kind: StateDirUnresolved}},
		"no binary is no matter": {Request{Target: Claude}, noBinary(), nil},
		"pi is not asked about":  {Request{Target: Pi}, aMachine(), nil},
	} {
		t.Run(name, func(t *testing.T) {
			report := using(&fakeRegistrar{}, tc.machine).Unregister(tc.request)

			switch {
			case tc.want == nil && report.Refusal != nil:
				t.Errorf("Refusal = %+v, want none", report.Refusal)
			case tc.want != nil && (report.Refusal == nil || *report.Refusal != *tc.want):
				t.Errorf("Refusal = %+v, want %+v", report.Refusal, *tc.want)
			}
		})
	}
}

func TestUnregister_AllIsEveryRuntimeWithAConfigurationFileAndNeverPi(t *testing.T) {
	registrar := &fakeRegistrar{}
	machine := aMachine()
	machine.installedPi = "/state/pi/labdrian-pi"

	report := using(registrar, machine).Unregister(Request{Target: All})

	if want := []string{Claude, Opencode, Codex}; !reflect.DeepEqual(targetsOf(report.Results), want) {
		t.Errorf("attempted %v, want %v", targetsOf(report.Results), want)
	}
	if len(machine.piProbes) != 0 {
		t.Errorf("pi was probed at %v, want no probe: unregister of a runtime that is not there is a no-op, not a skip", machine.piProbes)
	}
	want := []call{
		{"unregister", Claude, "/home", "/state/longterm-mem", ""},
		{"unregister", Opencode, "/home/.config/opencode", "/state/longterm-mem", ""},
		{"unregister", Codex, "/home/.codex", "/state/longterm-mem", ""},
	}
	if !reflect.DeepEqual(registrar.calls, want) {
		t.Errorf("registrar was asked %v, want %v", registrar.calls, want)
	}
}

// What was taken away is what the registrar says: nothing to do and removed are both done, an entry that is not
// ours is left and is not ours, and a runtime whose configuration is not there is no skip (it is a no-op for the
// registrar to say), so a failure is a failure whichever way the request named its targets.
func TestUnregister_ClassifiesWhatTheRegistrarSays(t *testing.T) {
	boom := errors.New("boom")
	notThere := fmt.Errorf("unregister: claude: %w", fs.ErrNotExist)

	for name, tc := range map[string]struct {
		request Request
		removal Removal
		err     error
		want    Outcome
	}{
		"nothing to do":         {Request{Target: Claude}, Noop, nil, Done},
		"removed":               {Request{Target: Claude}, Removed, nil, Done},
		"an entry not ours":     {Request{Target: Claude}, Unmanaged, nil, NotOurs},
		"a failure":             {Request{Target: Claude}, Noop, boom, Failed},
		"a missing file of all": {Request{Target: All}, Noop, notThere, Failed},
	} {
		t.Run(name, func(t *testing.T) {
			registrar := &fakeRegistrar{removal: map[string]Removal{Claude: tc.removal}, unregisterErr: map[string]error{Claude: tc.err}}

			report := using(registrar, aMachine()).Unregister(tc.request)

			got := report.Results[0]
			if got.Outcome != tc.want || (tc.want == Done && got.Removal != tc.removal) {
				t.Errorf("first result = %+v, want %v with removal %v", got, tc.want, tc.removal)
			}
			if tc.err != nil && !errors.Is(got.Err, tc.err) {
				t.Errorf("Err = %v, want the registrar's error to be carried", got.Err)
			}
		})
	}
}

func TestUnregister_ARuntimeWithNoRootIsNotAttempted(t *testing.T) {
	registrar := &fakeRegistrar{}
	machine := aMachine()
	machine.roots = nil

	report := using(registrar, machine).Unregister(Request{Target: All})

	if want := []Outcome{RootUnresolved, RootUnresolved, RootUnresolved}; !reflect.DeepEqual(outcomesOf(report.Results), want) {
		t.Errorf("outcomes = %v, want %v", outcomesOf(report.Results), want)
	}
	if len(registrar.calls) != 0 {
		t.Errorf("the registrar was asked %v, want nothing", registrar.calls)
	}
	if got := report.Failure(); got != SomePathUnresolved {
		t.Errorf("Failure = %v, want SomePathUnresolved", got)
	}
}

func TestRemoval_IsSaidInTheWordsTheCommandPrints(t *testing.T) {
	for removal, want := range map[Removal]string{Noop: "noop", Removed: "removed", Unmanaged: "unmanaged", Removal(99): "unknown"} {
		if got := removal.String(); got != want {
			t.Errorf("Removal(%d).String() = %q, want %q", int(removal), got, want)
		}
	}
}

// The refusal of an entry that is not ours says what to do about it, and nothing about who asked: the command that
// prints it puts its own name and the target's in front.
func TestErrConflict_SaysWhatToDo(t *testing.T) {
	for _, want := range []string{"not owned by longterm-mem", "remove that entry by hand", "run register again"} {
		if !strings.Contains(ErrConflict.Error(), want) {
			t.Errorf("ErrConflict = %q, want it to say %q", ErrConflict, want)
		}
	}
	if strings.HasPrefix(ErrConflict.Error(), "register:") {
		t.Errorf("ErrConflict = %q, which names the command its wrapper names too", ErrConflict)
	}
}
