// Package guardmarkers holds the two texts the shaper clearance guard is built on: the command that
// records a clearance and the directory the clearances are kept in. They are constants and nothing
// else.
//
// They live apart from the shaper because two packages need the same words and neither should
// import the other. The shaper's guard decides whether a tool call names them; the settings
// package writes them into the hook commands and the deny rule that Claude Code runs before a tool
// does. With the words here, the settings package no longer reaches the shaper, and through it the
// goal and strict-JSON packages the shaper imports.
package guardmarkers

const (
	// Command is the clearance record entry point the runtime deny guards refuse to let a model
	// run. It is matched anywhere in a command after whitespace is collapsed.
	Command = "shaper clearance record"

	// Store is the clearance store path segment the runtime deny guards refuse to let a model
	// touch. It is the fixed store directory under the state home.
	Store = "labdrian/shaper-clearance"
)
