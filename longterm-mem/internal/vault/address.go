package vault

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// allocateScript is the vault-relative address allocator entrypoint (D7): a real shell entrypoint
// (shebang and exec bit), so it is exec'd directly through Runner.Run, matching setup-retrieve.sh's
// convention (3a.4), never through RunInterpreted.
const allocateScript = "scripts/allocate-address.sh"

// allocateTimeout bounds a single allocate-address.sh call (D8's convention for vault subprocess calls).
const allocateTimeout = 10 * time.Second

// AddressAllocator hands out the next free address of a vault by running the vault's own allocator
// script, which advances the counter under a lock. It is the adapter behind the AddressAllocator port
// that internal/promote owns (promote does not import this package; the composition root wires the two).
type AddressAllocator struct {
	// Root is the vault checkout the script belongs to and runs in.
	Root string
}

// NextAddress runs scripts/allocate-address.sh from the vault root and returns the address it printed,
// without the newline. A script that cannot run, exits non-zero or prints nothing is an error.
//
// The errors carry no package prefix of their own: the caller that owns the operation says what it was
// doing.
func (a AddressAllocator) NextAddress() (string, error) {
	runner := &Runner{Root: a.Root}
	ctx, cancel := context.WithTimeout(context.Background(), allocateTimeout)
	defer cancel()

	stdout, stderr, exitCode, err := runner.Run(ctx, allocateScript)
	if err != nil {
		return "", fmt.Errorf("allocate address: %w", err)
	}
	if exitCode != 0 {
		return "", fmt.Errorf("%s exited %d: %s", allocateScript, exitCode, strings.TrimSpace(string(stderr)))
	}
	address := strings.TrimSpace(string(stdout))
	if address == "" {
		return "", fmt.Errorf("%s produced no address", allocateScript)
	}
	return address, nil
}
