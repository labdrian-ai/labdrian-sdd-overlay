package shaper

import "fmt"

// ForgeryDisclosure is the trust limit every output that can report the
// ready state must print. A ready outcome is not a signature: any process
// running as the same OS user, including any installed Pi extension, can
// forge a clearance record, and ready grants no execution authority.
const ForgeryDisclosure = "Trust limit: this clearance is bound to exact content by SHA-256 digests only; it is not a signature. " +
	"Any process running as the same OS user, including any installed Pi extension, can forge a clearance record, " +
	"and the Claude Code and Pi deny guards are speed bumps, not a security boundary. " +
	"Readiness grants no execution authority, and the roadmap's full Phase 3 plan outcome is not yet met."

// PlanIncompleteDisclosure states what a ready claim does not cover. Ready is
// claimed on the approved handoff core plus per-criterion planned acceptance
// verification only; planned checks are not executed, and their results are
// downstream fulfillment evidence. Ready is not a signature: any process
// running as the same OS user can forge a clearance record.
const PlanIncompleteDisclosure = "Plan completeness: ready covers the approved handoff core and the planned verification of each acceptance criterion only. " +
	"The roadmap's full Phase 3 plan outcome is not yet met: roles, tests, risks, estimates, memory_scope, and delivery_limit are absent. " +
	"Planned checks were not executed; their results are downstream fulfillment evidence, not proof that any criterion was met."

// ReadyDisclosure is what every output reporting the ready state must print:
// the forgery limit first (ready is not a signature: any process running as
// the same OS user, including any installed Pi extension, can forge a
// clearance record), then the plan completeness limit.
const ReadyDisclosure = ForgeryDisclosure + " " + PlanIncompleteDisclosure

// ReadContainedSource reads relPath strictly inside worktreeRoot under the
// same containment rules as LoadHandoff and BindGoal, without parsing it. It
// returns the cleaned root-relative path and the exact bytes read. A caller
// uses it to hand bytes that failed a strict parse to Evaluate, which then
// reports them as a blocker instead of an I/O failure.
func ReadContainedSource(worktreeRoot, relPath string) (string, []byte, error) {
	cleaned, err := cleanContainedRelPath(worktreeRoot, "path", relPath)
	if err != nil {
		return "", nil, fmt.Errorf("read source: %w", err)
	}
	data, err := readContainedRegularFile(worktreeRoot, cleaned, "source")
	if err != nil {
		return "", nil, fmt.Errorf("read source: %w", err)
	}
	return cleaned, data, nil
}

// SourceSHA256 is the lowercase hex SHA-256 of source bytes. It detects
// drift only and is not a signature.
func SourceSHA256(data []byte) string {
	return sha256Hex(data)
}
