package main

// The entry of the 'skills <verb>' subcommand: the verbs themselves are skills_*.go.

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// runSkills implements the 'skills <verb>' subcommand.
// Requires exactly one verb argument; fails LOUD on missing or unknown verb (ADR-4).
func runSkills(args []string) {
	runSkillsWithStdin(args, os.Stdin, os.Stdout, os.Stderr, os.Exit)
}

// runSkillsCore is the testable core of the skills subcommand: the entry with the ports of the
// program.
func runSkillsCore(verb string, args []string, stdout, stderr io.Writer, exit func(int)) {
	runSkillsCoreWith(newSkillsDeps(), verb, args, stdout, stderr, exit)
}

// runSkillsCoreWith is the entry of the skills subcommand over the ports it is given: it names the
// verb the person typed, finds it in the table and runs it.
func runSkillsCoreWith(deps skills.Deps, verb string, args []string, stdout, stderr io.Writer, exit func(int)) {
	if verb == "" {
		fmt.Fprintf(stderr, "error: skills requires a verb: %s\n", skillsVerbList)
		exit(1)
		return
	}
	run, ok := skillsCLIVerbs[verb]
	if !ok {
		fmt.Fprintf(stderr, "error: unknown skills verb %q (supported: %s)\n", verb, skillsVerbList)
		exit(1)
		return
	}
	run(deps, args, stdout, stderr, exit)
}

// wallClockUTC is the production clock handed to the skills core: the current
// time as an RFC 3339 UTC timestamp with whole seconds, the only shape an
// approval record accepts. engine/skills cannot read the time itself because
// its import allowlist excludes "time", so the wall clock is decided here.
func wallClockUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// verbFromArgs extracts the first positional argument as the verb, empty if absent.
func verbFromArgs(args []string) string {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			return a
		}
	}
	return ""
}
