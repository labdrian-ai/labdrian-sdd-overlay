package main

import (
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/runtimes"
)

// The words of a request refused before any runtime was touched, with the flags that put it right. The register
// goldens hold them end to end; these hold each one alone, and the one that has two forms.
func TestRefusalWords_AreTheWordsOfEachRefusal(t *testing.T) {
	for name, tc := range map[string]struct {
		refusal runtimes.Refusal
		want    string
	}{
		"no target":                       {runtimes.Refusal{Kind: runtimes.TargetRequired}, "--target is required"},
		"a target that names no runtime":  {runtimes.Refusal{Kind: runtimes.UnknownTarget, Target: "bogus"}, `unknown --target "bogus" (want claude|opencode|codex|pi|all)`},
		"a config root for every runtime": {runtimes.Refusal{Kind: runtimes.ConfigRootNeedsOneTarget}, `--config-root requires a single --target, not "all"`},
		"no state directory":              {runtimes.Refusal{Kind: runtimes.StateDirUnresolved}, "could not resolve the install-state directory; set HOME or pass --state-dir"},
		"no binary":                       {runtimes.Refusal{Kind: runtimes.BinaryUnresolved}, "could not resolve the longterm-mem binary path; set HOME or pass --binary"},
		"pi, where its package is looked for": {
			runtimes.Refusal{Kind: runtimes.PiNotInstalled, Root: "/home/someone/.labdrian-overlay/pi/labdrian-pi"},
			"pi: not installed (no package at /home/someone/.labdrian-overlay/pi/labdrian-pi, or not listed in ~/.pi/agent/settings.json); build it (labdrian-overlay apply --target pi) and run pi install first",
		},
		// A machine that cannot tell where the package is built has no place to say it was not found at: the words
		// say that, and what puts it right, and name no empty path.
		"pi, where the package directory cannot be told": {
			runtimes.Refusal{Kind: runtimes.PiNotInstalled},
			"pi: not installed (the place its package is built could not be told: set HOME or pass --config-root); build it (labdrian-overlay apply --target pi) and run pi install first",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := refusalWords(tc.refusal); got != tc.want {
				t.Errorf("refusalWords = %q, want %q", got, tc.want)
			}
		})
	}
}
