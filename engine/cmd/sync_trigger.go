package main

// The 'sync-trigger' subcommand: the detached longterm-mem sync a hook starts (engine/synctrigger runs it).

import (
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/synctrigger"
)

// defaultStateDirName is the overlay state root under $HOME, used when
// --state-dir is not given.
const defaultStateDirName = ".labdrian-overlay"

// runSyncTrigger implements the 'sync-trigger --event <e> --cwd <dir>
// [--state-dir <dir>] [--child]' subcommand.
func runSyncTrigger(p process, d deps, args []string) {
	runSyncTriggerCore(d, args, p.exit)
}

// runSyncTriggerCore is the testable core of the sync-trigger subcommand.
// It never rejects its own argv: any invalid or missing flag is left to
// synctrigger.Run to classify as error:usage, so this always exits 0
// (R-003) -- the same "core takes an injected exit" shape as
// runRuntimeCore above, but with a fixed exit(0) rather than a computed
// one, because sync-trigger has no failure that is allowed to propagate.
func runSyncTriggerCore(d deps, args []string, exit func(int)) {
	o, isChild := parseSyncTriggerArgs(args)
	if o.StateDir == "" {
		if home, err := d.homeDir(); err == nil {
			o.StateDir = filepath.Join(home, defaultStateDirName)
		}
	}

	if isChild {
		synctrigger.RunChild(o)
		exit(0)
		return
	}
	o.ChildArgv = syncTriggerChildArgv
	synctrigger.Run(o)
	exit(0)
}

// syncTriggerChildArgv is the command line of the detached child the parent starts: this
// command again, with the verb's own flags and --child. It is given to synctrigger.Options, which
// starts the child with it and knows nothing of what it says; parseSyncTriggerArgs is what reads
// it back.
func syncTriggerChildArgv(event, cwd, stateDir string) []string {
	return []string{"sync-trigger", "--event", event, "--cwd", cwd, "--state-dir", stateDir, "--child"}
}

// parseSyncTriggerArgs extracts sync-trigger flags into synctrigger.Options
// and reports whether "--child" was present. It deliberately does not
// validate values (missing/unknown flags simply leave fields empty) --
// synctrigger.Run and RunChild own that validation and both always return
// 0, so a parse error here would only duplicate a check that already
// cannot fail the caller.
func parseSyncTriggerArgs(args []string) (synctrigger.Options, bool) {
	var o synctrigger.Options
	child := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--event":
			i++
			if i < len(args) {
				o.Event = args[i]
			}
		case "--cwd":
			i++
			if i < len(args) {
				o.Cwd = args[i]
			}
		case "--state-dir":
			i++
			if i < len(args) {
				o.StateDir = args[i]
			}
		case "--child":
			child = true
		}
	}
	return o, child
}
