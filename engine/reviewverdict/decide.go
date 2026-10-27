package reviewverdict

import "github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewresult"

// Outcome is how a verified review ends.
type Outcome string

// The outcomes of a verified review.
const (
	// Blocks: a finding blocks, so the result goes back to the builder.
	Blocks Outcome = "blocks"
	// Unverified: nothing blocks, but a finding that could block was not checked (the check did not run).
	// The review is not clear, and it does not go to delivery until the check has run.
	Unverified Outcome = "unverified"
	// Clear: no finding blocks and none is left unchecked, so the result goes to delivery.
	Clear Outcome = "clear"
)

// Decision is what the verdicts make of the findings of a result, by the rule of skills/role-chain/SKILL.md.
// Each list holds finding ids in the order of the result, and is nil when it holds none.
type Decision struct {
	Outcome Outcome
	// Blocking are the severe findings the diff caused and the verifier reproduced.
	Blocking []string
	// Unverified are the severe findings that could block and whose check did not run.
	Unverified []string
	// Fix are the WARNING findings, to be fixed before delivery.
	Fix []string
	// Carry are the SUGGESTION findings and the severe findings of the base, to be carried as follow-ups.
	Carry []string
}

// Decide applies the rule of skills/role-chain/SKILL.md to the findings of a result and the verdicts that
// answer them. It reads nothing and keeps nothing, so the same arguments always give the same Decision.
//
//   - A severe finding (BLOCKER or CRITICAL) blocks only when it is introduced, behavior-activated or
//     worsened and the verifier reproduced it on head. One it did not reproduce does not block. One whose
//     check did not run is unverified: a check that cannot run never passes.
//   - A severe finding that is pre-existing or base-only never blocks: it is carried.
//   - A severe finding of unknown causality is checked on base first. One that reproduces on base is
//     carried, since the diff did not cause it; one that does not is decided like a finding the diff caused;
//     when the base check did not run it is unverified, unless head did not reproduce it either.
//   - A WARNING is fixed before delivery and a SUGGESTION is carried, whatever the verdict.
//
// A finding with no verdict is read as a check that did not run, never as one that did not reproduce. The
// outcome is Blocks when anything blocks, else Unverified when anything is unverified, else Clear.
func Decide(r reviewresult.Result, verdicts []Verdict) Decision {
	byID := make(map[string]Verdict, len(verdicts))
	for _, v := range verdicts {
		byID[v.ID] = v
	}
	var d Decision
	for _, f := range r.Findings {
		switch f.Severity {
		case reviewresult.SeverityWarning:
			d.Fix = append(d.Fix, f.ID)
		case reviewresult.SeveritySuggestion:
			d.Carry = append(d.Carry, f.ID)
		default: // BLOCKER and CRITICAL, and any value the contract does not list, which is read as severe
			switch severeFinding(f.Disposition, byID[f.ID]) {
			case blocks:
				d.Blocking = append(d.Blocking, f.ID)
			case unchecked:
				d.Unverified = append(d.Unverified, f.ID)
			case ofTheBase:
				d.Carry = append(d.Carry, f.ID)
			}
		}
	}
	switch {
	case len(d.Blocking) > 0:
		d.Outcome = Blocks
	case len(d.Unverified) > 0:
		d.Outcome = Unverified
	default:
		d.Outcome = Clear
	}
	return d
}

// A severe finding ends in one of these.
type fate int

const (
	refuted   fate = iota // the verifier did not reproduce it: nothing to do
	blocks                // it blocks
	unchecked             // the check it needs did not run
	ofTheBase             // the base has it too: a follow-up
)

// severeFinding says what becomes of a severe finding of the given causal disposition, from its verdict.
func severeFinding(disposition reviewresult.Disposition, v Verdict) fate {
	switch disposition {
	case reviewresult.DispositionPreExisting, reviewresult.DispositionBaseOnly:
		return ofTheBase
	case reviewresult.DispositionUnknown:
		if v.Result == NotReproduced {
			return refuted
		}
		switch v.Base {
		case Reproduced:
			return ofTheBase
		case NotReproduced: // not on base: head decides, below
		default:
			return unchecked
		}
	}
	switch v.Result {
	case Reproduced:
		return blocks
	case NotReproduced:
		return refuted
	default:
		return unchecked
	}
}
