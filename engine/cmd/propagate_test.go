package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/propagator/app"
)

// The words of the command for each error of the use case are the words it has always used; a
// script or a person may match on them. The golden files record them in whole runs, and this
// table pins each one where it is made.
func TestPropagateMessageSaysWhatTheCommandAlwaysSaid(t *testing.T) {
	disk := errors.New("no space left on device")
	for _, c := range []struct {
		name string
		err  error
		want string
	}{
		{"a contract that cannot be read", &app.ContractReadError{Err: disk}, "reading contract file: no space left on device"},
		{"a contract that does not parse", &app.ContractParseError{Err: errors.New("no frontmatter")}, "no frontmatter"},
		{"a registry that cannot be read", &app.RegistryReadError{Path: "/r.md", Err: disk}, "reading registry file: no space left on device"},
		{"a registry that is required", &app.RegistryRequiredError{Path: "/r.md"}, "registry required but not found at /r.md (--require-registry)"},
		{"a row that cannot be scoped", &app.RewriteError{Err: errors.New("bad block")}, "bad block"},
		{"a registry that cannot be written", &app.RegistryWriteError{Path: "/r.md", Err: disk}, "writing registry: no space left on device"},
		{"a registry empty on every attempt", &app.EmptyRegistryError{Path: "/r.md", Attempts: 3},
			"the registry at /r.md read empty on all 3 attempts — refusing to propagate"},
		{"a registry in an inconsistent state", &app.InconsistentRegistryError{Path: "/r.md", Attempts: 3},
			"the registry at /r.md was in an inconsistent state across 3 attempts — refusing to propagate"},
		{"a write that could not be verified", &app.UnverifiedWriteError{Path: "/r.md", Attempts: 3, Err: disk},
			"registry write to /r.md could not be verified after 3 attempts — the verification read itself failed: no space left on device"},
		{"a write that did not persist", &app.LostWriteError{Path: "/r.md", Attempts: 3},
			"registry write to /r.md did not persist after 3 attempts — a concurrent external writer (unrelated to engine propagate) appears to be clobbering it"},
		{"an error the use case does not know", errors.New("something else"), "something else"},
		{"an error wrapped by someone", fmt.Errorf("while propagating: %w", &app.RegistryRequiredError{Path: "/r.md"}),
			"registry required but not found at /r.md (--require-registry)"},
	} {
		if got := propagateMessage(c.err); got != c.want {
			t.Errorf("%s:\n got: %q\nwant: %q", c.name, got, c.want)
		}
	}
}

func TestParsePropagateOptions(t *testing.T) {
	got := parsePropagateOptions([]string{
		"--registry", "/a", "--contract-file", "/c.md", "--registry", "/b",
		"--contract-path", "p.md", "--embedded-contract", "anti-generic-design", "--require-registry", "--unknown",
	})
	want := propagateOptions{
		registry: "/b", contractFile: "/c.md", contractPath: "p.md", embeddedName: "anti-generic-design",
		contractPathExplicit: true, requireRegistry: true,
	}
	if got != want {
		t.Errorf("options\n got: %+v\nwant: %+v", got, want)
	}

	defaults := parsePropagateOptions([]string{"--registry", "/a", "--contract-file"})
	if defaults.contractFile != "" || defaults.contractPath != defaultContractPath || defaults.contractPathExplicit {
		t.Errorf("a dangling flag or an absent path changed the defaults: %+v", defaults)
	}
}

// The file of the contract is read each time the use case asks, so a contract that changes
// between two attempts is seen as it is on the second.
func TestFileContractIsReadEveryTimeItIsAsked(t *testing.T) {
	text, reads := "first", 0
	source := fileContract{path: "/c.md", read: func(path string) ([]byte, error) {
		reads++
		if path != "/c.md" {
			t.Errorf("read %q, want /c.md", path)
		}
		return []byte(text), nil
	}}
	for _, want := range []string{"first", "second"} {
		text = want
		if got, err := source.Text(); err != nil || got != want {
			t.Errorf("Text = %q, %v, want %q", got, err, want)
		}
	}
	if reads != 2 {
		t.Errorf("%d reads, want 2", reads)
	}

	failing := fileContract{path: "/c.md", read: func(string) ([]byte, error) { return nil, errors.New("denied") }}
	if _, err := failing.Text(); err == nil || !strings.Contains(err.Error(), "denied") {
		t.Errorf("Text = %v, want the read error", err)
	}
}

func TestTextContractIsItsText(t *testing.T) {
	if got, err := textContract("a contract").Text(); err != nil || got != "a contract" {
		t.Errorf("Text = %q, %v", got, err)
	}
}
