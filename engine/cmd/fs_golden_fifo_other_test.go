//go:build !unix

package main

// fifo skips the case: a named pipe cannot be made where syscall.Mkfifo is not defined.
func (w *registryWorld) fifo(rel string) {
	w.t.Helper()
	w.t.Skip("named pipes cannot be made on this platform")
}
