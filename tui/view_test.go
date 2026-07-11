package main

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Phase 4: TUI rendering coverage for REPO_BEHIND_ORIGIN (R-005/R-006).
//
// Both PR1 judgment-day rounds flagged this as a coverage gap: view.go's
// SyncBehindOrigin case and the "detrás de origin" counts-line append had no
// dedicated test asserting the rendering contract. These tests close that gap.
// ---------------------------------------------------------------------------

// TestViewDashboard_ShowsOriginBehindIndicator verifies that a target with
// RepoBehindOrigin > 0 renders a distinct origin-behind indicator: the
// "Detrás de origin" status label AND the "detrás de origin: N" count
// (R-005 Scenario: distinct indicator rendered).
func TestViewDashboard_ShowsOriginBehindIndicator(t *testing.T) {
	m := newModel()
	m.width = 80
	m.scr = screenResult
	m.result = commandResult{
		action: Action{Name: "Verificar sincronización", Command: "sync-check"},
		verdicts: []TargetVerdict{
			{
				Target:           "claude",
				Status:           SyncBehindOrigin,
				RepoBehindOrigin: 2,
				Action:           "run 'git pull' to update your local clone",
			},
		},
		output: "ok",
	}

	rendered := stripANSI(m.viewDashboard())
	if !strings.Contains(rendered, "Detrás de origin") {
		t.Errorf("viewDashboard must show 'Detrás de origin' status label when RepoBehindOrigin>0, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "detrás de origin: 2") {
		t.Errorf("viewDashboard must show 'detrás de origin: 2' count, got:\n%s", rendered)
	}
}

// TestViewDashboard_ZeroOriginBehind_NoIndicator verifies that a target with
// RepoBehindOrigin == 0 does NOT render the origin-behind count, even though
// the value is "known" (R-005 Scenario: zero count renders no origin
// indicator).
func TestViewDashboard_ZeroOriginBehind_NoIndicator(t *testing.T) {
	m := newModel()
	m.width = 80
	m.scr = screenResult
	m.result = commandResult{
		action: Action{Name: "Verificar sincronización", Command: "sync-check"},
		verdicts: []TargetVerdict{
			{
				Target:           "claude",
				Status:           SyncHealthy,
				RepoBehindOrigin: 0,
				Action:           "in sync with gentle-ai (healthy)",
			},
		},
		output: "ok",
	}

	rendered := stripANSI(m.viewDashboard())
	if strings.Contains(rendered, "detrás de origin") {
		t.Errorf("viewDashboard must NOT show origin-behind count when RepoBehindOrigin==0, got:\n%s", rendered)
	}
}

// TestViewDashboard_NeverHealthyWhileBehindOrigin verifies R-006: a target
// with UPSTREAM_CHANGED=0, OVERLAY_NOT_DEPLOYED=0, REPO_BEHIND_ORIGIN>0 (i.e.
// classify() returns SyncBehindOrigin) must never render as "Sincronizado"
// (healthy) — it must render its own distinct status instead.
func TestViewDashboard_NeverHealthyWhileBehindOrigin(t *testing.T) {
	m := newModel()
	m.width = 80
	m.scr = screenResult
	m.result = commandResult{
		action: Action{Name: "Verificar sincronización", Command: "sync-check"},
		verdicts: []TargetVerdict{
			{
				Target:             "claude",
				Status:             classify(0, 0, 3), // == SyncBehindOrigin per R-006
				UpstreamChanged:    0,
				OverlayNotDeployed: 0,
				RepoBehindOrigin:   3,
				Action:             "run 'git pull' to update your local clone",
			},
		},
		output: "ok",
	}

	rendered := stripANSI(m.viewDashboard())
	if strings.Contains(rendered, "Sincronizado") {
		t.Errorf("viewDashboard must NOT present a target as healthy while RepoBehindOrigin>0, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "detrás de origin: 3") {
		t.Errorf("viewDashboard must keep the REPO_BEHIND_ORIGIN count visible (R-006), got:\n%s", rendered)
	}
}
