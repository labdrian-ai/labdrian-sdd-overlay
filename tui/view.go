package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	// Color palette — dark-terminal friendly.
	colorGreen   = lipgloss.Color("78")  // soft green — in sync
	colorYellow  = lipgloss.Color("214") // amber — needs apply
	colorRed     = lipgloss.Color("203") // soft red — needs capture+apply
	colorGray    = lipgloss.Color("242")
	colorMuted   = lipgloss.Color("238")
	colorAccent  = lipgloss.Color("105") // soft purple — primary accent
	colorAccent2 = lipgloss.Color("75")  // sky blue — secondary accent
	colorWhite   = lipgloss.Color("255")

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorWhite).
			Background(colorAccent).
			Padding(0, 2)

	subHeaderStyle = lipgloss.NewStyle().
			Foreground(colorAccent2).
			Background(colorMuted).
			Padding(0, 2)

	footerStyle = lipgloss.NewStyle().
			Foreground(colorGray).
			BorderTop(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(colorMuted).
			MarginTop(1).
			PaddingTop(1)

	cursorStyle    = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	selectedStyle  = lipgloss.NewStyle().Foreground(colorGreen).Bold(true)
	dimStyle       = lipgloss.NewStyle().Foreground(colorGray)
	mutingStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	titleStyle     = lipgloss.NewStyle().Bold(true).Foreground(colorWhite).MarginBottom(1)
	highlightStyle = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent).
			Padding(0, 1)

	// outputBoxStyle is the named style for the raw command output box.
	// It uses a gray border to visually distinguish from the dashboard box (accent border).
	outputBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorGray).
			Padding(0, 1)

	warnBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorYellow).
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

	w := m.contentWidth()
	composed := strings.Join([]string{m.header(), body, m.footer()}, "\n")
	return lipgloss.NewStyle().MaxWidth(w).Render(composed)
}

func (m model) header() string {
	w := m.contentWidth()
	title := headerStyle.Render(" overlay TUI ")
	subtitle := subHeaderStyle.Render(" gentle-ai sync validator ")
	bar := lipgloss.JoinHorizontal(lipgloss.Top, title, subtitle)
	bar = lipgloss.NewStyle().Width(w).Render(bar)
	if m.rootErr != nil {
		return bar + "\n" + errStyle.Render("  Error al localizar el repositorio: "+m.rootErr.Error())
	}
	return bar + "\n" + mutingStyle.Render("  "+m.repoRoot)
}

func (m model) footer() string {
	w := m.contentWidth()
	var keys string
	switch m.scr {
	case screenTargets:
		keys = "↑/↓ navegar  ·  espacio seleccionar  ·  a selec. todos  ·  enter continuar  ·  q salir"
	case screenActions:
		keys = "↑/↓ navegar  ·  enter ejecutar  ·  esc volver  ·  q salir"
	case screenConfirm:
		keys = "y confirmar  ·  esc/n cancelar"
	case screenRunning:
		keys = "Ejecutando…"
	case screenResult:
		keys = "↑/↓ desplazar  ·  esc/enter volver  ·  q salir"
	}
	return footerStyle.Width(w).Render(keys)
}

func (m model) viewTargets() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Seleccionar destinos"))

	for i, t := range m.targets {
		cursor := "  "
		if i == m.tCursor {
			cursor = cursorStyle.Render("▸ ")
		}
		check := dimStyle.Render("[ ]")
		if m.selected[i] {
			check = selectedStyle.Render("[✓]")
		}
		name := fmt.Sprintf("%-9s", t.Name)
		var row string
		if i == m.tCursor {
			row = fmt.Sprintf("%s%s %s  %s\n", cursor, check,
				highlightStyle.Render(name), dimStyle.Render(t.Path))
		} else {
			row = fmt.Sprintf("%s%s %s  %s\n", cursor, check, name, mutingStyle.Render(t.Path))
		}
		b.WriteString(row)
	}

	if !m.anySelected() {
		b.WriteString("\n" + dimStyle.Render("(seleccionar al menos un destino para continuar)"))
	}
	return b.String()
}

func (m model) viewActions() string {
	var b strings.Builder
	sel := []string{}
	for _, t := range m.selectedTargets() {
		sel = append(sel, t.Name)
	}
	b.WriteString(titleStyle.Render("Elegir una acción"))
	b.WriteString(dimStyle.Render("destinos: "+strings.Join(sel, ", ")) + "\n\n")

	for i, a := range m.actions {
		cursor := "  "
		if i == m.aCursor {
			cursor = cursorStyle.Render("▸ ")
		}
		var tag string
		if a.Mutating {
			tag = lipgloss.NewStyle().Foreground(colorYellow).Render("  [modifica]")
		} else {
			tag = mutingStyle.Render("  [solo lectura]")
		}
		var nameStr string
		if i == m.aCursor {
			nameStr = highlightStyle.Render(a.Name)
		} else {
			nameStr = a.Name
		}
		b.WriteString(fmt.Sprintf("%s%s%s\n", cursor, nameStr, tag))
	}
	return b.String()
}

func (m model) viewConfirm() string {
	sel := []string{}
	for _, t := range m.selectedTargets() {
		sel = append(sel, t.Name)
	}

	detail := "Esta acción modifica los destinos."
	if m.pendingAction.ConfirmMessage != "" {
		detail = m.pendingAction.ConfirmMessage
	}

	var msg string
	if m.pendingAction.TargetAgnostic {
		msg = fmt.Sprintf(
			"Ejecutar %s\n\n%s %s",
			lipgloss.NewStyle().Bold(true).Foreground(colorYellow).Render(m.pendingAction.Name),
			lipgloss.NewStyle().Foreground(colorYellow).Bold(true).Render("Atención:"),
			detail,
		)
	} else {
		msg = fmt.Sprintf(
			"Ejecutar %s en: %s\n\n%s %s",
			lipgloss.NewStyle().Bold(true).Foreground(colorYellow).Render(m.pendingAction.Name),
			strings.Join(sel, ", "),
			lipgloss.NewStyle().Foreground(colorYellow).Bold(true).Render("Atención:"),
			detail,
		)
	}
	return warnBoxStyle.Render(msg)
}

func (m model) viewRunning() string {
	return titleStyle.Render(m.spinner.View() + " Ejecutando " + m.pendingAction.Name + "…")
}

func (m model) viewResult() string {
	var b strings.Builder

	if m.result.err != nil {
		b.WriteString(errStyle.Render("Resultado · " + m.result.action.Name))
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render("  ✗ Comando falló"))
		b.WriteString("\n")
	} else {
		b.WriteString(titleStyle.Render("Resultado · " + m.result.action.Name))
	}

	if len(m.result.verdicts) > 0 {
		b.WriteString(m.viewDashboard())
		b.WriteString("\n")
	}

	// Empty-verdict note for sync-check.
	if m.result.action.Command == "sync-check" && len(m.result.verdicts) == 0 {
		b.WriteString(dimStyle.Render("No se pudieron analizar veredictos") + "\n")
	}

	b.WriteString(m.viewOutput())
	return b.String()
}

// viewDashboard renders the headline per-target colored sync status.
func (m model) viewDashboard() string {
	w := m.contentWidth()
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(colorAccent2).Render("Estado de sincronización") + "\n")

	for _, v := range m.result.verdicts {
		var color lipgloss.Color
		var icon, label string
		switch v.Status {
		case SyncHealthy:
			color, icon, label = colorGreen, "✓", "Sincronizado"
		case SyncNeedsApply:
			color, icon, label = colorYellow, "!", "Pendiente de aplicación"
		case SyncNeedsCapture:
			color, icon, label = colorRed, "✗", "Requiere capture + apply"
		default:
			color, icon, label = colorGray, "?", "Sin datos"
		}

		st := lipgloss.NewStyle().Foreground(color).Bold(true)
		badge := st.Render(icon)
		targetName := lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf("%-9s", v.Target))
		statusLabel := lipgloss.NewStyle().Foreground(color).Render(label)
		head := fmt.Sprintf("  %s %s  %s", badge, targetName, statusLabel)
		counts := mutingStyle.Render(fmt.Sprintf("       cambios upstream: %d  overlay sin desplegar: %d",
			v.UpstreamChanged, v.OverlayNotDeployed))
		action := dimStyle.Render("       → " + v.Action)

		b.WriteString(head + "\n" + counts + "\n")
		if v.Action != "" {
			b.WriteString(action + "\n")
		}
	}
	return boxStyle.Width(w - 2).Render(strings.TrimRight(b.String(), "\n"))
}

// viewOutput renders the raw command output, scrollable.
func (m model) viewOutput() string {
	w := m.contentWidth()
	lines := splitOutputLines(m.result.output)

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

	title := lipgloss.NewStyle().Bold(true).Foreground(colorAccent2).Render("salida") + hint
	return title + "\n" + outputBoxStyle.Width(w-2).Render(shown)
}

// splitOutputLines splits output text into lines, trimming the trailing newline.
func splitOutputLines(output string) []string {
	return strings.Split(strings.TrimRight(output, "\n"), "\n")
}
