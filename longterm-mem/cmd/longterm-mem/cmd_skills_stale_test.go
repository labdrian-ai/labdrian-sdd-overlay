package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_DispatchesSkillsStaleSubcommand(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "missing-engram.db")
	t.Setenv(engramDBEnvVar, dbPath)

	var exit int
	stderr := captureStderr(t, func() {
		exit = run([]string{"skills-stale", "--project-root", root, "--project", "fixture-project"})
	})
	if exit != exitEngramUnavailable {
		t.Fatalf("run([skills-stale ...]) = %d, want %d, proving dispatch reaches the detector command", exit, exitEngramUnavailable)
	}
	if !strings.Contains(stderr, "skills-stale") {
		t.Fatalf("skills-stale error does not name the command: %q", stderr)
	}
}

func TestCmdSkillsStale_RequiresAbsoluteProjectRoot(t *testing.T) {
	var exit int
	stderr := captureStderr(t, func() {
		exit = run([]string{"skills-stale", "--project-root", "relative", "--project", "fixture-project"})
	})
	if exit != exitUsage {
		t.Fatalf("relative skills-stale project root exit = %d, want %d", exit, exitUsage)
	}
	if !strings.Contains(stderr, "absolute") {
		t.Fatalf("relative project-root refusal does not explain the absolute-path requirement: %q", stderr)
	}
}
