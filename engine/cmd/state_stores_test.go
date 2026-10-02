package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// The file-backed stores are handed their state home by the composition root, which
// resolves it from the environment once (statestore.Home); the adapters read no
// variable. These pin what a person sees for an unusable environment, which is the
// text each store printed when it resolved the home itself, and where a store lands
// for each way of naming the home. The binding store is pinned in
// workflow_bind_test.go; the workflow log and the role chain are pinned here.
var unusableStateHomes = []struct{ name, xdg, home, detail string }{
	{"relative XDG_STATE_HOME", "relative/path", "/home/someone", `XDG_STATE_HOME "relative/path" is not absolute`},
	{"relative HOME fallback", "", "relative/home", `XDG_STATE_HOME is unset and HOME "relative/home" is not an absolute path`},
	{"unset HOME fallback", "", "", `XDG_STATE_HOME is unset and HOME "" is not an absolute path`},
}

func TestNewWorkflowStoreResolvesTheStateHomeFromTheEnvironment(t *testing.T) {
	for _, tt := range unusableStateHomes {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", tt.xdg)
			t.Setenv("HOME", tt.home)
			want := "workflow store: " + tt.detail
			if store, err := newWorkflowStore(); err == nil || err.Error() != want || store != nil {
				t.Fatalf("newWorkflowStore() = %v, %v, want no store and %q", store, err, want)
			}
		})
	}

	// A log that is not ours is classified, never ignored, so classifying it at all
	// shows the store looked under the state home it was given.
	lands := func(t *testing.T, stateHome string) {
		t.Helper()
		logPath := filepath.Join(stateHome, "labdrian", "workflows", "proj-1", "wf-1.jsonl")
		writeFixtureFile(t, logPath, "hand-written notes\n")
		store, err := newWorkflowStore()
		if err != nil {
			t.Fatalf("newWorkflowStore() = %v, want nil", err)
		}
		loaded, err := store.Load("proj-1", "wf-1")
		if err != nil || loaded.Classification != workflow.ClassificationMalformed {
			t.Fatalf("Load() = %+v, %v, want the log under %s, classified malformed", loaded, err, stateHome)
		}
	}
	t.Run("XDG_STATE_HOME", func(t *testing.T) {
		xdg := t.TempDir()
		t.Setenv("XDG_STATE_HOME", xdg)
		lands(t, xdg)
	})
	t.Run("HOME fallback", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("XDG_STATE_HOME", "")
		t.Setenv("HOME", home)
		lands(t, filepath.Join(home, ".local", "state"))
	})
}

func TestNewRoleChainStoreResolvesTheStateHomeFromTheEnvironment(t *testing.T) {
	for _, tt := range unusableStateHomes {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", tt.xdg)
			t.Setenv("HOME", tt.home)
			want := "role chain store: " + tt.detail
			if store, err := newRoleChainStore(); err == nil || err.Error() != want || store != nil {
				t.Fatalf("newRoleChainStore() = %v, %v, want no store and %q", store, err, want)
			}
		})
	}

	lands := func(t *testing.T, stateHome string) {
		t.Helper()
		store, err := newRoleChainStore()
		if err != nil {
			t.Fatalf("newRoleChainStore() = %v, want nil", err)
		}
		record := rolesHandoffJSON(1, roles.EmptyChainDigest, "shaper", "estimator", "completed", "")
		path, err := store.Append([]byte(record))
		if err != nil {
			t.Fatalf("Append() = %v, want nil", err)
		}
		want := filepath.Join(stateHome, "labdrian", "role-chains", "proj-1", "goal-1", "chain-1", "000001.json")
		if path != want {
			t.Fatalf("Append() path = %q, want %q", path, want)
		}
		if _, err := os.Stat(want); err != nil {
			t.Fatalf("the record is not under %s: %v", stateHome, err)
		}
	}
	t.Run("XDG_STATE_HOME", func(t *testing.T) {
		xdg := t.TempDir()
		t.Setenv("XDG_STATE_HOME", xdg)
		lands(t, xdg)
	})
	t.Run("HOME fallback", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("XDG_STATE_HOME", "")
		t.Setenv("HOME", home)
		lands(t, filepath.Join(home, ".local", "state"))
	})
}

func TestNewClearanceStoreResolvesTheStateHomeFromTheEnvironment(t *testing.T) {
	for _, tt := range unusableStateHomes {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", tt.xdg)
			t.Setenv("HOME", tt.home)
			want := "clearance store: " + tt.detail
			if store, err := newClearanceStore(); err == nil || err.Error() != want || store != nil {
				t.Fatalf("newClearanceStore() = %v, %v, want no store and %q", store, err, want)
			}
		})
	}

	lands := func(t *testing.T, stateHome string) {
		t.Helper()
		store, err := newClearanceStore()
		if err != nil {
			t.Fatalf("newClearanceStore() = %v, want nil", err)
		}
		sha := strings.Repeat("a", 64)
		path, err := store.Path("proj-1", "goal-1", sha)
		if err != nil {
			t.Fatalf("Path() = %v, want nil", err)
		}
		want := filepath.Join(stateHome, "labdrian", "shaper-clearance", "proj-1", "goal-1", sha+".json")
		if path != want {
			t.Fatalf("Path() = %q, want %q", path, want)
		}
	}
	t.Run("XDG_STATE_HOME", func(t *testing.T) {
		xdg := t.TempDir()
		t.Setenv("XDG_STATE_HOME", xdg)
		lands(t, xdg)
	})
	t.Run("HOME fallback", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("XDG_STATE_HOME", "")
		t.Setenv("HOME", home)
		lands(t, filepath.Join(home, ".local", "state"))
	})
}

// The review receipt capture is built over the project root the verb was given, with the
// real git; an empty root is refused instead of looking at whatever directory the process
// happens to be in.
func TestNewReviewReceiptServiceIsBuiltOverTheProjectRoot(t *testing.T) {
	if svc, err := newReviewReceiptService(""); err == nil || svc != nil {
		t.Errorf("newReviewReceiptService(\"\") = %v, %v, want no service and an error", svc, err)
	}
	svc, err := newReviewReceiptService(t.TempDir())
	if err != nil || svc == nil {
		t.Fatalf("newReviewReceiptService(dir) = %v, %v, want a service", svc, err)
	}
	// A project with no openspec/changes has no change to capture into, whatever git says.
	if change, err := svc.DetectActiveChange(); err != nil || change != "" {
		t.Errorf("DetectActiveChange = %q, %v, want none", change, err)
	}
}
