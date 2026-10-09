package core_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

// fakeAdapter is an adapter that answers for target and does nothing else.
type fakeAdapter struct{ target core.Target }

func (a fakeAdapter) Target() core.Target { return a.target }
func (fakeAdapter) Apply() core.LifecycleResult {
	return core.LifecycleResult{}
}
func (fakeAdapter) Install() core.LifecycleResult {
	return core.LifecycleResult{}
}
func (fakeAdapter) Status() core.LifecycleResult {
	return core.LifecycleResult{}
}
func (fakeAdapter) SyncCheck() core.LifecycleResult {
	return core.LifecycleResult{}
}
func (fakeAdapter) Update() core.LifecycleResult {
	return core.LifecycleResult{}
}
func (fakeAdapter) Rollback() core.LifecycleResult {
	return core.LifecycleResult{}
}
func (fakeAdapter) Uninstall() core.LifecycleResult {
	return core.LifecycleResult{}
}

func factoryOf(target core.Target) core.Factory {
	return func(core.Config) core.Adapter { return fakeAdapter{target: target} }
}

func mustRegister(t *testing.T, r *core.Registry, target core.Target) {
	t.Helper()
	if err := r.Register(target, factoryOf(target)); err != nil {
		t.Fatalf("Register(%q): %v", target, err)
	}
}

// longtermMemLabel is the Target the longterm-mem component answers under. It names no runtime, so
// a registry refuses it like any other name capability does not declare.
const longtermMemLabel core.Target = "longterm-mem"

func TestRegisterRefusesWhatCapabilityDoesNotDeclare(t *testing.T) {
	r := core.NewRegistry()
	for _, target := range []core.Target{"", "all", "cursor", "Claude", longtermMemLabel} {
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
	r := core.NewRegistry()
	if err := r.Register(core.TargetClaude, nil); err == nil || !strings.Contains(err.Error(), "no factory") {
		t.Errorf("Register with a nil factory = %v, want a refusal naming the missing factory", err)
	}
	mustRegister(t, r, core.TargetClaude)
	if err := r.Register(core.TargetClaude, factoryOf(core.TargetClaude)); err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Errorf("a second Register of claude = %v, want a refusal naming it as already registered", err)
	}
	if got := r.Targets(); !reflect.DeepEqual(got, []core.Target{core.TargetClaude}) {
		t.Errorf("Targets() = %v, want only claude: the refused registrations add nothing", got)
	}
}

func TestNewBuildsTheAdapterOfARegisteredTargetFromTheConfig(t *testing.T) {
	r := core.NewRegistry()
	var seen core.Config
	err := r.Register(core.TargetCodex, func(cfg core.Config) core.Adapter {
		seen = cfg
		return fakeAdapter{target: core.TargetCodex}
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg := core.Config{Home: "/h", ConfigRoot: "/root"}
	adapter, err := r.New(core.TargetCodex, cfg)
	if err != nil {
		t.Fatalf("New(codex): %v", err)
	}
	if adapter.Target() != core.TargetCodex {
		t.Errorf("New(codex).Target() = %q", adapter.Target())
	}
	if seen != cfg {
		t.Errorf("the factory was given %+v, want the Config New was given: %+v", seen, cfg)
	}
}

func TestNewRefusesAnUnregisteredTargetAndAll(t *testing.T) {
	r := core.NewRegistry()
	mustRegister(t, r, core.TargetClaude)
	for _, target := range []core.Target{core.TargetPi, core.TargetAll, "future"} {
		adapter, err := r.New(target, core.Config{})
		if err == nil || !strings.Contains(err.Error(), "not registered") {
			t.Errorf("New(%q) = %v, %v, want an error naming it as not registered", target, adapter, err)
		}
		if adapter != nil {
			t.Errorf("New(%q) returned an adapter %v along with its error", target, adapter)
		}
	}
}

func TestNewRefusesAFactoryThatBuildsTheWrongAdapter(t *testing.T) {
	r := core.NewRegistry()
	if err := r.Register(core.TargetClaude, factoryOf(core.TargetPi)); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(core.TargetCodex, func(core.Config) core.Adapter { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := r.New(core.TargetClaude, core.Config{}); err == nil || !strings.Contains(err.Error(), `adapter of "pi"`) {
		t.Errorf("New(claude) with a factory that builds pi = %v, want an error naming pi", err)
	}
	if _, err := r.New(core.TargetCodex, core.Config{}); err == nil || !strings.Contains(err.Error(), "built no adapter") {
		t.Errorf("New(codex) with a factory that builds nothing = %v, want an error saying so", err)
	}
}

func TestParseAcceptsARegisteredTargetOrAllIgnoringSpace(t *testing.T) {
	r := core.NewRegistry()
	mustRegister(t, r, core.TargetClaude)
	for raw, want := range map[string]core.Target{
		"claude":    core.TargetClaude,
		"  claude ": core.TargetClaude,
		"all":       core.TargetAll,
		" all":      core.TargetAll,
	} {
		got, err := r.Parse(raw)
		if err != nil || got != want {
			t.Errorf("Parse(%q) = %q, %v, want %q", raw, got, err, want)
		}
	}
}

func TestParseRefusesWhatIsNotRegistered(t *testing.T) {
	r := core.NewRegistry()
	mustRegister(t, r, core.TargetClaude)
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
	r := core.NewRegistry()
	for _, target := range []core.Target{core.TargetPi, core.TargetClaude, core.TargetCodex} {
		mustRegister(t, r, target)
	}
	want := []core.Target{core.TargetPi, core.TargetClaude, core.TargetCodex}
	if got := r.Expand(core.TargetAll); !reflect.DeepEqual(got, want) {
		t.Errorf("Expand(all) = %v, want %v", got, want)
	}
	if got := r.Expand(core.TargetClaude); !reflect.DeepEqual(got, []core.Target{core.TargetClaude}) {
		t.Errorf("Expand(claude) = %v, want [claude]", got)
	}
	if got := core.NewRegistry().Expand(core.TargetAll); len(got) != 0 {
		t.Errorf("Expand(all) of an empty registry = %v, want none", got)
	}
}

func TestTargetsHandsOutAListOfItsOwn(t *testing.T) {
	r := core.NewRegistry()
	mustRegister(t, r, core.TargetClaude)
	first := r.Targets()
	first[0] = "tampered"
	if got := r.Targets()[0]; got != core.TargetClaude {
		t.Errorf("Targets()[0] = %q after a caller changed the list it was given, want claude", got)
	}
	expanded := r.Expand(core.TargetAll)
	expanded[0] = "tampered"
	if got := r.Targets()[0]; got != core.TargetClaude {
		t.Errorf("Targets()[0] = %q after a caller changed Expand(all), want claude", got)
	}
}

// TestTwoRegistriesShareNothing: registering in one registry is invisible to another, which is
// why no registry is a package-level variable.
func TestTwoRegistriesShareNothing(t *testing.T) {
	a, b := core.NewRegistry(), core.NewRegistry()
	mustRegister(t, a, core.TargetClaude)
	if _, err := b.Parse("claude"); err == nil {
		t.Error("a second registry parses claude, which only the first registered")
	}
}
