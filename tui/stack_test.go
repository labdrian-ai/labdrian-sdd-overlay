package main

// The one place the pinned sessions (pin_test.go) know how the program is put together. A session drives the
// program the way main builds it, against the stub backend, and reads only what a person sees (the screens) and
// what the backend receives (its invocations), so the same sessions and the same golden files hold the program
// whatever packages it is made of. When the packages change, this file is what changes with them.

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// stackModel builds the program's model over the backend under root, with the adapter main would choose.
func stackModel(root string, rootErr error) tea.Model {
	backend := newOverlayCLI(root)
	return newModel(deps{repoRoot: root, rootErr: rootErr, catalog: backend, backups: backend})
}

// menuDefinition lists the action menu as the program builds it: every row with the fields that decide how it is
// run and shown, and the sub-actions merged into it.
func menuDefinition() string {
	var b strings.Builder
	var dump func(a Action, indent string)
	dump = func(a Action, indent string) {
		fmt.Fprintf(&b, "%sName=%q Command=%q Args=%q\n", indent, a.Name, a.Command, a.Args)
		fmt.Fprintf(&b, "%s  Mutating=%t SupportsAll=%t TargetAgnostic=%t CopyTargetsOnly=%t\n",
			indent, a.Mutating, a.SupportsAll, a.TargetAgnostic, a.CopyTargetsOnly)
		fmt.Fprintf(&b, "%s  Hint=%q\n", indent, a.Hint)
		fmt.Fprintf(&b, "%s  ConfirmMessage=%q\n", indent, a.ConfirmMessage)
		for _, sub := range a.Also {
			dump(sub, indent+"    also: ")
		}
	}
	for _, a := range Actions() {
		dump(a, "")
	}
	return b.String()
}
