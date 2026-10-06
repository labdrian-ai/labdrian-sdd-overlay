package main

// The one parser of the command line of the verbs of `engine skills` that sit behind a use case
// (Phase 9 unit H20, decision D4). A verb says what it reads in a skillsFlagSpec; the parser
// splits the arguments into the flags the verb reads, the flags the wrapper appends to every
// verb and no verb reads, and the words that are no flag, and refuses a flag nobody named. It
// knows no verb and prints nothing: the adapter of a verb says what a refusal costs.

import (
	"fmt"
	"strings"
)

// skillsFlagSpec is the command line of one verb.
type skillsFlagSpec struct {
	// verb names the verb in a refusal: "skills <verb>: ...".
	verb string
	// values are the flags the verb reads, each followed by its value. A value flag that is the
	// last word has no value to take and is left unset, as the verbs always did; one that is
	// followed by another flag takes that flag as its value.
	values []string
	// switches are the flags the verb reads that take no value.
	switches []string
	// wrapper are the flags the `labdrian` wrapper appends after the arguments of every verb
	// (--registry, --manifest, --source-root) which this verb does not read: each is taken with its
	// value and read by no one, so that a verb that needs none of them is not refused for them.
	wrapper []string
	// words says how many words that are no flag the verb accepts: at most this many, or any
	// number, which are then ignored, when it is negative.
	words int
	// extraWord is the refusal of a word past the limit, with %q for the word.
	extraWord string
	// endOfOptions makes a bare "--" end the flags: every word after it is a word, even one that
	// begins with a dash.
	endOfOptions bool
}

// skillsArgs is a command line split by a skillsFlagSpec.
type skillsArgs struct {
	// values holds the last value given to each value flag that was given one.
	values map[string]string
	// switches holds the switches that were given.
	switches map[string]bool
	// words are the words that are no flag, in order.
	words []string
}

// value is the value of the flag, or fallback when it was not given one.
func (a skillsArgs) value(flag, fallback string) string {
	if v, ok := a.values[flag]; ok {
		return v
	}
	return fallback
}

// skillsUsageError is a command line the verb refuses, in the words of its refusal. The adapter
// prints "error: " and the message and exits 1.
type skillsUsageError struct{ message string }

func (e *skillsUsageError) Error() string { return e.message }

// parse splits args. A flag is every word that begins with a dash; one that the spec does not
// name is refused, naming it, before anything is read or written.
func (s skillsFlagSpec) parse(args []string) (skillsArgs, error) {
	out := skillsArgs{values: map[string]string{}, switches: map[string]bool{}}
	endOfOptions := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !endOfOptions {
			switch {
			case s.endOfOptions && arg == "--":
				endOfOptions = true
				continue
			case contains(s.values, arg):
				if i+1 < len(args) {
					out.values[arg] = args[i+1]
					i++
				}
				continue
			case contains(s.switches, arg):
				out.switches[arg] = true
				continue
			case contains(s.wrapper, arg):
				if i+1 < len(args) {
					i++
				}
				continue
			case strings.HasPrefix(arg, "-"):
				return skillsArgs{}, &skillsUsageError{fmt.Sprintf("skills %s: unknown flag %q", s.verb, arg)}
			}
		}
		if s.words >= 0 && len(out.words) >= s.words {
			return skillsArgs{}, &skillsUsageError{fmt.Sprintf(s.extraWord, arg)}
		}
		out.words = append(out.words, arg)
	}
	return out, nil
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

// The flags the wrapper appends to every verb.
const (
	flagRegistry   = "--registry"
	flagManifest   = "--manifest"
	flagSourceRoot = "--source-root"
)

// skillsRegistryReaderSpec is the command line of a verb that only reads the registry and says
// something of it (list, status): --registry is the flag it reads, the other two of the wrapper
// are taken and not read, and a word that is no flag is not read either.
func skillsRegistryReaderSpec(verb string) skillsFlagSpec {
	return skillsFlagSpec{verb: verb, values: []string{flagRegistry}, wrapper: []string{flagManifest, flagSourceRoot}, words: -1}
}

// skillsLintSpec is the command line of `skills lint`: one path or --rules, the flags of the
// wrapper taken and not read, and "--" to lint a path that begins with a dash. A second path is
// refused instead of dropped, which would let `lint a.md b.md` check the first file only
// and say it was clean.
var skillsLintSpec = skillsFlagSpec{
	verb:         "lint",
	switches:     []string{"--rules"},
	wrapper:      []string{flagRegistry, flagManifest, flagSourceRoot},
	words:        1,
	extraWord:    "skills lint: unexpected extra argument %q (lint accepts exactly one path)",
	endOfOptions: true,
}
