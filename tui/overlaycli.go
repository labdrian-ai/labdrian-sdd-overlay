package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/tui/internal/domain"
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

// binary is the path of the backend script under the repo root.
func (c overlayCLI) binary() string { return filepath.Join(c.root, "bin", "labdrian-overlay") }

// Run implements domain.Backend: it runs one invocation of the backend script
// from the repo root and returns what it printed, standard output and standard
// error together, and how it ended. There is no deadline: an action (apply,
// self-update) takes as long as it takes, and the operator can see it run.
func (c overlayCLI) Run(args []string) domain.Execution {
	cmd := exec.Command(c.binary(), args...)
	cmd.Dir = c.root
	cmd.Env = c.env
	out, err := cmd.CombinedOutput()
	run := domain.Execution{Output: string(out), Err: err}
	if err != nil {
		// The exit code lets callers tell a hard failure (exit 1) from a
		// degraded/warning result (exit 2); -1 when the process did not exit
		// on its own.
		run.ExitCode = -1
		var exited *exec.ExitError
		if errors.As(err, &exited) {
			run.ExitCode = exited.ExitCode()
		}
	}
	return run
}

// Report implements domain.RepoProbe with a cached-only sync-check (no
// --fetch/--check-origin, per R-001) from the repo root. Whatever the backend
// printed is the report, even when the process exits non-zero: the probe reads
// cached refs, it does not require a clean exit.
func (c overlayCLI) Report() string {
	cmd := exec.Command(c.binary(), "sync-check")
	cmd.Dir = c.root
	cmd.Env = c.env
	out, _ := cmd.CombinedOutput()
	return string(out)
}

// Targets implements TargetCatalog with `labdrian-overlay targets`.
func (c overlayCLI) Targets() ([]domain.Target, error) {
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

// LatestBackup implements BackupQuery with `labdrian-overlay restore --target
// <target> --list`. The backend owns the backup layout and the STATE_DIR it
// lives under, so the TUI no longer reads that directory itself.
func (c overlayCLI) LatestBackup(target string) (domain.Backup, bool, error) {
	out, err := c.query("restore", "--target", target, "--list")
	if err != nil {
		return domain.Backup{}, false, err
	}
	backup, ok, err := parseBackupList(out, target)
	if err != nil {
		return domain.Backup{}, false, fmt.Errorf("labdrian-overlay restore --target %s --list: %w", target, err)
	}
	return backup, ok, nil
}

// backupEntry matches one retained backup in `restore --list`:
// "  <timestamp> (<version>)".
var backupEntry = regexp.MustCompile(`^ {2}(\S+) \((.*)\)$`)

// backendUnknownVersion is how `restore --list` spells a version it could not
// read from a backup's .meta.
const backendUnknownVersion = "unknown"

// parseBackupList reads the output of `restore --target <target> --list`:
// either "No backups available for target '<target>'." or a header naming the
// target followed by one entry per retained backup, newest last. It returns
// the newest. It is strict: anything else, including a listing for another
// target, is an error, because a rollback is only ever offered for a backup
// the backend confirmed.
func parseBackupList(output, target string) (domain.Backup, bool, error) {
	text := strings.TrimSpace(output)
	if text == fmt.Sprintf("No backups available for target '%s'.", target) {
		return domain.Backup{}, false, nil
	}
	lines := strings.Split(text, "\n")
	if want := fmt.Sprintf("Backups for %s (newest last):", target); lines[0] != want {
		return domain.Backup{}, false, fmt.Errorf("unrecognized listing, expected %q first, got %q", want, lines[0])
	}
	if len(lines) < 2 {
		return domain.Backup{}, false, errors.New("the listing names no backups")
	}
	var newest domain.Backup
	for _, line := range lines[1:] {
		m := backupEntry.FindStringSubmatch(line)
		if m == nil {
			return domain.Backup{}, false, fmt.Errorf("unrecognized backup entry %q", line)
		}
		newest = domain.Backup{Timestamp: m[1], Version: m[2]}
		if newest.Version == backendUnknownVersion {
			newest.Version = ""
		}
	}
	return newest, true, nil
}

// query runs one backend subcommand and returns its stdout. A failure carries
// the backend's own stderr, which is where it explains itself.
func (c overlayCLI) query(args ...string) (string, error) {
	if c.root == "" {
		return "", errors.New("could not locate the overlay backend (bin/labdrian-overlay); set OVERLAY_DIR")
	}
	ctx, cancel := context.WithTimeout(context.Background(), backendQueryTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.binary(), args...)
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
func parseTargets(output string) ([]domain.Target, error) {
	body := strings.TrimRight(output, "\n")
	if body == "" {
		return nil, errors.New("the backend listed no targets")
	}
	var targets []domain.Target
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
		targets = append(targets, domain.Target{Name: name, Kind: domain.TargetKind(kind)})
	}
	return targets, nil
}
