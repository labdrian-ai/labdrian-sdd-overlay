package main

// roles skill-paths (labdrian-orchestrator-opencode T43 commit a, S8 L117 b): not a verb a person runs; the
// OpenCode runtime-parity plugin calls it on the prompt of a subagent launch. It reads the prompt on stdin and
// prints it with each bare skill id under '## Skills to load before work' made the absolute path of its
// SKILL.md under --skills-root (gate.RewriteSkillPaths). With --agent reviewer or polisher, the roles that read
// only their candidate, it rewrites that section alone (gate.RewriteListedSkillPaths). It launches nothing and
// stores nothing.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gate"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
)

// parseSkillPathsArgs reads --skills-root and the optional --agent, the flags of the verb.
func parseSkillPathsArgs(args []string) (root, agent string, err error) {
	rootGiven, agentGiven := false, false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a != "--skills-root" && a != "--agent" {
			if strings.HasPrefix(a, "-") {
				return "", "", fmt.Errorf("unknown flag %q", a)
			}
			return "", "", fmt.Errorf("unexpected argument %q", a)
		}
		if i+1 >= len(args) || a == "--agent" && args[i+1] == "" {
			return "", "", fmt.Errorf("%s requires a value", a)
		}
		given := &rootGiven
		if a == "--agent" {
			given = &agentGiven
		}
		if *given {
			return "", "", fmt.Errorf("%s given twice", a)
		}
		i++
		*given = true
		if a == "--agent" {
			agent = args[i]
		} else {
			root = args[i]
		}
	}
	if root == "" {
		return "", "", fmt.Errorf("--skills-root is required")
	}
	if !filepath.IsAbs(root) {
		return "", "", fmt.Errorf("--skills-root must be an absolute path, got %q", root)
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return "", "", fmt.Errorf("--skills-root %s is not a directory", root)
	}
	return root, agent, nil
}

// runRolesSkillPaths implements 'roles skill-paths --skills-root <dir> [--agent <id>]': exit 0 printed, 2 prompt refused
// (one line on stderr, nothing on stdout), 1 usage error or any other failure. Every exit is followed by a
// return, because tests inject a non-terminating exit.
func runRolesSkillPaths(args []string, stdin io.Reader, stdout, stderr io.Writer, exit func(int)) {
	fail := func(code int, err error) {
		fmt.Fprintf(stderr, "error: roles skill-paths: %v\n", err)
		exit(code)
	}
	root, agent, err := parseSkillPathsArgs(args)
	if err != nil {
		fail(1, err)
		return
	}
	prompt, err := io.ReadAll(stdin)
	if err != nil {
		fail(1, fmt.Errorf("read the prompt: %w", err))
		return
	}
	rewrite := gate.RewriteSkillPaths
	if roles.ReadsOnlyItsCandidate(agent) {
		rewrite = gate.RewriteListedSkillPaths
	}
	out, err := rewrite(string(prompt), root, isRegularFile)
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
