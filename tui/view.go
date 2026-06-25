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
	titleStyle     = lipgloss.NewStyle().Bold(true).Foreground(colorWhite)
	highlightStyle = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	sectionStyle   = lipgloss.NewStyle().Bold(true).Foreground(colorAccent2)

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

	termW := m.contentWidth()
	repoLine := m.repoLine()
	keys := m.footerKeys()

	// colW is the single content-column width for this screen: the widest
	// body/footer/repo line. The logo is excluded from the max (so it does not
	// inflate the column) but used as a floor so the hero art never clips.
	// Capped to the terminal width for narrow-width safety.
	colW := maxLineWidth(body)
	if w := maxLineWidth(keys); w > colW {
		colW = w
	}
	if w := maxLineWidth(repoLine); w > colW {
		colW = w
	}
	if lw := maxLineWidth(logoBanner(0)); lw > colW {
		colW = lw
	}
	if colW > termW {
		colW = termW
	}
	if colW < 1 {
		colW = 1
	}

	// Single vertical axis: the logo is centered WITHIN the column; body and
	// footer are left-aligned at the column's left edge. Then the whole column
	// is centered on screen by one uniform leftPad, so header + body + footer all
	// share the exact same left edge.
	header := logoBanner(colW) + "\n" + repoLine
	footer := footerStyle.Width(colW).Render(keys)
	composed := strings.Join([]string{header, body, footer}, "\n")

	leftPad := (termW - colW) / 2
	if leftPad < 0 {
		leftPad = 0
	}
	composed = indentBlock(composed, leftPad)

	return lipgloss.NewStyle().MaxWidth(termW).Render(composed)
}

// repoLine renders the repo-root path (or the locate error) at the column's
// left edge — no hardcoded indent.
func (m model) repoLine() string {
	if m.rootErr != nil {
		return errStyle.Render("Error al localizar el repositorio: " + m.rootErr.Error())
	}
	return mutingStyle.Render(m.repoRoot)
}

// footerKeys returns the raw key-hint legend for the active screen. The width
// styling and column alignment are applied by View().
func (m model) footerKeys() string {
	switch m.scr {
	case screenTargets:
		return "↑/↓ navegar  ·  espacio seleccionar  ·  a selec. todos  ·  enter continuar  ·  q salir"
	case screenActions:
		return "↑/↓ navegar  ·  enter ejecutar la acción seleccionada  ·  esc volver  ·  q salir"
	case screenConfirm:
		return "y/enter confirmar  ·  n/esc cancelar"
	case screenRunning:
		return "Ejecutando…"
	case screenResult:
		return "↑/↓ desplazar  ·  esc/enter volver  ·  q salir"
	}
	return ""
}

// maxLineWidth returns the widest visible line width (ANSI-aware) in s.
func maxLineWidth(s string) int {
	w := 0
	for _, line := range strings.Split(s, "\n") {
		if lw := lipgloss.Width(line); lw > w {
			w = lw
		}
	}
	return w
}

// indentBlock prepends pad spaces to every non-empty line, shifting an entire
// block to a shared left edge without filling blank lines with whitespace.
func indentBlock(s string, pad int) string {
	if pad <= 0 {
		return s
	}
	prefix := strings.Repeat(" ", pad)
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line == "" {
			continue
		}
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

// ouroborosArt is the labdrian hero logo: an ouroboros (a serpent biting its
// own tail) rasterized from an image into Braille glyphs, matching gentle-ai's
// dithered logo style. 34 cells wide.
var ouroborosArt = []string{
	"⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣀⠀⠐⣧⡀",
	"⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠈⢷⣦⣻⡻⣶⣄",
	"⠀⠀⠀⠀⠀⠀⢀⣤⠴⠖⠛⣋⠋⣝⣫⣛⡛⠟⢶⣦⣿⣿⣯⣽⣗⣷",
	"⠀⠀⠀⠀⣠⢞⣩⣴⣶⣿⣿⣿⣿⣿⣿⣿⣿⣿⠤⠈⡽⢿⣿⣿⠟⡿",
	"⠀⠀⠀⣼⣷⠿⠛⠉⠀⠀⠀⠀⠀⠉⣽⡿⣋⣥⣶⣿⣆⠃⠹⣿⡼⡿⣆⠠⣤⡀",
	"⠀⠀⣸⣿⠃⠀⠀⠀⠀⠀⠀⠀⠀⠀⠈⠙⣻⣟⠿⢿⣿⣿⣾⣤⣤⡴⠎⢷⣿⣷⣦⣴⡆",
	"⠀⢠⢿⣿⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠉⠻⣽⣶⠺⢿⣿⡏⠻⣿⣄⠀⣴⣝⢿⣿⣅",
	"⠀⢸⡼⢿⣇⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠙⠉⠀⠀⠉⠁⠀⠹⣿⣆⠉⢛⣸⡿⠾⠇",
	"⠀⣿⣶⣦⠻⣧⠀⢀⣾⣿⣆⣴⣶⡄⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢻⣿⣎⢹",
	"⢰⢿⣿⣷⣿⣿⣧⣌⣿⣿⠈⣯⠟⠁⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠈⣿⣿⠂",
	"⠀⠞⡿⠻⣿⣿⠨⡯⣿⣿⡿⣧⣯⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢀⣼⣿⠃",
	"⠀⠀⠀⠀⠘⢿⣶⣧⠼⠟⣿⢿⣿⣙⡶⣤⣀⠀⠀⠀⠀⠀⠀⣀⣤⢾⣱⠟⠁",
	"⠀⠀⠀⠀⠀⠀⠙⢿⣿⣿⣿⣿⣿⣀⣥⠼⡎⢻⡱⡖⢊⠼⣉⣵⣷⠿⠋",
	"⠀⠀⠀⠀⠀⠀⠀⠀⠉⠛⠿⠿⣿⣿⣿⣿⣿⣿⣷⡿⠿⠿⠛⠉⠁",
}

// logoRowColor gives a top-to-bottom gradient: bright head, purple body,
// sky-blue lower coils — a vertical fade like gentle-ai's logo.
func logoRowColor(i, n int) lipgloss.Color {
	switch {
	case i < 4:
		return colorWhite
	case i < n-4:
		return colorAccent
	default:
		return colorAccent2
	}
}

// centerBlock left-pads every line by the same amount so the block's true
// content bounding box is centered within width w. A uniform pad (not per-line
// centering) preserves the art's internal column alignment.
func centerBlock(lines []string, contentW, w int) string {
	pad := 0
	if w > contentW {
		pad = (w - contentW) / 2
	}
	prefix := strings.Repeat(" ", pad)
	return prefix + strings.Join(lines, "\n"+prefix)
}

// logoBanner renders the ouroboros hero logo centered to width w, with the
// labdrian wordmark centered below it (gentle-ai header anatomy: centered art).
func logoBanner(w int) string {
	const blank = '⠀' // Braille blank — visually empty but width 1

	// Crop all rows to the true content bounding box [left,right] so the serpent
	// is centered by its own mass, independent of the source image framing.
	rows := make([][]rune, len(ouroborosArt))
	left, right := 1<<30, -1
	for i, r := range ouroborosArt {
		rr := []rune(r)
		rows[i] = rr
		for c, ch := range rr {
			if ch != blank && ch != ' ' {
				if c < left {
					left = c
				}
				if c > right {
					right = c
				}
			}
		}
	}
	if right < left {
		left, right = 0, 0
	}
	cropW := right - left + 1

	n := len(rows)
	art := make([]string, n)
	for i, rr := range rows {
		var line strings.Builder
		for c := left; c <= right; c++ {
			if c < len(rr) {
				line.WriteRune(rr[c])
			} else {
				line.WriteRune(blank)
			}
		}
		art[i] = lipgloss.NewStyle().Foreground(logoRowColor(i, n)).Render(line.String())
	}

	mark := highlightStyle.Render("labdrian") + mutingStyle.Render("  ·  overlay sobre gentle-ai")
	return centerBlock(art, cropW, w) + "\n\n" + centerBlock([]string{mark}, lipgloss.Width(mark), w)
}

func (m model) viewTargets() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Seleccionar destinos") + "\n")

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
	b.WriteString(titleStyle.Render("Elegir una acción") + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(colorWhite).
		Render("Ejecutá capture, apply y la gestión de hooks desde acá — no hace falta la CLI.") + "\n")
	b.WriteString(dimStyle.Render("destinos: "+strings.Join(sel, ", ")) + "    " +
		dimStyle.Render("Flujo típico:  Verificar → Capturar → Aplicar") + "\n\n")

	for i, a := range m.actions {
		// Operational group header before the first action.
		if i == 0 {
			b.WriteString(sectionStyle.Render("── Sincronización ──") + "\n")
		}
		// Hooks group header before the first TargetAgnostic action.
		// These headers are display-only — they are not in the actions slice, so
		// cursor navigation and m.actions[m.aCursor] indexing are unaffected.
		if a.TargetAgnostic && (i == 0 || !m.actions[i-1].TargetAgnostic) {
			b.WriteString("\n" + sectionStyle.Render("── Hooks (global ~/.claude) ──") + "\n")
		}

		cursor := "  "
		if i == m.aCursor {
			cursor = cursorStyle.Render("▸ ")
		}

		var tag string
		if a.Mutating {
			tag = lipgloss.NewStyle().Foreground(colorYellow).Bold(true).Render("[modifica]")
		} else {
			tag = mutingStyle.Render("[solo lectura]")
		}

		name := fmt.Sprintf("%-32s", a.Name)
		if i == m.aCursor {
			name = highlightStyle.Render(name)
		}
		hint := dimStyle.Render(fmt.Sprintf("%-40s", a.Hint))

		b.WriteString(fmt.Sprintf("%s%s%s%s\n", cursor, name, hint, tag))
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
		if m.result.exitCode == 2 {
			// Exit 2 means DEGRADED (e.g. 'engine status' with a warning): not a
			// hard failure, but the install needs attention. Render as a warning.
			b.WriteString(lipgloss.NewStyle().Foreground(colorYellow).Bold(true).Render("Resultado · " + m.result.action.Name))
			b.WriteString("\n")
			b.WriteString(lipgloss.NewStyle().Foreground(colorYellow).Bold(true).Render("! Degradado — revisá la salida"))
			b.WriteString("\n")
		} else {
			b.WriteString(errStyle.Render("Resultado · " + m.result.action.Name))
			b.WriteString("\n")
			b.WriteString(lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render("✗ Comando falló"))
			b.WriteString("\n")
		}
	} else {
		b.WriteString(titleStyle.Render("Resultado · " + m.result.action.Name))
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorGreen).Bold(true).
			Render("✓ Completado") + "\n")
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
		head := fmt.Sprintf("%s %s  %s", badge, targetName, statusLabel)
		counts := mutingStyle.Render(fmt.Sprintf("  cambios upstream: %d  overlay sin desplegar: %d",
			v.UpstreamChanged, v.OverlayNotDeployed))
		action := dimStyle.Render("  → " + v.Action)

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
