// Package capability defines the typed capability model a runtime adapter
// uses to declare, with evidence, what it supports: a closed set of
// capabilities, a closed set of statuses, and one declaration per runtime
// whose every claim states its own proof or its own limit.
//
// A claim is honest by construction. A "supported" or "partial" claim names
// at least one test that proves it, and the evidence guard (CheckEvidence)
// verifies that each named test exists; a "partial" or "unsupported" claim
// must state the limit in words. Nothing in this package runs a runtime, so
// a declaration describes what the engine's own tests prove, never what a
// runtime happened to do on some machine.
//
// The status vocabulary is deliberately not runtime.CapabilityStatus: that
// type describes the result of one lifecycle action (and includes
// restart_required), while a Status here describes how much of a capability
// is proven. This package is a leaf over the standard library. It must not
// import engine/runtime, which would pull the Pi adapter's os/exec
// dependency into every consumer; a static test in engine/runtime pins this.
package capability

import "encoding/json"

// Capability is one member of the closed capability vocabulary.
type Capability string

// The closed capability vocabulary. The wire values are the JSON strings.
const (
	// Installation is installing, updating, uninstalling, and reporting the
	// status of the overlay's files inside the runtime's configuration.
	Installation Capability = "installation"
	// Projection is putting the active workflow (Goal, Profile, stage) into
	// a session the user already opened.
	Projection Capability = "projection"
	// Dispatch is the engine steering a session. The engine never starts
	// sessions; dispatch means projection into an existing one.
	Dispatch Capability = "dispatch"
	// Cancellation is stopping a workflow's influence on a session.
	Cancellation Capability = "cancellation"
	// Persistence is keeping the workflow a session follows on disk, outside
	// the session.
	Persistence Capability = "persistence"
	// Restart is a new session picking up the on-disk workflow again.
	Restart Capability = "restart"
	// Authentication is knowing whether the runtime has credentials.
	Authentication Capability = "authentication"
	// MemoryEnforcement is enforcing a memory plan inside a session.
	MemoryEnforcement Capability = "memory-enforcement"
	// Skills is carrying the overlay's governed skills into the runtime's skill
	// directories: the global tier through labdrian apply, and the project tier
	// through skills install, skills adopt, and the project-register family. Phase
	// 8 adds it after the Phase 7 set, so the positions of the others do not move.
	// Its claims are stated per runtime from the tests that prove them; the
	// engine declares the global tier's dependence on apply as a limit, because it
	// has no installer of its own for it.
	Skills Capability = "skills"
)

// capabilityOrder is the fixed order of the closed set. Declarations store
// their claims in this order, so two declarations are comparable by position
// and the printed JSON is stable.
var capabilityOrder = [...]Capability{
	Installation, Projection, Dispatch, Cancellation,
	Persistence, Restart, Authentication, MemoryEnforcement,
	Skills,
}

// Capabilities returns the closed capability set in its fixed order. The
// returned slice is a copy; callers may modify it.
func Capabilities() []Capability {
	out := make([]Capability, len(capabilityOrder))
	copy(out, capabilityOrder[:])
	return out
}

// Status is one member of the closed claim-status vocabulary.
type Status string

// The closed status vocabulary.
const (
	// Supported means the capability is provided and every behavior the
	// claim covers is proven by the named tests.
	Supported Status = "supported"
	// Partial means part of the capability is proven; Detail states the
	// limit.
	Partial Status = "partial"
	// Unsupported means the capability is not provided; Detail states why.
	Unsupported Status = "unsupported"
)

// The runtime targets a declaration can describe. They are plain strings, not
// runtime.Target values, so this package does not depend on engine/runtime; a
// test in engine/runtime pins that the two sets stay equal.
const (
	TargetClaude   = "claude"
	TargetCodex    = "codex"
	TargetPi       = "pi"
	TargetOpenCode = "opencode"
)

// targetOrder is the fixed order in which declarations are listed.
var targetOrder = [...]string{TargetClaude, TargetCodex, TargetPi, TargetOpenCode}

// Targets returns the declared runtime targets in their fixed order. The
// returned slice is a copy; callers may modify it.
func Targets() []string {
	out := make([]string, len(targetOrder))
	copy(out, targetOrder[:])
	return out
}

// Claim is one statement about one capability: its status, the tests that
// prove it, and, for a partial or unsupported claim, the limit.
//
// Tests are references of the form <directory relative to engine/>:<TestName>,
// for example "runtime:TestPiAdapter_UninstallUsesRemoveNotUninstall".
type Claim struct {
	Capability Capability `json:"capability"`
	Status     Status     `json:"status"`
	Tests      []string   `json:"tests"`
	Detail     string     `json:"detail,omitempty"`
}

// MarshalJSON encodes the claim with the same field order and tags as the
// struct, except that absent tests always serialize as an empty array. A nil
// slice would otherwise print as null, and a consumer must be able to read
// "tests" as an array on every claim, proven or not.
func (c Claim) MarshalJSON() ([]byte, error) {
	// plain has Claim's fields and tags but none of its methods, so encoding
	// it does not recurse back into this method.
	type plain Claim
	p := plain(c)
	if p.Tests == nil {
		p.Tests = []string{}
	}
	return json.Marshal(p)
}

// Declaration is everything one runtime declares about itself: one claim per
// capability, in the closed set's fixed order.
//
// Untested is empty for a runtime that can be exercised on this machine. It
// carries the reason, and only then, when the runtime cannot be run here, so
// every claim of such a declaration is bounded by tests that never ran the
// runtime itself.
type Declaration struct {
	Target   string  `json:"target"`
	Untested string  `json:"untested,omitempty"`
	Claims   []Claim `json:"claims"`
}
