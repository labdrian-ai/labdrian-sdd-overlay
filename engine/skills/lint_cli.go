package skills

import (
	"fmt"
	"io"
	"strings"
)

// RenderLintCore is the testable CLI core for `engine skills lint`. Two
// forms:
//
//   - `lint --rules` prints RenderLintRules() and exits 0.
//   - `lint <path>` reads the file, lints it with LintSkillFile, prints
//     every warning to stdout and every hard error to stderr, and exits 0
//     when there are zero hard errors (warnings alone never block), or 1
//     when at least one hard error is found.
//
// The `labdrian` wrapper always appends `--registry <path> --manifest
// <path> --source-root <path>` after every verb's own arguments (see
// design.md's CLI section). lint needs none of them, so it consumes and
// ignores each recognized flag and its value rather than misparsing the
// value as the lint target path.
//
// `lint` accepts exactly one positional argument (the path). A second
// positional argument, or any unrecognized flag (anything else starting
// with "-"), is rejected with exit 1 and a usage error naming the
// offending argument, instead of being silently dropped: dropping it would
// let a multi-path call like `lint a.md b.md` lint only the first file and
// exit 0 even if a later file has hard errors, which is a false-clean gate
// result (review-b75e4a27b9494ff8 R4-001).
func RenderLintCore(args []string, readFile readFileFn, stdout, stderr io.Writer, exit func(int)) {
	var path string
	rules := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--rules":
			rules = true
		case "--registry", "--manifest", "--source-root":
			if i+1 < len(args) {
				i++
			}
		default:
			if strings.HasPrefix(args[i], "-") {
				fmt.Fprintf(stderr, "error: skills lint: unknown flag %q\n", args[i])
				exit(1)
				return
			}
			if path == "" {
				path = args[i]
				continue
			}
			fmt.Fprintf(stderr, "error: skills lint: unexpected extra argument %q (lint accepts exactly one path)\n", args[i])
			exit(1)
			return
		}
	}

	if rules {
		fmt.Fprint(stdout, RenderLintRules())
		exit(0)
		return
	}

	if path == "" {
		fmt.Fprintln(stderr, "error: skills lint requires a path or --rules")
		exit(1)
		return
	}

	data, err := readFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "error: reading %q: %v\n", path, err)
		exit(1)
		return
	}

	hard, warnings := LintSkillFile(data)
	for _, w := range warnings {
		fmt.Fprintf(stdout, "[lint:%s] %s\n", w.Rule, w.Msg)
	}
	for _, e := range hard {
		fmt.Fprintln(stderr, e.Error())
	}

	if len(hard) > 0 {
		exit(1)
		return
	}
	exit(0)
}
