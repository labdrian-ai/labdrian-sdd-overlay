package installer_test

// The record the routing pins compare with: testdata/routing_pins.golden, one entry per pin,
//
//	@@@ <name>
//	<what the script did, as pinText or pinRunText wrote it>
//
// The record was made once, from bin/labdrian-overlay as it was before its routing moved into the
// engine, and is read here and nowhere changed: there is no flag that rewrites it, because a
// record the test can rewrite is a record that blesses whatever the program does. A pin that
// must change is edited by hand, and the diff says which.

import (
	_ "embed"
	"fmt"
	"strings"
	"testing"
)

//go:embed testdata/routing_pins.golden
var routingPinsGolden string

// recordedPins is the record by name.
var recordedPins = splitRecord(routingPinsGolden)

// splitRecord reads the record: the text of each entry by its name.
func splitRecord(record string) map[string]string {
	entries := make(map[string]string)
	for _, entry := range strings.Split("\n"+record, "\n@@@ ")[1:] {
		name, text, ok := strings.Cut(entry, "\n")
		if !ok {
			continue
		}
		// The separator took the line end of the entry before it; every entry ends with one.
		entries[name] = strings.TrimSuffix(text, "\n") + "\n"
	}
	return entries
}

// pinText is a text a pin records: it always ends with a line end, and an empty one says so.
func pinText(text string) string {
	if text == "" {
		return "(nothing)\n"
	}
	if !strings.HasSuffix(text, "\n") {
		return text + "\\ no newline at end\n"
	}
	return text
}

// pinRunText is a run as a pin records it: how it ended, then each stream.
func pinRunText(r pinRun) string {
	return fmt.Sprintf("exit %d\n--- stdout\n%s--- stderr\n%s", r.Exit, pinText(r.Stdout), pinText(r.Stderr))
}

// pin compares what the script did with the record of what it did.
func pin(t *testing.T, name, got string) {
	t.Helper()
	want, ok := recordedPins[name]
	if !ok {
		t.Fatalf("no pin named %q in testdata/routing_pins.golden", name)
	}
	if got != want {
		t.Errorf("%s differs from its pin\n--- got\n%s--- pinned\n%s", name, got, want)
	}
}
