package main

// The paused edit gate denies a fixed list of tools, and three prose copies
// retype it: the README, the help text, and the Claude Code cancellation
// declaration. Each copy is checked against projection.EditTools, the list the
// gate itself uses, so a tool added to the gate cannot be left out of the words.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
)

func TestEveryCopyOfTheGatedEditToolListNamesEveryTool(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	d, err := capability.Declare(capability.TargetClaude)
	if err != nil {
		t.Fatal(err)
	}
	cancellation := ""
	for _, c := range d.Claims {
		if c.Capability == capability.Cancellation {
			cancellation = c.Detail
		}
	}
	if cancellation == "" {
		t.Fatal("the Claude Code declaration has no cancellation claim")
	}
	copies := map[string]string{
		"README":                  string(readme),
		"usage()":                 captureUsage(t),
		"the cancellation detail": cancellation,
	}
	tools := projection.EditTools()
	if len(tools) == 0 {
		t.Fatal("EditTools() is empty")
	}
	for name, text := range copies {
		for _, tool := range tools {
			if !strings.Contains(text, tool) {
				t.Errorf("%s does not mention the gated edit tool %q", name, tool)
			}
		}
	}
}
