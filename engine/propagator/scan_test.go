package propagator

import (
	"reflect"
	"testing"
)

// scanned is what a walk over lines gives back, in order.
func scanned(lines []string) []registryLine {
	var out []registryLine
	for scan := newLineScanner(lines); scan.Next(); {
		out = append(out, scan.Line())
	}
	return out
}

func TestLineScannerTellsWhereEveryLineStandsAmongTheMarkerBlocks(t *testing.T) {
	lines := []string{
		"# R",
		"<!-- BEGIN: a (auto-generated) -->",
		"| row | in | block |",
		"<!-- END: a -->",
		"  <!-- BEGIN: indented -->  ",
		"inside the second\r",
		"\t<!-- END: indented -->",
		"after",
	}
	want := []registryLine{
		{Index: 0, Text: "# R", Trimmed: "# R"},
		{Index: 1, Text: lines[1], Trimmed: lines[1], Opens: true, InBlock: true},
		{Index: 2, Text: lines[2], Trimmed: lines[2], InBlock: true},
		{Index: 3, Text: lines[3], Trimmed: lines[3], Closes: true},
		{Index: 4, Text: lines[4], Trimmed: "<!-- BEGIN: indented -->", Opens: true, InBlock: true},
		{Index: 5, Text: "inside the second\r", Trimmed: "inside the second", InBlock: true},
		{Index: 6, Text: lines[6], Trimmed: "<!-- END: indented -->", Closes: true},
		{Index: 7, Text: "after", Trimmed: "after"},
	}
	if got := scanned(lines); !reflect.DeepEqual(got, want) {
		t.Errorf("scanned\n%+v\nwant\n%+v", got, want)
	}
}

func TestLineScannerKeepsWhatItDoesNotUnderstand(t *testing.T) {
	for _, tc := range []struct {
		name   string
		lines  []string
		inside []bool
	}{
		{"a block that is never closed holds every line after its opening one",
			[]string{"a", "<!-- BEGIN: x -->", "b", "c"}, []bool{false, true, true, true}},
		{"a closing marker with no opening one closes nothing",
			[]string{"a", "<!-- END: x -->", "b"}, []bool{false, false, false}},
		{"a second opening marker does not nest: one closing marker leaves the block",
			[]string{"<!-- BEGIN: x -->", "<!-- BEGIN: y -->", "inner", "<!-- END: y -->", "outside"}, []bool{true, true, true, false, false}},
		{"a marker is a marker only at the start of the line",
			[]string{"text <!-- BEGIN: x -->", "after"}, []bool{false, false}},
		{"a marker needs its colon", []string{"<!-- BEGIN x -->", "after"}, []bool{false, false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var inside []bool
			for _, l := range scanned(tc.lines) {
				inside = append(inside, l.InBlock)
			}
			if !reflect.DeepEqual(inside, tc.inside) {
				t.Errorf("InBlock = %v, want %v", inside, tc.inside)
			}
		})
	}
}

func TestLineScannerWalksNoLinesOnceAndEmptyInputOnce(t *testing.T) {
	if got := scanned(nil); len(got) != 0 {
		t.Errorf("no lines gave %+v", got)
	}
	if got := scanned([]string{""}); len(got) != 1 || got[0].Text != "" || got[0].Trimmed != "" || got[0].InBlock {
		t.Errorf("one empty line gave %+v", got)
	}
	scan := newLineScanner([]string{"only"})
	if !scan.Next() || scan.Next() || scan.Next() {
		t.Error("a scanner over one line must say true once and false after")
	}
}
