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
	// skipDoubleDash makes a bare "--" be dropped, and the flags after it still flags: the way
	// the verbs that always let it pass unread (add, remove, sync-manifest) took it. The labdrian
	// wrapper appends its flags after the arguments of the verb, so a "--" that ended the flags
	// would turn the registry the wrapper names into a word and leave the verb on its defaults.
	skipDoubleDash bool
	// valueIsNeverAFlag makes a value flag refuse a value that begins with a dash, because it is
	// far more likely to be the next flag than a value; the flag is refused too when it is the
	// last word and has no value at all. Without it a flag takes whatever follows it. It applies
	// to the flags the wrapper appends as well.
	valueIsNeverAFlag bool
	// dashValue, when it is set, words the refusal of a value that begins with a dash for the
	// flag it names, for a flag whose values are free text (an approver label); it returns "" for
	// the other flags, which are refused in the generic words.
	dashValue func(flag, value string) string
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
			case s.skipDoubleDash && arg == "--":
				continue
			case containsFlag(s.values, arg):
				value, taken, err := s.takeValue(args, i)
				if err != nil {
					return skillsArgs{}, err
				}
				if taken {
					out.values[arg] = value
					i++
				}
				continue
			case containsFlag(s.switches, arg):
				out.switches[arg] = true
				continue
			case containsFlag(s.wrapper, arg):
				_, taken, err := s.takeValue(args, i)
				if err != nil {
					return skillsArgs{}, err
				}
				if taken {
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

// takeValue is the value of the flag at args[i], and whether there is one to take. A flag that is
// the last word has none, and the verbs always left it unset; with valueIsNeverAFlag it is refused.
func (s skillsFlagSpec) takeValue(args []string, i int) (value string, taken bool, err error) {
	flag := args[i]
	if i+1 >= len(args) {
		if s.valueIsNeverAFlag {
			return "", false, &skillsUsageError{fmt.Sprintf("skills %s: flag %q requires a value", s.verb, flag)}
		}
		return "", false, nil
	}
	value = args[i+1]
	if s.valueIsNeverAFlag && strings.HasPrefix(value, "-") {
		if s.dashValue != nil {
			if message := s.dashValue(flag, value); message != "" {
				return "", false, &skillsUsageError{message}
			}
		}
		return "", false, &skillsUsageError{fmt.Sprintf("skills %s: flag %q requires a value; got flag token %q", s.verb, flag, value)}
	}
	return value, true, nil
}

// containsFlag reports whether the word is one of the flags a spec names.
func containsFlag(list []string, s string) bool {
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

// skillsValidateSpec is the command line of `skills validate`: the registry, the manifest and the
// source root it checks, in any order, and any words that are no flag, which it does not read.
var skillsValidateSpec = skillsFlagSpec{verb: "validate", values: []string{flagRegistry, flagManifest, flagSourceRoot}, words: -1}

// Flags of the verbs that write.
const (
	flagRepo     = "--repo"
	flagRef      = "--ref"
	flagID       = "--id"
	flagApprover = "--approver"
	// flagProjectID names the project a directory is, when the person says so.
	flagProjectID = "--project-id"
)

// skillsAddSpec is the command line of `skills add`: the registry, the manifest and the skills tree
// it works in, the repository and ref of an external skill, and the id, which is the first word
// (a later word is not read). "--" ends the flags, as it always passed unread.
var skillsAddSpec = skillsFlagSpec{
	verb:           "add",
	values:         []string{flagRegistry, flagManifest, flagSourceRoot, flagRepo, flagRef},
	words:          -1,
	skipDoubleDash: true,
}

// skillsRemoveSpec is the command line of `skills remove`: the registry and the manifest, the
// source root of the wrapper taken and not read, and the id as the first word.
var skillsRemoveSpec = skillsFlagSpec{
	verb:           "remove",
	values:         []string{flagRegistry, flagManifest},
	wrapper:        []string{flagSourceRoot},
	words:          -1,
	skipDoubleDash: true,
}

// skillsSyncSpec is the command line of `skills sync-manifest`: the registry and the manifest, the
// source root of the wrapper taken and not read, and no word it reads.
var skillsSyncSpec = skillsFlagSpec{
	verb:           "sync-manifest",
	values:         []string{flagRegistry, flagManifest},
	wrapper:        []string{flagSourceRoot},
	words:          -1,
	skipDoubleDash: true,
}

// skillsApproveSpec is the command line of `skills approve`: the skill, the approver and the skills
// tree, the registry whose lock it takes (it reads nothing from it), and the manifest of the
// wrapper, taken and not read. A value is never a
// flag, and there is no word and no end of options: approve takes no positional argument, so "--"
// would have nothing to protect and is refused as the unknown flag it is.
var skillsApproveSpec = skillsFlagSpec{
	verb:              "approve",
	values:            []string{flagID, flagApprover, flagSourceRoot, flagRegistry},
	wrapper:           []string{flagManifest},
	words:             0,
	extraWord:         "skills approve: unexpected argument %q (approve takes --id, not a positional)",
	valueIsNeverAFlag: true,
	// A label is free text, and the one value where a leading dash is at all plausible.
	dashValue: func(flag, value string) string {
		if flag != flagApprover {
			return ""
		}
		return fmt.Sprintf("skills approve: flag %q: the label %q starts with \"-\", which would be read as a flag; choose a label that does not start with \"-\"", flag, value)
	},
}

// skillsInstallSpec is the command line of `skills install` and of `skills adopt`, which read the
// same flags: the registry that admits skills to projects, the tree they are read from, and the id
// of the project; the manifest of the wrapper is taken and not read. A word that is no flag is not
// read, and a "--" is dropped, as install always passed both unread.
func skillsInstallSpec(verb string) skillsFlagSpec {
	return skillsFlagSpec{
		verb:           verb,
		values:         []string{flagRegistry, flagSourceRoot, flagProjectID},
		wrapper:        []string{flagManifest},
		words:          -1,
		skipDoubleDash: true,
	}
}
