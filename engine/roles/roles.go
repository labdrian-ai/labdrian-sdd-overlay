// Package roles defines the closed reusable-role vocabulary and the
// RoleHandoff v1 record that carries context, evidence, and interruption
// state between one role and the next. Roles are data only: a purpose, what
// a role consumes, and what it produces. Nothing here grants execution
// authority, launches an agent, or dispatches work; that belongs to
// runtime-specific adapters outside this package.
package roles

import "strings"

// Role is one member of the closed reusable-role vocabulary.
type Role string

// The closed role vocabulary, in canonical order. Sweeper and polisher are
// optional and may be skipped by a forward transition; every other role is
// mandatory.
const (
	RolePrototyper Role = "prototyper"
	RoleShaper     Role = "shaper"
	RoleEstimator  Role = "estimator"
	RoleBuilder    Role = "builder"
	RoleSweeper    Role = "sweeper"
	RolePolisher   Role = "polisher"
	RoleReviewer   Role = "reviewer"
	RoleDelivery   Role = "delivery"
)

// Vocabulary lists every role in canonical order.
var Vocabulary = []Role{
	RolePrototyper,
	RoleShaper,
	RoleEstimator,
	RoleBuilder,
	RoleSweeper,
	RolePolisher,
	RoleReviewer,
	RoleDelivery,
}

// RoleInfo is one role's data: what it is for, what it consumes, and what it
// produces. It grants no authority.
type RoleInfo struct {
	Role     Role
	Optional bool
	Purpose  string
	Consumes string
	Produces string
}

// Catalog holds RoleInfo for every role in canonical order.
var Catalog = []RoleInfo{
	{
		Role:     RolePrototyper,
		Purpose:  "Explore the problem space with throwaway spikes to de-risk unknowns before shaping a plan.",
		Consumes: "A rough goal or idea, with no committed scope yet.",
		Produces: "Exploration notes and a narrowed problem statement for shaping.",
	},
	{
		Role:     RoleShaper,
		Purpose:  "Turn a narrowed problem into a bounded, appetite-limited plan with scope and out-of-scope limits.",
		Consumes: "A goal and, when available, exploration notes.",
		Produces: "A handoff plan: scope, out-of-scope limits, roles, tests, risks, and estimates.",
	},
	{
		Role:     RoleEstimator,
		Purpose:  "Attach effort ranges and confidence to a shaped plan's stages before work is committed.",
		Consumes: "A shaped plan with its stages and acceptance criteria.",
		Produces: "Per-stage effort estimates and a stated confidence level.",
	},
	{
		Role:     RoleBuilder,
		Purpose:  "Implement the plan: write the code, tests, and docs for one bounded work unit.",
		Consumes: "A shaped, estimated plan and its acceptance criteria.",
		Produces: "Committed changes with passing checks and evidence of what was built.",
	},
	{
		Role:     RoleSweeper,
		Optional: true,
		Purpose:  "Sweep the built change for loose ends: dead code, stale comments, and leftover scaffolding.",
		Consumes: "A built change and its evidence.",
		Produces: "A cleaned change with sweep findings recorded as evidence.",
	},
	{
		Role:     RolePolisher,
		Optional: true,
		Purpose:  "Polish presentation: naming, messages, and documentation clarity, without changing behavior.",
		Consumes: "A built (and optionally swept) change.",
		Produces: "A polished change with the same behavior and clearer presentation.",
	},
	{
		Role:     RoleReviewer,
		Purpose:  "Review the change for correctness, risk, and reviewability; approve or send it back for rework.",
		Consumes: "A built change, its evidence, and its acceptance criteria.",
		Produces: "A review verdict: approved for delivery, or sent back to the builder with findings.",
	},
	{
		Role:     RoleDelivery,
		Purpose:  "Close the loop: the change is delivered and the chain for this goal is complete.",
		Consumes: "An approved, reviewed change.",
		Produces: "A terminal delivery record; no further role handoff follows in this chain.",
	},
}

// roleIndex maps each role to its position in the canonical sequence.
var roleIndex = func() map[Role]int {
	m := make(map[Role]int, len(Vocabulary))
	for i, r := range Vocabulary {
		m[r] = i
	}
	return m
}()

// optionalRole reports whether r is skippable in a forward transition.
var optionalRole = map[Role]bool{
	RoleSweeper:  true,
	RolePolisher: true,
}

// IsKnownRole reports whether r is a member of the closed vocabulary.
func IsKnownRole(r Role) bool {
	_, ok := roleIndex[r]
	return ok
}

// IsValidTransition reports whether a role handoff from from to to is
// allowed: forward to a later role in canonical order, skipping only
// optional roles strictly in between, or the single allowed backward
// transition reviewer -> builder (rework). Delivery is terminal: no
// transition begins from delivery. An unknown role on either side is never
// valid.
func IsValidTransition(from, to Role) bool {
	if !IsKnownRole(from) || !IsKnownRole(to) {
		return false
	}
	if from == RoleDelivery {
		return false
	}
	if from == RoleReviewer && to == RoleBuilder {
		return true
	}
	fi, ti := roleIndex[from], roleIndex[to]
	if ti <= fi {
		return false
	}
	for i := fi + 1; i < ti; i++ {
		if !optionalRole[Vocabulary[i]] {
			return false
		}
	}
	return true
}

// NextRoles returns, in canonical order, every role IsValidTransition allows
// as a next role from from. It returns nil when from is delivery (terminal)
// or unknown.
func NextRoles(from Role) []Role {
	if !IsKnownRole(from) {
		return nil
	}
	var next []Role
	for _, to := range Vocabulary {
		if IsValidTransition(from, to) {
			next = append(next, to)
		}
	}
	return next
}

// EmptyChainDigest is the sentinel prev_sha256 for the first record of a
// chain: 64 ASCII zero characters. It is not a real digest; it marks "no
// previous record."
var EmptyChainDigest = strings.Repeat("0", 64)
