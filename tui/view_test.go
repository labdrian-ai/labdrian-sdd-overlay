package main

import (
	"errors"
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

// ---------------------------------------------------------------------------
// Phase 4: dismissible behind-origin banner in repoLine() (R-002, D6).
// ---------------------------------------------------------------------------

// TestRepoLine_BehindOriginBannerStates covers every state repoLine() must
// resolve: shown for a positive, undismissed count; hidden for zero, NA, or
// once dismissed; and rootErr always keeps precedence over the banner (a
// repo-locate error is a more urgent signal than a stale-clone hint).
func TestRepoLine_BehindOriginBannerStates(t *testing.T) {
	cases := []struct {
		name            string
		behindOrigin    int
		bannerDismissed bool
		rootErr         error
		wantBanner      bool
	}{
		{"positive count, not dismissed -> shown", 2, false, nil, true},
		{"zero -> hidden", 0, false, nil, false},
		{"NA -> hidden", RepoBehindOriginNA, false, nil, false},
		{"positive but dismissed -> hidden", 2, true, nil, false},
		{"rootErr takes precedence even when positive", 2, false, errors.New("boom"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newModel()
			m.behindOrigin = tc.behindOrigin
			m.bannerDismissed = tc.bannerDismissed
			m.rootErr = tc.rootErr

			line := stripANSI(m.repoLine())
			hasBanner := strings.Contains(line, "detrás de origin/main")
			if hasBanner != tc.wantBanner {
				t.Errorf("repoLine() banner presence = %v, want %v; rendered: %q", hasBanner, tc.wantBanner, line)
			}
			if tc.rootErr != nil && !strings.Contains(line, tc.rootErr.Error()) {
				t.Errorf("repoLine() must still surface rootErr text, got: %q", line)
			}
		})
	}
}

// TestRepoLine_PersistentStatusLine verifies the menos-pasos follow-up: the
// repo status line is no longer silent when there is no bad news. It always
// names one of three states -- healthy, behind, or unresolvable -- instead
// of only speaking up for the "behind" case and staying blank otherwise,
// which left a viewer unable to tell "everything is fine" apart from "this
// was never checked".
func TestRepoLine_PersistentStatusLine(t *testing.T) {
	cases := []struct {
		name           string
		behindOrigin   int
		wantSubstr     string
		wantNotSubstrs []string
	}{
		{
			name:         "confirmed zero behind -> healthy state, not silent",
			behindOrigin: 0,
			wantSubstr:   "al día",
			wantNotSubstrs: []string{
				"detrás de origin/main",
				"No se pudo verificar",
			},
		},
		{
			name:         "NA -> explicit unresolvable state, not silent",
			behindOrigin: RepoBehindOriginNA,
			wantSubstr:   "No se pudo verificar",
			wantNotSubstrs: []string{
				"detrás de origin/main",
				"al día",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newModel()
			m.behindOrigin = tc.behindOrigin

			line := stripANSI(m.repoLine())
			if line == "" {
				t.Fatalf("repoLine() must never be silent for behindOrigin=%d, got empty string", tc.behindOrigin)
			}
			if !strings.Contains(line, tc.wantSubstr) {
				t.Errorf("repoLine() = %q, want it to contain %q", line, tc.wantSubstr)
			}
			for _, notWant := range tc.wantNotSubstrs {
				if strings.Contains(line, notWant) {
					t.Errorf("repoLine() = %q, must NOT contain %q for behindOrigin=%d", line, notWant, tc.behindOrigin)
				}
			}
		})
	}

	t.Run("rootErr still takes precedence over the healthy state", func(t *testing.T) {
		m := newModel()
		m.behindOrigin = 0
		m.rootErr = errors.New("boom")

		line := stripANSI(m.repoLine())
		if !strings.Contains(line, "boom") {
			t.Errorf("repoLine() = %q, rootErr must still take precedence", line)
		}
		if strings.Contains(line, "al día") {
			t.Errorf("repoLine() = %q, must not also render the healthy state alongside rootErr", line)
		}
	})
}
