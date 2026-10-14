package register

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/runtimes"
)

// InstallStateStore is where the record of what longterm-mem installed is kept: the dependency a Writer is given
// for the one effect that must follow a write to a runtime's configuration (install-state.json, D9). A runtime's
// own file and this record are two effects, and the window between them is the one failure a re-run cannot recover
// from, so installWithRollback exists to undo the first when the second fails. A test puts a failure exactly in
// that window by handing the Writer a store whose Save fails, without depending on how a filesystem happens to
// fail.
//
// That the window can be met outside a test is the reason to exercise it. This comment's ancestor claimed the
// rollback could never run, on the reasoning that anything making the state directory unwritable would first break
// the load; it was false, and the counter-example is worth keeping: a MISSING record loads as an empty one and no
// error, so a state directory at mode 0o500 (readable and searchable, not writable) loads perfectly and only fails
// later, at the save. Every fresh install against a directory the user, a restrictive umask or a partially restored
// backup left unwritable takes exactly that path (TestJSONInstall_RollbackIsReachableWithoutStubbingTheSave pins it
// with no stub at all). An error path documented as unreachable is a path nobody writes a test for.
type InstallStateStore interface {
	// Load reads the record kept for stateDir. A record nobody has written yet is an empty one, not an error.
	Load(stateDir string) (*InstallState, error)
	// Save keeps state as the record of stateDir, replacing what was there.
	Save(stateDir string, state *InstallState) error
}

// FileInstallState keeps the record in stateDir/install-state.json, through durable.WriteJSON.
type FileInstallState struct{}

var _ InstallStateStore = FileInstallState{}

func (FileInstallState) path(stateDir string) string {
	return filepath.Join(stateDir, installStateFileName)
}

// Load reads install-state.json in stateDir (LoadInstallState).
func (s FileInstallState) Load(stateDir string) (*InstallState, error) {
	return LoadInstallState(s.path(stateDir))
}

// Save writes install-state.json in stateDir (InstallState.Save).
func (s FileInstallState) Save(stateDir string, state *InstallState) error {
	return state.Save(s.path(stateDir))
}

// errNoInstallState is what a Writer built without a store answers, before it has touched a runtime's
// configuration: nothing could be recorded of what it did, and a config entry with no record is exactly the shape
// every later run refuses. Like every error under the per-target wrap of writer.go it names no command: the layer
// that knows the direction says "register: <target>: " or "unregister: <target>: " in front of it.
var errNoInstallState = errors.New("no install-state store was given, so nothing was recorded and nothing was written")

// Writer writes longterm-mem's MCP entry into the configuration of the runtimes, and takes it out again, recording
// what it did in the store it is given. It owns no state of its own: the composition root builds one over the
// store it chose.
type Writer struct {
	// State is where the record of what was installed is kept. A Writer without one writes nothing.
	State InstallStateStore
}

// A Writer is what the registration use case writes through (runtimes.Registrar).
var _ runtimes.Registrar = Writer{}

// Register installs (or reinstalls) the entry into target's configuration at configRoot, recording its fingerprint
// under stateDir: the writer of the runtime target names (RegisterClaude, RegisterOpencode, RegisterCodex,
// RegisterPi). A target it does not know is an error that names it.
func (w Writer) Register(target, configRoot, stateDir, binary string) error {
	switch target {
	case claudeTarget:
		return w.RegisterClaude(configRoot, stateDir, binary)
	case opencodeTarget:
		return w.RegisterOpencode(configRoot, stateDir, binary)
	case codexTarget:
		return w.RegisterCodex(configRoot, stateDir, binary)
	case piTarget:
		return w.RegisterPi(configRoot, stateDir, binary)
	default:
		return fmt.Errorf("register: %s: unknown target", target)
	}
}
