package main

// A sync-check over a partial selection is one invocation per target, and the
// dashboard is the report of all of them read as one: the program has always
// shown the verdicts of the whole transcript, not of each invocation by itself.
// These sessions pin the two ways that differs from reading each output alone,
// against the program as main composes it and the stub backend, judged by the
// dashboard the operator sees. They were written to pass on the program before
// the report was read by the adapter (1fb3c9a), and they state what it did:
//
//   - a target named in two invocations is one verdict, in the place where it was
//     first seen, with the later lines updating what the earlier ones gave;
//   - an agent file line belongs to the section that was last opened, even if that
//     was in an earlier invocation.

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// sayInTurn makes the backend print outputs for key in turn: the first output
// the first time key is invoked after this call, the second the second time,
// and so on.
func (s *stubBackend) sayInTurn(key string, outputs ...string) {
	s.t.Helper()
	for i, out := range outputs {
		s.say(fmt.Sprintf("%s-turn-%d", key, i+1), out)
	}
	dir := s.dir
	s.onRun(key, fmt.Sprintf(
		`n=$(cat %[1]q 2>/dev/null || echo 0); n=$((n+1)); echo "$n" > %[1]q; cp "%[2]s-turn-$n" "%[2]s" 2>/dev/null`,
		filepath.Join(dir, "turn-"+key), filepath.Join(dir, "out", key)))
}

// dashboardOf is the dashboard of the result screen as lines, without its box.
func dashboardOf(t *testing.T, view string) []string {
	t.Helper()
	var dashboard []string
	inside := false
	for _, line := range strings.Split(view, "\n") {
		switch {
		case strings.Contains(line, "Estado de sincronización"):
			inside = true
		case inside && strings.HasPrefix(line, "╰"):
			return dashboard
		}
		if inside {
			line = strings.TrimRight(line, " ")
			line = strings.TrimSuffix(line, "│")
			line = strings.TrimPrefix(line, "│ ")
			dashboard = append(dashboard, strings.TrimRight(line, " "))
		}
	}
	t.Fatalf("the result screen shows no dashboard:\n%s", view)
	return nil
}

// checkedClaudeAndCodex is the session on the result of a sync-check over
// claude and codex alone (two invocations), after the backend was told to
// print the given outputs in turn.
func checkedClaudeAndCodex(t *testing.T, first, second string) *session {
	t.Helper()
	s := newSession(t).start().press("down", "down", "down", "space", "up", "up", "space") // pi and opencode off
	s.toActions().onRow(rowSyncCheck)
	s.stub.sayInTurn("sync-check", first, second)
	s.press("enter")
	if got, want := receivedAfterLaunch(s), "sync-check --target claude|sync-check --target codex"; got != want {
		t.Fatalf("the backend received %q, want one sync-check per selected target: %q", got, want)
	}
	return s
}

func TestSyncCheckDashboard_ATargetNamedInTwoInvocationsIsOneVerdict(t *testing.T) {
	const first = `=== sync-check: claude (/stub/.claude) ===
  OVERLAY_NOT_DEPLOYED: agents/GADU.md (live missing)
VERDICT:claude:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=1 REPO_BEHIND_ORIGIN=0 REPO_BEHIND_RELEASE=0 RECORDED_VERSION=v1.0.0 DIGEST_MATCH=yes
ACTION:claude: run labdrian apply --target claude

=== sync-check: opencode (/stub/.config/opencode) ===
VERDICT:opencode:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=0 REPO_BEHIND_ORIGIN=0 REPO_BEHIND_RELEASE=0 RECORDED_VERSION=v1.0.0 DIGEST_MATCH=yes
ACTION:opencode: in sync with gentle-ai (healthy)
`
	const second = `=== sync-check: claude (/stub/.claude) ===
  UPSTREAM_CHANGED: agents/EXTRA.md
VERDICT:claude:UPSTREAM_CHANGED=2 OVERLAY_NOT_DEPLOYED=0 REPO_BEHIND_ORIGIN=0 REPO_BEHIND_RELEASE=0 RECORDED_VERSION=v1.1.0 DIGEST_MATCH=yes
ACTION:claude: run labdrian capture --target claude

=== sync-check: codex (/stub/.codex) ===
VERDICT:codex:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=0 REPO_BEHIND_ORIGIN=0 REPO_BEHIND_RELEASE=0 RECORDED_VERSION=v1.0.0 DIGEST_MATCH=yes
ACTION:codex: in sync with gentle-ai (healthy)
`
	s := checkedClaudeAndCodex(t, first, second)

	// One claude, where it was first seen, as the later invocation left it: its
	// counts, version and remedy replace the earlier ones, and the agent files
	// of both are listed. opencode and codex follow in the order they appeared.
	want := []string{
		"Estado de sincronización",
		"✗ claude     Requiere capture + apply",
		"  cambios upstream: 2  overlay sin desplegar: 0",
		"  versión: v1.1.0",
		"  → run labdrian capture --target claude",
		"  Agents:",
		"    ! agents/GADU.md",
		"    ✗ agents/EXTRA.md",
		"✓ opencode   Sincronizado",
		"  cambios upstream: 0  overlay sin desplegar: 0",
		"  versión: v1.0.0",
		"  → in sync with gentle-ai (healthy)",
		"✓ codex      Sincronizado",
		"  cambios upstream: 0  overlay sin desplegar: 0",
		"  versión: v1.0.0",
		"  → in sync with gentle-ai (healthy)",
	}
	if got := dashboardOf(t, s.view()); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("the dashboard is\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestSyncCheckDashboard_AnAgentFileBelongsToTheSectionLastOpenedEvenInAnEarlierInvocation(t *testing.T) {
	const first = `=== sync-check: claude (/stub/.claude) ===
VERDICT:claude:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=1 REPO_BEHIND_ORIGIN=0 REPO_BEHIND_RELEASE=0 RECORDED_VERSION=v1.0.0 DIGEST_MATCH=yes
ACTION:claude: run labdrian apply --target claude
`
	// The second invocation begins with a file line before any section of its
	// own: the section open is still claude's.
	const second = `  OVERLAY_NOT_DEPLOYED: agents/CARRIED.md (live missing)
=== sync-check: codex (/stub/.codex) ===
VERDICT:codex:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=0 REPO_BEHIND_ORIGIN=0 REPO_BEHIND_RELEASE=0 RECORDED_VERSION=v1.0.0 DIGEST_MATCH=yes
ACTION:codex: in sync with gentle-ai (healthy)
`
	s := checkedClaudeAndCodex(t, first, second)

	want := []string{
		"Estado de sincronización",
		"! claude     Pendiente de aplicación",
		"  cambios upstream: 0  overlay sin desplegar: 1",
		"  versión: v1.0.0",
		"  → run labdrian apply --target claude",
		"  Agents:",
		"    ! agents/CARRIED.md",
		"✓ codex      Sincronizado",
		"  cambios upstream: 0  overlay sin desplegar: 0",
		"  versión: v1.0.0",
		"  → in sync with gentle-ai (healthy)",
	}
	if got := dashboardOf(t, s.view()); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("the dashboard is\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
