package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	colorGreen  = lipgloss.Color("42")  // healthy
	colorYellow = lipgloss.Color("220") // needs apply
	colorRed    = lipgloss.Color("196") // needs capture+apply
	colorGray   = lipgloss.Color("245")
	colorAccent = lipgloss.Color("63")

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("231")).
			Background(colorAccent).
			Padding(0, 1)

	footerStyle = lipgloss.NewStyle().
			Foreground(colorGray).
			MarginTop(1)

	cursorStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	dimStyle    = lipgloss.NewStyle().Foreground(colorGray)
	titleStyle  = lipgloss.NewStyle().Bold(true).MarginBottom(1)

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent).
			Padding(0, 1)

	errStyle = lipgloss.NewStyle().Foreground(colorRed).Bold(true)
)

func (m model) View() string {
	if m.quitting {
		return ""
	}

	var body string
	switch m.scr {
	case screenTargets:
		body = m.viewTargets()
	case screenActions:
		body = m.viewActions()
	case screenConfirm:
		body = m.viewConfirm()
	case screenRunning:
		body = m.viewRunning()
	case screenResult:
		body = m.viewResult()
	}

	return strings.Join([]string{m.header(), body, m.footer()}, "\n")
}

func (m model) header() string {
	title := headerStyle.Render(" overlay TUI · gentle-ai sync validator ")
	if m.rootErr != nil {
		return title + "  " + errStyle.Render("repo root: "+m.rootErr.Error())
	}
	return title + "  " + dimStyle.Render(m.repoRoot)
}

func (m model) footer() string {
	var keys string
	switch m.scr {
	case screenTargets:
		keys = "↑/↓ navigate · space select · enter actions · q quit"
	case screenActions:
		keys = "↑/↓ navigate · enter run · esc back · q quit"
	case screenConfirm:
		keys = "y confirm · n cancel"
	case screenRunning:
		keys = "running…"
	case screenResult:
		keys = "↑/↓ scroll · esc back · q quit"
	}
	return footerStyle.Render(keys)
}

func (m model) viewTargets() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Select targets to operate on"))
	b.WriteString("\n")

	for i, t := range m.targets {
		cursor := "  "
		if i == m.tCursor {
			cursor = cursorStyle.Render("▸ ")
		}
		check := "[ ]"
		if m.selected[i] {
			check = lipgloss.NewStyle().Foreground(colorGreen).Render("[x]")
		}
		name := fmt.Sprintf("%-9s", t.Name)
		b.WriteString(fmt.Sprintf("%s%s %s  %s\n", cursor, check, name, dimStyle.Render(t.Path)))
	}

	if !m.anySelected() {
		b.WriteString("\n" + dimStyle.Render("(select at least one target to continue)"))
	}
	return b.String()
}

func (m model) viewActions() string {
	var b strings.Builder
	sel := []string{}
	for _, t := range m.selectedTargets() {
		sel = append(sel, t.Name)
	}
	b.WriteString(titleStyle.Render("Choose an action"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("targets: "+strings.Join(sel, ", ")) + "\n\n")

	for i, a := range m.actions {
		cursor := "  "
		if i == m.aCursor {
			cursor = cursorStyle.Render("▸ ")
		}
		tag := ""
		if a.Mutating {
			tag = lipgloss.NewStyle().Foreground(colorYellow).Render("  (mutating · confirms)")
		} else {
			tag = dimStyle.Render("  (read-only)")
		}
		b.WriteString(fmt.Sprintf("%s%s%s\n", cursor, a.Name, tag))
	}
	return b.String()
}

func (m model) viewConfirm() string {
	sel := []string{}
	for _, t := range m.selectedTargets() {
		sel = append(sel, t.Name)
	}
	msg := fmt.Sprintf(
		"Run %s on: %s ?\n\nThis MUTATES the targets.",
		lipgloss.NewStyle().Bold(true).Render(m.pendingAction.Name),
		strings.Join(sel, ", "),
	)
	return boxStyle.BorderForeground(colorYellow).Render(msg)
}

func (m model) viewRunning() string {
	return titleStyle.Render("Running " + m.pendingAction.Name + "…")
}

func (m model) viewResult() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Result · " + m.result.action.Name))
	b.WriteString("\n")

	if len(m.result.verdicts) > 0 {
		b.WriteString(m.viewDashboard())
		b.WriteString("\n")
	}

	b.WriteString(m.viewOutput())
	return b.String()
}

// viewDashboard renders the headline per-target colored sync status.
func (m model) viewDashboard() string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("gentle-ai sync dashboard") + "\n")

	for _, v := range m.result.verdicts {
		var color lipgloss.Color
		var label string
		switch v.Status {
		case SyncHealthy:
			color, label = colorGreen, "GREEN  in sync with gentle-ai (healthy)"
		case SyncNeedsApply:
			color, label = colorYellow, "YELLOW needs apply (overlay not deployed)"
		case SyncNeedsCapture:
			color, label = colorRed, "RED    gentle-ai synced — needs capture+apply"
		default:
			color, label = colorGray, "?      no verdict"
		}

		badge := lipgloss.NewStyle().Foreground(color).Bold(true).Render("●")
		head := fmt.Sprintf("%s %-9s %s", badge, v.Target,
			lipgloss.NewStyle().Foreground(color).Render(label))
		counts := dimStyle.Render(fmt.Sprintf("    upstream_changed=%d  overlay_not_deployed=%d",
			v.UpstreamChanged, v.OverlayNotDeployed))
		action := dimStyle.Render("    → " + v.Action)

		b.WriteString(head + "\n" + counts + "\n")
		if v.Action != "" {
			b.WriteString(action + "\n")
		}
	}
	return boxStyle.Render(strings.TrimRight(b.String(), "\n"))
}

// viewOutput renders the raw command output, scrollable.
func (m model) viewOutput() string {
	lines := strings.Split(strings.TrimRight(m.result.output, "\n"), "\n")

	// Reserve vertical room for header/footer/title/dashboard.
	viewport := m.height - 10
	if len(m.result.verdicts) > 0 {
		viewport -= len(m.result.verdicts)*3 + 3
	}
	if viewport < 5 {
		viewport = 5
	}

	start := m.scroll
	if start > len(lines)-1 {
		start = len(lines) - 1
	}
	if start < 0 {
		start = 0
	}
	end := start + viewport
	if end > len(lines) {
		end = len(lines)
	}

	shown := strings.Join(lines[start:end], "\n")
	hint := ""
	if len(lines) > viewport {
		hint = dimStyle.Render(fmt.Sprintf("  [lines %d-%d of %d]", start+1, end, len(lines)))
	}

	title := lipgloss.NewStyle().Bold(true).Render("output") + hint
	return title + "\n" + boxStyle.BorderForeground(colorGray).Render(shown)
}
