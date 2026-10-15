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
//
// The record is held to its shape: a header that names no entry, a name twice, text before the
// first header, and an entry no test compares are each a failure and not a pin that quietly
// stopped pinning (TestMain calls checkEveryPinWasConsumed).

import (
	_ "embed"
	"flag"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
)

//go:embed testdata/routing_pins.golden
var routingPinsGolden string

// The second record is the registry pins (registry_pins_repair_test.go and
// registry_pins_doctor_test.go): what the script does to .atl/skill-registry.md and what doctor
// says of it, in testdata/registry_pins.golden, read and kept by the rules above.
//
//go:embed testdata/registry_pins.golden
var registryPinsGolden string

// The third record is the longterm-mem pins (longterm_pins_install_test.go and
// longterm_pins_uninstall_test.go): which targets the script records as installed, and when it removes
// the shared binary and the engine's registration record, in testdata/longterm_pins.golden, read and
// kept by the rules above.
//
//go:embed testdata/longterm_pins.golden
var longtermPinsGolden string

// recordedPins is the records by name. A record that cannot be read stops the test binary at
// start, before any test could pass over it.
var recordedPins = mustParseRecords(map[string]string{
	"routing_pins.golden":  routingPinsGolden,
	"registry_pins.golden": registryPinsGolden,
	"longterm_pins.golden": longtermPinsGolden,
})

// mustParseRecords reads the records, and panics with what is wrong with them, a name in two of
// them included.
func mustParseRecords(records map[string]string) map[string]string {
	names := make([]string, 0, len(records))
	for name := range records {
		names = append(names, name)
	}
	sort.Strings(names)
	all := make(map[string]string)
	for _, file := range names {
		entries, err := parseRecord(records[file])
		if err != nil {
			panic(file + ": " + err.Error())
		}
		for name, text := range entries {
			if _, twice := all[name]; twice {
				panic(fmt.Sprintf("%s: entry %q is in another record too", file, name))
			}
			all[name] = text
		}
	}
	return all
}

// parseRecord reads the record: the text of each entry by its name. It refuses text before the
// first header, a header with no name, a header with no text after it (a header that was mistyped
// or cut off), and a name that is there twice.
func parseRecord(record string) (map[string]string, error) {
	entries := make(map[string]string)
	parts := strings.Split("\n"+record, "\n@@@ ")
	if before := strings.TrimSpace(parts[0]); before != "" {
		return nil, fmt.Errorf("text before the first entry: %q", before)
	}
	for i, entry := range parts[1:] {
		name, text, ok := strings.Cut(entry, "\n")
		switch {
		case name == "":
			return nil, fmt.Errorf("an entry has no name")
		case !ok || text == "":
			return nil, fmt.Errorf("entry %q has no text", name)
		}
		if _, twice := entries[name]; twice {
			return nil, fmt.Errorf("entry %q is there twice", name)
		}
		// The separator took the line end of the entry before it, so every entry but the last gets
		// it back, and an entry that ends with blank lines keeps them; every entry ends with a line
		// end, the last one's being the record's own.
		if i < len(parts)-2 || !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		entries[name] = text
	}
	return entries, nil
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

// consumed is the entries of the record that a test has compared.
var consumed = struct {
	sync.Mutex
	names map[string]bool
}{names: map[string]bool{}}

// pin compares what the script did with the record of what it did.
func pin(t *testing.T, name, got string) {
	t.Helper()
	want, ok := recordedPins[name]
	if !ok {
		t.Fatalf("no pin named %q in the pin records of testdata", name)
	}
	consumed.Lock()
	consumed.names[name] = true
	consumed.Unlock()
	if got != want {
		t.Errorf("%s differs from its pin\n--- got\n%s--- pinned\n%s", name, got, want)
	}
}

// checkEveryPinWasConsumed is what TestMain asks after the tests have passed: an entry of the
// record that no test compared is a pin whose test was renamed or removed, and it would go on
// looking like a guarantee. It asks nothing of a run that could not have compared them all (-short
// skips the integration tests that hold the pins, and -run chooses some of the tests).
func checkEveryPinWasConsumed() error {
	if testing.Short() {
		return nil
	}
	if run := flag.Lookup("test.run"); run != nil && run.Value.String() != "" {
		return nil
	}
	consumed.Lock()
	defer consumed.Unlock()
	var orphans []string
	for name := range recordedPins {
		if !consumed.names[name] {
			orphans = append(orphans, name)
		}
	}
	if len(orphans) == 0 {
		return nil
	}
	sort.Strings(orphans)
	return fmt.Errorf("the pin records of testdata have entries no test compares (a pin was renamed or removed, or its test did not run): %s", strings.Join(orphans, ", "))
}

func TestRoutingPinRecordIsWellFormed(t *testing.T) {
	if len(recordedPins) == 0 {
		t.Fatal("the record of the routing pins is empty")
	}
}

func TestRoutingPinRecordRefusesWhatIsNotARecord(t *testing.T) {
	for _, tc := range []struct {
		name    string
		record  string
		wantErr string
	}{
		{"a record of entries", "@@@ a\nexit 0\n@@@ b\n(nothing)\n", ""},
		{"nothing at all", "", ""},
		{"text before the first entry", "stray\n@@@ a\nexit 0\n", "text before the first entry"},
		{"a header with no text", "@@@ a\nexit 0\n@@@ b", `entry "b" has no text`},
		{"a header with an empty text", "@@@ a\n@@@ b\nexit 0\n", `entry "a" has no text`},
		{"a header with no name", "@@@ \nexit 0\n", "an entry has no name"},
		{"a name twice", "@@@ a\nexit 0\n@@@ a\nexit 1\n", `entry "a" is there twice`},
		{"a header mistyped so that it is text of the entry before", "@@ b\n@@@ a\nexit 0\n", "text before the first entry"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseRecord(tc.record)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Errorf("parseRecord: %v, want the record read", err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Errorf("parseRecord error = %v, want one that says %q", err, tc.wantErr)
			}
		})
	}
}

func TestRoutingPinRecordIsReadWhole(t *testing.T) {
	got, err := parseRecord("@@@ a\nexit 0\n--- stdout\nx\n@@@ b\n(nothing)\n")
	if err != nil {
		t.Fatal(err)
	}
	if got["a"] != "exit 0\n--- stdout\nx\n" || got["b"] != "(nothing)\n" || len(got) != 2 {
		t.Errorf("parseRecord = %q, want each entry with its own text and its line end", got)
	}
}

func TestRoutingPinRecordKeepsTheBlankLinesAnEntryEndsWith(t *testing.T) {
	got, err := parseRecord("@@@ a\nx\n\n\n@@@ b\ny\n\n@@@ c\nz\n")
	if err != nil {
		t.Fatal(err)
	}
	if got["a"] != "x\n\n\n" || got["b"] != "y\n\n" || got["c"] != "z\n" || len(got) != 3 {
		t.Errorf("parseRecord = %q, want the blank lines at the end of each entry kept", got)
	}
}
