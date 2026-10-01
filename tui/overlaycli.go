package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// backendQueryTimeout bounds one read-only question put to the backend. The
// answers are a few lines from a local script; a backend that has not
// answered by now is broken, and the caller fails closed instead of waiting.
const backendQueryTimeout = 15 * time.Second

// overlayCLI is the driven adapter behind the TUI's ports: it answers by
// asking the bash backend, bin/labdrian-overlay, which stays the single
// source of truth for everything it reports. It parses the backend's wire
// formats into the TUI's own types; nothing outside this file knows them.
type overlayCLI struct {
	// root is the overlay repo root that holds bin/labdrian-overlay; empty
	// when it could not be located.
	root string
	// env is the child process environment; nil inherits the caller's, which
	// is what a real run wants (STATE_DIR and HOME must reach the backend
	// unchanged). Tests set it to keep the backend off the real state.
	env []string
}

// newOverlayCLI returns the adapter over the backend under root.
func newOverlayCLI(root string) overlayCLI { return overlayCLI{root: root} }

// Targets implements TargetCatalog with `labdrian-overlay targets`.
func (c overlayCLI) Targets() ([]Target, error) {
	out, err := c.query("targets")
	if err != nil {
		return nil, err
	}
	targets, err := parseTargets(out)
	if err != nil {
		return nil, fmt.Errorf("labdrian-overlay targets: %w", err)
	}
	return targets, nil
}

// query runs one backend subcommand and returns its stdout. A failure carries
// the backend's own stderr, which is where it explains itself.
func (c overlayCLI) query(args ...string) (string, error) {
	if c.root == "" {
		return "", errors.New("could not locate the overlay backend (bin/labdrian-overlay); set OVERLAY_DIR")
	}
	ctx, cancel := context.WithTimeout(context.Background(), backendQueryTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, filepath.Join(c.root, "bin", "labdrian-overlay"), args...)
	cmd.Dir = c.root
	cmd.Env = c.env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		cause := strings.TrimSpace(stderr.String())
		if ctx.Err() != nil {
			cause = fmt.Sprintf("no answer within %s", backendQueryTimeout)
		}
		if cause == "" {
			return "", fmt.Errorf("labdrian-overlay %s: %w", strings.Join(args, " "), err)
		}
		return "", fmt.Errorf("labdrian-overlay %s: %w: %s", strings.Join(args, " "), err, cause)
	}
	return stdout.String(), nil
}

// parseTargets reads the output of `labdrian-overlay targets`: one
// "<name><TAB><kind>" line per target. It is strict on purpose. An empty
// catalog, a malformed or blank line, or a repeated name is an error and
// yields no targets at all, because a half-understood catalog is how a front
// end ends up acting on a target nobody chose. A kind it does not know is
// kept verbatim: it is simply not KindCopy, so no file-by-file action
// applies to it.
func parseTargets(output string) ([]Target, error) {
	body := strings.TrimRight(output, "\n")
	if body == "" {
		return nil, errors.New("the backend listed no targets")
	}
	var targets []Target
	seen := map[string]bool{}
	for i, line := range strings.Split(body, "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 2 {
			return nil, fmt.Errorf("line %d %q is not \"<name><TAB><kind>\"", i+1, line)
		}
		name, kind := fields[0], fields[1]
		if name == "" {
			return nil, fmt.Errorf("line %d %q has an empty target name", i+1, line)
		}
		if kind == "" {
			return nil, fmt.Errorf("line %d: target %q has an empty kind", i+1, name)
		}
		if seen[name] {
			return nil, fmt.Errorf("line %d: target %q is listed twice", i+1, name)
		}
		seen[name] = true
		targets = append(targets, Target{Name: name, Kind: TargetKind(kind)})
	}
	return targets, nil
}
