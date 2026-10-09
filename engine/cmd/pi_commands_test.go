package main

import (
	"context"
	"errors"
	"sync"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pipkg"
)

// fakePiCommands is the CommandRunner of every test of this package: none of them can start a
// real `pi`, because the program is wired with the process adapter only in main and these tests
// call the cores with this one. It records what it was asked to run, and fails the way a missing
// binary does.
type fakePiCommands struct {
	mu   sync.Mutex
	runs [][]string
}

func (f *fakePiCommands) LookPath(name string) (string, error) {
	return "/fake/bin/" + name, nil
}

func (f *fakePiCommands) Run(_ context.Context, bin string, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.runs = append(f.runs, append([]string{bin}, args...))
	return nil, errors.New("fork/exec " + bin + ": no such file or directory")
}

// noPi is what a test that is not about the commands of the Pi adapter hands the cores.
func noPi() *fakePiCommands { return &fakePiCommands{} }

// noGit is the git of a test of the command: the overlays these tests build are plain
// directories, so nothing is a repository and git is never started (the tests of cmd import no
// process adapter).
func noGit() pipkg.SourceRepo { return pipkg.NoRepository{} }

// scriptedPiCommands is fakePiCommands whose runs succeed, so a test can watch a whole lifecycle
// step reach the port.
type scriptedPiCommands struct {
	fakePiCommands
	missing bool
}

func (f *scriptedPiCommands) LookPath(name string) (string, error) {
	if f.missing {
		return "", errors.New("not installed")
	}
	return f.fakePiCommands.LookPath(name)
}

func (f *scriptedPiCommands) Run(_ context.Context, bin string, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.runs = append(f.runs, append([]string{bin}, args...))
	return nil, nil
}
