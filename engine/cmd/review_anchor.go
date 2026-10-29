package main

// review anchor: 'review anchor --merge <rev> [--cwd <repo>]' decides whether the merge of a pull request lands a tree
// that stored reviews cleared (own-rdd T7b, S7). The decision is the use case reviewapp.Service.CheckLanding; this file
// parses the command line, composes the service as the other review verbs do and says what it found.

import (
	"fmt"

	reviewapp "github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
)

// reviewLandingJSON is what 'review anchor' prints for a merge that lands a reviewed tree, in the order of S7 (f). The
// tree is the tree of the head and is left out when the merge changes no feature document.
type reviewLandingJSON struct {
	Merge    string                    `json:"merge"`
	Base     string                    `json:"base"`
	Head     string                    `json:"head"`
	Tree     string                    `json:"tree,omitempty"`
	Outcome  string                    `json:"outcome"`
	Features []reviewLandedFeatureJSON `json:"features"`
}

// reviewLandedFeatureJSON is one feature the merge changes: clear, or refused with the reason of its chain.
type reviewLandedFeatureJSON struct {
	Feature string `json:"feature"`
	Outcome string `json:"outcome"`
	Reason  string `json:"reason,omitempty"`
}

// runReviewAnchor implements 'review anchor --merge <rev> [--cwd <repo>]'. A merge that lands a reviewed tree, or that
// changes no feature document, is exit 0 with the landing as JSON on stdout. Every refusal, and any failure to tell, is
// exit 1 with the reason on stderr and nothing on stdout. Nothing is stored.
func runReviewAnchor(p process, d deps, args []string) {
	fail := func(err error) { reviewFailed(p, "anchor", err) }
	f, err := parseReviewFlags(args, "--merge", "--cwd")
	if err == nil {
		err = f.require("--merge")
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
	landing, err := newReviewService(d).CheckLanding(dir, f.merge)
	if err != nil {
		fail(err)
		return
	}
	if err := writeReviewJSON(p, landingJSON(landing)); err != nil {
		fail(fmt.Errorf("the merge lands a reviewed tree, but the answer could not be written: %w", err))
		return
	}
	p.exit(0)
}

// landingJSON is the answer of a merge that lands a reviewed tree.
func landingJSON(landing reviewapp.Landing) reviewLandingJSON {
	out := reviewLandingJSON{
		Merge: landing.Merge, Base: landing.Base, Head: landing.Head, Tree: landing.Tree, Outcome: landing.Outcome,
		Features: []reviewLandedFeatureJSON{},
	}
	for _, feature := range landing.Features {
		out.Features = append(out.Features, reviewLandedFeatureJSON{Feature: feature.Feature, Outcome: feature.Outcome, Reason: feature.Reason})
	}
	return out
}
