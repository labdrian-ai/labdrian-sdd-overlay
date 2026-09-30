package skills

// One reader of the project verbs' command line. project-register, project-revise,
// project-retire, and project-status read the same flags the same way; the lock layer
// (lock.go) must lock the directory they then work in, so it cannot read
// --project-root by any other rule than theirs. Both call parseProjectArgs.

import (
	"fmt"
	"strings"
)

// projectArgSpec says which flags a project verb takes and how it names itself.
// --manifest and --source-root are taken by all of them and ignored: the labdrian
// wrapper appends them to every skills verb, and a value must not be mistaken for
// the positional argument.
type projectArgSpec struct {
	verb         string // the verb as typed, in every message
	dryRun       bool   // --dry-run
	candidate    bool   // --candidate <key>
	reason       bool   // --reason <text>
	absorbedInto bool   // --absorbed-into <target>
	// extraHint follows "unexpected extra argument" when the verb has a positional
	// argument worth naming ("project-register accepts exactly one <draft-file>").
	extraHint string
}

var (
	projectRegisterSpec = projectArgSpec{verb: "project-register", dryRun: true, candidate: true,
		extraHint: "project-register accepts exactly one <draft-file>"}
	projectReviseSpec = projectArgSpec{verb: "project-revise", dryRun: true, candidate: true,
		extraHint: "project-revise accepts exactly one <draft-file>"}
	projectRetireSpec = projectArgSpec{verb: "project-retire", dryRun: true, reason: true, absorbedInto: true,
		extraHint: "project-retire accepts exactly one <id>"}
	projectStatusSpec = projectArgSpec{verb: "project-status"}
)

// projectArgSpecs is every verb that takes --project-root, by name: the lock layer
// looks the verb up here, so a verb that reads the flag and is not listed takes no
// lock and TestEveryProjectVerbIsLockedOnItsOwnRoot fails.
var projectArgSpecs = map[string]projectArgSpec{
	projectRegisterSpec.verb: projectRegisterSpec,
	projectReviseSpec.verb:   projectReviseSpec,
	projectRetireSpec.verb:   projectRetireSpec,
	projectStatusSpec.verb:   projectStatusSpec,
}

// projectArgs is what a project verb's command line said.
type projectArgs struct {
	Root         string // --project-root, the last one given
	Candidate    string
	Registry     string
	Reason       string
	AbsorbedInto string
	Positional   string // the draft file or the skill id
	DryRun       bool
}

// parseProjectArgs reads the arguments that follow the verb. It checks the shape of
// the command line and nothing else: whether the root is present, absolute, and real,
// and whether the positional argument is there, are for the verb, which words those
// refusals for itself.
//
// Every value-taking flag requires a following token that does not start with '-',
// so a missing value, or a value that is really the next flag, is a usage error and
// never an opportunity to reinterpret that flag as data; this is what keeps a safety
// flag such as --dry-run from being swallowed. An unrecognized dash-argument and a
// second positional are usage errors naming the token, never dropped, so a mistyped
// --dryrun cannot run for real. "--" ends the options, so a draft path that begins
// with a dash can still be named. When a flag is given twice the last one wins.
//
// The error text is the message without "error: ".
func parseProjectArgs(spec projectArgSpec, args []string) (projectArgs, error) {
	pa := projectArgs{Registry: defaultRegistryPath}
	endOfOptions := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !endOfOptions {
			value := func() (string, error) {
				if i+1 >= len(args) {
					return "", fmt.Errorf("skills %s: flag %q requires a value", spec.verb, arg)
				}
				if strings.HasPrefix(args[i+1], "-") {
					return "", fmt.Errorf("skills %s: flag %q requires a value; got flag token %q", spec.verb, arg, args[i+1])
				}
				i++
				return args[i], nil
			}
			var dst *string
			switch {
			case arg == "--":
				endOfOptions = true
				continue
			case arg == "--dry-run" && spec.dryRun:
				pa.DryRun = true
				continue
			case arg == "--project-root":
				dst = &pa.Root
			case arg == "--registry":
				dst = &pa.Registry
			case arg == "--candidate" && spec.candidate:
				dst = &pa.Candidate
			case arg == "--reason" && spec.reason:
				dst = &pa.Reason
			case arg == "--absorbed-into" && spec.absorbedInto:
				dst = &pa.AbsorbedInto
			case arg == "--manifest" || arg == "--source-root":
				// Wrapper-injected and unused here.
				if _, err := value(); err != nil {
					return projectArgs{}, err
				}
				continue
			case strings.HasPrefix(arg, "-"):
				return projectArgs{}, fmt.Errorf("skills %s: unknown flag %q", spec.verb, arg)
			}
			if dst != nil {
				v, err := value()
				if err != nil {
					return projectArgs{}, err
				}
				*dst = v
				continue
			}
		}
		if pa.Positional == "" {
			pa.Positional = arg
			continue
		}
		if spec.extraHint != "" {
			return projectArgs{}, fmt.Errorf("skills %s: unexpected extra argument %q (%s)", spec.verb, arg, spec.extraHint)
		}
		return projectArgs{}, fmt.Errorf("skills %s: unexpected extra argument %q", spec.verb, arg)
	}
	return pa, nil
}
