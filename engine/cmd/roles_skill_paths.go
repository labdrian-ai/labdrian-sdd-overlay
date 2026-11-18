package main

// roles skill-paths (labdrian-orchestrator-opencode T43 commit a, S8 L117 b): not a verb a person runs; the
// OpenCode runtime-parity plugin calls it on the prompt of a subagent launch. It reads the prompt on stdin and
// prints it with each bare skill id under '## Skills to load before work' made the absolute path of its
// SKILL.md under --skills-root (gate.RewriteSkillPaths). It launches nothing and stores nothing.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gate"
)

// parseSkillPathsArgs reads --skills-root, the only flag of the verb.
func parseSkillPathsArgs(args []string) (string, error) {
	root, given := "", false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a != "--skills-root" {
			if strings.HasPrefix(a, "-") {
				return "", fmt.Errorf("unknown flag %q", a)
			}
			return "", fmt.Errorf("unexpected argument %q", a)
		}
		if i+1 >= len(args) {
			return "", fmt.Errorf("%s requires a value", a)
		}
		if given {
			return "", fmt.Errorf("%s given twice", a)
		}
		i++
		root, given = args[i], true
	}
	if root == "" {
		return "", fmt.Errorf("--skills-root is required")
	}
	if !filepath.IsAbs(root) {
		return "", fmt.Errorf("--skills-root must be an absolute path, got %q", root)
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return "", fmt.Errorf("--skills-root %s is not a directory", root)
	}
	return root, nil
}

// runRolesSkillPaths implements 'roles skill-paths --skills-root <dir>': exit 0 printed, 2 prompt refused
// (one line on stderr, nothing on stdout), 1 usage error or any other failure. Every exit is followed by a
// return, because tests inject a non-terminating exit.
func runRolesSkillPaths(args []string, stdin io.Reader, stdout, stderr io.Writer, exit func(int)) {
	fail := func(code int, err error) {
		fmt.Fprintf(stderr, "error: roles skill-paths: %v\n", err)
		exit(code)
	}
	root, err := parseSkillPathsArgs(args)
	if err != nil {
		fail(1, err)
		return
	}
	prompt, err := io.ReadAll(stdin)
	if err != nil {
		fail(1, fmt.Errorf("read the prompt: %w", err))
		return
	}
	out, err := gate.RewriteSkillPaths(string(prompt), root, isRegularFile)
	if err != nil {
		fail(2, err)
		return
	}
	if _, err := io.WriteString(stdout, out); err != nil {
		fail(1, fmt.Errorf("write to stdout: %w", err))
		return
	}
	exit(0)
}

// isRegularFile is the port of gate.RewriteSkillPaths: whether path is an existing regular file.
func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
