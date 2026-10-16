package fieldrule

import "testing"

func TestCheckStringArray(t *testing.T) {
	// no-break space and ideographic space, built from their code points so the source holds no
	// invisible character.
	unicodeSpace := string(rune(0xA0)) + string(rune(0x3000))
	for _, tc := range []struct {
		name     string
		field    string
		values   []string
		minItems int
		want     string // the error text, or "" for no error
	}{
		{"a null array is refused even when no item is required", "constraints", nil, 0, "constraints must be a non-null string array"},
		{"a null array is refused before the count is looked at", "stages", nil, 1, "stages must be a non-null string array"},
		{"an empty array is fine when no item is required", "non_goals", []string{}, 0, ""},
		{"an empty array is refused when an item is required", "acceptance_criteria", []string{}, 1, "acceptance_criteria must contain at least 1 item(s)"},
		{"the count in the message is the minimum", "tests", []string{"a"}, 3, "tests must contain at least 3 item(s)"},
		{"exactly the minimum is enough", "tests", []string{"a", "b"}, 2, ""},
		{"more than the minimum is fine", "tests", []string{"a", "b", "c"}, 1, ""},
		{"a blank item is named by its position, counted from 1", "stages", []string{"a", "", "c"}, 1, "stages item 2 must be non-blank"},
		{"the first blank item is the one reported", "stages", []string{"a", "  ", ""}, 1, "stages item 2 must be non-blank"},
		{"an item of white space only is blank", "stages", []string{"\t\n "}, 1, "stages item 1 must be non-blank"},
		{"an item of Unicode white space only is blank", "stages", []string{unicodeSpace}, 1, "stages item 1 must be non-blank"},
		{"the count is checked before the items", "stages", []string{""}, 2, "stages must contain at least 2 item(s)"},
		{"an item is kept as authored: white space around text is not blank", "stages", []string{"  a  "}, 1, ""},
		{"an array of one empty item is refused even when no item is required", "out_of_scope", []string{""}, 0, "out_of_scope item 1 must be non-blank"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckStringArray(tc.field, tc.values, tc.minItems)
			switch {
			case tc.want == "" && err != nil:
				t.Errorf("CheckStringArray(%q, %q, %d) = %v, want nil", tc.field, tc.values, tc.minItems, err)
			case tc.want != "" && err == nil:
				t.Errorf("CheckStringArray(%q, %q, %d) = nil, want %q", tc.field, tc.values, tc.minItems, tc.want)
			case tc.want != "" && err.Error() != tc.want:
				t.Errorf("CheckStringArray(%q, %q, %d) = %q, want %q", tc.field, tc.values, tc.minItems, err.Error(), tc.want)
			}
		})
	}
}
