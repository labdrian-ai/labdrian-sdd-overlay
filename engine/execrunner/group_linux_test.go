//go:build linux

package execrunner

import (
	"context"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// readGroup is a shell program that prints the process group it runs in: the fifth field of /proc/self/stat (the id, the
// name, the state, the parent and the group), which splits as five fields while the name of the shell holds no blank.
const readGroup = `read -r _ _ _ _ g _ < /proc/self/stat; echo "$g"`

// X4 (T4y, `36d5bc62df38-4` F1): a program the runner starts runs in the process group of its caller, with no group of its
// own, so a kill sent to the caller's group reaches it. The runner is New() and nothing else: the check of the regression
// that gave a program a group of its own, which a kill of the caller's group no longer reached.
func TestAProgramRunsInTheCallersProcessGroup(t *testing.T) {
	overlaytest.RequirePrograms(t, "/bin/sh")
	want := strconv.Itoa(syscall.Getpgrp())

	run, err := New().Run(context.Background(), "/bin/sh", "-c", readGroup)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	stdout, _, err := New().Output(context.Background(), []string{"PATH="}, "/bin/sh", "-c", readGroup)
	if err != nil {
		t.Fatalf("Output: %v", err)
	}

	if got := strings.TrimSpace(string(run)); got != want {
		t.Errorf("Run: the program is in process group %q, want the caller's, %s", got, want)
	}
	if got := strings.TrimSpace(string(stdout)); got != want {
		t.Errorf("Output: the program is in process group %q, want the caller's, %s", got, want)
	}
}
