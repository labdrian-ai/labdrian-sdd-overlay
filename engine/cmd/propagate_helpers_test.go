package main

import (
	"io"
	"os"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/propagator/app"
)

// funcStore is a RegistryStore over two functions, so a test can hand the command a registry made
// of closures. A read that fails with a "does not exist" error is the absent registry the port
// reports as a NotFoundError, as the file adapter does; every other error is a failure to read.
type funcStore struct {
	read  func(string) ([]byte, error)
	write func(string, []byte, os.FileMode) error
}

func (s funcStore) Read(path string) ([]byte, error) {
	b, err := s.read(path)
	if err != nil && os.IsNotExist(err) {
		return nil, &app.NotFoundError{Err: err}
	}
	return b, err
}

func (s funcStore) Write(path string, content []byte) error {
	return s.write(path, content, 0o644)
}

// runPropagateFuncs runs the command over a registry made of the two functions. The reader serves
// the contract file as well, as the one file system a test has. A test prefers it when a registry
// of closures is all it needs; one that needs the real file store or a fake of the port itself
// calls propagateCommand, which this only wraps.
func runPropagateFuncs(
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	readFile func(string) ([]byte, error),
	writeFile func(string, []byte, os.FileMode) error,
	exit func(int),
) {
	propagateCommand(args, stdout, stderr, funcStore{read: readFile, write: writeFile}, readFile, exit)
}
