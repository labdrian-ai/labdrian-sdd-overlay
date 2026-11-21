package main

// 'review findings --feature <name> --stored <result> [--cwd <repo>]' is the findings feed of the learning loop
// (odd/tasks/merge-playbook-into-sdd-overlay.md, S3): it reads a stored result and the verdict beside it as 'review
// verify' and 'review check' read them (reviewapp.Service.Findings) and prints which findings count as evidence and
// which are left out and why. It stores nothing, and a result it cannot read is a refusal, never an empty list.

import (
	"fmt"

	reviewapp "github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
)

type reviewFindingsJSON struct {
	Review  string                     `json:"review"`
	Verdict string                     `json:"verdict,omitempty"`
	Kept    []reviewKeptFindingJSON    `json:"kept"`
	LeftOut []reviewLeftOutFindingJSON `json:"left_out"`
}

type reviewKeptFindingJSON struct {
	ID          string   `json:"id"`
	Location    string   `json:"location"`
	Severity    string   `json:"severity"`
	Claim       string   `json:"claim"`
	Class       string   `json:"evidence_class"`
	Disposition string   `json:"causal_disposition"`
	ProofRefs   []string `json:"proof_refs"`
}

type reviewLeftOutFindingJSON struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	Reason   string `json:"reason"`
}

// runReviewFindings implements 'review findings --feature --stored [--cwd]': exit 0 listed, 1 for a usage error, a
// result that cannot be read or any other failure.
func runReviewFindings(p process, d deps, args []string) {
	fail := func(err error) { reviewFailed(p, "findings", err) }
	f, err := parseReviewFlags(args, "--feature", "--stored", "--cwd")
	if err == nil {
		err = f.require("--feature", "--stored")
	}
	if err != nil {
		fail(err)
		return
	}
	dir, err := reviewPath(d, f.cwd)
	if err != nil {
		fail(err)
		return
	}
	listing, err := newReviewService(d).Findings(dir, f.feature, f.stored)
	if err != nil {
		fail(err)
		return
	}
	if err := writeReviewJSON(p, findingsJSON(listing)); err != nil {
		fail(fmt.Errorf("the findings were read, but the listing could not be written: %w", err))
		return
	}
	p.exit(0)
}

// findingsJSON is the listing as it is printed: the lists are never null, and keep the order of the reply.
func findingsJSON(l reviewapp.Findings) reviewFindingsJSON {
	out := reviewFindingsJSON{Review: l.Review, Verdict: l.Verdict, Kept: []reviewKeptFindingJSON{}, LeftOut: []reviewLeftOutFindingJSON{}}
	for _, k := range l.Kept {
		refs := k.ProofRefs
		if refs == nil {
			refs = []string{}
		}
		out.Kept = append(out.Kept, reviewKeptFindingJSON{
			ID: k.ID, Location: k.Location, Severity: string(k.Severity), Claim: k.Claim,
			Class: string(k.EvidenceClass), Disposition: string(k.Disposition), ProofRefs: refs,
		})
	}
	for _, o := range l.LeftOut {
		out.LeftOut = append(out.LeftOut, reviewLeftOutFindingJSON{ID: o.ID, Severity: string(o.Severity), Reason: o.Reason})
	}
	return out
}
