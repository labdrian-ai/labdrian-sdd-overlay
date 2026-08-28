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
				Status:             classify(0, 0, 3, 0, false), // == SyncBehindOrigin per R-006
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

// ---------------------------------------------------------------------------
// Slice 3b: release-based repoLine/bannerVisible rendering (R-011, D2).
//
// While behindRelease == RepoBehindOriginNA (no release tag exists yet, D1
// pre-first-tag bootstrap), repoLine/bannerVisible fall back to the legacy
// origin-only behavior verified above, byte-identical. These tests cover
// the NEW branch: once behindRelease resolves to a concrete value, it
// becomes the primary signal and raw origin drift demotes to an
// informational line, per D2 and the tui-self-update MODIFIED R-007.
// ---------------------------------------------------------------------------

// TestRepoLine_ReleaseBehindBannerStates mirrors
// TestRepoLine_BehindOriginBannerStates for the release-known branch: shown
// for a positive undismissed release-behind count; hidden when dismissed;
// healthy line at release-behind==0; and rootErr still takes precedence.
func TestRepoLine_ReleaseBehindBannerStates(t *testing.T) {
	cases := []struct {
		name            string
		behindRelease   int
		bannerDismissed bool
		rootErr         error
		wantBanner      bool
	}{
		{"positive release-behind, not dismissed -> shown", 2, false, nil, true},
		{"zero release-behind -> hidden", 0, false, nil, false},
		{"positive release-behind but dismissed -> hidden", 2, true, nil, false},
		{"rootErr takes precedence even when positive", 2, false, errors.New("boom"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newModel()
			m.behindRelease = tc.behindRelease
			m.bannerDismissed = tc.bannerDismissed
			m.rootErr = tc.rootErr

			line := stripANSI(m.repoLine())
			hasBanner := strings.Contains(line, "detrás del último tag")
			if hasBanner != tc.wantBanner {
				t.Errorf("repoLine() release banner presence = %v, want %v; rendered: %q", hasBanner, tc.wantBanner, line)
			}
			if tc.rootErr != nil && !strings.Contains(line, tc.rootErr.Error()) {
				t.Errorf("repoLine() must still surface rootErr text, got: %q", line)
			}
		})
	}
}

// TestRepoLine_OriginDriftDemotesToInformationalOnceReleaseKnown verifies
// D2's core rendering claim: once behindRelease is known and healthy (0),
// a positive behindOrigin no longer renders the actionable amber banner —
// it renders a dim informational line instead, and the release-healthy
// text is still present.
func TestRepoLine_OriginDriftDemotesToInformationalOnceReleaseKnown(t *testing.T) {
	m := newModel()
	m.behindRelease = 0
	m.behindOrigin = 4

	line := stripANSI(m.repoLine())
	if strings.Contains(line, "u actualizar y desplegar") {
		t.Errorf("repoLine() must NOT show the actionable banner for origin-only drift once release is known-healthy, got: %q", line)
	}
	if !strings.Contains(line, "informativo") {
		t.Errorf("repoLine() must demote origin drift to an informational line once release is known, got: %q", line)
	}
	if !strings.Contains(line, "4") {
		t.Errorf("repoLine() informational line must still name the origin-behind count, got: %q", line)
	}
}

// TestBannerVisible_ReleaseTakesPrecedenceOnceKnown verifies bannerVisible's
// D2 dual-mode contract directly: once behindRelease is resolved (not NA),
// it alone decides banner visibility -- origin drift alone must never
// trigger it, and a healthy release with positive origin drift must not
// show the banner either.
func TestBannerVisible_ReleaseTakesPrecedenceOnceKnown(t *testing.T) {
	cases := []struct {
		name          string
		behindRelease int
		behindOrigin  int
		want          bool
	}{
		{"release behind, origin healthy -> visible", 3, 0, true},
		{"release healthy, origin behind -> NOT visible (demoted)", 0, 5, false},
		{"both healthy -> not visible", 0, 0, false},
		{"both behind -> visible (release wins)", 2, 9, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newModel()
			m.behindRelease = tc.behindRelease
			m.behindOrigin = tc.behindOrigin
			if got := m.bannerVisible(); got != tc.want {
				t.Errorf("bannerVisible() = %v, want %v (behindRelease=%d behindOrigin=%d)", got, tc.want, tc.behindRelease, tc.behindOrigin)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Slice 3b: viewDashboard version/digest/release-behind rendering (R-003/R-011).
// ---------------------------------------------------------------------------

// TestViewDashboard_ShowsReleaseBehindIndicator mirrors
// TestViewDashboard_ShowsOriginBehindIndicator for the new SyncBehindRelease
// status and its RepoBehindRelease count.
func TestViewDashboard_ShowsReleaseBehindIndicator(t *testing.T) {
	m := newModel()
	m.width = 80
	m.scr = screenResult
	m.result = commandResult{
		action: Action{Name: "Verificar sincronización", Command: "sync-check"},
		verdicts: []TargetVerdict{
			{
				Target:            "claude",
				Status:            SyncBehindRelease,
				RepoBehindRelease: 2,
				RecordedVersion:   "v1.3.0",
				DigestMatch:       "yes",
				Action:            "run 'overlay self-update' to fetch release v1.4.0",
			},
		},
		output: "ok",
	}

	rendered := stripANSI(m.viewDashboard())
	if !strings.Contains(rendered, "Detrás del release") {
		t.Errorf("viewDashboard must show 'Detrás del release' status label, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "detrás del release: 2") {
		t.Errorf("viewDashboard must show 'detrás del release: 2' count, got:\n%s", rendered)
	}
}

// TestViewDashboard_VersionLineRendering covers the three RecordedVersion
// shapes: a concrete version (with and without a digest mismatch note) and
// the never-deployed ("NA") case.
func TestViewDashboard_VersionLineRendering(t *testing.T) {
	cases := []struct {
		name       string
		verdict    TargetVerdict
		wantSubstr string
	}{
		{
			name:       "in-sync version, no mismatch note",
			verdict:    TargetVerdict{Target: "claude", Status: SyncHealthy, RecordedVersion: "v1.4.0", DigestMatch: "yes"},
			wantSubstr: "versión: v1.4.0",
		},
		{
			name:       "digest mismatch appends note",
			verdict:    TargetVerdict{Target: "claude", Status: SyncNeedsApply, RecordedVersion: "v1.3.0", DigestMatch: "no"},
			wantSubstr: "versión: v1.3.0 (digest desactualizado)",
		},
		{
			name:       "never deployed shows friendly text, not raw NA",
			verdict:    TargetVerdict{Target: "claude", Status: SyncNeedsApply, RecordedVersion: "NA", DigestMatch: "NA"},
			wantSubstr: "versión: nunca desplegado",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newModel()
			m.width = 80
			m.scr = screenResult
			m.result = commandResult{
				action:   Action{Name: "Verificar sincronización", Command: "sync-check"},
				verdicts: []TargetVerdict{tc.verdict},
				output:   "ok",
			}
			rendered := stripANSI(m.viewDashboard())
			if !strings.Contains(rendered, tc.wantSubstr) {
				t.Errorf("viewDashboard must contain %q, got:\n%s", tc.wantSubstr, rendered)
			}
		})
	}
}

// TestViewDashboard_EmptyRecordedVersionRendersNoVersionLine verifies
// backwards compatibility: a verdict built without RecordedVersion (e.g. by
// code that predates this field, or a legacy sync-check run) renders no
// version line at all rather than a blank/garbled one.
func TestViewDashboard_EmptyRecordedVersionRendersNoVersionLine(t *testing.T) {
	m := newModel()
	m.width = 80
	m.scr = screenResult
	m.result = commandResult{
		action:   Action{Name: "Verificar sincronización", Command: "sync-check"},
		verdicts: []TargetVerdict{{Target: "claude", Status: SyncHealthy}},
		output:   "ok",
	}
	rendered := stripANSI(m.viewDashboard())
	if strings.Contains(rendered, "versión:") {
		t.Errorf("viewDashboard must NOT show a version line when RecordedVersion is empty, got:\n%s", rendered)
	}
}
