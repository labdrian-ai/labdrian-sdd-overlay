package ui

// How the model uses the RepoProber: it asks at launch and again after a
// self-update, in a command, and shows what it is told. What the probe reads
// is tested in package app and in the adapter.

import (
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/tui/internal/domain"
)

// TestProbeCmd_DeliversBothNumbersOfOneAnswer proves the D2 wiring: one
// question to the prober feeds both probeDoneMsg fields, so no second run is
// spent picking up the release-behind count.
func TestProbeCmd_DeliversBothNumbersOfOneAnswer(t *testing.T) {
	probe := &fakeProbe{origin: 1, release: 4}

	msg := probeCmd(probe)()

	pd, ok := msg.(probeDoneMsg)
	if !ok {
		t.Fatalf("expected probeDoneMsg, got %T", msg)
	}
	if pd.behind != 1 || pd.behindRelease != 4 {
		t.Errorf("probeDoneMsg = %+v, want behind 1 and behindRelease 4", pd)
	}
	if probe.asked != 1 {
		t.Errorf("the prober was asked %d times, want once", probe.asked)
	}
}

// TestProbeCmd_WithoutAProberThereIsNothingToAsk: a model wired without one
// (a composition mistake) issues no command and keeps its "unknown" defaults.
func TestProbeCmd_WithoutAProberThereIsNothingToAsk(t *testing.T) {
	if cmd := probeCmd(nil); cmd != nil {
		t.Error("probeCmd(nil) returned a command; there is nothing to run")
	}
}

// TestInit_AsksTheProberForHowFarTheRepoIsBehind: the launch-time probe is the
// prober's answer, delivered by the command Init issues.
func TestInit_AsksTheProberForHowFarTheRepoIsBehind(t *testing.T) {
	probe := &fakeProbe{origin: 3, release: domain.RepoBehindOriginNA}
	d := testDeps(&fakeCatalog{targets: threeCopyTargets()})
	d.Probe = probe
	m := newModel(d)

	var got *probeDoneMsg
	for _, msg := range collectMsgs(m.Init()) {
		if p, ok := msg.(probeDoneMsg); ok {
			got = &p
		}
	}
	if got == nil {
		t.Fatal("Init() must issue a command that delivers a probeDoneMsg")
	}
	if got.behind != 3 || got.behindRelease != domain.RepoBehindOriginNA {
		t.Errorf("probeDoneMsg = %+v, want the prober's answer (3 behind origin, release unknown)", got)
	}
	if probe.asked != 1 {
		t.Errorf("the prober was asked %d times at launch, want exactly once", probe.asked)
	}
}
