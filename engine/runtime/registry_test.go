package runtime_test

import (
	"reflect"
	"strings"
	"testing"

	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
)

// fakeAdapter is an adapter that answers for target and does nothing else.
type fakeAdapter struct{ target engineRuntime.Target }

func (a fakeAdapter) Target() engineRuntime.Target { return a.target }
func (fakeAdapter) Apply() engineRuntime.LifecycleResult {
	return engineRuntime.LifecycleResult{}
}
func (fakeAdapter) Install() engineRuntime.LifecycleResult {
	return engineRuntime.LifecycleResult{}
}
func (fakeAdapter) Status() engineRuntime.LifecycleResult {
	return engineRuntime.LifecycleResult{}
}
func (fakeAdapter) SyncCheck() engineRuntime.LifecycleResult {
	return engineRuntime.LifecycleResult{}
}
func (fakeAdapter) Update() engineRuntime.LifecycleResult {
	return engineRuntime.LifecycleResult{}
}
func (fakeAdapter) Rollback() engineRuntime.LifecycleResult {
	return engineRuntime.LifecycleResult{}
}
func (fakeAdapter) Uninstall() engineRuntime.LifecycleResult {
	return engineRuntime.LifecycleResult{}
}

func factoryOf(target engineRuntime.Target) engineRuntime.Factory {
	return func(engineRuntime.Config) engineRuntime.Adapter { return fakeAdapter{target: target} }
}

func mustRegister(t *testing.T, r *engineRuntime.Registry, target engineRuntime.Target) {
	t.Helper()
	if err := r.Register(target, factoryOf(target)); err != nil {
		t.Fatalf("Register(%q): %v", target, err)
	}
}

func TestRegisterRefusesWhatCapabilityDoesNotDeclare(t *testing.T) {
	r := engineRuntime.NewRegistry()
	for _, target := range []engineRuntime.Target{"", "all", "cursor", "Claude", engineRuntime.TargetLongtermMem} {
		err := r.Register(target, factoryOf(target))
		if err == nil || !strings.Contains(err.Error(), "not a declared target") {
			t.Errorf("Register(%q) = %v, want a refusal naming it as not a declared target", target, err)
		}
	}
	if got := r.Targets(); len(got) != 0 {
		t.Errorf("Targets() = %v after only refused registrations, want none", got)
	}
}

func TestRegisterRefusesANilFactoryAndADuplicate(t *testing.T) {
	r := engineRuntime.NewRegistry()
	if err := r.Register(engineRuntime.TargetClaude, nil); err == nil || !strings.Contains(err.Error(), "no factory") {
		t.Errorf("Register with a nil factory = %v, want a refusal naming the missing factory", err)
	}
	mustRegister(t, r, engineRuntime.TargetClaude)
	if err := r.Register(engineRuntime.TargetClaude, factoryOf(engineRuntime.TargetClaude)); err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Errorf("a second Register of claude = %v, want a refusal naming it as already registered", err)
	}
	if got := r.Targets(); !reflect.DeepEqual(got, []engineRuntime.Target{engineRuntime.TargetClaude}) {
		t.Errorf("Targets() = %v, want only claude: the refused registrations add nothing", got)
	}
}

func TestNewBuildsTheAdapterOfARegisteredTargetFromTheConfig(t *testing.T) {
	r := engineRuntime.NewRegistry()
	var seen engineRuntime.Config
	err := r.Register(engineRuntime.TargetCodex, func(cfg engineRuntime.Config) engineRuntime.Adapter {
		seen = cfg
		return fakeAdapter{target: engineRuntime.TargetCodex}
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg := engineRuntime.Config{Home: "/h", ConfigRoot: "/root"}
	adapter, err := r.New(engineRuntime.TargetCodex, cfg)
	if err != nil {
		t.Fatalf("New(codex): %v", err)
	}
	if adapter.Target() != engineRuntime.TargetCodex {
		t.Errorf("New(codex).Target() = %q", adapter.Target())
	}
	if seen != cfg {
		t.Errorf("the factory was given %+v, want the Config New was given: %+v", seen, cfg)
	}
}

func TestNewRefusesAnUnregisteredTargetAndAll(t *testing.T) {
	r := engineRuntime.NewRegistry()
	mustRegister(t, r, engineRuntime.TargetClaude)
	for _, target := range []engineRuntime.Target{engineRuntime.TargetPi, engineRuntime.TargetAll, "future"} {
		adapter, err := r.New(target, engineRuntime.Config{})
		if err == nil || !strings.Contains(err.Error(), "not registered") {
			t.Errorf("New(%q) = %v, %v, want an error naming it as not registered", target, adapter, err)
		}
		if adapter != nil {
			t.Errorf("New(%q) returned an adapter %v along with its error", target, adapter)
		}
	}
}

func TestNewRefusesAFactoryThatBuildsTheWrongAdapter(t *testing.T) {
	r := engineRuntime.NewRegistry()
	if err := r.Register(engineRuntime.TargetClaude, factoryOf(engineRuntime.TargetPi)); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(engineRuntime.TargetCodex, func(engineRuntime.Config) engineRuntime.Adapter { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := r.New(engineRuntime.TargetClaude, engineRuntime.Config{}); err == nil || !strings.Contains(err.Error(), `adapter of "pi"`) {
		t.Errorf("New(claude) with a factory that builds pi = %v, want an error naming pi", err)
	}
	if _, err := r.New(engineRuntime.TargetCodex, engineRuntime.Config{}); err == nil || !strings.Contains(err.Error(), "built no adapter") {
		t.Errorf("New(codex) with a factory that builds nothing = %v, want an error saying so", err)
	}
}

func TestParseAcceptsARegisteredTargetOrAllIgnoringSpace(t *testing.T) {
	r := engineRuntime.NewRegistry()
	mustRegister(t, r, engineRuntime.TargetClaude)
	for raw, want := range map[string]engineRuntime.Target{
		"claude":    engineRuntime.TargetClaude,
		"  claude ": engineRuntime.TargetClaude,
		"all":       engineRuntime.TargetAll,
		" all":      engineRuntime.TargetAll,
	} {
		got, err := r.Parse(raw)
		if err != nil || got != want {
			t.Errorf("Parse(%q) = %q, %v, want %q", raw, got, err, want)
		}
	}
}

func TestParseRefusesWhatIsNotRegistered(t *testing.T) {
	r := engineRuntime.NewRegistry()
	mustRegister(t, r, engineRuntime.TargetClaude)
	// pi is a declared target but this registry has not registered it: it is as unknown as a name
	// nobody declared.
	for _, raw := range []string{"pi", "future-cli", "", "Claude", "longterm-mem"} {
		got, err := r.Parse(raw)
		if err == nil || !strings.Contains(err.Error(), `unknown target "`+raw+`"`) {
			t.Errorf("Parse(%q) = %q, %v, want an error naming the target it refused", raw, got, err)
		}
	}
}

func TestExpandListsTheRegisteredTargetsInRegistrationOrder(t *testing.T) {
	r := engineRuntime.NewRegistry()
	for _, target := range []engineRuntime.Target{engineRuntime.TargetPi, engineRuntime.TargetClaude, engineRuntime.TargetCodex} {
		mustRegister(t, r, target)
	}
	want := []engineRuntime.Target{engineRuntime.TargetPi, engineRuntime.TargetClaude, engineRuntime.TargetCodex}
	if got := r.Expand(engineRuntime.TargetAll); !reflect.DeepEqual(got, want) {
		t.Errorf("Expand(all) = %v, want %v", got, want)
	}
	if got := r.Expand(engineRuntime.TargetClaude); !reflect.DeepEqual(got, []engineRuntime.Target{engineRuntime.TargetClaude}) {
		t.Errorf("Expand(claude) = %v, want [claude]", got)
	}
	if got := engineRuntime.NewRegistry().Expand(engineRuntime.TargetAll); len(got) != 0 {
		t.Errorf("Expand(all) of an empty registry = %v, want none", got)
	}
}

func TestTargetsHandsOutAListOfItsOwn(t *testing.T) {
	r := engineRuntime.NewRegistry()
	mustRegister(t, r, engineRuntime.TargetClaude)
	first := r.Targets()
	first[0] = "tampered"
	if got := r.Targets()[0]; got != engineRuntime.TargetClaude {
		t.Errorf("Targets()[0] = %q after a caller changed the list it was given, want claude", got)
	}
	expanded := r.Expand(engineRuntime.TargetAll)
	expanded[0] = "tampered"
	if got := r.Targets()[0]; got != engineRuntime.TargetClaude {
		t.Errorf("Targets()[0] = %q after a caller changed Expand(all), want claude", got)
	}
}

// TestTwoRegistriesShareNothing: registering in one registry is invisible to another, which is
// why no registry is a package-level variable.
func TestTwoRegistriesShareNothing(t *testing.T) {
	a, b := engineRuntime.NewRegistry(), engineRuntime.NewRegistry()
	mustRegister(t, a, engineRuntime.TargetClaude)
	if _, err := b.Parse("claude"); err == nil {
		t.Error("a second registry parses claude, which only the first registered")
	}
}
