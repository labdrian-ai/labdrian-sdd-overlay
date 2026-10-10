package core

import (
	"testing"
)

// recordingAdapter answers every action with a result that names the action, and records what it
// was asked, so a test can tell which method Perform called.
type recordingAdapter struct {
	target Target
	asked  []string
}

func (a *recordingAdapter) Target() Target { return a.target }

func (a *recordingAdapter) answer(action Action) LifecycleResult {
	a.asked = append(a.asked, string(action))
	return NewLifecycleResult(a.target, action, CapabilitySupported, "asked "+string(action), nil)
}

func (a *recordingAdapter) Apply() LifecycleResult     { return a.answer(ActionApply) }
func (a *recordingAdapter) Install() LifecycleResult   { return a.answer(ActionInstall) }
func (a *recordingAdapter) Status() LifecycleResult    { return a.answer(ActionStatus) }
func (a *recordingAdapter) SyncCheck() LifecycleResult { return a.answer(ActionSyncCheck) }
func (a *recordingAdapter) Update() LifecycleResult    { return a.answer(ActionUpdate) }
func (a *recordingAdapter) Rollback() LifecycleResult  { return a.answer(ActionRollback) }
func (a *recordingAdapter) Uninstall() LifecycleResult { return a.answer(ActionUninstall) }

func TestPerformAsksTheMethodOfTheAction(t *testing.T) {
	for _, action := range []Action{ActionApply, ActionInstall, ActionStatus, ActionSyncCheck, ActionUpdate, ActionRollback, ActionUninstall} {
		adapter := &recordingAdapter{target: TargetClaude}
		got := Perform(adapter, action)
		if len(adapter.asked) != 1 || adapter.asked[0] != string(action) {
			t.Errorf("Perform(%s) asked %q, want exactly %q", action, adapter.asked, action)
		}
		if got.Action != action || got.Message != "asked "+string(action) {
			t.Errorf("Perform(%s) = %+v, want the adapter's own answer", action, got)
		}
	}
}

func TestPerformAnswersAnActionItDoesNotKnowAsUnsupportedWithoutAskingTheAdapter(t *testing.T) {
	adapter := &recordingAdapter{target: TargetCodex}
	got := Perform(adapter, Action("rewind"))
	if len(adapter.asked) != 0 {
		t.Errorf("an unknown action reached the adapter: %q", adapter.asked)
	}
	if got.Target != TargetCodex || got.Status != CapabilityUnsupported || got.Message != "unknown runtime action" || got.Reasons != nil {
		t.Errorf("Perform(unknown) = %+v, want the unsupported answer of the adapter's target", got)
	}
	// The action it names is status, as the command has always printed it.
	if got.String() != "[codex] status: unsupported — unknown runtime action" {
		t.Errorf("the line it prints = %q", got.String())
	}
}

func result(target Target, status CapabilityStatus) LifecycleResult {
	return NewLifecycleResult(target, ActionStatus, status, "", nil)
}

func TestAggregateStatusOfAStatusRunFailsOnAnyStatusButSupportedAndCodexPartialAmongSeveral(t *testing.T) {
	cases := []struct {
		name    string
		results []LifecycleResult
		want    bool
	}{
		{"nothing ran", nil, false},
		{"one supported", []LifecycleResult{result(TargetClaude, CapabilitySupported)}, false},
		{"one restart required", []LifecycleResult{result(TargetClaude, CapabilityRestartRequired)}, true},
		{"one unsupported", []LifecycleResult{result(TargetClaude, CapabilityUnsupported)}, true},
		{"one partial", []LifecycleResult{result(TargetClaude, CapabilityPartial)}, true},
		{"codex alone, partial: the exemption is for a run over several", []LifecycleResult{result(TargetCodex, CapabilityPartial)}, true},
		{"codex partial among supported ones", []LifecycleResult{result(TargetClaude, CapabilitySupported), result(TargetCodex, CapabilityPartial)}, false},
		{"another target partial among supported ones", []LifecycleResult{result(TargetClaude, CapabilitySupported), result(TargetOpenCode, CapabilityPartial)}, true},
		{"codex partial and another unsupported", []LifecycleResult{result(TargetCodex, CapabilityPartial), result(TargetPi, CapabilityUnsupported)}, true},
		{"codex unsupported is not exempt", []LifecycleResult{result(TargetClaude, CapabilitySupported), result(TargetCodex, CapabilityUnsupported)}, true},
		{"codex restart required is not exempt", []LifecycleResult{result(TargetClaude, CapabilitySupported), result(TargetCodex, CapabilityRestartRequired)}, true},
		{"the failing one is the last", []LifecycleResult{result(TargetClaude, CapabilitySupported), result(TargetOpenCode, CapabilitySupported), result(TargetPi, CapabilityRestartRequired)}, true},
		{"a status the vocabulary does not have", []LifecycleResult{result(TargetClaude, CapabilityStatus("unknown"))}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := AggregateStatus(ActionStatus, c.results); got != c.want {
				t.Errorf("AggregateStatus(status, %v) = %v, want %v", c.results, got, c.want)
			}
		})
	}
}

func TestAggregateStatusOfAnyOtherActionFailsOnPartialAndUnsupportedOnly(t *testing.T) {
	for _, action := range []Action{ActionInstall, ActionUpdate, ActionUninstall, ActionApply, ActionRollback, ActionSyncCheck} {
		for _, c := range []struct {
			status CapabilityStatus
			want   bool
		}{
			{CapabilitySupported, false},
			{CapabilityRestartRequired, false},
			{CapabilityPartial, true},
			{CapabilityUnsupported, true},
		} {
			one := []LifecycleResult{NewLifecycleResult(TargetClaude, action, c.status, "", nil)}
			if got := AggregateStatus(action, one); got != c.want {
				t.Errorf("AggregateStatus(%s, %s) = %v, want %v", action, c.status, got, c.want)
			}
		}
		// The exemption of a partial codex belongs to status alone.
		several := []LifecycleResult{
			NewLifecycleResult(TargetClaude, action, CapabilitySupported, "", nil),
			NewLifecycleResult(TargetCodex, action, CapabilityPartial, "", nil),
		}
		if !AggregateStatus(action, several) {
			t.Errorf("AggregateStatus(%s) let a partial codex pass among several; only status does", action)
		}
	}
}

func TestAComponentFailsAStatusRunUnlessSupportedAndAnyOtherRunOnPartialOrUnsupported(t *testing.T) {
	for _, status := range []CapabilityStatus{CapabilitySupported, CapabilityPartial, CapabilityUnsupported, CapabilityRestartRequired, CapabilityStatus("unknown")} {
		want := status != CapabilitySupported
		if got := ComponentFailed(ActionStatus, NewLifecycleResult("longterm-mem", ActionStatus, status, "", nil)); got != want {
			t.Errorf("ComponentFailed(status, %s) = %v, want %v", status, got, want)
		}
	}
	for _, c := range []struct {
		status CapabilityStatus
		want   bool
	}{
		{CapabilitySupported, false},
		{CapabilityRestartRequired, false},
		{CapabilityStatus("unknown"), false},
		{CapabilityPartial, true},
		{CapabilityUnsupported, true},
	} {
		for _, action := range []Action{ActionInstall, ActionUninstall} {
			if got := ComponentFailed(action, NewLifecycleResult("longterm-mem", action, c.status, "", nil)); got != c.want {
				t.Errorf("ComponentFailed(%s, %s) = %v, want %v", action, c.status, got, c.want)
			}
		}
	}
}
