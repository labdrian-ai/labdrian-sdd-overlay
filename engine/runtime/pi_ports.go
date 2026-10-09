package runtime

import (
	"context"
	"time"
)

// CommandRunner is how the Pi adapter reaches the `pi` CLI: the one process the adapter starts.
// The adapter owns the port; the process adapter (engine/execrunner) is wired by the composition
// root, and a test hands the adapter a fake, so no test of the adapter can start a real `pi` --
// one that did removed a freshly installed package of a developer's machine.
type CommandRunner interface {
	// LookPath is the path of the program name, or an error when it is not installed.
	LookPath(name string) (string, error)
	// Run starts bin with args, a fixed vector that is never interpreted by a shell, waits for
	// it, and returns what it printed on both streams, also when it failed. A program still
	// running when ctx ends is stopped and the error says why.
	Run(ctx context.Context, bin string, args ...string) ([]byte, error)
}

// PackageBuilder is how the Pi adapter builds and checks the labdrian-pi package. The adapter
// owns the port; engine/pipkg is its adapter (pipkg.Packages), wired by the composition root with
// the registry reader and the options of the run.
type PackageBuilder interface {
	// Build writes the package built from the overlay and the registry into destDir.
	Build(overlayRoot, registryPath, destDir string) error
	// Check compares the package in destDir against the sources it would be built from. The
	// disclosure says what it compared against and is owed whether or not it found drift; a
	// non-nil error names the drift, or why the comparison could not be made.
	Check(overlayRoot, registryPath, destDir string) (disclosure string, err error)
}

// PiPorts are the two things the Pi adapter reaches outside itself, besides the files of ~/.pi.
type PiPorts struct {
	Commands CommandRunner
	Packages PackageBuilder
}

// PiOptions are the choices the composition root hands the Pi adapter.
type PiOptions struct {
	// SkipSubagents turns off the probe and the install of the Pi Subagents extension, for
	// environments that manage it separately.
	SkipSubagents bool
	// CommandTimeout is how long one `pi` command may run before it is stopped. Zero means
	// DefaultPiCommandTimeout.
	CommandTimeout time.Duration
}

// DefaultPiCommandTimeout is the deadline of one `pi` command. `pi install npm:...` fetches a
// package over the network, so it is minutes, not seconds; a command that is still running
// after this long is stopped and reported as failed instead of leaving the lifecycle step
// hanging for good.
const DefaultPiCommandTimeout = 5 * time.Minute

func (o PiOptions) commandTimeout() time.Duration {
	if o.CommandTimeout > 0 {
		return o.CommandTimeout
	}
	return DefaultPiCommandTimeout
}
