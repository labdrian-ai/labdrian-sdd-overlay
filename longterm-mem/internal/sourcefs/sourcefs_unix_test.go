//go:build unix

package sourcefs

import (
	"path/filepath"
	"syscall"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/ingest"
)

// A named pipe is neither a file, a directory nor a link: Lstat reports it as another kind, and ReadFile refuses it
// rather than open it (opening a pipe waits for a writer). Named pipes are a unix thing, so this file builds there
// only.
func TestANamedPipeIsAnotherKindAndIsNotRead(t *testing.T) {
	pipe := filepath.Join(t.TempDir(), "pipe")
	if err := syscall.Mkfifo(pipe, 0o600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}

	if got, err := (Reader{}).Lstat(pipe); err != nil || got != (ingest.Entry{Kind: ingest.EntryOther}) {
		t.Errorf("Lstat = %+v, %v, want another kind and no error", got, err)
	}
	if got, err := (Reader{}).ReadFile(pipe); err == nil || got != nil {
		t.Errorf("ReadFile = %q, %v, want a refusal and no bytes", got, err)
	}
}
