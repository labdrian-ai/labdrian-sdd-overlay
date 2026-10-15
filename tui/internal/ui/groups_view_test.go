package ui

// The section headers of the action menu come from the group each action
// declares. They used to be derived: "Hooks" from TargetAgnostic (which put
// "Actualizar repositorio", a target-agnostic row, under it), "Skills" and
// "Memoria" from the names of two commands. These tests give the screen menus
// whose rows are not the shipped ones, and the shipped menu as it is.

import (
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/tui/internal/domain"
)

// menuLines renders the actions screen of a model with the given menu and
// returns its lines below the title, with the rows told from the headers:
// a header is "== <label>", a row is "-- <name>", a blank line is "".
func menuLines(t *testing.T, menu ...domain.Action) []string {
	t.Helper()
	m, _ := modelWithMenu(t, menu...)
	var out []string
	started := false
	for _, line := range strings.Split(stripANSI(m.viewActions()), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Elegir una acción"):
			started = true
		case !started:
		case strings.HasPrefix(line, "── "):
			out = append(out, "== "+strings.Trim(line, "─ "))
		case line == "":
			out = append(out, "")
		default:
			out = append(out, "-- "+strings.Fields(strings.TrimPrefix(line, "▸"))[0])
		}
	}
	// The blank line under the title and the one the screen ends with are not
	// between sections.
	for len(out) > 0 && out[0] == "" {
		out = out[1:]
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

func rowIn(name string, group domain.Group, extra ...func(*domain.Action)) domain.Action {
	a := domain.Action{Name: name, Command: strings.ToLower(name), Group: group}
	for _, f := range extra {
		f(&a)
	}
	return a
}

// TestViewActions_AHeaderOpensEachRunOfOneGroup: the header is the group's
// label, once at the head of each run of consecutive rows of the group, with a
// blank line before every header but the first.
func TestViewActions_AHeaderOpensEachRunOfOneGroup(t *testing.T) {
	got := menuLines(t,
		rowIn("A", "Uno"), rowIn("B", "Uno"),
		rowIn("C", "Dos"), rowIn("D", "Dos"),
		rowIn("E", "Uno"),
	)
	want := []string{"== Uno", "-- A", "-- B", "", "== Dos", "-- C", "-- D", "", "== Uno", "-- E"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("menu = %q, want %q", got, want)
	}
}

// TestViewActions_TheHeaderIsNotDerivedFromTargetAgnostic: a target-agnostic
// row in the group above stays there; the header it used to open is opened by
// the group of the rows below it.
func TestViewActions_TheHeaderIsNotDerivedFromTargetAgnostic(t *testing.T) {
	agnostic := func(a *domain.Action) { a.TargetAgnostic = true }
	got := menuLines(t,
		rowIn("A", domain.GroupSync),
		rowIn("B", domain.GroupSync, agnostic),
		rowIn("C", domain.GroupHooks, agnostic),
		rowIn("D", domain.GroupHooks),
	)
	want := []string{"== Sincronización", "-- A", "-- B", "", "== Hooks", "-- C", "-- D"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("menu = %q, want %q: a header followed a target-agnostic row", got, want)
	}
}

// TestViewActions_TheHeaderIsNotDerivedFromTheCommand: rows whose commands are
// the ones that used to open the Skills and Memoria sections open nothing of
// the kind unless they say so.
func TestViewActions_TheHeaderIsNotDerivedFromTheCommand(t *testing.T) {
	named := func(command string) func(*domain.Action) { return func(a *domain.Action) { a.Command = command } }
	got := menuLines(t,
		rowIn("A", domain.GroupSync, named("skills")),
		rowIn("B", domain.GroupSync, named("longterm-mem")),
	)
	want := []string{"== Sincronización", "-- A", "-- B"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("menu = %q, want %q: a header followed the name of a command", got, want)
	}
}

// TestViewActions_AnActionWithNoGroupJoinsTheSectionAbove: it gets no header of
// its own, and the group above is not repeated after it.
func TestViewActions_AnActionWithNoGroupJoinsTheSectionAbove(t *testing.T) {
	got := menuLines(t, rowIn("A", "Uno"), rowIn("B", ""), rowIn("C", "Uno"))
	want := []string{"== Uno", "-- A", "-- B", "-- C"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("menu = %q, want %q", got, want)
	}
}

// TestViewActions_TheShippedMenuListsEachRowUnderItsGroup: "Actualizar
// repositorio" is part of the operational flow, not of the hooks lifecycle.
func TestViewActions_TheShippedMenuListsEachRowUnderItsGroup(t *testing.T) {
	m := newTestModel(t)
	m.scr = screenActions
	var section string
	under := map[string]string{}
	for _, line := range strings.Split(stripANSI(m.viewActions()), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "▸"))
		switch {
		case strings.HasPrefix(line, "── "):
			section = strings.Trim(line, "─ ")
		case line != "" && !strings.HasPrefix(line, "Elegir"):
			under[strings.Fields(line)[0]] = section
		}
	}

	want := map[string]string{
		"Estado": "Sincronización", "Verificar": "Sincronización", "Capturar": "Sincronización",
		"Aplicar": "Sincronización", "Restaurar": "Sincronización", "Actualizar": "Sincronización",
		"Instalar": "Hooks", "Desinstalar": "Hooks",
		"Skills":   "Skills",
		"Memoria:": "Memoria",
	}
	for row, section := range want {
		if under[row] != section {
			t.Errorf("%q is listed under %q, want %q", row, under[row], section)
		}
	}
}
