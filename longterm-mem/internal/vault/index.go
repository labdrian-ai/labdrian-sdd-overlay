package vault

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/durable"
)

const (
	// bm25IndexFile and chunksDir are the two markers Provisioned checks:
	// a vault is provisioned once bin/setup-retrieve.sh has produced a
	// BM25 index and chunked pages (D12, exploration #3121's documented
	// .vault-meta/ contents).
	bm25IndexFile = ".vault-meta/bm25/index.json"
	chunksDir     = ".vault-meta/chunks"

	// setupScript provisions a never-indexed vault (D12), run once before
	// the first refresh. It carries its own shebang and exec bit, so it
	// runs directly (Runner.Run) like any other shell entrypoint.
	setupScript = "bin/setup-retrieve.sh"

	// contextualPrefixScript and bm25IndexScript refresh an
	// already-provisioned vault's index (D12), always with --no-llm so no
	// page body ever egresses. Both are Python entrypoints with no
	// shebang/exec bit, so they run under python3 (Runner.RunInterpreted),
	// matching retrieve.py's convention (2b review).
	contextualPrefixScript = "scripts/contextual-prefix.py"
	bm25IndexScript        = "scripts/bm25-index.py"
	rebuildInterpreter     = "python3"

	// provisionedSentinel marks a provision step that ran to completion.
	// Rebuild writes it only after setupScript exits 0, right before the
	// refresh steps run. Its absence — even with bm25IndexFile and
	// chunksDir present — means provisioning was interrupted (SIGKILL/OOM,
	// disk-full, operator Ctrl-C) partway through, so Provisioned reports
	// false and the next Rebuild re-runs setupScript instead of silently
	// refreshing a half-built vault.
	provisionedSentinel = ".vault-meta/.longterm-mem-provisioned"
)

// Provisioned reports whether vaultRoot has already been fully indexed:
// provisionedSentinel exists (the provision step ran to completion), plus
// bm25IndexFile exists and chunksDir contains at least one entry (D12). The
// sentinel check comes first so a provision step interrupted after leaving
// those markers but before finishing never reads as provisioned. A vault
// failing any check has never been (fully) indexed — Rebuild provisions it
// first (R-005).
func Provisioned(vaultRoot string) bool {
	if _, err := os.Stat(filepath.Join(vaultRoot, provisionedSentinel)); err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(vaultRoot, bm25IndexFile)); err != nil {
		return false
	}
	entries, err := os.ReadDir(filepath.Join(vaultRoot, chunksDir))
	if err != nil {
		return false
	}
	return len(entries) > 0
}

// Rebuild provisions runner's vault first if it has never been fully
// indexed, or unconditionally when force is true (an operator's fix-forward
// path after an interrupted provision left no clean way to detect and
// re-provision), then always refreshes the index (R-005). Any step's
// failure stops the remaining steps immediately and is reported as an error
// that never claims success (R-025).
func Rebuild(ctx context.Context, runner *Runner, force bool) error {
	if force {
		if err := os.Remove(filepath.Join(runner.Root, provisionedSentinel)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("vault: clear provision sentinel: %w", err)
		}
	}

	if force || !Provisioned(runner.Root) {
		_, stderr, exitCode, err := runner.Run(ctx, setupScript, "--no-llm")
		if failErr := stepFailure(setupScript, stderr, exitCode, err); failErr != nil {
			return fmt.Errorf("vault: provision index: %w", failErr)
		}
		if err := writeProvisionedSentinel(runner.Root); err != nil {
			return fmt.Errorf("vault: mark provisioned: %w", err)
		}
	}

	_, stderr, exitCode, err := runner.RunInterpreted(ctx, rebuildInterpreter, contextualPrefixScript, "--all", "--no-llm")
	if failErr := stepFailure(contextualPrefixScript, stderr, exitCode, err); failErr != nil {
		return fmt.Errorf("vault: refresh index: %w", failErr)
	}

	_, stderr, exitCode, err = runner.RunInterpreted(ctx, rebuildInterpreter, bm25IndexScript, "build")
	if failErr := stepFailure(bm25IndexScript, stderr, exitCode, err); failErr != nil {
		return fmt.Errorf("vault: refresh index: %w", failErr)
	}

	return nil
}

// writeProvisionedSentinel durably marks vaultRoot as fully provisioned: it
// creates the sentinel's parent dir, then writes an ISO-8601 timestamp
// through durable.WriteFile, so a process killed mid-write never leaves a
// half-written sentinel that would falsely read as complete.
//
// It used to hand-roll that sequence with a FIXED "<sentinel>.tmp" name and
// no fsync — the two things that make the difference between a sentinel
// that survives a crash and one that only looks like it does. The fixed
// name is the sharper problem: anything left at that exact path by an
// earlier interrupted run (and an interrupted run is precisely what this
// sentinel exists to detect) blocks every future provision permanently,
// since os.WriteFile cannot overwrite, say, a directory. Without the fsync,
// a sentinel written and renamed just before a power loss can come back
// empty on the next boot while still reading as present — the exact false
// "provisioned" the sentinel was added to prevent.
func writeProvisionedSentinel(vaultRoot string) error {
	path := filepath.Join(vaultRoot, provisionedSentinel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return durable.WriteFile(path, []byte(time.Now().UTC().Format(time.RFC3339)), 0o644)
}

// stepFailure turns a Runner-level error or a non-success exit code into
// one error naming script, exit code, and captured stderr; nil when the
// step succeeded. It reuses statusForExitCode (status.go) rather than
// re-deriving a success boundary: only StatusOK (exit 0) counts as
// success, so R-025 never mistakes any other exit — including the
// unrelated not-provisioned sentinel 10 — for a completed step.
func stepFailure(script string, stderr []byte, exitCode int, err error) error {
	if err != nil {
		return err
	}
	if status, mapped := statusForExitCode(exitCode); !mapped || status != StatusOK {
		return fmt.Errorf("%s exited %d: %s", script, exitCode, string(stderr))
	}
	return nil
}
