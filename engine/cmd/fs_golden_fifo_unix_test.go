//go:build unix

package main

import (
	"os"
	"path/filepath"
	"syscall"
)

// fifo makes rel a named pipe.
func (w *registryWorld) fifo(rel string) {
	w.t.Helper()
	if err := os.MkdirAll(filepath.Dir(w.path(rel)), 0o755); err != nil {
		w.t.Fatal(err)
	}
	if err := syscall.Mkfifo(w.path(rel), 0o644); err != nil {
		w.t.Fatal(err)
	}
}
