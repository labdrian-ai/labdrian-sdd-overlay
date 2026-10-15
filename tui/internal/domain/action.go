package domain

// ResultKind says how the result of running an action is read and shown.
type ResultKind int

const (
	// ResultTranscript shows the transcript of the invocations, as the backend
	// printed it, and nothing more. It is the zero value.
	ResultTranscript ResultKind = iota
	// ResultSyncDashboard takes the result as a sync-check report: the verdicts
	// the executions hold (Execution.Verdicts) become the per-target dashboard
	// above the transcript, and a report that holds none says so.
	ResultSyncDashboard
)

// Group is the section of the action menu an action is listed under. Its value
// is the label of the section's header, in the language of the menu.
type Group string

const (
	// GroupSync is the operational flow: look, capture, apply, restore, and
	// the self-update that brings the repository itself up to date.
	GroupSync Group = "Sincronización"
	// GroupHooks is the lifecycle of the hooks in ~/.claude/settings.json.
	GroupHooks Group = "Hooks"
	// GroupSkills is the skills registry: status, validate, list.
	GroupSkills Group = "Skills"
	// GroupMemory is the longterm-mem lifecycle: the MCP server registered in
	// each runtime's own configuration.
	GroupMemory Group = "Memoria"
)

// Action is a backend subcommand the TUI can invoke. What the program does
// differently for one action than for another is declared here, in fields,
// and never inferred from the Command string: a screen or a use case that
// needs to know "is this the one that..." asks a field.
type Action struct {
	Name            string   // label shown in the menu
	Command         string   // bin/labdrian-overlay subcommand
	Args            []string // additional positional args appended after Command
	Mutating        bool     // requires confirmation
	SupportsAll     bool     // can pass --target all when the selection is the whole catalog
	TargetAgnostic  bool     // when true: invoke WITHOUT --target, skip target selection
	CopyTargetsOnly bool     // the backend refuses package targets: run only against KindCopy ones
	ConfirmMessage  string   // per-action confirm copy; empty falls back to generic
	Hint            string   // one-line purpose scent shown in the menu
	Also            []Action // nested sub-actions merged into this menu entry (invisible in the menu)

	// Group is the menu section the action is listed under: a header opens
	// each run of consecutive actions of one group. An action with no group
	// joins the section above it. The menu's order is the actions' order, so
	// the actions of a group are written next to each other.
	Group Group
	// ResultKind is how the result of running the action is read and shown.
	ResultKind ResultKind
	// RefreshProbe says a successful run changes what the launch-time probe
	// reads (it moves the repository), so the probe runs again and the banner
	// can correct itself. The banner's shortcut offers the first action that
	// declares it: the one that can clear what the banner reports.
	RefreshProbe bool
	// RequiresBackup says the action is only ever offered for a target that
	// has a backup to go back to: before the confirm screen the backend is
	// asked what each selected target has, and the action runs against the
	// ones that have one.
	RequiresBackup bool
}

// ApplicableTo narrows selected to the targets this action can run against:
// all of them, or only the KindCopy ones when the backend refuses package
// targets for it. The model uses it to decide what an action will be invoked
// on, and the confirm screen names the same set.
func (a Action) ApplicableTo(selected []Target) []Target {
	if !a.CopyTargetsOnly {
		return selected
	}
	var out []Target
	for _, t := range selected {
		if t.Kind == KindCopy {
			out = append(out, t)
		}
	}
	return out
}

// UsesTargets reports whether running this action -- the primary invocation
// plus every merged Also entry -- ever touches per-target state, so the
// confirm screen knows whether to name which targets it will run against.
// An action is target-agnostic only when EVERY invocation it produces is
// itself TargetAgnostic; one target-using invocation (e.g. "apply" chained
// onto "self-update" so a single button both fast-forwards main AND deploys
// it) makes the combined action no longer target-agnostic from the user's
// point of view, even though the primary invocation never receives
// --target.
func (a Action) UsesTargets() bool {
	if !a.TargetAgnostic {
		return true
	}
	for _, sub := range a.Also {
		if !sub.TargetAgnostic {
			return true
		}
	}
	return false
}

// Actions returns the action menu in display order: Estado, then sync-check,
// then capture, then apply (the natural usage flow), then restore, then repo
// maintenance (self-update), then the hooks lifecycle, then skills, then the
// longterm-mem lifecycle (status, install, uninstall). Related read-only
// sub-actions (status-hooks, version; skills validate/list) are folded into
// their primary entry via Also instead of appearing as separate top-level menu
// rows.
//
// This comment enumerates the menu order, so it is part of the menu: adding,
// removing or moving a row without amending it here leaves a record that no
// longer describes what it records. TestActionMenuShapeAndOrder pins the same
// sequence as Command+Args.
func Actions() []Action {
	return []Action{
		{Name: "Estado", Command: "status", Group: GroupSync, Mutating: false, SupportsAll: true,
			Hint: "Resumen del estado actual (targets + hooks)",
			Also: []Action{
				{Command: "status-hooks", TargetAgnostic: true},
				// version (R-002): reports installed vs. latest per target,
				// same read-only shape as status-hooks -- folded in rather
				// than a separate top-level entry.
				{Command: "version", TargetAgnostic: true},
			}},
		{Name: "Verificar sincronización", Command: "sync-check", Group: GroupSync, Mutating: false, SupportsAll: true,
			ResultKind: ResultSyncDashboard,
			Hint:       "Compara overlay vs upstream"},
		// Capture reads a target's deployed files back into the repo, so it
		// only exists for per-file (copy) targets: the backend refuses a
		// package target such as pi.
		{Name: "Capturar (actualizar upstream)", Command: "capture", Group: GroupSync, Mutating: true, SupportsAll: false,
			CopyTargetsOnly: true,
			Hint:            "Trae cambios de upstream al overlay"},
		{Name: "Aplicar cambios", Command: "apply", Group: GroupSync, Mutating: true, SupportsAll: true,
			Hint: "Despliega el overlay en los destinos"},
		// Restore — per-target, never SupportsAll (the backend's cmd_restore
		// explicitly refuses --target all: restore is a single-target,
		// explicit, destructive rollback by design, D4). ConfirmMessage here
		// is the generic overwrite warning (R-003); the restore planner
		// replaces it per-invocation with the exact backup timestamp+version being
		// restored for each selected target (D4), and refuses to enter the
		// confirm screen at all when none of the selected targets actually
		// have a backup (never offer restore for a target with zero backups).
		// Like capture, restore is refused by the backend for a package
		// target (it keeps no per-target backups), hence CopyTargetsOnly.
		{Name: "Restaurar respaldo", Command: "restore", Group: GroupSync, Mutating: true, SupportsAll: false,
			CopyTargetsOnly: true, RequiresBackup: true,
			ConfirmMessage: "Restaura el respaldo más reciente de cada destino seleccionado,\n" +
				"sobrescribiendo sus archivos actualmente desplegados.",
			Hint: "Revierte un destino a su respaldo más reciente"},
		// Repo maintenance — TargetAgnostic: fast-forwards main only, via the
		// backend's self-update subcommand (D1-D3). Placed right after the
		// core apply flow and before the hooks block (D7). Chains "apply" via
		// Also so one button press both catches main up AND deploys it —
		// before this, a user had to remember to run "Aplicar cambios"
		// separately afterward, or the fix would sit fast-forwarded in the
		// repo but never reach ~/.claude/skills (the exact confusion this
		// change exists to remove: main was fully in sync, the deployed
		// skill was not, and the TUI gave no hint that a second step was
		// needed).
		{
			Name:           "Actualizar repositorio",
			Group:          GroupSync,
			Command:        "self-update",
			Mutating:       true,
			TargetAgnostic: true,
			RefreshProbe:   true,
			ConfirmMessage: "Actualiza main a origin/main (fast-forward) y despliega el resultado en los destinos seleccionados (equivalente a Aplicar cambios). Tu rama actual no se toca y se vuelve a ella al terminar. Rechaza con árbol sucio o main local adelantado.",
			Hint:           "Pone main al día (ff-only) y lo despliega, en un solo paso",
			Also: []Action{
				{Command: "apply", SupportsAll: true},
			},
		},
		// Hooks lifecycle — TargetAgnostic: operate on ~/.claude/settings.json globally.
		{
			Name:           "Instalar hooks",
			Group:          GroupHooks,
			Command:        "install-hooks",
			Mutating:       true,
			TargetAgnostic: true,
			ConfirmMessage: "Modifica ~/.claude/settings.json (se crea un respaldo .bak antes de escribir).",
			Hint:           "Compila engine y cablea hooks (paso 1)",
		},
		{
			Name:           "Desinstalar hooks",
			Group:          GroupHooks,
			Command:        "uninstall-hooks",
			Mutating:       true,
			TargetAgnostic: true,
			ConfirmMessage: "Elimina los hooks de ~/.claude/settings.json (se crea un respaldo .bak antes de modificar).",
			Hint:           "Quita los hooks de settings.json",
		},
		// Skills registry — read-only, TargetAgnostic. The bash backend injects
		// --registry/--manifest/--source-root defaults, so no flags are needed here.
		{Name: "Skills", Command: "skills", Group: GroupSkills, Args: []string{"status"},
			TargetAgnostic: true, Mutating: false,
			Hint: "Estado + valida + lista el registro",
			Also: []Action{
				{Command: "skills", Args: []string{"validate"}, TargetAgnostic: true},
				{Command: "skills", Args: []string{"list"}, TargetAgnostic: true},
			}},
		// longterm-mem lifecycle — the MCP server that writes an
		// ownership-tagged entry into each runtime's OWN configuration file
		// and manages the Obsidian vault. It installs today as a side effect
		// of `apply` (overlay.manifest routes it through the mcp step), so
		// without these rows the operator gets the writes and no way to see
		// them. All three are target-using: cmd_longterm_mem validates the
		// positional subcommand first, then resolves --target for every one of
		// them (defaulting to "all") -- hence TargetAgnostic false and
		// SupportsAll true on all three. Setting either the other way makes
		// the TUI hand the backend an invocation it rejects.
		//
		// --purge is deliberately NOT exposed here, on any row. It deletes the
		// SHARED binary that runtimes still registered against it depend on,
		// the TUI has no second-level destructive confirmation to put in front
		// of that, and a one-keystroke path to it from a menu is not
		// proportionate to what it does. It stays a deliberate CLI act.
		// TestNoActionPassesPurge keeps this omission deliberate.
		{Name: "Memoria: estado", Command: "longterm-mem", Group: GroupMemory, Args: []string{"status"},
			Mutating: false, SupportsAll: true,
			Hint: "Registro MCP de longterm-mem por runtime"},
		{Name: "Memoria: instalar", Command: "longterm-mem", Group: GroupMemory, Args: []string{"install"},
			Mutating: true, SupportsAll: true,
			ConfirmMessage: "Compila el binario de longterm-mem, lo despliega en el directorio de estado del overlay\n" +
				"y escribe una entrada MCP con marca de propiedad en la configuración propia de cada\n" +
				"runtime seleccionado (Claude Code, opencode, codex). Antes de modificar cualquier\n" +
				"configuración se escribe un respaldo .bak junto al archivo.",
			Hint: "Compila, despliega y registra el MCP en cada runtime"},
		{Name: "Memoria: desinstalar", Command: "longterm-mem", Group: GroupMemory, Args: []string{"uninstall"},
			Mutating: true, SupportsAll: true,
			ConfirmMessage: "Elimina únicamente la entrada MCP con marca de propiedad de cada runtime seleccionado.\n" +
				"Una entrada que longterm-mem no posee se deja intacta y se reporta como no gestionada.\n" +
				"El binario compartido sobrevive a una desinstalación parcial: solo se elimina al\n" +
				"desinstalar todos los runtimes, o con --purge desde la CLI (no disponible aquí).",
			Hint: "Quita la entrada MCP propia de cada runtime"},
	}
}
